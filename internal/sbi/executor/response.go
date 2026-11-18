package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

var problemCausePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// safeResponseHeaderNames is the closed set of response headers returned to
// callers. Credential, cookie and tracking headers are never projected.
var safeResponseHeaderNames = map[string]struct{}{
	"location":                      {},
	"retry-after":                   {},
	"3gpp-sbi-callback":             {},
	"3gpp-sbi-target-apiroot":       {},
	"3gpp-sbi-message-priority":     {},
	"3gpp-sbi-producer-id":          {},
	"3gpp-sbi-timestamp":            {},
	"3gpp-sbi-discovery-required":   {},
	"3gpp-sbi-discovery-param-supi": {},
}

func (e *Executor) decodeResponse(ctx context.Context, op *Operation, response *http.Response, sink payload.AttachmentSink) (Result, error) {
	result := Result{
		HTTPStatus: response.StatusCode,
		Headers:    safeResponseHeaders(response.Header),
	}
	contentType := response.Header.Get("Content-Type")
	mediaType := ""
	if strings.TrimSpace(contentType) != "" {
		parsed, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return Result{}, protocolError(err, "invalid response Content-Type")
		}
		mediaType = strings.ToLower(parsed)
	}
	result.MediaType = mediaType

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, err := readBounded(ctx, response.Body, e.limits.MaxTotalBytes)
		if err != nil {
			return Result{}, e.mapResponseReadError(ctx, err, "error response body")
		}
		return Result{}, parseRemoteError(response.StatusCode, contentType, body)
	}

	if len(op.ResponseMediaTypes) == 0 {
		body, err := readBounded(ctx, response.Body, e.limits.MaxTotalBytes)
		if err != nil {
			return Result{}, e.mapResponseReadError(ctx, err, "response body")
		}
		if len(body) != 0 {
			return Result{}, protocolError(nil, "operation declares no response media type but the response carried a body")
		}
		return result, nil
	}
	if mediaType == "" {
		body, err := readBounded(ctx, response.Body, e.limits.MaxTotalBytes)
		if err != nil {
			return Result{}, e.mapResponseReadError(ctx, err, "response body")
		}
		if len(body) != 0 {
			return Result{}, protocolError(nil, "response carried a body without a Content-Type")
		}
		if op.ResponseSchema != nil {
			return Result{}, protocolError(nil, "response schema requires a JSON response body")
		}
		return result, nil
	}
	if !declaresMediaType(op.ResponseMediaTypes, mediaType) {
		return Result{}, protocolError(nil, "response media type %q is not declared by the operation", mediaType)
	}
	switch {
	case isJSONMediaType(mediaType):
		return e.decodeJSONResponse(ctx, op, response, result)
	case mediaType == "multipart/related":
		return e.decodeMultipartResponse(ctx, op, response, contentType, result, sink)
	default:
		return e.decodeBinaryResponse(ctx, op, response, result, sink)
	}
}

func (e *Executor) decodeJSONResponse(ctx context.Context, op *Operation, response *http.Response, result Result) (Result, error) {
	maximum := e.limits.MaxJSONBytes
	if e.limits.MaxTotalBytes < maximum {
		maximum = e.limits.MaxTotalBytes
	}
	raw, err := readBounded(ctx, response.Body, maximum)
	if err != nil {
		return Result{}, e.mapResponseReadError(ctx, err, "JSON response body")
	}
	if len(raw) == 0 {
		if op.ResponseSchema != nil {
			return Result{}, protocolError(nil, "response schema requires a JSON response body")
		}
		return result, nil
	}
	value, err := schemacheck.DecodeJSON(raw)
	if err != nil {
		return Result{}, protocolError(err, "malformed JSON response body")
	}
	if op.ResponseSchema != nil {
		if err := op.ResponseSchema.Validate(value); err != nil {
			return Result{}, protocolError(err, "JSON response does not match the declared schema")
		}
	}
	result.JSON = value
	return result, nil
}

func (e *Executor) decodeBinaryResponse(
	ctx context.Context,
	op *Operation,
	response *http.Response,
	result Result,
	sink payload.AttachmentSink,
) (Result, error) {
	if sink != nil {
		// P1: stream the whole binary body into the Host-owned sink (bounded by
		// the payload limits) instead of buffering it in memory. The declared
		// media type was already checked by decodeResponse.
		parsed, err := payload.DecodeResponse(
			ctx,
			response.Header.Get("Content-Type"),
			response.Body,
			payload.ResponseSpec{Kind: payload.KindBinary},
			sink,
			e.limits,
		)
		if err != nil {
			return Result{}, e.mapResponsePayloadError(ctx, err)
		}
		if parsed.Binary == nil {
			return Result{}, protocolError(nil, "binary response was not staged")
		}
		attachment := Attachment{
			ContentID:   parsed.Binary.ContentID,
			ContentType: parsed.Binary.ContentType,
			Size:        parsed.Binary.Size,
			SHA256:      parsed.Binary.SHA256,
		}
		result.BinaryAttachment = &attachment
		return result, nil
	}
	maximum := e.limits.MaxBinaryBytes
	if e.limits.MaxTotalBytes < maximum {
		maximum = e.limits.MaxTotalBytes
	}
	body, err := readBounded(ctx, response.Body, maximum)
	if err != nil {
		return Result{}, e.mapResponseReadError(ctx, err, "response body")
	}
	result.Body = body
	return result, nil
}

func (e *Executor) decodeMultipartResponse(
	ctx context.Context,
	op *Operation,
	response *http.Response,
	contentType string,
	result Result,
	customSink payload.AttachmentSink,
) (Result, error) {
	spec := payload.ResponseSpec{
		Kind: payload.KindMultipartRelated,
		Multipart: payload.MultipartSpec{
			Parts: make([]payload.PartBinding, 0, len(op.ResponseParts)),
		},
	}
	for _, part := range op.ResponseParts {
		spec.Multipart.Parts = append(spec.Multipart.Parts, payload.PartBinding{
			Name:             part.Name,
			ContentType:      part.ContentType,
			ContentIDPointer: part.JSONPointerField,
			Required:         part.Required,
		})
	}
	var sink payload.AttachmentSink
	var memory *memorySink
	if customSink != nil {
		sink = customSink
	} else {
		memory = newMemorySink(e.limits)
		sink = memory
	}
	parsed, err := payload.DecodeResponse(ctx, contentType, response.Body, spec, sink, e.limits)
	if err != nil {
		return Result{}, e.mapResponsePayloadError(ctx, err)
	}
	root, err := schemacheck.DecodeJSON(parsed.JSON)
	if err != nil {
		return Result{}, protocolError(err, "malformed multipart root JSON")
	}
	if op.ResponseSchema != nil {
		if err := op.ResponseSchema.Validate(root); err != nil {
			return Result{}, protocolError(err, "multipart root JSON does not match the declared schema")
		}
	}
	result.JSON = root
	if memory != nil {
		result.Attachments = memory.results(parsed.Attachments)
	} else {
		result.Attachments = stagedAttachments(parsed.Attachments)
	}
	return result, nil
}

// stagedAttachments converts Host-sink attachment metadata into executor
// results. It never carries bytes: the artifact lives only in the Host store.
func stagedAttachments(parsed []payload.Attachment) []Attachment {
	results := make([]Attachment, 0, len(parsed))
	for _, metadata := range parsed {
		results = append(results, Attachment{
			ContentID:   metadata.ContentID,
			ContentType: metadata.ContentType,
			Size:        metadata.Size,
			SHA256:      metadata.SHA256,
		})
	}
	return results
}

func (e *Executor) mapResponseReadError(ctx context.Context, err error, stage string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &TransportError{Message: "request aborted", Cause: ctxErr}
	}
	switch {
	case errors.Is(err, sbi.ErrResponseTooLarge):
		return protocolError(err, "%s exceeds the transport size limit", stage)
	case errors.Is(err, payload.ErrPayloadTooLarge):
		return protocolError(err, "%s exceeds the configured payload limits", stage)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return &TransportError{Message: "request aborted", Cause: err}
	default:
		return &TransportError{Message: "cannot read response body", Cause: err}
	}
}

func (e *Executor) mapResponsePayloadError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &TransportError{Message: "request aborted", Cause: ctxErr}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &TransportError{Message: "request aborted", Cause: err}
	}
	switch {
	case errors.Is(err, payload.ErrPayloadTooLarge):
		return protocolError(err, "response payload exceeds the configured limits")
	case errors.Is(err, payload.ErrUnknownPayloadPart):
		return protocolError(err, "response carried an undeclared multipart part")
	case errors.Is(err, payload.ErrInvalidPayload):
		return protocolError(err, "malformed multipart response")
	default:
		return protocolError(err, "cannot decode multipart response")
	}
}

func parseRemoteError(status int, contentType string, body []byte) error {
	mediaType, err := parseMediaType(contentType)
	if err == nil && mediaType == "application/problem+json" {
		problem, parseErr := parseProblemDetails(status, body)
		if parseErr == nil {
			return &RemoteError{HTTPStatus: status, Problem: problem, Cause: problem.Cause}
		}
		return protocolError(parseErr, "malformed application/problem+json response for HTTP %d", status)
	}
	return protocolError(nil, "unexpected HTTP status %d without a valid problem detail body", status)
}

func parseProblemDetails(httpStatus int, body []byte) (*ProblemDetails, error) {
	value, err := schemacheck.DecodeJSON(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("problem details must be a JSON object")
	}
	rawStatus, present := object["status"]
	if !present {
		return nil, errors.New("problem details is missing status")
	}
	statusNumber, ok := rawStatus.(json.Number)
	if !ok {
		return nil, errors.New("problem details status must be an integer")
	}
	status, err := strconv.Atoi(statusNumber.String())
	if err != nil {
		return nil, errors.New("problem details status must be an integer")
	}
	if status != httpStatus {
		return nil, errors.New("problem details status does not match the HTTP status")
	}
	rawCause, present := object["cause"]
	if !present {
		return nil, errors.New("problem details is missing cause")
	}
	cause, ok := rawCause.(string)
	if !ok || !problemCausePattern.MatchString(cause) {
		return nil, errors.New("problem details cause is not a safe machine cause")
	}
	problem := &ProblemDetails{Status: status, Cause: cause}
	for _, field := range []struct {
		name   string
		target *string
	}{
		{name: "type", target: &problem.Type},
		{name: "title", target: &problem.Title},
		{name: "detail", target: &problem.Detail},
		{name: "instance", target: &problem.Instance},
	} {
		raw, present := object[field.name]
		if !present {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("problem details %s must be a string", field.name)
		}
		*field.target = text
	}
	return problem, nil
}

func declaresMediaType(declared []string, mediaType string) bool {
	for _, raw := range declared {
		candidate, err := parseMediaType(raw)
		if err == nil && candidate == mediaType {
			return true
		}
	}
	return false
}

func safeResponseHeaders(header http.Header) map[string][]string {
	result := make(map[string][]string)
	for name := range header {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if _, ok := safeResponseHeaderNames[strings.ToLower(canonical)]; !ok {
			continue
		}
		values := header.Values(canonical)
		if len(values) == 0 {
			continue
		}
		result[canonical] = append([]string(nil), values...)
	}
	return result
}

type memorySink struct {
	limits      payload.Limits
	attachments []Attachment
	total       int64
}

func newMemorySink(limits payload.Limits) *memorySink {
	return &memorySink{limits: limits}
}

func (s *memorySink) Store(
	ctx context.Context,
	contentID string,
	contentType string,
	source io.Reader,
	maximum int64,
) (payload.Attachment, error) {
	if maximum < 0 {
		return payload.Attachment{}, payload.ErrPayloadTooLarge
	}
	data, err := readBounded(ctx, source, maximum)
	if err != nil {
		return payload.Attachment{}, err
	}
	s.total += int64(len(data))
	if s.total > s.limits.MaxTotalBytes {
		return payload.Attachment{}, payload.ErrPayloadTooLarge
	}
	digest := sha256.Sum256(data)
	size := int64(len(data))
	attachment := Attachment{
		ContentID:   contentID,
		ContentType: contentType,
		Size:        size,
		SHA256:      hex.EncodeToString(digest[:]),
		Data:        data,
	}
	s.attachments = append(s.attachments, attachment)
	return payload.Attachment{
		ContentID:    contentID,
		RelativePath: fmt.Sprintf("part-%04d.bin", len(s.attachments)),
		ContentType:  contentType,
		Size:         size,
		SHA256:       attachment.SHA256,
	}, nil
}

func (s *memorySink) results(parsed []payload.Attachment) []Attachment {
	byContentID := make(map[string]Attachment, len(s.attachments))
	for _, attachment := range s.attachments {
		byContentID[attachment.ContentID] = attachment
	}
	results := make([]Attachment, 0, len(parsed))
	for _, metadata := range parsed {
		attachment, ok := byContentID[metadata.ContentID]
		if !ok {
			attachment = Attachment{ContentID: metadata.ContentID}
		}
		attachment.ContentType = metadata.ContentType
		attachment.Size = metadata.Size
		attachment.SHA256 = metadata.SHA256
		results = append(results, attachment)
	}
	return results
}

func readBounded(ctx context.Context, reader io.Reader, maximum int64) ([]byte, error) {
	if reader == nil || maximum < 0 {
		return nil, payload.ErrInvalidPayload
	}
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: reader}, maximum+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, payload.ErrPayloadTooLarge
	}
	return body, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	return r.reader.Read(buffer)
}

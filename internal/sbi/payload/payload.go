package payload

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strconv"
	"strings"
)

var (
	ErrInvalidPayload     = errors.New("invalid SBI payload")
	ErrPayloadTooLarge    = errors.New("SBI payload exceeds configured size limit")
	ErrUnknownPayloadPart = errors.New("unknown SBI payload part")
)

type Kind string

const (
	KindNone             Kind = "none"
	KindJSON             Kind = "json"
	KindBinary           Kind = "binary"
	KindMultipartRelated Kind = "multipart-related"
)

type Limits struct {
	MaxJSONBytes   int64
	MaxBinaryBytes int64
	MaxTotalBytes  int64
	MaxParts       int
}

func normalizeLimits(input Limits) (Limits, error) {
	if input.MaxJSONBytes == 0 {
		input.MaxJSONBytes = 16 << 20
	}
	if input.MaxBinaryBytes == 0 {
		input.MaxBinaryBytes = 64 << 20
	}
	if input.MaxTotalBytes == 0 {
		input.MaxTotalBytes = 128 << 20
	}
	if input.MaxParts == 0 {
		input.MaxParts = 32
	}
	if input.MaxJSONBytes < 1 || input.MaxJSONBytes > 1<<30 ||
		input.MaxBinaryBytes < 1 || input.MaxBinaryBytes > 1<<30 ||
		input.MaxTotalBytes < 1 || input.MaxTotalBytes > 1<<30 ||
		input.MaxParts < 1 || input.MaxParts > 256 {
		return Limits{}, ErrInvalidPayload
	}
	return input, nil
}

type Source struct {
	RelativePath string
	Size         int64
}

type Resolver interface {
	Open(context.Context, string) (io.ReadCloser, int64, error)
}

type Attachment struct {
	ContentID    string `json:"contentId,omitempty"`
	RelativePath string `json:"relativePath"`
	ContentType  string `json:"contentType"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

type AttachmentSink interface {
	Store(context.Context, string, string, io.Reader, int64) (Attachment, error)
}

type PartBinding struct {
	Name             string
	ContentType      string
	ContentIDPointer string
	Required         bool
}

type MultipartSpec struct {
	Parts []PartBinding
}

type Encoded struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

type ResponseSpec struct {
	Kind        Kind
	ContentType string
	Multipart   MultipartSpec
}

type Parsed struct {
	JSON        json.RawMessage
	Binary      *Attachment
	Attachments []Attachment
}

// BuildJSON creates a bounded JSON request body.
func BuildJSON(body []byte, limits Limits) (Encoded, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Encoded{}, err
	}
	if !json.Valid(body) {
		return Encoded{}, ErrInvalidPayload
	}
	if int64(len(body)) > limits.MaxJSONBytes || int64(len(body)) > limits.MaxTotalBytes {
		return Encoded{}, ErrPayloadTooLarge
	}
	copyBody := append([]byte(nil), body...)
	return Encoded{
		Body: io.NopCloser(bytes.NewReader(copyBody)), ContentType: "application/json",
		ContentLength: int64(len(copyBody)),
	}, nil
}

// BuildBinary streams one opaque body from a Host-staged payload source.
// ContentLength is unknown so the reader can verify that the staged file has
// exactly the declared size without buffering it in memory.
func BuildBinary(ctx context.Context, resolver Resolver, source Source, contentType string, limits Limits) (Encoded, error) {
	ctx = contextOrBackground(ctx)
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Encoded{}, err
	}
	if resolver == nil || source.RelativePath == "" || source.Size < 0 {
		return Encoded{}, ErrInvalidPayload
	}
	if _, _, err := parseMediaType(contentType); err != nil {
		return Encoded{}, err
	}
	if source.Size > limits.MaxBinaryBytes || source.Size > limits.MaxTotalBytes {
		return Encoded{}, ErrPayloadTooLarge
	}
	reader, size, err := resolver.Open(ctx, source.RelativePath)
	if err != nil {
		return Encoded{}, err
	}
	if size < 0 || size != source.Size {
		reader.Close()
		return Encoded{}, ErrInvalidPayload
	}
	return Encoded{
		Body: &exactSizeReadCloser{ctx: ctx, ReadCloser: reader, remaining: size},
		ContentType: contentType, ContentLength: -1,
	}, nil
}

// BuildMultipartRelated writes the JSON root first, followed by operation-declared
// binary parts. ContentIDPointer identifies each RefToBinaryData.contentId string.
func BuildMultipartRelated(
	ctx context.Context,
	resolver Resolver,
	rootJSON []byte,
	spec MultipartSpec,
	sources map[string]Source,
	limits Limits,
) (Encoded, error) {
	ctx = contextOrBackground(ctx)
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Encoded{}, err
	}
	if resolver == nil || !json.Valid(rootJSON) {
		return Encoded{}, ErrInvalidPayload
	}
	if err := ctx.Err(); err != nil {
		return Encoded{}, err
	}
	if int64(len(rootJSON)) > limits.MaxJSONBytes {
		return Encoded{}, ErrPayloadTooLarge
	}
	if len(spec.Parts)+1 > limits.MaxParts {
		return Encoded{}, ErrPayloadTooLarge
	}

	root, err := decodeJSON(rootJSON)
	if err != nil {
		return Encoded{}, err
	}
	known := make(map[string]struct{}, len(spec.Parts))
	pointers := make(map[string]struct{}, len(spec.Parts))
	opened := make([]openPart, 0, len(spec.Parts))
	cleanup := func() {
		for _, part := range opened {
			_ = part.reader.Close()
		}
	}
	total := int64(len(rootJSON))

	for _, binding := range spec.Parts {
		if err := ctx.Err(); err != nil {
			cleanup()
			return Encoded{}, err
		}
		if binding.Name == "" || binding.ContentType == "" || !validJSONPointer(binding.ContentIDPointer) {
			cleanup()
			return Encoded{}, ErrInvalidPayload
		}
		if _, duplicate := known[binding.Name]; duplicate {
			cleanup()
			return Encoded{}, ErrInvalidPayload
		}
		known[binding.Name] = struct{}{}
		if _, duplicate := pointers[binding.ContentIDPointer]; duplicate {
			cleanup()
			return Encoded{}, ErrInvalidPayload
		}
		pointers[binding.ContentIDPointer] = struct{}{}

		source, exists := sources[binding.Name]
		if !exists {
			if binding.Required {
				cleanup()
				return Encoded{}, ErrInvalidPayload
			}
			existing, found, pointerErr := getJSONPointer(root, binding.ContentIDPointer)
			if pointerErr != nil || (found && existing != "") {
				cleanup()
				return Encoded{}, ErrInvalidPayload
			}
			continue
		}
		if source.RelativePath == "" || source.Size < 0 {
			cleanup()
			return Encoded{}, ErrInvalidPayload
		}
		if _, _, err := parseMediaType(binding.ContentType); err != nil {
			cleanup()
			return Encoded{}, err
		}
		if source.Size > limits.MaxBinaryBytes || total > limits.MaxTotalBytes-source.Size {
			cleanup()
			return Encoded{}, ErrPayloadTooLarge
		}
		reader, size, err := resolver.Open(ctx, source.RelativePath)
		if err != nil {
			cleanup()
			return Encoded{}, err
		}
		if size < 0 || size != source.Size {
			_ = reader.Close()
			cleanup()
			return Encoded{}, ErrInvalidPayload
		}
		contentID, err := newContentID()
		if err != nil {
			_ = reader.Close()
			cleanup()
			return Encoded{}, err
		}
		if err := setJSONPointer(root, binding.ContentIDPointer, contentID); err != nil {
			_ = reader.Close()
			cleanup()
			return Encoded{}, err
		}
		opened = append(opened, openPart{binding: binding, contentID: contentID, reader: reader, size: size})
		total += size
	}

	for name := range sources {
		if _, ok := known[name]; !ok {
			cleanup()
			return Encoded{}, ErrUnknownPayloadPart
		}
	}

	rootBody, err := json.Marshal(root)
	if err != nil {
		cleanup()
		return Encoded{}, ErrInvalidPayload
	}
	if int64(len(rootBody)) > limits.MaxJSONBytes ||
		total-int64(len(rootJSON))+int64(len(rootBody)) > limits.MaxTotalBytes {
		cleanup()
		return Encoded{}, ErrPayloadTooLarge
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := mime.FormatMediaType("multipart/related", map[string]string{
		"boundary": writer.Boundary(),
		"type":     "application/json",
	})

	go func() {
		defer cleanup()
		rootHeader := make(textproto.MIMEHeader)
		rootHeader.Set("Content-Type", "application/json")
		partWriter, writeErr := writer.CreatePart(rootHeader)
		if writeErr == nil {
			_, writeErr = io.Copy(partWriter, bytes.NewReader(rootBody))
		}
		for _, part := range opened {
			if writeErr != nil {
				break
			}
			header := make(textproto.MIMEHeader)
			header.Set("Content-Type", part.binding.ContentType)
			header.Set("Content-ID", "<"+part.contentID+">")
			partWriter, writeErr = writer.CreatePart(header)
			if writeErr == nil {
				var written int64
				written, writeErr = io.Copy(
					partWriter,
					io.LimitReader(&contextReader{ctx: ctx, reader: part.reader}, part.size+1),
				)
				if writeErr == nil && written != part.size {
					writeErr = ErrInvalidPayload
				}
			}
		}
		if writeErr == nil {
			writeErr = writer.Close()
		}
		if writeErr != nil {
			_ = pipeWriter.CloseWithError(writeErr)
			return
		}
		_ = pipeWriter.Close()
	}()

	return Encoded{Body: pipeReader, ContentType: contentType, ContentLength: -1}, nil
}

type openPart struct {
	binding   PartBinding
	contentID string
	reader    io.ReadCloser
	size      int64
}

// DecodeResponse validates response media types and streams binary attachments
// into the Host-owned sink. It never converts binary data to text or base64.
func DecodeResponse(
	ctx context.Context,
	contentType string,
	body io.Reader,
	spec ResponseSpec,
	sink AttachmentSink,
	limits Limits,
) (Parsed, error) {
	ctx = contextOrBackground(ctx)
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Parsed{}, err
	}
	if body == nil {
		return Parsed{}, ErrInvalidPayload
	}
	if err := ctx.Err(); err != nil {
		return Parsed{}, err
	}

	switch spec.Kind {
	case KindNone:
		return Parsed{}, nil
	case KindJSON:
		mediaType, _, err := parseMediaType(contentType)
		if err != nil {
			return Parsed{}, err
		}
		expected := spec.ContentType
		if expected == "" {
			expected = "application/json"
		}
		if !sameMediaType(mediaType, expected) {
			return Parsed{}, ErrInvalidPayload
		}
		raw, err := readBounded(ctx, body, limits.MaxJSONBytes)
		if err != nil {
			return Parsed{}, err
		}
		if !json.Valid(raw) {
			return Parsed{}, ErrInvalidPayload
		}
		return Parsed{JSON: raw}, nil
	case KindBinary:
		mediaType, _, err := parseMediaType(contentType)
		if err != nil {
			return Parsed{}, err
		}
		if spec.ContentType != "" && !sameMediaType(mediaType, spec.ContentType) {
			return Parsed{}, ErrInvalidPayload
		}
		if sink == nil {
			return Parsed{}, ErrInvalidPayload
		}
		attachment, err := sink.Store(ctx, "", contentType, &contextReader{ctx: ctx, reader: body}, limits.MaxBinaryBytes)
		if err != nil {
			return Parsed{}, err
		}
		if attachment.Size < 0 || attachment.Size > limits.MaxBinaryBytes || attachment.Size > limits.MaxTotalBytes {
			return Parsed{}, ErrPayloadTooLarge
		}
		return Parsed{Binary: &attachment}, nil
	case KindMultipartRelated:
		return decodeMultipart(ctx, contentType, body, spec.Multipart, sink, limits)
	default:
		return Parsed{}, ErrInvalidPayload
	}
}

func decodeMultipart(
	ctx context.Context,
	contentType string,
	body io.Reader,
	spec MultipartSpec,
	sink AttachmentSink,
	limits Limits,
) (Parsed, error) {
	mediaType, params, err := parseMediaType(contentType)
	if err != nil || mediaType != "multipart/related" || params["boundary"] == "" ||
		!sameMediaType(params["type"], "application/json") || sink == nil {
		return Parsed{}, ErrInvalidPayload
	}
	if len(spec.Parts)+1 > limits.MaxParts {
		return Parsed{}, ErrPayloadTooLarge
	}
	reader := multipart.NewReader(body, params["boundary"])
	rootPart, err := reader.NextPart()
	if err != nil {
		return Parsed{}, ErrInvalidPayload
	}
	rootMediaType, _, err := parseMediaType(rootPart.Header.Get("Content-Type"))
	if err != nil || rootMediaType != "application/json" {
		rootPart.Close()
		return Parsed{}, ErrInvalidPayload
	}
	rootJSON, err := readBounded(ctx, rootPart, limits.MaxJSONBytes)
	rootPart.Close()
	if err != nil {
		return Parsed{}, err
	}
	if !json.Valid(rootJSON) {
		return Parsed{}, ErrInvalidPayload
	}
	root, err := decodeJSON(rootJSON)
	if err != nil {
		return Parsed{}, err
	}

	expected := make(map[string]PartBinding, len(spec.Parts))
	pointers := make(map[string]struct{}, len(spec.Parts))
	for _, binding := range spec.Parts {
		if binding.Name == "" || binding.ContentType == "" || !validJSONPointer(binding.ContentIDPointer) {
			return Parsed{}, ErrInvalidPayload
		}
		if _, duplicate := pointers[binding.ContentIDPointer]; duplicate {
			return Parsed{}, ErrInvalidPayload
		}
		pointers[binding.ContentIDPointer] = struct{}{}
		contentID, found, err := getJSONPointer(root, binding.ContentIDPointer)
		if err != nil {
			return Parsed{}, err
		}
		if !found || contentID == "" {
			if binding.Required {
				return Parsed{}, ErrInvalidPayload
			}
			continue
		}
		contentID, err = normalizeContentID(contentID)
		if err != nil {
			return Parsed{}, err
		}
		if _, duplicate := expected[contentID]; duplicate {
			return Parsed{}, ErrInvalidPayload
		}
		if _, _, err := parseMediaType(binding.ContentType); err != nil {
			return Parsed{}, err
		}
		expected[contentID] = binding
	}

	seen := make(map[string]struct{}, len(expected))
	attachments := make([]Attachment, 0, len(expected))
	total := int64(len(rootJSON))
	for count := 1; ; count++ {
		if err := ctx.Err(); err != nil {
			return Parsed{}, err
		}
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return Parsed{}, ctx.Err()
			}
			return Parsed{}, ErrInvalidPayload
		}
		if count+1 > limits.MaxParts {
			part.Close()
			return Parsed{}, ErrPayloadTooLarge
		}
		contentID, err := normalizeContentID(part.Header.Get("Content-ID"))
		if err != nil {
			part.Close()
			return Parsed{}, err
		}
		binding, ok := expected[contentID]
		if !ok {
			part.Close()
			return Parsed{}, ErrUnknownPayloadPart
		}
		if _, duplicate := seen[contentID]; duplicate {
			part.Close()
			return Parsed{}, ErrInvalidPayload
		}
		partMediaType, _, err := parseMediaType(part.Header.Get("Content-Type"))
		if err != nil || !sameMediaType(partMediaType, binding.ContentType) {
			part.Close()
			return Parsed{}, ErrInvalidPayload
		}
		remaining := limits.MaxTotalBytes - total
		if remaining < 0 {
			part.Close()
			return Parsed{}, ErrPayloadTooLarge
		}
		maxPart := limits.MaxBinaryBytes
		if remaining < maxPart {
			maxPart = remaining
		}
		attachment, storeErr := sink.Store(
			ctx, contentID, part.Header.Get("Content-Type"), part, maxPart,
		)
		part.Close()
		if storeErr != nil {
			return Parsed{}, storeErr
		}
		if attachment.Size < 0 || attachment.Size > maxPart {
			return Parsed{}, ErrPayloadTooLarge
		}
		if rawLength := part.Header.Get("Content-Length"); rawLength != "" {
			declared, parseErr := strconv.ParseInt(rawLength, 10, 64)
			if parseErr != nil || declared != attachment.Size {
				return Parsed{}, ErrInvalidPayload
			}
		}
		total += attachment.Size
		seen[contentID] = struct{}{}
		attachments = append(attachments, attachment)
	}
	for contentID, binding := range expected {
		if _, ok := seen[contentID]; !ok {
			return Parsed{}, fmt.Errorf("%w: missing part %s", ErrInvalidPayload, binding.Name)
		}
	}
	return Parsed{JSON: rootJSON, Attachments: attachments}, nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func parseMediaType(raw string) (string, map[string]string, error) {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil || mediaType == "" {
		return "", nil, ErrInvalidPayload
	}
	return strings.ToLower(mediaType), params, nil
}

func sameMediaType(left, right string) bool {
	leftType, _, leftErr := mime.ParseMediaType(strings.TrimSpace(left))
	rightType, _, rightErr := mime.ParseMediaType(strings.TrimSpace(right))
	return leftErr == nil && rightErr == nil && strings.EqualFold(leftType, rightType)
}

func newContentID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func normalizeContentID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		value = value[1 : len(value)-1]
	}
	if strings.HasPrefix(strings.ToLower(value), "cid:") {
		value = value[4:]
	}
	if value == "" || strings.ContainsAny(value, "<>
	 ") {
		return "", ErrInvalidPayload
	}
	return value, nil
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, ErrInvalidPayload
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrInvalidPayload
	}
	return value, nil
}

func validJSONPointer(pointer string) bool {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return false
	}
	_, err := jsonPointerSegments(pointer)
	return err == nil
}

func jsonPointerSegments(pointer string) ([]string, error) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, ErrInvalidPayload
	}
	parts := strings.Split(pointer[1:], "/")
	for index, part := range parts {
		var builder strings.Builder
		for i := 0; i < len(part); i++ {
			if part[i] != '~' {
				builder.WriteByte(part[i])
				continue
			}
			if i+1 >= len(part) {
				return nil, ErrInvalidPayload
			}
			i++
			switch part[i] {
			case '0':
				builder.WriteByte('~')
			case '1':
				builder.WriteByte('/')
			default:
				return nil, ErrInvalidPayload
			}
		}
		parts[index] = builder.String()
	}
	return parts, nil
}

func setJSONPointer(root any, pointer, value string) error {
	segments, err := jsonPointerSegments(pointer)
	if err != nil {
		return err
	}
	current := root
	for _, segment := range segments[:len(segments)-1] {
		switch node := current.(type) {
		case map[string]any:
			next, ok := node[segment]
			if !ok {
				return ErrInvalidPayload
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return ErrInvalidPayload
			}
			current = node[index]
		default:
			return ErrInvalidPayload
		}
	}
	last := segments[len(segments)-1]
	switch node := current.(type) {
	case map[string]any:
		if old, exists := node[last]; exists && old != nil {
			if oldString, ok := old.(string); !ok || oldString != "" {
				return ErrInvalidPayload
			}
		}
		node[last] = value
	case []any:
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 || index >= len(node) {
			return ErrInvalidPayload
		}
		if oldString, ok := node[index].(string); !ok || oldString != "" {
			return ErrInvalidPayload
		}
		node[index] = value
	default:
		return ErrInvalidPayload
	}
	return nil
}

func getJSONPointer(root any, pointer string) (string, bool, error) {
	segments, err := jsonPointerSegments(pointer)
	if err != nil {
		return "", false, err
	}
	current := root
	for _, segment := range segments {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[segment]
			if !ok {
				return "", false, nil
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return "", false, nil
			}
			current = node[index]
		default:
			return "", false, nil
		}
	}
	value, ok := current.(string)
	if !ok {
		return "", false, nil
	}
	return value, true, nil
}

func readBounded(ctx context.Context, source io.Reader, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, ErrInvalidPayload
	}
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(source, maximum+1)}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, ErrPayloadTooLarge
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

type exactSizeReadCloser struct {
	ctx context.Context
	io.ReadCloser
	remaining int64
}

func (r *exactSizeReadCloser) Read(buffer []byte) (int, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, ErrInvalidPayload
		}
		return 0, err
	}
	if int64(len(buffer)) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	n, err := r.ReadCloser.Read(buffer)
	r.remaining -= int64(n)
	if err == io.EOF && r.remaining > 0 {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

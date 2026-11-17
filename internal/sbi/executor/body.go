package executor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

type preparedBody struct {
	body          io.Reader
	contentLength int64
	contentType   string
}

func (e *Executor) buildBody(ctx context.Context, op *Operation, resolved *resolvedArguments) (preparedBody, error) {
	if op.Body == nil {
		return preparedBody{}, nil
	}
	if err := ctx.Err(); err != nil {
		return preparedBody{}, &TransportError{Message: "request aborted", Cause: err}
	}
	switch op.Body.Kind {
	case PayloadJSON:
		raw, err := json.Marshal(resolved.body)
		if err != nil {
			return preparedBody{}, inputError(err, "JSON body is not serializable")
		}
		encoded, err := payload.BuildJSON(raw, e.limits)
		if err != nil {
			return preparedBody{}, buildInputError(err, "JSON body")
		}
		return preparedBody{
			body:          encoded.Body,
			contentLength: encoded.ContentLength,
			contentType:   encoded.ContentType,
		}, nil
	case PayloadBinary:
		return e.buildBinaryBody(ctx, op, resolved)
	case PayloadMultipart:
		return e.buildMultipartBody(ctx, op, resolved)
	default:
		return preparedBody{}, configError(nil, "operation %s: unsupported body kind %q", op.OperationID, op.Body.Kind)
	}
}

func (e *Executor) buildBinaryBody(ctx context.Context, op *Operation, resolved *resolvedArguments) (preparedBody, error) {
	if e.resolver == nil {
		return preparedBody{}, configError(nil, "operation %s: binary body requires an ArgumentResolver", op.OperationID)
	}
	reader, size, err := e.resolver.Open(ctx, resolved.binary)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return preparedBody{}, &TransportError{Message: "request aborted", Cause: ctxErr}
		}
		return preparedBody{}, inputError(err, "cannot open binary payload %q", resolved.binary)
	}
	tracked := newTrackedReader(reader)
	source := payload.Source{RelativePath: resolved.binary, Size: size}
	resolver := &singleSourceResolver{name: resolved.binary, reader: tracked, size: size}
	encoded, err := payload.BuildBinary(ctx, resolver, source, op.Body.MediaType, e.limits)
	if err != nil {
		_ = tracked.Close()
		return preparedBody{}, buildInputError(err, "binary body")
	}
	return preparedBody{
		body:          encoded.Body,
		contentLength: encoded.ContentLength,
		contentType:   encoded.ContentType,
	}, nil
}

func (e *Executor) buildMultipartBody(ctx context.Context, op *Operation, resolved *resolvedArguments) (preparedBody, error) {
	if e.resolver == nil {
		return preparedBody{}, configError(nil, "operation %s: multipart body requires an ArgumentResolver", op.OperationID)
	}
	limits := e.limits
	if op.Body.MaxParts > 0 && op.Body.MaxParts < limits.MaxParts {
		limits.MaxParts = op.Body.MaxParts
	}
	if len(op.Body.Parts)+1 > limits.MaxParts {
		return preparedBody{}, configError(nil, "operation %s: declared multipart part count exceeds the configured limit", op.OperationID)
	}

	opened := make(map[string]*trackedReader, len(resolved.parts))
	sources := make(map[string]payload.Source, len(resolved.parts))
	sizes := make(map[string]int64, len(resolved.parts))
	closeAll := func() {
		for _, reader := range opened {
			_ = reader.Close()
		}
	}
	for _, part := range op.Body.Parts {
		name, present := resolved.parts[part.Name]
		if !present {
			continue
		}
		reader, size, err := e.resolver.Open(ctx, name)
		if err != nil {
			closeAll()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return preparedBody{}, &TransportError{Message: "request aborted", Cause: ctxErr}
			}
			return preparedBody{}, inputError(err, "cannot open payload part %q", part.Name)
		}
		opened[part.Name] = newTrackedReader(reader)
		// The payload engine opens each source through its declared part name;
		// the Host-staged resolver name is kept only for the initial open. The
		// part name is unique per operation and never leaves this process.
		sources[part.Name] = payload.Source{RelativePath: part.Name, Size: size}
		sizes[part.Name] = size
	}

	spec := payload.MultipartSpec{Parts: make([]payload.PartBinding, 0, len(op.Body.Parts))}
	for _, part := range op.Body.Parts {
		spec.Parts = append(spec.Parts, payload.PartBinding{
			Name:             part.Name,
			ContentType:      part.ContentType,
			ContentIDPointer: part.JSONPointerField,
			Required:         part.Required,
		})
	}
	rootJSON, err := json.Marshal(resolved.body)
	if err != nil {
		closeAll()
		return preparedBody{}, inputError(err, "multipart root body is not serializable")
	}
	resolver := &multiSourceResolver{readers: opened, sizes: sizes}
	encoded, err := payload.BuildMultipartRelated(ctx, resolver, rootJSON, spec, sources, limits)
	if err != nil {
		closeAll()
		return preparedBody{}, buildInputError(err, "multipart body")
	}
	return preparedBody{
		body:          encoded.Body,
		contentLength: encoded.ContentLength,
		contentType:   encoded.ContentType,
	}, nil
}

func buildInputError(err error, stage string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &TransportError{Message: "request aborted", Cause: err}
	}
	switch {
	case errors.Is(err, payload.ErrPayloadTooLarge):
		return inputError(err, "%s exceeds the configured payload limits", stage)
	case errors.Is(err, payload.ErrUnknownPayloadPart):
		return inputError(err, "%s contains an unknown payload part", stage)
	case errors.Is(err, payload.ErrInvalidPayload):
		return inputError(err, "%s is not a valid payload", stage)
	default:
		return inputError(err, "cannot build %s", stage)
	}
}

func (e *Executor) mapTransportError(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return &TransportError{Message: "request aborted", Cause: ctxErr}
		}
	}
	switch {
	case errors.Is(err, sbi.ErrUnknownService):
		return configError(err, "unknown service key")
	case errors.Is(err, sbi.ErrInvalidRequest):
		return configError(err, "request rejected by the SBI transport")
	case errors.Is(err, sbi.ErrHTTP2Required):
		return protocolError(err, "endpoint did not negotiate HTTP/2")
	case errors.Is(err, sbi.ErrRequestTooLarge):
		return inputError(err, "request exceeds the transport size limit")
	case errors.Is(err, sbi.ErrResponseTooLarge):
		return protocolError(err, "response exceeds the transport size limit")
	default:
		return &TransportError{Message: "request did not complete", Cause: err}
	}
}

type trackedReader struct {
	mutex  sync.Mutex
	reader io.ReadCloser
	closed bool
}

func newTrackedReader(reader io.ReadCloser) *trackedReader {
	return &trackedReader{reader: reader}
}

func (r *trackedReader) Read(buffer []byte) (int, error) {
	return r.reader.Read(buffer)
}

func (r *trackedReader) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.reader.Close()
}

type singleSourceResolver struct {
	name   string
	reader *trackedReader
	size   int64
	used   bool
}

func (r *singleSourceResolver) Open(_ context.Context, name string) (io.ReadCloser, int64, error) {
	if r.used || name != r.name {
		return nil, 0, payload.ErrInvalidPayload
	}
	r.used = true
	return r.reader, r.size, nil
}

type multiSourceResolver struct {
	readers map[string]*trackedReader
	sizes   map[string]int64
	used    map[string]struct{}
}

func (r *multiSourceResolver) Open(_ context.Context, name string) (io.ReadCloser, int64, error) {
	reader, ok := r.readers[name]
	if !ok {
		return nil, 0, payload.ErrUnknownPayloadPart
	}
	if r.used == nil {
		r.used = make(map[string]struct{}, len(r.readers))
	}
	if _, duplicate := r.used[name]; duplicate {
		return nil, 0, payload.ErrInvalidPayload
	}
	r.used[name] = struct{}{}
	return reader, r.sizes[name], nil
}

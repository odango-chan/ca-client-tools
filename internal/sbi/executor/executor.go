// Package executor implements a generic, NF-agnostic executor for trusted SBI
// operation descriptors.
//
// An Operation is trusted package content produced from a versioned Operation
// Catalog. Callers supply only typed arguments; the executor owns the HTTP
// method, path template, service key, media types, trusted header bindings and
// all body framing. It never retries and it never accepts caller-supplied
// transport settings.
package executor

import (
	"context"
	"fmt"
	"io"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

// Effect classifies an operation as a read or a mutation.
type Effect string

const (
	EffectRead     Effect = "READ"
	EffectMutation Effect = "MUTATION"
)

// PayloadKind names the request body framing of an operation.
type PayloadKind string

const (
	PayloadJSON      PayloadKind = "json"
	PayloadBinary    PayloadKind = "binary"
	PayloadMultipart PayloadKind = "multipart"
)

// Parameter describes one typed path or query argument.
type Parameter struct {
	Name      string
	Type      string // string|integer|boolean
	Required  bool
	Minimum   *int64
	Maximum   *int64
	MinLength *int
	MaxLength *int
	Pattern   string
	Enum      []string
	Default   any
}

// HeaderBinding is a constant trusted request header. Header values can never
// come from operation arguments.
type HeaderBinding struct {
	Name  string
	Value string
}

// MultipartPart binds one declared multipart binary part to its root JSON
// reference field. ContentID is reserved metadata: the payload engine
// generates a unique Content-ID per invocation; a configured value is retained
// for catalog fidelity but never used as a caller-controlled value.
type MultipartPart struct {
	Name             string
	ContentType      string
	ContentID        string
	JSONPointerField string // JSON pointer in the root JSON, e.g. /n1MessageContent
	Required         bool
}

// BodySpec describes the request body contract of an operation.
type BodySpec struct {
	Kind             PayloadKind
	MediaType        string
	Schema           *schemacheck.Schema // root JSON schema for json/multipart bodies
	MultipartRoot    bool
	Parts            []MultipartPart
	MaxParts         int
	ContentTypeParam string
}

// Idempotency is the declared idempotency contract of a mutation operation.
// The executor never retries; the metadata records whether the caller must
// provide a key and where that key lives inside the arguments.
type Idempotency struct {
	Mode        string // "NONE" or "REQUIRED"
	KeyArgument string // JSON pointer into the arguments, when REQUIRED
}

// Operation is a trusted, fully-bound SBI operation descriptor.
type Operation struct {
	OperationID        string
	Effect             Effect
	ServiceKey         string
	Method             string
	PathTemplate       string
	PathParameters     []Parameter
	QueryParameters    []Parameter
	HeaderBindings     []HeaderBinding
	Body               *BodySpec
	ResponseMediaTypes []string
	ResponseSchema     *schemacheck.Schema
	ResponseParts      []MultipartPart // declared binary response parts for multipart/related responses
	Idempotency        *Idempotency    // only valid for MUTATION
}

// ArgumentResolver opens one Host-staged binary input by Host-generated
// relative name. Implementations must enforce root confinement.
type ArgumentResolver interface {
	Open(ctx context.Context, name string) (io.ReadCloser, int64, error)
}

// Attachment is one decoded binary response part. Data is bounded in memory;
// the executor never writes response bytes to the filesystem.
type Attachment struct {
	ContentID   string
	ContentType string
	Size        int64
	SHA256      string
	Data        []byte
}

// Result is the decoded outcome of one operation invocation. Attachments
// carries multipart/related response parts; Body carries a single bounded
// non-JSON response body. Either may be empty.
type Result struct {
	HTTPStatus  int
	MediaType   string
	JSON        any
	Body        []byte
	Attachments []Attachment
	Headers     map[string][]string
}

// Executor executes trusted operations through the shared SBI transport.
type Executor struct {
	client   *sbi.Client
	resolver ArgumentResolver
	limits   payload.Limits
}

// New creates an Executor. limits are composed against the default transport
// bounds so a payload accepted here always fits the shared transport defaults.
// Callers that lower sbi.Limits must lower these payload limits as well.
func New(client *sbi.Client, resolver ArgumentResolver, limits payload.Limits) (*Executor, error) {
	if client == nil {
		return nil, &ConfigurationError{Message: "executor: SBI client is required"}
	}
	normalized, err := payload.ComposeLimits(limits, payload.DefaultMaxTransportBytes, payload.DefaultMaxTransportBytes)
	if err != nil {
		return nil, &ConfigurationError{Message: "executor: invalid payload limits", Cause: err}
	}
	return &Executor{client: client, resolver: resolver, limits: normalized}, nil
}

// Execute validates one operation and its arguments, performs exactly one SBI
// request and decodes the response. It never retries.
func (e *Executor) Execute(ctx context.Context, op Operation, args map[string]any) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil || e.client == nil {
		return Result{}, &ConfigurationError{Message: "executor is not initialized"}
	}
	if err := validateOperation(&op); err != nil {
		return Result{}, err
	}
	resolved, err := resolveArguments(&op, args)
	if err != nil {
		return Result{}, err
	}
	prepared, err := e.buildBody(ctx, &op, resolved)
	if err != nil {
		return Result{}, err
	}
	request := sbi.Request{
		Method:      op.Method,
		Path:        resolved.path,
		Query:       resolved.query,
		Headers:     resolved.headers,
		ContentType: prepared.contentType,
		Body:        prepared.body,
	}
	if prepared.body != nil {
		request.ContentLength = prepared.contentLength
	}
	if len(op.ResponseMediaTypes) > 0 {
		request.Accept = joinMediaTypes(op.ResponseMediaTypes)
	}
	response, err := e.client.Do(ctx, op.ServiceKey, request)
	if err != nil {
		return Result{}, e.mapTransportError(ctx, err)
	}
	defer response.Body.Close()
	return e.decodeResponse(ctx, &op, response)
}

func joinMediaTypes(mediaTypes []string) string {
	result := ""
	for index, mediaType := range mediaTypes {
		if index > 0 {
			result += ", "
		}
		result += mediaType
	}
	return result
}

// String makes an Executor safe to mention in diagnostics.
func (e *Executor) String() string {
	if e == nil {
		return "executor<nil>"
	}
	return fmt.Sprintf("executor{limits:%+v}", e.limits)
}

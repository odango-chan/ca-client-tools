// Package provider implements the resident CA Client Tools Provider process
// protocol (ca-client-tools.provider/v1 and its additive v2 catalog/argument
// extension). It is a private Host-to-Provider process boundary: it owns
// framing, correlation, bounded concurrency, deadlines, cancellation and safe
// error mapping. It does not own public callable identity, Capability
// authorization, Scope, Owner Outcomes or Evidence.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/odango-chan/ca-client-tools/internal/jsonx"
)

const (
	ProtocolV1 = "ca-client-tools.provider/v1"
	ProtocolV2 = "ca-client-tools.provider/v2"

	ProviderID = "ca-client-tools.sbi"

	// Default and hard protocol bounds. The Host supplies requested limits in
	// initialize; the Provider accepts only values within the hard maxima and
	// never returns accepted values greater than those requested.
	DefaultMaxFrameBytes   = 8 << 20
	HardMaxFrameBytes      = 16 << 20
	DefaultMaxInFlight     = 8
	HardMaxInFlight        = 16
	DefaultMaxCallDuration = 5 * time.Second
	HardMaxCallDuration    = 30 * time.Second

	// maxRequestIDLength is the protocol bound for opaque correlation strings.
	maxRequestIDLength = 128
)

// Stable Provider/protocol error codes (see provider-protocol-v1.md §7 and the
// v2 extension document). Codes are the only machine identity exposed; error
// messages are safe categories and never contain remote bodies or credentials.
const (
	CodeProtocolVersionUnsupported    = "PROTOCOL_VERSION_UNSUPPORTED"
	CodeProtocolFrameInvalid          = "PROTOCOL_FRAME_INVALID"
	CodeProtocolFrameTooLarge         = "PROTOCOL_FRAME_TOO_LARGE"
	CodeProtocolRequestIDInvalid      = "PROTOCOL_REQUEST_ID_INVALID"
	CodeProtocolMethodUnsupported     = "PROTOCOL_METHOD_UNSUPPORTED"
	CodeProviderNotInitialized        = "PROVIDER_NOT_INITIALIZED"
	CodeProviderAlreadyInitialized    = "PROVIDER_ALREADY_INITIALIZED"
	CodeProviderConfigurationInvalid  = "PROVIDER_CONFIGURATION_INVALID"
	CodeProviderBindingCatalogInvalid = "PROVIDER_BINDING_CATALOG_INVALID"
	CodeBackendOperationUnsupported   = "BACKEND_OPERATION_UNSUPPORTED"
	CodeBackendArgumentsInvalid       = "BACKEND_ARGUMENTS_INVALID"
	CodeCallDeadlineExceeded          = "CALL_DEADLINE_EXCEEDED"
	CodeCallCancelled                 = "CALL_CANCELLED"
	CodeRemoteTransportError          = "REMOTE_TRANSPORT_ERROR"
	CodeRemoteResponseInvalid         = "REMOTE_RESPONSE_INVALID"
	CodeProviderInternal              = "PROVIDER_INTERNAL"
	CodeProviderShuttingDown          = "PROVIDER_SHUTTING_DOWN"
)

// Fault is a Provider protocol error: a stable code plus safe optional details.
type Fault struct {
	Code   string
	Detail map[string]any
}

func (f *Fault) Error() string { return f.Code }

func fault(code string) *Fault { return &Fault{Code: code} }

func faultDetail(code, key, value string) *Fault {
	return &Fault{Code: code, Detail: map[string]any{key: value}}
}

// ProviderInfo is the trusted deployment identity reported by initialize.
type ProviderInfo struct {
	ProviderID      string
	ProviderVersion string
	BuildDigest     string
}

// Limits are the accepted protocol bounds for one Provider process.
type Limits struct {
	MaxFrameBytes   int64
	MaxInFlight     int
	MaxCallDuration time.Duration
}

func (l Limits) wire() map[string]any {
	return map[string]any{
		"maxFrameBytes":     l.MaxFrameBytes,
		"maxInFlight":       l.MaxInFlight,
		"maxCallDurationMs": l.MaxCallDuration.Milliseconds(),
	}
}

// Argument is one typed backend argument declared by the trusted catalog.
type Argument struct {
	Name     string
	Type     string // "string", "integer" or "boolean"
	Required bool
	Minimum  *int64
	Maximum  *int64
	Pattern  *regexp.Regexp
	Default  any
}

// Binding is the Provider-visible portion of one trusted backend binding.
type Binding struct {
	BackendOperationID string
	Effect             string // READ or MUTATION
	HandlerProfile     string
	SourceID           string
	SourceOperationID  string
	RequestMediaType   string
	ResponseMediaTypes []string
	Arguments          []Argument
	// Available distinctions: a known binding that is unavailable because the
	// deployment has not configured its trusted service is reported by catalog
	// as NOT_CONFIGURED instead of being silently omitted.
	Available         bool
	UnavailableReason string // NOT_IMPLEMENTED or NOT_CONFIGURED
}

// ProblemDetails is a validated RFC 7807-style remote error mapped to a
// successful Provider result by a typed backend.
type ProblemDetails struct {
	Status int    `json:"status"`
	Cause  string `json:"cause"`
}

// Result is one completed backend invocation. HTTPStatus is always set.
// Response is the typed response body for successful remote responses;
// Problem is set only for a valid remote ProblemDetails response.
type Result struct {
	HTTPStatus int
	Response   any
	Problem    *ProblemDetails
}

// Runtime is the provider-local catalog and typed backend dispatch. It is
// created only after initialize has negotiated the protocol version. The
// implementation owns the trusted, embedded catalog and the typed clients; the
// protocol layer never accepts URLs, methods, paths, headers or credentials.
type Runtime interface {
	CatalogVersion() string
	CatalogDigest() string
	BindingIDs() []string
	Binding(id string) (Binding, bool)
	Invoke(ctx context.Context, backendOperationID string, arguments map[string]any) (Result, error)
	Close()
}

// NewRuntimeFunc builds the protocol-version-specific runtime. A configuration
// failure is reported as PROVIDER_CONFIGURATION_INVALID; a catalog failure as
// PROVIDER_BINDING_CATALOG_INVALID.
type NewRuntimeFunc func(protocolVersion string) (Runtime, error)

// ConfigError marks a trusted-configuration failure at initialize.
type ConfigError struct{ Reason string }

func (e *ConfigError) Error() string { return "provider configuration invalid: " + e.Reason }

// CatalogError marks an embedded trusted-catalog failure at initialize.
type CatalogError struct{ Reason string }

func (e *CatalogError) Error() string { return "provider binding catalog invalid: " + e.Reason }

// requestFrame is one parsed and envelope-validated request.
type requestFrame struct {
	ProtocolVersion string
	Type            string
	RequestID       string
	Method          string
	Params          map[string]any
}

type responseFrame struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Type            string     `json:"type"`
	RequestID       string     `json:"requestId"`
	OK              bool       `json:"ok"`
	Result          any        `json:"result,omitempty"`
	Error           *errorBody `json:"error,omitempty"`
}

type errorBody struct {
	Code   string         `json:"code"`
	Detail map[string]any `json:"detail,omitempty"`
}

// validOpaqueID reports a non-empty printable ASCII opaque identifier.
func validOpaqueID(value string) bool {
	if value == "" || len(value) > maxRequestIDLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func requireProtocolVersion(value string) *Fault {
	switch value {
	case ProtocolV1, ProtocolV2:
		return nil
	default:
		return fault(CodeProtocolVersionUnsupported)
	}
}

// decodeStrictObject decodes exactly one strict UTF-8 JSON object.
func decodeStrictObject(raw []byte) (map[string]any, *Fault) {
	if !utf8.Valid(raw) {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	decoded, err := jsonx.Decode(raw)
	if err != nil {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	return object, nil
}

func parseRequestFrame(raw []byte) (*requestFrame, *Fault) {
	object, f := decodeStrictObject(raw)
	if f != nil {
		return nil, f
	}
	allowed := map[string]struct{}{
		"protocolVersion": {}, "type": {}, "requestId": {}, "method": {}, "params": {},
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return nil, fault(CodeProtocolFrameInvalid)
		}
	}
	protocolVersion, ok := object["protocolVersion"].(string)
	if !ok {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	if requireProtocolVersion(protocolVersion) != nil {
		return nil, fault(CodeProtocolVersionUnsupported)
	}
	frameType, ok := object["type"].(string)
	if !ok || frameType != "request" {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	requestID, ok := object["requestId"].(string)
	if !ok || !validOpaqueID(requestID) {
		return nil, fault(CodeProtocolRequestIDInvalid)
	}
	method, ok := object["method"].(string)
	if !ok || method == "" {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	params, ok := object["params"].(map[string]any)
	if !ok {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	return &requestFrame{
		ProtocolVersion: protocolVersion,
		Type:            frameType,
		RequestID:       requestID,
		Method:          method,
		Params:          params,
	}, nil
}

func exactKeys(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func stringField(object map[string]any, key string) (string, bool) {
	value, ok := object[key].(string)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

func integerField(object map[string]any, key string) (int64, bool) {
	number, ok := object[key].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseInitializeParams(params map[string]any) (hostID, providerInstanceID string, limits Limits, f *Fault) {
	if !exactKeys(params, "hostInstanceId", "providerInstanceId", "limits") {
		return "", "", Limits{}, fault(CodeProtocolFrameInvalid)
	}
	hostID, ok := stringField(params, "hostInstanceId")
	if !ok || !validOpaqueID(hostID) {
		return "", "", Limits{}, fault(CodeProviderConfigurationInvalid)
	}
	providerInstanceID, ok = stringField(params, "providerInstanceId")
	if !ok || !validOpaqueID(providerInstanceID) {
		return "", "", Limits{}, fault(CodeProviderConfigurationInvalid)
	}
	rawLimits, ok := params["limits"].(map[string]any)
	if !ok || !exactKeys(rawLimits, "maxFrameBytes", "maxInFlight", "maxCallDurationMs") {
		return "", "", Limits{}, fault(CodeProtocolFrameInvalid)
	}
	frameBytes, ok := integerField(rawLimits, "maxFrameBytes")
	if !ok || frameBytes <= 0 || frameBytes > HardMaxFrameBytes {
		return "", "", Limits{}, fault(CodeProviderConfigurationInvalid)
	}
	inFlight, ok := integerField(rawLimits, "maxInFlight")
	if !ok || inFlight < 1 || inFlight > HardMaxInFlight {
		return "", "", Limits{}, fault(CodeProviderConfigurationInvalid)
	}
	callDuration, ok := integerField(rawLimits, "maxCallDurationMs")
	if !ok || callDuration < 1 || callDuration > HardMaxCallDuration.Milliseconds() {
		return "", "", Limits{}, fault(CodeProviderConfigurationInvalid)
	}
	return hostID, providerInstanceID, Limits{
		MaxFrameBytes:   frameBytes,
		MaxInFlight:     int(inFlight),
		MaxCallDuration: time.Duration(callDuration) * time.Millisecond,
	}, nil
}

func parseCatalogParams(params map[string]any) ([]string, *Fault) {
	if !exactKeys(params, "backendOperationIds") {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	rawIDs, ok := params["backendOperationIds"].([]any)
	if !ok {
		return nil, fault(CodeProtocolFrameInvalid)
	}
	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id, ok := rawID.(string)
		if !ok || !validOpaqueID(id) {
			return nil, fault(CodeProtocolFrameInvalid)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fault(CodeProtocolFrameInvalid)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

type invocationMetadata struct {
	InvocationID         string
	ProducerInvocationID string
	Deadline             time.Time
}

func parseInvokeParams(params map[string]any) (string, map[string]any, invocationMetadata, *Fault) {
	if !exactKeys(params, "backendOperationId", "arguments", "invocation") {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	operationID, ok := stringField(params, "backendOperationId")
	if !ok || !validOpaqueID(operationID) {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	arguments, ok := params["arguments"].(map[string]any)
	if !ok {
		return "", nil, invocationMetadata{}, fault(CodeBackendArgumentsInvalid)
	}
	rawInvocation, ok := params["invocation"].(map[string]any)
	if !ok || !exactKeys(rawInvocation, "invocationId", "producerInvocationId", "deadline") {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	invocationID, ok := stringField(rawInvocation, "invocationId")
	if !ok || !validOpaqueID(invocationID) {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	producerInvocationID, ok := stringField(rawInvocation, "producerInvocationId")
	if !ok || !validOpaqueID(producerInvocationID) {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	deadlineText, ok := stringField(rawInvocation, "deadline")
	if !ok || !strings.HasSuffix(deadlineText, "Z") {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	deadline, err := time.Parse(time.RFC3339, deadlineText)
	if err != nil {
		return "", nil, invocationMetadata{}, fault(CodeProtocolFrameInvalid)
	}
	return operationID, arguments, invocationMetadata{
		InvocationID:         invocationID,
		ProducerInvocationID: producerInvocationID,
		Deadline:             deadline.UTC(),
	}, nil
}

func parseCancelParams(params map[string]any) (string, *Fault) {
	if !exactKeys(params, "targetRequestId") {
		return "", fault(CodeProtocolFrameInvalid)
	}
	target, ok := stringField(params, "targetRequestId")
	if !ok || !validOpaqueID(target) {
		return "", fault(CodeProtocolRequestIDInvalid)
	}
	return target, nil
}

// validateArguments applies the trusted binding argument schema and returns a
// fresh argument map. Unknown, missing, mistyped or out-of-range arguments are
// rejected before any network access. Defaults come only from trusted catalog
// content, never from the caller.
func validateArguments(binding Binding, arguments map[string]any) (map[string]any, *Fault) {
	declared := make(map[string]Argument, len(binding.Arguments))
	for _, argument := range binding.Arguments {
		declared[argument.Name] = argument
	}
	for name := range arguments {
		if _, ok := declared[name]; !ok {
			return nil, faultDetail(CodeBackendArgumentsInvalid, "argument", name)
		}
	}
	result := make(map[string]any, len(binding.Arguments))
	for _, argument := range binding.Arguments {
		value, present := arguments[argument.Name]
		if !present {
			if argument.Required {
				return nil, faultDetail(CodeBackendArgumentsInvalid, "argument", argument.Name)
			}
			if argument.Default != nil {
				result[argument.Name] = argument.Default
			}
			continue
		}
		normalized, ok := validateArgumentValue(argument, value)
		if !ok {
			return nil, faultDetail(CodeBackendArgumentsInvalid, "argument", argument.Name)
		}
		result[argument.Name] = normalized
	}
	return result, nil
}

func validateArgumentValue(argument Argument, value any) (any, bool) {
	switch argument.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		if argument.Pattern != nil && !argument.Pattern.MatchString(text) {
			return nil, false
		}
		return text, true
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return nil, false
		}
		parsed, err := number.Int64()
		if err != nil {
			return nil, false
		}
		if argument.Minimum != nil && parsed < *argument.Minimum {
			return nil, false
		}
		if argument.Maximum != nil && parsed > *argument.Maximum {
			return nil, false
		}
		return parsed, true
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, false
		}
		return boolean, true
	default:
		return nil, false
	}
}

var errFrameTooLarge = errors.New("frame too large")
var errUnexpectedEOF = errors.New("unexpected end of frame")

// describeBinding projects one trusted binding into the private v2 catalog
// descriptor. It exposes identity, effect, media types, typed arguments and
// pinned source identity only: never an origin, URL, method, route, header,
// credential, public callable ID, Capability ID, Outcome or Evidence.
func describeBinding(binding Binding) map[string]any {
	arguments := make([]map[string]any, 0, len(binding.Arguments))
	for _, argument := range binding.Arguments {
		entry := map[string]any{
			"name":     argument.Name,
			"type":     argument.Type,
			"required": argument.Required,
		}
		if argument.Minimum != nil {
			entry["minimum"] = *argument.Minimum
		}
		if argument.Maximum != nil {
			entry["maximum"] = *argument.Maximum
		}
		if argument.Default != nil {
			entry["default"] = argument.Default
		}
		arguments = append(arguments, entry)
	}
	return map[string]any{
		"backendOperationId": binding.BackendOperationID,
		"effect":             binding.Effect,
		"request": map[string]any{
			"mediaType": binding.RequestMediaType,
		},
		"response": map[string]any{
			"mediaTypes": binding.ResponseMediaTypes,
		},
		"arguments": arguments,
		"source": map[string]any{
			"sourceId":    binding.SourceID,
			"operationId": binding.SourceOperationID,
		},
	}
}

func marshalResponse(frame responseFrame) ([]byte, error) {
	return json.Marshal(frame)
}

func errorResponseBody(f *Fault) *errorBody {
	if f == nil {
		return &errorBody{Code: CodeProviderInternal}
	}
	return &errorBody{Code: f.Code, Detail: f.Detail}
}

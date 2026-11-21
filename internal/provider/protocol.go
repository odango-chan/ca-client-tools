// Package provider implements the resident CA Client Tools Provider process
// protocol (ca-client-tools.provider/v2). It is a private Host-to-Provider
// process boundary: it owns framing, correlation, bounded concurrency,
// deadlines, cancellation and safe error mapping. It does not own public
// callable identity, Capability authorization, Scope, Owner Outcomes or
// Evidence.
//
// There is exactly one accepted wire identity. The legacy v1 identity was
// removed in P0 with no compatibility path: a frame carrying it is rejected
// with PROTOCOL_VERSION_UNSUPPORTED.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/odango-chan/ca-client-tools/internal/jsonx"
)

const (
	// ProtocolVersion is the sole accepted Provider process wire identity. The
	// suffix identifies the consolidated contract; there is no version
	// negotiation or fallback.
	ProtocolVersion = "ca-client-tools.provider/v2"

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
	// Provider protocol faults produced by the P1 Host Artifact Store channel
	// (contract §4.4). Host channel codes never cross to the Runtime caller.
	CodeArtifactReferenceInvalid = "ARTIFACT_REFERENCE_INVALID"
	CodeArtifactTransferFailed   = "ARTIFACT_TRANSFER_FAILED"
	CodeProviderInternal         = "PROVIDER_INTERNAL"
	CodeProviderShuttingDown     = "PROVIDER_SHUTTING_DOWN"
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

// ArtifactChannelConfig is the validated bootstrap of the Host Artifact Store
// channel passed in initialize (contract §4.1). The zero value means no channel
// is configured: artifact-bearing bindings are then NOT_CONFIGURED. The opaque
// process token is carried in memory only and is never logged or echoed.
type ArtifactChannelConfig struct {
	BaseURL           string
	ProcessToken      string
	RequestTimeout    time.Duration
	MaxArtifactBytes  int64
	MaxAggregateBytes int64
	MaxAttachments    int
}

// Configured reports whether the Host supplied an artifact channel.
func (c ArtifactChannelConfig) Configured() bool { return c.BaseURL != "" }

// ArtifactLimits are the effective artifact bounds the Provider enforces and
// reports from initialize. They are the smaller of the Host-advertised bounds
// and the Provider's own payload bounds.
type ArtifactLimits struct {
	RequestTimeout    time.Duration
	MaxArtifactBytes  int64
	MaxAggregateBytes int64
	MaxAttachments    int
}

func (l ArtifactLimits) wire() map[string]any {
	return map[string]any{
		"requestTimeoutMs":  l.RequestTimeout.Milliseconds(),
		"maxArtifactBytes":  l.MaxArtifactBytes,
		"maxAggregateBytes": l.MaxAggregateBytes,
		"maxAttachments":    l.MaxAttachments,
	}
}

// ArtifactSlot is one declared artifact-bearing location of a binding
// (contract §3). Slot is the catalog part name (or "body" for a whole binary
// body), Pointer is the argument location for inputs or the result location
// inside response for outputs, MediaType is the declared part/body media type
// and Required mirrors the catalog flag. It carries no origin, path, token or
// Content-ID.
type ArtifactSlot struct {
	Slot      string
	Pointer   string
	MediaType string
	Required  bool
}

// ArtifactSlots is the closed artifactSlots descriptor extension. It is present
// only when at least one direction declares an artifact slot.
type ArtifactSlots struct {
	Inputs  []ArtifactSlot
	Outputs []ArtifactSlot
}

// Configured reports whether the binding is artifact-bearing.
func (s *ArtifactSlots) Configured() bool {
	return s != nil && (len(s.Inputs) > 0 || len(s.Outputs) > 0)
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
	// ArgumentSchema is the closed nested invocation schema of a genericSBI
	// binding. It is nil for typed bindings, which use Arguments instead.
	ArgumentSchema json.RawMessage
	// ArtifactSlots is the closed artifactSlots descriptor extension of an
	// artifact-bearing genericSBI binding. It is nil for every other binding.
	ArtifactSlots *ArtifactSlots
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
	// ArtifactLimits reports the effective artifact bounds when the Host
	// configured an artifact channel for this process.
	ArtifactLimits() (ArtifactLimits, bool)
	Invoke(ctx context.Context, backendOperationID, invocationID string, arguments map[string]any) (Result, error)
	Close()
}

// NewRuntimeFunc builds the single trusted runtime after the handshake. The
// validated artifact-channel bootstrap is passed through unchanged; the zero
// value means artifact streaming is not configured. A configuration failure is
// reported as PROVIDER_CONFIGURATION_INVALID; a catalog failure as
// PROVIDER_BINDING_CATALOG_INVALID.
type NewRuntimeFunc func(ArtifactChannelConfig) (Runtime, error)

// HostInvocation is the Host-private occurrence identity supplied with an
// invoke frame for a typed MAS callable. It is never part of the caller's
// business arguments: the Host derives it from its own durable Run and
// dispatch claim, and the Provider only consumes it. Verification of
// authorization and at-most-once dispatch stays with the Host journal.
type HostInvocation struct {
	Version              string
	DeploymentID         string
	OwnerInstanceID      string
	OriginalInvocationID string
	SessionRef           string
	SMContextRef         string
	SUPI                 string
	PSI                  int64
}

// ContextualRuntime is implemented by a Runtime that accepts the Host-private
// invocation scope. A plain Runtime keeps working for read-only bindings and
// rejects an invocation that carries the private scope.
type ContextualRuntime interface {
	Runtime
	InvokeWithHostInvocation(ctx context.Context, backendOperationID, invocationID string,
		arguments map[string]any, host HostInvocation) (Result, error)
}

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
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
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
	if value == ProtocolVersion {
		return nil
	}
	return fault(CodeProtocolVersionUnsupported)
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

func parseInitializeParams(params map[string]any) (hostID, providerInstanceID string, limits Limits, channel ArtifactChannelConfig, f *Fault) {
	allowed := map[string]struct{}{
		"hostInstanceId": {}, "providerInstanceId": {}, "limits": {}, "artifactChannel": {},
	}
	for key := range params {
		if _, ok := allowed[key]; !ok {
			return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProtocolFrameInvalid)
		}
	}
	for _, required := range []string{"hostInstanceId", "providerInstanceId", "limits"} {
		if _, ok := params[required]; !ok {
			return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProtocolFrameInvalid)
		}
	}
	hostID, ok := stringField(params, "hostInstanceId")
	if !ok || !validOpaqueID(hostID) {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	providerInstanceID, ok = stringField(params, "providerInstanceId")
	if !ok || !validOpaqueID(providerInstanceID) {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	rawLimits, ok := params["limits"].(map[string]any)
	if !ok || !exactKeys(rawLimits, "maxFrameBytes", "maxInFlight", "maxCallDurationMs") {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProtocolFrameInvalid)
	}
	frameBytes, ok := integerField(rawLimits, "maxFrameBytes")
	if !ok || frameBytes <= 0 || frameBytes > HardMaxFrameBytes {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	inFlight, ok := integerField(rawLimits, "maxInFlight")
	if !ok || inFlight < 1 || inFlight > HardMaxInFlight {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	callDuration, ok := integerField(rawLimits, "maxCallDurationMs")
	if !ok || callDuration < 1 || callDuration > HardMaxCallDuration.Milliseconds() {
		return "", "", Limits{}, ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	limits = Limits{
		MaxFrameBytes:   frameBytes,
		MaxInFlight:     int(inFlight),
		MaxCallDuration: time.Duration(callDuration) * time.Millisecond,
	}
	if rawChannel, present := params["artifactChannel"]; present {
		channel, f = parseArtifactChannel(rawChannel, limits.MaxCallDuration)
		if f != nil {
			return "", "", Limits{}, ArtifactChannelConfig{}, f
		}
	}
	return hostID, providerInstanceID, limits, channel, nil
}

var artifactBaseURLPattern = regexp.MustCompile(`^http://127\.0\.0\.1:([1-9][0-9]{0,4})$`)

// parseArtifactChannel validates the closed artifactChannel bootstrap per
// contract §4.1. Every malformed value is PROVIDER_CONFIGURATION_INVALID; the
// process token is validated as an opaque bounded printable secret and is never
// echoed.
func parseArtifactChannel(raw any, maxCallDuration time.Duration) (ArtifactChannelConfig, *Fault) {
	invalid := func() (ArtifactChannelConfig, *Fault) {
		return ArtifactChannelConfig{}, fault(CodeProviderConfigurationInvalid)
	}
	object, ok := raw.(map[string]any)
	if !ok || !exactKeys(object, "baseUrl", "processToken", "requestTimeoutMs",
		"maxArtifactBytes", "maxAggregateBytes", "maxAttachments") {
		return invalid()
	}
	baseURL, ok := stringField(object, "baseUrl")
	if !ok {
		return invalid()
	}
	match := artifactBaseURLPattern.FindStringSubmatch(baseURL)
	if match == nil {
		return invalid()
	}
	port, err := strconv.Atoi(match[1])
	if err != nil || port < 1 || port > 65535 {
		return invalid()
	}
	processToken, ok := object["processToken"].(string)
	if !ok || len(processToken) < 1 || len(processToken) > 512 {
		return invalid()
	}
	for index := 0; index < len(processToken); index++ {
		if processToken[index] < 0x20 || processToken[index] > 0x7e {
			return invalid()
		}
	}
	timeoutMs, ok := integerField(object, "requestTimeoutMs")
	if !ok || timeoutMs < 1 || timeoutMs > maxCallDuration.Milliseconds() ||
		timeoutMs > HardMaxCallDuration.Milliseconds() {
		return invalid()
	}
	maxArtifactBytes, ok := integerField(object, "maxArtifactBytes")
	if !ok || maxArtifactBytes < 1 || maxArtifactBytes > 1<<30 {
		return invalid()
	}
	maxAggregateBytes, ok := integerField(object, "maxAggregateBytes")
	if !ok || maxAggregateBytes < 1 || maxAggregateBytes > 1<<30 {
		return invalid()
	}
	maxAttachments, ok := integerField(object, "maxAttachments")
	if !ok || maxAttachments < 1 || maxAttachments > 256 {
		return invalid()
	}
	return ArtifactChannelConfig{
		BaseURL:           baseURL,
		ProcessToken:      processToken,
		RequestTimeout:    time.Duration(timeoutMs) * time.Millisecond,
		MaxArtifactBytes:  maxArtifactBytes,
		MaxAggregateBytes: maxAggregateBytes,
		MaxAttachments:    int(maxAttachments),
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

type invokeEnvelope struct {
	OperationID string
	Arguments   map[string]any
	Invocation  invocationMetadata
	Host        *HostInvocation
}

func parseInvokeParams(params map[string]any) (invokeEnvelope, *Fault) {
	_, hasHost := params["hostInvocation"]
	if hasHost {
		if !exactKeys(params, "backendOperationId", "arguments", "invocation", "hostInvocation") {
			return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
		}
	} else if !exactKeys(params, "backendOperationId", "arguments", "invocation") {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	operationID, ok := stringField(params, "backendOperationId")
	if !ok || !validOpaqueID(operationID) {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	arguments, ok := params["arguments"].(map[string]any)
	if !ok {
		return invokeEnvelope{}, fault(CodeBackendArgumentsInvalid)
	}
	rawInvocation, ok := params["invocation"].(map[string]any)
	if !ok || !exactKeys(rawInvocation, "invocationId", "producerInvocationId", "deadline") {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	invocationID, ok := stringField(rawInvocation, "invocationId")
	if !ok || !validOpaqueID(invocationID) {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	producerInvocationID, ok := stringField(rawInvocation, "producerInvocationId")
	if !ok || !validOpaqueID(producerInvocationID) {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	deadlineText, ok := stringField(rawInvocation, "deadline")
	if !ok || !strings.HasSuffix(deadlineText, "Z") {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	deadline, err := time.Parse(time.RFC3339, deadlineText)
	if err != nil {
		return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
	}
	envelope := invokeEnvelope{
		OperationID: operationID,
		Arguments:   arguments,
		Invocation: invocationMetadata{
			InvocationID:         invocationID,
			ProducerInvocationID: producerInvocationID,
			Deadline:             deadline.UTC(),
		},
	}
	if hasHost {
		host, ok := params["hostInvocation"].(map[string]any)
		if !ok || !exactKeys(host, "version", "deploymentId", "ownerInstanceId",
			"originalInvocationId", "sessionRef", "supi", "psi") &&
			!exactKeys(host, "version", "deploymentId", "ownerInstanceId",
				"originalInvocationId", "sessionRef", "smContextRef", "supi", "psi") {
			return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
		}
		parsed, parsedOK := parseHostInvocation(host)
		if !parsedOK {
			return invokeEnvelope{}, fault(CodeProtocolFrameInvalid)
		}
		envelope.Host = &parsed
	}
	return envelope, nil
}

// parseHostInvocation validates the Host-private occurrence identity. It is
// deliberately strict: every field is opaque printable ASCII and the PSI is in
// the 5GSM range. The Provider never fills a missing field from arguments.
func parseHostInvocation(object map[string]any) (HostInvocation, bool) {
	text := func(key string) (string, bool) {
		value, ok := object[key].(string)
		if !ok || !validOpaqueID(value) {
			return "", false
		}
		return value, true
	}
	version, ok := text("version")
	if !ok || version != masHostInvocationVersion {
		return HostInvocation{}, false
	}
	deploymentID, ok := text("deploymentId")
	if !ok {
		return HostInvocation{}, false
	}
	ownerInstanceID, ok := text("ownerInstanceId")
	if !ok {
		return HostInvocation{}, false
	}
	originalInvocationID, ok := text("originalInvocationId")
	if !ok {
		return HostInvocation{}, false
	}
	sessionRef, ok := text("sessionRef")
	if !ok {
		return HostInvocation{}, false
	}
	supi, ok := text("supi")
	if !ok || !validSUPI(supi) {
		return HostInvocation{}, false
	}
	psi, ok := integerField(object, "psi")
	if !ok || psi < 1 || psi > 15 {
		return HostInvocation{}, false
	}
	host := HostInvocation{
		Version:              version,
		DeploymentID:         deploymentID,
		OwnerInstanceID:      ownerInstanceID,
		OriginalInvocationID: originalInvocationID,
		SessionRef:           sessionRef,
		SUPI:                 supi,
		PSI:                  psi,
	}
	if raw, present := object["smContextRef"]; present {
		value, ok := raw.(string)
		if !ok || !validOpaqueID(value) {
			return HostInvocation{}, false
		}
		host.SMContextRef = value
	}
	return host, true
}

// masHostInvocationVersion is the exact version of the private Host scope.
const masHostInvocationVersion = "openamsm.mas-host-invocation/v1"

// validSUPI accepts the canonical 3GPP SUPI spelling used by OpenAMSM.
func validSUPI(value string) bool {
	const prefix = "imsi-"
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) || len(value) > len(prefix)+15 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
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
//
// A genericSBI binding is validated recursively against its closed
// ArgumentSchema by the runtime, which owns the compiled schema; the protocol
// layer passes the decoded JSON values through unchanged so nested objects,
// arrays, nulls and number precision survive.
func validateArguments(binding Binding, arguments map[string]any) (map[string]any, *Fault) {
	if binding.HandlerProfile == "genericSBI" {
		return arguments, nil
	}
	declared := make(map[string]Argument, len(binding.Arguments))
	for _, argument := range binding.Arguments {
		declared[argument.Name] = argument
	}
	for name := range arguments {
		if _, ok := declared[name]; !ok {
			// The name is caller-controlled. Do not echo it in a protocol
			// detail field where it could carry arbitrary or sensitive text.
			return nil, fault(CodeBackendArgumentsInvalid)
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
	case "object":
		// Closed nested objects are validated by the typed binding itself;
		// the protocol layer only fixes the JSON kind here.
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		return object, true
	default:
		return nil, false
	}
}

var errFrameTooLarge = errors.New("frame too large")
var errUnexpectedEOF = errors.New("unexpected end of frame")

// describeBinding projects one trusted binding into the private catalog
// descriptor. It exposes identity, effect, media types, typed arguments or the
// generic argument schema, and pinned source identity only: never an origin,
// URL, method, route, header, credential, public callable ID, Capability ID,
// Outcome or Evidence.
func describeBinding(binding Binding) map[string]any {
	description := map[string]any{
		"backendOperationId": binding.BackendOperationID,
		"effect":             binding.Effect,
		"request": map[string]any{
			"mediaType": binding.RequestMediaType,
		},
		"response": map[string]any{
			"mediaTypes": binding.ResponseMediaTypes,
		},
		"source": map[string]any{
			"sourceId":    binding.SourceID,
			"operationId": binding.SourceOperationID,
		},
	}
	if binding.HandlerProfile == "genericSBI" {
		// Generic bindings publish the closed nested invocation schema and omit
		// the scalar typed argument list.
		description["argumentSchema"] = binding.ArgumentSchema
		if binding.ArtifactSlots.Configured() {
			description["artifactSlots"] = describeArtifactSlots(binding.ArtifactSlots)
		}
		return description
	}
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
	description["arguments"] = arguments
	return description
}

func describeArtifactSlots(slots *ArtifactSlots) map[string]any {
	convert := func(entries []ArtifactSlot) []map[string]any {
		result := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			result = append(result, map[string]any{
				"slot":      entry.Slot,
				"pointer":   entry.Pointer,
				"mediaType": entry.MediaType,
				"required":  entry.Required,
			})
		}
		return result
	}
	return map[string]any{
		"inputs":  convert(slots.Inputs),
		"outputs": convert(slots.Outputs),
	}
}

func marshalResponse(frame responseFrame) ([]byte, error) {
	return json.Marshal(frame)
}

func errorResponseBody(f *Fault) *errorBody {
	if f == nil {
		return &errorBody{Code: CodeProviderInternal, Message: safeFaultMessage(CodeProviderInternal)}
	}
	return &errorBody{Code: f.Code, Message: safeFaultMessage(f.Code), Detail: f.Detail}
}

// safeFaultMessage returns a stable, non-sensitive message for the protocol
// error registry. It deliberately excludes internal errors, caller values,
// remote response text, credentials and subscriber identifiers.
func safeFaultMessage(code string) string {
	switch code {
	case CodeProtocolVersionUnsupported:
		return "Protocol version is unsupported."
	case CodeProtocolFrameInvalid:
		return "Protocol frame is invalid."
	case CodeProtocolFrameTooLarge:
		return "Protocol frame exceeds the negotiated size limit."
	case CodeProtocolRequestIDInvalid:
		return "Request ID is invalid or has already been used."
	case CodeProtocolMethodUnsupported:
		return "Protocol method is unsupported."
	case CodeProviderNotInitialized:
		return "Provider must be initialized before this request."
	case CodeProviderAlreadyInitialized:
		return "Provider has already been initialized."
	case CodeProviderConfigurationInvalid:
		return "Provider trusted configuration is invalid."
	case CodeProviderBindingCatalogInvalid:
		return "Provider binding catalog is invalid."
	case CodeBackendOperationUnsupported:
		return "Backend operation is unsupported or unavailable."
	case CodeBackendArgumentsInvalid:
		return "Backend operation arguments are invalid."
	case CodeCallDeadlineExceeded:
		return "Invocation deadline was exceeded."
	case CodeCallCancelled:
		return "Invocation was cancelled."
	case CodeRemoteTransportError:
		return "Remote SBI request did not complete."
	case CodeRemoteResponseInvalid:
		return "Remote SBI response is invalid."
	case CodeArtifactReferenceInvalid:
		return "Artifact reference is invalid or no longer resolvable."
	case CodeArtifactTransferFailed:
		return "Artifact transfer did not complete."
	case CodeProviderShuttingDown:
		return "Provider is shutting down."
	case CodeProviderInternal:
		return "Provider encountered an internal error."
	default:
		return "Provider request failed."
	}
}

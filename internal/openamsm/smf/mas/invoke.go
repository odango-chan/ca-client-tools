package mas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

const invokePath = "/nsmf-mas-orchestration/v1/invoke"
const HostInvocationVersion = "openamsm.mas-host-invocation/v1"

// HostInvocation is supplied by the authorized integration after ingress
// admission. It never comes from the operation argument object.
type HostInvocation struct {
	Version, DeploymentID, OwnerInstanceID               string
	OriginalInvocationID, SessionRef, SMContextRef, SUPI string
	PSI                                                  int
}

// Occurrence binds an original operation to the exact first request and owner
// instance. It is private to the Host and Provider.
type Occurrence struct {
	HostInvocation
	Callable, IdempotencyKey, RequestDigest string
}

type CommitState string

const (
	Committed    CommitState = "COMMITTED"
	Deferred     CommitState = "DEFERRED"
	Denied       CommitState = "DENIED"
	NotCommitted CommitState = "NOT_COMMITTED"
	Unknown      CommitState = "OUTCOME_UNKNOWN"
)

// Decision is an authenticated, durable Owner decision returned by the Host
// journal. ProofRef identifies exact occurrence proof (or a durable pending
// handle for Deferred). An HTTP observation cannot create this record.
type Decision struct {
	Occurrence  Occurrence
	CommitState CommitState
	ProofRef    string
}

// Observation is transport information only. Outputs other than SMContextRef
// are unqualified business projections and cannot authorize graph progression.
type Observation struct {
	HTTPStatus   int
	Code         string
	Outputs      map[string]string
	SMContextRef string
	// RawOutputNames lists the Owner envelope output keys when the response
	// parses, even if the fixed projection rejects it. It is diagnostic only
	// and never carries a value.
	RawOutputNames []string
}

// DispatchJournal is implemented by the Host. Claim MUST durably and atomically
// bind the session reference to deployment/owner/SUPI/PSI, reject conflicting
// original IDs and request digests, and grant dispatch at most once. The claim
// survives crash, timeout and cancellation even without RecordObservation.
// ReadDecision is an exact-occurrence, read-only lookup of verified Owner proof.
// Neither method may redispatch, substitute session state or invoke compensation.
type DispatchJournal interface {
	Claim(context.Context, Occurrence) (granted bool, err error)
	RecordObservation(context.Context, Occurrence, Observation) error
	ReadDecision(context.Context, Occurrence) (Decision, error)
}

type InvokeOutcome struct {
	OriginalInvocationID string
	CommitState          CommitState
	ProofRef             string
	Observation          Observation
}

type InvokePrototype struct {
	wire    queryTransport
	journal DispatchJournal
	timeout time.Duration
}

// NewInvokePrototype constructs the private adapter. Production Provider v2
// activation remains gated until the Host supplies a qualified journal/Owner.
func NewInvokePrototype(origin, token string, timeout time.Duration, journal DispatchJournal) (*InvokePrototype, error) {
	u, err := parseOrigin(origin)
	if err != nil {
		return nil, err
	}
	if token == "" || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n") ||
		timeout <= 0 || timeout > maxTimeout || journal == nil {
		return nil, errors.New("invalid MAS Invoke deployment configuration")
	}
	const service = "openamsm-mas-invoke"
	client, err := sbi.NewClient(sbi.Config{
		Services:            map[string]sbi.ServiceConfig{service: {Origin: u.String(), HeaderTokenProfile: service}},
		HeaderTokenProfiles: map[string]sbi.HeaderTokenProfile{service: {Header: "X-OpenAMSM-Orch-Token", Token: token}},
		Limits:              sbi.Limits{RequestTimeout: timeout, MaxResponseBytes: defaultMaxResponse},
	})
	if err != nil {
		return nil, errors.New("invalid MAS Invoke transport configuration")
	}
	return &InvokePrototype{&sbiTransport{client: client, serviceKey: service}, journal, timeout}, nil
}

func (a *InvokePrototype) CloseIdleConnections() { a.wire.CloseIdleConnections() }

type invokeRequest struct {
	SessionRef     string            `json:"sessionRef"`
	SMContextRef   string            `json:"smContextRef,omitempty"`
	StepID         string            `json:"stepId"`
	MAS            string            `json:"mas"`
	Op             string            `json:"op"`
	Inputs         map[string]string `json:"inputs"`
	IdempotencyKey string            `json:"idempotencyKey"`
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func prepareInvoke(scope HostInvocation, callable string, raw []byte) (Occurrence, invokeBinding, []byte, error) {
	binding, ok := invokeBindings[callable]
	if !ok || scope.Version != HostInvocationVersion || !validSUPI(scope.SUPI) ||
		scope.PSI < 1 || scope.PSI > 15 || !privateRef(scope.SessionRef) ||
		!privateRef(scope.OriginalInvocationID) || !privateRef(scope.DeploymentID) ||
		!privateRef(scope.OwnerInstanceID) ||
		(scope.SMContextRef != "" && !privateRef(scope.SMContextRef)) {
		return Occurrence{}, binding, nil, errInvokeInput
	}
	inputs, err := binding.wireInputs(raw, scope)
	if err != nil {
		return Occurrence{}, binding, nil, err
	}
	// Exclude payload and per-attempt identity from the key. The journal rejects a
	// different digest on reuse of the original ID, rather than minting a new key.
	identity, _ := json.Marshal([3]string{scope.DeploymentID, scope.OwnerInstanceID, scope.OriginalInvocationID})
	key := digest(identity)
	body, _ := json.Marshal(invokeRequest{scope.SessionRef, scope.SMContextRef, binding.step, binding.mas, binding.op, inputs, key})
	requestIdentity, _ := json.Marshal([]any{scope, json.RawMessage(body)})
	return Occurrence{scope, callable, key, digest(requestIdentity)}, binding, body, nil
}

func privateRef(value string) bool {
	return len(value) > 0 && len(value) <= 127 && strings.IndexFunc(value, func(r rune) bool {
		return r <= 32 || r >= 127
	}) < 0
}

func (a *InvokePrototype) Invoke(ctx context.Context, scope HostInvocation, callable string, args []byte) (InvokeOutcome, error) {
	occurrence, binding, body, err := prepareInvoke(scope, callable, args)
	if err != nil {
		return InvokeOutcome{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	result := InvokeOutcome{OriginalInvocationID: scope.OriginalInvocationID, CommitState: Unknown}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	granted, err := a.journal.Claim(ctx, occurrence)
	if err != nil {
		return result, errors.New("MAS dispatch claim unavailable or conflicting")
	}
	if !granted {
		return a.reconcile(ctx, occurrence), nil
	}
	result.Observation = a.observe(ctx, binding, scope.SMContextRef, body)
	if err := a.journal.RecordObservation(ctx, occurrence, result.Observation); err != nil {
		return result, errors.New("MAS dispatch observation was not recorded")
	}
	if result.Observation.Code == "ACL_DENIED" {
		result.CommitState = Denied
	}
	return result, nil
}

// Reconcile looks up the exact original request without touching the SMF.
func (a *InvokePrototype) Reconcile(ctx context.Context, scope HostInvocation, callable string, args []byte) (InvokeOutcome, error) {
	occurrence, _, _, err := prepareInvoke(scope, callable, args)
	if err != nil {
		return InvokeOutcome{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return a.reconcile(ctx, occurrence), nil
}

func (a *InvokePrototype) reconcile(ctx context.Context, occurrence Occurrence) InvokeOutcome {
	result := InvokeOutcome{OriginalInvocationID: occurrence.OriginalInvocationID, CommitState: Unknown}
	decision, err := a.journal.ReadDecision(ctx, occurrence)
	if err != nil || decision.Occurrence != occurrence || !privateRef(decision.ProofRef) {
		return result
	}
	switch decision.CommitState {
	case Committed, Deferred, Denied, NotCommitted:
		result.CommitState, result.ProofRef = decision.CommitState, decision.ProofRef
	}
	return result
}

func (a *InvokePrototype) observe(ctx context.Context, binding invokeBinding, expectedSMContextRef string, body []byte) Observation {
	return observeWithWire(ctx, a.wire, binding, expectedSMContextRef, body)
}

func observeWithWire(ctx context.Context, wire queryTransport, binding invokeBinding, expectedSMContextRef string, body []byte) Observation {
	response, err := wire.Do(ctx, invokePath, "application/json", "application/json, application/problem+json", body)
	if err != nil {
		return Observation{Code: interruptionCode(ctx, err, "TRANSPORT_ERROR")}
	}
	defer response.Body.Close()
	observation := Observation{HTTPStatus: response.StatusCode, Code: "INVALID_RESPONSE"}
	raw, err := io.ReadAll(io.LimitReader(response.Body, defaultMaxResponse+1))
	if err != nil {
		observation.Code = interruptionCode(ctx, err, "INVALID_RESPONSE")
		return observation
	}
	if int64(len(raw)) > defaultMaxResponse {
		return observation
	}
	if response.StatusCode != http.StatusOK {
		problem, err := parseProblem(response.StatusCode, response.Header, raw)
		if err != nil {
			return observation
		}
		// Only the pinned pre-dispatch token rejection proves denial. Other 4xx
		// responses can follow resolver side effects or operation execution.
		switch {
		case problem.Status == 403 && problem.Cause == "ACL":
			observation.Code = "ACL_DENIED"
		case problem.Status == 504:
			observation.Code = "OWNER_TIMEOUT"
		default:
			observation.Code = "OWNER_ERROR"
		}
		return observation
	}
	if os.Getenv("CA_CLIENT_TOOLS_MAS_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "mas-debug status=%d contentType=%q body=%q\n",
			response.StatusCode, response.Header.Get("Content-Type"), string(raw))
	}
	observation.RawOutputNames = envelopeOutputNames(response.Header, raw)
	outputs, ref, ok := binding.project(response.Header, raw)
	if ok && (ref == "" || expectedSMContextRef == "" || ref == expectedSMContextRef) {
		observation.Code, observation.Outputs, observation.SMContextRef = "WIRE_OK", outputs, ref
	}
	return observation
}

// envelopeOutputNames returns the sorted Owner output keys without values.
func envelopeOutputNames(header http.Header, raw []byte) []string {
	media, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	if media != "application/json" {
		return nil
	}
	decoded, err := decodeUniqueJSON(raw)
	envelope, ok := decoded.(map[string]any)
	if err != nil || !ok {
		return nil
	}
	wire, ok := envelope["outputs"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(wire))
	for name := range wire {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func interruptionCode(ctx context.Context, err error, fallback string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "CANCELED"
	}
	return fallback
}

func (b invokeBinding) project(header http.Header, raw []byte) (map[string]string, string, bool) {
	media, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	decoded, err := decodeUniqueJSON(raw)
	envelope, ok := decoded.(map[string]any)
	if media != "application/json" || err != nil || !ok || len(envelope) != 5 ||
		envelope["status"] != "OK" || envelope["stepId"] != b.step ||
		envelope["mas"] != b.mas || envelope["op"] != b.op {
		return nil, "", false
	}
	wire, ok := envelope["outputs"].(map[string]any)
	if !ok {
		return nil, "", false
	}
	outputs := make(map[string]string)
	var ref string
	for _, spec := range strings.Fields(b.outputs) {
		optional := strings.HasSuffix(spec, "?")
		name, rule, _ := strings.Cut(strings.TrimSuffix(spec, "?"), ":")
		value, exists := wire[name]
		if optional && !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || !validOutput(rule, text) {
			return nil, "", false
		}
		delete(wire, name)
		switch name {
		case "sm_context_ref":
			ref = text
		case "secondary_auth_required":
			// Source-only discriminator for this profile.
			if text != "0" {
				return nil, "", false
			}
		case "n4_seid":
			// The legacy s11 MODIFY_UP_SESSION profile treats n4_seid as a
			// private scope ref. The typed QoS-delta callable declares it as an
			// optional public output and must not lose it.
			if b.step != "s11" || b.isQoSDelta() {
				outputs[name] = text
			}
		default:
			outputs[name] = text
		}
	}
	if len(wire) != 0 {
		return nil, "", false
	}
	if b.isQoSDelta() {
		state := outputs["qos_delta_state"]
		_, dispatched := outputs["n1n2_dispatched"]
		switch state {
		case "APPLIED":
			if !dispatched {
				return nil, "", false
			}
		case "ALREADY", "PFCP_APPLIED_N1N2_FAILED":
			if dispatched {
				return nil, "", false
			}
		default:
			return nil, "", false
		}
	}
	return outputs, ref, true
}

func canonicalUintIn(value string, minimum, maximum uint64) bool {
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && strconv.FormatUint(number, 10) == value &&
		number >= minimum && number <= maximum
}

func validOutput(rule, value string) bool {
	if rule == "hex" {
		// The Owner returns the N1/N2 command it built as opaque hex. These are
		// longer than the bounded reference scalars and carry no business
		// meaning to the Provider.
		return wireHex(value, 8192)
	}
	if !wireScalar(value) {
		return false
	}
	switch rule {
	case "":
		return true
	case "bit":
		return value == "0" || value == "1"
	case "ref":
		return privateRef(value)
	case "life":
		return value == "NONE" || value == "RESERVED" || value == "MODIFYING" || value == "OPERATIONAL" || value == "RELEASING"
	case "uint":
		number, err := strconv.ParseUint(value, 10, 64)
		// Canonical decimal only: the QoS Postcondition compares values as
		// strings, so a leading zero or sign must never reach the Host.
		return err == nil && strconv.FormatUint(number, 10) == value
	case "qosstate":
		return value == "APPLIED" || value == "ALREADY" || value == "PFCP_APPLIED_N1N2_FAILED"
	case "qfi":
		return canonicalUintIn(value, 1, 63)
	case "fiveqi":
		return canonicalUintIn(value, 1, 255)
	case "bitrate":
		return canonicalUintIn(value, 1, 9999999999999)
	case "ip":
		return net.ParseIP(value) != nil
	default:
		return value == rule
	}
}

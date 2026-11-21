package mas

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

// TypedField is one declared caller-visible argument of a fixed MAS callable.
// The Provider protocol validates the invocation object against this closed
// list before the typed binding is prepared; the binding itself then applies
// the fixed input projection and rejects any residual difference.
type TypedField struct {
	Name     string
	Type     string // "string", "integer" or "object"
	Required bool
}

// TypedBinding is the reviewed, fixed identity of one callable. A caller can
// select the callable but can never supply mas, op, stepId, the endpoint or
// the Host-private invocation scope.
type TypedBinding struct {
	Callable string
	Step     string
	MAS      string
	Op       string
	Fields   []TypedField
}

// OperationID is the private Provider backend operation id for this callable.
// It is stable, lower-case and derived from the callable only.
func (b TypedBinding) OperationID() string {
	return "openamsm.smf.mas." + strings.ToLower(b.Callable)
}

// Args returns the declared argument names in a deterministic order.
func (b TypedBinding) Args() []string {
	result := make([]string, 0, len(b.Fields))
	for _, field := range b.Fields {
		result = append(result, field.Name)
	}
	return result
}

// fieldType classifies the fixed input names. Names not listed are MAS
// reference/scalar strings; the projection in wireInputs still enforces the
// non-empty scalar, constant and reference rules.
func fieldType(name string) string {
	switch name {
	case "psi", "trigger":
		return "integer"
	case "sNssai":
		return "object"
	default:
		return "string"
	}
}

// TypedBindings returns the complete fixed callable table in deterministic
// order. It is derived from the same invokeBindings table that performs the
// actual projection, so a catalog binding can never drift from the wire
// contract: the Provider verifies both sides at startup.
func TypedBindings() []TypedBinding {
	callables := make([]string, 0, len(invokeBindings))
	for callable := range invokeBindings {
		callables = append(callables, callable)
	}
	sort.Strings(callables)
	result := make([]TypedBinding, 0, len(callables))
	for _, callable := range callables {
		binding := invokeBindings[callable]
		fields := []TypedField{{Name: "ue_identity", Type: "string", Required: true}}
		if binding.step != "s2" {
			fields = append(fields, TypedField{Name: "psi", Type: "integer", Required: true})
		}
		for _, field := range strings.Fields(binding.inputs) {
			name, _, _ := strings.Cut(field, "=")
			if name == "ue_identity" || name == "psi" {
				continue
			}
			fields = append(fields, TypedField{Name: name, Type: fieldType(name), Required: true})
		}
		result = append(result, TypedBinding{
			Callable: callable,
			Step:     binding.step,
			MAS:      binding.mas,
			Op:       binding.op,
			Fields:   fields,
		})
	}
	return result
}

// LookupTypedBinding returns the fixed binding for one callable.
func LookupTypedBinding(callable string) (TypedBinding, bool) {
	for _, binding := range TypedBindings() {
		if binding.Callable == callable {
			return binding, true
		}
	}
	return TypedBinding{}, false
}

// TypedInvoker is the Provider-side adapter that submits one fixed MAS
// callable to the private OpenAMSM orchestration endpoint. It owns no journal:
// the Host's durable dispatch claim is the single authority for at-most-once
// dispatch. The invoker derives the stable idempotency key from the Host
// occurrence identity so an OpenAMSM replay cannot perform the operation twice.
type TypedInvoker struct {
	wire    queryTransport
	timeout time.Duration
}

// NewTypedInvokerWithSBI builds the invoker on the one shared SBI transport.
func NewTypedInvokerWithSBI(client *sbi.Client, serviceKey string) (*TypedInvoker, error) {
	if client == nil || serviceKey == "" {
		return nil, errors.New("invalid MAS typed Invoke transport configuration")
	}
	return &TypedInvoker{wire: &sbiTransport{client: client, serviceKey: serviceKey}, timeout: 30 * time.Second}, nil
}

// SetTimeout bounds the owner round trip. The zero value keeps the default.
func (a *TypedInvoker) SetTimeout(timeout time.Duration) {
	if timeout > 0 && timeout <= maxTimeout {
		a.timeout = timeout
	}
}

func (a *TypedInvoker) CloseIdleConnections() { a.wire.CloseIdleConnections() }

// Invoke submits exactly one occurrence. It never retries and never converts
// an HTTP result into an authority decision: the returned Observation is
// transport/owner material only.
func (a *TypedInvoker) Invoke(ctx context.Context, scope HostInvocation, callable string, args []byte) (Observation, error) {
	_, binding, body, err := prepareInvoke(scope, callable, args)
	if err != nil {
		return Observation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	return observeWithWire(ctx, a.wire, binding, scope.SMContextRef, body), nil
}

// Replay submits the same exact occurrence again only when the Host can prove
// that the original dispatch never reached the Owner (for example the Host
// process crashed before the transport wrote the request). The Host never
// calls this after an ambiguous outcome; the OpenAMSM idempotency cache then
// returns the original response instead of performing the operation twice.
func (a *TypedInvoker) Replay(ctx context.Context, scope HostInvocation, callable string, args []byte) (Observation, error) {
	return a.Invoke(ctx, scope, callable, args)
}

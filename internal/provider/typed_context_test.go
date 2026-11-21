package provider

// Tests for the typed MAS Host-private invocation scope carried by the
// additive invoke frame parameter. The scope is never a caller argument and is
// accepted only by the typed MAS handler.

import (
	"context"
	"testing"
)

const typedMASOperationID = "openamsm.smf.mas.sc.validate_subscription"

type contextualFakeRuntime struct {
	*fakeRuntime
	host    *HostInvocation
	invoked int
}

func (f *contextualFakeRuntime) InvokeWithHostInvocation(ctx context.Context, backendOperationID, invocationID string,
	arguments map[string]any, host HostInvocation) (Result, error) {
	f.host = &host
	f.invoked++
	return Result{HTTPStatus: 200, Response: map[string]any{"observation": map[string]any{"code": "WIRE_OK"}}}, nil
}

func newContextualFake() *contextualFakeRuntime {
	base := newFakeCatalog()
	base.bindingIDs = append(base.bindingIDs, typedMASOperationID)
	base.bindings[typedMASOperationID] = Binding{
		BackendOperationID: typedMASOperationID,
		Effect:             "MUTATION",
		HandlerProfile:     "openamsmMASInvoke",
		SourceID:           "openamsm-mas-orchestration-openapi",
		SourceOperationID:  "SC.VALIDATE_SUBSCRIPTION",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/json"},
		Arguments: []Argument{
			{Name: "ue_identity", Type: "string", Required: true},
			{Name: "psi", Type: "integer", Required: true},
			{Name: "dnn", Type: "string", Required: true},
			{Name: "sNssai", Type: "object", Required: true},
		},
		Available: true,
	}
	return &contextualFakeRuntime{fakeRuntime: base}
}

func hostInvocationParams() map[string]any {
	return map[string]any{
		"version":              masHostInvocationVersion,
		"deploymentId":         "deployment:test",
		"ownerInstanceId":      "owner:smf-1",
		"originalInvocationId": "occ:run-1:node-1:0",
		"sessionRef":           "session:run-1",
		"supi":                 "imsi-001010000000001",
		"psi":                  5,
	}
}

func typedMASArguments() map[string]any {
	return map[string]any{
		"ue_identity": "imsi-001010000000001",
		"psi":         5,
		"dnn":         "internet",
		"sNssai":      map[string]any{"sst": 1},
	}
}

func TestInvokeCarriesHostScopeToTheTypedHandler(t *testing.T) {
	fake := newContextualFake()
	h := newHarness(t, buildRuntimeFactory(func() (Runtime, error) { return fake, nil }))
	h.mustInitialize(t, ProtocolVersion, "init-typed")
	params := invokeParams(typedMASOperationID, typedMASArguments(), "runtime-typed", "producer-typed", futureDeadline())
	params["hostInvocation"] = hostInvocationParams()
	h.send(t, wireRequest(ProtocolVersion, "invoke-typed", "invoke", params))
	frame := h.await(t, "invoke-typed")
	frame.expectOK(t, ProtocolVersion, "invoke-typed")
	if fake.invoked != 1 || fake.host == nil {
		t.Fatalf("typed handler invocations = %d, host = %#v", fake.invoked, fake.host)
	}
	if fake.host.SessionRef != "session:run-1" || fake.host.PSI != 5 || fake.host.SUPI != "imsi-001010000000001" {
		t.Fatalf("host scope = %#v", fake.host)
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

func TestInvokeRejectsHostScopeOnReadBinding(t *testing.T) {
	fake := newContextualFake()
	h := newHarness(t, buildRuntimeFactory(func() (Runtime, error) { return fake, nil }))
	h.mustInitialize(t, ProtocolVersion, "init-typed-read")
	params := invokeParams("openamsm.smf.mas.sc.query",
		map[string]any{"supi": "imsi-001010000000001", "pduSessionId": 5},
		"runtime-read", "producer-read", futureDeadline())
	params["hostInvocation"] = hostInvocationParams()
	h.send(t, wireRequest(ProtocolVersion, "invoke-read", "invoke", params))
	frame := h.await(t, "invoke-read")
	frame.expectError(t, ProtocolVersion, "invoke-read", CodeBackendArgumentsInvalid)
	if fake.invoked != 0 {
		t.Fatalf("typed handler ran for a read binding")
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

func TestInvokeRejectsTypedMASWithoutHostScope(t *testing.T) {
	fake := newContextualFake()
	h := newHarness(t, buildRuntimeFactory(func() (Runtime, error) { return fake, nil }))
	h.mustInitialize(t, ProtocolVersion, "init-typed-nohost")
	h.send(t, wireRequest(ProtocolVersion, "invoke-nohost", "invoke",
		invokeParams(typedMASOperationID, typedMASArguments(), "runtime-nohost", "producer-nohost", futureDeadline())))
	frame := h.await(t, "invoke-nohost")
	frame.expectError(t, ProtocolVersion, "invoke-nohost", CodeBackendArgumentsInvalid)
	if fake.invoked != 0 {
		t.Fatalf("typed handler ran without a Host scope")
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

func TestInvokeRejectsMalformedHostScope(t *testing.T) {
	fake := newContextualFake()
	h := newHarness(t, buildRuntimeFactory(func() (Runtime, error) { return fake, nil }))
	h.mustInitialize(t, ProtocolVersion, "init-typed-bad")
	host := hostInvocationParams()
	host["supi"] = "not-a-supi"
	params := invokeParams(typedMASOperationID, typedMASArguments(), "runtime-bad", "producer-bad", futureDeadline())
	params["hostInvocation"] = host
	h.send(t, wireRequest(ProtocolVersion, "invoke-bad", "invoke", params))
	frame := h.await(t, "invoke-bad")
	frame.expectError(t, ProtocolVersion, "invoke-bad", CodeProtocolFrameInvalid)
	if fake.invoked != 0 {
		t.Fatalf("typed handler ran with a malformed scope")
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

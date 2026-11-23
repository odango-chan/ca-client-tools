package providerapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/odango-chan/ca-client-tools/internal/provider"
)

// masInvokeRecorder records every private orchestration request so the test can
// assert the exact fixed step/mas/op and Host-private identity.
type masInvokeRecorder struct {
	mu       sync.Mutex
	requests []map[string]any
	token    string
}

func newMASInvokeServer(t *testing.T, recorder *masInvokeRecorder) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/nsmf-mas-orchestration/v1/invoke" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Header.Get("X-OpenAMSM-Orch-Token") != recorder.token {
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(writer, `{"status":403,"cause":"ACL"}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		recorder.mu.Lock()
		recorder.requests = append(recorder.requests, body)
		recorder.mu.Unlock()
		envelope := map[string]any{
			"status":  "OK",
			"stepId":  body["stepId"],
			"mas":     body["mas"],
			"op":      body["op"],
			"outputs": map[string]any{"validated": "1", "sub_summary": "ok", "secondary_auth_required": "0"},
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(envelope)
	})
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func typedMASRuntime(t *testing.T, recorder *masInvokeRecorder) provider.Runtime {
	t.Helper()
	server := newMASInvokeServer(t, recorder)
	return requireRuntime(t, map[string]string{
		EnvMASInvokeOrigin: server.URL,
		EnvMASInvokeToken:  recorder.token,
	})
}

func TestTypedMASBindingTableIsFixedAndAvailable(t *testing.T) {
	recorder := &masInvokeRecorder{token: "orch-secret"}
	runtime := typedMASRuntime(t, recorder)
	readOnly := map[string]bool{
		"openamsm.smf.mas.ssm.parse_n1_modify":  true,
		"openamsm.smf.mas.ssm.parse_n1_release": true,
	}
	found := 0
	for _, id := range runtime.BindingIDs() {
		if !strings.HasPrefix(id, "openamsm.smf.mas.") || strings.HasSuffix(id, ".query") {
			continue
		}
		found++
		binding, ok := runtime.Binding(id)
		wantEffect := "MUTATION"
		if readOnly[id] {
			wantEffect = "READ"
		}
		if !ok || !binding.Available || binding.HandlerProfile != "openamsmMASInvoke" || binding.Effect != wantEffect {
			t.Fatalf("typed MAS binding %s is not a fixed available %s: %#v", id, wantEffect, binding)
		}
	}
	if found != 31 {
		t.Fatalf("typed MAS bindings = %d, want 31", found)
	}
}

func TestTypedMASInvokeRequiresHostScopeAndUsesFixedIdentity(t *testing.T) {
	recorder := &masInvokeRecorder{token: "orch-secret"}
	runtime := typedMASRuntime(t, recorder)
	contextual, ok := runtime.(provider.ContextualRuntime)
	if !ok {
		t.Fatal("runtime does not accept the Host-private scope")
	}
	operationID := "openamsm.smf.mas.sc.validate_subscription"
	arguments := map[string]any{
		"ue_identity": "imsi-001010000000001",
		"psi":         int64(5),
		"dnn":         "internet",
		"sNssai":      map[string]any{"sst": int64(1)},
	}
	if _, err := runtime.Invoke(context.Background(), operationID, "inv-1", arguments); err == nil {
		t.Fatal("typed MAS invoke without a Host scope must fail closed")
	}
	host := provider.HostInvocation{
		Version:              "openamsm.mas-host-invocation/v1",
		DeploymentID:         "deployment:test",
		OwnerInstanceID:      "owner:smf-1",
		OriginalInvocationID: "occ:run-1:node-1:0",
		SessionRef:           "session:run-1",
		SUPI:                 "imsi-001010000000001",
		PSI:                  5,
	}
	result, err := contextual.InvokeWithHostInvocation(context.Background(), operationID, "inv-2", arguments, host)
	if err != nil {
		t.Fatal(err)
	}
	response, _ := result.Response.(map[string]any)
	observation, _ := response["observation"].(map[string]any)
	if observation["code"] != "WIRE_OK" || result.HTTPStatus != 200 {
		t.Fatalf("unexpected result: %#v", result)
	}
	outputs, _ := observation["outputs"].(map[string]any)
	if outputs["sub_summary"] != "ok" {
		t.Fatalf("owner outputs were not projected: %#v", observation)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("private orchestration requests = %d, want 1", len(recorder.requests))
	}
	request := recorder.requests[0]
	if request["stepId"] != "s1" || request["mas"] != "SC" || request["op"] != "VALIDATE_SUBSCRIPTION" {
		t.Fatalf("fixed step/mas/op drift: %#v", request)
	}
	if request["sessionRef"] != "session:run-1" {
		t.Fatalf("sessionRef = %v", request["sessionRef"])
	}
	idempotency, _ := request["idempotencyKey"].(string)
	if len(idempotency) != 64 {
		t.Fatalf("idempotencyKey = %q", idempotency)
	}
	inputs, _ := request["inputs"].(map[string]any)
	if inputs["ue_identity"] != "imsi-001010000000001" || inputs["s_nssai"] != "1" {
		t.Fatalf("projected inputs = %#v", inputs)
	}
}

func TestHostScopeIsRejectedOnNonMASBinding(t *testing.T) {
	recorder := &masInvokeRecorder{token: "orch-secret"}
	server := newMASInvokeServer(t, recorder)
	mas := newMASServer(t)
	runtime := requireRuntime(t, map[string]string{
		EnvSMFSBIOrigin:    mas.URL,
		EnvQueryToken:      "secret-token",
		EnvMASInvokeOrigin: server.URL,
		EnvMASInvokeToken:  recorder.token,
	})
	contextual := runtime.(provider.ContextualRuntime)
	host := provider.HostInvocation{
		Version:              "openamsm.mas-host-invocation/v1",
		DeploymentID:         "deployment:test",
		OwnerInstanceID:      "owner:smf-1",
		OriginalInvocationID: "occ:run-1:node-1:0",
		SessionRef:           "session:run-1",
		SUPI:                 "imsi-001010000000001",
		PSI:                  5,
	}
	if _, err := contextual.InvokeWithHostInvocation(context.Background(), "openamsm.smf.mas.pa.query", "inv-3",
		map[string]any{"supi": "imsi-001010000000001", "pduSessionId": int64(5)}, host); err == nil {
		t.Fatal("a read binding must reject the Host-private scope")
	}
}

func TestTypedMASOwnerErrorIsAnObservationNotAnOutcome(t *testing.T) {
	recorder := &masInvokeRecorder{token: "orch-secret"}
	server := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/problem+json")
		writer.WriteHeader(http.StatusGatewayTimeout)
		_, _ = fmt.Fprint(writer, `{"status":504,"cause":"E_TIMEOUT"}`)
	}), &http2.Server{}))
	server.Start()
	t.Cleanup(server.Close)
	runtime := requireRuntime(t, map[string]string{
		EnvMASInvokeOrigin: server.URL,
		EnvMASInvokeToken:  recorder.token,
	})
	contextual := runtime.(provider.ContextualRuntime)
	result, err := contextual.InvokeWithHostInvocation(context.Background(), "openamsm.smf.mas.sc.validate_subscription", "inv-4",
		map[string]any{
			"ue_identity": "imsi-001010000000001",
			"psi":         int64(5),
			"dnn":         "internet",
			"sNssai":      map[string]any{"sst": int64(1)},
		},
		provider.HostInvocation{
			Version:              "openamsm.mas-host-invocation/v1",
			DeploymentID:         "deployment:test",
			OwnerInstanceID:      "owner:smf-1",
			OriginalInvocationID: "occ:run-1:node-1:0",
			SessionRef:           "session:run-1",
			SUPI:                 "imsi-001010000000001",
			PSI:                  5,
		})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := result.Response.(map[string]any)
	observation, _ := response["observation"].(map[string]any)
	if observation["code"] != "OWNER_TIMEOUT" {
		t.Fatalf("owner timeout was not preserved as an observation: %#v", result)
	}
}

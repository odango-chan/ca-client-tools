//go:build providerfixture

package providerapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/provider"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// newFixtureUpstream is a deterministic loopback h2c upstream for the
// acceptance-only genericSBI bindings. It records hits and verifies trusted
// routing on the wire.
func newFixtureUpstream(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		if request.Header.Get("X-Acceptance-Trusted") != "fixture-7" {
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"status":403,"cause":"UNTRUSTED_ROUTE"}`))
			return
		}
		mode := request.URL.Query().Get("mode")
		switch mode {
		case "problem":
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"status":404,"cause":"ACCEPTANCE_NOT_FOUND"}`))
			return
		case "bad-problem":
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"cause":"not-json-shape"}`))
			return
		case "202":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"accepted":true}`))
			return
		case "204":
			writer.WriteHeader(http.StatusNoContent)
			return
		case "slow":
			<-request.Context().Done()
			return
		}
		body, _ := readAllJSON(request)
		items, _ := body["items"].([]any)
		writer.Header().Set("Content-Type", "application/json")
		encoded, _ := json.Marshal(map[string]any{
			"method":        request.Method,
			"path":          request.URL.Path,
			"mode":          mode,
			"number":        body["number"],
			"label":         body["label"],
			"itemCount":     len(items),
			"trustedHeader": request.Header.Get("X-Acceptance-Trusted"),
		})
		_, _ = writer.Write(encoded)
	})
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func readAllJSON(request *http.Request) (map[string]any, error) {
	var decoded map[string]any
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func fixtureRuntime(t *testing.T, upstream string) provider.Runtime {
	t.Helper()
	configPath := writeServiceConfig(t, `version: 1
services:
  acceptance-fixture-sbi:
    origin: `+upstream+`
    protocol: h2c
`)
	return requireRuntime(t, map[string]string{EnvServiceConfig: configPath})
}

func TestRuntimeGenericDispatchFixture(t *testing.T) {
	var hits atomic.Int64
	upstream := newFixtureUpstream(t, &hits)
	runtime := fixtureRuntime(t, upstream.URL)

	echo, ok := runtime.Binding("acceptance.generic.echo")
	if !ok || !echo.Available {
		t.Fatalf("fixture echo binding unavailable: %#v", echo)
	}
	if len(echo.ArgumentSchema) == 0 {
		t.Fatal("fixture generic binding has no published argument schema")
	}
	if echo.HandlerProfile != "genericSBI" || len(echo.Arguments) != 0 {
		t.Fatalf("unexpected fixture binding profile: %#v", echo)
	}

	result, err := runtime.Invoke(context.Background(), "acceptance.generic.echo", map[string]any{
		"body": map[string]any{
			"label":  "demo",
			"number": json.Number("9007199254740993"),
			"items": []any{
				map[string]any{"id": json.Number("1"), "tags": []any{"a", "b"}},
				map[string]any{"id": json.Number("2"), "weight": json.Number("1.5")},
			},
		},
	})
	if err != nil {
		t.Fatalf("generic invoke: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("httpStatus = %d", result.HTTPStatus)
	}
	response, ok := result.Response.(map[string]any)
	if !ok {
		t.Fatalf("response = %#v", result.Response)
	}
	if got := response["mode"]; got != "ok" {
		t.Fatalf("trusted default query parameter not applied: mode = %#v", got)
	}
	if got := response["number"]; got != json.Number("9007199254740993") {
		t.Fatalf("number precision lost: %#v", got)
	}
	if response["method"] != "POST" || response["path"] != "/acceptance/v1/echo" {
		t.Fatalf("trusted routing not applied: %#v", response)
	}
	if response["trustedHeader"] != "fixture-7" {
		t.Fatalf("trusted header not applied: %#v", response)
	}
	if response["itemCount"] != json.Number("2") && response["itemCount"] != float64(2) {
		t.Fatalf("nested array was not preserved: %#v", response["itemCount"])
	}

	// Valid remote ProblemDetails is a successful Provider result.
	problemResult, err := runtime.Invoke(context.Background(), "acceptance.generic.echo", map[string]any{
		"mode": "problem",
		"body": map[string]any{"label": "demo", "number": json.Number("1"), "items": []any{}},
	})
	if err != nil {
		t.Fatalf("problem invoke: %v", err)
	}
	if problemResult.HTTPStatus != http.StatusNotFound || problemResult.Problem == nil ||
		problemResult.Problem.Cause != "ACCEPTANCE_NOT_FOUND" {
		t.Fatalf("unexpected problem result: %#v", problemResult)
	}

	// 202 is preserved with its JSON body.
	accepted, err := runtime.Invoke(context.Background(), "acceptance.generic.echo", map[string]any{
		"mode": "202",
		"body": map[string]any{"label": "demo", "number": json.Number("1"), "items": []any{}},
	})
	if err != nil || accepted.HTTPStatus != http.StatusAccepted {
		t.Fatalf("202 result = %#v, err = %v", accepted, err)
	}

	// 204 is preserved with no projected response body.
	noContent, err := runtime.Invoke(context.Background(), "acceptance.generic.echo", map[string]any{
		"mode": "204",
		"body": map[string]any{"label": "demo", "number": json.Number("1"), "items": []any{}},
	})
	if err != nil || noContent.HTTPStatus != http.StatusNoContent || noContent.Response != nil {
		t.Fatalf("204 result = %#v, err = %v", noContent, err)
	}
}

func TestRuntimeGenericRejectsInvalidArgumentsBeforeIO(t *testing.T) {
	var hits atomic.Int64
	upstream := newFixtureUpstream(t, &hits)
	runtime := fixtureRuntime(t, upstream.URL)

	cases := map[string]map[string]any{
		"undeclared argument": {"body": map[string]any{"label": "x", "number": json.Number("1"), "items": []any{}}, "url": "http://evil"},
		"wrong nested type":   {"body": map[string]any{"label": "x", "number": "not-a-number", "items": []any{}}},
		"missing body field":  {"body": map[string]any{"label": "x"}},
		"obsolete field":      {"body": map[string]any{"label": "x", "number": json.Number("1"), "items": []any{}, "extra": true}},
	}
	before := hits.Load()
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.Invoke(context.Background(), "acceptance.generic.echo", arguments)
			if err == nil {
				t.Fatal("invalid nested arguments were accepted")
			}
			fault, ok := err.(*provider.Fault)
			if !ok || fault.Code != provider.CodeBackendArgumentsInvalid {
				t.Fatalf("error = %v, want BACKEND_ARGUMENTS_INVALID", err)
			}
		})
	}
	if got := hits.Load(); got != before {
		t.Fatalf("invalid arguments reached the upstream %d times", got-before)
	}
}

func TestRuntimeMutationGateBeforeIO(t *testing.T) {
	var hits atomic.Int64
	upstream := newFixtureUpstream(t, &hits)
	runtime := fixtureRuntime(t, upstream.URL)

	_, err := runtime.Invoke(context.Background(), "acceptance.generic.mutation", map[string]any{
		"requestId": "req-1",
		"body":      map[string]any{"value": "x"},
	})
	fault, ok := err.(*provider.Fault)
	if !ok || fault.Code != provider.CodeBackendOperationUnsupported {
		t.Fatalf("error = %v, want BACKEND_OPERATION_UNSUPPORTED", err)
	}
	if reason := fault.Detail["reason"]; reason != "MUTATION_GATED" {
		t.Fatalf("mutation fault reason = %#v, want MUTATION_GATED", reason)
	}
	if hits.Load() != 0 {
		t.Fatalf("gated Mutation reached the upstream %d times", hits.Load())
	}
}

func TestRuntimeGenericCancellationAndDeadline(t *testing.T) {
	var hits atomic.Int64
	upstream := newFixtureUpstream(t, &hits)
	runtime := fixtureRuntime(t, upstream.URL)

	args := map[string]any{
		"mode": "slow",
		"body": map[string]any{"label": "demo", "number": json.Number("1"), "items": []any{}},
	}
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := runtime.Invoke(deadlineCtx, "acceptance.generic.echo", args); err == nil {
		t.Fatal("deadline-exceeded generic invoke returned no error")
	}
	if deadlineCtx.Err() == nil {
		t.Fatal("invoke did not respect the invocation context deadline")
	}

	cancelCtx, cancelNow := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancelNow()
	}()
	if _, err := runtime.Invoke(cancelCtx, "acceptance.generic.echo", args); err == nil {
		t.Fatal("cancelled generic invoke returned no error")
	}
	if cancelCtx.Err() == nil {
		t.Fatal("invoke did not respect cancellation")
	}
}

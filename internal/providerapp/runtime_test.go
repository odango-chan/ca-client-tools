package providerapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	smfcontext "github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
	"github.com/odango-chan/ca-client-tools/internal/provider"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func lookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func newMASServer(t *testing.T) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-openamsm-query-token") != "secret-token" &&
			request.Header.Get("X-OpenAMSM-Query-Token") != "secret-token" {
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(writer, `{"status":403,"cause":"FORBIDDEN"}`)
			return
		}
		if !strings.HasPrefix(request.URL.Path, "/nsmf-") || !strings.HasSuffix(request.URL.Path, "/v1/query") {
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, `{"status":404,"cause":"UNKNOWN_QUERY_SERVICE"}`)
			return
		}
		var query struct {
			SUPI         string `json:"supi"`
			PDUSessionID int    `json:"pduSessionId"`
		}
		_ = json.NewDecoder(request.Body).Decode(&query)
		if query.PDUSessionID == 99 {
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(writer, `{"status":404,"cause":"SESSION_NOT_FOUND"}`)
			return
		}
		masID := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/nsmf-"), "-context/v1/query"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer,
			`{"masId":%q,"status":"AVAILABLE","key":{"supi":%q,"pduSessionId":%d},"context":{"lifecycle":"OPERATIONAL"}}`,
			masID, query.SUPI, query.PDUSessionID)
	})
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func newMetricsServer(t *testing.T, paths ...string) *httptest.Server {
	t.Helper()
	allowed := make(map[string]bool, len(paths))
	for _, path := range paths {
		allowed[path] = true
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !allowed[request.URL.Path] {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"items":[{"id":"one"}],"pager":{"page":0,"page_size":100,"count":1}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func requireRuntime(t *testing.T, protocolVersion string, env map[string]string) provider.Runtime {
	t.Helper()
	runtime, err := newRuntime(protocolVersion, lookup(env))
	if err != nil {
		t.Fatalf("newRuntime(%s): %v", protocolVersion, err)
	}
	t.Cleanup(runtime.Close)
	return runtime
}

func isConfigError(err error) bool {
	_, ok := err.(*provider.ConfigError)
	return ok
}

func TestV1RuntimeRequiresSMFConfiguration(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"no env":         {},
		"origin only":    {EnvSMFSBIOrigin: "http://127.0.0.1:1"},
		"token only":     {EnvQueryToken: "secret-token"},
		"invalid origin": {EnvSMFSBIOrigin: "https://127.0.0.1:1", EnvQueryToken: "secret-token"},
		"invalid token":  {EnvSMFSBIOrigin: "http://127.0.0.1:1", EnvQueryToken: "bad\ntoken"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newRuntime(provider.ProtocolV1, lookup(env))
			if err == nil || !isConfigError(err) {
				t.Fatalf("error = %v, want *provider.ConfigError", err)
			}
		})
	}
}

func TestV1RuntimeCatalogAndInvoke(t *testing.T) {
	server := newMASServer(t)
	runtime := requireRuntime(t, provider.ProtocolV1, map[string]string{
		EnvSMFSBIOrigin: server.URL,
		EnvQueryToken:   "secret-token",
	})
	if runtime.CatalogVersion() != "" {
		t.Fatalf("v1 catalog version = %q, want empty (v1 artifact has no catalogVersion)", runtime.CatalogVersion())
	}
	if ids := runtime.BindingIDs(); len(ids) != 8 {
		t.Fatalf("binding ids = %d, want 8", len(ids))
	}
	if _, ok := runtime.Binding("openamsm.amf.ue-info.list"); ok {
		t.Fatal("v1 runtime must not expose the AMF list binding")
	}
	result, err := runtime.Invoke(context.Background(), "openamsm.smf.mas.sc.query", map[string]any{
		"supi": "imsi-001010000000001", "pduSessionId": int64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	typed, ok := result.Response.(*smfcontext.Result)
	if !ok || result.HTTPStatus != 200 || typed.Status != "AVAILABLE" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestV1RuntimeMapsProblemDetails(t *testing.T) {
	server := newMASServer(t)
	runtime := requireRuntime(t, provider.ProtocolV1, map[string]string{
		EnvSMFSBIOrigin: server.URL,
		EnvQueryToken:   "secret-token",
	})
	result, err := runtime.Invoke(context.Background(), "openamsm.smf.mas.sc.query", map[string]any{
		"supi": "imsi-001010000000001", "pduSessionId": int64(99),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.HTTPStatus != http.StatusNotFound || result.Problem == nil ||
		result.Problem.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected problem result: %#v", result)
	}
}

func TestV2RuntimeAvailability(t *testing.T) {
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	runtime := requireRuntime(t, provider.ProtocolV2, map[string]string{
		EnvAMFMetricsOrigin: amf.URL,
		EnvSMFMetricsOrigin: smf.URL,
	})
	if runtime.CatalogVersion() != "2" {
		t.Fatalf("catalog version = %q", runtime.CatalogVersion())
	}
	if ids := runtime.BindingIDs(); len(ids) != 11 {
		t.Fatalf("binding ids = %d, want 11", len(ids))
	}
	masBinding, ok := runtime.Binding("openamsm.smf.mas.sc.query")
	if !ok || masBinding.Available || masBinding.UnavailableReason != "NOT_CONFIGURED" {
		t.Fatalf("unexpected MAS binding: %#v", masBinding)
	}
	amfBinding, ok := runtime.Binding("openamsm.amf.ue-info.list")
	if !ok || !amfBinding.Available {
		t.Fatalf("unexpected AMF binding: %#v", amfBinding)
	}

	result, err := runtime.Invoke(context.Background(), "openamsm.amf.ue-info.list", map[string]any{
		"page": int64(0), "pageSize": int64(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.HTTPStatus != 200 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := runtime.Invoke(context.Background(), "openamsm.smf.mas.sc.query", map[string]any{
		"supi": "imsi-001010000000001", "pduSessionId": int64(5),
	}); err == nil {
		t.Fatal("expected NOT_CONFIGURED fault for the unconfigured MAS binding")
	}
}

func TestV2RuntimeConfigurationErrors(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"no service":      {},
		"partial sbi":     {EnvSMFSBIOrigin: "http://127.0.0.1:1"},
		"invalid metrics": {EnvAMFMetricsOrigin: "not-a-url"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newRuntime(provider.ProtocolV2, lookup(env)); err == nil || !isConfigError(err) {
				t.Fatalf("error = %v, want *provider.ConfigError", err)
			}
		})
	}
}

func TestV2RuntimeAllToolsConfigured(t *testing.T) {
	mas := newMASServer(t)
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	runtime := requireRuntime(t, provider.ProtocolV2, map[string]string{
		EnvSMFSBIOrigin:     mas.URL,
		EnvQueryToken:       "secret-token",
		EnvAMFMetricsOrigin: amf.URL,
		EnvSMFMetricsOrigin: smf.URL,
	})
	for _, id := range runtime.BindingIDs() {
		binding, ok := runtime.Binding(id)
		if !ok || !binding.Available {
			t.Fatalf("binding %s is not available: %#v", id, binding)
		}
	}
	contextResult, err := runtime.Invoke(context.Background(), "openamsm.smf.mas.pa.query", map[string]any{
		"supi": "imsi-001010000000001", "pduSessionId": int64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if contextResult.HTTPStatus != 200 {
		t.Fatalf("unexpected context result: %#v", contextResult)
	}
	for _, id := range []string{"openamsm.amf.ue-info.list", "openamsm.amf.gnb-info.list", "openamsm.smf.pdu-info.list"} {
		result, err := runtime.Invoke(context.Background(), id, map[string]any{"page": int64(0), "pageSize": int64(100)})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if result.HTTPStatus != 200 {
			t.Fatalf("%s: %#v", id, result)
		}
	}
}

func TestProviderServeIntegrationV2(t *testing.T) {
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	env := map[string]string{
		EnvAMFMetricsOrigin: amf.URL,
		EnvSMFMetricsOrigin: smf.URL,
	}
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- provider.Serve(context.Background(), provider.Options{
			In:  inputReader,
			Out: outputWriter,
			Err: io.Discard,
			Info: provider.ProviderInfo{
				ProviderID:      provider.ProviderID,
				ProviderVersion: ProviderVersion,
				BuildDigest:     BuildDigest(),
			},
			NewRuntime: func(protocolVersion string) (provider.Runtime, error) {
				return newRuntime(protocolVersion, lookup(env))
			},
		})
		_ = outputWriter.Close()
	}()
	t.Cleanup(func() {
		_ = inputWriter.Close()
		_ = inputReader.Close()
		_ = outputReader.Close()
	})

	frames := []map[string]any{
		{
			"protocolVersion": provider.ProtocolV2, "type": "request", "requestId": "init",
			"method": "initialize",
			"params": map[string]any{
				"hostInstanceId": "host", "providerInstanceId": "ca-client-tools.sbi",
				"limits": map[string]any{"maxFrameBytes": 1 << 20, "maxInFlight": 4, "maxCallDurationMs": 2000},
			},
		},
		{
			"protocolVersion": provider.ProtocolV2, "type": "request", "requestId": "catalog",
			"method": "catalog",
			"params": map[string]any{"backendOperationIds": []string{"openamsm.amf.ue-info.list"}},
		},
		{
			"protocolVersion": provider.ProtocolV2, "type": "request", "requestId": "invoke",
			"method": "invoke",
			"params": map[string]any{
				"backendOperationId": "openamsm.amf.ue-info.list",
				"arguments":          map[string]any{"page": 0, "pageSize": 100},
				"invocation": map[string]any{
					"invocationId": "inv", "producerInvocationId": "producer",
					"deadline": time.Now().Add(5 * time.Second).UTC().Format(time.RFC3339),
				},
			},
		},
	}
	go func() {
		encoder := json.NewEncoder(inputWriter)
		for _, frame := range frames {
			if err := encoder.Encode(frame); err != nil {
				break
			}
		}
		_ = inputWriter.Close()
	}()

	decoder := json.NewDecoder(outputReader)
	for _, want := range []string{"init", "catalog", "invoke"} {
		var response map[string]any
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response["requestId"] != want || response["ok"] != true {
			t.Fatalf("unexpected response: %#v", response)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("provider serve: %v", err)
	}
}

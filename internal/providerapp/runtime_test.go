package providerapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// writeServiceConfig writes a trusted serviceconfig v1 file and returns its path.
func writeServiceConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "services.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireRuntime(t *testing.T, env map[string]string) provider.Runtime {
	t.Helper()
	runtime, err := newRuntime(lookup(env))
	if err != nil {
		t.Fatalf("newRuntime: %v", err)
	}
	t.Cleanup(runtime.Close)
	return runtime
}

func isConfigError(err error) bool {
	_, ok := err.(*provider.ConfigError)
	return ok
}

func TestRuntimeRequiresTrustedConfiguration(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"no env":          {},
		"origin only":     {EnvSMFSBIOrigin: "http://127.0.0.1:1"},
		"token only":      {EnvQueryToken: "secret-token"},
		"invalid origin":  {EnvSMFSBIOrigin: "https://127.0.0.1:1", EnvQueryToken: "secret-token"},
		"invalid token":   {EnvSMFSBIOrigin: "http://127.0.0.1:1", EnvQueryToken: "bad\ntoken"},
		"invalid metrics": {EnvAMFMetricsOrigin: "not-a-url"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newRuntime(lookup(env))
			if err == nil || !isConfigError(err) {
				t.Fatalf("error = %v, want *provider.ConfigError", err)
			}
		})
	}
}

func TestRuntimeTypedCatalogAndInvoke(t *testing.T) {
	server := newMASServer(t)
	runtime := requireRuntime(t, map[string]string{
		EnvSMFSBIOrigin: server.URL,
		EnvQueryToken:   "secret-token",
	})
	if runtime.CatalogVersion() != "2" {
		t.Fatalf("catalog version = %q, want 2", runtime.CatalogVersion())
	}
	if ids := runtime.BindingIDs(); len(ids) < 11 {
		t.Fatalf("binding ids = %d, want at least the 11 production bindings", len(ids))
	}
	if !strings.HasPrefix(runtime.CatalogDigest(), "sha256:") {
		t.Fatalf("catalog digest = %q", runtime.CatalogDigest())
	}
	amfBinding, ok := runtime.Binding("openamsm.amf.ue-info.list")
	if !ok || amfBinding.Available || amfBinding.UnavailableReason != "NOT_CONFIGURED" {
		t.Fatalf("unexpected AMF binding: %#v", amfBinding)
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

	problem, err := runtime.Invoke(context.Background(), "openamsm.smf.mas.sc.query", map[string]any{
		"supi": "imsi-001010000000001", "pduSessionId": int64(99),
	})
	if err != nil {
		t.Fatal(err)
	}
	if problem.HTTPStatus != http.StatusNotFound || problem.Problem == nil ||
		problem.Problem.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected problem result: %#v", problem)
	}
}

func TestRuntimeMetricsAvailability(t *testing.T) {
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	runtime := requireRuntime(t, map[string]string{
		EnvAMFMetricsOrigin: amf.URL,
		EnvSMFMetricsOrigin: smf.URL,
	})
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

func TestRuntimeAllToolsConfigured(t *testing.T) {
	mas := newMASServer(t)
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	runtime := requireRuntime(t, map[string]string{
		EnvSMFSBIOrigin:     mas.URL,
		EnvQueryToken:       "secret-token",
		EnvAMFMetricsOrigin: amf.URL,
		EnvSMFMetricsOrigin: smf.URL,
	})
	for _, id := range runtime.BindingIDs() {
		if strings.HasPrefix(id, "acceptance.") {
			// Acceptance-only fixture bindings exist only in the providerfixture
			// build and are NOT_CONFIGURED in this env.
			continue
		}
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

// TestRuntimeTrustedServiceConfig covers the optional trusted multi-service
// configuration used to resolve genericSBI service keys.
func TestRuntimeTrustedServiceConfig(t *testing.T) {
	mas := newMASServer(t)
	configPath := writeServiceConfig(t, `version: 1
services:
  extra-sbi-service:
    nfType: SMF
    origin: http://127.0.0.1:1
    protocol: h2c
`)
	runtime := requireRuntime(t, map[string]string{
		EnvSMFSBIOrigin:  mas.URL,
		EnvQueryToken:    "secret-token",
		EnvServiceConfig: configPath,
	})
	if ids := runtime.BindingIDs(); len(ids) < 11 {
		t.Fatalf("binding ids = %d, want at least the 11 production bindings", len(ids))
	}

	// Conflicting duplicate service keys are rejected.
	duplicate := writeServiceConfig(t, `version: 1
services:
  openamsm-smf-context:
    origin: http://127.0.0.1:2
    protocol: h2c
`)
	if _, err := newRuntime(lookup(map[string]string{
		EnvSMFSBIOrigin:  mas.URL,
		EnvQueryToken:    "secret-token",
		EnvServiceConfig: duplicate,
	})); err == nil || !isConfigError(err) {
		t.Fatalf("duplicate service key error = %v, want *provider.ConfigError", err)
	}

	// A malformed or empty trusted service file fails closed.
	for name, content := range map[string]string{
		"missing file": "",
		"empty services": `version: 1
services: {}
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "services.yaml")
			if content != "" {
				path = writeServiceConfig(t, content)
			}
			if _, err := newRuntime(lookup(map[string]string{
				EnvSMFSBIOrigin:  mas.URL,
				EnvQueryToken:    "secret-token",
				EnvServiceConfig: path,
			})); err == nil || !isConfigError(err) {
				t.Fatalf("error = %v, want *provider.ConfigError", err)
			}
		})
	}
}

func TestProviderServeIntegration(t *testing.T) {
	mas := newMASServer(t)
	amf := newMetricsServer(t, "/ue-info", "/gnb-info")
	smf := newMetricsServer(t, "/pdu-info")
	env := map[string]string{
		EnvSMFSBIOrigin:     mas.URL,
		EnvQueryToken:       "secret-token",
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
			NewRuntime: func() (provider.Runtime, error) {
				return newRuntime(lookup(env))
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
			"protocolVersion": provider.ProtocolVersion, "type": "request", "requestId": "init",
			"method": "initialize",
			"params": map[string]any{
				"hostInstanceId": "host", "providerInstanceId": "ca-client-tools.sbi",
				"limits": map[string]any{"maxFrameBytes": 1 << 20, "maxInFlight": 4, "maxCallDurationMs": 2000},
			},
		},
		{
			"protocolVersion": provider.ProtocolVersion, "type": "request", "requestId": "catalog",
			"method": "catalog",
			"params": map[string]any{"backendOperationIds": []string{"openamsm.amf.ue-info.list"}},
		},
		{
			"protocolVersion": provider.ProtocolVersion, "type": "request", "requestId": "invoke",
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

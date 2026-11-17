package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

const testServiceKey = "test-service"

// newH2CTestServer starts a cleartext HTTP/2 test server. http.Protocols
// unencrypted HTTP/2 is the same native h2c prior-knowledge path used by the
// shared transport tests; no TLS and no loopback-external network is involved.
func newH2CTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func testSbiConfig(server *httptest.Server) sbi.Config {
	return sbi.Config{
		Services: map[string]sbi.ServiceConfig{
			testServiceKey: {Origin: server.URL, APIRoot: "/api/v1"},
		},
		Limits: sbi.Limits{
			RequestTimeout:   2 * time.Second,
			DialTimeout:      time.Second,
			MaxRequestBytes:  1 << 20,
			MaxResponseBytes: 1 << 20,
		},
	}
}

func newTestClient(t *testing.T, server *httptest.Server, mutate func(*sbi.Config)) *sbi.Client {
	t.Helper()
	config := testSbiConfig(server)
	if mutate != nil {
		mutate(&config)
	}
	client, err := sbi.NewClient(config)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func newTestExecutor(t *testing.T, client *sbi.Client, resolver ArgumentResolver, limits payload.Limits) *Executor {
	t.Helper()
	executor, err := New(client, resolver, limits)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return executor
}

func mustSchema(t *testing.T, raw string) *schemacheck.Schema {
	t.Helper()
	schema, err := schemacheck.Compile([]byte(raw))
	if err != nil {
		t.Fatalf("Compile schema: %v", err)
	}
	return schema
}

func int64Pointer(value int64) *int64 { return &value }
func intPointer(value int) *int       { return &value }

type fakeResolver struct {
	files map[string][]byte
	opens atomic.Int32
}

func newFakeResolver(files map[string][]byte) *fakeResolver {
	return &fakeResolver{files: files}
}

func (r *fakeResolver) Open(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.opens.Add(1)
	data, ok := r.files[name]
	if !ok {
		return nil, 0, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func readOperation(t *testing.T) Operation {
	t.Helper()
	return Operation{
		OperationID:  "Test_QueryUEContext",
		Effect:       EffectRead,
		ServiceKey:   testServiceKey,
		Method:       http.MethodGet,
		PathTemplate: "/ue-contexts/{ueContextId}",
		PathParameters: []Parameter{
			{Name: "ueContextId", Type: "string", Required: true, Pattern: `^imsi-[0-9]{1,15}(:[0-9]+)?$`},
		},
		QueryParameters: []Parameter{
			{Name: "trace", Type: "string", Required: false, MaxLength: intPointer(32)},
			{Name: "page", Type: "integer", Required: false, Minimum: int64Pointer(0), Maximum: int64Pointer(99)},
		},
		HeaderBindings:     []HeaderBinding{{Name: "X-Trusted-Request", Value: "trusted-value"}},
		ResponseMediaTypes: []string{"application/json"},
		ResponseSchema:     mustSchema(t, `{"type":"object","required":["status"],"properties":{"status":{"type":"string"}}}`),
	}
}

func jsonPostOperation(t *testing.T) Operation {
	t.Helper()
	return Operation{
		OperationID:        "Test_CreateContext",
		Effect:             EffectMutation,
		ServiceKey:         testServiceKey,
		Method:             http.MethodPost,
		PathTemplate:       "/ue-contexts",
		Body:               &BodySpec{Kind: PayloadJSON, MediaType: "application/json", Schema: mustSchema(t, `{"type":"object","required":["supi"],"properties":{"supi":{"type":"string"}}}`)},
		ResponseMediaTypes: []string{"application/json"},
		ResponseSchema:     mustSchema(t, `{"type":"object","required":["cause"],"properties":{"cause":{"type":"string"}}}`),
		Idempotency:        &Idempotency{Mode: "NONE"},
	}
}

package sbi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func testConfig(server *httptest.Server) Config {
	return Config{
		Services: map[string]ServiceConfig{
			"smf-mas-query": {
				Origin:             server.URL,
				APIRoot:            "/nsmf-sc-context/v1",
				HeaderTokenProfile: "openamsm-query",
			},
		},
		HeaderTokenProfiles: map[string]HeaderTokenProfile{
			"openamsm-query": {
				Header: "X-OpenAMSM-Query-Token",
				Token:  "private-query-token",
			},
		},
		Limits: Limits{
			RequestTimeout:   3 * time.Second,
			DialTimeout:      time.Second,
			MaxRequestBytes:  1024,
			MaxResponseBytes: 1024,
		},
	}
}

func TestDoUsesH2CAndPreservesResponse(t *testing.T) {
	type observed struct {
		protocol int
		path     string
		query    string
		token    string
		body     string
	}
	observations := make(chan observed, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		observations <- observed{
			protocol: request.ProtoMajor,
			path:     request.URL.Path,
			query:    request.URL.RawQuery,
			token:    request.Header.Get("X-OpenAMSM-Query-Token"),
			body:     string(body),
		}
		writer.Header().Set("3gpp-Sbi-Message-Priority", "PRIORITY_1")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(writer, "{\"cause\":\"accepted\"}")
	}))
	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "smf-mas-query", Request{
		Method:        http.MethodPost,
		Path:          "/query",
		Query:         url.Values{"trace": {"one two"}},
		ContentType:   "application/json",
		Body:          strings.NewReader("{\"supi\":\"imsi-001010000000001\",\"pduSessionId\":5}"),
		ContentLength: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 || response.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected response: protocol=%s status=%d", response.Proto, response.StatusCode)
	}
	if response.Header.Get("3gpp-Sbi-Message-Priority") != "PRIORITY_1" {
		t.Fatal("response header was not preserved")
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{\"cause\":\"accepted\"}" {
		t.Fatalf("unexpected response body: %s", raw)
	}
	got := <-observations
	if got.protocol != 2 || got.path != "/nsmf-sc-context/v1/query" ||
		got.query != "trace=one+two" || got.token != "private-query-token" ||
		got.body != "{\"supi\":\"imsi-001010000000001\",\"pduSessionId\":5}" {
		t.Fatalf("unexpected request: %+v", got)
	}
}

func TestDoReusesH2CConnection(t *testing.T) {
	var newConnections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	for range 2 {
		response, err := client.Do(context.Background(), "smf-mas-query", Request{
			Method: http.MethodGet,
			Path:   "/query",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if got := newConnections.Load(); got != 1 {
		t.Fatalf("expected one reused h2c connection, got %d", got)
	}
}

func TestDoRejectsHTTP1OnlyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "smf-mas-query", Request{
		Method: http.MethodGet,
		Path:   "/query",
	})
	if err == nil {
		if response != nil {
			response.Body.Close()
		}
		t.Fatal("expected HTTP/1-only endpoint to be rejected")
	}
}

func TestDoEnforcesResponseLimit(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "12345")
	}))
	config := testConfig(server)
	config.Limits.MaxResponseBytes = 4
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "smf-mas-query", Request{
		Method: http.MethodGet,
		Path:   "/query",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected response size error, got %v", err)
	}
}

func TestDoHonorsCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
	}))
	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, "smf-mas-query", Request{Method: http.MethodGet, Path: "/wait"})
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("request did not reach server")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not stop after cancellation")
	}
}

func TestDoDoesNotFollowRedirects(t *testing.T) {
	var targetCalls atomic.Int32
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// The configured API root is a prefix of the effective path, so match
		// the operation suffix rather than the absolute path.
		if strings.HasSuffix(request.URL.Path, "/redirect") {
			writer.Header().Set("Location", "target")
			writer.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/target") {
			targetCalls.Add(1)
		}
	}))
	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "smf-mas-query", Request{
		Method: http.MethodGet,
		Path:   "/redirect",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || targetCalls.Load() != 0 {
		t.Fatalf("redirect was followed or response changed: status=%d targetCalls=%d", response.StatusCode, targetCalls.Load())
	}
}

func TestDoRejectsPathTraversalAndCallerTokenOverride(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	client, err := NewClient(testConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "smf-mas-query", Request{
		Method: http.MethodGet,
		Path:   "/../admin",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected path traversal rejection, got %v", err)
	}
	_, err = client.Do(context.Background(), "smf-mas-query", Request{
		Method:  http.MethodGet,
		Path:    "/query",
		Headers: http.Header{"X-OpenAMSM-Query-Token": {"attacker"}},
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected caller token override rejection, got %v", err)
	}
}

func TestDoRejectsRequestBodyOverLimit(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		_, _ = io.WriteString(writer, "{}")
	}))
	config := testConfig(server)
	config.Limits.MaxRequestBytes = 4
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "smf-mas-query", Request{
		Method: http.MethodPost,
		Path:   "/query",
		Body:   strings.NewReader("12345"),
	})
	if err == nil {
		t.Fatal("expected request size error")
	}
	if !errors.Is(err, ErrRequestTooLarge) && !strings.Contains(err.Error(), ErrRequestTooLarge.Error()) {
		t.Fatalf("expected request size error, got %v", err)
	}
}

func TestNewClientRejectsHTTPSOrigin(t *testing.T) {
	_, err := NewClient(Config{
		Services: map[string]ServiceConfig{
			"smf-mas-query": {Origin: "https://smf.example"},
		},
	})
	if err == nil {
		t.Fatal("expected HTTPS origin to be rejected because this client is h2c-only")
	}
}

func TestNewClientRejectsUnknownHeaderTokenProfile(t *testing.T) {
	_, err := NewClient(Config{
		Services: map[string]ServiceConfig{
			"smf-mas-query": {
				Origin:             "http://smf.example:7777",
				HeaderTokenProfile: "missing",
			},
		},
	})
	if err == nil {
		t.Fatal("expected unknown header-token profile to be rejected")
	}
}

func TestResolveEndpointKeepsEscapedPathAndEncodesQuery(t *testing.T) {
	origin, err := url.Parse("http://smf.example:7777")
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := resolveEndpoint(origin, "/nsmf-sc-context/v1", "/ue-contexts/imsi-001%3A01", url.Values{"x": {"a b"}})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.EscapedPath() != "/nsmf-sc-context/v1/ue-contexts/imsi-001%3A01" ||
		endpoint.RawQuery != "x=a+b" {
		t.Fatalf("unexpected endpoint: %s", endpoint.String())
	}
}

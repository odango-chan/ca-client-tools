package sbi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testTLSProfile(t *testing.T, server *httptest.Server) TLSProfile {
	t.Helper()
	certificate := server.Certificate()
	path := filepath.Join(t.TempDir(), "server-ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return TLSProfile{CAFile: path}
}

func newHTTP2TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func testConfig(server *httptest.Server, profile TLSProfile) Config {
	return Config{
		Services: map[string]ServiceConfig{
			"namf-comm": {
				Origin:     server.URL,
				APIRoot:    "/namf-comm/v1",
				TLSProfile: "test",
			},
		},
		TLSProfiles: map[string]TLSProfile{"test": profile},
		Limits: Limits{
			RequestTimeout:   3 * time.Second,
			MaxRequestBytes:  1024,
			MaxResponseBytes: 1024,
		},
	}
}

func TestDoUsesHTTP2AndPreservesResponse(t *testing.T) {
	type observed struct {
		protocol int
		path     string
		query    string
		token    string
		body     string
	}
	observations := make(chan observed, 1)
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	profile := testTLSProfile(t, server)
	config := testConfig(server, profile)
	config.HeaderTokenProfiles = map[string]HeaderTokenProfile{
		"query-token": {Header: "X-OpenAMSM-Query-Token", Token: "private-token"},
	}
	service := config.Services["namf-comm"]
	service.AuthProfile = "query-token"
	config.Services["namf-comm"] = service

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "namf-comm", Request{
		Method:        http.MethodPost,
		Path:          "/n1-n2-message-transfer",
		Query:         url.Values{"trace": {"one two"}},
		ContentType:   "application/json",
		Body:          strings.NewReader("{\"ueContextId\":\"ue-1\"}"),
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
	if got.protocol != 2 || got.path != "/namf-comm/v1/n1-n2-message-transfer" ||
		got.query != "trace=one+two" || got.token != "private-token" ||
		got.body != "{\"ueContextId\":\"ue-1\"}" {
		t.Fatalf("unexpected request: %+v", got)
	}
}

func TestDoRejectsHTTP1Downgrade(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	t.Cleanup(server.Close)
	profile := testTLSProfile(t, server)
	client, err := NewClient(testConfig(server, profile))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "namf-comm", Request{
		Method: http.MethodGet,
		Path:   "/resource",
	})
	if !errors.Is(err, ErrHTTP2Required) {
		t.Fatalf("expected HTTP/2 requirement error, got %v", err)
	}
}

func TestDoReusesHTTP2Connection(t *testing.T) {
	var newConnections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	server.EnableHTTP2 = true
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	profile := testTLSProfile(t, server)
	client, err := NewClient(testConfig(server, profile))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	for index := 0; index < 2; index++ {
		response, err := client.Do(context.Background(), "namf-comm", Request{
			Method: http.MethodGet,
			Path:   "/resource",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if got := newConnections.Load(); got != 1 {
		t.Fatalf("expected one reused HTTP/2 connection, got %d", got)
	}
}

func TestOAuthClientCredentialsTokenIsCached(t *testing.T) {
	var tokenRequests atomic.Int32
	var serviceRequests atomic.Int32
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			tokenRequests.Add(1)
			username, password, ok := request.BasicAuth()
			if !ok || username != "client" || password != "secret" {
				http.Error(writer, "bad client authentication", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(writer, "{\"access_token\":\"token-value\",\"token_type\":\"Bearer\",\"expires_in\":3600}")
		case "/namf-comm/v1/resource":
			serviceRequests.Add(1)
			if request.Header.Get("Authorization") != "Bearer token-value" {
				http.Error(writer, "missing bearer token", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(writer, "{}")
		default:
			http.NotFound(writer, request)
		}
	}))
	profile := testTLSProfile(t, server)
	config := testConfig(server, profile)
	config.OAuthProfiles = map[string]OAuthProfile{
		"namf-oauth": {
			TokenURL:     server.URL + "/token",
			ClientID:     "client",
			ClientSecret: "secret",
			Scopes:       []string{"namf-comm"},
			TLSProfile:   "test",
		},
	}
	service := config.Services["namf-comm"]
	service.AuthProfile = "namf-oauth"
	config.Services["namf-comm"] = service
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	for index := 0; index < 2; index++ {
		response, err := client.Do(context.Background(), "namf-comm", Request{
			Method: http.MethodGet,
			Path:   "/resource",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if tokenRequests.Load() != 1 || serviceRequests.Load() != 2 {
		t.Fatalf("unexpected request counts: token=%d service=%d", tokenRequests.Load(), serviceRequests.Load())
	}
}

func TestDoEnforcesResponseLimit(t *testing.T) {
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "12345")
	}))
	profile := testTLSProfile(t, server)
	config := testConfig(server, profile)
	config.Limits.MaxResponseBytes = 4
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "namf-comm", Request{
		Method: http.MethodGet,
		Path:   "/resource",
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
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
	}))
	profile := testTLSProfile(t, server)
	client, err := NewClient(testConfig(server, profile))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, "namf-comm", Request{Method: http.MethodGet, Path: "/wait"})
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
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			writer.Header().Set("Location", "/target")
			writer.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		if request.URL.Path == "/target" {
			targetCalls.Add(1)
		}
	}))
	profile := testTLSProfile(t, server)
	client, err := NewClient(testConfig(server, profile))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	response, err := client.Do(context.Background(), "namf-comm", Request{
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

func TestDoRejectsPathTraversalAndCallerAuthOverride(t *testing.T) {
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{}")
	}))
	profile := testTLSProfile(t, server)
	client, err := NewClient(testConfig(server, profile))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "namf-comm", Request{
		Method: http.MethodGet,
		Path:   "/../admin",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected path traversal rejection, got %v", err)
	}
	_, err = client.Do(context.Background(), "namf-comm", Request{
		Method:  http.MethodGet,
		Path:    "/resource",
		Headers: http.Header{"Authorization": {"Bearer attacker"}},
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected caller auth override rejection, got %v", err)
	}
}

func TestDoRejectsRequestBodyOverLimit(t *testing.T) {
	server := newHTTP2TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		_, _ = io.WriteString(writer, "{}")
	}))
	profile := testTLSProfile(t, server)
	config := testConfig(server, profile)
	config.Limits.MaxRequestBytes = 4
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "namf-comm", Request{
		Method: http.MethodPost,
		Path:   "/resource",
		Body:   strings.NewReader("12345"),
	})
	if err == nil {
		t.Fatal("expected request size error")
	}
	if !errors.Is(err, ErrRequestTooLarge) && !strings.Contains(err.Error(), ErrRequestTooLarge.Error()) {
		t.Fatalf("expected request size error, got %v", err)
	}
}

func TestNewClientRejectsHTTPServiceOrigin(t *testing.T) {
	_, err := NewClient(Config{
		Services: map[string]ServiceConfig{
			"namf-comm": {Origin: "http://amf.example"},
		},
	})
	if err == nil {
		t.Fatal("expected non-TLS SBI service origin to be rejected")
	}
}

func TestNewClientRejectsInvalidOAuthProfile(t *testing.T) {
	_, err := NewClient(Config{
		Services: map[string]ServiceConfig{
			"namf-comm": {Origin: "https://amf.example", AuthProfile: "bad"},
		},
		OAuthProfiles: map[string]OAuthProfile{
			"bad": {TokenURL: "https://nrf.example/token", ClientID: "client"},
		},
	})
	if err == nil {
		t.Fatal("expected incomplete OAuth profile to be rejected")
	}
}

func TestResolveEndpointKeepsEscapedPathAndEncodesQuery(t *testing.T) {
	origin, err := url.Parse("https://amf.example")
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := resolveEndpoint(origin, "/namf-comm/v1", "/ue-contexts/imsi-001%3A01", url.Values{"x": {"a b"}})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.EscapedPath() != "/namf-comm/v1/ue-contexts/imsi-001%3A01" ||
		endpoint.RawQuery != "x=a+b" {
		t.Fatalf("unexpected endpoint: %s", endpoint.String())
	}
}

func TestOAuthRejectsHTTP1TokenEndpoint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "{\"access_token\":\"token\",\"token_type\":\"Bearer\",\"expires_in\":60}")
	}))
	t.Cleanup(server.Close)
	profile := testTLSProfile(t, server)
	config := testConfig(server, profile)
	config.OAuthProfiles = map[string]OAuthProfile{
		"oauth": {
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
			TLSProfile:   "test",
		},
	}
	service := config.Services["namf-comm"]
	service.AuthProfile = "oauth"
	config.Services["namf-comm"] = service
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	_, err = client.Do(context.Background(), "namf-comm", Request{Method: http.MethodGet, Path: "/resource"})
	if !errors.Is(err, errAuthentication) {
		t.Fatalf("expected OAuth HTTP/2 failure, got %v", err)
	}
}

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

func TestExecuteGETEncodesPathAndQuery(t *testing.T) {
	type observed struct {
		protocol      int
		escapedPath   string
		rawQuery      string
		trustedHeader string
	}
	observations := make(chan observed, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observations <- observed{
			protocol:      request.ProtoMajor,
			escapedPath:   request.URL.EscapedPath(),
			rawQuery:      request.URL.RawQuery,
			trustedHeader: request.Header.Get("X-Trusted-Request"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	result, err := executor.Execute(context.Background(), readOperation(t), map[string]any{
		"ueContextId": "imsi-001010000000001:7",
		"trace":       "a b",
		"page":        json.Number("3"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected status %d", result.HTTPStatus)
	}
	object, ok := result.JSON.(map[string]any)
	if !ok || object["status"] != "OK" {
		t.Fatalf("unexpected decoded JSON: %#v", result.JSON)
	}
	got := <-observations
	if got.protocol != 2 {
		t.Fatalf("expected HTTP/2, got %d", got.protocol)
	}
	if got.escapedPath != "/api/v1/ue-contexts/imsi-001010000000001:7" {
		t.Fatalf("unexpected escaped path %q", got.escapedPath)
	}
	if got.rawQuery != "page=3&trace=a+b" {
		t.Fatalf("unexpected query %q", got.rawQuery)
	}
	if got.trustedHeader != "trusted-value" {
		t.Fatalf("trusted header not sent: %q", got.trustedHeader)
	}
}

func TestExecuteRejectsInvalidArgumentsAndConfiguration(t *testing.T) {
	var calls atomic.Int32
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	reservedHeader := readOperation(t)
	reservedHeader.HeaderBindings = []HeaderBinding{{Name: "Authorization", Value: "Bearer x"}}
	injectedHeader := readOperation(t)
	injectedHeader.HeaderBindings = []HeaderBinding{{Name: "X-Test", Value: "a\r\nInjected: b"}}
	badMethod := readOperation(t)
	badMethod.Method = "TRACE"
	relativePath := readOperation(t)
	relativePath.PathTemplate = "ue-contexts/{ueContextId}"
	undeclaredToken := readOperation(t)
	undeclaredToken.PathTemplate = "/ue-contexts/{ueContextId}/{other}"
	unknownToken := readOperation(t)
	unknownToken.PathParameters = append(unknownToken.PathParameters, Parameter{Name: "unused", Type: "string"})
	queryAsPath := readOperation(t)
	queryAsPath.PathParameters = []Parameter{{Name: "ueContextId", Type: "integer"}}

	inputCases := []struct {
		name string
		args map[string]any
	}{
		{"unknown argument", map[string]any{"ueContextId": "imsi-1", "method": "DELETE"}},
		{"caller header injection", map[string]any{"ueContextId": "imsi-1", "headers": map[string]any{"Authorization": "x"}}},
		{"path slash", map[string]any{"ueContextId": "a/b"}},
		{"path traversal", map[string]any{"ueContextId": ".."}},
		{"path question mark", map[string]any{"ueContextId": "a?b"}},
		{"path hash", map[string]any{"ueContextId": "a#b"}},
		{"path backslash", map[string]any{"ueContextId": `a\b`}},
		{"path pattern", map[string]any{"ueContextId": "ue-1"}},
		{"path control", map[string]any{"ueContextId": "a\x01b"}},
		{"query decimal integer", map[string]any{"ueContextId": "imsi-1", "page": json.Number("1.5")}},
		{"query string for integer", map[string]any{"ueContextId": "imsi-1", "page": "1"}},
		{"query above maximum", map[string]any{"ueContextId": "imsi-1", "page": json.Number("100")}},
		{"query too long", map[string]any{"ueContextId": "imsi-1", "trace": strings.Repeat("x", 33)}},
	}
	for _, testCase := range inputCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := executor.Execute(context.Background(), readOperation(t), testCase.args)
			var inputError *InputError
			if !errors.As(err, &inputError) {
				t.Fatalf("expected *InputError, got %T: %v", err, err)
			}
		})
	}

	configurationCases := []struct {
		name       string
		operation  Operation
		wantHeader bool
	}{
		{"reserved header", reservedHeader, true},
		{"header CRLF", injectedHeader, true},
		{"bad method", badMethod, false},
		{"relative path", relativePath, false},
		{"undeclared token", undeclaredToken, false},
		{"unused path parameter", unknownToken, false},
		{"non-string path parameter", queryAsPath, false},
	}
	for _, testCase := range configurationCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := executor.Execute(context.Background(), testCase.operation, map[string]any{"ueContextId": "imsi-1"})
			var configurationError *ConfigurationError
			if !errors.As(err, &configurationError) {
				t.Fatalf("expected *ConfigurationError, got %T: %v", err, err)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("invalid operations must not reach the network, got %d calls", got)
	}
}

func TestExecuteJSONPostAndResponseSchema(t *testing.T) {
	type observed struct {
		method      string
		contentType string
		body        string
	}
	observations := make(chan observed, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		observations <- observed{
			method:      request.Method,
			contentType: request.Header.Get("Content-Type"),
			body:        string(body),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"cause":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	result, err := executor.Execute(context.Background(), jsonPostOperation(t), map[string]any{
		"body": map[string]any{"supi": "imsi-001010000000001"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected status %d", result.HTTPStatus)
	}
	got := <-observations
	if got.method != http.MethodPost || got.contentType != "application/json" {
		t.Fatalf("unexpected request: %+v", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got.body), &decoded); err != nil {
		t.Fatalf("invalid JSON request body: %v", err)
	}
	if decoded["supi"] != "imsi-001010000000001" {
		t.Fatalf("unexpected request body: %s", got.body)
	}

	mismatchServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"not-a-cause"}`)
	}))
	mismatchClient := newTestClient(t, mismatchServer, nil)
	mismatchExecutor := newTestExecutor(t, mismatchClient, nil, payload.Limits{})
	_, err = mismatchExecutor.Execute(context.Background(), jsonPostOperation(t), map[string]any{
		"body": map[string]any{"supi": "imsi-1"},
	})
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for response schema mismatch, got %T: %v", err, err)
	}

	invalidBodyExecutor := newTestExecutor(t, client, nil, payload.Limits{})
	_, err = invalidBodyExecutor.Execute(context.Background(), jsonPostOperation(t), map[string]any{
		"body": map[string]any{"other": "x"},
	})
	var inputError *InputError
	if !errors.As(err, &inputError) {
		t.Fatalf("expected *InputError for body schema mismatch, got %T: %v", err, err)
	}
}

func TestExecuteNoResponseBody(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})
	operation := Operation{
		OperationID:  "Test_DeleteContext",
		Effect:       EffectMutation,
		ServiceKey:   testServiceKey,
		Method:       http.MethodDelete,
		PathTemplate: "/ue-contexts/{ueContextId}",
		PathParameters: []Parameter{
			{Name: "ueContextId", Type: "string", Required: true},
		},
		Idempotency: &Idempotency{Mode: "NONE"},
	}
	result, err := executor.Execute(context.Background(), operation, map[string]any{"ueContextId": "imsi-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusNoContent || result.JSON != nil || len(result.Body) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteRejectsUndeclaredMediaTypeAndMalformedJSON(t *testing.T) {
	textServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(writer, "not json")
	}))
	textExecutor := newTestExecutor(t, newTestClient(t, textServer, nil), nil, payload.Limits{})
	_, err := textExecutor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for undeclared media type, got %T: %v", err, err)
	}

	duplicateServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"A","status":"B"}`)
	}))
	duplicateExecutor := newTestExecutor(t, newTestClient(t, duplicateServer, nil), nil, payload.Limits{})
	_, err = duplicateExecutor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for duplicate JSON keys, got %T: %v", err, err)
	}

	unexpectedServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"OK"}`)
	}))
	noBodyOperation := Operation{
		OperationID:  "Test_NoResponse",
		Effect:       EffectRead,
		ServiceKey:   testServiceKey,
		Method:       http.MethodGet,
		PathTemplate: "/health",
	}
	unexpectedExecutor := newTestExecutor(t, newTestClient(t, unexpectedServer, nil), nil, payload.Limits{})
	_, err = unexpectedExecutor.Execute(context.Background(), noBodyOperation, nil)
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for unexpected body, got %T: %v", err, err)
	}
}

func TestExecuteMutationIdempotencyRules(t *testing.T) {
	var calls atomic.Int32
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"cause":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})
	args := map[string]any{"body": map[string]any{"supi": "imsi-1"}}

	missingContract := jsonPostOperation(t)
	missingContract.Idempotency = nil
	_, err := executor.Execute(context.Background(), missingContract, args)
	var configurationError *ConfigurationError
	if !errors.As(err, &configurationError) {
		t.Fatalf("expected *ConfigurationError for mutation without idempotency, got %T: %v", err, err)
	}
	if calls.Load() != 0 {
		t.Fatal("mutation without idempotency reached the network")
	}

	required := jsonPostOperation(t)
	required.Idempotency = &Idempotency{Mode: "REQUIRED", KeyArgument: "/idempotencyKey"}
	_, err = executor.Execute(context.Background(), required, args)
	var inputError *InputError
	if !errors.As(err, &inputError) {
		t.Fatalf("expected *InputError for missing idempotency key, got %T: %v", err, err)
	}
	if calls.Load() != 0 {
		t.Fatal("mutation without idempotency key reached the network")
	}
	withKey := map[string]any{
		"body":           map[string]any{"supi": "imsi-1"},
		"idempotencyKey": "key-1",
	}
	if _, err := executor.Execute(context.Background(), required, withKey); err != nil {
		t.Fatalf("Execute with idempotency key: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one request, got %d", calls.Load())
	}

	// An idempotency contract never adds a retry: a failing mutation is
	// attempted exactly once.
	failingServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/problem+json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"status":503,"cause":"SERVICE_UNAVAILABLE"}`)
	}))
	failingExecutor := newTestExecutor(t, newTestClient(t, failingServer, nil), nil, payload.Limits{})
	_, err = failingExecutor.Execute(context.Background(), required, withKey)
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) {
		t.Fatalf("expected *RemoteError, got %T: %v", err, err)
	}
	if remoteError.Cause != "SERVICE_UNAVAILABLE" || remoteError.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("unexpected remote error: %+v", remoteError)
	}
}

func TestExecuteCancellationAndTimeout(t *testing.T) {
	entered := make(chan struct{}, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-request.Context().Done()
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, readOperation(t), map[string]any{"ueContextId": "imsi-1"})
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("request did not reach the server")
	}
	cancel()
	select {
	case err := <-result:
		var transportError *TransportError
		if !errors.As(err, &transportError) {
			t.Fatalf("expected *TransportError, got %T: %v", err, err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled to be preserved, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled request did not stop")
	}

	timeoutServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	timeoutClient := newTestClient(t, timeoutServer, func(config *sbi.Config) {
		config.Limits.RequestTimeout = 50 * time.Millisecond
		config.Limits.DialTimeout = time.Second
	})
	timeoutExecutor := newTestExecutor(t, timeoutClient, nil, payload.Limits{})
	_, err := timeoutExecutor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
	var transportError *TransportError
	if !errors.As(err, &transportError) {
		t.Fatalf("expected *TransportError for timeout, got %T: %v", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded to be preserved, got %v", err)
	}
}

func TestExecuteTrustedTokenProfileHeaderIsNotCallerControlled(t *testing.T) {
	type observed struct {
		token   string
		trusted string
	}
	observations := make(chan observed, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observations <- observed{
			token:   request.Header.Get("X-OpenAMSM-Query-Token"),
			trusted: request.Header.Get("X-Trusted-Request"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"OK"}`)
	}))
	client := newTestClient(t, server, func(config *sbi.Config) {
		config.HeaderTokenProfiles = map[string]sbi.HeaderTokenProfile{
			"query-token": {Header: "X-OpenAMSM-Query-Token", Token: "private-token"},
		}
		service := config.Services[testServiceKey]
		service.HeaderTokenProfile = "query-token"
		config.Services[testServiceKey] = service
	})
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	result, err := executor.Execute(context.Background(), readOperation(t), map[string]any{
		"ueContextId": "imsi-1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected status %d", result.HTTPStatus)
	}
	got := <-observations
	if got.token != "private-token" || got.trusted != "trusted-value" {
		t.Fatalf("unexpected headers: %+v", got)
	}

	// The operation's trusted header binding cannot name the configured token
	// header either: it would collide with the transport-injected value and is
	// rejected by the transport, and no argument can add it.
	tokenOverride := readOperation(t)
	tokenOverride.HeaderBindings = append(tokenOverride.HeaderBindings, HeaderBinding{Name: "X-OpenAMSM-Query-Token", Value: "caller"})
	_, err = executor.Execute(context.Background(), tokenOverride, map[string]any{"ueContextId": "imsi-1"})
	var configurationError *ConfigurationError
	if !errors.As(err, &configurationError) {
		t.Fatalf("expected *ConfigurationError for token header binding, got %T: %v", err, err)
	}
}

func TestExecuteOversizedRequestAndResponse(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"cause":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	limits := payload.Limits{MaxJSONBytes: 32, MaxBinaryBytes: 32, MaxTotalBytes: 64}
	executor := newTestExecutor(t, client, nil, limits)

	_, err := executor.Execute(context.Background(), jsonPostOperation(t), map[string]any{
		"body": map[string]any{"supi": strings.Repeat("x", 128)},
	})
	var inputError *InputError
	if !errors.As(err, &inputError) {
		t.Fatalf("expected *InputError for oversized request, got %T: %v", err, err)
	}

	largeServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"`+strings.Repeat("x", 128)+`"}`)
	}))
	largeExecutor := newTestExecutor(t, newTestClient(t, largeServer, nil), nil, limits)
	_, err = largeExecutor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for oversized response, got %T: %v", err, err)
	}
}

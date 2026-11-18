//go:build acceptance

package acceptance

// P0 process-level generic SBI acceptance. Every test in this file runs a
// deterministic loopback h2c upstream inside the test process and drives the
// providerfixture Provider binary over its real NDJSON stdin/stdout protocol.
// It never contacts the live OpenAMSM lab, and it never imports product
// packages to fake dispatch: the only production path exercised is the child
// process (initialize, catalog, invoke, cancel).
//
// The upstream is trusted by the child only through a serviceconfig v1 file
// pointing at the loopback origin under the acceptance-only service key
// "acceptance-fixture-sbi". The genericSBI bindings themselves exist only in
// the explicit providerfixture build.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	providerbindings "github.com/odango-chan/ca-client-tools/contracts/provider-bindings"
)

const (
	echoOperationID      = "acceptance.generic.echo"
	mutationOperationID  = "acceptance.generic.mutation"
	binaryOperationID    = "acceptance.generic.binary"
	multipartOperationID = "acceptance.generic.multipart"
	echoPath             = "/acceptance/v1/echo"
	mutationPath         = "/acceptance/v1/mutation"

	trustedFixtureHeader      = "X-Acceptance-Trusted"
	trustedFixtureHeaderValue = "fixture-7"

	// slowHandlerLinger is how long the "slow" upstream handler waits after the
	// request context ends before returning. It keeps the handler from
	// implicitly writing a 200 response before the Provider observes the
	// cancellation/deadline, making those assertions deterministic.
	slowHandlerLinger = 500 * time.Millisecond
)

// observedUpstreamRequest is one request as the loopback upstream saw it. It is
// used to prove trusted routing (method/path/header) and that the caller could
// not influence the wire request.
type observedUpstreamRequest struct {
	method        string
	escapedPath   string
	rawQuery      string
	trustedHeader string
	contentType   string
	accept        string
	authorization string
	body          []byte
}

// fixtureUpstream is the deterministic h2c upstream for the acceptance-only
// genericSBI bindings. Its behavior is selected by the trusted catalog query
// parameter "mode" (default "ok").
type fixtureUpstream struct {
	server *httptest.Server

	mu            sync.Mutex
	requests      []observedUpstreamRequest
	echoCalls     int
	mutationCalls int

	slowEntered chan struct{}
}

func newFixtureUpstream(t *testing.T) *fixtureUpstream {
	t.Helper()
	upstream := &fixtureUpstream{slowEntered: make(chan struct{}, 8)}
	server := httptest.NewUnstartedServer(http.HandlerFunc(upstream.handle))
	// Loopback-only cleartext HTTP/2: the same h2c prior-knowledge path the
	// shared SBI transport uses against a real SBI listener.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	upstream.server = server
	return upstream
}

func (u *fixtureUpstream) handle(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	switch {
	case request.Method == http.MethodPost && request.URL.EscapedPath() == echoPath:
		u.record(request, body)
		u.mu.Lock()
		u.echoCalls++
		u.mu.Unlock()
		switch request.URL.Query().Get("mode") {
		case "slow":
			select {
			case u.slowEntered <- struct{}{}:
			default:
			}
			<-request.Context().Done()
			time.Sleep(slowHandlerLinger)
			return
		case "status202":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(writer, `{"accepted":true}`)
		case "status204":
			writer.WriteHeader(http.StatusNoContent)
		case "problem404":
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"status":404,"cause":"SESSION_NOT_FOUND"}`)
		case "malformedProblem":
			writer.Header().Set("Content-Type", "application/problem+json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"status":404,"cause":"lowercase_cause"}`)
		case "nonproblem":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(writer, `{"oops":true}`)
		default:
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			if len(body) == 0 {
				_, _ = io.WriteString(writer, `{}`)
				return
			}
			// Echo the exact request body bytes so response JSON preservation
			// and integer precision are observable end to end.
			_, _ = writer.Write(body)
		}
	case request.Method == http.MethodPost && request.URL.EscapedPath() == mutationPath:
		u.record(request, body)
		u.mu.Lock()
		u.mutationCalls++
		u.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"dispatched":true}`)
	default:
		http.NotFound(writer, request)
	}
}

func (u *fixtureUpstream) record(request *http.Request, body []byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, observedUpstreamRequest{
		method:        request.Method,
		escapedPath:   request.URL.EscapedPath(),
		rawQuery:      request.URL.RawQuery,
		trustedHeader: request.Header.Get(trustedFixtureHeader),
		contentType:   request.Header.Get("Content-Type"),
		accept:        request.Header.Get("Accept"),
		authorization: request.Header.Get("Authorization"),
		body:          append([]byte(nil), body...),
	})
}

func (u *fixtureUpstream) snapshot() ([]observedUpstreamRequest, int, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	requests := make([]observedUpstreamRequest, len(u.requests))
	copy(requests, u.requests)
	return requests, u.echoCalls, u.mutationCalls
}

func (u *fixtureUpstream) origin() string { return u.server.URL }

func fixtureInitializeFrame(requestID string, maxCallDurationMs int64) frame {
	return frame{
		ProtocolVersion: protocolV2,
		Type:            "request",
		RequestID:       requestID,
		Method:          "initialize",
		Params: map[string]any{
			"hostInstanceId":     "acceptance-host",
			"providerInstanceId": "ca-client-tools.sbi",
			"limits": map[string]any{
				"maxFrameBytes":     8388608,
				"maxInFlight":       8,
				"maxCallDurationMs": maxCallDurationMs,
			},
		},
	}
}

// echoArguments is the nested object+array argument set for the fixture READ
// binding. The body number 9007199254740993 is deliberately larger than
// float64 can represent exactly.
func echoArguments() map[string]any {
	return map[string]any{
		"body": map[string]any{
			"label":  "acceptance",
			"number": json.Number("9007199254740993"),
			"items": []any{
				map[string]any{
					"id":     json.Number("0"),
					"tags":   []any{"alpha", "beta"},
					"weight": json.Number("1.5"),
				},
				map[string]any{"id": json.Number("2")},
			},
		},
	}
}

func echoArgumentsWithMode(mode string) map[string]any {
	arguments := echoArguments()
	arguments["mode"] = mode
	return arguments
}

func mutateEchoBody(mutate func(body map[string]any)) map[string]any {
	arguments := echoArguments()
	mutate(arguments["body"].(map[string]any))
	return arguments
}

func requestStringList(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value is not a JSON array: %#v", value)
	}
	result := make([]string, len(raw))
	for index, entry := range raw {
		text, ok := entry.(string)
		if !ok {
			t.Fatalf("array entry %d is not a string: %#v", index, entry)
		}
		result[index] = text
	}
	return result
}

// TestProviderGenericInitializeCatalogAndDispatch proves the catalog identity
// and the generic READ path through the real child process: nested
// object/array arguments, integer precision, trusted defaults, trusted
// routing, response preservation and status/ProblemDetails mapping.
func TestProviderGenericInitializeCatalogAndDispatch(t *testing.T) {
	upstream := newFixtureUpstream(t)
	// The typed service keys are configured against the same loopback origin so
	// the catalog exposes all eleven production typed descriptors for regression
	// assertions. No typed remote call is performed.
	process := startFixtureProviderAllServices(t, upstream.origin())

	initialized := expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))
	if initialized["providerId"] != "ca-client-tools.sbi" {
		t.Fatalf("providerId = %v", initialized["providerId"])
	}
	if initialized["catalogVersion"] != "2" {
		t.Fatalf("catalogVersion = %v, want \"2\"", initialized["catalogVersion"])
	}
	// The digest must be the SHA-256 of the exact embedded v2 artifact bytes
	// (including the final newline), reported as "sha256:"+lowercase hex.
	sum := sha256.Sum256(providerbindings.Catalog)
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])
	if initialized["backendBindingCatalogDigest"] != wantDigest {
		t.Fatalf("backendBindingCatalogDigest = %v, want %s", initialized["backendBindingCatalogDigest"], wantDigest)
	}
	if buildDigest, _ := initialized["buildDigest"].(string); !strings.HasPrefix(buildDigest, "sha256:") {
		t.Fatalf("buildDigest = %v", initialized["buildDigest"])
	}

	// Catalog request order interleaves the two fixture IDs with the eleven
	// production IDs. The single embedded catalog partitions every requested ID
	// exactly once, preserving request order.
	productionIDs := allV2IDs()
	requested := make([]string, 0, len(productionIDs)+2)
	requested = append(requested, mutationOperationID)
	for index, id := range productionIDs {
		requested = append(requested, id)
		if index == 0 {
			requested = append(requested, echoOperationID)
		}
	}
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "catalog-001", requested)))
	if catalogResult["catalogVersion"] != "2" {
		t.Fatalf("catalog catalogVersion = %v, want \"2\"", catalogResult["catalogVersion"])
	}
	supported := requestStringList(t, catalogResult["supported"])
	if !reflect.DeepEqual(supported, requested) {
		t.Fatalf("supported = %#v, want %#v (request order preserved)", supported, requested)
	}
	unsupported, ok := catalogResult["unsupported"].([]any)
	if !ok || len(unsupported) != 0 {
		t.Fatalf("unsupported = %#v, want empty", catalogResult["unsupported"])
	}

	descriptors, ok := catalogResult["bindings"].([]any)
	if !ok || len(descriptors) != len(requested) {
		t.Fatalf("bindings = %#v, want %d descriptors", catalogResult["bindings"], len(requested))
	}
	for index, wantID := range requested {
		descriptor, ok := descriptors[index].(map[string]any)
		if !ok {
			t.Fatalf("bindings[%d] = %#v", index, descriptors[index])
		}
		if descriptor["backendOperationId"] != wantID {
			t.Fatalf("bindings[%d].backendOperationId = %v, want %s", index, descriptor["backendOperationId"], wantID)
		}
		wantEffect := "READ"
		if wantID == mutationOperationID {
			wantEffect = "MUTATION"
		}
		if descriptor["effect"] != wantEffect {
			t.Fatalf("bindings[%d].effect = %v, want %s", index, descriptor["effect"], wantEffect)
		}
		if wantID == mutationOperationID || wantID == echoOperationID {
			// Generic bindings publish the closed nested argument schema and
			// omit the scalar typed argument list.
			if _, exists := descriptor["arguments"]; exists {
				t.Fatalf("generic descriptor %s exposes scalar arguments", wantID)
			}
			schema, ok := descriptor["argumentSchema"].(map[string]any)
			if !ok || schema["type"] != "object" {
				t.Fatalf("generic descriptor %s argumentSchema = %#v", wantID, descriptor["argumentSchema"])
			}
		} else {
			// Typed descriptors publish their frozen scalar argument profile
			// (two arguments for every shipped read binding) and no nested
			// schema.
			if _, exists := descriptor["argumentSchema"]; exists {
				t.Fatalf("typed descriptor %s exposes argumentSchema", wantID)
			}
			arguments, ok := descriptor["arguments"].([]any)
			if !ok || len(arguments) != 2 {
				t.Fatalf("typed descriptor %s arguments = %#v", wantID, descriptor["arguments"])
			}
		}
		for _, field := range []string{"request", "response", "source"} {
			if _, exists := descriptor[field]; !exists {
				t.Fatalf("descriptor %s is missing %q", wantID, field)
			}
		}
	}
	descriptorText, err := json.Marshal(descriptors)
	if err != nil {
		t.Fatalf("marshal descriptors: %v", err)
	}
	for _, forbidden := range []string{
		"origin", "http://", "https://", `"url"`, `"method"`, `"path"`,
		"serviceKey", "token", "Authorization", trustedFixtureHeader,
	} {
		if strings.Contains(string(descriptorText), forbidden) {
			t.Fatalf("catalog descriptor leaked %q: %s", forbidden, descriptorText)
		}
	}

	// READ invoke: nested object+array arguments, exact integer precision,
	// optional mode default applied, trusted fixed header, response preserved.
	echo := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-ok", echoOperationID, echoArguments())))
	if status, _ := number(echo["httpStatus"]); status != 200 {
		t.Fatalf("echo httpStatus = %v", echo["httpStatus"])
	}
	echoBody, ok := echo["response"].(map[string]any)
	if !ok {
		t.Fatalf("echo response = %#v", echo["response"])
	}
	if !reflect.DeepEqual(echoBody, echoArguments()["body"]) {
		t.Fatalf("echo response was not preserved: %#v", echoBody)
	}
	preciseNumber, ok := echoBody["number"].(json.Number)
	if !ok || preciseNumber.String() != "9007199254740993" {
		t.Fatalf("echo body number = %#v, want exact json.Number 9007199254740993", echoBody["number"])
	}
	items, ok := echoBody["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("echo body items = %#v", echoBody["items"])
	}
	firstItem, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("echo body items[0] = %#v", items[0])
	}
	firstID, _ := firstItem["id"].(json.Number)
	if firstID.String() != "0" {
		t.Fatalf("echo body items[0] = %#v", items[0])
	}
	if tags, ok := firstItem["tags"].([]any); !ok || len(tags) != 2 || tags[1] != "beta" {
		t.Fatalf("echo body items[0].tags = %#v", firstItem["tags"])
	}

	requests, echoCalls, mutationCalls := upstream.snapshot()
	if echoCalls != 1 || mutationCalls != 0 || len(requests) != 1 {
		t.Fatalf("upstream calls after echo: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}
	observed := requests[0]
	if observed.method != http.MethodPost || observed.escapedPath != echoPath {
		t.Fatalf("upstream request = %s %s, want POST %s", observed.method, observed.escapedPath, echoPath)
	}
	if observed.rawQuery != "mode=ok" {
		t.Fatalf("optional query default was not applied upstream: mode query = %q", observed.rawQuery)
	}
	if observed.trustedHeader != trustedFixtureHeaderValue {
		t.Fatalf("trusted header upstream = %q, want %q", observed.trustedHeader, trustedFixtureHeaderValue)
	}
	if observed.contentType != "application/json" {
		t.Fatalf("upstream Content-Type = %q", observed.contentType)
	}
	if observed.authorization != "" {
		t.Fatalf("upstream received an Authorization header: %q", observed.authorization)
	}
	if !strings.Contains(string(observed.body), "9007199254740993") {
		t.Fatalf("upstream request body lost integer precision: %s", observed.body)
	}

	// 202 is preserved without polling.
	accepted := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-202", echoOperationID, echoArgumentsWithMode("status202"))))
	if status, _ := number(accepted["httpStatus"]); status != 202 {
		t.Fatalf("202 httpStatus = %v", accepted["httpStatus"])
	}
	acceptedBody, ok := accepted["response"].(map[string]any)
	if !ok || acceptedBody["accepted"] != true {
		t.Fatalf("202 response = %#v", accepted["response"])
	}

	// 204 with no body yields only the status.
	noContent := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-204", echoOperationID, echoArgumentsWithMode("status204"))))
	if len(noContent) != 1 {
		t.Fatalf("204 result = %#v, want only httpStatus", noContent)
	}
	if status, _ := number(noContent["httpStatus"]); status != 204 {
		t.Fatalf("204 httpStatus = %v", noContent["httpStatus"])
	}
	if _, exists := noContent["response"]; exists {
		t.Fatalf("204 result must not carry a response field: %#v", noContent)
	}

	// A valid remote application/problem+json 404 is a successful protocol
	// result, not a provider fault.
	problem := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-problem", echoOperationID, echoArgumentsWithMode("problem404"))))
	if len(problem) != 2 {
		t.Fatalf("problem result = %#v, want httpStatus+problem", problem)
	}
	if status, _ := number(problem["httpStatus"]); status != 404 {
		t.Fatalf("problem httpStatus = %v", problem["httpStatus"])
	}
	problemDetails, ok := problem["problem"].(map[string]any)
	if !ok {
		t.Fatalf("problem = %#v", problem["problem"])
	}
	if status, _ := number(problemDetails["status"]); status != 404 {
		t.Fatalf("problem.status = %v", problemDetails["status"])
	}
	if problemDetails["cause"] != "SESSION_NOT_FOUND" {
		t.Fatalf("problem.cause = %v", problemDetails["cause"])
	}
	if _, exists := problem["response"]; exists {
		t.Fatalf("problem result must not carry a response field: %#v", problem)
	}

	// A malformed problem body and a non-problem non-200 response are
	// REMOTE_RESPONSE_INVALID.
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-malformed-problem", echoOperationID,
		echoArgumentsWithMode("malformedProblem"))), "REMOTE_RESPONSE_INVALID")
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-nonproblem", echoOperationID,
		echoArgumentsWithMode("nonproblem"))), "REMOTE_RESPONSE_INVALID")

	// Faults do not terminate the process; it serves a final valid invoke.
	final := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-echo-final", echoOperationID, echoArguments())))
	if status, _ := number(final["httpStatus"]); status != 200 {
		t.Fatalf("final echo httpStatus = %v", final["httpStatus"])
	}

	requests, echoCalls, mutationCalls = upstream.snapshot()
	if echoCalls != 7 || mutationCalls != 0 || len(requests) != 7 {
		t.Fatalf("upstream calls at end: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}
	for index, observed := range requests {
		if observed.method != http.MethodPost || observed.escapedPath != echoPath {
			t.Fatalf("upstream request %d = %s %s", index, observed.method, observed.escapedPath)
		}
		if observed.trustedHeader != trustedFixtureHeaderValue {
			t.Fatalf("upstream request %d trusted header = %q", index, observed.trustedHeader)
		}
		if observed.authorization != "" {
			t.Fatalf("upstream request %d carried authorization", index)
		}
	}
}

// TestProviderGenericUnconfiguredBindings proves the unavailable side of the
// catalog partition: with only the fixture service configured, the two fixture
// bindings are supported and all eleven production typed bindings are reported
// NOT_CONFIGURED. No live deployment is involved.
func TestProviderGenericUnconfiguredBindings(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	productionIDs := allV2IDs()
	requested := append([]string{echoOperationID, mutationOperationID}, productionIDs...)
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "catalog-unconfigured", requested)))
	supported := requestStringList(t, catalogResult["supported"])
	wantSupported := []string{echoOperationID, mutationOperationID}
	if !reflect.DeepEqual(supported, wantSupported) {
		t.Fatalf("supported = %#v, want %#v", supported, wantSupported)
	}
	unsupported, ok := catalogResult["unsupported"].([]any)
	if !ok || len(unsupported) != len(productionIDs) {
		t.Fatalf("unsupported = %#v, want %d entries", catalogResult["unsupported"], len(productionIDs))
	}
	for index, raw := range unsupported {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unsupported[%d] = %#v", index, raw)
		}
		if entry["backendOperationId"] != productionIDs[index] {
			t.Fatalf("unsupported[%d] = %v, want %s", index, entry["backendOperationId"], productionIDs[index])
		}
		if entry["reason"] != "NOT_CONFIGURED" {
			t.Fatalf("unsupported[%d].reason = %v, want NOT_CONFIGURED", index, entry["reason"])
		}
	}
	if descriptors, _ := catalogResult["bindings"].([]any); len(descriptors) != len(wantSupported) {
		t.Fatalf("bindings = %#v, want %d descriptors", catalogResult["bindings"], len(wantSupported))
	}
}

// TestProviderGenericArtifactBindingsRejectedBeforeIO proves the P0 response
// projection boundary through the real child process: the acceptance-only
// binary and multipart/related genericSBI bindings are reported
// NOT_IMPLEMENTED and are rejected with BACKEND_OPERATION_UNSUPPORTED before
// any remote I/O. This is the process-level proof that an artifact-bearing
// response can never be returned as a partial JSON success that silently drops
// attachments.
func TestProviderGenericArtifactBindingsRejectedBeforeIO(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	requested := []string{binaryOperationID, multipartOperationID, echoOperationID}
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "catalog-artifacts", requested)))
	supported := requestStringList(t, catalogResult["supported"])
	if !reflect.DeepEqual(supported, []string{echoOperationID}) {
		t.Fatalf("supported = %#v, want only %s", supported, echoOperationID)
	}
	unsupported, ok := catalogResult["unsupported"].([]any)
	if !ok || len(unsupported) != 2 {
		t.Fatalf("unsupported = %#v, want the two artifact bindings", catalogResult["unsupported"])
	}
	for index, wantID := range []string{binaryOperationID, multipartOperationID} {
		entry, ok := unsupported[index].(map[string]any)
		if !ok {
			t.Fatalf("unsupported[%d] = %#v", index, unsupported[index])
		}
		if entry["backendOperationId"] != wantID || entry["reason"] != "NOT_IMPLEMENTED" {
			t.Fatalf("unsupported[%d] = %#v, want %s/NOT_IMPLEMENTED", index, entry, wantID)
		}
	}

	for index, operationID := range []string{binaryOperationID, multipartOperationID} {
		decoded := process.roundTrip(invokeFrame(protocolV2, fmt.Sprintf("invoke-artifact-%02d", index), operationID, map[string]any{}))
		expectFault(t, decoded, "BACKEND_OPERATION_UNSUPPORTED")
		detail := decoded.Error.Detail
		if detail == nil || detail["reason"] != "NOT_IMPLEMENTED" {
			t.Fatalf("%s fault detail = %#v, want reason NOT_IMPLEMENTED", operationID, detail)
		}
		if len(detail) != 1 {
			t.Fatalf("%s fault detail leaked extra fields: %#v", operationID, detail)
		}
	}

	requests, echoCalls, mutationCalls := upstream.snapshot()
	if len(requests) != 0 || echoCalls != 0 || mutationCalls != 0 {
		t.Fatalf("artifact-bearing operations reached the upstream: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}

	// The process keeps serving a normal JSON success after the rejections.
	if result := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-after-artifacts", echoOperationID, echoArguments()))); result["httpStatus"] == nil {
		t.Fatalf("valid invoke after artifact rejection = %#v", result)
	}
	requests, echoCalls, _ = upstream.snapshot()
	if len(requests) != 1 || echoCalls != 1 {
		t.Fatalf("valid invoke did not reach the upstream: requests=%d echo=%d", len(requests), echoCalls)
	}
}

// TestProviderGenericArgumentsRejectedBeforeIO proves invalid invocation
// arguments (including caller attempts to add an undeclared argument, header or
// URL) are rejected with BACKEND_ARGUMENTS_INVALID before any upstream request.
func TestProviderGenericArgumentsRejectedBeforeIO(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	undeclaredHeader := echoArguments()
	undeclaredHeader["headers"] = map[string]any{"X-Evil": "1"}
	callerURL := echoArguments()
	callerURL["url"] = "http://127.0.0.1:1/evil"
	callerMethod := echoArguments()
	callerMethod["method"] = "DELETE"

	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{"undeclared top-level argument header", undeclaredHeader},
		{"undeclared top-level argument url", callerURL},
		{"undeclared top-level argument method", callerMethod},
		{"undeclared nested body field", mutateEchoBody(func(body map[string]any) { body["extra"] = true })},
		{"wrong nested type", mutateEchoBody(func(body map[string]any) { body["number"] = "not-an-integer" })},
		{"missing required body field", mutateEchoBody(func(body map[string]any) { delete(body, "items") })},
		{"nested integer below minimum", mutateEchoBody(func(body map[string]any) {
			body["items"].([]any)[0].(map[string]any)["id"] = json.Number("-1")
		})},
		{"wrong nested array shape", mutateEchoBody(func(body map[string]any) { body["items"] = "not-an-array" })},
	}
	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requestID := fmt.Sprintf("invalid-%02d", index)
			expectFault(t, process.roundTrip(invokeFrame(protocolV2, requestID, echoOperationID, testCase.arguments)),
				"BACKEND_ARGUMENTS_INVALID")
		})
	}

	requests, echoCalls, mutationCalls := upstream.snapshot()
	if len(requests) != 0 || echoCalls != 0 || mutationCalls != 0 {
		t.Fatalf("invalid arguments reached the upstream: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}

	// The process remains healthy and a valid invoke reaches the upstream.
	if result := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "valid-after-invalid", echoOperationID, echoArguments()))); result["httpStatus"] == nil {
		t.Fatalf("valid invoke result = %#v", result)
	}
	requests, echoCalls, mutationCalls = upstream.snapshot()
	if len(requests) != 1 || echoCalls != 1 || mutationCalls != 0 {
		t.Fatalf("valid invoke did not reach the upstream: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}
}

// TestProviderGenericDuplicateKeyRejected proves a duplicate JSON key at a
// nested depth is a PROTOCOL_FRAME_INVALID fault and that the process keeps
// serving afterwards.
func TestProviderGenericDuplicateKeyRejected(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	raw := `{"protocolVersion":"ca-client-tools.provider/v2","type":"request","requestId":"dup-key-001","method":"invoke","params":{"backendOperationId":"acceptance.generic.echo","arguments":{"body":{"label":"one","label":"two","number":1,"items":[]}},"invocation":{"invocationId":"inv-dup-key","producerInvocationId":"producer-dup-key","deadline":"2030-01-01T00:00:00Z"}}}` + "\n"
	if err := process.stdinWriteRaw(raw); err != nil {
		t.Fatalf("write duplicate-key frame: %v", err)
	}
	// The duplicate key is unrecoverable during best-effort correlation, so the
	// fault response may carry an empty requestId.
	duplicate := process.wait("")
	expectFault(t, duplicate, "PROTOCOL_FRAME_INVALID")

	requests, echoCalls, _ := upstream.snapshot()
	if len(requests) != 0 || echoCalls != 0 {
		t.Fatalf("duplicate-key frame reached the upstream: requests=%d echo=%d", len(requests), echoCalls)
	}

	// The process still serves a valid request.
	final := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-after-duplicate", echoOperationID, echoArguments())))
	if status, _ := number(final["httpStatus"]); status != 200 {
		t.Fatalf("invoke after duplicate-key fault httpStatus = %v", final["httpStatus"])
	}
}

// TestProviderGenericMutationGate proves the production Mutation gate rejects
// an otherwise configured generic MUTATION binding before any remote I/O.
func TestProviderGenericMutationGate(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	mutationArguments := map[string]any{
		"requestId": "mutation-request-001",
		"body":      map[string]any{"value": "acceptance"},
	}
	decoded := process.roundTrip(invokeFrame(protocolV2, "invoke-mutation", mutationOperationID, mutationArguments))
	expectFault(t, decoded, "BACKEND_OPERATION_UNSUPPORTED")
	detail := decoded.Error.Detail
	if detail == nil || detail["reason"] != "MUTATION_GATED" {
		t.Fatalf("mutation gate detail = %#v, want reason MUTATION_GATED", detail)
	}

	requests, echoCalls, mutationCalls := upstream.snapshot()
	if len(requests) != 0 || echoCalls != 0 || mutationCalls != 0 {
		t.Fatalf("gated mutation reached the upstream: requests=%d echo=%d mutation=%d", len(requests), echoCalls, mutationCalls)
	}
}

// TestProviderGenericDeadlineExceeded proves a tiny negotiated maxCallDuration
// bounds a slow generic invoke and yields CALL_DEADLINE_EXCEEDED.
func TestProviderGenericDeadlineExceeded(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 100)))

	started := time.Now()
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-deadline", echoOperationID, echoArgumentsWithMode("slow"))),
		"CALL_DEADLINE_EXCEEDED")
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("deadline invoke took %s", elapsed)
	}
	_, echoCalls, mutationCalls := upstream.snapshot()
	if echoCalls != 1 || mutationCalls != 0 {
		t.Fatalf("deadline slow invoke: echo=%d mutation=%d", echoCalls, mutationCalls)
	}
}

// TestProviderGenericCancellation proves an in-flight generic invoke that is
// cancelled yields exactly one terminal CALL_CANCELLED response and a
// CANCEL_REQUESTED disposition. The slow upstream never completes on its own,
// so the completion race is not present here.
func TestProviderGenericCancellation(t *testing.T) {
	upstream := newFixtureUpstream(t)
	process := startFixtureProvider(t, upstream.origin())
	expectOK(t, process.roundTrip(fixtureInitializeFrame("init-001", 30000)))

	process.write(invokeFrame(protocolV2, "invoke-cancel-target", echoOperationID, echoArgumentsWithMode("slow")))
	select {
	case <-upstream.slowEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("slow generic invoke did not reach the upstream")
	}

	cancelResult := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-001", "invoke-cancel-target")))
	if cancelResult["disposition"] != "CANCEL_REQUESTED" {
		t.Fatalf("cancel disposition = %v, want CANCEL_REQUESTED", cancelResult["disposition"])
	}

	terminal := process.wait("invoke-cancel-target")
	expectFault(t, terminal, "CALL_CANCELLED")
	if count := process.responseCount("invoke-cancel-target"); count != 1 {
		t.Fatalf("cancelled invoke terminal response count = %d, want exactly 1", count)
	}
	// A late completion would show up as a second response for the same ID.
	time.Sleep(250 * time.Millisecond)
	if count := process.responseCount("invoke-cancel-target"); count != 1 {
		t.Fatalf("cancelled invoke produced %d terminal responses, want exactly 1", count)
	}

	secondCancel := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-002", "invoke-cancel-target")))
	if secondCancel["disposition"] != "ALREADY_COMPLETED" {
		t.Fatalf("second cancel disposition = %v, want ALREADY_COMPLETED", secondCancel["disposition"])
	}
	notFound := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-003", "no-such-request")))
	if notFound["disposition"] != "NOT_FOUND" {
		t.Fatalf("unknown cancel disposition = %v, want NOT_FOUND", notFound["disposition"])
	}
}

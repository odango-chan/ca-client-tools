package provider

// Tests for the catalog, invoke, cancellation, bounded-concurrency and
// lifecycle (drain) behavior of the resident Provider process.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func makeUnavailable(binding Binding, reason string) Binding {
	binding.Available = false
	binding.UnavailableReason = reason
	return binding
}

func waitStarted(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case operationID := <-started:
		return operationID
	case <-time.After(awaitTimeout):
		t.Fatal("timed out waiting for the fake runtime to start a call")
		return ""
	}
}

func assertNoStart(t *testing.T, started <-chan string, within time.Duration) {
	t.Helper()
	select {
	case operationID := <-started:
		t.Fatalf("fake runtime started %q while the maxInFlight bound should have blocked it", operationID)
	case <-time.After(within):
	}
}

func supiFor(index int) string {
	return fmt.Sprintf("imsi-%015d", index)
}

// TestCatalog covers the private backend operation catalog contract.
func TestCatalog(t *testing.T) {
	const (
		scID      = "openamsm.smf.mas.sc.query"
		paID      = "openamsm.smf.mas.pa.query"
		caID      = "openamsm.smf.mas.ca.query"
		iamID     = "openamsm.smf.mas.iam.query"
		unknownID = "openamsm.smf.mas.unknown.query"
	)

	t.Run("partition_preserves_request_order", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.bindings[paID] = makeUnavailable(fake.bindings[paID], "")
		fake.bindings[iamID] = makeUnavailable(fake.bindings[iamID], "NOT_IMPLEMENTED")
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-catalog-v1")

		requested := []string{scID, unknownID, paID, caID, iamID}
		result := h.catalog(t, ProtocolV2, "catalog-partition", requested).expectOK(t, ProtocolV2, "catalog-partition")
		requireExactKeys(t, "catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")

		supported := requireStringSlice(t, "catalog supported", result["supported"])
		requireEqualStrings(t, "catalog supported", supported, []string{scID, caID})

		unsupported := requireObjectSlice(t, "catalog unsupported", result["unsupported"])
		if len(unsupported) != 3 {
			t.Fatalf("catalog unsupported = %v, want 3 entries", unsupported)
		}
		wantUnsupported := []struct {
			id     string
			reason string
		}{
			{unknownID, "NOT_IMPLEMENTED"},
			{paID, "NOT_CONFIGURED"},
			{iamID, "NOT_IMPLEMENTED"},
		}
		for index, want := range wantUnsupported {
			entry := unsupported[index]
			requireExactKeys(t, "unsupported entry", entry, "backendOperationId", "reason")
			if got := requireString(t, "unsupported backendOperationId", entry["backendOperationId"]); got != want.id {
				t.Fatalf("unsupported[%d].backendOperationId = %q, want %q", index, got, want.id)
			}
			if got := requireString(t, "unsupported reason", entry["reason"]); got != want.reason {
				t.Fatalf("unsupported[%d].reason = %q, want %q", index, got, want.reason)
			}
		}

		// The response must partition the requested IDs exactly: no duplicate,
		// unrequested, altered or omitted ID.
		seen := make(map[string]int, len(requested))
		for _, id := range supported {
			seen[id]++
		}
		for _, entry := range unsupported {
			seen[requireString(t, "unsupported id", entry["backendOperationId"])]++
		}
		for _, id := range requested {
			if seen[id] != 1 {
				t.Fatalf("requested ID %q appears %d times in the catalog partition %v", id, seen[id], seen)
			}
			delete(seen, id)
		}
		if len(seen) != 0 {
			t.Fatalf("catalog returned unrequested IDs: %v", seen)
		}

		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("empty_request_returns_empty_lists", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-catalog-empty")
		result := h.catalog(t, ProtocolV2, "catalog-empty", nil).expectOK(t, ProtocolV2, "catalog-empty")
		requireExactKeys(t, "catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")
		if supported := requireStringSlice(t, "supported", result["supported"]); len(supported) != 0 {
			t.Fatalf("supported = %v, want empty", supported)
		}
		if unsupported := requireObjectSlice(t, "unsupported", result["unsupported"]); len(unsupported) != 0 {
			t.Fatalf("unsupported = %v, want empty", unsupported)
		}
		if got := h.framesFor("catalog-empty"); len(got) != 1 {
			t.Fatalf("catalog received %d responses, want 1", len(got))
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("invalid_requests_are_rejected", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-catalog-invalid")

		tooLongID := strings.Repeat("b", 129)
		cases := []struct {
			name     string
			params   map[string]any
			wantCode string
		}{
			{"duplicate_requested_id", map[string]any{
				"backendOperationIds": []any{scID, scID},
			}, CodeProtocolFrameInvalid},
			{"non_array", map[string]any{"backendOperationIds": scID}, CodeProtocolFrameInvalid},
			{"null_array", map[string]any{"backendOperationIds": nil}, CodeProtocolFrameInvalid},
			{"element_not_string", map[string]any{"backendOperationIds": []any{scID, 7}}, CodeProtocolFrameInvalid},
			{"element_empty_string", map[string]any{"backendOperationIds": []any{""}}, CodeProtocolFrameInvalid},
			{"element_too_long", map[string]any{"backendOperationIds": []any{tooLongID}}, CodeProtocolFrameInvalid},
			{"element_with_space", map[string]any{"backendOperationIds": []any{"an id with spaces"}}, CodeProtocolFrameInvalid},
			{"unknown_param_member", map[string]any{"backendOperationIds": []any{}, "extra": true}, CodeProtocolFrameInvalid},
			{"missing_param_member", map[string]any{}, CodeProtocolFrameInvalid},
			{"params_missing_entirely", nil, CodeProtocolFrameInvalid},
		}
		for index, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				requestID := fmt.Sprintf("catalog-bad-%d", index)
				h.send(t, wireRequest(ProtocolV2, requestID, "catalog", testCase.params))
				h.await(t, requestID).expectError(t, ProtocolV2, requestID, testCase.wantCode)
			})
		}
		if fake.callCount() != 0 {
			t.Fatalf("catalog invoked the backend %d times, want 0", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("catalog_does_not_leak_private_fields", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-catalog-v1-fields")
		frame := h.catalog(t, ProtocolV2, "catalog-v1-fields", allBindingIDs())
		result := frame.expectOK(t, ProtocolV2, "catalog-v1-fields")
		requireExactKeys(t, "catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")
		if got := requireString(t, "catalogVersion", result["catalogVersion"]); got != testCatalogVersion {
			t.Fatalf("catalogVersion = %q, want %q", got, testCatalogVersion)
		}
		for _, forbidden := range []string{"handlerProfile", "serviceKey", "origin", "url", "method", "path", "token", "header"} {
			if containsJSONKey(result, forbidden) {
				t.Fatalf("catalog result leaks forbidden field %q: %v", forbidden, result)
			}
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// containsJSONKey recursively reports whether a decoded JSON value has the
// given object key anywhere.
func containsJSONKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for objectKey, child := range typed {
			if objectKey == key || containsJSONKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsJSONKey(child, key) {
				return true
			}
		}
	}
	return false
}

// TestInvoke covers backend dispatch and the Provider result shape.
func TestInvoke(t *testing.T) {
	t.Run("success_200_typed_response", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			return Result{HTTPStatus: 200, Response: map[string]any{
				"masId":  "SC",
				"status": "AVAILABLE",
				"key": map[string]any{
					"supi":         arguments["supi"],
					"pduSessionId": arguments["pduSessionId"],
				},
			}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-invoke-success")
		result := h.invoke(t, ProtocolV2, "invoke-success", standardArguments()).expectOK(t, ProtocolV2, "invoke-success")

		requireExactKeys(t, "invoke result", result, "httpStatus", "response")
		if got := requireNumber(t, "httpStatus", result["httpStatus"]); got != 200 {
			t.Fatalf("httpStatus = %d, want 200", got)
		}
		response, ok := result["response"].(map[string]any)
		if !ok {
			t.Fatalf("response = %#v, want object", result["response"])
		}
		if got := requireString(t, "response.masId", response["masId"]); got != "SC" {
			t.Fatalf("response.masId = %q, want SC", got)
		}

		invocation := fake.onlyInvocation(t)
		if invocation.OperationID != "openamsm.smf.mas.sc.query" {
			t.Fatalf("backend operation = %q, want openamsm.smf.mas.sc.query", invocation.OperationID)
		}
		if got := requireString(t, "supi", invocation.Arguments["supi"]); got != "imsi-001010000000001" {
			t.Fatalf("supi argument = %q, want imsi-001010000000001", got)
		}
		pduSessionID, ok := invocation.Arguments["pduSessionId"].(int64)
		if !ok || pduSessionID != 5 {
			t.Fatalf("pduSessionId argument = %#v, want int64(5)", invocation.Arguments["pduSessionId"])
		}
		if len(invocation.Arguments) != 2 {
			t.Fatalf("backend received arguments %v, want exactly supi and pduSessionId", invocation.Arguments)
		}

		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("valid_problem_details_is_a_result", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(context.Context, string, map[string]any) (Result, error) {
			return Result{HTTPStatus: 404, Problem: &ProblemDetails{Status: 404, Cause: "SESSION_NOT_FOUND"}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-invoke-problem")
		result := h.invoke(t, ProtocolV2, "invoke-problem", standardArguments()).expectOK(t, ProtocolV2, "invoke-problem")

		requireExactKeys(t, "problem result", result, "httpStatus", "problem")
		if got := requireNumber(t, "httpStatus", result["httpStatus"]); got != 404 {
			t.Fatalf("httpStatus = %d, want 404", got)
		}
		if _, present := result["response"]; present {
			t.Fatal("ProblemDetails result must not also carry response")
		}
		problem, ok := result["problem"].(map[string]any)
		if !ok {
			t.Fatalf("problem = %#v, want object", result["problem"])
		}
		requireExactKeys(t, "problem", problem, "status", "cause")
		if got := requireNumber(t, "problem.status", problem["status"]); got != 404 {
			t.Fatalf("problem.status = %d, want 404", got)
		}
		if got := requireString(t, "problem.cause", problem["cause"]); got != "SESSION_NOT_FOUND" {
			t.Fatalf("problem.cause = %q, want SESSION_NOT_FOUND", got)
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("backend_faults_pass_through", func(t *testing.T) {
		cases := []struct {
			code   string
			detail map[string]any
		}{
			{CodeRemoteTransportError, nil},
			{CodeRemoteResponseInvalid, nil},
			{CodeBackendArgumentsInvalid, map[string]any{"argument": "supi"}},
		}
		for _, testCase := range cases {
			testCase := testCase
			t.Run(testCase.code, func(t *testing.T) {
				fake := newFakeCatalog()
				fake.invokeFn = func(context.Context, string, map[string]any) (Result, error) {
					return Result{}, &Fault{Code: testCase.code, Detail: testCase.detail}
				}
				h := newHarness(t, fixedRuntimeFactory(fake))
				h.mustInitialize(t, ProtocolV2, "init-invoke-fault")
				detail := h.invoke(t, ProtocolV2, "invoke-fault", standardArguments()).expectError(t, ProtocolV2, "invoke-fault", testCase.code)
				if testCase.detail != nil {
					if got := requireString(t, "fault detail argument", detail["argument"]); got != "supi" {
						t.Fatalf("fault detail argument = %q, want supi", got)
					}
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})

	t.Run("backend_error_mapping", func(t *testing.T) {
		cases := []struct {
			name     string
			err      error
			wantCode string
		}{
			{"deadline_exceeded", context.DeadlineExceeded, CodeCallDeadlineExceeded},
			{"canceled", context.Canceled, CodeCallCancelled},
			{"unknown_error", errors.New("backend exploded"), CodeProviderInternal},
		}
		for _, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fake := newFakeCatalog()
				fake.invokeFn = func(context.Context, string, map[string]any) (Result, error) {
					return Result{}, testCase.err
				}
				h := newHarness(t, fixedRuntimeFactory(fake))
				h.mustInitialize(t, ProtocolV2, "init-invoke-error-map")
				h.invoke(t, ProtocolV2, "invoke-error-map", standardArguments()).expectError(t, ProtocolV2, "invoke-error-map", testCase.wantCode)
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})

	t.Run("unknown_operation_is_unsupported", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-invoke-unknown")
		h.invokeOperation(t, ProtocolV2, "invoke-unknown", "openamsm.smf.mas.does-not-exist", standardArguments(), futureDeadline())
		h.await(t, "invoke-unknown").expectError(t, ProtocolV2, "invoke-unknown", CodeBackendOperationUnsupported)
		if fake.callCount() != 0 {
			t.Fatalf("unknown operation reached the backend %d times, want 0", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("unavailable_operation_reports_reason", func(t *testing.T) {
		cases := []struct {
			name        string
			unavailable string
			wantReason  string
		}{
			{"not_configured_default", "", "NOT_CONFIGURED"},
			{"explicit_reason", "NOT_IMPLEMENTED", "NOT_IMPLEMENTED"},
		}
		for _, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fake := newFakeCatalog()
				fake.bindings["openamsm.smf.mas.sc.query"] = makeUnavailable(fake.bindings["openamsm.smf.mas.sc.query"], testCase.unavailable)
				h := newHarness(t, fixedRuntimeFactory(fake))
				h.mustInitialize(t, ProtocolV2, "init-invoke-unavailable")
				detail := h.invoke(t, ProtocolV2, "invoke-unavailable", standardArguments()).expectError(t, ProtocolV2, "invoke-unavailable", CodeBackendOperationUnsupported)
				if got := requireString(t, "reason detail", detail["reason"]); got != testCase.wantReason {
					t.Fatalf("unavailable reason = %q, want %q", got, testCase.wantReason)
				}
				if fake.callCount() != 0 {
					t.Fatalf("unavailable operation reached the backend %d times, want 0", fake.callCount())
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})
}

// TestInvokeArgumentValidation covers typed argument validation before any
// backend dispatch. The fake must never be called for invalid arguments.
func TestInvokeArgumentValidation(t *testing.T) {
	cases := []struct {
		name         string
		arguments    map[string]any
		wantCode     string
		wantArgument string
	}{
		{"unknown_argument", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": 5, "extra": true}, CodeBackendArgumentsInvalid, ""},
		{"missing_required_supi", map[string]any{"pduSessionId": 5}, CodeBackendArgumentsInvalid, "supi"},
		{"missing_required_pdu_session_id", map[string]any{"supi": "imsi-001010000000001"}, CodeBackendArgumentsInvalid, "pduSessionId"},
		{"supi_wrong_type", map[string]any{"supi": 42, "pduSessionId": 5}, CodeBackendArgumentsInvalid, "supi"},
		{"supi_pattern_mismatch", map[string]any{"supi": "001010000000001", "pduSessionId": 5}, CodeBackendArgumentsInvalid, "supi"},
		{"supi_empty", map[string]any{"supi": "", "pduSessionId": 5}, CodeBackendArgumentsInvalid, "supi"},
		{"pdu_session_id_wrong_type_string", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": "5"}, CodeBackendArgumentsInvalid, "pduSessionId"},
		{"pdu_session_id_wrong_type_float", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": 1.5}, CodeBackendArgumentsInvalid, "pduSessionId"},
		{"pdu_session_id_wrong_type_boolean", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": true}, CodeBackendArgumentsInvalid, "pduSessionId"},
		{"pdu_session_id_below_minimum", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": -1}, CodeBackendArgumentsInvalid, "pduSessionId"},
		{"pdu_session_id_above_maximum", map[string]any{"supi": "imsi-001010000000001", "pduSessionId": 256}, CodeBackendArgumentsInvalid, "pduSessionId"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeCatalog()
			h := newHarness(t, fixedRuntimeFactory(fake))
			h.mustInitialize(t, ProtocolV2, "init-arg-invalid")
			h.invokeOperation(t, ProtocolV2, "invoke-arg-invalid", "openamsm.smf.mas.sc.query", testCase.arguments, futureDeadline())
			detail := h.await(t, "invoke-arg-invalid").expectError(t, ProtocolV2, "invoke-arg-invalid", testCase.wantCode)
			if testCase.wantArgument != "" {
				if got := requireString(t, "argument detail", detail["argument"]); got != testCase.wantArgument {
					t.Fatalf("invalid argument detail = %q, want %q", got, testCase.wantArgument)
				}
			}
			if fake.callCount() != 0 {
				t.Fatalf("invalid arguments reached the backend %d times, want 0", fake.callCount())
			}
			h.closeInput(t)
			h.waitServe(t, nil)
		})
	}

	// arguments not an object cannot be passed through the typed helper.
	t.Run("arguments_not_object_frame", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-arg-not-object")
		params := invokeParams("openamsm.smf.mas.sc.query", nil, "runtime-invocation-arg", "producer-arg", futureDeadline())
		params["arguments"] = []any{"imsi-001010000000001", 5}
		h.send(t, wireRequest(ProtocolV2, "invoke-arg-not-object", "invoke", params))
		h.await(t, "invoke-arg-not-object").expectError(t, ProtocolV2, "invoke-arg-not-object", CodeBackendArgumentsInvalid)
		if fake.callCount() != 0 {
			t.Fatalf("invalid arguments reached the backend %d times, want 0", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("boolean_argument_type", func(t *testing.T) {
		fake := newFakeCatalog()
		scID := "openamsm.smf.mas.sc.query"
		binding := fake.bindings[scID]
		binding.Arguments = append(binding.Arguments, Argument{Name: "strict", Type: "boolean", Required: false})
		fake.bindings[scID] = binding
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-arg-boolean")

		validArguments := standardArguments()
		validArguments["strict"] = true
		h.invokeOperation(t, ProtocolV2, "invoke-arg-boolean-ok", scID, validArguments, futureDeadline())
		h.await(t, "invoke-arg-boolean-ok").expectOK(t, ProtocolV2, "invoke-arg-boolean-ok")
		invocation := fake.onlyInvocation(t)
		if strict, ok := invocation.Arguments["strict"].(bool); !ok || !strict {
			t.Fatalf("boolean argument = %#v, want true", invocation.Arguments["strict"])
		}

		invalidArguments := standardArguments()
		invalidArguments["strict"] = "true"
		h.invokeOperation(t, ProtocolV2, "invoke-arg-boolean-bad", scID, invalidArguments, futureDeadline())
		detail := h.await(t, "invoke-arg-boolean-bad").expectError(t, ProtocolV2, "invoke-arg-boolean-bad", CodeBackendArgumentsInvalid)
		if got := requireString(t, "argument detail", detail["argument"]); got != "strict" {
			t.Fatalf("invalid boolean argument detail = %q, want strict", got)
		}
		if fake.callCount() != 1 {
			t.Fatalf("backend calls = %d, want 1 (the invalid boolean must not dispatch)", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// TestInvokeMetadataValidation covers invoke frame/envelope and invocation
// metadata validation.
func TestInvokeMetadataValidation(t *testing.T) {
	validArguments := standardArguments()
	longID := strings.Repeat("c", 129)
	cases := []struct {
		name     string
		mutate   func(params map[string]any)
		wantCode string
	}{
		{"unknown_params_member", func(params map[string]any) { params["extra"] = true }, CodeProtocolFrameInvalid},
		{"missing_backend_operation_id", func(params map[string]any) { delete(params, "backendOperationId") }, CodeProtocolFrameInvalid},
		{"missing_arguments", func(params map[string]any) { delete(params, "arguments") }, CodeProtocolFrameInvalid},
		{"missing_invocation", func(params map[string]any) { delete(params, "invocation") }, CodeProtocolFrameInvalid},
		{"empty_backend_operation_id", func(params map[string]any) { params["backendOperationId"] = "" }, CodeProtocolFrameInvalid},
		{"backend_operation_id_with_space", func(params map[string]any) { params["backendOperationId"] = "openamsm smf" }, CodeProtocolFrameInvalid},
		{"backend_operation_id_too_long", func(params map[string]any) { params["backendOperationId"] = longID }, CodeProtocolFrameInvalid},
		{"invocation_not_object", func(params map[string]any) { params["invocation"] = "not-an-object" }, CodeProtocolFrameInvalid},
		{"invocation_unknown_member", func(params map[string]any) {
			params["invocation"].(map[string]any)["extra"] = true
		}, CodeProtocolFrameInvalid},
		{"missing_invocation_id", func(params map[string]any) {
			delete(params["invocation"].(map[string]any), "invocationId")
		}, CodeProtocolFrameInvalid},
		{"missing_producer_invocation_id", func(params map[string]any) {
			delete(params["invocation"].(map[string]any), "producerInvocationId")
		}, CodeProtocolFrameInvalid},
		{"missing_deadline", func(params map[string]any) {
			delete(params["invocation"].(map[string]any), "deadline")
		}, CodeProtocolFrameInvalid},
		{"empty_invocation_id", func(params map[string]any) {
			params["invocation"].(map[string]any)["invocationId"] = ""
		}, CodeProtocolFrameInvalid},
		{"invocation_id_with_space", func(params map[string]any) {
			params["invocation"].(map[string]any)["invocationId"] = "runtime invocation"
		}, CodeProtocolFrameInvalid},
		{"invocation_id_too_long", func(params map[string]any) {
			params["invocation"].(map[string]any)["invocationId"] = longID
		}, CodeProtocolFrameInvalid},
		{"empty_producer_invocation_id", func(params map[string]any) {
			params["invocation"].(map[string]any)["producerInvocationId"] = ""
		}, CodeProtocolFrameInvalid},
		{"producer_invocation_id_too_long", func(params map[string]any) {
			params["invocation"].(map[string]any)["producerInvocationId"] = longID
		}, CodeProtocolFrameInvalid},
		{"deadline_with_offset", func(params map[string]any) {
			params["invocation"].(map[string]any)["deadline"] = "2026-11-17T09:30:00+08:00"
		}, CodeProtocolFrameInvalid},
		{"deadline_lowercase_z", func(params map[string]any) {
			params["invocation"].(map[string]any)["deadline"] = "2026-11-17T09:30:00z"
		}, CodeProtocolFrameInvalid},
		{"deadline_bad_format", func(params map[string]any) {
			params["invocation"].(map[string]any)["deadline"] = "not-a-timestamp"
		}, CodeProtocolFrameInvalid},
		{"deadline_empty", func(params map[string]any) {
			params["invocation"].(map[string]any)["deadline"] = ""
		}, CodeProtocolFrameInvalid},
		{"deadline_date_only", func(params map[string]any) {
			params["invocation"].(map[string]any)["deadline"] = "2026-11-17"
		}, CodeProtocolFrameInvalid},
	}
	for index, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeCatalog()
			h := newHarness(t, fixedRuntimeFactory(fake))
			h.mustInitialize(t, ProtocolV2, "init-meta-invalid")
			params := invokeParams("openamsm.smf.mas.sc.query", validArguments, "runtime-invocation-01", "producer-01", futureDeadline())
			testCase.mutate(params)
			requestID := fmt.Sprintf("invoke-meta-bad-%d", index)
			h.send(t, wireRequest(ProtocolV2, requestID, "invoke", params))
			h.await(t, requestID).expectError(t, ProtocolV2, requestID, testCase.wantCode)
			if fake.callCount() != 0 {
				t.Fatalf("invalid metadata reached the backend %d times, want 0", fake.callCount())
			}
			h.closeInput(t)
			h.waitServe(t, nil)
		})
	}
}

// TestInvokeDeadline covers absolute-deadline and max-call-duration bounds.
func TestInvokeDeadline(t *testing.T) {
	t.Run("deadline_already_past_does_not_invoke", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-deadline-past")

		frame := h.invokeWithDeadline(t, ProtocolV2, "invoke-deadline-past", standardArguments(), pastDeadline())
		frame.expectError(t, ProtocolV2, "invoke-deadline-past", CodeCallDeadlineExceeded)
		if fake.callCount() != 0 {
			t.Fatalf("expired deadline reached the backend %d times, want 0", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("injected_now_bounds_deadline", func(t *testing.T) {
		fake := newFakeCatalog()
		base := time.Now().UTC()
		h := newHarnessWithOptions(t, Options{
			Info:       defaultInfo(),
			NewRuntime: fixedRuntimeFactory(fake),
			// The Provider believes it is an hour later than the deadline.
			Now: func() time.Time { return base.Add(time.Hour) },
		})
		h.mustInitialize(t, ProtocolV2, "init-deadline-now")
		deadline := base.Add(30 * time.Minute).UTC().Format(time.RFC3339)
		frame := h.invokeWithDeadline(t, ProtocolV2, "invoke-deadline-now", standardArguments(), deadline)
		frame.expectError(t, ProtocolV2, "invoke-deadline-now", CodeCallDeadlineExceeded)
		if fake.callCount() != 0 {
			t.Fatalf("injected-now expired deadline reached the backend %d times, want 0", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("max_call_duration_bounds_slow_backend", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(ctx context.Context, _ string, _ map[string]any) (Result, error) {
			<-ctx.Done()
			return Result{}, ctx.Err()
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitializeWithLimits(t, ProtocolV2, "init-deadline-max-duration", defaultMaxFrameBytes, defaultMaxInFlight, 50)

		startedAt := time.Now()
		frame := h.invokeWithDeadline(t, ProtocolV2, "invoke-slow", standardArguments(), futureDeadline())
		frame.expectError(t, ProtocolV2, "invoke-slow", CodeCallDeadlineExceeded)
		elapsed := time.Since(startedAt)
		if elapsed < 40*time.Millisecond {
			t.Fatalf("slow invoke completed after %s, want the 50ms maxCallDuration bound to apply", elapsed)
		}
		if elapsed > 3*time.Second {
			t.Fatalf("slow invoke completed after %s, far beyond the 50ms bound", elapsed)
		}
		if fake.callCount() != 1 {
			t.Fatalf("slow invoke reached the backend %d times, want 1", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// TestInvokeCancellation covers the cancel method and exactly-once completion.
func TestInvokeCancellation(t *testing.T) {
	t.Run("cancel_in_flight_invoke", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 4)
		fake.invokeFn = blockingInvoke(started)
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-cancel")

		h.invokeOperation(t, ProtocolV2, "invoke-cancel-target", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)

		h.expectCancelDisposition(t, ProtocolV2, "cancel-request", "invoke-cancel-target", "CANCEL_REQUESTED")
		original := h.await(t, "invoke-cancel-target")
		original.expectError(t, ProtocolV2, "invoke-cancel-target", CodeCallCancelled)

		// The cancel acknowledgment is not the terminal response, and the
		// original request receives exactly one terminal response.
		if frames := h.framesFor("invoke-cancel-target"); len(frames) != 1 {
			t.Fatalf("original invoke received %d terminal responses, want exactly 1", len(frames))
		}

		// A second cancel of the now-completed invocation reports ALREADY_COMPLETED.
		h.expectCancelDisposition(t, ProtocolV2, "cancel-request-2", "invoke-cancel-target", "ALREADY_COMPLETED")

		// The process stays usable.
		fake.invokeFn = nil
		result := h.invoke(t, ProtocolV2, "invoke-after-cancel", standardArguments()).expectOK(t, ProtocolV2, "invoke-after-cancel")
		requireExactKeys(t, "invoke result", result, "httpStatus", "response")

		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("cancel_queued_invocation_never_dispatches", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 2)
		release := make(chan struct{})
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			started <- operationID
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			return Result{HTTPStatus: 200, Response: map[string]any{"ok": true}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitializeWithLimits(t, ProtocolV2, "init-cancel-queued", defaultMaxFrameBytes, 1, defaultMaxCallDuration)

		// The first invoke occupies the single in-flight slot.
		h.invokeOperation(t, ProtocolV2, "invoke-occupies-slot", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)
		// The second invoke is accepted but queued behind the semaphore.
		h.invokeOperation(t, ProtocolV2, "invoke-queued", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		h.expectCancelDisposition(t, ProtocolV2, "cancel-queued", "invoke-queued", "CANCEL_REQUESTED")

		queued := h.await(t, "invoke-queued")
		queued.expectError(t, ProtocolV2, "invoke-queued", CodeCallCancelled)
		if fake.callCount() != 1 {
			t.Fatalf("backend calls = %d, want 1 (a cancelled queued invoke must never dispatch)", fake.callCount())
		}

		close(release)
		h.await(t, "invoke-occupies-slot").expectOK(t, ProtocolV2, "invoke-occupies-slot")
		if fake.callCount() != 1 {
			t.Fatalf("backend calls = %d, want 1", fake.callCount())
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("cancel_unknown_target", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-cancel-unknown")
		h.expectCancelDisposition(t, ProtocolV2, "cancel-unknown", "never-seen-request", "NOT_FOUND")
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("cancel_initialize_and_catalog_targets_are_not_found", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-cancel-init-target")
		h.catalog(t, ProtocolV2, "catalog-cancel-target", []string{allBindingIDs()[0]}).expectOK(t, ProtocolV2, "catalog-cancel-target")

		h.expectCancelDisposition(t, ProtocolV2, "cancel-init-target", "init-cancel-init-target", "NOT_FOUND")
		h.expectCancelDisposition(t, ProtocolV2, "cancel-catalog-target", "catalog-cancel-target", "NOT_FOUND")
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("cancel_invalid_params", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-cancel-invalid")

		longID := strings.Repeat("d", 129)
		cases := []struct {
			name     string
			params   map[string]any
			wantCode string
		}{
			{"missing_target", map[string]any{}, CodeProtocolFrameInvalid},
			{"unknown_member", map[string]any{"targetRequestId": "x", "extra": true}, CodeProtocolFrameInvalid},
			{"target_not_string", map[string]any{"targetRequestId": 7}, CodeProtocolRequestIDInvalid},
			{"empty_target", map[string]any{"targetRequestId": ""}, CodeProtocolRequestIDInvalid},
			{"target_too_long", map[string]any{"targetRequestId": longID}, CodeProtocolRequestIDInvalid},
			{"target_with_space", map[string]any{"targetRequestId": "target id"}, CodeProtocolRequestIDInvalid},
		}
		for index, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				requestID := fmt.Sprintf("cancel-bad-%d", index)
				h.send(t, wireRequest(ProtocolV2, requestID, "cancel", testCase.params))
				h.await(t, requestID).expectError(t, ProtocolV2, requestID, testCase.wantCode)
			})
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// TestConcurrency covers bounded in-flight dispatch, out-of-order completion,
// fault isolation and duplicate request IDs.
func TestConcurrency(t *testing.T) {
	t.Run("max_in_flight_is_enforced", func(t *testing.T) {
		const invokeCount = 6
		fake := newFakeCatalog()
		releases := make(map[string]chan struct{}, invokeCount)
		for index := 0; index < invokeCount; index++ {
			releases[supiFor(index)] = make(chan struct{})
		}
		var active atomic.Int64
		var maxActive atomic.Int64
		started := make(chan string, invokeCount)
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			defer active.Add(-1)
			supi, _ := arguments["supi"].(string)
			started <- supi
			select {
			case <-releases[supi]:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			return Result{HTTPStatus: 200, Response: map[string]any{"supi": supi}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitializeWithLimits(t, ProtocolV2, "init-concurrency", defaultMaxFrameBytes, 2, defaultMaxCallDuration)

		requestIDs := make([]string, invokeCount)
		for index := 0; index < invokeCount; index++ {
			requestIDs[index] = fmt.Sprintf("invoke-concurrent-%d", index)
			arguments := map[string]any{"supi": supiFor(index), "pduSessionId": index}
			h.invokeOperation(t, ProtocolV2, requestIDs[index], "openamsm.smf.mas.sc.query", arguments, futureDeadline())
		}

		startOrder := make([]string, 0, invokeCount)
		startOrder = append(startOrder, waitStarted(t, started), waitStarted(t, started))
		if got := active.Load(); got != 2 {
			t.Fatalf("active backend calls = %d after two starts, want 2", got)
		}
		assertNoStart(t, started, 100*time.Millisecond)

		// Release in reverse order; each release admits exactly one queued call.
		for index := 1; index >= 0; index-- {
			close(releases[startOrder[index]])
		}
		startOrder = append(startOrder, waitStarted(t, started), waitStarted(t, started))
		assertNoStart(t, started, 100*time.Millisecond)
		for index := 3; index >= 2; index-- {
			close(releases[startOrder[index]])
		}
		startOrder = append(startOrder, waitStarted(t, started), waitStarted(t, started))
		for index := 5; index >= 4; index-- {
			close(releases[startOrder[index]])
		}

		if got := maxActive.Load(); got > 2 {
			t.Fatalf("observed %d concurrent backend calls, want at most 2", got)
		}
		if got := maxActive.Load(); got != 2 {
			t.Fatalf("observed at most %d concurrent backend calls, want the bound to be exercised (2)", got)
		}

		// Every invoke receives exactly one terminal response, correlated by
		// requestId; completion order is not assumed.
		seenSupi := make(map[string]struct{}, invokeCount)
		for index, requestID := range requestIDs {
			result := h.await(t, requestID).expectOK(t, ProtocolV2, requestID)
			requireExactKeys(t, "invoke result", result, "httpStatus", "response")
			if got := requireNumber(t, "httpStatus", result["httpStatus"]); got != 200 {
				t.Fatalf("httpStatus = %d, want 200", got)
			}
			response, ok := result["response"].(map[string]any)
			if !ok {
				t.Fatalf("response = %#v, want object", result["response"])
			}
			supi := requireString(t, "response.supi", response["supi"])
			if supi != supiFor(index) {
				t.Fatalf("response supi = %q, want %q", supi, supiFor(index))
			}
			seenSupi[supi] = struct{}{}
			if frames := h.framesFor(requestID); len(frames) != 1 {
				t.Fatalf("request %q received %d terminal responses, want 1", requestID, len(frames))
			}
		}
		if len(seenSupi) != invokeCount {
			t.Fatalf("correlated %d distinct responses, want %d", len(seenSupi), invokeCount)
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("duplicate_request_id_while_running", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 2)
		release := make(chan struct{})
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			started <- operationID
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			return Result{HTTPStatus: 200, Response: map[string]any{"ok": true}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitializeWithLimits(t, ProtocolV2, "init-duplicate-id", defaultMaxFrameBytes, 2, defaultMaxCallDuration)

		h.invokeOperation(t, ProtocolV2, "duplicate-invoke", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)
		h.invokeOperation(t, ProtocolV2, "duplicate-invoke", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		h.await(t, "duplicate-invoke").expectError(t, ProtocolV2, "duplicate-invoke", CodeProtocolRequestIDInvalid)

		close(release)
		// The first (accepted) invocation must still complete exactly once.
		result := h.await(t, "duplicate-invoke").expectOK(t, ProtocolV2, "duplicate-invoke")
		requireExactKeys(t, "invoke result", result, "httpStatus", "response")
		frames := h.framesFor("duplicate-invoke")
		successes := 0
		rejections := 0
		for _, frame := range frames {
			if frame.ok {
				successes++
			} else if frame.errorCode == CodeProtocolRequestIDInvalid {
				rejections++
			}
		}
		if successes != 1 || rejections != 1 {
			t.Fatalf("duplicate request produced %d successes and %d id-invalid rejections, want 1 and 1", successes, rejections)
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("fault_does_not_terminate_process", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(_ context.Context, operationID string, _ map[string]any) (Result, error) {
			if operationID == "openamsm.smf.mas.sc.query" {
				return Result{}, &Fault{Code: CodeRemoteTransportError}
			}
			return Result{HTTPStatus: 200, Response: map[string]any{"ok": true}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-fault-isolation")
		h.invoke(t, ProtocolV2, "invoke-fault-isolated", standardArguments()).expectError(t, ProtocolV2, "invoke-fault-isolated", CodeRemoteTransportError)
		if !h.serveReturned() {
			// still alive
		}
		h.invokeOperation(t, ProtocolV2, "invoke-after-fault", "openamsm.smf.mas.pa.query", standardArguments(), futureDeadline())
		result := h.await(t, "invoke-after-fault").expectOK(t, ProtocolV2, "invoke-after-fault")
		requireExactKeys(t, "invoke result", result, "httpStatus", "response")
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// TestLifecycle covers EOF drain, context cancellation and runtime Close.
func TestLifecycle(t *testing.T) {
	t.Run("eof_without_in_flight_returns_nil", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-eof-clean")
		h.closeInput(t)
		h.waitServe(t, nil)
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
		}
		if frames := h.allFrames(); len(frames) != 1 {
			t.Fatalf("stdout emitted %d frames, want only the initialize response", len(frames))
		}
		h.assertStdoutProtocolOnly(t)

		// No output is written after Serve returns.
		before := len(h.sink.rawBytes())
		time.Sleep(20 * time.Millisecond)
		if after := len(h.sink.rawBytes()); after != before {
			t.Fatalf("stdout grew by %d bytes after Serve returned", after-before)
		}
	})

	t.Run("eof_drains_in_flight_invoke", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 2)
		release := make(chan struct{})
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			started <- operationID
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			return Result{HTTPStatus: 200, Response: map[string]any{"ok": true}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-eof-drain")
		h.invokeOperation(t, ProtocolV2, "invoke-draining", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)
		h.closeInput(t)

		select {
		case <-h.serveDone:
			t.Fatal("Serve returned before the in-flight invoke completed")
		case <-time.After(100 * time.Millisecond):
		}
		close(release)

		result := h.await(t, "invoke-draining").expectOK(t, ProtocolV2, "invoke-draining")
		requireExactKeys(t, "invoke result", result, "httpStatus", "response")
		h.waitServe(t, nil)
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
		}
		h.assertStdoutProtocolOnly(t)
		before := len(h.sink.rawBytes())
		time.Sleep(20 * time.Millisecond)
		if after := len(h.sink.rawBytes()); after != before {
			t.Fatalf("stdout grew by %d bytes after Serve returned", after-before)
		}
	})

	t.Run("context_cancellation_aborts_in_flight_and_returns_canceled", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 2)
		cancelled := make(chan struct{}, 2)
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			started <- operationID
			<-ctx.Done()
			cancelled <- struct{}{}
			return Result{}, ctx.Err()
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-ctx-cancel")
		h.invokeOperation(t, ProtocolV2, "invoke-ctx-cancel", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)
		h.cancelFunc()
		h.waitServe(t, context.Canceled)
		select {
		case <-cancelled:
		case <-time.After(awaitTimeout):
			t.Fatal("in-flight backend call did not observe context cancellation")
		}
		frame := h.await(t, "invoke-ctx-cancel")
		frame.expectError(t, ProtocolV2, "invoke-ctx-cancel", CodeCallCancelled)
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
		}
	})

	t.Run("context_cancellation_without_in_flight_closes_runtime", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-ctx-cancel-idle")
		h.cancelFunc()
		h.waitServe(t, context.Canceled)
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
		}
		if frames := h.allFrames(); len(frames) != 1 {
			t.Fatalf("stdout emitted %d frames, want only the initialize response", len(frames))
		}
	})

	t.Run("runtime_close_exactly_once_when_both_paths_race", func(t *testing.T) {
		fake := newFakeCatalog()
		started := make(chan string, 2)
		fake.invokeFn = func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
			started <- operationID
			<-ctx.Done()
			return Result{}, ctx.Err()
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-close-race")
		h.invokeOperation(t, ProtocolV2, "invoke-close-race", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		waitStarted(t, started)
		// Cancel the process context and close stdin at nearly the same time.
		h.cancelFunc()
		h.closeInput(t)
		select {
		case <-h.serveDone:
		case <-time.After(awaitTimeout):
			t.Fatal("Serve did not return after cancellation")
		}
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
		}
	})

	t.Run("stdout_write_failure_terminates_process", func(t *testing.T) {
		fake := newFakeCatalog()
		inReader, inWriter := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		go func() {
			errCh <- Serve(ctx, Options{
				In:         inReader,
				Out:        failingWriter{err: errors.New("stdout unavailable")},
				Err:        io.Discard,
				Info:       defaultInfo(),
				NewRuntime: fixedRuntimeFactory(fake),
			})
		}()
		if _, err := io.WriteString(inWriter, wireRequest(ProtocolV2, "init-write-failure", "initialize", defaultInitParams())+"\n"); err != nil {
			t.Fatalf("write initialize frame: %v", err)
		}
		_ = inWriter.Close()
		select {
		case err := <-errCh:
			if err == nil {
				t.Fatal("Serve returned nil after a stdout write failure")
			}
		case <-time.After(awaitTimeout):
			t.Fatal("Serve did not return after a stdout write failure")
		}
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times after write failure, want exactly 1", fake.closeCount())
		}
	})
}

// failingWriter always fails, simulating a closed or broken stdout.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestServeOptionsValidation covers the defensive Options validation.
func TestServeOptionsValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		options Options
	}{
		{"missing_input", Options{Out: io.Discard, NewRuntime: fixedRuntimeFactory(newFakeCatalog())}},
		{"missing_output", Options{In: strings.NewReader(""), NewRuntime: fixedRuntimeFactory(newFakeCatalog())}},
		{"missing_runtime_factory", Options{In: strings.NewReader(""), Out: io.Discard}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if err := Serve(ctx, testCase.options); err == nil {
				t.Fatal("Serve accepted an incomplete Options value")
			}
		})
	}
}

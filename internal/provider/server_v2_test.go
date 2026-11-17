package provider

// Tests for the additive Provider protocol v2 extension: version-specific
// catalog metadata, argument descriptors, declared-argument validation with
// trusted catalog defaults, and the unchanged invoke result shapes.

import (
	"context"
	"fmt"
	"testing"
)

// listOperationID is one of the three documented v2 list bindings.
const listOperationID = "openamsm.amf.ue-info.list"

// requireNoForbiddenJSONKeys proves a decoded JSON value never carries private
// routing/credential metadata.
func requireNoForbiddenJSONKeys(t *testing.T, what string, value any, forbidden []string) {
	t.Helper()
	for _, key := range forbidden {
		if containsJSONKey(value, key) {
			t.Fatalf("%s leaks forbidden field %q", what, key)
		}
	}
}

// TestProtocolV2Initialize covers the v2 handshake result difference.
func TestProtocolV2Initialize(t *testing.T) {
	fake := newFakeCatalog()
	factory := &runtimeFactory{build: func(string) (Runtime, error) { return fake, nil }}
	h := newHarness(t, factory.newRuntime)

	result := h.mustInitialize(t, ProtocolV2, "init-v2")
	requireExactKeys(t, "v2 initialize result", result,
		"providerId", "providerVersion", "buildDigest", "backendBindingCatalogDigest", "limits", "catalogVersion")
	if got := requireString(t, "catalogVersion", result["catalogVersion"]); got != testCatalogVersion {
		t.Fatalf("catalogVersion = %q, want %q", got, testCatalogVersion)
	}
	if versions := factory.recordedVersions(); len(versions) != 1 || versions[0] != ProtocolV2 {
		t.Fatalf("NewRuntime protocol versions = %v, want [%q]", versions, ProtocolV2)
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

// TestProtocolV2Catalog covers the v2 catalog result: catalogVersion and the
// full binding descriptors for all 11 documented read-only bindings.
func TestProtocolV2Catalog(t *testing.T) {
	fake := newFakeCatalog()
	h := newHarness(t, fixedRuntimeFactory(fake))
	h.mustInitialize(t, ProtocolV2, "init-v2-catalog")

	requested := allBindingIDs()
	result := h.catalog(t, ProtocolV2, "catalog-v2-all", requested).expectOK(t, ProtocolV2, "catalog-v2-all")
	requireExactKeys(t, "v2 catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")
	if got := requireString(t, "catalogVersion", result["catalogVersion"]); got != testCatalogVersion {
		t.Fatalf("catalogVersion = %q, want %q", got, testCatalogVersion)
	}
	supported := requireStringSlice(t, "supported", result["supported"])
	requireEqualStrings(t, "supported", supported, requested)
	if unsupported := requireObjectSlice(t, "unsupported", result["unsupported"]); len(unsupported) != 0 {
		t.Fatalf("unsupported = %v, want empty", unsupported)
	}

	descriptors := requireObjectSlice(t, "bindings", result["bindings"])
	if len(descriptors) != len(requested) {
		t.Fatalf("bindings has %d descriptors, want %d", len(descriptors), len(requested))
	}
	forbidden := []string{
		"origin", "url", "method", "path", "token", "header", "serviceKey",
		"credential", "secret", "authorization", "handlerProfile", "callableId",
		"capabilityId", "outcome", "evidence",
	}
	allowedArgumentKeys := map[string]struct{}{
		"name": {}, "type": {}, "required": {}, "minimum": {}, "maximum": {}, "default": {},
	}
	for index, operationID := range requested {
		descriptor := descriptors[index]
		requireExactKeys(t, "binding descriptor", descriptor,
			"backendOperationId", "effect", "request", "response", "arguments", "source")
		if got := requireString(t, "backendOperationId", descriptor["backendOperationId"]); got != operationID {
			t.Fatalf("bindings[%d].backendOperationId = %q, want %q (order must match supported)", index, got, operationID)
		}
		if got := requireString(t, "effect", descriptor["effect"]); got != "READ" {
			t.Fatalf("bindings[%d].effect = %q, want READ", index, got)
		}
		wantBinding := testBinding(operationID)

		request, ok := descriptor["request"].(map[string]any)
		if !ok {
			t.Fatalf("bindings[%d].request = %#v, want object", index, descriptor["request"])
		}
		requireExactKeys(t, "binding request", request, "mediaType")
		if got := requireString(t, "request.mediaType", request["mediaType"]); got != wantBinding.RequestMediaType {
			t.Fatalf("bindings[%d].request.mediaType = %q, want %q", index, got, wantBinding.RequestMediaType)
		}

		response, ok := descriptor["response"].(map[string]any)
		if !ok {
			t.Fatalf("bindings[%d].response = %#v, want object", index, descriptor["response"])
		}
		requireExactKeys(t, "binding response", response, "mediaTypes")
		mediaTypes := requireStringSlice(t, "response.mediaTypes", response["mediaTypes"])
		requireEqualStrings(t, "response.mediaTypes", mediaTypes, wantBinding.ResponseMediaTypes)

		source, ok := descriptor["source"].(map[string]any)
		if !ok {
			t.Fatalf("bindings[%d].source = %#v, want object", index, descriptor["source"])
		}
		requireExactKeys(t, "binding source", source, "sourceId", "operationId")
		if got := requireString(t, "source.sourceId", source["sourceId"]); got != wantBinding.SourceID {
			t.Fatalf("bindings[%d].source.sourceId = %q, want %q", index, got, wantBinding.SourceID)
		}
		if got := requireString(t, "source.operationId", source["operationId"]); got != wantBinding.SourceOperationID {
			t.Fatalf("bindings[%d].source.operationId = %q, want %q", index, got, wantBinding.SourceOperationID)
		}

		arguments := requireObjectSlice(t, "binding arguments", descriptor["arguments"])
		if len(arguments) != len(wantBinding.Arguments) {
			t.Fatalf("bindings[%d] declares %d arguments, want %d", index, len(arguments), len(wantBinding.Arguments))
		}
		for argumentIndex, wantArgument := range wantBinding.Arguments {
			argument := arguments[argumentIndex]
			for key := range argument {
				if _, allowed := allowedArgumentKeys[key]; !allowed {
					t.Fatalf("bindings[%d].arguments[%d] has unknown descriptor field %q", index, argumentIndex, key)
				}
			}
			if got := requireString(t, "argument.name", argument["name"]); got != wantArgument.Name {
				t.Fatalf("bindings[%d].arguments[%d].name = %q, want %q", index, argumentIndex, got, wantArgument.Name)
			}
			if got := requireString(t, "argument.type", argument["type"]); got != wantArgument.Type {
				t.Fatalf("bindings[%d].arguments[%d].type = %q, want %q", index, argumentIndex, got, wantArgument.Type)
			}
			required, ok := argument["required"].(bool)
			if !ok || required != wantArgument.Required {
				t.Fatalf("bindings[%d].arguments[%d].required = %#v, want %v", index, argumentIndex, argument["required"], wantArgument.Required)
			}
			if wantArgument.Minimum != nil {
				if got := requireNumber(t, "argument.minimum", argument["minimum"]); got != *wantArgument.Minimum {
					t.Fatalf("bindings[%d].arguments[%d].minimum = %d, want %d", index, argumentIndex, got, *wantArgument.Minimum)
				}
			} else if _, present := argument["minimum"]; present {
				t.Fatalf("bindings[%d].arguments[%d] declares an undeclared minimum", index, argumentIndex)
			}
			if wantArgument.Maximum != nil {
				if got := requireNumber(t, "argument.maximum", argument["maximum"]); got != *wantArgument.Maximum {
					t.Fatalf("bindings[%d].arguments[%d].maximum = %d, want %d", index, argumentIndex, got, *wantArgument.Maximum)
				}
			} else if _, present := argument["maximum"]; present {
				t.Fatalf("bindings[%d].arguments[%d] declares an undeclared maximum", index, argumentIndex)
			}
			if wantArgument.Default != nil {
				if _, present := argument["default"]; !present {
					t.Fatalf("bindings[%d].arguments[%d] is missing catalog default", index, argumentIndex)
				}
			}
		}
		requireNoForbiddenJSONKeys(t, "catalog binding descriptor", descriptor, forbidden)
	}
	requireNoForbiddenJSONKeys(t, "v2 catalog result", result, forbidden)
	h.closeInput(t)
	h.waitServe(t, nil)
}

// TestProtocolV2CatalogUnavailable covers partition behavior in v2: descriptors
// exist only for supported bindings, unavailable/unknown IDs are listed.
func TestProtocolV2CatalogUnavailable(t *testing.T) {
	fake := newFakeCatalog()
	fake.bindings["openamsm.smf.mas.iam.query"] = makeUnavailable(fake.bindings["openamsm.smf.mas.iam.query"], "NOT_CONFIGURED")
	h := newHarness(t, fixedRuntimeFactory(fake))
	h.mustInitialize(t, ProtocolV2, "init-v2-catalog-unavailable")

	requested := []string{"openamsm.smf.mas.sc.query", "openamsm.smf.mas.iam.query", "openamsm.smf.mas.unknown.query"}
	result := h.catalog(t, ProtocolV2, "catalog-v2-unavailable", requested).expectOK(t, ProtocolV2, "catalog-v2-unavailable")
	requireExactKeys(t, "v2 catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")
	supported := requireStringSlice(t, "supported", result["supported"])
	requireEqualStrings(t, "supported", supported, []string{"openamsm.smf.mas.sc.query"})
	unsupported := requireObjectSlice(t, "unsupported", result["unsupported"])
	if len(unsupported) != 2 {
		t.Fatalf("unsupported = %v, want 2 entries", unsupported)
	}
	if got := requireString(t, "unsupported[0].backendOperationId", unsupported[0]["backendOperationId"]); got != "openamsm.smf.mas.iam.query" {
		t.Fatalf("unsupported[0] = %q, want iam", got)
	}
	if got := requireString(t, "unsupported[0].reason", unsupported[0]["reason"]); got != "NOT_CONFIGURED" {
		t.Fatalf("unsupported[0].reason = %q, want NOT_CONFIGURED", got)
	}
	if got := requireString(t, "unsupported[1].reason", unsupported[1]["reason"]); got != "NOT_IMPLEMENTED" {
		t.Fatalf("unsupported[1].reason = %q, want NOT_IMPLEMENTED", got)
	}
	descriptors := requireObjectSlice(t, "bindings", result["bindings"])
	if len(descriptors) != 1 {
		t.Fatalf("bindings has %d descriptors, want 1 (only supported bindings are described)", len(descriptors))
	}
	if got := requireString(t, "bindings[0].backendOperationId", descriptors[0]["backendOperationId"]); got != "openamsm.smf.mas.sc.query" {
		t.Fatalf("bindings[0].backendOperationId = %q, want sc", got)
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

// TestProtocolVersionMismatch covers strict version locking after initialize.
func TestProtocolVersionMismatch(t *testing.T) {
	t.Run("v1_frame_after_v2_initialize", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-v2-lock")

		// The response version is the negotiated version, not the rejected one.
		h.invoke(t, ProtocolV1, "invoke-v1-after-v2", standardArguments()).expectError(t, ProtocolV2, "invoke-v1-after-v2", CodeProtocolVersionUnsupported)
		h.catalog(t, ProtocolV1, "catalog-v1-after-v2", allBindingIDs()).expectError(t, ProtocolV2, "catalog-v1-after-v2", CodeProtocolVersionUnsupported)
		h.cancel(t, ProtocolV1, "cancel-v1-after-v2", "invoke-v1-after-v2").expectError(t, ProtocolV2, "cancel-v1-after-v2", CodeProtocolVersionUnsupported)

		// The negotiated v2 protocol still works.
		h.catalog(t, ProtocolV2, "catalog-v2-still-works", allBindingIDs()).expectOK(t, ProtocolV2, "catalog-v2-still-works")
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("v2_frame_after_v1_initialize", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV1, "init-v1-lock")

		h.invoke(t, ProtocolV2, "invoke-v2-after-v1", standardArguments()).expectError(t, ProtocolV1, "invoke-v2-after-v1", CodeProtocolVersionUnsupported)
		h.catalog(t, ProtocolV2, "catalog-v2-after-v1", allBindingIDs()).expectError(t, ProtocolV1, "catalog-v2-after-v1", CodeProtocolVersionUnsupported)

		h.catalog(t, ProtocolV1, "catalog-v1-still-works", allBindingIDs()).expectOK(t, ProtocolV1, "catalog-v1-still-works")
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("unknown_protocol_version", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		request := `{"protocolVersion":"ca-client-tools.provider/v3","type":"request","requestId":"init-v3","method":"initialize","params":{}}` + "\n"
		h.sendRaw(t, request)
		h.await(t, "init-v3").expectError(t, ProtocolV1, "init-v3", CodeProtocolVersionUnsupported)
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

// TestProtocolV2Invoke covers v2 invoke with the documented list argument
// profile (optional page/pageSize with trusted catalog defaults).
func TestProtocolV2Invoke(t *testing.T) {
	t.Run("success_and_result_shape", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(_ context.Context, operationID string, arguments map[string]any) (Result, error) {
			return Result{HTTPStatus: 200, Response: map[string]any{
				"operationId": operationID,
				"items":       []any{},
				"pager":       map[string]any{"page": arguments["page"], "pageSize": arguments["pageSize"]},
			}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-v2-invoke")
		result := h.invokeOperationV2(t, "invoke-v2-list", listOperationID, map[string]any{
			"page":     1,
			"pageSize": 50,
		})

		requireExactKeys(t, "v2 invoke result", result, "httpStatus", "response")
		if got := requireNumber(t, "httpStatus", result["httpStatus"]); got != 200 {
			t.Fatalf("httpStatus = %d, want 200", got)
		}
		response, ok := result["response"].(map[string]any)
		if !ok {
			t.Fatalf("response = %#v, want object", result["response"])
		}
		if got := requireString(t, "response.operationId", response["operationId"]); got != listOperationID {
			t.Fatalf("response.operationId = %q, want %q", got, listOperationID)
		}

		invocation := fake.onlyInvocation(t)
		if invocation.OperationID != listOperationID {
			t.Fatalf("backend operation = %q, want %q", invocation.OperationID, listOperationID)
		}
		requireExactKeys(t, "provider-validated arguments", invocation.Arguments, "page", "pageSize")
		if page, ok := invocation.Arguments["page"].(int64); !ok || page != 1 {
			t.Fatalf("page = %#v, want int64(1)", invocation.Arguments["page"])
		}
		if pageSize, ok := invocation.Arguments["pageSize"].(int64); !ok || pageSize != 50 {
			t.Fatalf("pageSize = %#v, want int64(50)", invocation.Arguments["pageSize"])
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("problem_result_shape_unchanged", func(t *testing.T) {
		fake := newFakeCatalog()
		fake.invokeFn = func(context.Context, string, map[string]any) (Result, error) {
			return Result{HTTPStatus: 404, Problem: &ProblemDetails{Status: 404, Cause: "SESSION_NOT_FOUND"}}, nil
		}
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-v2-problem")
		h.invokeOperation(t, ProtocolV2, "invoke-v2-problem", "openamsm.smf.mas.sc.query", standardArguments(), futureDeadline())
		result := h.await(t, "invoke-v2-problem").expectOK(t, ProtocolV2, "invoke-v2-problem")
		requireExactKeys(t, "v2 problem result", result, "httpStatus", "problem")
		if got := requireNumber(t, "httpStatus", result["httpStatus"]); got != 404 {
			t.Fatalf("httpStatus = %d, want 404", got)
		}
		problem, ok := result["problem"].(map[string]any)
		if !ok {
			t.Fatalf("problem = %#v, want object", result["problem"])
		}
		requireExactKeys(t, "problem", problem, "status", "cause")
		if got := requireString(t, "cause", problem["cause"]); got != "SESSION_NOT_FOUND" {
			t.Fatalf("cause = %q, want SESSION_NOT_FOUND", got)
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("declared_argument_validation", func(t *testing.T) {
		cases := []struct {
			name      string
			arguments map[string]any
			detailKey string
		}{
			{"unknown_argument", map[string]any{"page": 0, "unexpected": 1}, "unexpected"},
			{"page_wrong_type_string", map[string]any{"page": "0"}, "page"},
			{"page_wrong_type_boolean", map[string]any{"page": true}, "page"},
			{"page_below_minimum", map[string]any{"page": -1}, "page"},
			{"page_size_below_minimum", map[string]any{"pageSize": 0}, "pageSize"},
			{"page_size_above_maximum", map[string]any{"pageSize": 101}, "pageSize"},
			{"page_size_wrong_type_float", map[string]any{"pageSize": 1.5}, "pageSize"},
			{"page_size_wrong_type_string", map[string]any{"pageSize": "100"}, "pageSize"},
		}
		for index, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fake := newFakeCatalog()
				h := newHarness(t, fixedRuntimeFactory(fake))
				h.mustInitialize(t, ProtocolV2, "init-v2-arg-invalid")
				requestID := fmt.Sprintf("invoke-v2-arg-bad-%d", index)
				h.invokeOperation(t, ProtocolV2, requestID, listOperationID, testCase.arguments, futureDeadline())
				detail := h.await(t, requestID).expectError(t, ProtocolV2, requestID, CodeBackendArgumentsInvalid)
				if got := requireString(t, "argument detail", detail["argument"]); got != testCase.detailKey {
					t.Fatalf("invalid argument detail = %q, want %q", got, testCase.detailKey)
				}
				if fake.callCount() != 0 {
					t.Fatalf("invalid v2 arguments reached the backend %d times, want 0", fake.callCount())
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})

	t.Run("catalog_defaults_applied_to_absent_arguments", func(t *testing.T) {
		// Defaults are trusted catalog content applied by the Provider for
		// absent optional arguments (docs/provider-protocol-v2.md §5). The
		// hardened Runtime therefore always receives a complete argument set.
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-v2-defaults")
		result := h.invokeOperationV2(t, "invoke-v2-defaults", listOperationID, map[string]any{})
		requireExactKeys(t, "v2 invoke result", result, "httpStatus", "response")

		invocation := fake.onlyInvocation(t)
		requireExactKeys(t, "provider-validated arguments", invocation.Arguments, "page", "pageSize")
		if page, ok := invocation.Arguments["page"].(int64); !ok || page != 0 {
			t.Fatalf("default page = %#v, want int64(0)", invocation.Arguments["page"])
		}
		if pageSize, ok := invocation.Arguments["pageSize"].(int64); !ok || pageSize != 100 {
			t.Fatalf("default pageSize = %#v, want int64(100)", invocation.Arguments["pageSize"])
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("caller_values_override_defaults", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-v2-optional")
		h.invokeOperation(t, ProtocolV2, "invoke-v2-optional", listOperationID, map[string]any{
			"page":     2,
			"pageSize": 5,
		}, futureDeadline())
		h.await(t, "invoke-v2-optional").expectOK(t, ProtocolV2, "invoke-v2-optional")
		invocation := fake.onlyInvocation(t)
		requireExactKeys(t, "provider-validated arguments", invocation.Arguments, "page", "pageSize")
		if page, ok := invocation.Arguments["page"].(int64); !ok || page != 2 {
			t.Fatalf("page = %#v, want int64(2)", invocation.Arguments["page"])
		}
		if pageSize, ok := invocation.Arguments["pageSize"].(int64); !ok || pageSize != 5 {
			t.Fatalf("pageSize = %#v, want int64(5)", invocation.Arguments["pageSize"])
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})
}

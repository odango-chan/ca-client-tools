package provider

// Tests for the sole process-protocol framing/envelope contract and the
// initialize handshake. Every test drives the real Serve loop over pipes; no
// network or real Runtime is used.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestFramingEnvelope covers strict NDJSON framing and common-envelope
// rejection before initialization. All of these frames are invalid, so the
// process must reject each one and stay usable; stdout must contain only
// protocol frames.
func TestFramingEnvelope(t *testing.T) {
	fake := newFakeCatalog()
	h := newHarness(t, fixedRuntimeFactory(fake))

	validVersion := ProtocolV2
	base := func(requestID string) map[string]any {
		return map[string]any{
			"protocolVersion": validVersion,
			"type":            "request",
			"requestId":       requestID,
			"method":          "cancel",
			"params":          map[string]any{"targetRequestId": "some-target"},
		}
	}
	marshal := func(object map[string]any) string {
		raw, err := json.Marshal(object)
		if err != nil {
			t.Fatalf("marshal test frame: %v", err)
		}
		return string(raw)
	}

	longID := strings.Repeat("a", 129)

	cases := []struct {
		name       string
		raw        string
		responseID string
		wantCode   string
	}{
		{
			name:       "blank_frame",
			raw:        "\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "whitespace_only_frame",
			raw:        "   \t \n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "invalid_utf8",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"utf8-bad","method":"cancel","params":{},"x":"` + string([]byte{0xff, 0xfe}) + `"}` + "\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "non_json",
			raw:        "this is not json\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "non_object_json",
			raw:        "[1,2,3]\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "json_string_root",
			raw:        `"a string"` + "\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "duplicate_json_keys",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"dup-key","method":"cancel","params":{"targetRequestId":"a"},"params":{"targetRequestId":"b"}}` + "\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "multiple_json_values",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"multi-value","method":"cancel","params":{}}{"a":1}` + "\n",
			responseID: "",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "unknown_envelope_field",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"env-unknown","method":"cancel","params":{},"extra":true}` + "\n",
			responseID: "env-unknown",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "missing_protocol_version",
			raw:        `{"type":"request","requestId":"missing-version","method":"cancel","params":{}}` + "\n",
			responseID: "missing-version",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "missing_type",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","requestId":"missing-type","method":"cancel","params":{}}` + "\n",
			responseID: "missing-type",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "missing_request_id",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","method":"cancel","params":{}}` + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "missing_method",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"missing-method","params":{}}` + "\n",
			responseID: "missing-method",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "missing_params",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"missing-params","method":"cancel"}` + "\n",
			responseID: "missing-params",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "wrong_type_protocol_version",
			raw:        `{"protocolVersion":1,"type":"request","requestId":"wrong-version-type","method":"cancel","params":{}}` + "\n",
			responseID: "wrong-version-type",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "wrong_type_request_id",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":42,"method":"cancel","params":{}}` + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "wrong_type_envelope_type",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"response","requestId":"wrong-type-type","method":"cancel","params":{}}` + "\n",
			responseID: "wrong-type-type",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "wrong_type_method",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"wrong-method-type","method":7,"params":{}}` + "\n",
			responseID: "wrong-method-type",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "wrong_type_params",
			raw:        `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"wrong-params-type","method":"cancel","params":[]}` + "\n",
			responseID: "wrong-params-type",
			wantCode:   CodeProtocolFrameInvalid,
		},
		{
			name:       "empty_request_id",
			raw:        marshal(base("")) + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "request_id_too_long",
			raw:        marshal(base(longID)) + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "request_id_with_space",
			raw:        marshal(base("has space")) + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "request_id_non_ascii",
			raw:        marshal(base("héllo")) + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
		{
			name:       "request_id_control_character",
			raw:        marshal(base("control\tchar")) + "\n",
			responseID: "",
			wantCode:   CodeProtocolRequestIDInvalid,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			h.sendRaw(t, testCase.raw)
			frame := h.await(t, testCase.responseID)
			frame.expectError(t, ProtocolV2, testCase.responseID, testCase.wantCode)
		})
	}

	t.Run("multiple_frames_in_one_write", func(t *testing.T) {
		h.sendRaw(t, "\nnot json\n")
		h.await(t, "").expectError(t, ProtocolV2, "", CodeProtocolFrameInvalid)
		h.await(t, "").expectError(t, ProtocolV2, "", CodeProtocolFrameInvalid)
	})

	// The process stays alive after every malformed frame: a valid initialize
	// still succeeds and produces the normal handshake result.
	t.Run("invoke_before_initialize_rejected", func(t *testing.T) {
		frame := h.invoke(t, ProtocolV2, "invoke-before-init", standardArguments())
		frame.expectError(t, ProtocolV2, "invoke-before-init", CodeProviderNotInitialized)
	})

	t.Run("legacy_v1_frame_before_initialize_rejected", func(t *testing.T) {
		requestID := "legacy-catalog-before-init"
		h.send(t, wireRequest(legacyProtocolVersion, requestID, "catalog", map[string]any{"backendOperationIds": []any{}}))
		h.await(t, requestID).expectError(t, ProtocolV2, requestID, CodeProtocolVersionUnsupported)
	})

	t.Run("process_survives_malformed_frames", func(t *testing.T) {
		result := h.initializeRequest(t, ProtocolV2, "init-after-bad", defaultInitParams()).expectOK(t, ProtocolV2, "init-after-bad")
		requireExactKeys(t, "initialize result", result,
			"providerId", "providerVersion", "buildDigest", "backendBindingCatalogDigest", "limits", "catalogVersion")
	})

	h.closeInput(t)
	h.waitServe(t, nil)
	if fake.callCount() != 0 {
		t.Fatalf("fake runtime was invoked %d times, want 0", fake.callCount())
	}
	h.assertStdoutProtocolOnly(t)
}

// TestFramingUnterminatedFinalFrame covers a final frame without a trailing LF:
// it is reported as PROTOCOL_FRAME_INVALID and the process then shuts down
// normally on EOF.
func TestFramingUnterminatedFinalFrame(t *testing.T) {
	fake := newFakeCatalog()
	h := newHarness(t, fixedRuntimeFactory(fake))
	h.mustInitialize(t, ProtocolV2, "init-unterminated")

	h.sendRaw(t, wireRequest(ProtocolV2, "unterminated", "catalog", map[string]any{
		"backendOperationIds": []any{},
	}))
	h.closeInput(t)

	frame := h.await(t, "")
	frame.expectError(t, ProtocolV2, "", CodeProtocolFrameInvalid)
	h.waitServe(t, nil)
	if fake.closeCount() != 1 {
		t.Fatalf("runtime Close called %d times, want exactly 1", fake.closeCount())
	}
	h.assertStdoutProtocolOnly(t)
}

// TestFramingMaxFrameBytes covers the negotiated frame-size bound: an oversized
// frame is rejected with PROTOCOL_FRAME_TOO_LARGE, the process stays
// frame-synchronized, and valid frames keep working.
func TestFramingMaxFrameBytes(t *testing.T) {
	fake := newFakeCatalog()
	h := newHarness(t, fixedRuntimeFactory(fake))
	// Negotiate a small frame limit; the initialize frame itself must fit.
	h.mustInitializeWithLimits(t, ProtocolV2, "init-small-frame", 512, defaultMaxInFlight, defaultMaxCallDuration)

	// A syntactically valid catalog frame deliberately larger than 512 bytes.
	oversized := `{"protocolVersion":"` + ProtocolV2 + `","type":"request","requestId":"oversized-frame","method":"catalog","params":{"backendOperationIds":["` + strings.Repeat("a", 600) + `"]}}`
	if len(oversized)+1 <= 512 {
		t.Fatalf("test frame is %d bytes, must exceed the negotiated 512-byte limit", len(oversized)+1)
	}
	h.send(t, oversized)
	oversizedResponse := h.awaitAnyFrame(t)
	if oversizedResponse.frameType != "response" {
		t.Fatalf("oversized frame response type = %q, want response", oversizedResponse.frameType)
	}
	if oversizedResponse.protocolVersion != ProtocolV2 {
		t.Fatalf("oversized frame response protocolVersion = %q, want %q", oversizedResponse.protocolVersion, ProtocolV2)
	}
	if oversizedResponse.requestID != "" && !validOpaqueID(oversizedResponse.requestID) {
		t.Fatalf("oversized frame response requestId = %q, want empty or a valid opaque ID", oversizedResponse.requestID)
	}
	if oversizedResponse.ok || oversizedResponse.errorCode != CodeProtocolFrameTooLarge {
		t.Fatalf("oversized frame response = %s, want error code %s (negotiated maxFrameBytes=512 must be enforced for the first post-initialize frame)", oversizedResponse.line, CodeProtocolFrameTooLarge)
	}

	// A line that crosses the 64 KiB read buffer boundary is still bounded by
	// the negotiated limit and leaves the stream frame-synchronized.
	h.send(t, `{"protocolVersion":"`+ProtocolV2+`","type":"request","requestId":"huge-frame","method":"catalog","params":{"backendOperationIds":["`+strings.Repeat("b", 70*1024)+`"]}}`)
	hugeResponse := h.awaitAnyFrame(t)
	if hugeResponse.ok || hugeResponse.errorCode != CodeProtocolFrameTooLarge {
		t.Fatalf("large over-limit frame response = %s, want error code %s", hugeResponse.line, CodeProtocolFrameTooLarge)
	}

	// The oversized lines were drained: the next frame is processed normally.
	result := h.catalog(t, ProtocolV2, "catalog-after-oversized", []string{allBindingIDs()[0]}).expectOK(t, ProtocolV2, "catalog-after-oversized")
	requireExactKeys(t, "catalog result", result, "supported", "unsupported", "catalogVersion", "bindings")

	// Two valid frames written in a single write are read as two frames.
	h.sendRaw(t,
		wireRequest(ProtocolV2, "catalog-batched-1", "catalog", map[string]any{"backendOperationIds": []any{allBindingIDs()[0]}})+"\n"+
			wireRequest(ProtocolV2, "catalog-batched-2", "catalog", map[string]any{"backendOperationIds": []any{allBindingIDs()[1]}})+"\n")
	h.await(t, "catalog-batched-1").expectOK(t, ProtocolV2, "catalog-batched-1")
	h.await(t, "catalog-batched-2").expectOK(t, ProtocolV2, "catalog-batched-2")

	h.closeInput(t)
	h.waitServe(t, nil)
	h.assertStdoutProtocolOnly(t)
}

// TestInitialize covers the startup handshake: pre-initialize method rejection,
// parameter validation, hard limits, trusted identity/digest validation and the
// exact successful result shape.
func TestInitialize(t *testing.T) {
	t.Run("non_initialize_first_is_rejected", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))

		h.send(t, wireRequest(ProtocolV2, "catalog-before-init", "catalog", map[string]any{
			"backendOperationIds": []any{},
		}))
		h.await(t, "catalog-before-init").expectError(t, ProtocolV2, "catalog-before-init", CodeProviderNotInitialized)

		h.send(t, wireRequest(ProtocolV2, "cancel-before-init", "cancel", map[string]any{
			"targetRequestId": "x",
		}))
		h.await(t, "cancel-before-init").expectError(t, ProtocolV2, "cancel-before-init", CodeProviderNotInitialized)

		// The process remains usable: the first accepted initialize succeeds.
		result := h.mustInitialize(t, ProtocolV2, "init-after-preinit")
		requireExactKeys(t, "initialize result", result,
			"providerId", "providerVersion", "buildDigest", "backendBindingCatalogDigest", "limits", "catalogVersion")
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("success_exact_result_and_limits", func(t *testing.T) {
		fake := newFakeCatalog()
		factory := &runtimeFactory{build: func() (Runtime, error) { return fake, nil }}
		h := newHarness(t, factory.newRuntime)

		requested := initParams(4096, 3, 1234)
		frame := h.initializeRequest(t, ProtocolV2, "init-success", requested)
		result := frame.expectOK(t, ProtocolV2, "init-success")

		requireExactKeys(t, "initialize result", result,
			"providerId", "providerVersion", "buildDigest", "backendBindingCatalogDigest", "limits", "catalogVersion")
		if got := requireString(t, "catalogVersion", result["catalogVersion"]); got != testCatalogVersion {
			t.Fatalf("catalogVersion = %q, want %q", got, testCatalogVersion)
		}
		if got := requireString(t, "providerId", result["providerId"]); got != ProviderID {
			t.Fatalf("providerId = %q, want %q", got, ProviderID)
		}
		if got := requireString(t, "providerVersion", result["providerVersion"]); got != testProviderVersion {
			t.Fatalf("providerVersion = %q, want %q", got, testProviderVersion)
		}
		if got := requireString(t, "buildDigest", result["buildDigest"]); got != testBuildDigest {
			t.Fatalf("buildDigest = %q, want %q", got, testBuildDigest)
		}
		if got := requireString(t, "backendBindingCatalogDigest", result["backendBindingCatalogDigest"]); got != testCatalogDigest {
			t.Fatalf("backendBindingCatalogDigest = %q, want %q", got, testCatalogDigest)
		}
		limits, ok := result["limits"].(map[string]any)
		if !ok {
			t.Fatalf("limits = %#v, want object", result["limits"])
		}
		requireExactKeys(t, "initialize limits", limits, "maxFrameBytes", "maxInFlight", "maxCallDurationMs")
		if got := requireNumber(t, "maxFrameBytes", limits["maxFrameBytes"]); got != 4096 {
			t.Fatalf("accepted maxFrameBytes = %d, want requested 4096", got)
		}
		if got := requireNumber(t, "maxInFlight", limits["maxInFlight"]); got != 3 {
			t.Fatalf("accepted maxInFlight = %d, want requested 3", got)
		}
		if got := requireNumber(t, "maxCallDurationMs", limits["maxCallDurationMs"]); got != 1234 {
			t.Fatalf("accepted maxCallDurationMs = %d, want requested 1234", got)
		}
		if calls := factory.initializeCalls(); calls != 1 {
			t.Fatalf("NewRuntime calls = %d, want 1", calls)
		}
		if fake.callCount() != 0 {
			t.Fatalf("initialize performed %d Invoke calls, want 0", fake.callCount())
		}

		h.closeInput(t)
		h.waitServe(t, nil)
		if fake.closeCount() != 1 {
			t.Fatalf("runtime Close called %d times after EOF, want 1", fake.closeCount())
		}
	})

	t.Run("legacy_v1_initialize_rejected", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.initializeRequest(t, legacyProtocolVersion, "init-legacy", defaultInitParams()).expectError(t, ProtocolV2, "init-legacy", CodeProtocolVersionUnsupported)
		if calls := fake.callCount(); calls != 0 {
			t.Fatalf("legacy initialize dispatched %d Invoke calls, want 0", calls)
		}

		// The process remains usable with the sole accepted identity.
		result := h.mustInitialize(t, ProtocolV2, "init-after-legacy")
		if got := requireString(t, "catalogVersion", result["catalogVersion"]); got != testCatalogVersion {
			t.Fatalf("catalogVersion = %q, want %q", got, testCatalogVersion)
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("duplicate_initialize", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		h.mustInitialize(t, ProtocolV2, "init-first")

		frame := h.initializeRequest(t, ProtocolV2, "init-second", defaultInitParams())
		frame.expectError(t, ProtocolV2, "init-second", CodeProviderAlreadyInitialized)

		// The process remains usable after the rejected second initialize.
		result := h.catalog(t, ProtocolV2, "catalog-after-second-init", []string{allBindingIDs()[0]}).expectOK(t, ProtocolV2, "catalog-after-second-init")
		supported, ok := result["supported"].([]any)
		if !ok || len(supported) != 1 {
			t.Fatalf("catalog supported = %#v, want one entry", result["supported"])
		}
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("request_id_boundary_and_reuse", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		boundaryID := strings.Repeat("e", 128)
		h.mustInitialize(t, ProtocolV2, boundaryID)

		// A request ID is unique for the process lifetime and is never reused.
		h.send(t, wireRequest(ProtocolV2, boundaryID, "catalog", map[string]any{"backendOperationIds": []any{}}))
		h.await(t, boundaryID).expectError(t, ProtocolV2, boundaryID, CodeProtocolRequestIDInvalid)

		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("request_id_reuse_after_failed_initialize", func(t *testing.T) {
		fake := newFakeCatalog()
		h := newHarness(t, fixedRuntimeFactory(fake))
		failedID := "reused-after-failed-init"
		malformed := map[string]any{"hostInstanceId": "host-instance-01"}
		h.initializeRequest(t, ProtocolV2, failedID, malformed).expectError(t, ProtocolV2, failedID, CodeProtocolFrameInvalid)
		h.initializeRequest(t, ProtocolV2, failedID, defaultInitParams()).expectError(t, ProtocolV2, failedID, CodeProtocolRequestIDInvalid)
		h.closeInput(t)
		h.waitServe(t, nil)
	})

	t.Run("malformed_initialize_params", func(t *testing.T) {
		validLimits := func() map[string]any {
			return map[string]any{"maxFrameBytes": int64(1 << 20), "maxInFlight": int64(8), "maxCallDurationMs": int64(5000)}
		}
		fullParams := func() map[string]any {
			return map[string]any{
				"hostInstanceId":     "host-instance-01",
				"providerInstanceId": ProviderID,
				"limits":             validLimits(),
			}
		}
		without := func(key string) map[string]any {
			params := fullParams()
			delete(params, key)
			return params
		}
		limitsWithout := func(key string) map[string]any {
			params := fullParams()
			limits := validLimits()
			delete(limits, key)
			params["limits"] = limits
			return params
		}
		limitsWith := func(key string, value any) map[string]any {
			params := fullParams()
			limits := validLimits()
			limits[key] = value
			params["limits"] = limits
			return params
		}
		limitsExtra := func() map[string]any {
			params := fullParams()
			limits := validLimits()
			limits["extra"] = int64(1)
			params["limits"] = limits
			return params
		}

		longID := strings.Repeat("b", 129)
		cases := []struct {
			name     string
			params   map[string]any
			wantCode string
		}{
			{"missing_host_instance_id", without("hostInstanceId"), CodeProtocolFrameInvalid},
			{"missing_provider_instance_id", without("providerInstanceId"), CodeProtocolFrameInvalid},
			{"missing_limits", without("limits"), CodeProtocolFrameInvalid},
			{"missing_params_member", map[string]any{"hostInstanceId": "host-instance-01"}, CodeProtocolFrameInvalid},
			{"extra_params_member", func() map[string]any {
				params := fullParams()
				params["extra"] = true
				return params
			}(), CodeProtocolFrameInvalid},
			{"empty_params", map[string]any{}, CodeProtocolFrameInvalid},
			{"limits_not_object", func() map[string]any {
				params := fullParams()
				params["limits"] = "limits"
				return params
			}(), CodeProtocolFrameInvalid},
			{"limits_missing_max_frame_bytes", limitsWithout("maxFrameBytes"), CodeProtocolFrameInvalid},
			{"limits_missing_max_in_flight", limitsWithout("maxInFlight"), CodeProtocolFrameInvalid},
			{"limits_missing_max_call_duration", limitsWithout("maxCallDurationMs"), CodeProtocolFrameInvalid},
			{"limits_extra_member", limitsExtra(), CodeProtocolFrameInvalid},
			{"empty_host_instance_id", func() map[string]any {
				params := fullParams()
				params["hostInstanceId"] = ""
				return params
			}(), CodeProviderConfigurationInvalid},
			{"host_instance_id_with_space", func() map[string]any {
				params := fullParams()
				params["hostInstanceId"] = "host instance 01"
				return params
			}(), CodeProviderConfigurationInvalid},
			{"host_instance_id_too_long", func() map[string]any {
				params := fullParams()
				params["hostInstanceId"] = longID
				return params
			}(), CodeProviderConfigurationInvalid},
			{"empty_provider_instance_id", func() map[string]any {
				params := fullParams()
				params["providerInstanceId"] = ""
				return params
			}(), CodeProviderConfigurationInvalid},
			{"provider_instance_id_non_ascii", func() map[string]any {
				params := fullParams()
				params["providerInstanceId"] = "provider-ü"
				return params
			}(), CodeProviderConfigurationInvalid},
			{"frame_bytes_zero", limitsWith("maxFrameBytes", int64(0)), CodeProviderConfigurationInvalid},
			{"frame_bytes_negative", limitsWith("maxFrameBytes", int64(-1)), CodeProviderConfigurationInvalid},
			{"frame_bytes_above_hard_max", limitsWith("maxFrameBytes", int64(HardMaxFrameBytes+1)), CodeProviderConfigurationInvalid},
			{"frame_bytes_wrong_type", limitsWith("maxFrameBytes", "8388608"), CodeProviderConfigurationInvalid},
			{"frame_bytes_fractional", limitsWith("maxFrameBytes", 1.5), CodeProviderConfigurationInvalid},
			{"in_flight_zero", limitsWith("maxInFlight", int64(0)), CodeProviderConfigurationInvalid},
			{"in_flight_above_hard_max", limitsWith("maxInFlight", int64(HardMaxInFlight+1)), CodeProviderConfigurationInvalid},
			{"in_flight_wrong_type", limitsWith("maxInFlight", true), CodeProviderConfigurationInvalid},
			{"duration_zero", limitsWith("maxCallDurationMs", int64(0)), CodeProviderConfigurationInvalid},
			{"duration_negative", limitsWith("maxCallDurationMs", int64(-5)), CodeProviderConfigurationInvalid},
			{"duration_above_hard_max", limitsWith("maxCallDurationMs", int64(HardMaxCallDuration.Milliseconds())+1), CodeProviderConfigurationInvalid},
			{"duration_wrong_type", limitsWith("maxCallDurationMs", 1.5), CodeProviderConfigurationInvalid},
		}

		for index, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fake := newFakeCatalog()
				factory := &runtimeFactory{build: func() (Runtime, error) { return fake, nil }}
				h := newHarness(t, factory.newRuntime)
				requestID := "bad-init-" + string(rune('a'+index))
				frame := h.initializeRequest(t, ProtocolV2, requestID, testCase.params)
				frame.expectError(t, ProtocolV2, requestID, testCase.wantCode)
				if runtimes := factory.recordedRuntimes(); len(runtimes) != 0 {
					t.Fatalf("runtime factory was called for a rejected initialize: %d runtimes", len(runtimes))
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})

	t.Run("factory_errors_map_to_stable_codes", func(t *testing.T) {
		cases := []struct {
			name     string
			failErr  error
			wantCode string
		}{
			{"config_error", &ConfigError{Reason: "missing origin"}, CodeProviderConfigurationInvalid},
			{"catalog_error", &CatalogError{Reason: "bad binding"}, CodeProviderBindingCatalogInvalid},
			{"unknown_error", errors.New("boom"), CodeProviderInternal},
		}
		for _, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				factory := &runtimeFactory{failErr: testCase.failErr}
				h := newHarness(t, factory.newRuntime)
				frame := h.initializeRequest(t, ProtocolV2, "init-factory-error", defaultInitParams())
				frame.expectError(t, ProtocolV2, "init-factory-error", testCase.wantCode)
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}

		t.Run("retry_after_factory_error", func(t *testing.T) {
			attempts := 0
			fake := newFakeCatalog()
			factory := &runtimeFactory{build: func() (Runtime, error) {
				attempts++
				if attempts == 1 {
					return nil, &ConfigError{Reason: "transient"}
				}
				return fake, nil
			}}
			h := newHarness(t, factory.newRuntime)
			h.initializeRequest(t, ProtocolV2, "init-factory-fail", defaultInitParams()).expectError(t, ProtocolV2, "init-factory-fail", CodeProviderConfigurationInvalid)
			h.mustInitialize(t, ProtocolV2, "init-factory-retry")
			h.closeInput(t)
			h.waitServe(t, nil)
		})
	})

	t.Run("invalid_build_digest", func(t *testing.T) {
		digests := []string{
			"",
			"deadbeef",
			"sha256:" + strings.Repeat("A", 64),
			"sha256:" + strings.Repeat("a", 63),
			"sha256:" + strings.Repeat("a", 65),
			"sha512:" + strings.Repeat("a", 64),
			"sha256:" + strings.Repeat("g", 64),
		}
		for index, digest := range digests {
			digest := digest
			t.Run("digest_"+string(rune('a'+index)), func(t *testing.T) {
				fake := newFakeCatalog()
				factory := &runtimeFactory{build: func() (Runtime, error) { return fake, nil }}
				info := defaultInfo()
				info.BuildDigest = digest
				h := newHarnessWithInfo(t, info, factory.newRuntime)
				frame := h.initializeRequest(t, ProtocolV2, "init-bad-build-digest", defaultInitParams())
				frame.expectError(t, ProtocolV2, "init-bad-build-digest", CodeProviderConfigurationInvalid)
				if fake.closeCount() != 1 {
					t.Fatalf("runtime Close called %d times after rejected initialize, want 1", fake.closeCount())
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})

	t.Run("invalid_catalog_digest", func(t *testing.T) {
		for index, digest := range []string{"", "sha256:nothex", "sha256:" + strings.Repeat("F", 64)} {
			digest := digest
			t.Run("digest_"+string(rune('a'+index)), func(t *testing.T) {
				fake := newFakeCatalog()
				fake.catalogDigest = digest
				factory := &runtimeFactory{build: func() (Runtime, error) { return fake, nil }}
				h := newHarness(t, factory.newRuntime)
				frame := h.initializeRequest(t, ProtocolV2, "init-bad-catalog-digest", defaultInitParams())
				frame.expectError(t, ProtocolV2, "init-bad-catalog-digest", CodeProviderBindingCatalogInvalid)
				if fake.closeCount() != 1 {
					t.Fatalf("runtime Close called %d times after rejected initialize, want 1", fake.closeCount())
				}
				h.closeInput(t)
				h.waitServe(t, nil)
			})
		}
	})
}

// TestInitializeDoesNotQueryBackend proves initialization never performs an
// implicit live health query.
func TestInitializeDoesNotQueryBackend(t *testing.T) {
	fake := newFakeCatalog()
	h := newHarness(t, fixedRuntimeFactory(fake))
	h.mustInitialize(t, ProtocolV2, "init-no-health-query")
	if fake.callCount() != 0 {
		t.Fatalf("initialize invoked the backend %d times, want 0", fake.callCount())
	}
	h.closeInput(t)
	h.waitServe(t, nil)
}

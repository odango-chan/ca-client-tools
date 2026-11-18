package provider

import (
	"fmt"
	"strings"
	"testing"
)

// artifactLimitsRuntime wraps the fake runtime with reported artifact limits.
type artifactLimitsRuntime struct {
	*fakeRuntime
	limits     ArtifactLimits
	configured bool
}

func (r *artifactLimitsRuntime) ArtifactLimits() (ArtifactLimits, bool) {
	return r.limits, r.configured
}

func artifactChannelValue() map[string]any {
	return map[string]any{
		"baseUrl":           "http://127.0.0.1:53211",
		"processToken":      "fixture-process-token-not-a-secret",
		"requestTimeoutMs":  int64(3000),
		"maxArtifactBytes":  int64(1048576),
		"maxAggregateBytes": int64(4194304),
		"maxAttachments":    int64(8),
	}
}

func TestInitializeArtifactChannel(t *testing.T) {
	var captured ArtifactChannelConfig
	factory := func(config ArtifactChannelConfig) (Runtime, error) {
		captured = config
		return &artifactLimitsRuntime{
			fakeRuntime: newFakeCatalog(),
			limits: ArtifactLimits{
				RequestTimeout:    3e9,
				MaxArtifactBytes:  1048576,
				MaxAggregateBytes: 4194304,
				MaxAttachments:    8,
			},
			configured: true,
		}, nil
	}
	h := newHarness(t, factory)
	params := defaultInitParams()
	params["artifactChannel"] = artifactChannelValue()
	result := h.initializeRequest(t, ProtocolV2, "init-artifact", params).expectOK(t, ProtocolV2, "init-artifact")

	channel, ok := result["artifactChannel"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result artifactChannel = %#v", result["artifactChannel"])
	}
	if channel["requestTimeoutMs"] != nil {
		// Response frames are decoded with UseNumber, so compare the textual
		// JSON number form.
		for key, want := range map[string]string{
			"requestTimeoutMs":  "3000",
			"maxArtifactBytes":  "1048576",
			"maxAggregateBytes": "4194304",
			"maxAttachments":    "8",
		} {
			if got := fmt.Sprint(channel[key]); got != want {
				t.Fatalf("effective artifact limit %s = %q, want %s", key, got, want)
			}
		}
	} else {
		t.Fatalf("effective artifact limits = %#v", channel)
	}
	if captured.BaseURL != "http://127.0.0.1:53211" || captured.ProcessToken != "fixture-process-token-not-a-secret" ||
		captured.MaxArtifactBytes != 1048576 || captured.MaxAttachments != 8 {
		t.Fatalf("runtime channel config = %+v", captured)
	}
	// The opaque process token is never echoed in the protocol result.
	wire := string(h.sink.rawBytes())
	if strings.Contains(wire, "fixture-process-token-not-a-secret") {
		t.Fatalf("initialize result leaked the process token: %s", wire)
	}
}

func TestInitializeWithoutArtifactChannel(t *testing.T) {
	var captured ArtifactChannelConfig
	factory := func(config ArtifactChannelConfig) (Runtime, error) {
		captured = config
		return newFakeCatalog(), nil
	}
	h := newHarness(t, factory)
	result := h.mustInitialize(t, ProtocolV2, "init-plain")
	if _, present := result["artifactChannel"]; present {
		t.Fatalf("initialize without a channel reported artifactChannel: %#v", result)
	}
	if captured.Configured() {
		t.Fatalf("runtime received an artifact channel: %+v", captured)
	}
}

func TestInitializeArtifactChannelMalformed(t *testing.T) {
	cases := map[string]func(map[string]any){
		"missing process token": func(value map[string]any) { delete(value, "processToken") },
		"extra member":          func(value map[string]any) { value["origin"] = "http://127.0.0.1:1" },
		"non-loopback host":     func(value map[string]any) { value["baseUrl"] = "http://0.0.0.0:53211" },
		"https base url":        func(value map[string]any) { value["baseUrl"] = "https://127.0.0.1:53211" },
		"base url path":         func(value map[string]any) { value["baseUrl"] = "http://127.0.0.1:53211/host" },
		"port out of range":     func(value map[string]any) { value["baseUrl"] = "http://127.0.0.1:70000" },
		"empty token":           func(value map[string]any) { value["processToken"] = "" },
		"control token":         func(value map[string]any) { value["processToken"] = "bad\ntoken" },
		"long token":            func(value map[string]any) { value["processToken"] = strings.Repeat("a", 513) },
		"zero timeout":          func(value map[string]any) { value["requestTimeoutMs"] = int64(0) },
		"timeout over call":     func(value map[string]any) { value["requestTimeoutMs"] = int64(6000) },
		"zero artifact":         func(value map[string]any) { value["maxArtifactBytes"] = int64(0) },
		"negative aggregate":    func(value map[string]any) { value["maxAggregateBytes"] = int64(-1) },
		"zero attachments":      func(value map[string]any) { value["maxAttachments"] = int64(0) },
		"fractional bound":      func(value map[string]any) { value["maxArtifactBytes"] = 1.5 },
		"wrong type":            func(value map[string]any) { value["maxArtifactBytes"] = "1024" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, fixedRuntimeFactory(newFakeCatalog()))
			params := defaultInitParams()
			value := artifactChannelValue()
			mutate(value)
			params["artifactChannel"] = value
			frame := h.initializeRequest(t, ProtocolV2, "init-bad-channel", params)
			frame.requireEnvelope(t, ProtocolV2, "init-bad-channel")
			if frame.ok || frame.errorCode != CodeProviderConfigurationInvalid {
				t.Fatalf("fault = %s, want PROVIDER_CONFIGURATION_INVALID", frame.errorCode)
			}
		})
	}
}

func TestInitializeUnknownParamRejected(t *testing.T) {
	h := newHarness(t, fixedRuntimeFactory(newFakeCatalog()))
	params := defaultInitParams()
	params["unexpected"] = true
	frame := h.initializeRequest(t, ProtocolV2, "init-unknown", params)
	frame.requireEnvelope(t, ProtocolV2, "init-unknown")
	if frame.ok || frame.errorCode != CodeProtocolFrameInvalid {
		t.Fatalf("fault = %s, want PROTOCOL_FRAME_INVALID", frame.errorCode)
	}
}

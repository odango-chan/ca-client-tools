package providerapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/provider"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
)

// TestArtifactSlotsMatchPinnedFixtures validates the derived artifactSlots
// extension against the shared conformance slot vectors. The vectors pin the
// closed descriptor shape; the private catalog content-ID pointers are never
// published.
func TestArtifactSlotsMatchPinnedFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "contracts", "host-artifact-store", "v1")
	load := func(name string) any {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		var envelope struct {
			FixtureVersion string `json:"fixtureVersion"`
			Value          any    `json:"value"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("decode fixture %s: %v", name, err)
		}
		if envelope.FixtureVersion != "ca-host.artifact-store/v1" {
			t.Fatalf("fixture %s version = %q", name, envelope.FixtureVersion)
		}
		return envelope.Value
	}
	wire := func(slots *provider.ArtifactSlots) any {
		t.Helper()
		if slots == nil {
			t.Fatal("no artifact slots were derived")
		}
		entries := func(values []provider.ArtifactSlot) []any {
			result := make([]any, 0, len(values))
			for _, value := range values {
				result = append(result, map[string]any{
					"slot":      value.Slot,
					"pointer":   value.Pointer,
					"mediaType": value.MediaType,
					"required":  value.Required,
				})
			}
			return result
		}
		return map[string]any{
			"inputs":  entries(slots.Inputs),
			"outputs": entries(slots.Outputs),
		}
	}

	binary := executor.Operation{
		OperationID:        "acceptance.artifact.binary",
		Body:               &executor.BodySpec{Kind: executor.PayloadBinary, MediaType: "application/octet-stream"},
		ResponseMediaTypes: []string{"application/octet-stream"},
	}
	if got, want := wire(artifactSlotsForOperation(binary)), load("slots-binary.json"); !reflect.DeepEqual(got, want) {
		t.Fatalf("binary slots = %#v, want %#v", got, want)
	}

	multipart := executor.Operation{
		OperationID: "acceptance.artifact.multipart",
		Body: &executor.BodySpec{
			Kind:      executor.PayloadMultipart,
			MediaType: "multipart/related",
			Parts: []executor.MultipartPart{
				{Name: "n1Message", ContentType: "application/vnd.3gpp.5gnas", JSONPointerField: "/messages/0/n1MessageContent/contentId", Required: true},
				{Name: "n2Message", ContentType: "application/vnd.3gpp.ngap", JSONPointerField: "/messages/1/n2MessageContent/contentId", Required: false},
			},
		},
		ResponseMediaTypes: []string{"multipart/related"},
		ResponseParts: []executor.MultipartPart{
			{Name: "n2Response", ContentType: "application/vnd.3gpp.ngap", JSONPointerField: "/n2MessageContent/contentId", Required: true},
			{Name: "n1Response", ContentType: "application/vnd.3gpp.5gnas", JSONPointerField: "/messages/0/n1MessageContent/contentId", Required: true},
		},
	}
	if got, want := wire(artifactSlotsForOperation(multipart)), load("slots-multipart.json"); !reflect.DeepEqual(got, want) {
		t.Fatalf("multipart slots = %#v, want %#v", got, want)
	}

	// A JSON-only binding publishes no artifactSlots extension.
	if slots := artifactSlotsForOperation(executor.Operation{ResponseMediaTypes: []string{"application/json"}}); slots != nil {
		t.Fatalf("JSON binding derived artifact slots: %#v", slots)
	}
}

// TestValidateArtifactArgumentsLocationRules proves the pre-I/O closed reference
// validation for declared slots and rejects reference values at undeclared
// locations without echoing them.
func TestValidateArtifactArgumentsLocationRules(t *testing.T) {
	slots := &provider.ArtifactSlots{
		Inputs: []provider.ArtifactSlot{
			{Slot: "body", Pointer: "/body", MediaType: "application/octet-stream", Required: true},
			{Slot: "n1Message", Pointer: "/payloads/n1Message", MediaType: "application/vnd.3gpp.5gnas", Required: true},
		},
	}
	valid := "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	validArgument := map[string]any{
		"body":     map[string]any{"artifactRef": valid},
		"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}},
	}
	if fault := validateArtifactArguments(slots, validArgument); fault != nil {
		t.Fatalf("valid references rejected: %#v", fault)
	}
	cases := map[string]map[string]any{
		"missing binary slot": {"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}}},
		"plain path at slot":  {"body": "/var/lib/artifacts/input.bin", "payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}}},
		"bad token":           {"body": map[string]any{"artifactRef": "art_short"}, "payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}}},
		"extra member":        {"body": map[string]any{"artifactRef": valid, "size": 4}, "payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}}},
		"reference undeclared": {
			"body":     map[string]any{"artifactRef": valid},
			"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}, "n2Message": map[string]any{"artifactRef": valid}},
		},
		"reference nested at non-slot value": {
			"body":     map[string]any{"artifactRef": valid},
			"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}},
			"extra":    map[string]any{"nested": map[string]any{"artifactRef": valid}},
		},
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			fault := validateArtifactArguments(slots, arguments)
			if fault == nil || fault.Code != provider.CodeBackendArgumentsInvalid {
				t.Fatalf("%s fault = %#v, want BACKEND_ARGUMENTS_INVALID", name, fault)
			}
			if fault.Detail != nil {
				t.Fatalf("%s fault leaked caller values: %#v", name, fault.Detail)
			}
		})
	}
}

package providerapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/provider"
	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

// TestGenericNonArtifactInvocationRejectsReferenceAtUndeclaredLocation is the
// contract §2.4/§5.3 repro: a Host artifact reference value is valid only at a
// declared artifact slot. A genericSBI binding that declares no artifact slots
// has an empty declared set, so a reference smuggled through an open nested
// argument object must be BACKEND_ARGUMENTS_INVALID before remote I/O instead of
// being forwarded verbatim to the upstream NF.
func TestGenericNonArtifactInvocationRejectsReferenceAtUndeclaredLocation(t *testing.T) {
	var requests int
	server := newGenericTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"ok":true}`)
	}))
	client, err := sbi.NewClient(sbi.Config{Services: map[string]sbi.ServiceConfig{
		"test-service": {Origin: server.URL},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	const operationID = "acceptance.nonartifact.echo"
	bodySchema, err := schemacheck.Compile([]byte(`{
		"type": "object",
		"properties": {"label": {"type": "string"}},
		"additionalProperties": true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	argumentSchema, err := schemacheck.Compile([]byte(`{
		"type": "object",
		"properties": {"body": {"type": "object", "additionalProperties": true}},
		"required": ["body"],
		"additionalProperties": false
	}`))
	if err != nil {
		t.Fatal(err)
	}
	genericExecutor, err := executor.New(client, nil, payload.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &providerRuntime{
		operations: map[string]executor.Operation{
			operationID: {
				OperationID:        operationID,
				Effect:             executor.EffectRead,
				ServiceKey:         "test-service",
				Method:             http.MethodPost,
				PathTemplate:       "/acceptance/nonartifact/echo",
				Body:               &executor.BodySpec{Kind: executor.PayloadJSON, MediaType: "application/json", Schema: bodySchema},
				ResponseMediaTypes: []string{"application/json"},
			},
		},
		argumentSchemas: map[string]*schemacheck.Schema{operationID: argumentSchema},
		artifactSlots:   map[string]*provider.ArtifactSlots{operationID: nil},
		executor:        genericExecutor,
	}

	token := "art_" + strings.Repeat("A", 43)
	_, invokeErr := runtime.invokeGeneric(context.Background(), operationID, "inv-reference-scope", map[string]any{
		"body": map[string]any{"artifactRef": token},
	})
	fault, ok := invokeErr.(*provider.Fault)
	if !ok || fault.Code != provider.CodeBackendArgumentsInvalid {
		t.Fatalf("invoke error = %#v, want BACKEND_ARGUMENTS_INVALID", invokeErr)
	}
	if requests != 0 {
		t.Fatalf("reference at an undeclared location reached the upstream %d times", requests)
	}

	// A reference-free invocation of the same binding still dispatches.
	result, err := runtime.invokeGeneric(context.Background(), operationID, "inv-reference-scope-ok", map[string]any{
		"body": map[string]any{"label": "demo"},
	})
	if err != nil {
		t.Fatalf("reference-free invoke: %v", err)
	}
	response, ok := result.Response.(map[string]any)
	if !ok || response["ok"] != true {
		t.Fatalf("reference-free response = %#v", result.Response)
	}
	if requests != 1 {
		t.Fatalf("reference-free invoke reached the upstream %d times, want 1", requests)
	}

	// The rejected reference token is never echoed in the protocol fault.
	encoded, marshalErr := json.Marshal(fault)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), token) || strings.Contains(fault.Error(), token) {
		t.Fatalf("fault leaked the reference token: %s", encoded)
	}
}

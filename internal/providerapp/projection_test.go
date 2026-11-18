package providerapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/provider"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
)

// TestProjectGenericResultBoundaries is the regression test for the response
// projection order defect: a multipart response decodes to a JSON root AND
// attachments, so the JSON branch must never win over the unsupported-artifact
// branch. It also covers pure binary responses and the normal JSON/204 results.
func TestProjectGenericResultBoundaries(t *testing.T) {
	attachment := executor.Attachment{
		ContentID:   "attachment-1",
		ContentType: "application/vnd.3gpp.ngap",
		Size:        9,
		SHA256:      strings.Repeat("a", 64),
		Data:        []byte("secret-n2"),
	}
	cases := []struct {
		name       string
		executed   executor.Result
		wantFault  bool
		wantStatus int
		wantJSON   bool
	}{
		{
			name: "json root and attachments are rejected together",
			executed: executor.Result{
				HTTPStatus:  200,
				JSON:        map[string]any{"n2MessageContent": "attachment-1"},
				Attachments: []executor.Attachment{attachment},
			},
			wantFault: true,
		},
		{
			name:      "binary response is rejected",
			executed:  executor.Result{HTTPStatus: 200, Body: []byte("binary-payload")},
			wantFault: true,
		},
		{
			name:      "multipart response without JSON root is rejected",
			executed:  executor.Result{HTTPStatus: 200, Attachments: []executor.Attachment{attachment}},
			wantFault: true,
		},
		{
			name:       "ordinary JSON response is projected",
			executed:   executor.Result{HTTPStatus: 200, JSON: map[string]any{"ok": true}},
			wantStatus: 200,
			wantJSON:   true,
		},
		{
			name:       "202 JSON response keeps its status",
			executed:   executor.Result{HTTPStatus: 202, JSON: map[string]any{"accepted": true}},
			wantStatus: 202,
			wantJSON:   true,
		},
		{
			name:       "204 with no body keeps its status",
			executed:   executor.Result{HTTPStatus: 204},
			wantStatus: 204,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := projectGenericResult(testCase.executed)
			if testCase.wantFault {
				if err == nil {
					t.Fatalf("artifact-bearing result was projected as success: %#v", result)
				}
				fault, ok := err.(*provider.Fault)
				if !ok || fault.Code != provider.CodeRemoteResponseInvalid {
					t.Fatalf("error = %#v, want REMOTE_RESPONSE_INVALID fault", err)
				}
				if result.Response != nil || result.HTTPStatus != 0 {
					t.Fatalf("partial success leaked with the fault: %#v", result)
				}
				encoded, marshalErr := json.Marshal(fault)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				for _, secret := range []string{"secret-n2", "binary-payload", "attachment-1"} {
					if strings.Contains(string(encoded), secret) || strings.Contains(fault.Error(), secret) {
						t.Fatalf("fault leaked artifact content %q: %s", secret, encoded)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("projectGenericResult: %v", err)
			}
			if result.HTTPStatus != testCase.wantStatus {
				t.Fatalf("httpStatus = %d, want %d", result.HTTPStatus, testCase.wantStatus)
			}
			if testCase.wantJSON {
				if result.Response == nil {
					t.Fatal("JSON response was dropped")
				}
			} else if result.Response != nil || result.Problem != nil {
				t.Fatalf("empty response unexpectedly projected: %#v", result)
			}
		})
	}
}

func TestResponseProjectionSupported(t *testing.T) {
	cases := []struct {
		name string
		op   executor.Operation
		want bool
	}{
		{name: "json", op: executor.Operation{ResponseMediaTypes: []string{"application/json"}}, want: true},
		{name: "problem+json", op: executor.Operation{ResponseMediaTypes: []string{"application/problem+json"}}, want: true},
		{name: "no declared body", op: executor.Operation{}, want: true},
		{name: "binary", op: executor.Operation{ResponseMediaTypes: []string{"application/octet-stream"}}, want: false},
		{name: "multipart", op: executor.Operation{ResponseMediaTypes: []string{"multipart/related"}}, want: false},
		{name: "parts declaration", op: executor.Operation{ResponseMediaTypes: []string{"application/json"}, ResponseParts: []executor.MultipartPart{{Name: "part"}}}, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := responseProjectionSupported(testCase.op); got != testCase.want {
				t.Fatalf("responseProjectionSupported = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestInvokeGenericRejectsArtifactOperationBeforeExecutor proves the execution
// entrance rejects an artifact-bearing operation before any executor call (the
// runtime intentionally has no executor here, so a bypass would panic). Under
// P1 an artifact-bearing binding without a configured Host channel is
// NOT_CONFIGURED.
func TestInvokeGenericRejectsArtifactOperationBeforeExecutor(t *testing.T) {
	for _, operation := range []executor.Operation{
		{OperationID: "test.binary", ResponseMediaTypes: []string{"application/octet-stream"}},
		{OperationID: "test.multipart", ResponseMediaTypes: []string{"multipart/related"}, ResponseParts: []executor.MultipartPart{{Name: "part", ContentType: "application/vnd.3gpp.ngap", JSONPointerField: "/part/contentId"}}},
	} {
		runtime := &providerRuntime{
			operations:    map[string]executor.Operation{operation.OperationID: operation},
			artifactSlots: map[string]*provider.ArtifactSlots{operation.OperationID: artifactSlotsForOperation(operation)},
		}
		_, err := runtime.invokeGeneric(context.Background(), operation.OperationID, "inv-test", map[string]any{})
		fault, ok := err.(*provider.Fault)
		if !ok || fault.Code != provider.CodeBackendOperationUnsupported {
			t.Fatalf("%s: error = %#v, want BACKEND_OPERATION_UNSUPPORTED", operation.OperationID, err)
		}
		if reason := fault.Detail["reason"]; reason != "NOT_CONFIGURED" {
			t.Fatalf("%s: reason = %#v, want NOT_CONFIGURED", operation.OperationID, reason)
		}
	}
}

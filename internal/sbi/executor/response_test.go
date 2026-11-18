package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

func TestExecuteProblemDetailsRemoteError(t *testing.T) {
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/problem+json")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, `{
			"type":"about:blank",
			"title":"Context not found",
			"status":404,
			"detail":"the requested context does not exist",
			"instance":"/ue-contexts/imsi-1",
			"cause":"CONTEXT_NOT_FOUND"
		}`)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})

	_, err := executor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) {
		t.Fatalf("expected *RemoteError, got %T: %v", err, err)
	}
	if remoteError.HTTPStatus != http.StatusNotFound || remoteError.Cause != "CONTEXT_NOT_FOUND" {
		t.Fatalf("unexpected remote error: %+v", remoteError)
	}
	if remoteError.Problem == nil || remoteError.Problem.Status != http.StatusNotFound ||
		remoteError.Problem.Title != "Context not found" ||
		remoteError.Problem.Detail != "the requested context does not exist" {
		t.Fatalf("unexpected problem details: %+v", remoteError.Problem)
	}
}

func TestExecuteMalformedProblemDetailsAndNonProblemErrors(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		status      int
		body        string
	}{
		{"status mismatch", "application/problem+json", http.StatusNotFound, `{"status":500,"cause":"INTERNAL"}`},
		{"missing cause", "application/problem+json", http.StatusBadRequest, `{"status":400}`},
		{"unsafe cause", "application/problem+json", http.StatusBadRequest, `{"status":400,"cause":"bad cause with spaces"}`},
		{"lowercase cause", "application/problem+json", http.StatusBadRequest, `{"status":400,"cause":"badCause"}`},
		{"duplicate keys", "application/problem+json", http.StatusBadRequest, `{"status":400,"cause":"A","cause":"B"}`},
		{"non-object problem", "application/problem+json", http.StatusBadRequest, `[]`},
		{"non-integer status", "application/problem+json", http.StatusBadRequest, `{"status":"400","cause":"A"}`},
		{"non-string title", "application/problem+json", http.StatusBadRequest, `{"status":400,"cause":"A","title":1}`},
		{"plain text error", "text/plain", http.StatusInternalServerError, "boom"},
		{"missing content type", "", http.StatusBadGateway, `{"cause":"X"}`},
		{"json but not problem", "application/json", http.StatusInternalServerError, `{"cause":"INTERNAL"}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if testCase.contentType != "" {
					writer.Header().Set("Content-Type", testCase.contentType)
				}
				writer.WriteHeader(testCase.status)
				_, _ = io.WriteString(writer, testCase.body)
			}))
			executor := newTestExecutor(t, newTestClient(t, server, nil), nil, payload.Limits{})
			_, err := executor.Execute(context.Background(), readOperation(t), map[string]any{"ueContextId": "imsi-1"})
			var protocolError *ProtocolError
			if !errors.As(err, &protocolError) {
				t.Fatalf("expected *ProtocolError, got %T: %v", err, err)
			}
		})
	}
}

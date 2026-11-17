package mas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestQueryOutcomeReturnsTyped200Result(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"lifecycle":"OPERATIONAL"}}`)
	}))
	defer server.Close()

	outcome, err := newTestClient(t, server.URL).QueryOutcome(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.HTTPStatus != http.StatusOK || outcome.Result == nil || outcome.Problem != nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if outcome.Result.Status != "AVAILABLE" || outcome.Result.MASID != "SC" {
		t.Fatalf("unexpected typed result: %#v", outcome.Result)
	}
}

func TestQueryOutcomeReturnsValidatedProblemDetails(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"title":"Not Found","status":404,"detail":"private diagnostic text imsi-001010000000001","cause":"SESSION_NOT_FOUND"}`)
	}))
	defer server.Close()

	outcome, err := newTestClient(t, server.URL).QueryOutcome(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.HTTPStatus != http.StatusNotFound || outcome.Result != nil || outcome.Problem == nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if outcome.Problem.Status != http.StatusNotFound || outcome.Problem.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected problem: %#v", outcome.Problem)
	}
}

func TestQueryOutcomeRejectsNonProblemRemoteResponses(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		status      int
		body        string
	}{
		{name: "json media type", contentType: "application/json", status: http.StatusNotFound, body: `{"status":404,"cause":"SESSION_NOT_FOUND"}`},
		{name: "missing media type", contentType: "", status: http.StatusNotFound, body: `{"status":404,"cause":"SESSION_NOT_FOUND"}`},
		{name: "status mismatch", contentType: "application/problem+json", status: http.StatusNotFound, body: `{"status":403,"cause":"FORBIDDEN"}`},
		{name: "missing cause", contentType: "application/problem+json", status: http.StatusNotFound, body: `{"status":404}`},
		{name: "unsafe cause", contentType: "application/problem+json", status: http.StatusNotFound, body: `{"status":404,"cause":"not a machine token"}`},
		{name: "duplicate key", contentType: "application/problem+json", status: http.StatusNotFound, body: `{"status":404,"status":404,"cause":"SESSION_NOT_FOUND"}`},
		{name: "non-object", contentType: "application/problem+json", status: http.StatusNotFound, body: `[]`},
		{name: "trailing data", contentType: "application/problem+json", status: http.StatusNotFound, body: `{"status":404,"cause":"SESSION_NOT_FOUND"} x`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.contentType != "" {
					w.Header().Set("Content-Type", test.contentType)
				}
				w.WriteHeader(test.status)
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			_, err := newTestClient(t, server.URL).QueryOutcome(context.Background(), Query{
				MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
			})
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
		})
	}
}

func TestQueryOutcomePropagatesTransportFailureAndCancellation(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, server.URL).QueryOutcome(ctx, Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	var protocolErr *ProtocolError
	var remoteErr *RemoteError
	if errors.As(err, &protocolErr) || errors.As(err, &remoteErr) {
		t.Fatalf("cancellation must not be mapped to remote/protocol error: %v", err)
	}
}

func TestQueryOutcomeInputValidationBeforeNetwork(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("network was used for an invalid query")
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).QueryOutcome(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(300),
	})
	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if strings.Contains(err.Error(), "imsi-") {
		t.Fatalf("input error leaked identity: %v", err)
	}
}

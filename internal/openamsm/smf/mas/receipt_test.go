package mas

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type receiptTransport struct {
	path   string
	body   []byte
	status int
	reply  string
}

func (t *receiptTransport) Do(ctx context.Context, path, contentType, accept string, body []byte) (*http.Response, error) {
	t.path, t.body = path, body
	status := t.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(t.reply)),
	}, nil
}

func (t *receiptTransport) CloseIdleConnections() {}

func TestReceiptQueryReturnsTheOwnerReceipt(t *testing.T) {
	transport := &receiptTransport{reply: `{"status":"OK","receipt":{"state":"COMMITTED","originalInvocationId":"occ-1"}}`}
	invoker := &TypedInvoker{wire: transport, timeout: time.Second}
	receipt, err := invoker.ReceiptQuery(context.Background(), ReceiptScope{
		OriginalInvocationID: "occ-1",
		DeploymentID:         "deployment:test",
		OwnerInstanceID:      "owner:smf-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if transport.path != receiptPath {
		t.Fatalf("path = %q, want %q", transport.path, receiptPath)
	}
	var decoded map[string]any
	if err := json.Unmarshal(receipt, &decoded); err != nil || decoded["state"] != "COMMITTED" {
		t.Fatalf("receipt = %s, err = %v", receipt, err)
	}
	var request map[string]string
	if err := json.Unmarshal(transport.body, &request); err != nil ||
		request["originalInvocationId"] != "occ-1" {
		t.Fatalf("request body = %s", transport.body)
	}
}

func TestReconcileIsReadOnlyAndCarriesTheAuditFields(t *testing.T) {
	transport := &receiptTransport{reply: `{"resolved":false,"reDispatched":false,"reason":"OWNER_CANNOT_PROVE_ORIGINAL_OUTCOME"}`}
	invoker := &TypedInvoker{wire: transport, timeout: time.Second}
	decision, err := invoker.Reconcile(context.Background(), ReceiptScope{
		OriginalInvocationID: "occ-1",
		DeploymentID:         "deployment:test",
		OwnerInstanceID:      "owner:smf-1",
	}, "acceptance", "crash recovery check")
	if err != nil {
		t.Fatal(err)
	}
	if transport.path != reconcilePath {
		t.Fatalf("path = %q, want %q", transport.path, reconcilePath)
	}
	var decoded map[string]any
	if err := json.Unmarshal(decision, &decoded); err != nil || decoded["resolved"] != false {
		t.Fatalf("decision = %s, err = %v", decision, err)
	}
	var request map[string]string
	if err := json.Unmarshal(transport.body, &request); err != nil ||
		request["operator"] != "acceptance" || request["reason"] != "crash recovery check" {
		t.Fatalf("request body = %s", transport.body)
	}
}

func TestReceiptReadsRejectInvalidScopeBeforeIO(t *testing.T) {
	transport := &receiptTransport{reply: `{}`}
	invoker := &TypedInvoker{wire: transport, timeout: time.Second}
	if _, err := invoker.ReceiptQuery(context.Background(), ReceiptScope{OriginalInvocationID: ""}); err == nil {
		t.Fatal("empty scope must fail closed")
	}
	if _, err := invoker.Reconcile(context.Background(), ReceiptScope{
		OriginalInvocationID: "occ-1", DeploymentID: "deployment:test", OwnerInstanceID: "owner:smf-1",
	}, "", "reason"); err == nil {
		t.Fatal("empty operator must fail closed")
	}
	if transport.path != "" {
		t.Fatalf("invalid scope performed I/O on %q", transport.path)
	}
}

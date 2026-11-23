package mas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ReceiptScope is the exact Owner occurrence the durable receipt/reconcile
// reads are bound to. It is Host-supplied correlation, never an operation
// argument of a MAS business callable.
type ReceiptScope struct {
	OriginalInvocationID string
	DeploymentID         string
	OwnerInstanceID      string
}

func (s ReceiptScope) valid() bool {
	return privateRef(s.OriginalInvocationID) && privateRef(s.DeploymentID) &&
		privateRef(s.OwnerInstanceID)
}

const (
	receiptPath   = "/nsmf-mas-orchestration/v1/receipt"
	reconcilePath = "/nsmf-mas-orchestration/v1/reconcile"
)

// ReceiptQuery reads the Owner durable receipt for one exact original
// invocation. A PREPARED record comes back as state UNKNOWN; the call never
// dispatches or re-dispatches a mutation.
func (a *TypedInvoker) ReceiptQuery(ctx context.Context, scope ReceiptScope) (json.RawMessage, error) {
	if !scope.valid() {
		return nil, errInvokeInput
	}
	body, err := json.Marshal(map[string]string{
		"originalInvocationId": scope.OriginalInvocationID,
		"deploymentId":         scope.DeploymentID,
		"ownerInstanceId":      scope.OwnerInstanceID,
	})
	if err != nil {
		return nil, errInvokeInput
	}
	decoded, err := a.ownerRead(ctx, receiptPath, body)
	if err != nil {
		return nil, err
	}
	receipt, ok := decoded["receipt"]
	if !ok || len(receipt) == 0 {
		return nil, fmt.Errorf("owner receipt response is missing the receipt")
	}
	return receipt, nil
}

// Reconcile asks the Owner for the exact-original decision. It is read-only:
// the returned decision can resolve an UNKNOWN only when the Owner can prove
// it, and never causes another destructive dispatch.
func (a *TypedInvoker) Reconcile(ctx context.Context, scope ReceiptScope, operator, reason string) (json.RawMessage, error) {
	if !scope.valid() || !privateRef(operator) || len(reason) == 0 || len(reason) > 512 {
		return nil, errInvokeInput
	}
	body, err := json.Marshal(map[string]string{
		"originalInvocationId": scope.OriginalInvocationID,
		"deploymentId":         scope.DeploymentID,
		"ownerInstanceId":      scope.OwnerInstanceID,
		"operator":             operator,
		"reason":               reason,
	})
	if err != nil {
		return nil, errInvokeInput
	}
	decoded, err := a.ownerRead(ctx, reconcilePath, body)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func (a *TypedInvoker) ownerRead(ctx context.Context, path string, body []byte) (map[string]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	response, err := a.wire.Do(ctx, path, "application/json", "application/json", body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, defaultMaxResponse+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > defaultMaxResponse {
		return nil, fmt.Errorf("owner read response is too large")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("owner read failed with HTTP %d", response.StatusCode)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded == nil {
		return nil, fmt.Errorf("owner read response is not a JSON object")
	}
	return decoded, nil
}


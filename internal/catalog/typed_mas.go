package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
)

// HandlerMASInvoke is the typed OpenAMSM MAS orchestration handler. Unlike the
// genericSBI handler it cannot express an arbitrary URL: the endpoint is fixed
// in source and every mas/op/stepId comes from the reviewed callable table.
const HandlerMASInvoke = "openamsmMASInvoke"

// HandlerMASReceipt and HandlerMASReconcile are the read-only Owner evidence
// handlers. They use the same private transport and token as the typed invoke
// handler but expose only the fixed receipt/reconcile resources.
const (
	HandlerMASReceipt   = "openamsmMASReceipt"
	HandlerMASReconcile = "openamsmMASReconcile"
)

// MASInvokeServiceKey is the trusted service key of the private MAS
// orchestration endpoint.
const MASInvokeServiceKey = "openamsm-mas-invoke"

// masInvokePath is the only path the MAS typed handler can ever call.
const masInvokePath = "/nsmf-mas-orchestration/v1/invoke"

const (
	masReceiptPath   = "/nsmf-mas-orchestration/v1/receipt"
	masReconcilePath = "/nsmf-mas-orchestration/v1/reconcile"
)

// masInvokeSourceID names the reviewed source of the typed callable table.
const masInvokeSourceID = "openamsm-mas-orchestration-openapi"

// InstallTypedMASBindings appends the fixed typed MAS callables to a loaded
// catalog. The table is trusted Go content derived from the same source as the
// wire projection, so it cannot drift from the Owner contract. The reported
// catalog digest is extended deterministically so a Host can tell the exact
// binding closure it negotiated.
func InstallTypedMASBindings(target *Catalog) error {
	if target == nil {
		return fmt.Errorf("catalog: typed MAS install requires a loaded catalog")
	}
	typed := mas.TypedBindings()
	added := make([]Binding, 0, len(typed))
	for _, entry := range typed {
		if _, duplicate := target.index[entry.OperationID()]; duplicate {
			return fmt.Errorf("catalog: typed MAS binding %q already exists", entry.OperationID())
		}
		effect := EffectMutation
		if entry.ReadOnly {
			effect = EffectRead
		}
		binding := Binding{
			BackendOperationID: entry.OperationID(),
			Effect:             effect,
			Handler:            HandlerMASInvoke,
			ServiceKey:         MASInvokeServiceKey,
			SourceID:           masInvokeSourceID,
			SourceOperationID:  entry.Callable,
			Method:             "POST",
			Path:               masInvokePath,
			RequestMediaType:   "application/json",
			ResponseMediaTypes: []string{"application/json"},
			Arguments:          typedArguments(entry),
		}
		added = append(added, binding)
	}
	added = append(added, masEvidenceBindings()...)
	encoded, err := json.Marshal(added)
	if err != nil {
		return fmt.Errorf("catalog: typed MAS binding closure is not encodable")
	}
	target.Bindings = append(target.Bindings, added...)
	if err := target.buildIndex(); err != nil {
		return err
	}
	// Domain-separated extension digest: the base artifact bytes keep their
	// pinned identity, and the typed closure is bound into one value.
	sum := sha256.Sum256([]byte("CA-CLIENT-TOOLS-TYPED-MAS\x00v1\x00" + target.Digest + "\x00" + string(encoded)))
	target.Digest = "sha256:" + hex.EncodeToString(sum[:])
	return nil
}

func typedArguments(entry mas.TypedBinding) []Argument {
	arguments := make([]Argument, 0, len(entry.Fields))
	for _, field := range entry.Fields {
		arguments = append(arguments, Argument{
			Name:     field.Name,
			Type:     field.Type,
			Required: field.Required,
		})
	}
	return arguments
}

// masEvidenceBindings are the read-only Owner evidence reads. They are not MAS
// business callables: the Owner ToolCatalog declares them as READ operations
// (MAS.RECEIPT_QUERY / MAS.RECONCILE) and they never dispatch a mutation.
func masEvidenceBindings() []Binding {
	common := []Argument{
		{Name: "originalInvocationId", Type: "string", Required: true},
		{Name: "deploymentId", Type: "string", Required: true},
		{Name: "ownerInstanceId", Type: "string", Required: true},
	}
	reconcile := append(append([]Argument(nil), common...),
		Argument{Name: "operator", Type: "string", Required: true},
		Argument{Name: "reason", Type: "string", Required: true},
	)
	return []Binding{
		{
			BackendOperationID: "openamsm.smf.mas.receipt.query",
			Effect:             EffectRead,
			Handler:            HandlerMASReceipt,
			ServiceKey:         MASInvokeServiceKey,
			SourceID:           masInvokeSourceID,
			SourceOperationID:  "MAS.RECEIPT_QUERY",
			Method:             "POST",
			Path:               masReceiptPath,
			RequestMediaType:   "application/json",
			ResponseMediaTypes: []string{"application/json"},
			Arguments:          common,
		},
		{
			BackendOperationID: "openamsm.smf.mas.reconcile",
			Effect:             EffectRead,
			Handler:            HandlerMASReconcile,
			ServiceKey:         MASInvokeServiceKey,
			SourceID:           masInvokeSourceID,
			SourceOperationID:  "MAS.RECONCILE",
			Method:             "POST",
			Path:               masReconcilePath,
			RequestMediaType:   "application/json",
			ResponseMediaTypes: []string{"application/json"},
			Arguments:          reconcile,
		},
	}
}

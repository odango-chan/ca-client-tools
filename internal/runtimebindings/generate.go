// Package runtimebindings generates the private Runtime-only binding sidecar
// from the pinned OpenAMSM Owner ToolCatalog publication and the Provider's own
// reviewed typed closure.
//
// The sidecar is not a public ToolCatalog: it does not enter the Compiler
// Interface Package, the CapabilityCatalog or any Skill-visible authorization.
// It carries only the exact private execution metadata the Host cannot derive:
//
//   - the exact public/private identity pair (runtimeOperationId ->
//     backendOperationId), which is verified against the Go typed closure;
//   - the Owner contract reference by package identity and digest, so the
//     Effect, Owner Outcomes, input/output Schema, receipt, commit proof and
//     reconcile semantics stay owned by OpenAMSM instead of being redefined;
//   - where the Provider puts its observation envelope and how its declared
//     observation codes map to Owner/Runtime dispositions;
//   - where the Owner durable receipt sits, and which of its fields must bind
//     the exact Host invocation scope;
//   - how to build the exact-original reconcile call and read its decision;
//   - the Host-private invocation scope and private argument slots.
//
// Every one of those values is read from the pinned Owner publication or from
// the reviewed Go tables; this package authors no business semantics.
package runtimebindings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
)

// OwnerDirectory is the pinned Owner publication directory, relative to the
// repository root.
const OwnerDirectory = "contracts/owner/openamsm-mas/v1alpha4"

// SidecarName is the exact runtime-only sidecar file name.
const SidecarName = "runtime-bindings.yaml"

// SidecarAPIVersion is the Host-facing sidecar identity.
const SidecarAPIVersion = "ca.tool-provider-runtime-bindings/v1"

// SchemaDirectory is the package-relative directory of the Schema exports the
// sidecar references.
const SchemaDirectory = "runtime/schemas"

const (
	ownerCallablePrefix = "urn:openamsm:mas:v1alpha4:"
	schemaIDSuffixIn    = "-input"
	schemaIDSuffixOut   = "-output"
)

// legacyUnpublished lists the callables the Go Provider still contains but the
// Owner deliberately did not publish. The value is the audited reason.
//
// The first seven are declared by the Owner manifest's notPublished list. The
// eighth, UPN.MODIFY_QOS_FLOWS, is a legacy OES step that the Owner replaced
// with UPN.MODIFY_QOS_DELTA for Scheme C; it is not exported by any Owner
// contract, so it must never appear in the public or Runtime binding surface.
var legacyUnpublished = map[string]string{
	"SC.UPDATE_SM_CONTEXT":         "legacy OES step m0; no merged Scheme C transaction",
	"UPN.MODIFY_UP_SESSION_INTENT": "legacy OES step m3; no merged Scheme C transaction",
	"SSM.SIGNAL_UE_AN_MODIFY":      "legacy OES step m4; Scheme C Modify uses UPN.MODIFY_QOS_DELTA",
	"SSM.SIGNAL_UE_AN_RELEASE":     "legacy OES step r3; Scheme C Release uses parse/build/confirm",
	"PA.DELETE_SM_POLICY":          "legacy OES step r4; Scheme C uses PA.DELETE_SM_POLICY_C",
	"SC.RELEASE_SM_CONTEXT":        "legacy OES step r5; Scheme C uses SC.RELEASE_SM_CONTEXT_C",
	"SE.EMIT_RELEASE_EVENT":        "legacy OES step r6; Scheme C uses SE.EMIT_RELEASE_EVENT_C",
	"UPN.MODIFY_QOS_FLOWS":         "legacy OES QoS flow list; Owner publishes UPN.MODIFY_QOS_DELTA",
}

// Options are the exact inputs of one deterministic generation.
type Options struct {
	OwnerDir        string
	ProviderVersion string
}

// GeneratedFile is one deterministic package file, relative to the package root.
type GeneratedFile struct {
	Path  string
	Bytes []byte
}

// Package is the generated sidecar plus the Schema exports it references.
type Package struct {
	OwnerPublicationDigest string
	OwnerManifestDigest    string
	Bindings               int
	MutationBindings       int
	ReadBindings           int
	Files                  []GeneratedFile
	Sidecar                []byte
}

// Generate builds the sidecar and its Schema exports in memory.
func Generate(options Options) (*Package, error) {
	if options.OwnerDir == "" {
		options.OwnerDir = OwnerDirectory
	}
	if strings.TrimSpace(options.ProviderVersion) == "" {
		return nil, fmt.Errorf("runtimebindings: provider version is required")
	}
	owner, err := loadOwner(options.OwnerDir)
	if err != nil {
		return nil, err
	}
	business, evidence, err := auditedClosure(owner)
	if err != nil {
		return nil, err
	}

	registry := newSchemaRegistry(owner)
	for _, operation := range owner.Manifest.Operations {
		contract, ok := owner.Contracts[operation.Callable]
		if !ok {
			return nil, fmt.Errorf("runtimebindings: Owner operation %q has no ToolCatalog contract", operation.Callable)
		}
		for _, reference := range []string{contract.InputSchema.Ref, contract.OutputSchema.Ref} {
			document, err := registry.resolveReference(reference)
			if err != nil {
				return nil, fmt.Errorf("runtimebindings: %s: %w", operation.Callable, err)
			}
			if _, err := registry.export(reference, document); err != nil {
				return nil, err
			}
		}
		if contract.ReceiptSchema != nil {
			document, err := registry.resolveReference(contract.ReceiptSchema.Ref)
			if err != nil {
				return nil, fmt.Errorf("runtimebindings: %s receipt: %w", operation.Callable, err)
			}
			if _, err := registry.export(contract.ReceiptSchema.Ref, document); err != nil {
				return nil, err
			}
		}
	}

	bindings := make([]bindingDocument, 0, len(owner.Manifest.Operations))
	for _, operation := range owner.Manifest.Operations {
		binding, err := buildBinding(owner, registry, operation)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].RuntimeOperationID < bindings[j].RuntimeOperationID
	})
	_ = business
	_ = evidence

	files, err := registry.exportFiles()
	if err != nil {
		return nil, err
	}
	digests := make([]fileDigest, 0, len(files))
	for _, file := range files {
		digests = append(digests, fileDigest{Path: file.Path, SHA256: sha256Hex(file.Bytes)})
	}
	sort.Slice(digests, func(i, j int) bool { return digests[i].Path < digests[j].Path })

	document := sidecarDocument{
		APIVersion:      SidecarAPIVersion,
		ProviderID:      "ca-client-tools.sbi",
		ProviderVersion: options.ProviderVersion,
		OwnerContract: ownerContractDocument{
			StableID:          owner.Manifest.Package.StableID,
			Version:           owner.Manifest.Package.Version,
			PublicationDigest: owner.Manifest.PublicationDigest,
			ManifestDigest:    owner.ManifestDigest,
			Operations:        owner.Manifest.OperationCount,
		},
		Files:    digests,
		Bindings: bindings,
	}
	encoded, err := encodeYAML(document)
	if err != nil {
		return nil, err
	}
	all := append([]GeneratedFile{{Path: SidecarName, Bytes: encoded}}, files...)
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })

	mutationBindings := 0
	for _, binding := range bindings {
		if binding.Effect == "MUTATION" {
			mutationBindings++
		}
	}
	return &Package{
		OwnerPublicationDigest: owner.Manifest.PublicationDigest,
		OwnerManifestDigest:    owner.ManifestDigest,
		Bindings:               len(bindings),
		MutationBindings:       mutationBindings,
		ReadBindings:           len(bindings) - mutationBindings,
		Files:                  all,
		Sidecar:                encoded,
	}, nil
}

// Verify regenerates the sidecar from the pinned Owner publication and requires
// a byte-identical result.
func Verify(outDir string, options Options) error {
	generated, err := Generate(options)
	if err != nil {
		return err
	}
	for _, file := range generated.Files {
		target := filepath.Join(outDir, filepath.FromSlash(file.Path))
		actual, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("runtimebindings: read %s: %w", file.Path, err)
		}
		if string(actual) != string(file.Bytes) {
			return fmt.Errorf("runtimebindings: %s differs from the generated release content", file.Path)
		}
	}
	return nil
}

// auditedClosure proves the Go typed closure and the Owner publication are the
// same business surface modulo the audited legacy set.
func auditedClosure(owner *ownerPackage) ([]string, []string, error) {
	typed := mas.TypedBindings()
	goCallables := make(map[string]mas.TypedBinding, len(typed))
	for _, binding := range typed {
		goCallables[binding.Callable] = binding
	}
	ownerCallables := make(map[string]ownerOperation, len(owner.Manifest.Operations))
	for _, operation := range owner.Manifest.Operations {
		ownerCallables[operation.Callable] = operation
	}

	var missingImplementation []string
	business := make([]string, 0, len(ownerCallables))
	for callable := range ownerCallables {
		operation := ownerCallables[callable]
		bind, ok := goCallables[callable]
		if !ok {
			if operation.Table == "evidence" {
				business = append(business, callable)
				continue
			}
			missingImplementation = append(missingImplementation, callable)
			continue
		}
		if err := verifyEffect(operation, bind); err != nil {
			return nil, nil, err
		}
		business = append(business, callable)
	}
	if len(missingImplementation) > 0 {
		sort.Strings(missingImplementation)
		return nil, nil, fmt.Errorf(
			"runtimebindings: Owner callables have no Go implementation: %s",
			strings.Join(missingImplementation, ","),
		)
	}

	var unexpected []string
	for callable := range goCallables {
		if _, published := ownerCallables[callable]; published {
			continue
		}
		if _, audited := legacyUnpublished[callable]; !audited {
			unexpected = append(unexpected, callable)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return nil, nil, fmt.Errorf(
			"runtimebindings: Go callables are neither published nor audited as legacy: %s",
			strings.Join(unexpected, ","),
		)
	}
	var staleLegacy []string
	for callable := range legacyUnpublished {
		if _, published := ownerCallables[callable]; published {
			staleLegacy = append(staleLegacy, callable)
		}
		if _, exists := goCallables[callable]; !exists {
			staleLegacy = append(staleLegacy, callable)
		}
	}
	if len(staleLegacy) > 0 {
		sort.Strings(staleLegacy)
		return nil, nil, fmt.Errorf(
			"runtimebindings: audited legacy callables are stale: %s",
			strings.Join(staleLegacy, ","),
		)
	}
	for callable, reason := range owner.Manifest.NotPublishedIndex {
		if want, ok := legacyUnpublished[callable]; !ok || strings.TrimSpace(want) == "" {
			return nil, nil, fmt.Errorf("runtimebindings: Owner notPublished %q is not audited", callable)
		}
		if strings.TrimSpace(reason) == "" {
			return nil, nil, fmt.Errorf("runtimebindings: Owner notPublished %q has no reason", callable)
		}
	}

	evidence := make([]string, 0, 2)
	for callable, operation := range ownerCallables {
		if operation.Table == "evidence" {
			evidence = append(evidence, callable)
		}
	}
	sort.Strings(business)
	sort.Strings(evidence)
	return business, evidence, nil
}

func verifyEffect(operation ownerOperation, bind mas.TypedBinding) error {
	want := strings.ToUpper(strings.TrimSpace(operation.Effect))
	got := "MUTATION"
	if bind.ReadOnly {
		got = "READ"
	}
	if want != got {
		return fmt.Errorf(
			"runtimebindings: %s Effect differs: owner=%s provider=%s",
			operation.Callable, want, got,
		)
	}
	return nil
}

func buildBinding(
	owner *ownerPackage,
	registry *schemaRegistry,
	operation ownerOperation,
) (bindingDocument, error) {
	contract := owner.Contracts[operation.Callable]
	effect := strings.ToUpper(strings.TrimSpace(operation.Effect))
	if effect != "READ" && effect != "MUTATION" {
		return bindingDocument{}, fmt.Errorf("runtimebindings: %s has unsupported Effect %q", operation.Callable, operation.Effect)
	}
	if effect == "MUTATION" && contract.ReceiptSchema == nil {
		return bindingDocument{}, fmt.Errorf("runtimebindings: %s is MUTATION without an Owner receipt Schema", operation.Callable)
	}

	backendID := ""
	privateNames := []privateArgument{}
	hostInvocation := (*hostInvocationDocument)(nil)
	ownerMAS, ownerOp, ownerStep := "", "", ""
	if operation.Table != "evidence" {
		bind, ok := mas.LookupTypedBinding(operation.Callable)
		if !ok {
			return bindingDocument{}, fmt.Errorf("runtimebindings: %s has no typed binding", operation.Callable)
		}
		backendID = bind.OperationID()
		hostInvocation = businessHostInvocation(bind)
		privateNames = privateArgumentsFor(operation.Callable)
		ownerMAS, ownerOp, ownerStep = bind.MAS, bind.Op, bind.Step
	} else {
		evidence, err := evidenceBinding(operation.Callable)
		if err != nil {
			return bindingDocument{}, err
		}
		backendID = evidence
	}

	inputPath, err := registry.exportPath(contract.InputSchema.Ref)
	if err != nil {
		return bindingDocument{}, err
	}
	outputPath, err := registry.exportPath(contract.OutputSchema.Ref)
	if err != nil {
		return bindingDocument{}, err
	}
	binding := bindingDocument{
		RuntimeBindingID:   runtimeBindingID(operation.Callable),
		RuntimeOperationID: operation.Callable,
		BackendOperationID: backendID,
		OwnerCallableID:    operation.Callable,
		Effect:             effect,
		CapabilityID:       operation.Capability,
		OwnerOutcomes:      append([]string(nil), contract.OwnerOutcomes...),
		InputSchema:        schemaReference{Ref: inputPath},
		OutputSchema:       schemaReference{Ref: outputPath},
		HostInvocation:     hostInvocation,
		PrivateArguments:   privateNames,
	}
	if effect == "READ" {
		binding.OwnerOutcomes = []string{}
		if operation.Table != "evidence" {
			binding.Observation = providerObservation("READ")
		}
		return binding, nil
	}
	binding.Observation = providerObservation("MUTATION")
	if contract.ReceiptSchema == nil || contract.Mutation == nil {
		return bindingDocument{}, fmt.Errorf("runtimebindings: %s is MUTATION without an Owner commit-proof contract", operation.Callable)
	}
	receiptPath, err := registry.exportPath(contract.ReceiptSchema.Ref)
	if err != nil {
		return bindingDocument{}, err
	}
	binding.CommitProof = &commitProofDocument{
		Mode:          "OWNER_RECEIPT",
		ReceiptSchema: schemaReference{Ref: receiptPath},
		Receipt: receiptDocument{
			Path:               "receipt",
			SchemaVersionField: "schemaVersion",
			SchemaVersion:      owner.ReceiptSchemaIdentity,
			InvocationIDField:  "originalInvocationId",
			StateField:         "state",
			States: map[string]string{
				"COMMITTED":     "COMMITTED",
				"NOT_COMMITTED": "NOT_COMMITTED",
				"UNKNOWN":       "UNKNOWN",
			},
			Bindings: []receiptBinding{
				{Field: "operation.callable", Source: "RUNTIME_OPERATION_ID"},
				{Field: "operation.mas", Source: "LITERAL", Value: ownerMAS},
				{Field: "operation.op", Source: "LITERAL", Value: ownerOp},
				{Field: "operation.stepId", Source: "LITERAL", Value: ownerStep},
				{Field: "owner.deploymentId", Source: "HOST_SCOPE", HostScopeField: "deploymentId"},
				{Field: "owner.instanceId", Source: "HOST_SCOPE", HostScopeField: "ownerInstanceId"},
				{Field: "scope.supi", Source: "HOST_SCOPE", HostScopeField: "supi"},
				{Field: "scope.psi", Source: "HOST_SCOPE", HostScopeField: "psi"},
			},
			Commit: &commitBindingDocument{
				Path:              "commit",
				StateField:        "commitState",
				CommittedValue:    "COMMITTED",
				InvocationIDField: "originalInvocationId",
			},
		},
		Reconcile: &reconcileDocument{
			BindingID: "mas-reconcile",
			Arguments: []reconcileArgument{
				{Field: "originalInvocationId", Source: "ORIGINAL_INVOCATION_ID"},
				{Field: "deploymentId", Source: "HOST_SCOPE", HostScopeField: "deploymentId"},
				{Field: "ownerInstanceId", Source: "HOST_SCOPE", HostScopeField: "ownerInstanceId"},
				{Field: "operator", Source: "LITERAL", Value: "ca-agent-host"},
				{Field: "reason", Source: "LITERAL", Value: "exact-original reconcile"},
			},
			Decision: decisionDocument{
				Path:          "response.decision",
				ResolvedField: "resolved",
				StateField:    "state",
				ReceiptPath:   "receipt",
			},
		},
	}
	return binding, nil
}

func businessHostInvocation(bind mas.TypedBinding) *hostInvocationDocument {
	arguments := []scopeArgument{}
	for _, field := range bind.Fields {
		switch field.Name {
		case "ue_identity":
			arguments = append(arguments, scopeArgument{Argument: "ue_identity", ScopeField: "supi"})
		case "psi":
			arguments = append(arguments, scopeArgument{Argument: "psi", ScopeField: "psi"})
		}
	}
	return &hostInvocationDocument{
		Version:   mas.HostInvocationVersion,
		Arguments: arguments,
		Fields: []scopeField{
			{Name: "deploymentId", Source: "HOST_CONTEXT", Required: true},
			{Name: "ownerInstanceId", Source: "HOST_CONTEXT", Required: true},
			{Name: "originalInvocationId", Source: "DERIVED_ORIGINAL_INVOCATION_ID", Required: true},
			{Name: "sessionRef", Source: "HOST_CONTEXT", Required: true},
			{Name: "supi", Source: "HOST_CONTEXT", Required: true},
			{Name: "psi", Source: "HOST_CONTEXT", Required: true, Type: "integer", Minimum: intPointer(1), Maximum: intPointer(15)},
			{Name: "smContextRef", Source: "HOST_CONTEXT", Required: false},
		},
	}
}

func privateArgumentsFor(callable string) []privateArgument {
	switch callable {
	case "SSM.PARSE_N1_MODIFY", "SSM.PARSE_N1_RELEASE":
		return []privateArgument{{Name: "n1_sm", Source: "PRIVATE_MATERIAL"}}
	case "SSM.BUILD_RELEASE_COMMAND":
		return []privateArgument{{Name: "release_txn", Source: "PRIVATE_MATERIAL"}}
	default:
		return []privateArgument{}
	}
}

func evidenceBinding(callable string) (string, error) {
	for _, binding := range catalog.RuntimeEvidenceBindings() {
		if binding.SourceOperationID == callable {
			return binding.BackendOperationID, nil
		}
	}
	return "", fmt.Errorf("runtimebindings: %s has no Provider evidence binding", callable)
}

func providerObservation(effect string) *observationDocument {
	document := &observationDocument{
		EnvelopePath: "response.observation",
		CodeField:    "code",
		OutputsField: "outputs",
		Outcomes: map[string]string{
			"ACL_DENIED":       "DENIED",
			"OWNER_TIMEOUT":    "UNKNOWN",
			"OWNER_ERROR":      "UNKNOWN",
			"TRANSPORT_ERROR":  "UNKNOWN",
			"TIMEOUT":          "UNKNOWN",
			"CANCELED":         "UNKNOWN",
			"INVALID_RESPONSE": "UNKNOWN",
		},
	}
	if effect == "MUTATION" {
		document.Outcomes["WIRE_OK"] = "RECEIPT"
	} else {
		document.Outcomes["WIRE_OK"] = "OUTPUTS"
	}
	return document
}

func runtimeBindingID(callable string) string {
	slug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(callable, ".", "-"), "_", "-"))
	return slug
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func intPointer(value int) *int { return &value }

func encodeYAML(value any) ([]byte, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("runtimebindings: encode YAML: %w", err)
	}
	return append(encoded, '\n'), nil
}

func encodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("runtimebindings: encode JSON: %w", err)
	}
	return append(encoded, '\n'), nil
}

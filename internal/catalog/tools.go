package catalog

import (
	"fmt"
	"net/url"
	"strings"
)

// Owner outcome vocabulary accepted for a public Tool contract. SUCCEEDED is
// mandatory; FAILED and NOT_COMMITTED are the only additional outcomes the
// frozen ToolCatalog may declare.
const (
	OutcomeSucceeded    = "SUCCEEDED"
	OutcomeFailed       = "FAILED"
	OutcomeNotCommit    = "NOT_COMMITTED"
	toolKindOperation   = "operation"
	toolKindObservation = "observation"
)

// validExportID applies the frozen Callable/Capability ExportId grammar:
// dot-separated non-empty segments, each starting with an ASCII letter and
// continuing with letters, digits, '_' or '-'.
func validExportID(value string) bool {
	if value == "" || len(value) > maxExportIDLength {
		return false
	}
	for _, segment := range strings.Split(value, ".") {
		if segment == "" {
			return false
		}
		first := segment[0]
		if !(first >= 'a' && first <= 'z') && !(first >= 'A' && first <= 'Z') {
			return false
		}
		for index := 1; index < len(segment); index++ {
			c := segment[index]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '_' || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// validSchemaURI accepts an absolute Schema identity URI without a fragment.
func validSchemaURI(value string) bool {
	if value == "" || len(value) > maxSchemaURILength {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Fragment != "" || parsed.Scheme == "" {
		return false
	}
	if parsed.Scheme == "urn" {
		return len(strings.TrimPrefix(value, "urn:")) > 0 && !strings.Contains(value, " ")
	}
	return parsed.Host != "" && !strings.Contains(value, " ")
}

// validateV2Tools validates the canonical public Tool contracts and their
// standalone Schema sources. It proves the Tool definition is complete,
// internally unambiguous and consistent with the private binding it names.
func validateV2Tools(file *v2File) (*InterfaceIdentity, []Tool, []SchemaSource, error) {
	if file.Metadata.Interface == nil {
		if len(file.Spec.Tools) != 0 || len(file.Spec.SchemaFiles) != 0 {
			return nil, nil, nil, catalogError("metadata.interface is required when public tools or schema files are declared")
		}
		return nil, nil, nil, nil
	}
	identity := &InterfaceIdentity{
		PackageID: file.Metadata.Interface.PackageID,
		Name:      file.Metadata.Interface.Name,
		Publisher: file.Metadata.Interface.Publisher,
		Owner:     file.Metadata.Interface.Owner,
	}
	if !validSchemaURI(identity.PackageID) {
		return nil, nil, nil, catalogError("metadata.interface.packageId must be an absolute URI")
	}
	if identity.Name == "" || identity.Publisher == "" || identity.Owner == "" {
		return nil, nil, nil, catalogError("metadata.interface name, publisher and owner must not be empty")
	}

	schemaIDs := make(map[string]struct{}, len(file.Spec.SchemaFiles))
	schemaPaths := make(map[string]struct{}, len(file.Spec.SchemaFiles))
	sources := make([]SchemaSource, 0, len(file.Spec.SchemaFiles))
	for position, schemaFile := range file.Spec.SchemaFiles {
		where := fmt.Sprintf("spec.schemaFiles[%d]", position)
		if !validSchemaURI(schemaFile.ID) {
			return nil, nil, nil, fmt.Errorf("catalog: %s.id must be an absolute Schema URI", where)
		}
		if _, duplicate := schemaIDs[schemaFile.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("catalog: %s: duplicate Schema id %q", where, schemaFile.ID)
		}
		schemaIDs[schemaFile.ID] = struct{}{}
		if err := validateRelativePath(schemaFile.Path, where+".path"); err != nil {
			return nil, nil, nil, err
		}
		if !strings.HasPrefix(schemaFile.Path, "schemas/") {
			return nil, nil, nil, fmt.Errorf("catalog: %s.path must be under schemas/", where)
		}
		if _, duplicate := schemaPaths[schemaFile.Path]; duplicate {
			return nil, nil, nil, fmt.Errorf("catalog: %s: duplicate Schema path %q", where, schemaFile.Path)
		}
		schemaPaths[schemaFile.Path] = struct{}{}
		sources = append(sources, SchemaSource{ID: schemaFile.ID, Path: schemaFile.Path})
	}

	resolve := func(ref *v2SchemaRef, where string) (string, error) {
		if ref == nil {
			return "", nil
		}
		if !validSchemaURI(ref.Ref) {
			return "", fmt.Errorf("catalog: %s.$ref must be an absolute Schema URI", where)
		}
		if _, declared := schemaIDs[ref.Ref]; !declared {
			return "", fmt.Errorf("catalog: %s.$ref %q is not declared in spec.schemaFiles", where, ref.Ref)
		}
		return ref.Ref, nil
	}

	tools := make([]Tool, 0, len(file.Spec.Tools))
	callableIDs := make(map[string]struct{}, len(file.Spec.Tools))
	backendIDs := make(map[string]struct{}, len(file.Spec.Tools))
	for position := range file.Spec.Tools {
		raw := &file.Spec.Tools[position]
		where := fmt.Sprintf("spec.tools[%d]", position)
		if !validExportID(raw.CallableID) {
			return nil, nil, nil, fmt.Errorf("catalog: %s.callableId must be a Callable ExportId", where)
		}
		if _, duplicate := callableIDs[raw.CallableID]; duplicate {
			return nil, nil, nil, fmt.Errorf("catalog: %s: duplicate callableId %q", where, raw.CallableID)
		}
		callableIDs[raw.CallableID] = struct{}{}
		if _, duplicate := backendIDs[raw.BackendOperationID]; duplicate {
			return nil, nil, nil, fmt.Errorf("catalog: %s: duplicate backendOperationId %q", where, raw.BackendOperationID)
		}
		backendIDs[raw.BackendOperationID] = struct{}{}
		binding, exists := findRawBinding(file.Spec.Bindings, raw.BackendOperationID)
		if !exists {
			return nil, nil, nil, fmt.Errorf("catalog: %s.backendOperationId %q is not declared in spec.bindings", where, raw.BackendOperationID)
		}
		if raw.Effect != binding.Effect {
			return nil, nil, nil, fmt.Errorf("catalog: %s.effect %q differs from binding %q effect %q", where, raw.Effect, raw.BackendOperationID, binding.Effect)
		}
		switch raw.Kind {
		case toolKindOperation:
			if raw.InputSchema == nil {
				return nil, nil, nil, fmt.Errorf("catalog: %s: operation tools must declare inputSchema", where)
			}
			if raw.QueryContextSchema != nil {
				return nil, nil, nil, fmt.Errorf("catalog: %s: operation tools must not declare queryContextSchema", where)
			}
		case toolKindObservation:
			if raw.QueryContextSchema == nil {
				return nil, nil, nil, fmt.Errorf("catalog: %s: observation tools must declare queryContextSchema", where)
			}
			if raw.InputSchema != nil {
				return nil, nil, nil, fmt.Errorf("catalog: %s: observation tools must not declare inputSchema", where)
			}
			if raw.Effect != string(EffectRead) {
				return nil, nil, nil, fmt.Errorf("catalog: %s: observation tools must be READ", where)
			}
		default:
			return nil, nil, nil, fmt.Errorf("catalog: %s.kind must be operation or observation", where)
		}
		switch raw.Effect {
		case string(EffectRead), string(EffectMutation):
		default:
			return nil, nil, nil, fmt.Errorf("catalog: %s.effect must be READ or MUTATION", where)
		}
		outcomes, err := validateOwnerOutcomes(raw.OwnerOutcomes, where+".ownerOutcomes")
		if err != nil {
			return nil, nil, nil, err
		}
		if len(raw.Capabilities) == 0 {
			return nil, nil, nil, fmt.Errorf("catalog: %s.capabilities must not be empty", where)
		}
		capabilities := make([]string, 0, len(raw.Capabilities))
		seenCapability := make(map[string]struct{}, len(raw.Capabilities))
		for _, capability := range raw.Capabilities {
			if !validExportID(capability) {
				return nil, nil, nil, fmt.Errorf("catalog: %s.capabilities contains an invalid CapabilityId %q", where, capability)
			}
			if _, duplicate := seenCapability[capability]; duplicate {
				return nil, nil, nil, fmt.Errorf("catalog: %s.capabilities duplicates %q", where, capability)
			}
			seenCapability[capability] = struct{}{}
			capabilities = append(capabilities, capability)
		}
		inputID, err := resolve(raw.InputSchema, where+".inputSchema")
		if err != nil {
			return nil, nil, nil, err
		}
		queryID, err := resolve(raw.QueryContextSchema, where+".queryContextSchema")
		if err != nil {
			return nil, nil, nil, err
		}
		outputID, err := resolve(raw.OutputSchema, where+".outputSchema")
		if err != nil {
			return nil, nil, nil, err
		}
		if outputID == "" {
			return nil, nil, nil, fmt.Errorf("catalog: %s.outputSchema is required", where)
		}
		receiptID, err := resolve(raw.ReceiptSchema, where+".receiptSchema")
		if err != nil {
			return nil, nil, nil, err
		}
		if raw.Effect == string(EffectMutation) {
			if err := validateMutationContract(raw, where, schemaIDs); err != nil {
				return nil, nil, nil, err
			}
		} else if raw.ReceiptSchema != nil || raw.Mutation != nil {
			return nil, nil, nil, fmt.Errorf("catalog: %s: READ tools must not declare receiptSchema or mutation", where)
		}
		tools = append(tools, Tool{
			CallableID:           raw.CallableID,
			BackendOperationID:   raw.BackendOperationID,
			Kind:                 raw.Kind,
			Effect:               Effect(raw.Effect),
			OwnerOutcomes:        outcomes,
			Capabilities:         capabilities,
			InputSchemaID:        inputID,
			QueryContextSchemaID: queryID,
			OutputSchemaID:       outputID,
			ReceiptSchemaID:      receiptID,
		})
	}
	return identity, tools, sources, nil
}

func findRawBinding(bindings []v2Binding, backendOperationID string) (v2Binding, bool) {
	for _, binding := range bindings {
		if binding.BackendOperationID == backendOperationID {
			return binding, true
		}
	}
	return v2Binding{}, false
}

func validateOwnerOutcomes(outcomes []string, where string) ([]string, error) {
	if len(outcomes) == 0 || len(outcomes) > 3 {
		return nil, fmt.Errorf("catalog: %s must declare one to three outcomes", where)
	}
	seen := make(map[string]struct{}, len(outcomes))
	hasSucceeded := false
	for _, outcome := range outcomes {
		switch outcome {
		case OutcomeSucceeded:
			hasSucceeded = true
		case OutcomeFailed, OutcomeNotCommit:
		default:
			return nil, fmt.Errorf("catalog: %s contains an unsupported outcome %q", where, outcome)
		}
		if _, duplicate := seen[outcome]; duplicate {
			return nil, fmt.Errorf("catalog: %s duplicates %q", where, outcome)
		}
		seen[outcome] = struct{}{}
	}
	if !hasSucceeded {
		return nil, fmt.Errorf("catalog: %s must contain SUCCEEDED", where)
	}
	return append([]string(nil), outcomes...), nil
}

// validateMutationContract requires the exact static mutation contract the
// frozen ToolCatalog mandates. It never invents a receipt: the referenced
// Schemas must be declared source files owned by this catalog.
func validateMutationContract(raw *v2Tool, where string, schemaIDs map[string]struct{}) error {
	if raw.ReceiptSchema == nil {
		return fmt.Errorf("catalog: %s: MUTATION tools must declare receiptSchema", where)
	}
	if raw.Mutation == nil || raw.Mutation.CommitProof == nil {
		return fmt.Errorf("catalog: %s: MUTATION tools must declare mutation.commitProof", where)
	}
	proof := raw.Mutation.CommitProof
	if _, declared := schemaIDs[proof.Schema]; !validSchemaURI(proof.Schema) || !declared {
		return fmt.Errorf("catalog: %s: mutation.commitProof.schema must be a declared Schema URI", where)
	}
	if proof.Path == "" {
		return fmt.Errorf("catalog: %s: mutation.commitProof.path is required", where)
	}
	if proof.Bind != "originalInvocationId" {
		return fmt.Errorf("catalog: %s: mutation.commitProof.bind must be originalInvocationId", where)
	}
	if raw.Mutation.Reconcile == nil {
		return nil
	}
	reconcile := raw.Mutation.Reconcile
	if !validExportID(reconcile.Callable) {
		return fmt.Errorf("catalog: %s: mutation.reconcile.callable must be a Callable ExportId", where)
	}
	if reconcile.Bind != "originalInvocationId" {
		return fmt.Errorf("catalog: %s: mutation.reconcile.bind must be originalInvocationId", where)
	}
	if _, declared := schemaIDs[reconcile.ProofSchema]; !validSchemaURI(reconcile.ProofSchema) || !declared {
		return fmt.Errorf("catalog: %s: mutation.reconcile.proofSchema must be a declared Schema URI", where)
	}
	if reconcile.ProofPath == "" {
		return fmt.Errorf("catalog: %s: mutation.reconcile.proofPath is required", where)
	}
	return nil
}

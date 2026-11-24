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
)

// -- generated document -----------------------------------------------------

type sidecarDocument struct {
	APIVersion      string                `yaml:"apiVersion"`
	ProviderID      string                `yaml:"providerId"`
	ProviderVersion string                `yaml:"providerVersion"`
	OwnerContract   ownerContractDocument `yaml:"ownerContract"`
	Files           []fileDigest          `yaml:"files"`
	Bindings        []bindingDocument     `yaml:"bindings"`
}

type ownerContractDocument struct {
	StableID          string `yaml:"stableId"`
	Version           string `yaml:"version"`
	PublicationDigest string `yaml:"publicationDigest"`
	ManifestDigest    string `yaml:"manifestDigest"`
	Operations        int    `yaml:"operations"`
}

type fileDigest struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

type schemaReference struct {
	Ref string `yaml:"$ref" json:"$ref"`
}

type bindingDocument struct {
	RuntimeBindingID   string                  `yaml:"runtimeBindingId"`
	RuntimeOperationID string                  `yaml:"runtimeOperationId"`
	BackendOperationID string                  `yaml:"backendOperationId"`
	OwnerCallableID    string                  `yaml:"ownerCallableId"`
	Effect             string                  `yaml:"effect"`
	CapabilityID       string                  `yaml:"capabilityId"`
	OwnerOutcomes      []string                `yaml:"ownerOutcomes"`
	InputSchema        schemaReference         `yaml:"inputSchema"`
	OutputSchema       schemaReference         `yaml:"outputSchema"`
	HostInvocation     *hostInvocationDocument `yaml:"hostInvocation"`
	PrivateArguments   []privateArgument       `yaml:"privateArguments"`
	Observation        *observationDocument    `yaml:"observation,omitempty"`
	CommitProof        *commitProofDocument    `yaml:"commitProof,omitempty"`
}

type hostInvocationDocument struct {
	Version   string          `yaml:"version"`
	Arguments []scopeArgument `yaml:"arguments"`
	Fields    []scopeField    `yaml:"fields"`
}

// scopeArgument pins one Skill-authored argument to one Host-private scope
// value. It is private execution metadata: the Provider cannot serve the
// callable when the two disagree, so a Skill must not be able to author a
// different SUPI/PSI than the Host invocation scope.
type scopeArgument struct {
	Argument   string `yaml:"argument"`
	ScopeField string `yaml:"scopeField"`
}

type scopeField struct {
	Name     string `yaml:"name"`
	Source   string `yaml:"source"`
	Required bool   `yaml:"required"`
	Type     string `yaml:"type,omitempty"`
	Minimum  *int   `yaml:"minimum,omitempty"`
	Maximum  *int   `yaml:"maximum,omitempty"`
}

type privateArgument struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
}

type observationDocument struct {
	EnvelopePath string            `yaml:"envelopePath"`
	CodeField    string            `yaml:"codeField"`
	OutputsField string            `yaml:"outputsField"`
	Outcomes     map[string]string `yaml:"outcomes"`
}

type commitProofDocument struct {
	Mode          string             `yaml:"mode"`
	ReceiptSchema schemaReference    `yaml:"receiptSchema"`
	Receipt       receiptDocument    `yaml:"receipt"`
	Reconcile     *reconcileDocument `yaml:"reconcile,omitempty"`
}

type receiptDocument struct {
	Path               string                 `yaml:"path"`
	SchemaVersionField string                 `yaml:"schemaVersionField"`
	SchemaVersion      string                 `yaml:"schemaVersion"`
	InvocationIDField  string                 `yaml:"invocationIdField"`
	StateField         string                 `yaml:"stateField"`
	States             map[string]string      `yaml:"states"`
	Bindings           []receiptBinding       `yaml:"bindings"`
	Commit             *commitBindingDocument `yaml:"commit,omitempty"`
}

type receiptBinding struct {
	Field          string `yaml:"field"`
	Source         string `yaml:"source"`
	Value          string `yaml:"value,omitempty"`
	HostScopeField string `yaml:"hostScopeField,omitempty"`
}

type commitBindingDocument struct {
	Path              string `yaml:"path"`
	StateField        string `yaml:"stateField"`
	CommittedValue    string `yaml:"committedValue"`
	InvocationIDField string `yaml:"invocationIdField"`
}

type reconcileDocument struct {
	BindingID string              `yaml:"bindingId"`
	Arguments []reconcileArgument `yaml:"arguments"`
	Decision  decisionDocument    `yaml:"decision"`
}

type reconcileArgument struct {
	Field          string `yaml:"field"`
	Source         string `yaml:"source"`
	Value          string `yaml:"value,omitempty"`
	HostScopeField string `yaml:"hostScopeField,omitempty"`
}

type decisionDocument struct {
	Path          string `yaml:"path"`
	ResolvedField string `yaml:"resolvedField"`
	StateField    string `yaml:"stateField"`
	ReceiptPath   string `yaml:"receiptPath"`
}

// -- pinned Owner publication ----------------------------------------------

type ownerPackage struct {
	Dir                   string
	Manifest              ownerManifest
	ManifestDigest        string
	Catalog               ownerCatalog
	Capabilities          map[string]map[string]string
	Contracts             map[string]ownerToolContract
	Schemas               map[string][]byte
	SchemaDocuments       map[string]any
	ReceiptSchemaIdentity string
}

type ownerManifest struct {
	APIVersion        string              `json:"apiVersion"`
	Artifacts         map[string]string   `json:"artifacts"`
	BuildIdentity     string              `json:"buildIdentity"`
	NotPublished      []ownerNotPublished `json:"notPublished"`
	NotPublishedIndex map[string]string   `json:"-"`
	OperationCount    int                 `json:"operationCount"`
	Operations        []ownerOperation    `json:"operations"`
	Owner             json.RawMessage     `json:"owner"`
	Package           struct {
		StableID string `json:"stableId"`
		Version  string `json:"version"`
	} `json:"package"`
	PublicationDigest string `json:"publicationDigest"`
	Release           string `json:"release"`
	SchemaVersion     string `json:"schemaVersion"`
}

type ownerNotPublished struct {
	Callable string `json:"callable"`
	Reason   string `json:"reason"`
}

type ownerOperation struct {
	Callable    string `json:"callable"`
	Capability  string `json:"capability"`
	CommitBasis string `json:"commitBasis"`
	Effect      string `json:"effect"`
	Guarantee   string `json:"guarantee"`
	Source      string `json:"source"`
	Step        string `json:"step"`
	Table       string `json:"table"`
}

type ownerCatalog struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Owner string `yaml:"owner"`
		Name  string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Contracts []ownerToolContract `yaml:"contracts"`
	} `yaml:"spec"`
}

type ownerToolContract struct {
	Kind          string           `yaml:"kind"`
	ID            string           `yaml:"id"`
	Effect        string           `yaml:"effect"`
	OwnerOutcomes []string         `yaml:"ownerOutcomes"`
	InputSchema   schemaReference  `yaml:"inputSchema"`
	OutputSchema  schemaReference  `yaml:"outputSchema"`
	ReceiptSchema *schemaReference `yaml:"receiptSchema"`
	Guarantees    struct {
		Evidence struct {
			Provides []struct {
				Guarantee string `yaml:"guarantee"`
				Path      string `yaml:"path"`
			} `yaml:"provides"`
		} `yaml:"evidence"`
	} `yaml:"guarantees"`
	Mutation *ownerMutation `yaml:"mutation"`
}

type ownerMutation struct {
	CommitProof struct {
		Schema string `yaml:"schema"`
		Path   string `yaml:"path"`
		Bind   string `yaml:"bind"`
	} `yaml:"commitProof"`
	FailureClassPath string `yaml:"failureClassPath"`
	FailureClasses   []struct {
		ID           string `yaml:"id"`
		Outcome      string `yaml:"outcome"`
		ProofSchema  string `yaml:"proofSchema"`
		ProofPath    string `yaml:"proofPath"`
		NoSideEffect bool   `yaml:"noSideEffect"`
		Retryable    bool   `yaml:"retryable"`
	} `yaml:"failureClasses"`
	// mas/op/step are filled from the reviewed Go binding, never guessed from
	// the Owner Schema or from the callable name.
	mas  string
	op   string
	step string
}

func loadCapabilities(directory string) (map[string]map[string]string, error) {
	payload, err := os.ReadFile(filepath.Join(directory, "capability-contracts.yaml"))
	if err != nil {
		return nil, fmt.Errorf("runtimebindings: read Owner capability contracts: %w", err)
	}
	var document struct {
		APIVersion string            `yaml:"apiVersion"`
		Kind       string            `yaml:"kind"`
		Metadata   map[string]string `yaml:"metadata"`
		Spec       struct {
			Capabilities []struct {
				ID        string `yaml:"id"`
				Owner     string `yaml:"owner"`
				Callables []struct {
					ID             string `yaml:"id"`
					Kind           string `yaml:"kind"`
					Effect         string `yaml:"effect"`
					Owner          string `yaml:"owner"`
					ScopePolicyRef string `yaml:"scopePolicyRef"`
				} `yaml:"callables"`
			} `yaml:"capabilities"`
		} `yaml:"spec"`
	}
	strict := yaml.NewDecoder(strings.NewReader(string(payload)))
	strict.KnownFields(true)
	if err := strict.Decode(&document); err != nil {
		return nil, fmt.Errorf("runtimebindings: decode Owner capability contracts: %w", err)
	}
	if document.APIVersion != "connectionagent.io/capability-contracts/v1alpha4" ||
		document.Kind != "CapabilityContracts" {
		return nil, fmt.Errorf("runtimebindings: unsupported Owner capability contract identity")
	}
	capabilities := make(map[string]map[string]string, len(document.Spec.Capabilities))
	for _, capability := range document.Spec.Capabilities {
		if capability.ID == "" {
			return nil, fmt.Errorf("runtimebindings: Owner capability has no id")
		}
		if _, duplicate := capabilities[capability.ID]; duplicate {
			return nil, fmt.Errorf("runtimebindings: duplicate Owner capability %q", capability.ID)
		}
		members := make(map[string]string, len(capability.Callables))
		for _, member := range capability.Callables {
			if _, duplicate := members[member.ID]; duplicate {
				return nil, fmt.Errorf("runtimebindings: capability %s repeats callable %s", capability.ID, member.ID)
			}
			members[member.ID] = member.Effect
		}
		capabilities[capability.ID] = members
	}
	return capabilities, nil
}

func loadOwner(directory string) (*ownerPackage, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("runtimebindings: read Owner manifest: %w", err)
	}
	var manifest ownerManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("runtimebindings: decode Owner manifest: %w", err)
	}
	if manifest.SchemaVersion != "openamsm.mas-publication/v1" {
		return nil, fmt.Errorf("runtimebindings: unsupported Owner publication schema %q", manifest.SchemaVersion)
	}
	if manifest.Package.StableID != "urn:openamsm:mas:tool-contracts" {
		return nil, fmt.Errorf("runtimebindings: unexpected Owner package %q", manifest.Package.StableID)
	}
	if manifest.OperationCount != len(manifest.Operations) {
		return nil, fmt.Errorf("runtimebindings: Owner operationCount does not match operations")
	}
	manifest.NotPublishedIndex = make(map[string]string, len(manifest.NotPublished))
	for _, entry := range manifest.NotPublished {
		manifest.NotPublishedIndex[entry.Callable] = entry.Reason
	}

	schemas := make(map[string][]byte, len(manifest.Artifacts))
	documents := make(map[string]any, len(manifest.Artifacts))
	for relative, expected := range manifest.Artifacts {
		payload, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("runtimebindings: read Owner artifact %s: %w", relative, err)
		}
		actual := "sha256:" + hex.EncodeToString(sha256Sum(payload))
		if actual != expected {
			return nil, fmt.Errorf("runtimebindings: Owner artifact %s digest differs from manifest", relative)
		}
		if strings.HasSuffix(relative, ".schema.json") {
			var document any
			if err := json.Unmarshal(payload, &document); err != nil {
				return nil, fmt.Errorf("runtimebindings: Owner Schema %s is not JSON: %w", relative, err)
			}
			object, ok := document.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("runtimebindings: Owner Schema %s is not an object", relative)
			}
			identity, _ := object["$id"].(string)
			if identity == "" {
				return nil, fmt.Errorf("runtimebindings: Owner Schema %s has no $id", relative)
			}
			if _, duplicate := documents[identity]; duplicate {
				return nil, fmt.Errorf("runtimebindings: duplicate Owner Schema identity %q", identity)
			}
			documents[identity] = document
			schemas[relative] = payload
		}
	}

	catalogBytes, err := os.ReadFile(filepath.Join(directory, "tool-catalog.yaml"))
	if err != nil {
		return nil, fmt.Errorf("runtimebindings: read Owner ToolCatalog: %w", err)
	}
	var toolCatalog ownerCatalog
	strict := yaml.NewDecoder(strings.NewReader(string(catalogBytes)))
	strict.KnownFields(true)
	if err := strict.Decode(&toolCatalog); err != nil {
		return nil, fmt.Errorf("runtimebindings: decode Owner ToolCatalog: %w", err)
	}
	if toolCatalog.APIVersion != "connectionagent.io/toolcatalog/v1alpha4" ||
		toolCatalog.Kind != "ToolCatalog" {
		return nil, fmt.Errorf("runtimebindings: unsupported Owner ToolCatalog identity")
	}
	contracts := make(map[string]ownerToolContract, len(toolCatalog.Spec.Contracts))
	for _, contract := range toolCatalog.Spec.Contracts {
		if contract.Kind != "operation" {
			return nil, fmt.Errorf("runtimebindings: unsupported Owner contract kind %q", contract.Kind)
		}
		if _, duplicate := contracts[contract.ID]; duplicate {
			return nil, fmt.Errorf("runtimebindings: duplicate Owner contract %q", contract.ID)
		}
		contracts[contract.ID] = contract
	}
	capabilities, err := loadCapabilities(directory)
	if err != nil {
		return nil, err
	}

	receiptIdentity := ""
	if document, ok := documents["urn:openamsm:mas:v1alpha4:mutation-receipt"]; ok {
		if object, ok := document.(map[string]any); ok {
			if properties, ok := object["properties"].(map[string]any); ok {
				if schemaVersion, ok := properties["schemaVersion"].(map[string]any); ok {
					receiptIdentity, _ = schemaVersion["const"].(string)
				}
			}
		}
	}

	packageRoot := &ownerPackage{
		Dir:                   directory,
		Manifest:              manifest,
		ManifestDigest:        "sha256:" + hex.EncodeToString(sha256Sum(manifestBytes)),
		Catalog:               toolCatalog,
		Capabilities:          capabilities,
		Contracts:             contracts,
		Schemas:               schemas,
		SchemaDocuments:       documents,
		ReceiptSchemaIdentity: receiptIdentity,
	}
	if receiptIdentity == "" {
		return nil, fmt.Errorf("runtimebindings: Owner receipt Schema has no schemaVersion identity")
	}
	if len(contracts) != len(manifest.Operations) {
		return nil, fmt.Errorf(
			"runtimebindings: Owner ToolCatalog declares %d contracts for %d manifest operations",
			len(contracts), len(manifest.Operations),
		)
	}
	for _, operation := range manifest.Operations {
		if _, ok := contracts[operation.Callable]; !ok {
			return nil, fmt.Errorf("runtimebindings: Owner ToolCatalog is missing %s", operation.Callable)
		}
		members, ok := capabilities[operation.Capability]
		if !ok {
			return nil, fmt.Errorf(
				"runtimebindings: %s names an unknown Owner capability %q",
				operation.Callable, operation.Capability,
			)
		}
		declared, ok := members[operation.Callable]
		if !ok {
			return nil, fmt.Errorf(
				"runtimebindings: capability %s does not declare %s",
				operation.Capability, operation.Callable,
			)
		}
		if !strings.EqualFold(declared, operation.Effect) {
			return nil, fmt.Errorf(
				"runtimebindings: %s Effect differs between the catalog and the capability contract",
				operation.Callable,
			)
		}
	}
	return packageRoot, nil
}

func sha256Sum(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return sum[:]
}

// -- Schema exports ---------------------------------------------------------

type schemaRegistry struct {
	owner  *ownerPackage
	byRef  map[string]string
	byPath map[string][]byte
}

func newSchemaRegistry(owner *ownerPackage) *schemaRegistry {
	return &schemaRegistry{
		owner:  owner,
		byRef:  make(map[string]string, len(owner.SchemaDocuments)),
		byPath: make(map[string][]byte, len(owner.SchemaDocuments)),
	}
}

func (r *schemaRegistry) resolveReference(reference string) (any, error) {
	identity, ok := ownerSchemaIdentity(reference)
	if !ok {
		return nil, fmt.Errorf("unsupported Owner Schema reference %q", reference)
	}
	document, ok := r.owner.SchemaDocuments[identity]
	if !ok {
		return nil, fmt.Errorf("Owner Schema reference %q does not resolve", reference)
	}
	return document, nil
}

func (r *schemaRegistry) export(reference string, document any) (string, error) {
	identity, ok := ownerSchemaIdentity(reference)
	if !ok {
		return "", fmt.Errorf("unsupported Owner Schema reference %q", reference)
	}
	relative := pathForSchemaIdentity(identity)
	if _, exists := r.byRef[identity]; exists {
		return relative, nil
	}
	inlined, err := inlineOwnerRefs(r.owner, document, map[string]bool{identity: true})
	if err != nil {
		return "", err
	}
	encoded, err := encodeJSON(inlined)
	if err != nil {
		return "", err
	}
	r.byRef[identity] = relative
	r.byPath[relative] = encoded
	return relative, nil
}

func (r *schemaRegistry) exportPath(reference string) (string, error) {
	identity, ok := ownerSchemaIdentity(reference)
	if !ok {
		return "", fmt.Errorf("runtimebindings: unsupported Owner Schema reference %q", reference)
	}
	relative, ok := r.byRef[identity]
	if !ok {
		return "", fmt.Errorf("runtimebindings: Owner Schema %q was not exported", reference)
	}
	return relative, nil
}

func (r *schemaRegistry) exportFiles() ([]GeneratedFile, error) {
	paths := make([]string, 0, len(r.byPath))
	for relative := range r.byPath {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	files := make([]GeneratedFile, 0, len(paths))
	for _, relative := range paths {
		files = append(files, GeneratedFile{Path: relative, Bytes: r.byPath[relative]})
	}
	return files, nil
}

func ownerSchemaIdentity(reference string) (string, bool) {
	if !strings.HasPrefix(reference, ownerCallablePrefix) {
		return "", false
	}
	return reference, true
}

func pathForSchemaIdentity(identity string) string {
	slug := strings.TrimPrefix(identity, ownerCallablePrefix)
	kind := ""
	switch {
	case strings.HasSuffix(slug, schemaIDSuffixIn):
		slug = strings.TrimSuffix(slug, schemaIDSuffixIn)
		kind = "input"
	case strings.HasSuffix(slug, schemaIDSuffixOut):
		slug = strings.TrimSuffix(slug, schemaIDSuffixOut)
		kind = "output"
	}
	name := strings.ReplaceAll(strings.ReplaceAll(slug, "_", "-"), ":", "-")
	fileName := name
	if kind != "" {
		fileName += "." + kind
	}
	return filepath.ToSlash(filepath.Join(SchemaDirectory, fileName+".schema.json"))
}

func inlineOwnerRefs(owner *ownerPackage, value any, visiting map[string]bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok && strings.HasPrefix(reference, ownerCallablePrefix) {
			if visiting[reference] {
				return nil, fmt.Errorf("runtimebindings: Owner Schema reference cycle at %q", reference)
			}
			target, ok := owner.SchemaDocuments[reference]
			if !ok {
				return nil, fmt.Errorf("runtimebindings: Owner Schema reference %q does not resolve", reference)
			}
			next := make(map[string]bool, len(visiting)+1)
			for key := range visiting {
				next[key] = true
			}
			next[reference] = true
			return inlineOwnerRefs(owner, target, next)
		}
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			inlined, err := inlineOwnerRefs(owner, child, visiting)
			if err != nil {
				return nil, err
			}
			result[key] = inlined
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			inlined, err := inlineOwnerRefs(owner, child, visiting)
			if err != nil {
				return nil, err
			}
			result = append(result, inlined)
		}
		return result, nil
	default:
		return value, nil
	}
}

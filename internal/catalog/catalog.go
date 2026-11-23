// Package catalog loads and validates the trusted Provider Backend Binding
// Catalog embedded in the Provider build.
//
// The catalog is trusted package content: it maps private backend operation IDs
// to their typed handler or generic SBI binding, effect classification, source
// pin and typed argument profile. It is not a ToolCatalog, not a Capability
// grant and not Skill-authored input. Loading is strict and fails closed:
// malformed YAML, duplicate keys, aliases, merge keys, unknown fields and
// unknown schema keywords are rejected, and the artifact digest is computed
// over the exact embedded bytes.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	providerbindings "github.com/odango-chan/ca-client-tools/contracts/provider-bindings"
)

// Effect is the reviewed read/mutation classification of a backend binding.
type Effect string

const (
	EffectRead     Effect = "READ"
	EffectMutation Effect = "MUTATION"
)

// Typed handler profiles. A binding selects exactly one profile.
const (
	HandlerSMFContextQuery = "openamsmSMFContextQuery"
	HandlerAMFMetricsList  = "openamsmAMFMetricsList"
	HandlerSMFMetricsList  = "openamsmSMFMetricsList"
	HandlerGenericSBI      = "genericSBI"
)

// Supported catalog identity. P0 consolidated to exactly one artifact and one
// apiVersion with no version dispatch or compatibility path.
const (
	APIVersion = "ca-client-tools.provider-bindings/v2"
	Kind       = "ProviderBackendBindingCatalog"
)

const (
	// sourceKindOpenAPI and sourceKindOpenAMSMSource are the two accepted
	// source pin kinds.
	sourceKindOpenAPI        = "openapi"
	sourceKindOpenAMSMSource = "openamsm-source"

	// catalogProviderID is the only accepted catalog provider identity.
	catalogProviderID = "ca-client-tools.sbi"

	// openAMSMSourceRepository is the only accepted source repository.
	openAMSMSourceRepository = "odango-chan/env-openamsm"

	// catalogVersion is the only accepted catalog version.
	catalogVersion = "2"

	// masServiceKey is the service key of the eight SMF context-query bindings.
	masServiceKey = "openamsm-smf-context"

	// maxExportIDLength bounds a Callable/Capability ExportId.
	maxExportIDLength = 256

	// maxSchemaURILength bounds a Schema identity URI.
	maxSchemaURILength = 512
)

// Source is one pinned evidence source for the catalog content.
type Source struct {
	SourceID   string
	Kind       string // "openapi" or "openamsm-source"
	Repository string
	Commit     string
	Path       string
	GitBlobSha string
}

// Argument is one typed backend argument declared by the catalog.
type Argument struct {
	Name      string
	Type      string // "string", "integer" or "boolean"
	Required  bool
	Minimum   *int64
	Maximum   *int64
	MinLength *int
	MaxLength *int
	Pattern   string
	Default   any
}

// HeaderBinding is one trusted fixed header value declared by a generic
// binding. Callers never supply header values.
type HeaderBinding struct {
	Name  string
	Value string
}

// GenericPart binds one declared multipart part to its root JSON reference.
type GenericPart struct {
	Name             string
	ContentType      string
	JSONPointerField string
	Required         bool
}

// GenericBody is the declared generic SBI request body shape.
type GenericBody struct {
	Kind             string // "json", "binary" or "multipart"
	MediaType        string
	RequestSchema    json.RawMessage
	MultipartRoot    bool
	MaxParts         int
	ContentTypeParam string
	Parts            []GenericPart
}

// Idempotency is the reviewed idempotency/reconciliation contract of a
// mutation binding. Absence means no automatic retry is allowed.
type Idempotency struct {
	Mode        string
	KeyArgument string
}

// GenericSpec is the wire contract of a generic SBI binding. A generic binding
// is fully self-describing: method, path, parameters, headers, body and
// response are trusted catalog content, never caller input. ArgumentSchema is
// the closed nested invocation schema the Provider validates before dispatch.
type GenericSpec struct {
	Method             string
	Path               string
	PathParameters     []Argument
	QueryParameters    []Argument
	HeaderBindings     []HeaderBinding
	Body               *GenericBody
	ArgumentSchema     json.RawMessage
	ResponseMediaTypes []string
	ResponseSchema     json.RawMessage
	ResponseParts      []GenericPart
	Idempotency        *Idempotency
}

// InterfaceIdentity is the owning Compiler Interface Package identity of the
// public Tool contracts. The package version is supplied by the release
// generator, not duplicated in the catalog source.
type InterfaceIdentity struct {
	PackageID string
	Name      string
	Publisher string
	Owner     string
}

// SchemaSource is one standalone JSON Schema source file of a public Tool
// contract. The generator converts it to the exported JSON Schema document.
type SchemaSource struct {
	ID   string
	Path string
}

// Tool is one canonical public Tool contract. It is the release generator's
// only source for the Compiler ToolCatalog entry, the Interface Package export
// and the runtime provider.yaml binding.
type Tool struct {
	CallableID           string
	BackendOperationID   string
	Kind                 string // "operation" or "observation"
	Effect               Effect
	OwnerOutcomes        []string
	Capabilities         []string
	InputSchemaID        string
	QueryContextSchemaID string
	OutputSchemaID       string
	ReceiptSchemaID      string
}

// Binding is one private backend operation binding.
type Binding struct {
	BackendOperationID string
	Effect             Effect
	Handler            string
	ServiceKey         string
	ContextType        string // SC/PA/UPN/SSM/IAM/UPS/CA/SAA for SMF context bindings
	SourceID           string
	SourceOperationID  string
	SourceSymbol       string
	Method             string
	Path               string
	RequestMediaType   string
	ResponseMediaTypes []string
	ResponseSchemaRef  string
	Arguments          []Argument
	Generic            *GenericSpec
}

// Catalog is one loaded and validated Provider Backend Binding Catalog. It is
// immutable after load: the loaders fully initialize it, Find/IDs/SourceByID
// only read it, and callers must not mutate the exposed slices or maps.
type Catalog struct {
	APIVersion  string
	Version     string
	Digest      string // "sha256:" + lowercase hex of the exact artifact bytes
	Interface   *InterfaceIdentity
	Sources     []Source
	SchemaFiles []SchemaSource
	Bindings    []Binding
	Tools       []Tool
	Schemas     map[string]json.RawMessage

	index       map[string]int
	sourceIndex map[string]int
	toolIndex   map[string]int
}

// Load loads, validates and inventory-checks the single embedded catalog
// artifact. It is the only production load path.
func Load() (*Catalog, error) {
	return loadShipped(providerbindings.Catalog)
}

// Parse loads and structurally validates one catalog artifact. It is used by
// tests and by the acceptance-only fixture build; production callers use
// Load, which also enforces the shipped inventory.
func Parse(raw []byte) (*Catalog, error) {
	return load(raw)
}

// Find returns the binding with the exact backend operation ID.
func (c *Catalog) Find(backendOperationID string) (Binding, bool) {
	if c == nil {
		return Binding{}, false
	}
	position, ok := c.index[backendOperationID]
	if !ok {
		return Binding{}, false
	}
	return c.Bindings[position], true
}

// IDs returns the backend operation IDs in stable declaration order. The
// returned slice is a copy and may be modified by the caller.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, len(c.Bindings))
	for position := range c.Bindings {
		ids[position] = c.Bindings[position].BackendOperationID
	}
	return ids
}

// SourceByID returns the declared pin for a source ID.
func (c *Catalog) SourceByID(sourceID string) (Source, bool) {
	if c == nil {
		return Source{}, false
	}
	position, ok := c.sourceIndex[sourceID]
	if !ok {
		return Source{}, false
	}
	return c.Sources[position], true
}

// Tool returns the public Tool contract with the exact Callable ID.
func (c *Catalog) Tool(callableID string) (Tool, bool) {
	if c == nil {
		return Tool{}, false
	}
	position, ok := c.toolIndex[callableID]
	if !ok {
		return Tool{}, false
	}
	return c.Tools[position], true
}

// buildIndex builds the immutable lookup indexes.
func (c *Catalog) buildIndex() error {
	c.index = make(map[string]int, len(c.Bindings))
	for position, binding := range c.Bindings {
		if _, duplicate := c.index[binding.BackendOperationID]; duplicate {
			return fmt.Errorf("catalog: duplicate backend operation ID %q", binding.BackendOperationID)
		}
		c.index[binding.BackendOperationID] = position
	}
	c.sourceIndex = make(map[string]int, len(c.Sources))
	for position, source := range c.Sources {
		if _, duplicate := c.sourceIndex[source.SourceID]; duplicate {
			return fmt.Errorf("catalog: duplicate source ID %q", source.SourceID)
		}
		c.sourceIndex[source.SourceID] = position
	}
	c.toolIndex = make(map[string]int, len(c.Tools))
	for position, tool := range c.Tools {
		if _, duplicate := c.toolIndex[tool.CallableID]; duplicate {
			return fmt.Errorf("catalog: duplicate callable ID %q", tool.CallableID)
		}
		c.toolIndex[tool.CallableID] = position
	}
	return nil
}

// digest returns the canonical artifact digest: "sha256:" plus the lowercase
// hex SHA-256 of the exact artifact bytes.
func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

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

// Supported catalog identities.
const (
	APIVersionV1 = "ca-client-tools.provider-bindings/v1"
	APIVersionV2 = "ca-client-tools.provider-bindings/v2"
	Kind         = "ProviderBackendBindingCatalog"
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

	// v1SourceID is the synthetic source identity of the single v1 OpenAPI
	// source entry. The frozen v1 artifact has no explicit sourceId, so this
	// stable identity is derived at load time.
	v1SourceID = "openamsm-mas-context-openapi"

	// v2CatalogVersion is the only accepted v2 catalog version.
	v2CatalogVersion = "2"

	// masServiceKey is the service key of the eight SMF context-query bindings.
	masServiceKey = "openamsm-smf-context"
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

// GenericBody is the declared generic SBI request body shape.
type GenericBody struct {
	Kind             string // "json", "binary" or "multipart"
	MediaType        string
	RequestSchema    json.RawMessage
	MultipartRoot    bool
	MaxParts         int
	ContentTypeParam string
}

// Idempotency is the reviewed idempotency/reconciliation contract of a
// mutation binding. Absence means no automatic retry is allowed.
type Idempotency struct {
	Mode        string
	KeyArgument string
}

// GenericSpec is the wire contract of a generic SBI binding. A generic binding
// is fully self-describing: method, path, parameters, headers, body and
// response are trusted catalog content, never caller input.
type GenericSpec struct {
	Method             string
	Path               string
	PathParameters     []Argument
	QueryParameters    []Argument
	HeaderBindings     []HeaderBinding
	Body               *GenericBody
	ResponseMediaTypes []string
	ResponseSchema     json.RawMessage
	Idempotency        *Idempotency
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
	APIVersion string
	Version    string
	Digest     string // "sha256:" + lowercase hex of the exact artifact bytes
	Sources    []Source
	Bindings   []Binding
	Schemas    map[string]json.RawMessage

	index       map[string]int
	sourceIndex map[string]int
}

// LoadV1 loads and validates the frozen embedded v1 artifact.
func LoadV1() (*Catalog, error) {
	return loadV1(providerbindings.V1)
}

// LoadV2 loads, validates and inventory-checks the embedded v2 artifact.
func LoadV2() (*Catalog, error) {
	return loadV2Shipped(providerbindings.V2)
}

// Load loads one catalog artifact and detects its apiVersion. It is used by
// tests and by version-specific callers; production callers use LoadV1/LoadV2.
func Load(raw []byte) (*Catalog, error) {
	version, err := detectAPIVersion(raw)
	if err != nil {
		return nil, err
	}
	switch version {
	case APIVersionV1:
		return loadV1(raw)
	case APIVersionV2:
		return loadV2(raw)
	default:
		return nil, fmt.Errorf("catalog: unsupported apiVersion %q", version)
	}
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
	return nil
}

// digest returns the canonical artifact digest: "sha256:" plus the lowercase
// hex SHA-256 of the exact artifact bytes.
func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

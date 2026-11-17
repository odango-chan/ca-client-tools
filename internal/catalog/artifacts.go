package catalog

import (
	"encoding/json"
	"fmt"
)

// Raw v1 artifact structure. It mirrors the frozen contract exactly; strict
// decoding rejects any additional field at any level.
type v1File struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   v1Metadata `yaml:"metadata"`
	Spec       v1Spec     `yaml:"spec"`
}

type v1Metadata struct {
	ProviderID string   `yaml:"providerId"`
	Source     v1Source `yaml:"source"`
}

type v1Source struct {
	Repository        string `yaml:"repository"`
	Commit            string `yaml:"commit"`
	OpenAPIPath       string `yaml:"openApiPath"`
	OpenAPIGitBlobSha string `yaml:"openApiGitBlobSha"`
}

type v1Spec struct {
	Bindings []v1Binding `yaml:"bindings"`
}

type v1Binding struct {
	BackendOperationID string          `yaml:"backendOperationId"`
	Handler            string          `yaml:"handler"`
	Source             v1BindingSource `yaml:"source"`
}

type v1BindingSource struct {
	ServiceKey  string `yaml:"serviceKey"`
	ContextType string `yaml:"contextType"`
	OperationID string `yaml:"operationId"`
}

// Raw v2 artifact structure. It is the additive shape: metadata sources and
// catalog version, a schema map and typed or generic bindings.
type v2File struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   v2Metadata `yaml:"metadata"`
	Spec       v2Spec     `yaml:"spec"`
}

type v2Metadata struct {
	ProviderID     string     `yaml:"providerId"`
	CatalogVersion string     `yaml:"catalogVersion"`
	Sources        []v2Source `yaml:"sources"`
}

type v2Source struct {
	SourceID   string `yaml:"sourceId"`
	Kind       string `yaml:"kind"`
	Repository string `yaml:"repository"`
	Commit     string `yaml:"commit"`
	Path       string `yaml:"path"`
	GitBlobSha string `yaml:"gitBlobSha"`
}

type v2Spec struct {
	Schemas  map[string]any `yaml:"schemas"`
	Bindings []v2Binding    `yaml:"bindings"`
}

type v2Binding struct {
	BackendOperationID string          `yaml:"backendOperationId"`
	Effect             string          `yaml:"effect"`
	Handler            string          `yaml:"handler"`
	ServiceKey         string          `yaml:"serviceKey"`
	ContextType        string          `yaml:"contextType"`
	Source             v2BindingSource `yaml:"source"`
	Request            *v2Request      `yaml:"request"`
	Response           *v2Response     `yaml:"response"`
	Arguments          []v2Argument    `yaml:"arguments"`
	Generic            *v2Generic      `yaml:"generic"`
}

type v2BindingSource struct {
	SourceID     string `yaml:"sourceId"`
	OperationID  string `yaml:"operationId"`
	SourceSymbol string `yaml:"sourceSymbol"`
	Method       string `yaml:"method"`
	Path         string `yaml:"path"`
}

type v2Request struct {
	MediaType string `yaml:"mediaType"`
}

type v2Response struct {
	MediaTypes []string `yaml:"mediaTypes"`
	SchemaRef  string   `yaml:"schemaRef"`
}

type v2Argument struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Required  bool   `yaml:"required"`
	Minimum   *int64 `yaml:"minimum"`
	Maximum   *int64 `yaml:"maximum"`
	MinLength *int   `yaml:"minLength"`
	MaxLength *int   `yaml:"maxLength"`
	Pattern   string `yaml:"pattern"`
	Default   any    `yaml:"default"`
}

type v2Generic struct {
	Method             string            `yaml:"method"`
	Path               string            `yaml:"path"`
	PathParameters     []v2Argument      `yaml:"pathParameters"`
	QueryParameters    []v2Argument      `yaml:"queryParameters"`
	HeaderBindings     []v2HeaderBinding `yaml:"headerBindings"`
	Body               *v2GenericBody    `yaml:"body"`
	ResponseMediaTypes []string          `yaml:"responseMediaTypes"`
	ResponseSchema     any               `yaml:"responseSchema"`
	Idempotency        *v2Idempotency    `yaml:"idempotency"`
}

type v2GenericBody struct {
	Kind             string `yaml:"kind"`
	MediaType        string `yaml:"mediaType"`
	RequestSchema    any    `yaml:"requestSchema"`
	MultipartRoot    bool   `yaml:"multipartRoot"`
	MaxParts         int    `yaml:"maxParts"`
	ContentTypeParam string `yaml:"contentTypeParam"`
}

type v2HeaderBinding struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type v2Idempotency struct {
	Mode        string `yaml:"mode"`
	KeyArgument string `yaml:"keyArgument"`
}

// v1BindingExpectation is one frozen v1 binding identity.
type v1BindingExpectation struct {
	BackendOperationID string
	ContextType        string
	OperationID        string
}

// shippedV1Bindings is the frozen v1 declaration order and identity.
var shippedV1Bindings = []v1BindingExpectation{
	{BackendOperationID: "openamsm.smf.mas.sc.query", ContextType: "SC", OperationID: "queryScContext"},
	{BackendOperationID: "openamsm.smf.mas.pa.query", ContextType: "PA", OperationID: "queryPaContext"},
	{BackendOperationID: "openamsm.smf.mas.upn.query", ContextType: "UPN", OperationID: "queryUpnContext"},
	{BackendOperationID: "openamsm.smf.mas.ssm.query", ContextType: "SSM", OperationID: "querySsmContext"},
	{BackendOperationID: "openamsm.smf.mas.iam.query", ContextType: "IAM", OperationID: "queryIamContext"},
	{BackendOperationID: "openamsm.smf.mas.ups.query", ContextType: "UPS", OperationID: "queryUpsContext"},
	{BackendOperationID: "openamsm.smf.mas.ca.query", ContextType: "CA", OperationID: "queryCaContext"},
	{BackendOperationID: "openamsm.smf.mas.saa.query", ContextType: "SAA", OperationID: "querySaaContext"},
}

// v2BindingExpectation is one pinned v2 shipped binding identity.
type v2BindingExpectation struct {
	BackendOperationID string
	Effect             Effect
	Handler            string
	ServiceKey         string
	ContextType        string
	SourceID           string
	SourceOperationID  string
	SourceSymbol       string
	Method             string
	Path               string
	ResponseSchemaRef  string
}

// shippedV2Sources is the exact v2 source pin inventory.
var shippedV2Sources = []Source{
	{
		SourceID:   "openamsm-mas-context-openapi",
		Kind:       sourceKindOpenAPI,
		Repository: openAMSMSourceRepository,
		Commit:     "594dec78b95bff4fdc34deac8a3a9681fb41228f",
		Path:       "upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
		GitBlobSha: "46045c2f8f1cae31c6e6c98d5d20addbebe41969",
	},
	{
		SourceID:   "openamsm-amf-init-c",
		Kind:       sourceKindOpenAMSMSource,
		Repository: openAMSMSourceRepository,
		Commit:     "594dec78b95bff4fdc34deac8a3a9681fb41228f",
		Path:       "upstream/openamsm/src/amf/init.c",
		GitBlobSha: "1ee5c9b148c68ceb7ceb377a29e982b3c9985878",
	},
	{
		SourceID:   "openamsm-smf-init-c",
		Kind:       sourceKindOpenAMSMSource,
		Repository: openAMSMSourceRepository,
		Commit:     "594dec78b95bff4fdc34deac8a3a9681fb41228f",
		Path:       "upstream/openamsm/src/smf/init.c",
		GitBlobSha: "ce8aea73ddcc0dddb84f6c839fe2f4b6c096a88b",
	},
}

// shippedV2Bindings is the exact v2 binding inventory.
var shippedV2Bindings = []v2BindingExpectation{
	{
		BackendOperationID: "openamsm.smf.mas.sc.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "SC",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryScContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.pa.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "PA",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryPaContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.upn.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "UPN",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryUpnContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.ssm.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "SSM",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "querySsmContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.iam.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "IAM",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryIamContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.ups.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "UPS",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryUpsContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.ca.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "CA",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "queryCaContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.smf.mas.saa.query",
		Effect:             EffectRead,
		Handler:            HandlerSMFContextQuery,
		ServiceKey:         masServiceKey,
		ContextType:        "SAA",
		SourceID:           "openamsm-mas-context-openapi",
		SourceOperationID:  "querySaaContext",
		ResponseSchemaRef:  "masContextResult",
	},
	{
		BackendOperationID: "openamsm.amf.ue-info.list",
		Effect:             EffectRead,
		Handler:            HandlerAMFMetricsList,
		ServiceKey:         "openamsm-amf-metrics",
		SourceID:           "openamsm-amf-init-c",
		SourceOperationID:  "listAmfUEInfo",
		SourceSymbol:       "amf_dump_ue_info",
		Method:             "GET",
		Path:               "/ue-info",
		ResponseSchemaRef:  "metricsPage",
	},
	{
		BackendOperationID: "openamsm.amf.gnb-info.list",
		Effect:             EffectRead,
		Handler:            HandlerAMFMetricsList,
		ServiceKey:         "openamsm-amf-metrics",
		SourceID:           "openamsm-amf-init-c",
		SourceOperationID:  "listAmfGNBInfo",
		SourceSymbol:       "amf_dump_gnb_info",
		Method:             "GET",
		Path:               "/gnb-info",
		ResponseSchemaRef:  "metricsPage",
	},
	{
		BackendOperationID: "openamsm.smf.pdu-info.list",
		Effect:             EffectRead,
		Handler:            HandlerSMFMetricsList,
		ServiceKey:         "openamsm-smf-metrics",
		SourceID:           "openamsm-smf-init-c",
		SourceOperationID:  "listSmfPDUInfo",
		SourceSymbol:       "smf_dump_pdu_info",
		Method:             "GET",
		Path:               "/pdu-info",
		ResponseSchemaRef:  "metricsPage",
	},
}

// allowedContextTypes is the closed SMF context-query context vocabulary.
var allowedContextTypes = map[string]struct{}{
	"SC":  {},
	"PA":  {},
	"UPN": {},
	"SSM": {},
	"IAM": {},
	"UPS": {},
	"CA":  {},
	"SAA": {},
}

// loadV1 loads and validates one v1 artifact.
func loadV1(raw []byte) (*Catalog, error) {
	root, err := validateDocument(raw)
	if err != nil {
		return nil, err
	}
	version, err := apiVersionFromRoot(root)
	if err != nil {
		return nil, err
	}
	if version != APIVersionV1 {
		return nil, fmt.Errorf("catalog: v1 loader received apiVersion %q", version)
	}
	var file v1File
	if err := decodeStrict(raw, &file); err != nil {
		return nil, err
	}
	if err := validateV1Artifact(&file); err != nil {
		return nil, err
	}
	return buildV1Catalog(raw, &file)
}

// validateV1Artifact validates the frozen v1 contract exactly.
func validateV1Artifact(file *v1File) error {
	if file.Kind != Kind {
		return fmt.Errorf("catalog: metadata.kind must be %q", Kind)
	}
	if file.Metadata.ProviderID != catalogProviderID {
		return fmt.Errorf("catalog: metadata.providerId must be %q", catalogProviderID)
	}
	source := file.Metadata.Source
	if source.Repository != openAMSMSourceRepository {
		return fmt.Errorf("catalog: metadata.source.repository must be %q", openAMSMSourceRepository)
	}
	if !commitPattern.MatchString(source.Commit) {
		return catalogError("metadata.source.commit must be 40 lowercase hex characters")
	}
	if err := validateRelativePath(source.OpenAPIPath, "metadata.source.openApiPath"); err != nil {
		return err
	}
	if !commitPattern.MatchString(source.OpenAPIGitBlobSha) {
		return catalogError("metadata.source.openApiGitBlobSha must be 40 lowercase hex characters")
	}
	if len(file.Spec.Bindings) != len(shippedV1Bindings) {
		return fmt.Errorf("catalog: v1 spec.bindings must declare exactly %d bindings, got %d", len(shippedV1Bindings), len(file.Spec.Bindings))
	}
	for position, expected := range shippedV1Bindings {
		binding := file.Spec.Bindings[position]
		if binding.BackendOperationID != expected.BackendOperationID {
			return fmt.Errorf("catalog: v1 spec.bindings[%d] must be %q, got %q", position, expected.BackendOperationID, binding.BackendOperationID)
		}
		if binding.Handler != HandlerSMFContextQuery {
			return fmt.Errorf("catalog: v1 binding %q handler must be %q", binding.BackendOperationID, HandlerSMFContextQuery)
		}
		if binding.Source.ServiceKey != masServiceKey {
			return fmt.Errorf("catalog: v1 binding %q serviceKey must be %q", binding.BackendOperationID, masServiceKey)
		}
		if binding.Source.ContextType != expected.ContextType {
			return fmt.Errorf("catalog: v1 binding %q contextType must be %q", binding.BackendOperationID, expected.ContextType)
		}
		if binding.Source.OperationID != expected.OperationID {
			return fmt.Errorf("catalog: v1 binding %q operationId must be %q", binding.BackendOperationID, expected.OperationID)
		}
	}
	return nil
}

// buildV1Catalog converts a validated v1 artifact into the public model.
func buildV1Catalog(raw []byte, file *v1File) (*Catalog, error) {
	catalog := &Catalog{
		APIVersion: file.APIVersion,
		Version:    "",
		Digest:     digest(raw),
		Sources: []Source{{
			SourceID:   v1SourceID,
			Kind:       sourceKindOpenAPI,
			Repository: file.Metadata.Source.Repository,
			Commit:     file.Metadata.Source.Commit,
			Path:       file.Metadata.Source.OpenAPIPath,
			GitBlobSha: file.Metadata.Source.OpenAPIGitBlobSha,
		}},
		Schemas: map[string]json.RawMessage{},
	}
	for _, binding := range file.Spec.Bindings {
		catalog.Bindings = append(catalog.Bindings, Binding{
			BackendOperationID: binding.BackendOperationID,
			Effect:             EffectRead,
			Handler:            binding.Handler,
			ServiceKey:         binding.Source.ServiceKey,
			ContextType:        binding.Source.ContextType,
			SourceID:           v1SourceID,
			SourceOperationID:  binding.Source.OperationID,
		})
	}
	if err := catalog.buildIndex(); err != nil {
		return nil, err
	}
	return catalog, nil
}

// v2Validated holds the artifact content converted during validation.
type v2Validated struct {
	sources  map[string]v2Source
	schemas  map[string]json.RawMessage
	generics []*GenericSpec
}

// loadV2 loads and structurally validates one v2 artifact. The frozen shipped
// inventory check is applied separately by loadV2Shipped.
func loadV2(raw []byte) (*Catalog, error) {
	root, err := validateDocument(raw)
	if err != nil {
		return nil, err
	}
	version, err := apiVersionFromRoot(root)
	if err != nil {
		return nil, err
	}
	if version != APIVersionV2 {
		return nil, fmt.Errorf("catalog: v2 loader received apiVersion %q", version)
	}
	var file v2File
	if err := decodeStrict(raw, &file); err != nil {
		return nil, err
	}
	validated, err := validateV2Artifact(&file)
	if err != nil {
		return nil, err
	}
	return buildV2Catalog(raw, &file, validated)
}

// loadV2Shipped loads the embedded v2 artifact and enforces the frozen shipped
// inventory: exactly three pinned sources and exactly eleven read bindings with
// their declared handler, service key and source mapping.
func loadV2Shipped(raw []byte) (*Catalog, error) {
	catalog, err := loadV2(raw)
	if err != nil {
		return nil, err
	}
	if err := validateShippedV2(catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}

// validateV2Artifact validates the structural v2 contract.
func validateV2Artifact(file *v2File) (*v2Validated, error) {
	if file.Kind != Kind {
		return nil, fmt.Errorf("catalog: kind must be %q", Kind)
	}
	if file.Metadata.ProviderID != catalogProviderID {
		return nil, fmt.Errorf("catalog: metadata.providerId must be %q", catalogProviderID)
	}
	if file.Metadata.CatalogVersion != v2CatalogVersion {
		return nil, fmt.Errorf("catalog: metadata.catalogVersion must be %q", v2CatalogVersion)
	}
	if len(file.Metadata.Sources) == 0 {
		return nil, catalogError("metadata.sources must not be empty")
	}
	validated := &v2Validated{
		sources: make(map[string]v2Source, len(file.Metadata.Sources)),
		schemas: make(map[string]json.RawMessage, len(file.Spec.Schemas)),
	}
	for position, source := range file.Metadata.Sources {
		where := fmt.Sprintf("metadata.sources[%d]", position)
		if err := validateV2Source(source, where); err != nil {
			return nil, err
		}
		if _, duplicate := validated.sources[source.SourceID]; duplicate {
			return nil, fmt.Errorf("catalog: %s: duplicate sourceId %q", where, source.SourceID)
		}
		validated.sources[source.SourceID] = source
	}
	if len(file.Spec.Schemas) == 0 {
		return nil, catalogError("spec.schemas must not be empty")
	}
	for name, value := range file.Spec.Schemas {
		if !operationIDPattern.MatchString(name) {
			return nil, fmt.Errorf("catalog: spec.schemas: invalid schema name %q", name)
		}
		encoded, err := validateSchemaValue(value, "spec.schemas."+name)
		if err != nil {
			return nil, err
		}
		validated.schemas[name] = encoded
	}
	if len(file.Spec.Bindings) == 0 {
		return nil, catalogError("spec.bindings must not be empty")
	}
	validated.generics = make([]*GenericSpec, len(file.Spec.Bindings))
	seen := make(map[string]struct{}, len(file.Spec.Bindings))
	for position := range file.Spec.Bindings {
		binding := &file.Spec.Bindings[position]
		where := fmt.Sprintf("spec.bindings[%d]", position)
		if !operationIDPattern.MatchString(binding.BackendOperationID) {
			return nil, fmt.Errorf("catalog: %s: invalid backendOperationId %q", where, binding.BackendOperationID)
		}
		if _, duplicate := seen[binding.BackendOperationID]; duplicate {
			return nil, fmt.Errorf("catalog: %s: duplicate backendOperationId %q", where, binding.BackendOperationID)
		}
		seen[binding.BackendOperationID] = struct{}{}
		generic, err := validateV2Binding(binding, where, validated.sources, validated.schemas)
		if err != nil {
			return nil, err
		}
		validated.generics[position] = generic
	}
	return validated, nil
}

// validateV2Source validates one source pin.
func validateV2Source(source v2Source, where string) error {
	if !operationIDPattern.MatchString(source.SourceID) {
		return fmt.Errorf("catalog: %s: invalid sourceId %q", where, source.SourceID)
	}
	switch source.Kind {
	case sourceKindOpenAPI, sourceKindOpenAMSMSource:
	default:
		return fmt.Errorf("catalog: %s: unsupported source kind %q", where, source.Kind)
	}
	if source.Repository == "" {
		return fmt.Errorf("catalog: %s: repository must not be empty", where)
	}
	if !commitPattern.MatchString(source.Commit) {
		return fmt.Errorf("catalog: %s: commit must be 40 lowercase hex characters", where)
	}
	if err := validateRelativePath(source.Path, where+".path"); err != nil {
		return err
	}
	if !commitPattern.MatchString(source.GitBlobSha) {
		return fmt.Errorf("catalog: %s: gitBlobSha must be 40 lowercase hex characters", where)
	}
	return nil
}

// validateV2Binding validates one v2 binding and returns its converted generic
// spec when the binding uses the genericSBI profile.
func validateV2Binding(binding *v2Binding, where string, sources map[string]v2Source, schemas map[string]json.RawMessage) (*GenericSpec, error) {
	if !serviceKeyPattern.MatchString(binding.ServiceKey) {
		return nil, fmt.Errorf("catalog: %s: invalid serviceKey %q", where, binding.ServiceKey)
	}
	effect := Effect(binding.Effect)
	switch effect {
	case EffectRead, EffectMutation:
	default:
		return nil, fmt.Errorf("catalog: %s: unsupported effect %q", where, binding.Effect)
	}
	source, ok := sources[binding.Source.SourceID]
	if !ok {
		return nil, fmt.Errorf("catalog: %s: sourceId %q is not declared in metadata.sources", where, binding.Source.SourceID)
	}
	if !operationIDPattern.MatchString(binding.Source.OperationID) {
		return nil, fmt.Errorf("catalog: %s: source.operationId must be a non-empty operation ID", where)
	}
	switch source.Kind {
	case sourceKindOpenAPI:
		if binding.Source.SourceSymbol != "" {
			return nil, fmt.Errorf("catalog: %s: openapi-source bindings must not set sourceSymbol", where)
		}
		if binding.Source.Method != "" || binding.Source.Path != "" {
			return nil, fmt.Errorf("catalog: %s: openapi-source bindings must not set source method/path", where)
		}
	case sourceKindOpenAMSMSource:
		if binding.Source.SourceSymbol == "" {
			return nil, fmt.Errorf("catalog: %s: openamsm-source bindings require sourceSymbol", where)
		}
		if !validMethod(binding.Source.Method) {
			return nil, fmt.Errorf("catalog: %s: openamsm-source bindings require a supported method", where)
		}
		if err := validateAbsolutePath(binding.Source.Path, where+".source.path"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("catalog: %s: unsupported source kind %q", where, source.Kind)
	}
	if binding.Request == nil || binding.Request.MediaType != "application/json" {
		return nil, fmt.Errorf("catalog: %s: request.mediaType must be application/json", where)
	}
	if binding.Response == nil || len(binding.Response.MediaTypes) == 0 {
		return nil, fmt.Errorf("catalog: %s: response.mediaTypes must not be empty", where)
	}
	for position, mediaType := range binding.Response.MediaTypes {
		if !mediaTypePattern.MatchString(mediaType) {
			return nil, fmt.Errorf("catalog: %s: response.mediaTypes[%d] is an invalid media type", where, position)
		}
	}
	if binding.Response.SchemaRef != "" {
		if _, ok := schemas[binding.Response.SchemaRef]; !ok {
			return nil, fmt.Errorf("catalog: %s: response.schemaRef %q is not declared in spec.schemas", where, binding.Response.SchemaRef)
		}
	}
	if binding.ContextType != "" {
		if _, ok := allowedContextTypes[binding.ContextType]; !ok {
			return nil, fmt.Errorf("catalog: %s: unsupported contextType %q", where, binding.ContextType)
		}
	}
	switch binding.Handler {
	case HandlerSMFContextQuery:
		return nil, validateSMFContextBinding(binding, source, where)
	case HandlerAMFMetricsList:
		return nil, validateMetricsBinding(binding, source, where, "openamsm-amf-metrics")
	case HandlerSMFMetricsList:
		return nil, validateMetricsBinding(binding, source, where, "openamsm-smf-metrics")
	case HandlerGenericSBI:
		generic, err := validateGenericBinding(binding, effect, source, where)
		if err != nil {
			return nil, err
		}
		return generic, nil
	default:
		return nil, fmt.Errorf("catalog: %s: unsupported handler %q", where, binding.Handler)
	}
}

func validateSMFContextBinding(binding *v2Binding, source v2Source, where string) error {
	if binding.Generic != nil {
		return fmt.Errorf("catalog: %s: SMF context bindings must not declare a generic spec", where)
	}
	if source.Kind != sourceKindOpenAPI {
		return fmt.Errorf("catalog: %s: SMF context bindings require an openapi source", where)
	}
	if binding.ServiceKey != masServiceKey {
		return fmt.Errorf("catalog: %s: SMF context bindings require serviceKey %q", where, masServiceKey)
	}
	if _, ok := allowedContextTypes[binding.ContextType]; !ok {
		return fmt.Errorf("catalog: %s: SMF context bindings require a supported contextType", where)
	}
	if binding.Response.SchemaRef == "" {
		return fmt.Errorf("catalog: %s: SMF context bindings require response.schemaRef", where)
	}
	return validateMASArguments(binding.Arguments, where+".arguments")
}

func validateMetricsBinding(binding *v2Binding, source v2Source, where, serviceKey string) error {
	if binding.Generic != nil {
		return fmt.Errorf("catalog: %s: metrics bindings must not declare a generic spec", where)
	}
	if source.Kind != sourceKindOpenAMSMSource {
		return fmt.Errorf("catalog: %s: metrics bindings require an openamsm-source source", where)
	}
	if binding.ContextType != "" {
		return fmt.Errorf("catalog: %s: metrics bindings must not declare a contextType", where)
	}
	if binding.ServiceKey != serviceKey {
		return fmt.Errorf("catalog: %s: metrics bindings require serviceKey %q", where, serviceKey)
	}
	if binding.Response.SchemaRef == "" {
		return fmt.Errorf("catalog: %s: metrics bindings require response.schemaRef", where)
	}
	return validateMetricsArguments(binding.Arguments, where+".arguments")
}

func validateGenericBinding(binding *v2Binding, effect Effect, source v2Source, where string) (*GenericSpec, error) {
	if binding.Generic == nil {
		return nil, fmt.Errorf("catalog: %s: genericSBI bindings require a generic spec", where)
	}
	if source.Kind != sourceKindOpenAPI {
		return nil, fmt.Errorf("catalog: %s: genericSBI bindings require an openapi source", where)
	}
	if binding.ContextType != "" {
		return nil, fmt.Errorf("catalog: %s: genericSBI bindings must not declare a contextType", where)
	}
	names := make(map[string]struct{}, len(binding.Arguments))
	for position := range binding.Arguments {
		argumentWhere := fmt.Sprintf("%s.arguments[%d]", where, position)
		if err := validateArgument(binding.Arguments[position], argumentWhere, false); err != nil {
			return nil, err
		}
		name := binding.Arguments[position].Name
		if _, duplicate := names[name]; duplicate {
			return nil, fmt.Errorf("catalog: %s.arguments: duplicate argument name %q", where, name)
		}
		names[name] = struct{}{}
	}
	generic, err := validateGenericSpec(binding.Generic, binding.Arguments, where+".generic")
	if err != nil {
		return nil, err
	}
	if effect == EffectMutation && generic.Idempotency == nil {
		return nil, fmt.Errorf("catalog: %s: MUTATION genericSBI bindings require an idempotency contract", where)
	}
	return generic, nil
}

// validateMASArguments enforces the frozen SMF context-query argument profile.
func validateMASArguments(arguments []v2Argument, where string) error {
	if len(arguments) != 2 {
		return fmt.Errorf("catalog: %s: SMF context bindings require exactly supi and pduSessionId", where)
	}
	for position := range arguments {
		if err := validateArgument(arguments[position], fmt.Sprintf("%s[%d]", where, position), false); err != nil {
			return err
		}
	}
	supi := arguments[0]
	if supi.Name != "supi" || supi.Type != "string" || !supi.Required || supi.Pattern != `^imsi-[0-9]{1,15}$` {
		return fmt.Errorf("catalog: %s: first argument must be the required supi string with pattern ^imsi-[0-9]{1,15}$", where)
	}
	if supi.Minimum != nil || supi.Maximum != nil || supi.MinLength != nil || supi.MaxLength != nil || supi.Default != nil {
		return fmt.Errorf("catalog: %s: supi must not declare bounds or a default", where)
	}
	pduSessionID := arguments[1]
	if pduSessionID.Name != "pduSessionId" || pduSessionID.Type != "integer" || !pduSessionID.Required ||
		pduSessionID.Minimum == nil || *pduSessionID.Minimum != 0 ||
		pduSessionID.Maximum == nil || *pduSessionID.Maximum != 255 {
		return fmt.Errorf("catalog: %s: second argument must be the required integer pduSessionId in [0,255]", where)
	}
	if pduSessionID.Pattern != "" || pduSessionID.MinLength != nil || pduSessionID.MaxLength != nil || pduSessionID.Default != nil {
		return fmt.Errorf("catalog: %s: pduSessionId must not declare string bounds, a pattern or a default", where)
	}
	return nil
}

// validateMetricsArguments enforces the frozen metrics pagination profile.
func validateMetricsArguments(arguments []v2Argument, where string) error {
	if len(arguments) != 2 {
		return fmt.Errorf("catalog: %s: metrics bindings require exactly page and pageSize", where)
	}
	for position := range arguments {
		if err := validateArgument(arguments[position], fmt.Sprintf("%s[%d]", where, position), false); err != nil {
			return err
		}
	}
	page := arguments[0]
	if page.Name != "page" || page.Type != "integer" || page.Required || page.Minimum == nil || *page.Minimum != 0 || page.Maximum != nil {
		return fmt.Errorf("catalog: %s: first argument must be the optional integer page with minimum 0", where)
	}
	if value, ok := integerValue(page.Default); !ok || value != 0 {
		return fmt.Errorf("catalog: %s: page default must be 0", where)
	}
	pageSize := arguments[1]
	if pageSize.Name != "pageSize" || pageSize.Type != "integer" || pageSize.Required || pageSize.Minimum == nil || *pageSize.Minimum != 1 ||
		pageSize.Maximum == nil || *pageSize.Maximum != 100 {
		return fmt.Errorf("catalog: %s: second argument must be the optional integer pageSize in [1,100]", where)
	}
	if value, ok := integerValue(pageSize.Default); !ok || value != 100 {
		return fmt.Errorf("catalog: %s: pageSize default must be 100", where)
	}
	return nil
}

// buildV2Catalog converts a structurally validated v2 artifact into the public
// model.
func buildV2Catalog(raw []byte, file *v2File, validated *v2Validated) (*Catalog, error) {
	catalog := &Catalog{
		APIVersion: file.APIVersion,
		Version:    file.Metadata.CatalogVersion,
		Digest:     digest(raw),
		Sources:    make([]Source, 0, len(file.Metadata.Sources)),
		Schemas:    validated.schemas,
	}
	for _, source := range file.Metadata.Sources {
		catalog.Sources = append(catalog.Sources, Source{
			SourceID:   source.SourceID,
			Kind:       source.Kind,
			Repository: source.Repository,
			Commit:     source.Commit,
			Path:       source.Path,
			GitBlobSha: source.GitBlobSha,
		})
	}
	for position, rawBinding := range file.Spec.Bindings {
		binding := Binding{
			BackendOperationID: rawBinding.BackendOperationID,
			Effect:             Effect(rawBinding.Effect),
			Handler:            rawBinding.Handler,
			ServiceKey:         rawBinding.ServiceKey,
			ContextType:        rawBinding.ContextType,
			SourceID:           rawBinding.Source.SourceID,
			SourceOperationID:  rawBinding.Source.OperationID,
			SourceSymbol:       rawBinding.Source.SourceSymbol,
			Method:             rawBinding.Source.Method,
			Path:               rawBinding.Source.Path,
			RequestMediaType:   rawBinding.Request.MediaType,
			ResponseMediaTypes: append([]string(nil), rawBinding.Response.MediaTypes...),
			ResponseSchemaRef:  rawBinding.Response.SchemaRef,
			Arguments:          convertArguments(rawBinding.Arguments),
		}
		if generic := validated.generics[position]; generic != nil {
			binding.Generic = generic
			binding.Method = generic.Method
			binding.Path = generic.Path
		}
		catalog.Bindings = append(catalog.Bindings, binding)
	}
	if err := catalog.buildIndex(); err != nil {
		return nil, err
	}
	return catalog, nil
}

// validateShippedV2 enforces the exact frozen shipped v2 inventory.
func validateShippedV2(catalog *Catalog) error {
	if len(catalog.Sources) != len(shippedV2Sources) {
		return fmt.Errorf("catalog: v2 artifact must declare exactly %d sources, got %d", len(shippedV2Sources), len(catalog.Sources))
	}
	for position, expected := range shippedV2Sources {
		if catalog.Sources[position] != expected {
			return fmt.Errorf("catalog: v2 metadata.sources[%d] does not match the pinned source inventory", position)
		}
	}
	if len(catalog.Bindings) != len(shippedV2Bindings) {
		return fmt.Errorf("catalog: v2 artifact must declare exactly %d bindings, got %d", len(shippedV2Bindings), len(catalog.Bindings))
	}
	for _, expected := range shippedV2Bindings {
		binding, ok := catalog.Find(expected.BackendOperationID)
		if !ok {
			return fmt.Errorf("catalog: v2 artifact is missing required binding %q", expected.BackendOperationID)
		}
		if binding.Effect != EffectRead {
			return fmt.Errorf("catalog: shipped v2 binding %q must have effect READ", binding.BackendOperationID)
		}
		if binding.Handler != expected.Handler ||
			binding.ServiceKey != expected.ServiceKey ||
			binding.ContextType != expected.ContextType ||
			binding.SourceID != expected.SourceID ||
			binding.SourceOperationID != expected.SourceOperationID ||
			binding.SourceSymbol != expected.SourceSymbol ||
			binding.Method != expected.Method ||
			binding.Path != expected.Path ||
			binding.ResponseSchemaRef != expected.ResponseSchemaRef {
			return fmt.Errorf("catalog: shipped v2 binding %q does not match the pinned inventory", expected.BackendOperationID)
		}
	}
	return nil
}

// errorsNew is a small helper prefixing catalog errors consistently.
func catalogError(format string, arguments ...any) error {
	return fmt.Errorf("catalog: "+format, arguments...)
}

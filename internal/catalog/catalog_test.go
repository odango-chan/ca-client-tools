package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	providerbindings "github.com/odango-chan/ca-client-tools/contracts/provider-bindings"
)

// Exact artifact digests. They are SHA-256 over the embedded file bytes,
// including the final LF. The v1 value is the frozen expected digest; the v2
// value is computed from the shipped v2 artifact bytes.
const (
	wantV1Digest = "sha256:cbb8de356531c1f5e43c558a59eaa4bec94bd3d074505307ed99fc09fd42ac57"
	wantV2Digest = "sha256:b2de218a63a596de0b948617c8e864c8e6e4bf8bc8758e6652cb5d0e7f4bfd50"
)

// wantV2IDs is the shipped v2 declaration order.
var wantV2IDs = []string{
	"openamsm.smf.mas.sc.query",
	"openamsm.smf.mas.pa.query",
	"openamsm.smf.mas.upn.query",
	"openamsm.smf.mas.ssm.query",
	"openamsm.smf.mas.iam.query",
	"openamsm.smf.mas.ups.query",
	"openamsm.smf.mas.ca.query",
	"openamsm.smf.mas.saa.query",
	"openamsm.amf.ue-info.list",
	"openamsm.amf.gnb-info.list",
	"openamsm.smf.pdu-info.list",
}

// genericSources injects two additional pinned openapi sources.
const genericSources = `    - sourceId: 3gpp-nsmf-pdusession-openapi
      kind: openapi
      repository: 3gpp/5G_APIs
      commit: 0123456789abcdef0123456789abcdef01234567
      path: TS29502_Nsmf_PDUSession.yaml
      gitBlobSha: 0123456789abcdef0123456789abcdef01234567
    - sourceId: 3gpp-namf-comm-openapi
      kind: openapi
      repository: 3gpp/5G_APIs
      commit: 0123456789abcdef0123456789abcdef01234567
      path: TS29518_Namf_Communication.yaml
      gitBlobSha: 0123456789abcdef0123456789abcdef01234567
`

// genericBindings adds three genericSBI bindings across two services: one
// READ search with JSON body and query parameter, one MUTATION update with a
// path parameter and idempotency contract, and one READ list without a body.
const genericBindings = `    - backendOperationId: 3gpp.nsmf.pdusession.search
      effect: READ
      handler: genericSBI
      serviceKey: nsmf-pdusession
      source:
        sourceId: 3gpp-nsmf-pdusession-openapi
        operationId: searchSMContext
      request:
        mediaType: application/json
      response:
        mediaTypes: [application/json]
      generic:
        method: POST
        path: /nsmf-pdusession/v1/sm-contexts/search
        queryParameters:
          - name: supported-features
            type: string
            required: false
        headerBindings:
          - name: Accept
            value: application/json
        body:
          kind: json
          mediaType: application/json
          requestSchema:
            type: object
            additionalProperties: false
            properties:
              imsi:
                type: string
                pattern: '^imsi-[0-9]{1,15}$'
        responseMediaTypes: [application/json]
        responseSchema:
          type: object
          properties:
            items:
              type: array
              items:
                type: object

    - backendOperationId: 3gpp.nsmf.smcontext.update
      effect: MUTATION
      handler: genericSBI
      serviceKey: nsmf-pdusession
      source:
        sourceId: 3gpp-nsmf-pdusession-openapi
        operationId: updateSMContext
      arguments:
        - name: correlationId
          type: string
          required: false
      request:
        mediaType: application/json
      response:
        mediaTypes: [application/json]
      generic:
        method: POST
        path: /nsmf-pdusession/v1/sm-contexts/{smContextRef}/modify
        pathParameters:
          - name: smContextRef
            type: string
            required: true
        body:
          kind: json
          mediaType: application/json
          requestSchema:
            type: object
        responseMediaTypes: [application/json]
        responseSchema:
          type: object
        idempotency:
          mode: IDEMPOTENCY_KEY
          keyArgument: correlationId

    - backendOperationId: 3gpp.namf.comm.ue-context.list
      effect: READ
      handler: genericSBI
      serviceKey: namf-comm
      source:
        sourceId: 3gpp-namf-comm-openapi
        operationId: listUeContext
      request:
        mediaType: application/json
      response:
        mediaTypes: [application/json]
      generic:
        method: GET
        path: /namf-comm/v1/ue-contexts
        responseMediaTypes: [application/json]
        responseSchema:
          type: array
          items:
            type: object
`

// genericFixture builds a structurally valid v2 artifact with the genericSBI
// extension.
func genericFixture(t *testing.T) []byte {
	t.Helper()
	raw := mutate(t, providerbindings.V2,
		"  sources:\n    - sourceId: openamsm-mas-context-openapi",
		"  sources:\n"+genericSources+"    - sourceId: openamsm-mas-context-openapi")
	return mutate(t, raw,
		"  bindings:\n    - backendOperationId: openamsm.smf.mas.sc.query",
		"  bindings:\n"+genericBindings+"    - backendOperationId: openamsm.smf.mas.sc.query")
}

func mustLoadV1(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadV1()
	if err != nil {
		t.Fatalf("LoadV1() error: %v", err)
	}
	if catalog == nil {
		t.Fatal("LoadV1() returned nil catalog")
	}
	return catalog
}

func mustLoadV2(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadV2()
	if err != nil {
		t.Fatalf("LoadV2() error: %v", err)
	}
	if catalog == nil {
		t.Fatal("LoadV2() returned nil catalog")
	}
	return catalog
}

// mutate replaces exactly one occurrence of old in raw.
func mutate(t *testing.T, raw []byte, old, new string) []byte {
	t.Helper()
	text := string(raw)
	if count := strings.Count(text, old); count != 1 {
		t.Fatalf("mutate: expected exactly one occurrence of %q, found %d", old, count)
	}
	return []byte(strings.Replace(text, old, new, 1))
}

// mutateFirst replaces the first occurrence of old in raw.
func mutateFirst(t *testing.T, raw []byte, old, new string) []byte {
	t.Helper()
	text := string(raw)
	if !strings.Contains(text, old) {
		t.Fatalf("mutateFirst: %q not found", old)
	}
	return []byte(strings.Replace(text, old, new, 1))
}

// mutateAll replaces every occurrence of old in raw.
func mutateAll(t *testing.T, raw []byte, old, new string) []byte {
	t.Helper()
	text := string(raw)
	if !strings.Contains(text, old) {
		t.Fatalf("mutateAll: %q not found", old)
	}
	return []byte(strings.ReplaceAll(text, old, new))
}

// replaceRange replaces everything from start (inclusive) to end (exclusive).
func replaceRange(t *testing.T, raw []byte, start, end, replacement string) []byte {
	t.Helper()
	text := string(raw)
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		t.Fatalf("replaceRange: start marker %q not found", start)
	}
	endOffset := strings.Index(text[startIndex:], end)
	if endOffset < 0 {
		t.Fatalf("replaceRange: end marker %q not found after start", end)
	}
	endIndex := startIndex + endOffset
	return []byte(text[:startIndex] + replacement + text[endIndex:])
}

// cutFrom replaces everything from start (inclusive) to the end of the file.
func cutFrom(t *testing.T, raw []byte, start, replacement string) []byte {
	t.Helper()
	text := string(raw)
	index := strings.Index(text, start)
	if index < 0 {
		t.Fatalf("cutFrom: marker %q not found", start)
	}
	return []byte(text[:index] + replacement)
}

func mustFind(t *testing.T, catalog *Catalog, backendOperationID string) Binding {
	t.Helper()
	binding, ok := catalog.Find(backendOperationID)
	if !ok {
		t.Fatalf("Find(%q) = false", backendOperationID)
	}
	return binding
}

func requireInt64Pointer(t *testing.T, value *int64, want int64, label string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s: pointer is nil, want %d", label, want)
	}
	if *value != want {
		t.Fatalf("%s = %d, want %d", label, *value, want)
	}
}

func requireIntPointer(t *testing.T, value *int, want int, label string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s: pointer is nil, want %d", label, want)
	}
	if *value != want {
		t.Fatalf("%s = %d, want %d", label, *value, want)
	}
}

func TestLoadV1(t *testing.T) {
	catalog := mustLoadV1(t)
	if catalog.APIVersion != APIVersionV1 {
		t.Fatalf("APIVersion = %q, want %q", catalog.APIVersion, APIVersionV1)
	}
	if catalog.Version != "" {
		t.Fatalf("Version = %q, want empty for v1", catalog.Version)
	}
	if catalog.Digest != wantV1Digest {
		t.Fatalf("Digest = %q, want %q", catalog.Digest, wantV1Digest)
	}
	wantSource := Source{
		SourceID:   "openamsm-mas-context-openapi",
		Kind:       "openapi",
		Repository: "odango-chan/env-openamsm",
		Commit:     "594dec78b95bff4fdc34deac8a3a9681fb41228f",
		Path:       "upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
		GitBlobSha: "46045c2f8f1cae31c6e6c98d5d20addbebe41969",
	}
	if len(catalog.Sources) != 1 {
		t.Fatalf("len(Sources) = %d, want 1", len(catalog.Sources))
	}
	if catalog.Sources[0] != wantSource {
		t.Fatalf("Sources[0] = %+v, want %+v", catalog.Sources[0], wantSource)
	}
	if len(catalog.Bindings) != 8 {
		t.Fatalf("len(Bindings) = %d, want 8", len(catalog.Bindings))
	}
	wantIDs := make([]string, 0, len(shippedV1Bindings))
	for _, expected := range shippedV1Bindings {
		wantIDs = append(wantIDs, expected.BackendOperationID)
	}
	if !reflect.DeepEqual(catalog.IDs(), wantIDs) {
		t.Fatalf("IDs() = %v, want %v", catalog.IDs(), wantIDs)
	}
	for position, expected := range shippedV1Bindings {
		binding := catalog.Bindings[position]
		if binding.BackendOperationID != expected.BackendOperationID {
			t.Fatalf("Bindings[%d].BackendOperationID = %q, want %q", position, binding.BackendOperationID, expected.BackendOperationID)
		}
		if binding.Effect != EffectRead {
			t.Fatalf("%q Effect = %q, want READ", binding.BackendOperationID, binding.Effect)
		}
		if binding.Handler != HandlerSMFContextQuery {
			t.Fatalf("%q Handler = %q", binding.BackendOperationID, binding.Handler)
		}
		if binding.ServiceKey != masServiceKey {
			t.Fatalf("%q ServiceKey = %q", binding.BackendOperationID, binding.ServiceKey)
		}
		if binding.ContextType != expected.ContextType {
			t.Fatalf("%q ContextType = %q, want %q", binding.BackendOperationID, binding.ContextType, expected.ContextType)
		}
		if binding.SourceID != v1SourceID {
			t.Fatalf("%q SourceID = %q, want %q", binding.BackendOperationID, binding.SourceID, v1SourceID)
		}
		if binding.SourceOperationID != expected.OperationID {
			t.Fatalf("%q SourceOperationID = %q, want %q", binding.BackendOperationID, binding.SourceOperationID, expected.OperationID)
		}
		if binding.Generic != nil || binding.Method != "" || binding.Path != "" || len(binding.Arguments) != 0 {
			t.Fatalf("%q has unexpected v2 fields: %+v", binding.BackendOperationID, binding)
		}
	}
	if source, ok := catalog.SourceByID(v1SourceID); !ok || source != wantSource {
		t.Fatalf("SourceByID(%q) = %+v, %v", v1SourceID, source, ok)
	}
	if _, ok := catalog.SourceByID("missing"); ok {
		t.Fatal("SourceByID(missing) = true, want false")
	}
	if _, ok := catalog.Find("missing"); ok {
		t.Fatal("Find(missing) = true, want false")
	}
}

func TestLoadV2(t *testing.T) {
	catalog := mustLoadV2(t)
	if catalog.APIVersion != APIVersionV2 {
		t.Fatalf("APIVersion = %q, want %q", catalog.APIVersion, APIVersionV2)
	}
	if catalog.Version != "2" {
		t.Fatalf("Version = %q, want 2", catalog.Version)
	}
	if catalog.Digest != wantV2Digest {
		t.Fatalf("Digest = %q, want %q", catalog.Digest, wantV2Digest)
	}
	if !reflect.DeepEqual(catalog.Sources, shippedV2Sources) {
		t.Fatalf("Sources = %+v, want %+v", catalog.Sources, shippedV2Sources)
	}
	if len(catalog.Bindings) != 11 {
		t.Fatalf("len(Bindings) = %d, want 11", len(catalog.Bindings))
	}
	if !reflect.DeepEqual(catalog.IDs(), wantV2IDs) {
		t.Fatalf("IDs() = %v, want %v", catalog.IDs(), wantV2IDs)
	}
	for _, expected := range shippedV2Bindings {
		binding := mustFind(t, catalog, expected.BackendOperationID)
		if binding.Effect != expected.Effect {
			t.Fatalf("%q Effect = %q, want %q", expected.BackendOperationID, binding.Effect, expected.Effect)
		}
		if binding.Handler != expected.Handler {
			t.Fatalf("%q Handler = %q, want %q", expected.BackendOperationID, binding.Handler, expected.Handler)
		}
		if binding.ServiceKey != expected.ServiceKey {
			t.Fatalf("%q ServiceKey = %q, want %q", expected.BackendOperationID, binding.ServiceKey, expected.ServiceKey)
		}
		if binding.ContextType != expected.ContextType {
			t.Fatalf("%q ContextType = %q, want %q", expected.BackendOperationID, binding.ContextType, expected.ContextType)
		}
		if binding.SourceID != expected.SourceID {
			t.Fatalf("%q SourceID = %q, want %q", expected.BackendOperationID, binding.SourceID, expected.SourceID)
		}
		if binding.SourceOperationID != expected.SourceOperationID {
			t.Fatalf("%q SourceOperationID = %q, want %q", expected.BackendOperationID, binding.SourceOperationID, expected.SourceOperationID)
		}
		if binding.SourceSymbol != expected.SourceSymbol {
			t.Fatalf("%q SourceSymbol = %q, want %q", expected.BackendOperationID, binding.SourceSymbol, expected.SourceSymbol)
		}
		if binding.Method != expected.Method || binding.Path != expected.Path {
			t.Fatalf("%q Method/Path = %q %q, want %q %q", expected.BackendOperationID, binding.Method, binding.Path, expected.Method, expected.Path)
		}
		if binding.ResponseSchemaRef != expected.ResponseSchemaRef {
			t.Fatalf("%q ResponseSchemaRef = %q, want %q", expected.BackendOperationID, binding.ResponseSchemaRef, expected.ResponseSchemaRef)
		}
		if binding.RequestMediaType != "application/json" {
			t.Fatalf("%q RequestMediaType = %q", expected.BackendOperationID, binding.RequestMediaType)
		}
		if !reflect.DeepEqual(binding.ResponseMediaTypes, []string{"application/json"}) {
			t.Fatalf("%q ResponseMediaTypes = %v", expected.BackendOperationID, binding.ResponseMediaTypes)
		}
		if binding.Generic != nil {
			t.Fatalf("%q Generic = %+v, want nil", expected.BackendOperationID, binding.Generic)
		}
	}

	mas := mustFind(t, catalog, "openamsm.smf.mas.sc.query")
	if len(mas.Arguments) != 2 {
		t.Fatalf("MAS arguments = %d, want 2", len(mas.Arguments))
	}
	supi := mas.Arguments[0]
	if supi.Name != "supi" || supi.Type != "string" || !supi.Required || supi.Pattern != `^imsi-[0-9]{1,15}$` {
		t.Fatalf("supi argument = %+v", supi)
	}
	pduSessionID := mas.Arguments[1]
	if pduSessionID.Name != "pduSessionId" || pduSessionID.Type != "integer" || !pduSessionID.Required {
		t.Fatalf("pduSessionId argument = %+v", pduSessionID)
	}
	requireInt64Pointer(t, pduSessionID.Minimum, 0, "pduSessionId.Minimum")
	requireInt64Pointer(t, pduSessionID.Maximum, 255, "pduSessionId.Maximum")

	metrics := mustFind(t, catalog, "openamsm.amf.ue-info.list")
	if len(metrics.Arguments) != 2 {
		t.Fatalf("metrics arguments = %d, want 2", len(metrics.Arguments))
	}
	page := metrics.Arguments[0]
	if page.Name != "page" || page.Type != "integer" || page.Required {
		t.Fatalf("page argument = %+v", page)
	}
	requireInt64Pointer(t, page.Minimum, 0, "page.Minimum")
	if page.Default != 0 {
		t.Fatalf("page.Default = %#v, want 0", page.Default)
	}
	pageSize := metrics.Arguments[1]
	if pageSize.Name != "pageSize" || pageSize.Type != "integer" || pageSize.Required {
		t.Fatalf("pageSize argument = %+v", pageSize)
	}
	requireInt64Pointer(t, pageSize.Minimum, 1, "pageSize.Minimum")
	requireInt64Pointer(t, pageSize.Maximum, 100, "pageSize.Maximum")
	if pageSize.Default != 100 {
		t.Fatalf("pageSize.Default = %#v, want 100", pageSize.Default)
	}

	if len(catalog.Schemas) != 3 {
		t.Fatalf("len(Schemas) = %d, want 3", len(catalog.Schemas))
	}
	for _, name := range []string{"masContextResult", "metricsPage", "problemDetails"} {
		raw, ok := catalog.Schemas[name]
		if !ok {
			t.Fatalf("Schemas[%q] missing", name)
		}
		if !json.Valid(raw) {
			t.Fatalf("Schemas[%q] is not valid JSON: %s", name, raw)
		}
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatalf("Schemas[%q] unmarshal: %v", name, err)
		}
		if _, ok := object["type"]; !ok {
			t.Fatalf("Schemas[%q] has no type keyword", name)
		}
	}
	// The schemas are stored as canonical JSON, not as YAML.
	if !strings.Contains(string(catalog.Schemas["masContextResult"]), `"required"`) {
		t.Fatalf("masContextResult schema missing required keyword: %s", catalog.Schemas["masContextResult"])
	}
}

func TestLoadDigestIsOverExactBytes(t *testing.T) {
	fixture := genericFixture(t)
	catalog, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load(fixture) error: %v", err)
	}
	sum := sha256.Sum256(fixture)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if catalog.Digest != want {
		t.Fatalf("Digest = %q, want %q", catalog.Digest, want)
	}
	// Loading the same bytes twice is deterministic.
	again, err := Load(fixture)
	if err != nil {
		t.Fatalf("second Load(fixture) error: %v", err)
	}
	if again.Digest != catalog.Digest {
		t.Fatalf("digest is not deterministic: %q vs %q", catalog.Digest, again.Digest)
	}
}

func TestLoadDispatch(t *testing.T) {
	v1, err := Load(providerbindings.V1)
	if err != nil {
		t.Fatalf("Load(v1) error: %v", err)
	}
	if v1.APIVersion != APIVersionV1 {
		t.Fatalf("Load(v1).APIVersion = %q", v1.APIVersion)
	}
	v2, err := Load(providerbindings.V2)
	if err != nil {
		t.Fatalf("Load(v2) error: %v", err)
	}
	if v2.APIVersion != APIVersionV2 {
		t.Fatalf("Load(v2).APIVersion = %q", v2.APIVersion)
	}
	if _, err := Load([]byte("apiVersion: ca-client-tools.provider-bindings/v9\nkind: x\n")); err == nil {
		t.Fatal("Load(unknown apiVersion) error = nil, want error")
	}
	if _, err := Load([]byte("kind: ProviderBackendBindingCatalog\n")); err == nil {
		t.Fatal("Load(missing apiVersion) error = nil, want error")
	}
}

func TestCatalogIDsReturnsCopy(t *testing.T) {
	catalog := mustLoadV1(t)
	ids := catalog.IDs()
	ids[0] = "mutated"
	if again := catalog.IDs(); !reflect.DeepEqual(again, func() []string {
		values := make([]string, 0, len(shippedV1Bindings))
		for _, expected := range shippedV1Bindings {
			values = append(values, expected.BackendOperationID)
		}
		return values
	}()) {
		t.Fatalf("IDs() returned a mutable alias of the catalog state: %v", again)
	}
}

func TestNilCatalogAccessors(t *testing.T) {
	var catalog *Catalog
	if _, ok := catalog.Find("x"); ok {
		t.Fatal("nil Find = true")
	}
	if ids := catalog.IDs(); ids != nil {
		t.Fatalf("nil IDs = %v", ids)
	}
	if _, ok := catalog.SourceByID("x"); ok {
		t.Fatal("nil SourceByID = true")
	}
}

func TestLoadRejectsInvalidDocument(t *testing.T) {
	v1 := providerbindings.V1
	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "invalid utf8", raw: []byte{0xff, 0xfe, 0x00, 0x01}, want: "UTF-8"},
		{name: "empty", raw: []byte(""), want: "empty"},
		{name: "whitespace only", raw: []byte("  \n\t\n"), want: "empty"},
		{name: "null root", raw: []byte("null\n"), want: "root must be a mapping"},
		{name: "sequence root", raw: []byte("- apiVersion: x\n"), want: "root must be a mapping"},
		{name: "scalar root", raw: []byte("hello\n"), want: "root must be a mapping"},
		{name: "multiple documents", raw: append(append([]byte(nil), v1...), []byte("---\napiVersion: ca-client-tools.provider-bindings/v1\n")...), want: "exactly one YAML document"},
		{name: "duplicate root key", want: "duplicate mapping key", raw: mutate(t, v1,
			"kind: ProviderBackendBindingCatalog\n",
			"kind: ProviderBackendBindingCatalog\napiVersion: ca-client-tools.provider-bindings/v1\n")},
		{name: "duplicate nested key", want: "duplicate mapping key", raw: mutateFirst(t, v1,
			"        serviceKey: openamsm-smf-context\n",
			"        serviceKey: openamsm-smf-context\n        serviceKey: openamsm-smf-context\n")},
		{name: "anchor", want: "anchors", raw: mutate(t, v1,
			"    commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f\n",
			"    commit: &commit 594dec78b95bff4fdc34deac8a3a9681fb41228f\n")},
		{name: "anchor and alias", want: "anchors", raw: []byte(`apiVersion: ca-client-tools.provider-bindings/v1
kind: ProviderBackendBindingCatalog
metadata: &meta
  providerId: ca-client-tools.sbi
  source:
    repository: odango-chan/env-openamsm
    commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f
    openApiPath: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml
    openApiGitBlobSha: 46045c2f8f1cae31c6e6c98d5d20addbebe41969
spec: *meta
`)},
		{name: "merge key", want: "merge", raw: mutate(t, v1,
			"spec:\n  bindings:\n",
			"merged: {<<: {}}\nspec:\n  bindings:\n")},
		{name: "non-string mapping key", want: "mapping keys", raw: mutate(t, v1,
			"kind: ProviderBackendBindingCatalog\n",
			"kind: ProviderBackendBindingCatalog\n1: numeric key\n")},
		{name: "unknown root field", want: "not found", raw: mutate(t, v1,
			"kind: ProviderBackendBindingCatalog\n",
			"kind: ProviderBackendBindingCatalog\nreviewer: someone\n")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Load(testCase.raw)
			if err == nil {
				t.Fatalf("Load() error = nil, want error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Load() error = %q, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestV1ContractRejectsMutations(t *testing.T) {
	v1 := providerbindings.V1
	cases := []struct {
		name   string
		mutate func(t *testing.T) []byte
	}{
		{name: "wrong apiVersion", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "ca-client-tools.provider-bindings/v1", "ca-client-tools.provider-bindings/v9")
		}},
		{name: "wrong kind", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "kind: ProviderBackendBindingCatalog", "kind: SomethingElse")
		}},
		{name: "wrong providerId", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "  providerId: ca-client-tools.sbi", "  providerId: ca-client-tools.other")
		}},
		{name: "missing source", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v1, "  source:\n", "spec:\n", "")
		}},
		{name: "wrong repository", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "    repository: odango-chan/env-openamsm", "    repository: someone/env-openamsm")
		}},
		{name: "uppercase commit", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "    commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f", "    commit: 594DEC78B95BFF4FDC34DEAC8A3A9681FB41228F")
		}},
		{name: "short commit", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "    commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f", "    commit: 594dec7")
		}},
		{name: "backslash openapi path", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    openApiPath: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
				"    openApiPath: upstream\\openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml")
		}},
		{name: "absolute openapi path", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    openApiPath: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
				"    openApiPath: /upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml")
		}},
		{name: "dotdot openapi path", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    openApiPath: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
				"    openApiPath: upstream/../openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml")
		}},
		{name: "empty openapi path", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    openApiPath: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml",
				"    openApiPath:")
		}},
		{name: "bad blob sha", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    openApiGitBlobSha: 46045c2f8f1cae31c6e6c98d5d20addbebe41969",
				"    openApiGitBlobSha: not-a-sha")
		}},
		{name: "extra metadata field", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "  providerId: ca-client-tools.sbi\n", "  reviewer: someone\n  providerId: ca-client-tools.sbi\n")
		}},
		{name: "extra spec member", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "spec:\n  bindings:\n", "spec:\n  schemas: {}\n  bindings:\n")
		}},
		{name: "extra binding field", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v1,
				"      handler: openamsmSMFContextQuery\n",
				"      handler: openamsmSMFContextQuery\n      effect: READ\n")
		}},
		{name: "seven bindings", mutate: func(t *testing.T) []byte {
			return cutFrom(t, v1, "    - backendOperationId: openamsm.smf.mas.saa.query", "")
		}},
		{name: "nine bindings", mutate: func(t *testing.T) []byte {
			return mutate(t, v1,
				"    - backendOperationId: openamsm.smf.mas.pa.query",
				"    - backendOperationId: openamsm.smf.mas.sc.query\n      handler: openamsmSMFContextQuery\n      source:\n        serviceKey: openamsm-smf-context\n        contextType: SC\n        operationId: queryScContext\n\n    - backendOperationId: openamsm.smf.mas.pa.query")
		}},
		{name: "wrong handler", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v1, "      handler: openamsmSMFContextQuery", "      handler: genericSBI")
		}},
		{name: "wrong serviceKey", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v1, "        serviceKey: openamsm-smf-context", "        serviceKey: openamsm-other")
		}},
		{name: "wrong operationId", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v1, "        operationId: queryScContext", "        operationId: queryOtherContext")
		}},
		{name: "wrong contextType", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v1, "        contextType: SC", "        contextType: XX")
		}},
		{name: "missing contextType", mutate: func(t *testing.T) []byte {
			return mutate(t, v1, "        contextType: SC\n", "")
		}},
		{name: "wrong binding order", mutate: func(t *testing.T) []byte {
			first := "    - backendOperationId: openamsm.smf.mas.sc.query"
			second := "    - backendOperationId: openamsm.smf.mas.pa.query"
			return replaceRange(t, v1, first, second, second)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Load(testCase.mutate(t)); err == nil {
				t.Fatalf("Load(mutated v1) error = nil, want error")
			}
		})
	}
}

func TestV2StructuralRejectsMutations(t *testing.T) {
	v2 := providerbindings.V2
	cases := []struct {
		name   string
		mutate func(t *testing.T) []byte
	}{
		{name: "wrong kind", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "kind: ProviderBackendBindingCatalog", "kind: ToolCatalog")
		}},
		{name: "wrong providerId", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "  providerId: ca-client-tools.sbi", "  providerId: ca-client-tools.other")
		}},
		{name: "wrong catalogVersion", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "  catalogVersion: \"2\"", "  catalogVersion: \"3\"")
		}},
		{name: "missing catalogVersion", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "  catalogVersion: \"2\"\n", "")
		}},
		{name: "duplicate source id", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "    - sourceId: openamsm-smf-init-c", "    - sourceId: openamsm-amf-init-c")
		}},
		{name: "invalid source kind", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      kind: openamsm-source", "      kind: git")
		}},
		{name: "uppercase commit", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f",
				"      commit: 594DEC78B95BFF4FDC34DEAC8A3A9681FB41228F")
		}},
		{name: "source path backslash", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      path: upstream/openamsm/src/amf/init.c", "      path: upstream\\openamsm\\src\\amf\\init.c")
		}},
		{name: "source path dotdot", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      path: upstream/openamsm/src/amf/init.c", "      path: upstream/../openamsm/src/amf/init.c")
		}},
		{name: "bad git blob sha", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      gitBlobSha: 1ee5c9b148c68ceb7ceb377a29e982b3c9985878", "      gitBlobSha: 1ee5c9b1")
		}},
		{name: "missing sources", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v2, "  sources:\n", "spec:\n", "  sources: []\n")
		}},
		{name: "empty schemas", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v2, "  schemas:\n", "  bindings:\n", "  schemas: {}\n")
		}},
		{name: "schema not an object", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v2, "    problemDetails:\n", "  bindings:\n", "    problemDetails: 42\n")
		}},
		{name: "schema unknown keyword", mutate: func(t *testing.T) []byte {
			return mutate(t, v2,
				"    problemDetails:\n      type: object\n",
				"    problemDetails:\n      type: object\n      description: not allowed\n")
		}},
		{name: "schema unsupported type", mutate: func(t *testing.T) []byte {
			return mutate(t, v2,
				"    problemDetails:\n      type: object\n",
				"    problemDetails:\n      type: widget\n")
		}},
		{name: "schema invalid pattern", mutate: func(t *testing.T) []byte {
			return mutate(t, v2,
				"          pattern: '^[A-Z][A-Z0-9_]{0,63}$'",
				"          pattern: '['")
		}},
		{name: "empty bindings", mutate: func(t *testing.T) []byte {
			return cutFrom(t, v2, "  bindings:\n", "  bindings: []\n")
		}},
		{name: "duplicate binding id", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "    - backendOperationId: openamsm.smf.pdu-info.list", "    - backendOperationId: openamsm.amf.gnb-info.list")
		}},
		{name: "invalid backendOperationId", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "    - backendOperationId: openamsm.smf.pdu-info.list", "    - backendOperationId: 'bad id'")
		}},
		{name: "unknown handler", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      handler: openamsmSMFContextQuery", "      handler: openamsmMagicHandler")
		}},
		{name: "invalid effect", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      effect: READ", "      effect: WRITE")
		}},
		{name: "missing request", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      request:\n        mediaType: application/json\n      response:\n        mediaTypes: [application/json]\n        schemaRef: masContextResult",
				"      response:\n        mediaTypes: [application/json]\n        schemaRef: masContextResult")
		}},
		{name: "wrong request media type", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "        mediaType: application/json\n      response:", "        mediaType: application/xml\n      response:")
		}},
		{name: "empty response media types", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "        mediaTypes: [application/json]", "        mediaTypes: []")
		}},
		{name: "unknown schema ref", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "        schemaRef: masContextResult", "        schemaRef: missingSchema")
		}},
		{name: "openapi source with symbol", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        operationId: queryScContext\n",
				"        operationId: queryScContext\n        sourceSymbol: injected\n")
		}},
		{name: "openapi source with method and path", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        operationId: queryScContext\n",
				"        operationId: queryScContext\n        method: GET\n        path: /injected\n")
		}},
		{name: "openamsm source missing symbol", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "        sourceSymbol: amf_dump_ue_info\n", "")
		}},
		{name: "openamsm source invalid method", mutate: func(t *testing.T) []byte {
			return mutate(t, v2,
				"        method: GET\n        path: /ue-info",
				"        method: TRACE\n        path: /ue-info")
		}},
		{name: "openamsm source relative path", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "        path: /ue-info", "        path: ue-info")
		}},
		{name: "openamsm source path with query", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "        path: /gnb-info", "        path: /gnb-info?page=0")
		}},
		{name: "metrics binding with generic", mutate: func(t *testing.T) []byte {
			return mutate(t, v2,
				"        schemaRef: metricsPage\n      arguments:\n        - name: page\n          type: integer\n          required: false\n          minimum: 0\n          default: 0\n        - name: pageSize\n          type: integer\n          required: false\n          minimum: 1\n          maximum: 100\n          default: 100\n\n    - backendOperationId: openamsm.amf.gnb-info.list",
				"        schemaRef: metricsPage\n      generic:\n        method: GET\n        path: /injected\n        responseMediaTypes: [application/json]\n      arguments:\n        - name: page\n          type: integer\n          required: false\n          minimum: 0\n          default: 0\n        - name: pageSize\n          type: integer\n          required: false\n          minimum: 1\n          maximum: 100\n          default: 100\n\n    - backendOperationId: openamsm.amf.gnb-info.list")
		}},
		{name: "mas wrong supi pattern", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          pattern: '^imsi-[0-9]{1,15}$'", "          pattern: '.*'")
		}},
		{name: "mas missing supi argument", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v2,
				"        - name: supi\n          type: string\n          required: true\n          pattern: '^imsi-[0-9]{1,15}$'\n",
				"        - name: pduSessionId\n", "")
		}},
		{name: "mas pduSessionId out of range", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          maximum: 255", "          maximum: 256")
		}},
		{name: "mas pduSessionId min greater than max", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          minimum: 0\n          maximum: 255", "          minimum: 300\n          maximum: 255")
		}},
		{name: "mas invalid argument type", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        - name: supi\n          type: string\n",
				"        - name: supi\n          type: number\n")
		}},
		{name: "mas invalid argument pattern", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          pattern: '^imsi-[0-9]{1,15}$'", "          pattern: '['")
		}},
		{name: "mas duplicate argument name", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        - name: pduSessionId\n          type: integer\n",
				"        - name: supi\n          type: integer\n")
		}},
		{name: "mas unknown argument field", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        - name: supi\n          type: string\n",
				"        - name: supi\n          type: string\n          maximumLength: 15\n")
		}},
		{name: "metrics wrong default type", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"        - name: page\n          type: integer\n          required: false\n          minimum: 0\n          default: 0",
				"        - name: page\n          type: integer\n          required: false\n          minimum: 0\n          default: fifty")
		}},
		{name: "metrics wrong page minimum", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          minimum: 1\n          maximum: 100", "          minimum: 0\n          maximum: 100")
		}},
		{name: "metrics wrong page default", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "          default: 100", "          default: 50")
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := testCase.mutate(t)
			if _, err := Load(mutated); err == nil {
				t.Fatalf("Load(mutated v2) error = nil, want error")
			}
		})
	}
}

func TestShippedV2InventoryRejectsMutations(t *testing.T) {
	v2 := providerbindings.V2
	cases := []struct {
		name   string
		mutate func(t *testing.T) []byte
	}{
		{name: "wrong source repository", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      repository: odango-chan/env-openamsm",
				"      repository: odango-chan/env-openamsm-fork")
		}},
		{name: "wrong source commit", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f",
				"      commit: 0000000000000000000000000000000000000000")
		}},
		{name: "wrong source path", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      path: upstream/openamsm/src/amf/init.c",
				"      path: upstream/openamsm/src/smf/init.c")
		}},
		{name: "wrong source blob", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2,
				"      gitBlobSha: 1ee5c9b148c68ceb7ceb377a29e982b3c9985878",
				"      gitBlobSha: 0000000000000000000000000000000000000000")
		}},
		{name: "extra source", mutate: func(t *testing.T) []byte {
			extra := "    - sourceId: extra-openapi\n      kind: openapi\n      repository: 3gpp/5G_APIs\n      commit: 0123456789abcdef0123456789abcdef01234567\n      path: extra.yaml\n      gitBlobSha: 0123456789abcdef0123456789abcdef01234567\n"
			return mutate(t, v2, "  sources:\n    - sourceId: openamsm-mas-context-openapi", "  sources:\n"+extra+"    - sourceId: openamsm-mas-context-openapi")
		}},
		{name: "missing source", mutate: func(t *testing.T) []byte {
			mutated := replaceRange(t, v2, "    - sourceId: openamsm-smf-init-c", "spec:\n", "")
			return mutate(t, mutated, "        sourceId: openamsm-smf-init-c", "        sourceId: openamsm-amf-init-c")
		}},
		{name: "missing binding", mutate: func(t *testing.T) []byte {
			return replaceRange(t, v2,
				"    - backendOperationId: openamsm.smf.mas.sc.query",
				"    - backendOperationId: openamsm.smf.mas.pa.query", "")
		}},
		{name: "extra binding", mutate: func(t *testing.T) []byte {
			extra := "    - backendOperationId: openamsm.amf.ue-info.list.extra\n      effect: READ\n      handler: openamsmAMFMetricsList\n      serviceKey: openamsm-amf-metrics\n      source:\n        sourceId: openamsm-amf-init-c\n        operationId: listAmfUEInfoExtra\n        sourceSymbol: amf_dump_ue_info\n        method: GET\n        path: /ue-info-extra\n      request:\n        mediaType: application/json\n      response:\n        mediaTypes: [application/json]\n        schemaRef: metricsPage\n      arguments:\n        - name: page\n          type: integer\n          required: false\n          minimum: 0\n          default: 0\n        - name: pageSize\n          type: integer\n          required: false\n          minimum: 1\n          maximum: 100\n          default: 100\n"
			return mutate(t, v2, "  bindings:\n    - backendOperationId: openamsm.smf.mas.sc.query", "  bindings:\n"+extra+"    - backendOperationId: openamsm.smf.mas.sc.query")
		}},
		{name: "mutation effect in shipped artifact", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "      effect: READ", "      effect: MUTATION")
		}},
		{name: "context mismatch", mutate: func(t *testing.T) []byte {
			return mutate(t, v2, "      contextType: SC\n", "      contextType: PA\n")
		}},
		{name: "wrong source operation", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, v2, "        operationId: queryScContext", "        operationId: queryPaContext")
		}},
		{name: "wrong metrics handler family", mutate: func(t *testing.T) []byte {
			mutated := mutateAll(t, v2, "      handler: openamsmAMFMetricsList", "      handler: openamsmSMFMetricsList")
			return mutateAll(t, mutated, "      serviceKey: openamsm-amf-metrics", "      serviceKey: openamsm-smf-metrics")
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := testCase.mutate(t)
			// The mutation must stay structurally valid; only the frozen shipped
			// inventory check rejects it.
			if _, err := Load(mutated); err != nil {
				t.Fatalf("mutated artifact is not structurally valid: %v", err)
			}
			if _, err := loadV2Shipped(mutated); err == nil {
				t.Fatalf("loadV2Shipped(mutated v2) error = nil, want error")
			}
		})
	}
}

func TestGenericBindingsAccepted(t *testing.T) {
	fixture := genericFixture(t)
	catalog, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load(generic fixture) error: %v", err)
	}
	if len(catalog.Bindings) != 14 {
		t.Fatalf("len(Bindings) = %d, want 14", len(catalog.Bindings))
	}
	if len(catalog.Sources) != 5 {
		t.Fatalf("len(Sources) = %d, want 5", len(catalog.Sources))
	}
	search := mustFind(t, catalog, "3gpp.nsmf.pdusession.search")
	if search.Handler != HandlerGenericSBI || search.Effect != EffectRead {
		t.Fatalf("search binding profile = %+v", search)
	}
	if search.Method != "POST" || search.Path != "/nsmf-pdusession/v1/sm-contexts/search" {
		t.Fatalf("search method/path = %q %q", search.Method, search.Path)
	}
	if search.Generic == nil {
		t.Fatal("search Generic = nil")
	}
	if search.Generic.Body == nil || search.Generic.Body.Kind != "json" || search.Generic.Body.MediaType != "application/json" {
		t.Fatalf("search body = %+v", search.Generic.Body)
	}
	if !json.Valid(search.Generic.Body.RequestSchema) {
		t.Fatalf("search request schema is not valid JSON: %s", search.Generic.Body.RequestSchema)
	}
	if len(search.Generic.QueryParameters) != 1 || search.Generic.QueryParameters[0].Name != "supported-features" {
		t.Fatalf("search query parameters = %+v", search.Generic.QueryParameters)
	}
	if len(search.Generic.HeaderBindings) != 1 || search.Generic.HeaderBindings[0].Name != "Accept" {
		t.Fatalf("search header bindings = %+v", search.Generic.HeaderBindings)
	}
	if search.Generic.Idempotency != nil {
		t.Fatalf("READ generic binding must not require idempotency: %+v", search.Generic.Idempotency)
	}
	if !json.Valid(search.Generic.ResponseSchema) {
		t.Fatalf("search response schema is not valid JSON: %s", search.Generic.ResponseSchema)
	}

	update := mustFind(t, catalog, "3gpp.nsmf.smcontext.update")
	if update.Effect != EffectMutation {
		t.Fatalf("update effect = %q, want MUTATION", update.Effect)
	}
	if update.Generic == nil || update.Generic.Idempotency == nil {
		t.Fatalf("update idempotency = %+v", update.Generic)
	}
	if update.Generic.Idempotency.Mode != "IDEMPOTENCY_KEY" || update.Generic.Idempotency.KeyArgument != "correlationId" {
		t.Fatalf("update idempotency = %+v", update.Generic.Idempotency)
	}
	if len(update.Generic.PathParameters) != 1 || update.Generic.PathParameters[0].Name != "smContextRef" {
		t.Fatalf("update path parameters = %+v", update.Generic.PathParameters)
	}

	list := mustFind(t, catalog, "3gpp.namf.comm.ue-context.list")
	if list.Generic == nil || list.Generic.Body != nil {
		t.Fatalf("list generic body = %+v, want nil", list.Generic)
	}
	if list.Method != "GET" {
		t.Fatalf("list method = %q, want GET", list.Method)
	}
}

func TestGenericBindingRejectsMutations(t *testing.T) {
	fixture := func(t *testing.T) []byte { return genericFixture(t) }
	cases := []struct {
		name   string
		mutate func(t *testing.T) []byte
	}{
		{name: "mutation without idempotency", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"\n        idempotency:\n          mode: IDEMPOTENCY_KEY\n          keyArgument: correlationId", "")
		}},
		{name: "unsupported body kind", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        body:\n          kind: json\n          mediaType: application/json\n          requestSchema:\n            type: object\n            additionalProperties: false\n            properties:\n              imsi:\n                type: string\n                pattern: '^imsi-[0-9]{1,15}$'",
				"        body:\n          kind: cbor\n          mediaType: application/json\n          requestSchema:\n            type: object")
		}},
		{name: "json body wrong media type", mutate: func(t *testing.T) []byte {
			return mutateFirst(t, fixture(t),
				"          mediaType: application/json\n          requestSchema:",
				"          mediaType: text/plain\n          requestSchema:")
		}},
		{name: "multipart body missing content type parameter", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        body:\n          kind: json\n          mediaType: application/json\n          requestSchema:\n            type: object\n            additionalProperties: false\n            properties:\n              imsi:\n                type: string\n                pattern: '^imsi-[0-9]{1,15}$'",
				"        body:\n          kind: multipart\n          mediaType: multipart/related\n          multipartRoot: true\n          maxParts: 2\n          requestSchema:\n            type: object")
		}},
		{name: "path template parameter not declared", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t), "{smContextRef}/modify", "{otherRef}/modify")
		}},
		{name: "duplicate header binding", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"          - name: Accept\n            value: application/json\n",
				"          - name: Accept\n            value: application/json\n          - name: Accept\n            value: application/json\n")
		}},
		{name: "invalid method", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        method: POST\n        path: /nsmf-pdusession/v1/sm-contexts/search",
				"        method: TRACE\n        path: /nsmf-pdusession/v1/sm-contexts/search")
		}},
		{name: "unknown schema keyword", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        responseSchema:\n          type: object\n          properties:\n            items:",
				"        responseSchema:\n          type: object\n          description: nope\n          properties:\n            items:")
		}},
		{name: "relative path", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t), "        path: /namf-comm/v1/ue-contexts", "        path: namf-comm/v1/ue-contexts")
		}},
		{name: "unknown idempotency key argument", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t), "          keyArgument: correlationId", "          keyArgument: missingArgument")
		}},
		{name: "openapi generic source with symbol", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        sourceId: 3gpp-namf-comm-openapi\n        operationId: listUeContext",
				"        sourceId: 3gpp-namf-comm-openapi\n        operationId: listUeContext\n        sourceSymbol: injected")
		}},
		{name: "openapi generic source with method", mutate: func(t *testing.T) []byte {
			return mutate(t, fixture(t),
				"        sourceId: 3gpp-namf-comm-openapi\n        operationId: listUeContext",
				"        sourceId: 3gpp-namf-comm-openapi\n        operationId: listUeContext\n        method: GET")
		}},
		{name: "missing generic spec", mutate: func(t *testing.T) []byte {
			return replaceRange(t, fixture(t),
				"      generic:\n        method: POST\n        path: /nsmf-pdusession/v1/sm-contexts/search",
				"    - backendOperationId: 3gpp.nsmf.smcontext.update\n", "")
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := testCase.mutate(t)
			if _, err := Load(mutated); err == nil {
				t.Fatalf("Load(mutated generic fixture) error = nil, want error")
			}
		})
	}
}

func TestGenericSpecFieldsRejectedOnTypedHandler(t *testing.T) {
	v2 := providerbindings.V2
	mutated := mutateFirst(t, v2,
		"      request:\n        mediaType: application/json\n      response:\n        mediaTypes: [application/json]\n        schemaRef: masContextResult",
		"      generic:\n        method: GET\n        path: /injected\n        responseMediaTypes: [application/json]\n      request:\n        mediaType: application/json\n      response:\n        mediaTypes: [application/json]\n        schemaRef: masContextResult")
	if _, err := Load(mutated); err == nil {
		t.Fatal("Load(typed handler with generic spec) error = nil, want error")
	}
}

func TestMergeKeyDetectionDoesNotRejectStringValue(t *testing.T) {
	// A literal "<<" string value is not a YAML merge key; only the !!merge tag
	// is forbidden. This guards against over-rejecting legitimate schema consts.
	mutated := mutate(t, providerbindings.V2,
		"        cause:\n          type: string\n          pattern: '^[A-Z][A-Z0-9_]{0,63}$'",
		"        cause:\n          type: string\n          const: '<<'\n          pattern: '^[A-Z][A-Z0-9_]{0,63}$'")
	catalog, err := Load(mutated)
	if err != nil {
		t.Fatalf("Load(schema with string << value) error: %v", err)
	}
	if !json.Valid(catalog.Schemas["problemDetails"]) {
		t.Fatalf("problemDetails schema is not valid JSON: %s", catalog.Schemas["problemDetails"])
	}
	var problemDetails map[string]any
	if err := json.Unmarshal(catalog.Schemas["problemDetails"], &problemDetails); err != nil {
		t.Fatalf("problemDetails schema unmarshal: %v", err)
	}
	properties, ok := problemDetails["properties"].(map[string]any)
	if !ok {
		t.Fatalf("problemDetails properties = %#v", problemDetails["properties"])
	}
	cause, ok := properties["cause"].(map[string]any)
	if !ok {
		t.Fatalf("problemDetails cause schema = %#v", properties["cause"])
	}
	if cause["const"] != "<<" {
		t.Fatalf("problemDetails cause const = %#v, want <<", cause["const"])
	}
}

func TestShippedV2AllRead(t *testing.T) {
	catalog := mustLoadV2(t)
	for _, binding := range catalog.Bindings {
		if binding.Effect != EffectRead {
			t.Fatalf("shipped v2 binding %q effect = %q, want READ", binding.BackendOperationID, binding.Effect)
		}
	}
}

# Provider Backend Binding Catalog v2

Status: **implemented in this branch** (`feat/unified-provider-read-tools-20261117`).
This document is normative for the additive v2 catalog artifact and its Go
loader (`internal/catalog`). It does not describe Host/Runtime policy.

The v2 artifact is trusted package content embedded in the Provider build. It is
**not** a ToolCatalog, **not** a CapabilityContracts grant and **not** a
Skill-authored Operation Manifest. Loading it grants nothing.

## 1. Purpose

The catalog is the single trusted mapping from a private backend operation ID to
one executor profile and its reviewed metadata:

- the typed handler profile that executes the operation;
- the trusted service key used to resolve the destination in deployment
  configuration;
- the read/mutation effect classification;
- the pinned evidence source (OpenAPI document or OpenAMSM source file);
- the typed argument profile, or the full generic SBI wire contract.

v2 is an **additive extension** of the frozen v1 contract. It keeps every v1
private backend operation ID and adds the three source-verified OpenAMSM list
operations so the unified Provider can expose all eleven existing read-only
tools. The v1 artifact and its loader behaviour are unchanged; v1 stays frozen.

## 2. Artifacts and embedding

Two artifacts exist under `contracts/provider-bindings/`:

| Artifact | apiVersion | File |
|---|---|---|
| v1 | `ca-client-tools.provider-bindings/v1` | `v1/provider-bindings.yaml` |
| v2 | `ca-client-tools.provider-bindings/v2` | `v2/provider-bindings.yaml` |

`contracts/provider-bindings/embed.go` embeds the exact bytes as `V1` and `V2`.
The YAML files are the only authoritative copies; the loader never
re-serializes them for identity purposes and no other copy may exist.

`internal/catalog` exposes:

- `LoadV1() (*Catalog, error)` — loads the frozen v1 artifact;
- `LoadV2() (*Catalog, error)` — loads v2 and enforces the frozen shipped
  inventory;
- `Load(raw []byte) (*Catalog, error)` — detects `apiVersion` and applies the
  structural contract; used by tests and future version dispatch;
- `Catalog.Find`, `Catalog.IDs`, `Catalog.SourceByID` — read-only lookups.

`Catalog` is immutable after a successful load. The loader fully initializes
its slices, schema map and private lookup indexes; callers must not mutate the
exposed values.

## 3. Root shape

Both artifacts are a single YAML document with exactly this root shape:

~~~yaml
apiVersion: ca-client-tools.provider-bindings/v2
kind: ProviderBackendBindingCatalog
metadata: { ... }
spec: { ... }
~~~

No other root key is accepted. `kind` must be exactly
`ProviderBackendBindingCatalog`.

## 4. v2 metadata

~~~yaml
metadata:
  providerId: ca-client-tools.sbi
  catalogVersion: "2"
  sources:
    - sourceId: openamsm-mas-context-openapi
      kind: openapi
      repository: odango-chan/env-openamsm
      commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f
      path: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml
      gitBlobSha: 46045c2f8f1cae31c6e6c98d5d20addbebe41969
    - ...
~~~

- `providerId` must be exactly `ca-client-tools.sbi`.
- `catalogVersion` must be exactly `"2"` (string). A different version is a
  different artifact contract, not a forward-compatible variant.
- `sources` is the complete, ordered pin inventory. Every source ID is unique.
  The shipped v2 artifact must declare exactly the three entries in §5.
- `kind` is `openapi` or `openamsm-source`; any other kind is rejected.
- `commit` and `gitBlobSha` are exactly 40 lowercase hexadecimal characters.
- `path` is a normalized repository-relative path: non-empty, `/` separators,
  no leading `/`, no `\`, no `.`/`..` segments, no empty segments.
- Every `spec.bindings[*].source.sourceId` must resolve to a declared source.

## 5. Shipped v2 source pins

Repository: `odango-chan/env-openamsm`, commit
`594dec78b95bff4fdc34deac8a3a9681fb41228f`.

| sourceId | kind | path | gitBlobSha |
|---|---|---|---|
| `openamsm-mas-context-openapi` | `openapi` | `upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml` | `46045c2f8f1cae31c6e6c98d5d20addbebe41969` |
| `openamsm-amf-init-c` | `openamsm-source` | `upstream/openamsm/src/amf/init.c` | `1ee5c9b148c68ceb7ceb377a29e982b3c9985878` |
| `openamsm-smf-init-c` | `openamsm-source` | `upstream/openamsm/src/smf/init.c` | `ce8aea73ddcc0dddb84f6c839fe2f4b6c096a88b` |

`LoadV2` requires exactly these three sources, in this order. Adding, removing,
reordering or repinning a source is a catalog artifact change and must be
reviewed as such.

The frozen v1 artifact has no explicit source ID. The loader exposes its
`metadata.source` as a single `Source` with the derived stable ID
`openamsm-mas-context-openapi`, kind `openapi`, and `Path` mapped from
`metadata.source.openApiPath`. Every v1 binding reports that source ID.

## 6. v2 spec.schemas

`spec.schemas` is a non-empty map from schema reference name to a JSON schema
object. The loader validates each schema structurally against a closed keyword
set, then stores a canonical JSON encoding in `Catalog.Schemas`:

- `type` — string or non-empty array of strings from `object`, `array`,
  `string`, `integer`, `number`, `boolean`, `null`;
- `properties` — non-empty object of property name to schema;
- `required` — non-empty array of unique non-empty strings;
- `additionalProperties` — boolean or schema;
- `items` — schema;
- `enum` — non-empty array;
- `const` — any JSON value;
- `minimum`, `maximum` — finite numbers;
- `minLength`, `maxLength` — non-negative integers;
- `pattern` — string compiling as a Go regular expression;
- `oneOf` — non-empty array of schemas.

Any other keyword is rejected. Schemas are documentation of the trusted wire
contract for the Provider/local schema checks; they are not a general-purpose
JSON Schema validator and never accept caller-authored schemas.

## 7. v2 spec.bindings

Every binding declares:

~~~yaml
- backendOperationId: openamsm.smf.mas.sc.query   # required, unique, stable
  effect: READ                                    # READ | MUTATION
  handler: openamsmSMFContextQuery                # one of the handler profiles
  serviceKey: openamsm-smf-context                # trusted config lookup key
  contextType: SC                                 # only for SMF context bindings
  source:
    sourceId: openamsm-mas-context-openapi        # must resolve to a source
    operationId: queryScContext                   # required
  request:
    mediaType: application/json                   # required, exactly JSON
  response:
    mediaTypes: [application/json]                # non-empty
    schemaRef: masContextResult                   # must exist in spec.schemas
  arguments: [ ... ]                              # typed profile, see §9
~~~

Field rules:

- `backendOperationId` is a stable private operation ID (`[A-Za-z0-9][A-Za-z0-9._-]*`)
  and must be unique in the artifact. It is never a public tool identity.
- `effect` is `READ` or `MUTATION`. In the shipped v2 artifact every binding is
  `READ`; `LoadV2` rejects a shipped artifact that contains a mutation.
- `handler` selects exactly one profile from §8.
- `serviceKey` is a logical deployment configuration key, not an origin. The
  catalog never carries URLs, origins or credentials.
- `source.operationId` is required for every binding.
- `response.schemaRef`, when present, must name a declared schema. Typed
  handler bindings require it.
- `request.mediaType` is required and must be exactly `application/json` for
  every binding. Actual SBI request bodies/media types beyond this are declared
  only inside a `genericSBI` binding.
- Unknown fields at any level are rejected, so a binding cannot smuggle
  `origin`, `url`, `token`, `headers`, `method` or `path` into a typed handler
  profile.

## 8. Handler profiles

| Handler | Used for | Source kind | Required binding fields |
|---|---|---|---|
| `openamsmSMFContextQuery` | eight SMF context-query SBI operations | `openapi` | `contextType` in `SC, PA, UPN, SSM, IAM, UPS, CA, SAA`; `serviceKey: openamsm-smf-context`; `response.schemaRef`; exact argument profile |
| `openamsmAMFMetricsList` | AMF `/ue-info` and `/gnb-info` list operations | `openamsm-source` | `source.sourceSymbol`, `source.method`, `source.path`; `serviceKey: openamsm-amf-metrics`; no `contextType`; `response.schemaRef`; metrics argument profile |
| `openamsmSMFMetricsList` | SMF `/pdu-info` list operation | `openamsm-source` | `source.sourceSymbol`, `source.method`, `source.path`; `serviceKey: openamsm-smf-metrics`; no `contextType`; `response.schemaRef`; metrics argument profile |
| `genericSBI` | standardized SBI operations described by the catalog itself | `openapi` | a complete `generic` spec, see §10; no `contextType`; MUTATION requires idempotency |

Source rules per source kind:

- `openapi` source bindings must not set `sourceSymbol`, `source.method` or
  `source.path`. The OpenAPI pin proves the wire contract; a typed handler owns
  its route, and a generic binding carries method/path itself.
- `openamsm-source` source bindings require non-empty `operationId`,
  `sourceSymbol`, a supported `method` (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`)
  and an absolute normalized `path`: starts with `/`, no `?`, `#` or `\`, no
  `.`/`..` segments, no empty segments, printable ASCII.

Typed handler bindings (the first three profiles) must not declare a `generic`
object. This prevents a caller-shaped wire contract from being attached to an
operation that is executed by a typed client.

The metrics listener is plain HTTP without application authentication; the
catalog does not change that. Origins must remain trusted and
network-restricted by deployment configuration.

## 9. Argument profiles

Every argument declares `name`, `type` (`string`, `integer` or `boolean`),
`required`, optional `minimum`/`maximum` (integer only), optional
`minLength`/`maxLength`/`pattern` (string only) and an optional type-correct
`default` that must also satisfy the declared bounds/pattern. Optional query
parameters apply their trusted default when omitted; required parameters remain
required. Argument names are unique within a list. Unknown argument fields are
rejected.

Shipped profiles:

- **SMF context bindings** declare exactly two required arguments in order:
  - `supi` — `string`, pattern `^imsi-[0-9]{1,15}$`;
  - `pduSessionId` — `integer`, minimum `0`, maximum `255`.
- **Metrics bindings** declare exactly two optional arguments in order:
  - `page` — `integer`, minimum `0`, default `0`;
  - `pageSize` — `integer`, minimum `1`, maximum `100`, default `100`.

`LoadV2` preserves these profiles exactly; a changed pattern, bound or default
is an artifact change, not a compatible extension.

## 10. genericSBI binding shape

A `genericSBI` binding declares the complete trusted wire contract:

~~~yaml
- backendOperationId: 3gpp.nsmf.pdusession.search
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
    pathParameters:
      - name: smContextRef
        type: string
        required: true
    queryParameters:
      - name: supported-features
        type: string
        required: false
    headerBindings:
      - name: Accept
        value: application/json
    body:
      kind: multipart       # json | binary | multipart
      mediaType: multipart/related
      requestSchema: { type: object, ... }
      multipartRoot: true
      maxParts: 4              # includes the JSON root
      contentTypeParam: 3gpp
      parts:
        - name: n1Message
          contentType: application/vnd.3gpp.5gnas
          jsonPointerField: /n1MessageContent
          required: true
    responseMediaTypes: [multipart/related]
    responseSchema: { type: object, ... }
    responseParts:
      - name: n2Message
        contentType: application/vnd.3gpp.ngap
        jsonPointerField: /n2MessageContent
        required: true
    idempotency:
      mode: IDEMPOTENCY_KEY
      keyArgument: correlationId
~~~

Validation rules:

- `method` is one of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`; `path` is an
  absolute normalized path as defined in §8.
- Path template `{name}` placeholders must match the declared
  `pathParameters` exactly. Path parameters must be `required`; a name may not
  be declared as both a path and a query parameter.
- `headerBindings` are trusted fixed name/value pairs. They are never supplied
  by the Agent. Header names must be valid HTTP tokens, values must be trimmed,
  non-empty and free of CR/LF/NUL, and names must be unique case-insensitively.
- `body` is optional. `json` bodies require media type `application/json` and a
  JSON object schema; `binary` bodies must not declare a JSON schema;
  `multipart` request bodies require `multipart/related`, `multipartRoot: true`,
  a JSON root schema, and one or more closed `parts` declarations. Each part
  binds a unique name, media type and JSON pointer into the root object.
  `maxParts` includes the JSON root and must be at least one plus the declared
  binary part count.
- `responseMediaTypes` is non-empty and unique; `responseSchema` is optional
  and validated with the same closed schema keyword set. A
  `multipart/related` response requires closed `responseParts` declarations;
  response parts are rejected when multipart is not declared.
- `idempotency` is required for `MUTATION` generic bindings and rejected for
  typed handler bindings. `mode` is `NONE` or `IDEMPOTENCY_KEY`; the latter
  requires `keyArgument` to name a declared argument, while `NONE` forbids
  one. The catalog-to-executor bridge maps `IDEMPOTENCY_KEY` to the executor's
  `REQUIRED` mode and converts the declared argument name to its top-level
  JSON Pointer. Absence of an idempotency contract means no automatic retry is
  permitted (Host/Runtime policy still decides whether any retry is allowed).
- For uniform access, the loader mirrors `generic.method`/`generic.path` into
  `Binding.Method`/`Binding.Path`. `Binding.Generic` holds the typed wire spec.

**No production genericSBI binding is shipped in the v2 artifact.** The loader
supports and validates the shape, and the tests exercise READ, MUTATION and
idempotency fixtures, but the eleven shipped bindings remain typed read
operations. Adding a real generic binding requires the per-operation evidence
gate (3GPP OpenAPI pin, deployment source support, capability map entry) before
it may appear in the embedded artifact.

## 11. Availability versus authorization

The catalog **never authorizes** anything and **never decides visibility**:

- The catalog entry only states what the Provider can execute and how.
- Public callable identity, default tool registration, activation and
  deployment availability are Host-owned. A missing/unconfigured trusted
  service makes a binding unavailable at the Provider layer; availability is
  Provider-app state, not catalog state.
- Runtime owns callable admission and Capability authorization. A catalog
  entry, protocol method or CLI command never grants permission.
- Sensitive operations (for example AMF metrics) remain Host-private unless a
  separate contract authorizes a safe projection.
- The catalog carries no public operation IDs, capability IDs, scope, owner
  outcome or evidence semantics.

## 12. Versioning and extension rules

- v1 is frozen. Its root shape, eight bindings, order, source pin and handler/
  serviceKey/operationId triples may not change. Compatible additions belong in
  a new versioned artifact.
- v2 is additive to v1: every v1 backend operation ID must remain present with
  the same handler profile, service key and source operation ID.
- A new artifact version is required for any change to: root shape, source kind
  vocabulary, handler profile vocabulary, argument semantics, generic binding
  shape, or the meaning of `effect`.
- Adding a binding, pin or schema to an existing shipped artifact is still an
  artifact/digest change and must be reviewed, sourced and tested. It must not
  silently change the meaning of an existing backend operation ID.
- `catalogVersion` identifies the artifact contract (`"2"`). It is not the
  Provider protocol version and not the public tool set version.
- Unknown fields are always rejected; there is no forward-compatible "ignore
  what you do not understand" mode in the trusted loader.

## 13. Validation rules summary

The loader fails closed and returns an error for:

1. invalid UTF-8, empty documents, non-mapping roots, more than one YAML
   document;
2. YAML anchors, aliases and merge keys (`<<`), non-string mapping keys, and
   duplicate mapping keys at any depth;
3. unknown fields at any struct level (strict typed decoding) and unknown
   schema keywords;
4. missing or extra root members, wrong `apiVersion`/`kind`/`providerId`;
5. v1: any deviation from the frozen eight-binding inventory;
6. v2: wrong `catalogVersion`, malformed or unresolved source pins, empty
   schema or binding sets, unknown handler/effect, handler-profile violations,
   invalid or unresolved schema references, unknown arguments fields,
   malformed bounds/patterns/defaults, and generic binding violations;
7. v2 shipped inventory (`LoadV2`): not exactly the three pinned sources, not
   exactly the eleven required backend operation IDs, or any binding that is
   not `READ` or does not match its declared handler/service key/source
   mapping.

Structural loading through `Load` accepts a v2 artifact with additional
declared sources or generic bindings so that tests and future reviewed
artifacts can be validated before they become the shipped inventory; the
embedded artifact is additionally held to the frozen inventory by `LoadV2`.

## 14. Digest definition

`Catalog.Digest` is the SHA-256 of the **exact artifact bytes** passed to
`Load`/`LoadV1`/`LoadV2`, including the final LF, formatted as
`sha256:` followed by lowercase hexadecimal. It is never computed over a
re-serialized object, so YAML formatting changes are visible identity changes.

Shipped digests:

| Artifact | SHA-256 |
|---|---|
| v1 | `cbb8de356531c1f5e43c558a59eaa4bec94bd3d074505307ed99fc09fd42ac57` |
| v2 | `b2de218a63a596de0b948617c8e864c8e6e4bf8bc8758e6652cb5d0e7f4bfd50` |

The Provider process reports the loaded catalog digest and version from
`Catalog.Digest` and `Catalog.Version`; it does not recompute or normalize them.
Changing an artifact without updating its digest expectation is a test failure.

## 15. Evidence status

- v1 catalog + loader: implemented and tested; the v1 artifact is frozen.
- v2 catalog + loader: implemented and tested in this branch.
- Generic SBI binding shape: loader-supported and fixture-tested only; no
  production generic binding is shipped and **no real standardized-SBI
  end-to-end acceptance is claimed** by this document.
- The three shipped v2 list operations and the eight context-query operations
  remain the source-verified OpenAMSM read operations; their live acceptance is
  recorded separately in `docs/evidence/` and is not restated here.

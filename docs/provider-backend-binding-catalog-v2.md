# Provider Backend Binding Catalog

Implemented. The sole Catalog is the strict, trusted `ca-client-tools.provider-bindings/v2` artifact. See [WORK](WORK.md) for the verified state. The Catalog maps private backend IDs and supplies no Host/Runtime authorization.

## 1. Purpose

The catalog is the single trusted mapping from a private backend operation ID to
one executor profile and its reviewed metadata:

- the typed handler profile that executes the operation;
- the trusted service key used to resolve the destination in deployment
  configuration;
- the read/mutation effect classification;
- the pinned evidence source (OpenAPI document or OpenAMSM source file);
- the typed argument profile, or the full generic SBI wire contract.

## 2. Artifact and embedding

Use `contracts/provider-bindings/v2/provider-bindings.yaml` as the single source. Embed and digest its exact bytes. P0 removed the old v1 artifact and its loader and eliminated version dispatch; `catalog.Load` is the only production load path and `catalog.Parse` exists for tests and the acceptance-only fixture build. Validate structure, source pins, required read-tool coverage, service keys, handler profiles and effect before publication. Keep the loaded catalog immutable. A reviewed edit replaces the single artifact and updates digest expectations.

An acceptance-only build (`-tags providerfixture`) may append test-only genericSBI bindings to the loaded catalog for process-level acceptance. That code is compiled out of every production build and is never part of a deployed Provider; the production catalog digest still covers exactly the embedded v2 bytes.

## 3. Root shape

The artifact is a single YAML document with exactly this root shape:

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
  Keep the three baseline pins in §5 and review any additions.
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

Each pin change requires reviewed source evidence and updated tests.

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
- `effect` is `READ` or `MUTATION`. The deployed Catalog contains only `READ` bindings while the production Mutation gate is closed.
- `handler` selects exactly one profile from §8.
- `serviceKey` is a logical deployment configuration key, not an origin. The
  catalog never carries URLs, origins or credentials.
- `source.operationId` is required for every binding.
- `response.schemaRef`, when present, must name a declared schema. Typed
  handler bindings require it.
- `request.mediaType` is required and must be exactly `application/json` for
  every binding. Actual SBI request bodies/media types beyond this are declared
  only inside a `genericSBI` binding.
- `genericSBI` bindings declare the closed nested `generic.argumentSchema` and
  must not declare the scalar `arguments` list; typed bindings declare
  `arguments` and must not declare a `generic` object.
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

Retain these read profiles for behavioral regression coverage.

## 10. genericSBI binding shape

Illustrative fixture only. A deployed `genericSBI` binding requires source evidence for its complete trusted wire contract:

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
    argumentSchema: { type: object, ... }
    method: POST
    path: /nsmf-pdusession/v1/sm-contexts/{smContextRef}/search
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
~~~

Validation rules:

- `argumentSchema` is required, is validated with the closed schema keyword set,
  and must be a closed object (`type: object`, `additionalProperties: false`)
  whose top-level properties are exactly the declared path/query parameter
  names, `body`, `payloads` for multipart requests, and the idempotency key
  argument when declared. Path/query property types must match the declared
  parameter types. The Provider validates every invocation against this schema
  recursively before any network I/O.
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
- A `genericSBI` binding whose declared response is a single binary body or
  `multipart/related` is artifact-bearing: it publishes the closed
  `artifactSlots` descriptor extension and is available exactly when the Host
  configured an artifact channel for the process (otherwise `NOT_CONFIGURED`,
  rejected before any remote I/O). The P1 Provider projects the response through
  the pinned Host Artifact Store contract. A `genericSBI` binding with a
  non-JSON response shape that is not artifact-bearing remains
  `NOT_IMPLEMENTED`. The catalog shape remains valid and unchanged; enabling an
  artifact-bearing binding requires the published Host contract, not a catalog
  change.
- Runtime owns callable admission and Capability authorization. A catalog
  entry, protocol method or CLI command never grants permission.
- Sensitive operations (for example AMF metrics) remain Host-private unless a
  separate contract authorizes a safe projection.
- The catalog carries no public operation IDs, capability IDs, scope, owner
  outcome or evidence semantics.

## 12. Catalog change rules

Review schema, source and operation changes together with the single protocol and implementation. The demo carries no backward-compatibility obligation. Declare one closed nested argument schema under `generic.argumentSchema` for each generic binding, using the bounded schema vocabulary above. The argument root is an object. Close declared objects with `additionalProperties: false` wherever members are enumerated. Keep path and query values at their declared top-level names, structured request content under `body`, and named multipart inputs under `payloads`; reserve the exact P1 reference value shape for the Host contract. Check declaration consistency with path/query parameters, request body, attachment slots and any effect-specific metadata. Publish the safe argument schema in the descriptor while keeping private wire bindings in the artifact. Typed profiles retain their explicit argument declarations.

## 13. Validation rules summary

The loader fails closed and returns an error for:

1. invalid UTF-8, empty documents, non-mapping roots, more than one YAML
   document;
2. YAML anchors, aliases and merge keys (`<<`), non-string mapping keys, and
   duplicate mapping keys at any depth;
3. unknown fields at any struct level (strict typed decoding) and unknown
   schema keywords;
4. missing or extra root members, wrong `apiVersion`/`kind`/`providerId`;
5. wrong `catalogVersion`, malformed or unresolved source pins, empty
   schema or binding sets, unknown handler/effect, handler-profile violations,
   invalid or unresolved schema references, unknown arguments fields,
   malformed bounds/patterns/defaults, and generic binding violations
   (including a missing, non-object or non-closed `argumentSchema`, a
   path/query/body/idempotency mismatch, or a scalar `arguments` list on a
   generic binding);
6. shipped inventory violations, including a missing required read binding, an unresolved source pin or a production mutation while the gate is closed.

## 14. Digest and evidence

Report SHA-256 of the exact embedded bytes as `sha256:` plus lowercase hexadecimal, including the final newline in the input. The Host compares this with trusted deployment metadata. The acceptance-only `providerfixture` build appends its test bindings after loading and does not change the reported production artifact digest. Loader/executor fixtures exercise generic bindings. Provider-dispatched generic operation acceptance is a P0 deliverable and is recorded in the current dated evidence. The eleven OpenAMSM read bindings have historical live results in [evidence](evidence/provider-acceptance-20261117.md).

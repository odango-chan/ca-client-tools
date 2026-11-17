# Provider Backend Binding Catalog v1

**Status:** FROZEN FOR IMPLEMENTATION — first private SMF Observation slice only  
**Artifact:** `contracts/provider-bindings/v1/provider-bindings.yaml`  
**Provider:** `ca-client-tools.sbi`

## 1. Purpose and authority

This document defines the exact structure and validation rules for the first private Provider Backend Binding Catalog artifact.

The catalog is trusted package content embedded in the Provider build. It maps a private backend operation ID to one fixed typed Go handler and a pinned private source operation. It is not:

- the 3GPP OpenAPI Operation Catalog;
- the Host-owned ToolCatalog;
- a CapabilityContracts document or authorization grant;
- a Skill-authored Operation Manifest;
- a runtime URL, HTTP method, route or credential registry.

The Host owns the mapping from an admitted public callable identity `(owner, kind, id)` to one exact private backend operation ID. The Provider sees only private backend IDs.

## 2. Exact v1 document shape

The root object has exactly these members:

| Member | Required | Rule |
|---|---:|---|
| `apiVersion` | yes | exact `ca-client-tools.provider-bindings/v1` |
| `kind` | yes | exact `ProviderBackendBindingCatalog` |
| `metadata` | yes | exact object defined below |
| `spec` | yes | exact object containing `bindings` |

No unknown members are permitted at any level. Duplicate YAML mapping keys, invalid UTF-8, YAML aliases/merge keys that obscure the effective value, multiple YAML documents, non-object roots and empty documents MUST be rejected.

### 2.1 metadata

`metadata` has exactly:

| Member | Rule |
|---|---|
| `providerId` | exact `ca-client-tools.sbi` |
| `source` | exact source object below |

`metadata.source` has exactly:

| Member | Rule |
|---|---|
| `repository` | exact `odango-chan/env-openamsm` |
| `commit` | full 40-character lowercase hexadecimal Git commit ID |
| `openApiPath` | repository-relative path; no absolute path, backslash, empty segment, `.` or `..` |
| `openApiGitBlobSha` | 40-character lowercase hexadecimal Git blob object ID |

**Digest distinction:** `openApiGitBlobSha` is the Git blob object ID returned by the pinned repository tree. It is not a SHA-256 digest. The current source file's Git blob ID is `46045c2f8f1cae31c6e6c98d5d20addbebe41969`. The separate Provider `backendBindingCatalogDigest` is SHA-256 over the exact UTF-8 bytes of the embedded YAML file, including the final LF.

### 2.2 spec.bindings

`spec` has exactly one member, `bindings`. It is an array containing exactly eight entries, one for each first-slice private SMF projection.

Each binding has exactly:

| Member | Rule |
|---|---|
| `backendOperationId` | exact private ID from §3 |
| `handler` | exact `openamsmSMFContextQuery` |
| `source` | exact object below |

Each `source` object has exactly:

- `serviceKey`: exact `openamsm-smf-context`;
- `contextType`: one of `SC`, `PA`, `UPN`, `SSM`, `IAM`, `UPS`, `CA`, `SAA`;
- `operationId`: exact source OpenAPI operationId listed in §3.

No binding may add a URL, origin, HTTP method, path template, raw header, token, secret, retry policy, public callable ID, Capability ID, Effect, Tool Schema, Owner Outcome or Evidence guarantee.

## 3. Exact first-slice bindings

| Private backend operation ID | contextType | Source OpenAPI operationId |
|---|---|---|
| `openamsm.smf.mas.sc.query` | `SC` | `queryScContext` |
| `openamsm.smf.mas.pa.query` | `PA` | `queryPaContext` |
| `openamsm.smf.mas.upn.query` | `UPN` | `queryUpnContext` |
| `openamsm.smf.mas.ssm.query` | `SSM` | `querySsmContext` |
| `openamsm.smf.mas.iam.query` | `IAM` | `queryIamContext` |
| `openamsm.smf.mas.ups.query` | `UPS` | `queryUpsContext` |
| `openamsm.smf.mas.ca.query` | `CA` | `queryCaContext` |
| `openamsm.smf.mas.saa.query` | `SAA` | `querySaaContext` |

The mapping is one-to-one. Duplicate backend IDs, duplicate context types, duplicate source operationIds, missing entries, additional entries or a mismatch between the ID suffix, context type and operationId MUST fail validation.

The handler is deliberately shared. The fixed context type selects the existing typed client's MAS query parameter. It is not an Agent/Skill-supplied selector.

## 4. Source verification

Build-time/source acceptance MUST verify against the pinned `env-openamsm` repository:

1. the exact commit exists;
2. `openApiPath` exists at that commit;
3. its Git blob object ID equals `openApiGitBlobSha`;
4. all eight operationIds exist in that exact OpenAPI file;
5. the typed Go client and source mapping preserve the exact operation and response semantics.

The resident Provider does not fetch GitHub, resolve a floating branch, or update this catalog at runtime. It embeds the reviewed artifact. The Host independently verifies the executable/package digest and the Provider-reported `backendBindingCatalogDigest` against trusted deployment metadata.

## 5. Loader requirements

The Provider loader MUST:

- decode the embedded file with a maintained YAML parser and strict typed decoding;
- reject unknown fields, duplicate keys, aliases/merge keys, multiple documents and malformed types;
- validate every fixed value and the exact eight-entry mapping in §§2–3;
- calculate the catalog SHA-256 over the exact embedded bytes, not a re-serialized YAML object;
- expose only the exact private backend IDs to the Provider `catalog` method;
- fail initialization with `PROVIDER_BINDING_CATALOG_INVALID` on any mismatch.

The loader does not interpret public Callable IDs or Capability membership. It does not select an operation based on an Agent argument. The Host selects the exact backend ID only after ToolCatalog, CapabilityContracts, Scope and Runtime admission.

## 6. Versioning and extension

This v1 artifact is limited to the eight private OpenAMSM SMF context-query handlers. It is not the general standardized 3GPP Operation Catalog and does not authorize generic HTTP execution or mutation.

A future standardized SBI operation must be added only through a separately reviewed binding/catalog version or explicitly versioned extension that references a pinned 3GPP OpenAPI source and defines the typed request/response/payload binding. Do not silently add arbitrary fields or generalized route overrides to this v1 file.

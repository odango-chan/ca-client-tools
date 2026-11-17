# SMF Context Observation Contract v1

**Status:** OWNER CONTRACT CANDIDATE — FROZEN FOR IMPLEMENTATION  
**Implementation status:** Typed CLI client exists; Provider ToolCatalog export and Host dispatch are NOT IMPLEMENTED / NOT QUALIFIED.  
**CA-SL profile:** `connectionagent.io/v1alpha4` (FROZEN)

## 1. Ownership and public boundary

The eight callable contracts are published by the `ca-client-tools` Tool Owner. That Owner owns the CA-facing callable identities, normalized Schemas, validation and Provider implementation. The authoritative source of returned network state is the SMF (`smf-owner`).

- ToolCatalog `metadata.owner`: `ca-client-tools`
- Evidence Descriptor `authority`: `smf-owner`

The backing query routes are private OpenAMSM interfaces, not standardized 3GPP SBI services. Private OpenAMSM MAS codes and route identifiers are Provider implementation details only. Skill source, Capability IDs and public callable IDs MUST NOT expose a MAS selector.

The public callable identity selects the context type. The caller does not supply `masId`, route path, HTTP method, URL, service key or header.

## 2. Eight Observation callables

All eight exports have `kind: observation`, `effect: READ`, and `ownerOutcomes: [SUCCEEDED, FAILED]`.

| Public context type | Callable ID | Private source type / operation | Private route |
|---|---|---|---|
| `session` | `smf.context.session.query` | `SC` / `queryScContext` | `POST /nsmf-sc-context/v1/query` |
| `policy` | `smf.context.policy.query` | `PA` / `queryPaContext` | `POST /nsmf-pa-context/v1/query` |
| `up-node` | `smf.context.up-node.query` | `UPN` / `queryUpnContext` | `POST /nsmf-upn-context/v1/query` |
| `signaling-session` | `smf.context.signaling-session.query` | `SSM` / `querySsmContext` | `POST /nsmf-ssm-context/v1/query` |
| `ip-address` | `smf.context.ip-address.query` | `IAM` / `queryIamContext` | `POST /nsmf-iam-context/v1/query` |
| `up-selection` | `smf.context.up-selection.query` | `UPS` / `queryUpsContext` | `POST /nsmf-ups-context/v1/query` |
| `charging` | `smf.context.charging.query` | `CA` / `queryCaContext` | `POST /nsmf-ca-context/v1/query` |
| `session-auth` | `smf.context.session-auth.query` | `SAA` / `querySaaContext` | `POST /nsmf-saa-context/v1/query` |

The public result exposes `type`, `status`, `key` and `context`. The Provider validates the private response `masId` against the selected callable and removes it from the public result.

## 3. QueryContext v1

All eight exports use one closed QueryContext Schema: `urn:connectionagent:ca-client-tools:smf-context-query:v1`. It requires `supi` (non-empty string) and `pduSessionId` (integer 0–255), with no additional properties.

This Agent-callable contract intentionally supports only trusted Subject SUPI plus PDU Session ID. CLI v1 additionally supports IMSI and IPv4/IPv6 locating keys; those forms are not implicitly exposed to Skills. A future Agent-facing extension must separately define identity provenance and Scope before widening this Schema.

The Provider validates input using the existing typed client before network dispatch. The returned `key` MUST contain exactly SUPI and PDU Session ID and match the request exactly. Missing, extra, conflicting or mismatched key fields are `REMOTE_RESPONSE_INVALID`, never `UNAVAILABLE`.

## 4. Normalized Output contract

Each callable has a separate closed Output Schema under `contracts/smf-context/v1alpha4/schemas/`. The root requires `type`, `status`, `key` and `context`.

- `type` is a per-callable constant.
- `status` is exactly `AVAILABLE` or `UNAVAILABLE`.
- `key` is the exact echoed SUPI and PDU Session ID.
- `context` is a closed object containing only fields defined for that context type.

The nine machine-readable Schemas (one QueryContext and eight Output Schemas) comply with CA-SL v1alpha4 G.4: closed objects, no `$defs`, `oneOf`, `anyOf`, `pattern`, `format` or dynamic properties.

G.4 cannot express the conditional relationship between status and context. The Provider adapter MUST therefore enforce:

| Context type | Required when `AVAILABLE` | `UNAVAILABLE` |
|---|---|---|
| `session` | `lifecycle`, `dnn`, `sst`, `sd`, `subscriptionValidated`, `secondaryAuthRequired`, `uecmRegistered` | `context: {}` |
| `policy` | `bound`, `dnn`, `sst`, `sd` | `context: {}` |
| `up-node` | `estDone`, `n3Synced`, `dnn`, `sst`, `sd` | `context: {}` |
| `signaling-session` | `fteidReady` | `context: {}` |
| `ip-address` | `allocated`, `allocator` | `context: {}` |
| `up-selection` | `decided` | `context: {}` |
| `charging` | `bound` | `context: {}` |
| `session-auth` | `required`, `eapRound` | `context: {}` |

For AVAILABLE, all required fields MUST exist and satisfy their Schema; optional fields may be omitted exactly as the source projection omits empty values. Unknown fields are rejected. For UNAVAILABLE, context MUST be an empty object. The adapter MUST NOT fabricate defaults.

## 5. Context field inventory

| Context type | Fields |
|---|---|
| `session` | `lifecycle`, `dnn`, `sst`, `sd`, `amfRef?`, `subscriptionValidated`, `secondaryAuthRequired`, `uecmRegistered`, `subSummary?` |
| `policy` | `bound`, `policyAssocId?`, `upIntent?`, `authorizedQos?`, `dnn`, `sst`, `sd`, `pduSessionType?` |
| `up-node` | `n4Seid?`, `estDone`, `n3Synced`, `dnn`, `sst`, `sd`, `pduSessionType?` |
| `signaling-session` | `fteidReady`, `anTunnel?`, `ranTeid?`, `anTunnelIp?`, `lastSignal?` |
| `ip-address` | `allocated`, `allocator`, `ueAddr?` |
| `up-selection` | `decided`, `upAnchor?` |
| `charging` | `bound`, `chargingRef?` |
| `session-auth` | `required`, `phase?`, `eapRound` |

`?` means the source may omit the field when empty or not applicable. The schemas retain exact JSON types and known closed enumerations.

## 6. Outcome and status semantics

| Source result | Provider result | CA-SL result |
|---|---|---|
| HTTP 200, valid echo, AVAILABLE and valid typed context | `ok=true`, `ownerOutcome=SUCCEEDED`, output | Observation SUCCEEDED |
| HTTP 200, valid echo, UNAVAILABLE and empty context | `ok=true`, `ownerOutcome=SUCCEEDED`, output | Observation SUCCEEDED; business context unavailable |
| Valid HTTP ProblemDetails | `ok=true`, `ownerOutcome=FAILED`, safe cause/status | Observation FAILED |
| Transport timeout / connection failure | `ok=false`, `REMOTE_TRANSPORT_ERROR` | Runtime UNKNOWN |
| Malformed response, wrong private type, mismatched key or invalid context | `ok=false`, `REMOTE_RESPONSE_INVALID` | Runtime UNKNOWN |
| Host authorization failure | No Provider request | Runtime DENIED |

UNAVAILABLE means only that the queried SMF projection is unavailable/not written for that lookup. It does not mean the PDU Session does not exist. It is not `KNOWN_ABSENT`, `NOT_FOUND`, `FAILED` or proof of a negative business fact.

HTTP 404 with `SESSION_NOT_FOUND` is Owner FAILED, not UNAVAILABLE. HTTP 403 `FORBIDDEN` is a remote Owner failure, not Host Runtime DENIED (reserved for local admission/authorization).

The Provider MUST NOT retry automatically. Host/Runtime owns deadlines, cancellation and routes.

## 7. Evidence guarantee

Each ToolCatalog export provides one Evidence guarantee at `path: output`. The separately published `evidence-guarantees.yaml` declares:

- authority: `smf-owner`;
- producer: exact `ca-client-tools` Observation callable with SUCCEEDED Outcome;
- scope: Subject SUPI and PDU Session ID, checked against QueryContext;
- correlation: Host Invocation ID and producer invocation ID;
- fact value: normalized query result, including status;
- selection: `exact`.

There is deliberately no `freshness` declaration. The pinned OpenAMSM response has no Owner `observedAt`; Host receipt time MUST NOT be relabelled as source observation time.

There is also no Lifecycle identity in the query request or returned key. This Evidence establishes that the trusted SMF query adapter returned this result for the SUPI/PSI pair and Invocation; it cannot establish which Host Lifecycle generation the SMF projection belongs to. Skills MUST NOT use it as Lifecycle proof or freshness-sensitive evidence.

For UNAVAILABLE, Evidence proves the query result and its scope, not existence or absence of the underlying resource. A Predicate requiring an available context must return UNKNOWN unless another independent adequate source proves the fact.

## 8. Capability and Host admission

The deployment may publish Capability ID `ca.smf.context.read` with eight exact members. Each member has `owner: ca-client-tools`, `kind: observation`, one exact callable ID, `effect: READ` and `scopePolicyRef: deployment.smf-context-read`.

The Skill grants only `ca.smf.context.read`; its Procedure references the individual `ca::smf.context.<type>.query` callable through its dependency Alias. The Alias `ca` resolves to the immutable package snapshot exporting ToolCatalog Owner `ca-client-tools`; the Alias is not the Owner ID or Capability ID. Host verifies ToolCatalog, Capability membership, Scope, Provider support and trusted deployment configuration. Provider support never grants authorization.

## 9. Provider binding and source reuse

The Go Provider lives in `ca-client-tools` and calls `internal/openamsm/smf/mas.Client.Query` directly. It MUST NOT invoke the CLI as a subprocess or copy/reimplement the HTTP/2 client, query validation, echoed-key binding or ProblemDetails handling.

The immutable Provider Callable Binding Catalog is `contracts/provider-bindings/v1/provider-bindings.yaml`. It maps each public callable identity to the fixed typed handler and private context type, and pins the source commit/OpenAPI digest. It is embedded in the Provider build; the process protocol reports the SHA-256 of its exact packaged bytes. The existing typed client remains the sole owner of HTTP method, route path, OpenAPI operationId and h2c transport. Those details are not copied into the Provider binding catalog. Callable identity selects the private context type; no caller-provided selector is accepted.

The Host ↔ Provider boundary is [Provider Process Protocol v1](provider-protocol-v1.md). The first slice is JSON-only and does not depend on HostArtifactRef or binary/multipart features.

## 10. Source pin and qualification boundary

- Repository: `odango-chan/env-openamsm`
- Commit: `594dec78b95bff4fdc34deac8a3a9681fb41228f`
- OpenAPI: `upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml`
- OpenAPI blob: `46045c2f8f1cae31c6e6c98d5d20addbebe41969`

The eight projection implementations are under `upstream/openamsm/src/smf/mas/{sc,pa,upn,ssm,iam,ups,ca,saa}/`; shared resolution and echo behavior are under `upstream/openamsm/src/smf/mas/common/`.

The existing CLI typed client and real OpenAMSM acceptance establish client-side behavior only. Recorded live query acceptance used an operator-authorized local OpenAMSM build overlay for lowercase query-token lookup; that overlay is not in the pinned upstream tree. The selected deployment MUST include the required token-header behavior. No Host Provider process or Host/Runtime E2E is claimed.

## 11. Acceptance gates

1. All nine Schemas pass the CA-SL v1alpha4 G.4 profile.
2. ToolCatalog has exactly eight Observation exports with these identities, READ Effect, Owner Outcomes, Schemas and Evidence guarantees.
3. Provider binding maps each identity to one fixed private route/type and rejects unknown/altered identities.
4. Query input is SUPI + PSI only; no MAS selector or arbitrary route data is accepted.
5. Output validation checks private type, exact echoed key, closed context fields and status-specific requirements.
6. AVAILABLE/UNAVAILABLE, valid ProblemDetails and transport/protocol errors remain distinct.
7. Evidence scope/correlation are validated; no freshness or Lifecycle guarantee is invented.
8. A real query succeeds through the resident Provider and actual Host/Runtime dispatch path.

CLI acceptance and Provider-local tests are not Host-dispatch E2E acceptance.

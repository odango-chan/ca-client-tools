# ca-provider-5gc Capability Migration Map

**Status:** DESIGN TARGET — no migration or retirement is claimed complete.  
**Scope:** Decompose the existing `ca-provider-5gc` implementation into the long-term CA Tool architecture.  
**Primary target:** `ca-client-tools` for typed NF client callables and operation-specific protocol codecs; CA-Agent Host for binary artifact storage; `env-fabric` for durable network-mutation transactions; CA Runtime for invocation governance and continuation.  
**Language boundary:** CA-SL `connectionagent.io/v1alpha4` remains frozen. No new Skill syntax or compatibility mode is introduced.

## 1. Decision

`ca-provider-5gc` is a migration source and transitional implementation, not a permanent parallel 5GC Tool plane.

The migration is intentionally broader than moving read-only query clients. It includes reusable NF client operations, request/response adapters, operation-specific NAS/NGAP codecs and message construction, and the callable surfaces currently exposed by the Provider. It does **not** mean copying the entire Provider service, its local state machines or its transaction journal into `ca-client-tools`.

From this decision onward, new client-side NF callables and reusable protocol codecs belong in `ca-client-tools`. `ca-provider-5gc` should receive only fixes required to keep the migration bridge safe; it should not become the home for new parallel Tool capabilities. Its Provider API and service are retired after the migration gates pass. The `ca-client-tools` package layout should use NF/protocol names (for example AMF, SMF, NAS and NGAP), not preserve a permanent `fivegc` namespace.

The target is to make each supported network action an exact typed Tool callable, backed by the appropriate NF client and codec, while keeping authorization, artifacts, lifecycle and transaction authority in their designated components.

## 1.1 First delivery: one end-to-end SMF context-query slice

This combines the earlier **Goal 1 (independent client Tool package)** and **Goal 4 (install it in ca-agent-demo and complete real Host-dispatch acceptance)**. They are one delivery and one acceptance gate, not two separately completable milestones.

The first vertical slice is eight private SMF context-query backend operations exposed through nine Host-owned public Observation callables: the generic SC-backed session read plus eight optional MAS projections. The generic session read and MAS-specific SC projection share one private SC backend operation.

1. Move/reuse the source-verified h2c client, stable private context types, request/response validation and error mapping in `ca-client-tools`; publish only the eight private Provider backend operation bindings there. Public callable identities, ToolCatalog exports, normalized Output Schemas, Owner Outcomes and Evidence Producer belong to `ca-agent-host`, not `ca-client-tools`. Preserve the eight projection types (`session`, `policy`, `up-node`, `signaling-session`, `ip-address`, `up-selection`, `charging`, `session-auth`). Do not expose private MAS wire codes as Skill inputs.
2. The Host adapter maps those exact admitted callable identities to private
   backend operation IDs supported by the installed `ca-client-tools.sbi`
   Provider. The Provider Backend Binding Catalog contains backend operation
   IDs and fixed typed-handler bindings only; it does not contain public
   ToolCatalog callable identities. The Provider implementation is
   deterministic and supports only those exact registered backend operations.
   The Host/Provider boundary is a persistent, language-neutral NDJSON process
   protocol; inside the Go Provider, typed clients and codecs are called through
   direct Go package APIs. The CLI remains an operator/test interface and is
   not the dispatch path.
3. Implement the narrow RuntimeHost.v2 HostBusinessCorrelation envelope in `ca-runtime` (preserving v1 and excluding Temporal/pending-Operation continuation). Install/select the `ca-client-tools` Provider in `ca-agent-demo`; Host starts one long-lived `ca-client-tools provider serve` process per configured Provider instance and connects its bounded NDJSON `catalog`/`invoke`/`cancel` protocol to the existing Host/Runtime dispatch path. Do not spawn a new process or invoke a CLI command per Tool call. Host resolves public ToolCatalog and deployment CapabilityContracts, maps each admitted public callable to one private backendOperationId, exposes only the active Skill's exact callable set, and Runtime enforces Capability, Scope and Effect on every invocation. HostBusinessCorrelation is sent only to Runtime, never in Provider Protocol v1.
4. Exercise the real query through the actual Host/Runtime/Provider dispatch path against the selected OpenAMSM deployment using test-only, non-published callable/Evidence fixtures to exercise the Runtime stage plumbing. Verify the normalized public output, preserve `AVAILABLE` / `UNAVAILABLE`, validate source `masId` and echoed SUPI/PSI without exposing private wire fields, and retain diagnostic request/response evidence. Do not publish these test fixtures or claim a real Skill/Evidence qualification.
5. Prove the negative boundary: an ungranted callable, wrong Scope, unsupported context type or unregistered Provider binding is rejected before any NF request. No arbitrary URL, path, method, header or token is accepted from Skill/Agent input.

**Implementation completion rule:** CLI success, client unit tests, Provider-local invocation, or a mocked Host dispatch are intermediate evidence only. Implementation passes only when the real OpenAMSM query succeeds through the installed Provider and the actual Host/Runtime dispatch path, including positive/negative admission and Scope tests. Report this as `PROVIDER_HOST_DISPATCH_IMPLEMENTATION_PASS`.

**Separate product-qualification gate:** The current SMF v0.10 response has no source timestamp or accepted live-authoritative-read freshness guarantee. The four Host ToolCatalog/CapabilityContracts proposal files remain non-published; valid Evidence Descriptors cannot yet be published or linked. Real v1alpha4 Skill linking and Evidence-dependent Predicate PASS remain `NOT_QUALIFIED` until the SMF Owner freshness contract, trusted Host ToolCatalog and deployment CapabilityContracts are published. Never invent `observedAt`, `maxAgeMs` or production Evidence.

This first slice is JSON-only. It MUST reuse the existing h2c/query implementation and MUST NOT wait for NAS/NGAP codecs, multipart transfer or HostArtifactRef storage. Those are required for later binary Operation migration, not for the eight SMF context read-only queries backed by private OpenAMSM routes. Conversely, a direct CLI call MUST NOT be presented as proof that Host dispatch integration is complete.

The eight SMF context-query routes are private OpenAMSM interfaces, not standardized 3GPP SBI services. Preserve that distinction in the ToolCatalog and documentation. Do not add server patches or infer new endpoints as part of this migration. The Provider-local five-second `operation-context:read` projection is a different behavior and is not one of these eight NF queries; decompose it separately rather than copying its local store.

## 2. Target responsibility split

| Component | Target responsibility | Explicitly not responsible for |
|---|---|---|
| `ca-client-tools` | Typed NF Observation/Operation backends; pinned Provider Backend Binding Catalog; HTTP/2 transport; JSON/binary/multipart Payload Engine; operation-specific NAS/NGAP codecs and message builders; typed request/response validation | Skill routing; Capability grants; Host artifact persistence; Runtime Wait/Outcome/Evidence/Reconcile; Fabric transaction journal |
| CA-Agent Host | Canonical HostArtifactRef store; immutable byte ingestion; digest/media metadata; invocation/producer/consumer/scope binding; per-dispatch staging; process lifecycle and trusted Provider configuration | NAS/NGAP interpretation; NF state authority; business routing |
| CA Runtime | Exact callable admission; Capability/Scope/Effect checks; SourcePath and producer provenance; Invocation identity; Procedure progression; Wait/Ingress correlation; Outcome/Evidence/Reconcile orchestration | SBI wire construction; binary persistence; network-side transaction journal |
| `env-fabric` | Durable network mutation reservation/journal; hold/claim; single-sender enforcement; SCP routing; exact target-producer checks; governed dispatch and transaction evidence | Skill business flow; Capability authorization; generic NF client library; Host artifact storage |
| `ca-skills` | Business intent, Capability IDs, Procedure Nodes, typed arguments, predicates and routes | NF endpoints, codec logic, HTTP details, local file paths |
| NF / OpenAMSM | Authoritative network resource state and actual network-side operation result | CA Skill routing or Runtime authorization |

The Tool Provider may expose a callable that delegates a mutation to Fabric. The fact that the callable is hosted by `ca-client-tools` does not move Fabric's transaction authority into the client process.

## 3. Migration inventory

The following map is based on the current Provider implementation and its recorded CreatedSession call chain. A row marked implemented describes the source-side component only; it does not imply cross-process or live acceptance.

| Current `ca-provider-5gc` capability | Target home | Migration treatment |
|---|---|---|
| Provider-local `operation-context:read` (5-second CreatedSession projection) | Decompose into Host/Runtime context and source-backed Observation inputs | Do not copy this route as if it were a standardized NF client. Its current result combines Provider-local builder/UPF resource state and operation-derived metadata. Re-home invocation/lifecycle facts in Host/Runtime; obtain network facts from the owning NF's verified Observation callable; derive preparation-only values inside the typed prepare Tool where appropriate. Retire the Provider-local TTL store after parity. |
| Eight SMF context projections (private OpenAMSM query routes) and other verified NF context clients | `ca-client-tools` | Move the actual NF client, request/response Schema, decoding and error mapping into typed Observation callables. Preserve the private OpenAMSM interface boundary; expose only the SMF context queries permitted by the active Capability/Scope. |
| AMF / SMF / other verified NF read clients | `ca-client-tools` | Consolidate as exact typed callables when the endpoint is source-verified. Keep the eight SMF context queries available to Agent only under the relevant Capability/Scope; keep sensitive AMF projections Host/Tool-internal by default. Do not resurrect the removed UPF `/session-info` endpoint or CLI without a separate source-backed decision. |
| UDM / PCF dispatch request construction and response adapters | `ca-client-tools` callable + governed dispatch authority | Move typed client operations, request/response adapters and pinned protocol bindings into `ca-client-tools` only after the exact NF endpoint is source-verified in the selected OpenAMSM deployment or its explicit private contract. The existing Provider route alone is not proof that a direct client endpoint is available. Where the current path requires Fabric dispatch, the callable delegates through Fabric rather than calling the NF directly. Keep Effect, Scope, correlation and authoritative result contracts in trusted ToolCatalog/Owner contracts; until verification, UDM/PCF client onboarding remains DEFERRED. |
| `establishment-response:prepare` | `ca-client-tools` | Migrate as a typed preparation Operation. Its operation-specific builder uses the NAS/NGAP codec and emits binary output streams to Host; it does not persist a Provider-owned artifact. |
| `createdsessioncodec/nas_accept.go`, `n2_setup.go` and related decoders | `ca-client-tools` | Move reusable protocol encoding/decoding into versioned operation-specific codec packages. Preserve byte-level fixtures, malformed-input rejection, fuzz coverage and protocol-release pinning. These are internal Tool implementation libraries, not Skill Scripts or LLM work. |
| `ProtectedArtifactStore` and `csa:v1:` | CA-Agent Host | Replace with the canonical Host Artifact Store and HostArtifactRef. Host stores immutable bytes and binds producer, Invocation, permitted consumer, purpose, media type, digest, expiry and trusted UE/Session/Lifecycle context. The old store/reference remains only behind a temporary compatibility adapter. |
| `amf-n1n2:transfer` callable and request/response adapter | `ca-client-tools` | Migrate as a typed transfer callable that consumes HostArtifactRef through Host staging. Where the operation uses Fabric's governed transfer route, the callable delegates to Fabric; it must not send directly to AMF and bypass the transaction path. |
| `fabric_transfer_client.go` and Provider-to-Fabric client adapter | `ca-client-tools` integration adapter, with server authority remaining in `env-fabric` | Rehome the client-side typed adapter if it remains the callable boundary. Keep Fabric's server contract, reservation, journal, SCP routing, exact producer validation and transaction evidence in Fabric. |
| Provider-local C transfer journal / reservation | Decompose; do not port wholesale | Prepared-byte lineage moves into HostArtifactRef metadata; Invocation/operation identity remains in Host/Runtime responsibility records; durable network-mutation reservation and single-sender authority remain in Fabric. Retain no duplicate single-sender authority in the client. Remove the Provider-local journal only after equivalent cross-process behavior is demonstrated at the designated authority. |
| `c-transfer-intents` query and Provider intent projection consumed by Fabric | Replace with Host/Runtime-to-Fabric contract | Do not copy the Provider intent-query endpoint or its local intent database into ca-client-tools. Bind the prepared HostArtifactRef, original callable/Invocation lineage and Fabric hold/request digest through the trusted dispatch contract; Fabric validates its own durable journal and hold. Retire the Provider query after cross-process parity is proven. |
| PCF callback listener, callback token binding and receipt lifecycle | CA-Agent Host / Runtime, with typed payload contracts | Move ingress admission, Invocation correlation, lifecycle and continuation out of the long-lived NF client. Keep only protocol-specific parsing/validation in a reusable adapter where needed. Do not create a second Provider-local Wait/Ingress runtime. |
| `createdsession_n2_response_source.go` interface (currently not wired to a live source) | Future Host/Runtime Observation integration, backed by an authenticated Fabric source | Do not port an unimplemented interface as if it were a working client. First publish and verify the Fabric query contract and exact source identity/correlation semantics. |
| Provider HTTP routes and service lifecycle | Retire after migration | Keep a temporary compatibility façade only while old Skills/callers remain. Do not add new capabilities exclusively to the old Provider API. |

## 4. Canonical binary artifact flow

All CA-generated binary outputs use one Host-owned artifact path:

1. The Skill supplies typed business parameters and ordinary v1alpha4 SourcePaths. It does not construct binary bytes, paths, MIME boundaries or base64.
2. Runtime admits the exact callable and verifies Capability, Scope, Effect, SourcePath availability and producer provenance.
3. The Host invokes the trusted `ca-client-tools` Provider with a per-dispatch output staging area.
4. The operation-specific Tool implementation constructs the NAS/NGAP or other operation-defined bytes and streams them to the Host-created output area.
5. Host validates the complete output, computes the digest over original payload bytes, stores it immutably and issues HostArtifactRef with the applicable Invocation/producer/consumer/purpose/scope binding.
6. A later Tool receives the HostArtifactRef through a declared Input Schema. Host reauthorizes and stages the bytes for that exact dispatch.
7. The Tool performs the declared operation. If it is a network mutation governed by Fabric, the callable delegates through Fabric and preserves the original operation identity and transaction lineage.
8. Host removes temporary staging data. Durable Host artifacts remain until no pending dispatch or unresolved mutation responsibility requires them.

The Host does not decode NAS/NGAP. The generic Payload Engine does not invent protocol messages. The operation-specific Tool codec builds bytes; the Host stores and authorizes their use; the Payload Engine transports the declared parts.

The legacy `csa:v1:` value is not a new public Schema or a second artifact class. No new Skill or Tool should emit it. The compatibility adapter is removed after all consumers have migrated and the retirement gate passes.

## 5. Invariants that must survive migration

- **No unrestricted HTTP Tool.** Every operation is an exact registered callable mapped to a pinned OpenAPI/private API binding. Skill input cannot choose origin, method, path, headers, token, Content-ID or local path.
- **No hidden workflow Tool.** A callable represents one declared network action. Multi-step business routing remains in the Skill Procedure and Runtime.
- **No nested Agent.** The Provider is deterministic. It does not launch an LLM to encode a message or decide how to complete the same call.
- **No Script codec.** CA-SL Procedure Scripts remain pure structured-data computation. NAS/NGAP encoding and binary parsing live in the Tool implementation.
- **No artifact-store duplication.** CA-generated bytes are persisted by Host, not by each NF client or Provider.
- **No transaction bypass.** Fabric retains reservation, hold/claim, durable journal, single-sender, SCP routing and exact target-producer validation wherever the current mutation contract requires them.
- **No false success.** HTTP 2xx, a successful local codec or a Provider process exit does not by itself prove a final network Postcondition or Mutation COMMITTED.
- **No false evidence.** Fixture parity, source verification, local tests, cross-process tests and live NF acceptance remain separate evidence states.
- **No OpenAMSM internals in Skill source.** Skill references stable CA callable IDs and schemas, not private route paths, provider class names or implementation details.

## 6. Migration sequence and exit gates

| Phase | Work | Exit gate |
|---|---|---|
| 1. Inventory and freeze | Pin current Provider/Fabric/OpenAMSM source revisions; map each public route to exact Tool callable and authority owner | No unmapped public route or duplicated state owner |
| 2. Generic client foundation | Complete the minimum shared HTTP/2 Transport, eight private backend bindings and long-lived Provider process boundary (`provider serve` + bounded NDJSON `catalog`/`invoke`/`cancel`). Binary/multipart support is not a prerequisite for the first slice. | All eight private SMF backend operation IDs can be invoked through the resident Provider without arbitrary HTTP authority or per-call process launches |
| 3. **SMF context Observation vertical slice (merges former Goal 1 + Goal 4)** | Implement eight private query backends in ca-client-tools; implement RuntimeHost.v2 HostBusinessCorrelation in ca-runtime; implement nine public Host Observation callables and dispatch in ca-agent-demo; connect Capability/Scope admission and the resident Provider | Real diagnostic OpenAMSM query passes through actual Host/Runtime/Provider dispatch using test-only non-published callable/Evidence fixtures; public normalization and AVAILABLE/UNAVAILABLE semantics match; negative authorization/binding tests fail closed. Real Skill linking/Evidence qualification is a separate gate blocked by the SMF freshness contract. CLI-only or Provider-only PASS is insufficient |
| 4. Codec extraction | Move NAS/NGAP builders/decoders and their fixtures into ca-client-tools operation packages | Byte-level parity, protocol-release pinning, malformed input and fuzz tests pass |
| 5. Host artifact integration | Implement HostArtifactRef Schema/API, immutable store, output ingestion, input staging, quota/expiry and provenance checks | Cross-Invocation/UE/Session/consumer isolation and cancellation/partial-output tests pass |
| 6. Remaining callable migration | Migrate source-verified Operation/Observation callables beyond the first SMF context Observation slice; decompose the Provider-local operation-context projection into Host/Runtime context and verified Observation inputs; preserve ToolCatalog identity and schemas | Request/response parity; no duplicated context store or competing callable authority |
| 7. Fabric integration | Route governed mutations through Fabric and remove duplicate Provider transaction state | Cross-process reservation, journal recovery, single-sender and exact target producer tests pass |
| 8. Binary Skill/Host cutover and retirement | Replace `csa:v1:` with HostArtifactRef, migrate N1/N2 preparation/transfer and retire old Provider routes/store after all consumers move | Full Skill regression, real binary E2E and migration audit pass; no remaining legacy consumer |

A phase may be implemented independently, but the legacy path is not retired until all dependent callables, Skills and integrations have migrated and their evidence is recorded.

## 7. Explicit non-goals

This migration does not move the following into `ca-client-tools`:

- CA-SL parsing, Skill Procedure routing or Compiler behavior;
- Host Artifact Store persistence or Host policy;
- Runtime Capability/Scope admission, Wait/Ingress, Outcome, Evidence or Reconcile;
- Fabric's durable transaction journal, hold/claim, single-sender enforcement or SCP routing;
- authoritative NF resource state;
- arbitrary Agent-facing curl, shell execution or caller-selected HTTP destinations.

## 8. Source references

- `ca-provider-5gc/internal/providerapi/createdsession_prepare.go`
- `ca-provider-5gc/internal/createdsessioncodec/nas_accept.go`
- `ca-provider-5gc/internal/createdsessioncodec/n2_setup.go`
- `ca-provider-5gc/internal/createdsessioncodec/artifact_store.go`
- `ca-provider-5gc/internal/providerapi/createdsession_transfer.go`
- `ca-provider-5gc/internal/createdsessioncodec/fabric_transfer_client.go`
- `ca-provider-5gc/internal/providerapi/createdsession_c_journal.go`
- `ca-provider-5gc/internal/providerapi/createdsession_callbacks.go`
- `ca-provider-5gc/internal/providerapi/createdsession_n2_response_source.go`
- [CreatedSession Provider/Fabric audit (2026-10-29)](https://github.com/odango-chan/ca-provider-5gc)



## 9. First-slice Owner and backend contract

The public CA-SL Tool Owner is ca-agent-host. It publishes the generic
`context.smf.session.read` Observation and eight optional
`smf.mas.<type>.context.query` Observation callables, their normalized Schemas,
Owner Outcome mapping and Evidence Producer. Generic callable names omit
`mas` and implementation names such as `openamsm`; only explicitly
MAS-projection-specific public callable IDs include `mas`. `openamsm` is
reserved for private Provider backend operation IDs. See
[ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208).

ca-client-tools owns the Go typed client and private Provider backend
operations only. The Provider Backend Binding Catalog maps those internal
operation IDs to pinned private OpenAPI operationIds and typed Go handlers;
the typed client remains the executable owner of the HTTP routes. It is not a
ToolCatalog and does not grant Agent access. The generic `context.smf.session.read` callable and
MAS-specific `smf.mas.sc.context.query` callable intentionally share the
`openamsm.smf.mas.sc.query` backend operation; their Host adapters retain
separate output and normalization contracts. The other seven MAS callables
map to their corresponding private backend operations. The Host adapter maps
the admitted public callable to the backend operation after Runtime admission
and Scope checks.

The first Provider process slice is JSON-only and does not depend on
HostArtifactRef or binary/multipart support. The generic Host session read
uses the shared SC query operation and normalizes only exact 404
SESSION_NOT_FOUND to ABSENT; 200 UNAVAILABLE means the session exists but the
queried projection is not written. The `ca-provider-5gc` query path is a
migration source and is not retained as a parallel generic-read backend.

The v0.10 response has no source observedAt. Provider transport and Host query
integration can be implemented, but Evidence-dependent Predicate PASS remains
NOT_QUALIFIED until the SMF freshness gate in PR #208 is closed.

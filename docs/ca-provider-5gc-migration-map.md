# ca-provider-5gc Capability Migration Map

**Status:** DESIGN TARGET — no migration or retirement is claimed complete.  
**Scope:** Decompose the existing `ca-provider-5gc` implementation into the long-term CA Tool architecture.  
**Primary target:** `ca-client-tools` for typed NF client callables and operation-specific protocol codecs; CA-Agent Host for binary artifact storage; `env-fabric` for durable network-mutation transactions; CA Runtime for invocation governance and continuation.  
**Language boundary:** CA-SL `connectionagent.io/v1alpha4` remains frozen. No new Skill syntax or compatibility mode is introduced.

## 1. Decision

`ca-provider-5gc` is a migration source and transitional implementation, not a permanent parallel 5GC Tool plane.

The migration is intentionally broader than moving read-only query clients. It includes reusable NF client operations, request/response adapters, operation-specific NAS/NGAP codecs and message construction, and the callable surfaces currently exposed by the Provider. It does **not** mean copying the entire Provider service, its local state machines or its transaction journal into `ca-client-tools`.

The target is to make each supported network action an exact typed Tool callable, backed by the appropriate NF client and codec, while keeping authorization, artifacts, lifecycle and transaction authority in their designated components.

## 2. Target responsibility split

| Component | Target responsibility | Explicitly not responsible for |
|---|---|---|
| `ca-client-tools` | Typed NF Observation/Operation callables; pinned SBI Operation Binding Catalog; HTTP/2 transport; JSON/binary/multipart payload engine; operation-specific NAS/NGAP codecs and message builders; typed request/response validation | Skill routing; Capability grants; Host artifact persistence; Runtime Wait/Outcome/Evidence/Reconcile; Fabric transaction journal |
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
| `operation-context:read` and typed NF context clients | `ca-client-tools` | Move the client, request/response Schema, decoding and error mapping into typed Observation callables. Preserve the private OpenAMSM interface boundary; expose only the SMF context queries permitted by the active Capability/Scope. |
| AMF / SMF / other verified NF read clients | `ca-client-tools` | Consolidate as exact typed callables when the endpoint is source-verified. Keep the eight SMF context queries available to Agent only under the relevant Capability/Scope; keep sensitive AMF projections Host/Tool-internal by default. Do not resurrect the removed UPF `/session-info` endpoint or CLI without a separate source-backed decision. |
| UDM / PCF dispatch request construction and response adapters | `ca-client-tools` | Move the typed client operation and its protocol binding. Keep operation Effect, Scope, correlation and authoritative result contract in trusted ToolCatalog/Owner contracts. |
| `establishment-response:prepare` | `ca-client-tools` | Migrate as a typed preparation Operation. Its operation-specific builder uses the NAS/NGAP codec and emits binary output streams to Host; it does not persist a Provider-owned artifact. |
| `createdsessioncodec/nas_accept.go`, `n2_setup.go` and related decoders | `ca-client-tools` | Move reusable protocol encoding/decoding into versioned operation-specific codec packages. Preserve byte-level fixtures, malformed-input rejection, fuzz coverage and protocol-release pinning. These are internal Tool implementation libraries, not Skill Scripts or LLM work. |
| `ProtectedArtifactStore` and `csa:v1:` | CA-Agent Host | Replace with the canonical Host Artifact Store and HostArtifactRef. Host stores immutable bytes and binds producer, Invocation, permitted consumer, purpose, media type, digest, expiry and trusted UE/Session/Lifecycle context. The old store/reference remains only behind a temporary compatibility adapter. |
| `amf-n1n2:transfer` callable and request/response adapter | `ca-client-tools` | Migrate as a typed transfer callable that consumes HostArtifactRef through Host staging. Where the operation uses Fabric's governed transfer route, the callable delegates to Fabric; it must not send directly to AMF and bypass the transaction path. |
| `fabric_transfer_client.go` and Provider-to-Fabric client adapter | `ca-client-tools` integration adapter, with server authority remaining in `env-fabric` | Rehome the client-side typed adapter if it remains the callable boundary. Keep Fabric's server contract, reservation, journal, SCP routing, exact producer validation and transaction evidence in Fabric. |
| Provider-local C transfer journal / reservation | Decompose; do not port wholesale | Compare every invariant with Fabric's durable journal and Host/Runtime responsibility model. Retain no duplicate single-sender authority in the client. Remove the Provider-local journal only after equivalent cross-process behavior is demonstrated at the designated authority. |
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
| 2. Generic client foundation | Complete shared HTTP/2 Transport, Payload Engine, pinned SBI Operation Binding Catalog and Provider subprocess contract | JSON, single binary and multipart streaming tests pass; no arbitrary HTTP surface |
| 3. Codec extraction | Move NAS/NGAP builders/decoders and their fixtures into ca-client-tools operation packages | Byte-level parity, protocol-release pinning, malformed input and fuzz tests pass |
| 4. Host artifact integration | Implement HostArtifactRef Schema/API, immutable store, output ingestion, input staging, quota/expiry and provenance checks | Cross-Invocation/UE/Session/consumer isolation and cancellation/partial-output tests pass |
| 5. Callable migration | Move operation-context, prepare, transfer, UDM/PCF and other source-verified client callables; preserve ToolCatalog identity and schemas | Request/response parity; old and new callables are not simultaneously authoritative |
| 6. Fabric integration | Route governed mutations through Fabric and remove duplicate Provider transaction state | Cross-process reservation, journal recovery, single-sender and exact target producer tests pass |
| 7. Skill/Host cutover | Replace `csa:v1:` with HostArtifactRef and update pinned Tool contracts and Skill SourcePaths | Full Skill regression and real end-to-end acceptance pass |
| 8. Retirement | Remove old Provider routes, ProtectedArtifactStore, legacy reference support and duplicate adapters | No remaining consumer; migration audit and clean repository inventory pass |

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

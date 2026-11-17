# SBI Capability and Provider Interoperability Contract

**Status:** DESIGN DRAFT — for cross-repository review; no runtime implementation is claimed.  
**Applies to:** ca-client-tools, ca-skills and ca-agent-demo.  
**Normative boundary:** This document does not modify or supersede the frozen CA-SL connectionagent.io/v1alpha4 language contract or its v1alpha4 ToolCatalog / CapabilityContracts profile. Where this document and CA-SL differ, CA-SL and its versioned ToolCatalog / CapabilityContracts contracts prevail.

## 1. Purpose

Define how a Skill that grants one or more SBI-related Capabilities can use a trusted SBI client Provider without giving the Skill or Agent arbitrary HTTP access.

The intended result is:

- the Skill declares business-level Capability Grants and references exact callable exports;
- the deployment supplies trusted CapabilityContracts;
- the Tool Owner publishes callable contracts in ToolCatalog;
- ca-client-tools implements a bounded subset of those callables through its SBI Operation Binding Catalog, HTTP/2 transport and Payload Engine;
- the Host exposes only the exact callables used by the admitted Skill and permitted by deployment policy;
- Runtime remains responsible for per-call admission, authorization, Effect, Outcome, Evidence and Reconcile.

This is an interoperability design, not a claim that all 3GPP SBI operations are implemented by the current OpenAMSM source.

## 2. Critical identity distinction

The words Capability, callable, Tool and Provider implementation identify different things. They MUST NOT be collapsed.

| Identity | Authority | Meaning |
|---|---|---|
| Dependency Alias | Skill Contract / resolved dependency snapshot | Local name selecting one exact immutable dependency snapshot |
| Callable identity | ToolCatalog Owner | Exact exported callable: (owner, kind, id) |
| Capability ID | Deployment CapabilityContracts | Authorization group containing an explicit set of exact callables and Scope policies |
| Provider identity | Installed deployment package | Executable implementation package, such as ca-client-tools.sbi |
| Implementation binding | Trusted deployment registration | Selects which installed Provider implementation serves one exact callable |

### 2.1 Reading ca::sbi.udm.sm_data.get

Under the frozen CA-SL qualified-reference syntax:

- ca is the Dependency Alias;
- sbi.udm.sm_data.get is the exported callable ID;
- the full ca::sbi.udm.sm_data.get is a qualified callable reference;
- it is **not, by itself, a Capability ID**.

The Skill's capabilities list contains Capability IDs. Those IDs are resolved through the deployment's trusted CapabilityContracts. A Capability ID may use an sbi.* naming convention if the deployment explicitly publishes it, but the prefix has no automatic authorization meaning.

Do not infer Capability membership from string prefixes, names, Tool display names or package paths.

### 2.2 Skill authoring: Grant versus target

CA-SL v1alpha4 deliberately has two different references in Skill source:

- `assurance/contract.yaml` → `capabilities`: a list of **Capability IDs only**. This is the Skill's authorization request. A Skill does not grant itself a callable by listing its callable ID.
- `assurance/procedure.yaml` → `operation.tool`: a `QualifiedCallableId`; `observation.tool`: a `QualifiedQueryId`. These identify the exact callable a Procedure node intends to invoke. They are not Capability Grants.

The Compiler resolves each Procedure target through its declared Dependency Alias and the selected ToolCatalog snapshot, then matches the resulting exact `(owner, id, kind)` identity against trusted CapabilityContracts. The target is admissible only when at least one matching membership belongs to a Capability ID granted by the Skill; Effect consistency and applicable Scope checks also apply.

Therefore, the Skill's **authorization identity is the Capability ID**, while its Procedure's **execution reference is the qualified callable identity**. Do not put callable IDs in `contract.yaml.capabilities`, and do not reinterpret a v1alpha4 `operation.tool` / `observation.tool` reference as a Capability ID. The earlier authoring model that put a `capability:` field directly on each Procedure action is not the frozen v1alpha4 grammar.

### 2.3 Capability cardinality

CA-SL defines Capability as an authorization grouping boundary, not as an Operation alias.

- One Capability MAY contain multiple Operation and Observation callable identities.
- One exact callable MAY be a member of multiple Capabilities when the deployment contract explicitly declares those memberships.
- A Capability MUST enumerate exact members. Wildcards, prefix expansion and implicit inheritance are forbidden.
- A Skill Grant authorizes only the explicitly declared membership, subject to Scope and Runtime admission.
- The Host SHOULD expose only the exact ordinary callable identities referenced by the admitted Skill, not every callable in every granted Capability. Reconcile callables required by those Operations are resolved and bound for Runtime use but are not automatically exposed as separate Agent tools.

Therefore, the Provider does not implement a Capability as a single indivisible function. It implements exact callable interfaces that may be members of one or more Capabilities.

### 2.4 Who decides, who executes, and who handles binary payloads

The frozen CA-SL v1alpha4 execution model separates business intent from protocol execution:

| Role | Responsibility |
|---|---|
| Skill Contract | Declares Goal, Request / Context schemas, dependencies and Capability ID grants |
| Skill Procedure | Declares the business Node graph, exact `operation.tool` / `observation.tool` references, typed `arguments`, predicates and routes |
| Compiler | Links the source against exact dependency snapshots, ToolCatalog exports and trusted CapabilityContracts; checks argument paths, schemas, provenance and authorization membership |
| Host | Admits the Skill, supplies trusted Context, resolves deployment bindings/configuration, owns HostArtifactRef storage/lifecycle and per-dispatch staging, and enforces the common tool security boundary |
| Runtime | Executes the linked Procedure graph, materializes explicit arguments, validates artifact-reference provenance, applies per-call admission / Scope / Effect / budget / deadline checks, dispatches the exact callable through the Host Provider boundary, records outcomes and advances routes |
| SBI Tool Provider | Validates the exact registered callable and typed input, consumes Host-staged binary streams, constructs the wire request from trusted bindings, performs HTTP/2 and JSON / multipart / binary transport, and returns a structured result |
| Tool Owner / NF | Owns authoritative network state and the external operation result; publishes the business contract consumed by the typed Tool implementation |

The canonical HostArtifactRef and the migration off legacy `csa:v1:` references are specified in the [Host Artifact Contract v0alpha1](https://github.com/odango-chan/ca-agent-demo/pull/211). It is a design draft; no generic Host Artifact Store is claimed to be implemented.

Host and Runtime may be separate components or run in one process; their responsibilities do not collapse. The Runtime does not construct SBI paths or binary framing. The Provider does not choose the Skill's business next step, authorize a Capability or decide the final Skill outcome.

**LLM participation is not a second execution engine.** In a deterministic Skill execution, Runtime follows the compiled Procedure and dispatches its declared Tool Operations. In an LLM-assisted execution mode, the model may interpret the user's business request, supply values for declared business inputs, or request an admitted business Tool call where that mode permits it. The request still crosses the same Host / Runtime admission and dispatch boundary. The model never receives Provider process access, arbitrary HTTP authority, SBI credentials, or a separate privileged Reconcile callable. This same Host security and dispatch boundary applies to `RUNTIME_ONLY`, `LLM_RUNTIME` and `LLM_ONLY`. In `LLM_ONLY`, where there is no Skill Contract, authorization and Scope come from an independent trusted execution session; they are not inferred from a nonexistent `contract.capabilities`. A Provider invocation MUST NOT launch another LLM / Agent to decide how to perform the same call. The Provider is a deterministic protocol adapter, not an Agent.

**NAS / NGAP and other opaque binary payloads:**

1. The Skill supplies business-level values and explicit references to prior Tool outputs or admitted Request / Context fields, using the target callable's published Input Schema. It does not author HTTP headers, MIME boundaries, file paths, raw transport framing or arbitrary base64 blobs.
2. If the API needs an already-existing CA-generated binary payload, it is exposed as a HostArtifactRef under the Host Artifact Contract. New Tools do not introduce a second Provider-specific reference type for these bytes. A legacy `csa:v1:` reference may be used only by the exact temporary compatibility callable while migration is incomplete. The Skill binds the typed reference through an ordinary v1alpha4 SourcePath; CA-SL itself defines neither a filesystem path nor a generic BinaryRef primitive.
3. The Host resolves a HostArtifactRef, stages the bytes in a private per-dispatch directory and passes a Host-generated relative file identifier to the Provider. The Provider streams the bytes into the exact OpenAPI-declared multipart part. Accepted response attachments flow back through Host-created output staging; the Host ingests them and issues new HostArtifactRefs. The generic SBI Provider never resolves a legacy `csa:v1:` reference.
4. If a NAS / NGAP payload must be encoded from semantic fields, the operation-specific codec/message builder is part of the typed Tool implementation shipped in `ca-client-tools`. The generic HTTP/2 Payload Engine transports bytes and maps declared multipart parts; it does not decide business semantics. The current `ca-provider-5gc` NAS/NGAP codec is a migration source and should be moved into this Tool implementation rather than retained as a parallel Provider.
5. Skill-owned Procedure Scripts remain pure local structured-data transformations. They cannot access files, network or transport, and must not become an alternate NAS / NGAP encoder or SBI client. The LLM should receive only the structured fields or safe metadata needed for its business reasoning, not a base64 copy of the opaque payload.

This binary lifecycle is a Host / Artifact / Provider contract, not a new CA-SL v1alpha4 syntax feature. The exact Artifact reference schema and process framing must be frozen in their own versioned contracts before implementation.

## 3. Authority and ownership

| Component | Owns | Does not own |
|---|---|---|
| ca-skills | Skill Contract, Capability Grants, Procedure and exact callable references | Provider selection, URLs, HTTP method/path, credentials, deployment authorization |
| ToolCatalog Owner | Exact callable identity, kind, Input/Output or QueryContext schemas, Effect, Owner outcomes, Mutation/Reconcile and Evidence contract | Deployment grants, client transport implementation |
| Deployment CapabilityContracts | Capability IDs, exact callable membership, expected Effect and Scope policy references | Callable schemas, network paths, client implementation |
| ca-client-tools | SBI client implementation, pinned SBI Operation Binding Catalog, HTTP/2 transport, typed NF callables, operation-specific codecs/message builders, Payload Engine | Skill authorization, Capability membership, NF state authority, Host artifact persistence, Runtime Outcome/Evidence/Reconcile |
| ca-agent-demo Host | Trusted artifact loading, Provider installation/configuration and resident process lifecycle, active-Skill Tool exposure, trusted configuration, canonical HostArtifactRef lifecycle and staging | Re-defining ToolCatalog/Owner contracts or granting a Capability because a Provider reports support |
| CA Runtime | Callable admission, Capability and Scope authorization, Effect enforcement, Invocation identity, Outcome, Evidence and Reconcile | SBI path construction or arbitrary Provider discovery |
| Network Function / authoritative Owner | Actual network operation and authoritative state/result | Skill flow selection |

The ToolCatalog metadata.owner identifies the actual publisher/Owner of the callable contract. It may be a dedicated NF Tool Owner or an explicitly designated CA adapter Owner (potentially ca-client-tools) when that component genuinely publishes and maintains the callable contract. It MUST NOT be assigned to ca-client-tools solely because that repository supplies the transport implementation. In either case, ToolCatalog ownership does not make the publisher the authoritative NF, resource or fact Owner.

For standardized operations, the pinned 3GPP OpenAPI source is the authority for the standardized wire definition. The ToolCatalog Owner remains responsible for the published CA callable contract and any normalized output/Evidence promises. The serving NF remains the authority for actual network state.

## 4. Three separate trusted inputs

The implementation MUST preserve three independent contract authorities and one separate deployment implementation binding. No one file or repository may silently replace the authority of another.

### 4.1 ToolCatalog — callable contract

The Owner-published ToolCatalog defines the exact callable:

- kind: operation | observation;
- stable exported id;
- Owner identity;
- Effect and Owner outcome contract;
- Operation Input/Output Schema, or Observation QueryContext/Output Schema;
- applicable Mutation receipt, failure class, Reconcile and Evidence guarantees.

This follows CA-SL v1alpha4 §9 and Annex F. The ToolCatalog is the source for the Agent-facing input/output schema and Effect contract.

A normal HTTP GET is not automatically an Observation Query. It is an Observation only when the Owner publishes it as kind: observation with the required query context, authority, Scope, temporal, correlation and Evidence guarantees. Otherwise it remains an Operation with Effect READ.

### 4.2 CapabilityContracts — deployment authorization

The deployment-owned CapabilityContracts maps each Capability ID to exact callable members:

- owner;
- id;
- kind;
- effect;
- trusted scopePolicyRef.

The Skill cannot create or modify this mapping. The Provider cannot grant a Capability. The Host and Runtime MUST validate the trusted deployment artifact and apply the same authorization semantics required by CA-SL.

The Provider MUST NOT return a Capability list and ask the Host to trust it as authorization.

### 4.3 SBI Operation Binding Catalog — transport execution and Provider support

ca-client-tools owns one separate, versioned SBI Operation Binding Catalog. Each entry maps an exact ToolCatalog callable identity to a pinned wire operation. **The set of valid entries in this catalog is the Provider's support declaration; there is no second Provider-support manifest that repeats the same callable list.**

For a standardized SBI operation, the binding references:

- the exact 3GPP release and API document revision;
- the pinned OpenAPI source revision and digest;
- the exact OpenAPI operationId;
- a trusted deployment service key;
- any CA-specific multipart part mapping and HostArtifactRef field binding not already derivable from the OpenAPI definition.

The HTTP method, path template, parameters, media types, status codes and wire schemas MUST be derived from the pinned OpenAPI operation, not re-authored as an independent handwritten copy.

For a private OpenAMSM interface, the binding MUST identify a separately versioned private contract and MUST NOT label it as a standardized 3GPP operation.

The SBI Operation Binding Catalog MUST NOT redefine:

- Capability membership or grants;
- ToolCatalog Owner identity;
- Tool Input/Output schemas or Effect;
- deployment Scope policy;
- secret values or service origins.

Service origins, token profiles and resource limits come from trusted deployment configuration. The binding contains a service key, not an Agent-selectable URL.

## 5. Provider implementation selection

### 5.1 Provider support is the binding catalog

The installed Provider package ships the SBI Operation Binding Catalog as trusted, versioned package content. A callable is supported only when exactly one valid binding exists for its exact (owner, kind, id) identity and the pinned package digest is verified. A Provider cannot make an unknown callable valid by returning a self-asserted list.

The Provider `catalog` protocol method described in §7 returns a filtered support projection from this same catalog. It does not introduce a second manifest or duplicate schemas, Effect, Capability membership or Scope.

### 5.2 Static implementation selection; Skill-scoped exposure

Two different forms of selection must remain distinct:

1. **Deployment implementation selection:** At deployment startup, trusted configuration selects an installed Provider implementation for each exact callable. The selected binding is validated and frozen. No runtime auto-discovery, environment-name guessing or silent fallback is introduced.
2. **Active-Skill Tool exposure:** When a Skill is admitted, the Host exposes only the exact callable subset used by that Skill, after resolving its Capability Grants, ToolCatalog dependencies, SBI Operation Binding Catalog and deployment policy. This is dynamic Tool visibility, not dynamic loading or selection of executable code.

If more than one Provider can implement the same callable, deployment configuration MUST select exactly one before the Resident becomes READY. Missing, duplicate, ambiguous or unsupported bindings fail closed. A running Invocation MUST NOT switch implementations. An uncertain Mutation and its Reconcile MUST remain bound to the original implementation identity.

The existing ca-agent-demo R5.8 manual registration is a Demo-specific implementation-selection mechanism. It is not a new CA-SL language construct or the normative SBI Operation Binding Catalog format. Its current operation keys and historical use of the word “Capability” MUST NOT be copied as the cross-repository identity model without reconciliation against CA-SL's separate CapabilityContracts and callable identities. The Provider process adapter MUST plug into the existing Host Tool dispatch/registration path; it MUST NOT create a second Tool registry or move Runtime authorization into the Provider.

## 6. Host resolution and Tool registration

The Host performs the following deterministic resolution before exposing Agent tools:

1. Admit and freeze the Skill package and its dependency snapshots.
2. Verify the Skill's declared Capability Grants.
3. Load and verify the deployment CapabilityContracts artifact and its trusted deployment/publisher binding.
4. Resolve every exact callable referenced by the Skill's Procedure to one ToolCatalog export, including Owner, kind, Effect and schemas. Also resolve each declared Reconcile callable from its Mutation contract; it is a separately authorized callable even though it is not a normal Procedure target.
5. Prove that each callable is a member of at least one Capability granted by the Skill, and apply the intersection of applicable trusted Scope policies.
6. Resolve the exact callable against the installed Provider's SBI Operation Binding Catalog.
7. Verify deployment service configuration and source/deployment availability. A generic transport or a 3GPP OpenAPI definition alone does not prove that the selected OpenAMSM deployment implements the operation.
8. Build Agent-facing Tool descriptors from the Owner-published ToolCatalog contract for ordinary callable targets permitted to be Agent-visible. Reconcile-only callables are bound for Runtime internal dispatch and are not exposed as independent Agent tools by default. The Provider supplies implementation support; it does not author or override the Tool schema.
9. Freeze the Tool set and implementation bindings for the active Skill/Invocation.
10. At every dispatch, Runtime rechecks callable admission, Capability Grant, Scope and Effect. Tool visibility is not authorization.

If a required callable is absent from ToolCatalog, not a member of a granted Capability, unsupported by the installed Provider, unavailable in deployment configuration or not source-verified for the selected NF deployment, the Skill MUST NOT be reported READY for that callable.

A Capability may contain more callables than a Skill uses. The Host MUST NOT expose unused members merely because the Skill granted the Capability.

## 7. Provider process boundary

The Host/Provider integration is a language-neutral executable process protocol.
The Provider is a preinstalled Go executable shipped by `ca-client-tools`.
It is not an Agent-facing HTTP server, MCP gateway, arbitrary command runner
or a Go library imported into the Python Host.

The Host MUST use a fixed executable path and fixed bootstrap argument vector,
never a shell. The conceptual bootstrap command is:

~~~text
ca-client-tools provider serve
~~~

The Host starts one long-lived process for each configured Provider instance
and reuses it across catalog and Tool invocation requests. It MUST NOT launch
a fresh operating-system process or invoke a CLI subcommand for each Tool call.
The Host verifies the executable/package digest and pinned SBI Operation
Binding Catalog against trusted deployment metadata before making the Provider
available. A running Invocation remains pinned to its selected Provider
identity and package digest.

### 7.1 Process lifetime and ownership

- The Host/Resident owns installation, trusted configuration, process startup,
  readiness, concurrency limits, deadlines, cancellation, draining, shutdown
  and restart policy.
- The Provider owns only its own process resources, typed client instances,
  connection pools and bounded request execution. It does not own Skill state,
  Invocation lifecycle, Capability authorization, pending-operation state,
  Wait/Ingress, transaction journals, Outcome, Evidence or Reconcile.
- The Host marks the Provider available only after executable identity and
  package digest checks succeed and the required callable support projection
  has been validated against ToolCatalog, CapabilityContracts and the pinned
  SBI Operation Binding Catalog.
- On graceful shutdown, the Host stops admitting new calls, drains in-flight
  requests, then closes the Provider's stdin. A bounded grace period may be
  followed by forced process termination.
- A normal Tool failure is a correlated response and MUST NOT terminate the
  Provider process. A fatal process exit fails outstanding calls. If a mutation
  may have reached the network, its result is uncertain and MUST be handled by
  the existing Runtime/Fabric Reconcile path; a restarted Provider MUST NOT
  automatically replay it.
- `requestId` is for protocol correlation only. It is not an idempotency key
  and does not authorize retries.

### 7.2 Framing and request correlation

The control protocol uses bounded newline-delimited JSON (NDJSON):

- one UTF-8 JSON request object per stdin line;
- one matching UTF-8 JSON response object per stdout line;
- stdout is reserved exclusively for protocol frames; diagnostics go to
  stderr;
- each request carries a unique `requestId`, echoed unchanged by its response;
- responses may complete out of order, allowing bounded concurrent requests;
  the Host correlates them by `requestId`;
- frame size, in-flight request count, per-call duration and aggregate resource
  use are bounded by trusted deployment configuration;
- pretty-printed/multi-line JSON, unsolicited stdout text and unknown protocol
  versions are rejected.

The protocol has three machine-oriented methods:

- `catalog`: return a support projection for exact callable identities
  already resolved by the Host;
- `invoke`: execute one exact callable identity with its validated input;
- `cancel`: cancel one in-flight request and propagate cancellation to the
  associated Go `context.Context` and active HTTP request.

These methods are messages on the resident process. They are not separate
CLI invocations. The exact v1 envelope fields, error-code registry and startup
handshake schema remain to be frozen before implementation; examples in this
document are illustrative, not a frozen wire schema.

### 7.3 Catalog support projection

A `catalog` request contains exact callable identities, not Capability IDs:

~~~json
{
  "method": "catalog",
  "requestId": "req-catalog-001",
  "callables": [
    {
      "owner": "smf-owner",
      "kind": "observation",
      "id": "smf.context.session.query"
    }
  ]
}
~~~

The response is a support projection only:

~~~json
{
  "requestId": "req-catalog-001",
  "status": "ok",
  "data": {
    "supported": [
      {
        "owner": "smf-owner",
        "kind": "observation",
        "id": "smf.context.session.query",
        "bindingRef": "openamsm.smf.context.session"
      }
    ],
    "unsupported": []
  }
}
~~~

The Host MUST verify every returned identity is an exact member of the request
and matches the trusted ToolCatalog and pinned Provider SBI Operation Binding
Catalog. Duplicate, unknown or altered identities fail closed. The Provider
response MUST NOT supply or override Capability, Effect, Input/Output Schema,
URL, HTTP method, path, headers or credentials.

### 7.4 Invocation and cancellation

An `invoke` frame carries:

- exact callable identity (Owner, kind, id);
- bounded JSON arguments validated against the ToolCatalog Input or
  QueryContext Schema;
- Host-generated Invocation metadata required by the trusted binding;
- Host-staged relative artifact identifiers where the operation contract
  requires binary inputs or outputs.

It MUST NOT accept Agent-supplied URLs, methods, paths, arbitrary headers,
token values, service keys, absolute filesystem paths, executable code or
arbitrary command-line arguments.

A `cancel` frame identifies the target in-flight `requestId`. The Provider
cancels the matching operation context and its active transport request. If
the operation has already crossed a network side-effect boundary, cancellation
does not prove that the side effect did not occur; the normal Reconcile
contract remains authoritative.

Per-call validation, transport and remote errors are returned as correlated
protocol responses. The Host/Runtime independently validates output, Owner
Outcome, Evidence and Mutation proof. Provider process success or HTTP 2xx
alone MUST NOT be treated as Runtime SUCCEEDED or Mutation COMMITTED.

### 7.5 Binary data channel

Binary data is never embedded in JSON or Base64 for the Agent. For binary
operations, the Host resolves HostArtifactRef values into private,
per-invocation staging areas and passes only Host-generated relative artifact
identifiers through the process protocol. The Provider streams staged bytes
into the SBI request and writes response attachments only to Host-created
output staging areas. The Host validates and ingests accepted bytes as new
HostArtifactRefs. The Provider MUST NOT create a parallel persistent artifact
store or choose staging paths.

The first SMF MAS context-query vertical slice is JSON-only and does not
depend on the binary staging channel or HostArtifactRef implementation.

### 7.6 Go implementation API and CLI separation

Inside `ca-client-tools`, Provider handlers call the typed Go client,
transport and codec packages directly through normal Go APIs. They MUST NOT
spawn the CLI, shell out to another executable or send a request back into
their own Provider process.

The CLI is a separate operator/developer/test entrypoint. It may reuse the
same typed Go packages, but it is not the Host-to-Provider integration
contract. The Python Host does not import a Go SDK; Host integrations speak
the process protocol. This keeps the external boundary language-neutral while
allowing the Provider implementation to use efficient typed in-process Go
calls.

## 8. Direct SBI calls versus existing governed network workflows

The existence of an SBI operation name does not mean that a direct client call may replace the existing Owner workflow.

Before an operation is exposed through the generic SBI Provider, verify:

- its exact 3GPP or private API contract;
- the selected OpenAMSM source actually implements the endpoint;
- the Tool Owner and authoritative state Owner;
- identity, Scope, correlation and lifecycle bindings;
- mutation ownership, single-sender, idempotency and Reconcile requirements;
- the live acceptance plan.

The existing `ca-provider-5gc` CreatedSession implementation is a migration source, not a permanent parallel Tool plane. New NF client callables and reusable protocol codecs belong in `ca-client-tools`; `ca-provider-5gc` is migration-only and should receive no new parallel Tool capabilities. Its client-side callable behaviors and reusable codecs are in scope for absorption into `ca-client-tools`, including source-verified NF context queries, NAS/NGAP preparation, SBI request/response adapters and N1/N2 transfer callables. This is not a blind copy: each behavior must be split at its actual authority boundary. The existing Provider-local `operation-context:read` projection is not an NF client endpoint; decompose it into Host/Runtime-owned invocation/lifecycle context, source-backed NF Observation inputs and preparation-only values derived inside the typed prepare Tool. Do not port the Provider's short-lived local projection store as a second context authority.

- Move typed NF client calls, OpenAPI bindings, request/response schemas, NAS/NGAP codecs and operation-specific message construction into `ca-client-tools`.
- Move CA-generated binary bytes from the Provider's `ProtectedArtifactStore` to the Host Artifact Store. New preparation callables return binary output streams; Host ingests them and issues HostArtifactRefs. New transfer callables consume HostArtifactRefs through Host staging. The legacy `csa:v1:` reference is a temporary migration bridge only.
- Keep Fabric's durable mutation journal, reservation/claim, single-sender, SCP routing, exact target-producer validation and network dispatch authority in `env-fabric`. A migrated Tool callable must use that governed path where required; it must not bypass Fabric with a direct AMF HTTP request.
- Move invocation correlation, Skill routing, Capability/Scope authorization, Wait/Ingress continuation, Outcome/Evidence and Reconcile orchestration to Host/Runtime contracts. Do not recreate these in a Provider-local state machine.
- Keep authoritative NF state in the actual NF/OpenAMSM. A client Tool response is not itself proof of a final network Postcondition.

The existing `sbi.udm.sm_data.get` and other Provider-backed reads should also be migrated to the typed client Tool model after preserving their exact provenance, response and error semantics. The migration is explicit and parity-tested; no legacy path is redirected silently.

Current ca-client-tools evidence must be kept distinct:

- the eight SMF MAS Context queries are private OpenAMSM interfaces, not standardized 3GPP SBI operations; they are still eligible for typed Tool callables under their published private contract;
- AMF/SMF metrics endpoints are private HTTP projections, not SBI ToolCatalog operations by default;
- the shared h2c Transport and Payload Engine do not prove that any additional NF API is implemented;
- UDM/PCF/AUSF/NRF and other operations remain deferred until their pinned source and deployment support are verified.

A generic Provider is a reusable execution mechanism, not a claim of universal NF API coverage.

## 9. Effect, asynchronous responses and binary data

- Effect comes from ToolCatalog and MUST agree with the independently trusted CapabilityContracts declaration. Provider support metadata does not define Effect.
- READ operations still require Capability authorization, Scope and audit.
- Mutation requests are not automatically retried. A retry requires the exact Owner contract and Runtime policy to permit it.
- HTTP 202 and correlation headers are returned to Host/Runtime. The Provider does not create pending-operation state, Wait, Ingress listeners, continuation or a journal.
- A successful HTTP response does not itself prove business Postcondition or external Mutation commitment.
- Host owns the canonical HostArtifactRef storage, access control, Invocation/producer/consumer binding, expiry, cleanup and per-dispatch staging. All CA-generated Tool payloads use this artifact path, including NAS/NGAP prepared bytes.
- New Providers MUST NOT create a parallel persistent artifact store or expose base64 payloads to the model. The legacy `csa:v1:` reference may be forwarded only by the temporary compatibility adapter while the CreatedSession migration is incomplete.
- Transport and Payload Engine preserve protocol and payload semantics; Runtime/Owner retain Outcome, Evidence and Reconcile authority.

## 10. Configuration and security

The first deployment profile uses static, trusted service configuration. NRF discovery, OAuth2, TLS/mTLS management and token lifecycle are deferred as previously decided.

The following values are never Skill/Agent-controlled:

- service origin and API root;
- HTTP method and path;
- trusted header/token profile and token value;
- redirect/proxy policy;
- transport timeout and resource limits;
- binary staging roots;
- Provider executable path and argv.

The Provider uses only configured service keys and trusted binding metadata. It does not accept absolute URLs or caller-selected network destinations. The Host terminates the process on cancellation and discards partial outputs; the Provider cancels the associated request.

## 11. Migration boundary with current repositories

### ca-skills

- Keep CA-SL v1alpha4 FROZEN; do not introduce v1alpha3 fallback or compatibility semantics.
- Do not add Provider, implementation ID, URL, HTTP method/path or deployment service configuration to Skill source.
- Keep `contract.yaml.capabilities` (Capability IDs) separate from `procedure.yaml` `operation.tool` / `observation.tool` callable references.
- Do not rename current Skill callable references as part of this Provider design.

### ca-agent-demo

- Reuse existing Skill admission, ToolCatalog validation, CapabilityContracts, Runtime backend and READY coverage.
- Do not create a second Runtime, Capability registry, generic Tool registry, discovery service or MCP gateway.
- The current manual registration remains deployment-selected and frozen. The SBI Provider adapter binds exact callable identities and preserves Host/Runtime governance.
- Migrate the reusable client-side portions of `ca-provider-5gc` into typed `ca-client-tools` callables. Replace Provider-owned CA binary storage with HostArtifactRef. Keep Fabric's durable mutation transaction and network dispatch authority in `env-fabric`; do not copy its journal/claim/single-sender state into the SBI client.
- Keep only a temporary compatibility adapter for old `csa:v1:` references until all Skills and callers migrate; then retire the old Provider API and store.

### ca-client-tools

- Keep the current CLI v1 contract unchanged.
- Keep AMF/SMF metrics clients separate from standardized SBI operations.
- Keep the eight SMF MAS context-query endpoints as private OpenAMSM wire interfaces, not standardized 3GPP SBI services. Their stable CA-facing Observation callable contracts and public context-type vocabulary may be exposed through ToolCatalog; private route IDs and MAS wire codes remain inside the adapter.
- Deliver those query callables and their ca-agent-demo Host/Runtime integration as one end-to-end acceptance slice. Reuse the existing h2c/JSON client path; do not wait for binary/multipart support or HostArtifactRef storage.
- Implement the trusted SBI Operation Binding Catalog and Provider process contract as the exact callable-to-implementation binding mechanism. Private OpenAMSM operations use an explicitly pinned private binding, not an invented 3GPP OpenAPI operationId.
- Keep CLI v1 separate from Agent Tool callables; the CLI may reuse the same typed NF clients without becoming the Agent dispatch mechanism.

## 12. Implementation sequence

| Step | Work | Exit condition |
|---|---|---|
| 0 | Review and freeze the identity/authority split in this document | CA-SL, ToolCatalog, CapabilityContracts and Provider roles have no ambiguous identity mapping |
| 1 | Define the versioned SBI Operation Binding Catalog schema | No duplicate Capability, Effect or Tool schema authority |
| 2 | Load and validate pinned OpenAPI plus CA execution bindings | Unknown operation, unpinned source, unsupported media type or service key fails closed |
| 3 | Implement the long-lived Provider process protocol | Fixed bootstrap executable/argv, one resident process, bounded NDJSON catalog/invoke/cancel frames, request correlation, cancellation, drain/shutdown and no arbitrary HTTP |
| 4 | Deliver the eight SMF MAS read-only context queries as one end-to-end vertical slice: publish their exact Observation callable contracts and private OpenAMSM bindings in ca-client-tools; install/select the Provider in ca-agent-demo; connect Host/Runtime dispatch and Capability/Scope admission; run a real query through an admitted Skill | All eight query types are callable through the actual Host/Runtime path against OpenAMSM; public output and AVAILABLE/UNAVAILABLE semantics match; unauthorized/unsupported calls fail before NF dispatch. CLI-only or Provider-only PASS is insufficient |
| 5 | Migrate CreatedSession prepare/transfer callables and NAS/NGAP codecs from ca-provider-5gc | HostArtifactRef replaces csa:v1; output/input parity passes; Fabric journal, reservation, single-sender and SCP path remain intact |
| 6 | Add or migrate mutation callables only with their exact NF/Fabric transaction contract | No bypass of hold/claim, journal, single-sender, callback correlation or exact-original Reconcile |
| 7 | Add multipart/binary callables and Host Artifact Store integration | HostArtifactRef lifecycle, mixed-part streaming, cancellation, limits and real payload E2E pass |

The first live end-to-end callable slice is the eight source-verified SMF MAS context-query interfaces in the pinned OpenAMSM deployment. They are private OpenAMSM contracts, not standardized 3GPP SBI operations. Do not select a UDM/PCF API merely because the 3GPP OpenAPI repository defines it, and do not infer an endpoint that the selected deployment does not implement.

## 13. Required acceptance gates

1. A Skill with a valid Capability Grant and exact callable reference resolves successfully.
2. Missing Grant, wrong Owner, wrong kind, wrong callable ID, duplicate membership, Effect mismatch or missing Scope fails closed.
3. A Provider cannot create a Capability, widen membership or alter Scope.
4. A Provider cannot execute a callable absent from its pinned SBI Operation Binding Catalog.
5. A Provider cannot alter ToolCatalog schemas, Effect or Owner outcomes.
6. A Skill cannot override service key, origin, method, path, headers, token profile or binary paths.
7. Only ordinary callables actually referenced by the admitted Skill and allowed by Host policy are exposed to the Agent; Reconcile-only callables are bound for Runtime use, not exposed as independent Agent tools by default.
8. Runtime repeats Capability, Scope and Effect checks on every dispatch.
9. Two Capabilities containing the same callable apply the required trusted Scope intersection; no prefix or wildcard matching occurs.
10. A Provider switch during an Invocation is rejected; uncertain Mutation Reconcile remains pinned to the original Provider.
11. A 202 response does not create Provider-side Wait/Ingress state.
12. Binary input/output remains Host-owned and invocation-scoped.
13. Source support, client implementation, fixture tests, CLI live acceptance and Host/Runtime dispatch E2E acceptance are reported as separate evidence states.
14. The eight SMF MAS context queries succeed through the installed ca-client-tools Provider and actual ca-agent-demo Host/Runtime dispatch against OpenAMSM; the public envelope preserves type/status/key/context and AVAILABLE/UNAVAILABLE, while source masId is validated but not exposed.
15. An ungranted callable, wrong Scope, unsupported context type or unregistered Provider binding is rejected before any NF request. CLI-only or mocked Host acceptance does not pass this gate.

## 14. Decisions intentionally left open

The following must be decided before implementation, not guessed in code:

- exact SBI Operation Binding Catalog apiVersion and canonical digest framing;
- exact SBI Operation Binding Catalog schema and how generated OpenAPI files are packaged;
- exact Provider NDJSON v1 envelope fields, startup handshake, cancellation/error frames and process-drain semantics (the long-lived process and framing model are fixed; the field-level schema remains open);
- additional standardized SBI Operation callables to onboard after the private SMF MAS query vertical slice;
- exact ToolCatalog Owner identity for each callable and the authorized relationship between that Owner and the Provider implementation.

These are contract-version decisions. They do not justify adding Capability fields to Skill source or changing frozen CA-SL semantics.

## 15. Non-goals

This design does not implement:

- arbitrary Agent-facing curl or raw HTTP;
- automatic Provider/plugin discovery or runtime implementation switching;
- OAuth2, TLS/mTLS or NRF discovery;
- a second persistent Provider artifact store;
- Host/Runtime Wait, Ingress, Journal, Evidence or Reconcile;
- direct PFCP/N4 transport;
- migration of Fabric's transaction authority or its durable journal into ca-client-tools;
- universal support for all 3GPP SBI APIs.

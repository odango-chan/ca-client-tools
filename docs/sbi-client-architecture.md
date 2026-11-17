# CA Client Tools SBI Client Architecture

Status: **SBI Client v1 architecture — partially implemented. PR #1 read-client/CLI baseline and PR #2 shared h2c transport are merged; PR #3 streaming JSON/binary/multipart Payload Engine is merged; the Provider Callable Binding Catalog and long-lived Tool Provider process protocol remain design-only.**
Architecture baseline: **SBI Client v1 (separate from CLI v1)**

> Scope notice: PR #1 delivers the OpenAMSM read-only clients and CLI v1.
> PR #2 adds the shared h2c HTTP/2 transport and trusted OpenAMSM query-token
> profile. PR #3 implemented and merged the streaming JSON/binary/multipart
> Payload Engine, with Windows-native and WSL local acceptance PASS. No
> catalogued binary operation has been accepted live. The Operation Binding
> Catalog and persistent Tool Provider process protocol remain future design; TLS, mTLS, OAuth2 and
> NRF discovery are deferred.
> None of these workstreams changes the CLI v1 contract. The real SMF context
> client uses the existing private query-token mechanism over cleartext HTTP/2.

This document is the normative design for the reusable SBI client, the
Provider Callable Binding Catalog, the CA-Agent Tool Provider contract, and binary/multipart
payload handling. It extends the existing read-client and CLI v1 baseline; it
does not claim that the new runtime components have been implemented.

## 1. Decision

CA Client Tools is not read-only by definition. It provides reusable network
client capabilities for both read and mutation operations, with each
individual operation explicitly described, registered, and authorized.

The repository owns:

- a reusable Go SBI HTTP/2 client and request executor;
- a trusted, versioned Provider Callable Binding Catalog derived primarily from pinned 3GPP OpenAPI;
- a Tool Provider that exposes operation descriptors and executes admitted
  operations for CA-Agent;
- OpenAMSM-specific client adapters and private API definitions;
- a stable local CLI for operator and test use.

The transport and payload engine are NF/service-agnostic. The current
deployment/source target remains OpenAMSM. This is not an Open5GS compatibility
layer and does not imply that every standardized 3GPP operation is available
in the selected OpenAMSM deployment.

The existing AMF/SMF metrics clients and UPF diagnostic projection are not
standardized SBI operations. They remain separate typed HTTP clients. Direct
N4 transport between SMF and UPF is PFCP, not SBI. However, a standardized SBI
operation may carry an opaque PFCP message as a multipart binary part (for
example, Nsmf_PDUSession over N16a). The SBI Payload Engine transports that
declared byte stream; it does not implement the N4 PFCP socket protocol or
parse PFCP messages.

## 2. Ownership and invocation flow

The Tool Provider is a preinstalled, long-lived Go executable shipped with the
CA-Agent deployment. The Host starts it once for the configured Provider
instance and reuses that process for catalog and invocation requests; it does
not launch a new process for every Tool call. The current CA-Agent Demo Host
is Python, so the Host/Provider boundary is a language-neutral process protocol,
not an in-process Go SDK import and not a new HTTP/MCP daemon. The Provider
uses Go package APIs internally to call typed NF clients and codecs directly.

The Host dynamically activates only the callable descriptors used by the
active Skill, deriving their schemas and Effect from ToolCatalog and their
authorization from trusted CapabilityContracts. It verifies Provider support
against the pinned Provider Callable Binding Catalog.

For a binary argument typed as a **HostArtifactRef**, the Host validates the
reference against its own Artifact Store, the current Invocation, producer
lineage, allowed consumer callable, expiry and applicable Scope. It stages
the bytes into a private per-dispatch directory and passes only Host-generated
relative file identifiers to the Go process. The process streams those files
into the SBI request and writes response attachments into a separate
Host-created output directory. The Host validates and ingests accepted
attachments as new HostArtifactRefs, then removes the temporary staging
directory.

The existing `csa:v1:` value is a **legacy ca-provider-5gc reference**, not a
permanent CA artifact type. During migration it may be forwarded only to the
exact legacy compatibility operation. New ca-client-tools callables do not
issue or require it. Their binary outputs are streamed back to Host, which
ingests the bytes and issues a HostArtifactRef. The legacy reference is retired
after the preparation/transfer migration and parity acceptance.

This keeps the language boundary explicit, avoids base64 and avoids a second
persistent artifact store in ca-client-tools. The Host must use a fixed
executable and argument vector, never a shell. The Host Artifact Contract is
being reviewed in [ca-agent-demo PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211).

The Host dynamically activates the operation tools needed by the currently
loaded Skill. The Provider Callable Binding Catalog and executors are trusted package content.
ToolCatalog and CapabilityContracts remain separate trusted inputs; the
Skill supplies only Capability Grants, exact callable references and typed
arguments.

~~~text
ca-skills
  declares required Capabilities
          |
          v
ca-agent Host
  resolves the active Skill and deployment policy
  resolves callable refs through ToolCatalog + CapabilityContracts
  checks the Provider Provider Callable Binding Catalog
  builds active-Skill Tool descriptors
          |
          v
ca-runtime
  admits the callable and authorizes its Capability
          |
          v
ca-agent Host Provider Process Adapter
  manages one resident Provider process
  sends NDJSON invoke/cancel frames and correlates responses
          |
          v
ca-client-tools Tool Provider
  validates operation identity and arguments
  resolves the trusted Provider Callable Binding Catalog entry
  calls typed Go client/codec packages directly
          |
          v
SBI Executor
  h2c HTTP/2 transport + trusted OpenAMSM query-token header + Payload Engine
          |
          v
AMF / SMF / UDM / PCF / AUSF / NRF / other configured NF services
~~~

The resident Provider process accepts machine-oriented `catalog`, `invoke`
and `cancel` protocol messages. `catalog` accepts exact callable identities
already resolved by the Host and returns a support projection from the
Provider's pinned Provider Callable Binding Catalog; `invoke` executes one exact
callable identity; `cancel` cancels one in-flight request. These are protocol
methods on one long-lived process, not separate per-call CLI launches. The
catalog response is not a Capability grant and does not return Tool schemas
or Effect. The Host builds Agent-facing Tool descriptors from the
Owner-published ToolCatalog and trusted deployment CapabilityContracts. The
Provider protocol itself is never exposed as an Agent tool.

Control and JSON operation data use a bounded newline-delimited JSON stream
on stdin/stdout. Each request and response is one UTF-8 JSON frame; every
request has a correlation ID, and responses may complete out of order. stdout
is reserved exclusively for protocol frames; diagnostics go to stderr. The
Host may keep multiple bounded invocations in flight and correlates each
response to its request. The exact v1 envelope fields and error-code registry
remain a separately versioned implementation contract.

Binary data is not embedded in JSON or Base64. For later binary operations,
the Host creates private per-invocation staging areas and passes only
Host-generated relative artifact identifiers through the protocol. The Host
controls staging roots and permissions; the Skill cannot select a path or
directory.

Ownership is deliberately split:

| Component | Owns |
|---|---|
| ca-client-tools | SBI transport, Provider Callable Binding Catalog, typed NF callables, operation-specific NAS/NGAP codecs and message construction, request/response handling, Payload Engine, Tool Provider implementation and local CLI |
| ca-agent | Preinstalling/integrating the Provider, active-Skill tool registration, trusted configuration injection, Host policy, canonical HostArtifactRef storage/lifecycle and per-dispatch staging |
| ca-runtime | Callable admission, Capability / Scope authorization, execution identity, Procedure progression, per-call artifact-reference provenance/admission, Outcome, Evidence and Reconcile |
| ca-skills | Business Procedure and declared callable dependencies |
| env-openamsm | OpenAMSM source, build, deployment, NF lifecycle and deployment-specific source overlays |
| env-fabric | Durable network-mutation journal, reservation/claim, single-sender enforcement, SCP routing and exact target-producer validation |

CA Client Tools does not own Skill selection, authorization decisions, Runtime
state, pending-operation lifecycle, business orchestration, Evidence, Outcome,
or Reconcile. The Host/Runtime contracts remain authoritative for those
concerns.

The Provider executable is a process boundary, not the Tool implementation
boundary inside the Go repository. Provider handlers call typed Go client and
codec packages directly; they do not spawn the CLI, invoke shell commands or
re-enter the Provider process. The CLI is a separate operator/test entrypoint
that may reuse the same Go packages. Host integrations use the process protocol,
not a Go SDK dependency.

## 3. Provider Callable Binding Catalog

### 3.1 Source of truth

For standardized SBI operations, use the applicable release-specific 3GPP
OpenAPI definition as the source for:

- operationId, HTTP method and path template;
- path, query and header parameters;
- request and response schemas;
- supported request/response media types;
- response status codes and declared response headers.

3GPP TS 29.501 clause 5.3 defines the OpenAPI specification files for 5GC
SBI APIs. The 3GPP 5G APIs repository is available at
https://forge.3gpp.org/rep/all/5G_APIs. Pin the release, document version,
source revision and digest used by a build. Do not consume an unpinned latest
definition at runtime.

Keep three authorities separate:

- the Owner-published CA-SL ToolCatalog defines the callable identity,
  kind, Owner, Effect, Owner outcomes and Input/Output or QueryContext Schema;
- deployment CapabilityContracts defines Capability IDs, exact callable
  membership, expected Effect and Scope policies;
- the CA Client Tools Provider Callable Binding Catalog maps an exact callable
  identity either to a pinned 3GPP OpenAPI operation or to a source-pinned private
  typed Observation binding, together with its trusted service key.

The CA-specific execution binding adds only transport metadata not already
owned by those contracts, for example a trusted service key and operation-
specific multipart/HostArtifactRef bindings. It MUST NOT duplicate Capability
membership, Scope policy, ToolCatalog schemas or Effect.

OpenAMSM-private interfaces, including the context-query API, use separately
identified private typed Observation bindings. They are not standardized
3GPP SBI operations. Private route identifiers and MAS selectors remain inside
the Provider binding and MUST NOT appear in Skill source or public callable IDs.

### 3.2 Trusted definitions, dynamic registration

Callable definitions and Provider execution bindings are trusted, versioned
package content. They may be generated from pinned OpenAPI files and completed
by reviewed CA metadata. They are not authored or uploaded by a Skill at
invocation time.

Dynamic registration means:

1. the Skill declares Capability Grants and references exact callable exports;
2. the Host resolves each callable through the Skill's immutable dependency
   snapshots and the deployment's trusted CapabilityContracts;
3. the Host verifies that the exact callable is supported by an installed,
   pinned Provider and a trusted SBI execution binding;
4. the Host applies deployment availability and policy;
5. only the exact callable descriptors used by the active Skill are activated;
6. Runtime admission and Capability authorization still apply to every call.

A Skill must not supply or override an arbitrary URL, HTTP method, path,
header-token profile, raw header, content type, service key or Provider
binding. Unknown, unavailable, unregistered or unauthorized callables fail
closed.

The Provider's support declaration is not a Capability grant and is not a
Tool contract. The Host builds Agent-facing Tool descriptors from the
Owner-published ToolCatalog and deployment authorization inputs.

### 3.3 Callable identity and execution metadata

A callable is identified by the exact tuple (owner, kind, id). The
Dependency Alias selects the immutable package snapshot; the exported id
identifies the callable within that snapshot. For example,
`ca::sbi.udm.sm_data.get` is a qualified callable reference: `ca` is the
Dependency Alias and `sbi.udm.sm_data.get` is the exported callable id. It
is not, by itself, a Capability ID.

Capability is a separate deployment authorization grouping. One Capability
may contain multiple exact callables, and one callable may be included in
multiple Capabilities. Membership is declared only by trusted
CapabilityContracts; prefix matching, wildcard membership and implicit
expansion are forbidden.

The first SMF slice's exact private bindings are published in the versioned
`contracts/provider-bindings/v1/provider-bindings.yaml` file. It is embedded in
the Provider build; `bindingCatalogDigest` is SHA-256 of its exact UTF-8 bytes
including the final LF. The minimum Provider callable-binding metadata is:

| Field | Meaning |
|---|---|
| callable identity | Exact Owner, callable kind and exported id |
| API source | Pinned 3GPP release/document/OpenAPI revision and digest, or an explicitly identified private contract |
| source binding | Exact OpenAPI operationId for a standardized Operation, or a fixed private typed Observation binding identity |
| service key | Trusted service entry used to resolve the destination |
| payload bindings | JSON/binary/multipart mappings not derivable from the pinned API definition |
| support evidence | Separate source, client, fixture and live-deployment evidence |

Effect, Input/Output schemas, Owner outcomes, Capability membership and Scope
are owned by ToolCatalog and CapabilityContracts respectively; they MUST NOT
be copied into the SBI execution binding as competing authorities.

A 3GPP definition proves the standardized contract, not that the selected
OpenAMSM deployment implements it. Operation onboarding must separately record
deployment/source evidence in the NF capability map and interface inventory.
A private Observation binding instead pins the exact OpenAMSM source/OpenAPI
revision and its deployment evidence; it must not claim 3GPP standardization.
## 4. Tool exposure and mutation safety

The Provider exposes concrete registered callables, not a general-purpose
Agent-facing HTTP request tool.

- A registered READ callable (Operation or Observation) may be Agent-callable
  only when its Capability is declared and admitted by Host/Runtime policy.
- A registered Mutation Operation follows the same admission path, with its
  Effect explicitly visible in trusted metadata and subject to Host/Runtime policy.
- Host-internal callables may be available to the Host without being exposed
  to the Agent.
- A CLI subcommand or catalog entry does not itself grant permission.
- Raw request mode is for local operator diagnostics only and is never
  registered as an Agent Tool.

Mutation requests are not retried automatically. A retry may be enabled only
when the operation contract defines the applicable idempotency key, duplicate
suppression or reconciliation rule and the Host/Runtime policy permits it.
The client preserves the remote HTTP status, declared response headers and
ProblemDetails semantics; it does not invent business success or absence.

## 5. Transport and trusted configuration

### 5.1 Go h2c HTTP/2 client

The current OpenAMSM deployment uses cleartext HTTP/2 (h2c) with prior
knowledge. The shared SBI Transport provides bounded HTTP/2 transport for
callables already migrated to it and does not negotiate or fall back to
HTTP/1.1. The existing typed SMF context-query client has its own bounded h2c
transport; the first Provider reuses that client directly. Migration of that
client to the shared Transport is a separate refactor, not a prerequisite for
the first Provider slice, and MUST NOT duplicate its query implementation.

The client does not implement TLS/mTLS configuration, certificate management,
OAuth2 token acquisition or token refresh. Those are not client requirements
for the selected OpenAMSM interface. Any network protection or TLS termination
belongs to deployment infrastructure outside this client.

The transport provides:

- HTTP/2 connection reuse and bounded connection management;
- request context cancellation and bounded timeouts;
- bounded request and response sizes;
- configured API paths and declared operation headers;
- the existing private OpenAMSM query-token header where required;
- structured status, response-header and ProblemDetails handling.

Do not inherit ambient proxy settings, follow redirects, accept absolute
request URLs from callers, or allow operation arguments to override the
configured destination or trusted query-token header. Enforce bounded
timeouts, response sizes, connection lifetimes and cancellation.

### 5.2 Configuration-first service resolution

The architecture may use static, trusted service configuration; NRF discovery
is not required and is not implemented as a prerequisite. The YAML below is a
future multi-service deployment-configuration illustration, not an input to
Provider Process Protocol v1. The first SMF Observation slice has one private
service and uses only the exact allowlisted environment variables
`CA_OPENAMSM_SMF_SBI_ORIGIN` and `CA_OPENAMSM_QUERY_TOKEN`. No config-file path,
service map or secret is accepted in the v1 NDJSON frames. A generalized
configuration-file contract requires separate versioning.

Illustrative configuration:

~~~yaml
version: 1

services:
  namf-comm:
    origin: http://amf.sbi.example:7777
    apiRoot: /namf-comm/v1

  openamsm-smf-context:
    origin: http://smf:7777
    apiRoot: /
    headerTokenProfile: openamsm-query-token

headerTokenProfiles:
  openamsm-query-token:
    header: X-OpenAMSM-Query-Token
    tokenFile: /etc/ca/secrets/openamsm-query-token

limits:
  requestTimeout: 10s
  dialTimeout: 5s
  maxRequestBytes: 16777216
  maxResponseBytes: 16777216
~~~

This is an architectural example, not a claim that these endpoint names,
credentials or scopes are deployed. The exact values come from the deployment.

Configuration responsibilities:

- service entries provide static h2c origins and API roots;
- the existing OpenAMSM query token is supplied through a trusted header-token
  profile; the Host/deployment loads the token from its secret facility or
  secret file and injects it into the client configuration;
- no TLS profile, OAuth profile, token endpoint, token cache or credential
  lifecycle is implemented by this client.

Do not place plaintext secrets in committed configuration. Do not let a Skill
select or modify a service origin, header-token profile or token.

## 6. Payload model and artifact references

### 6.1 Supported payload classes

The Payload Engine supports the media types declared by an operation's
OpenAPI contract, including:

1. JSON request/response bodies;
2. a single opaque binary request/response body with its declared media type;
3. multipart messages containing a JSON root and one or more binary parts.

Examples of 3GPP binary media types include
application/vnd.3gpp.5gnas, application/vnd.3gpp.ngap and
application/vnd.3gpp.pfcp. The PFCP subtype represents an opaque PFCP message
payload carried inside an SBI body; it is not direct N4 UDP transport. For
application/vnd.3gpp.pfcp, TS 29.502 specifies the PFCP application-layer
message headers but excludes IP and UDP transport headers. The implementation
must not assume that every binary part is one of these types; the pinned
operation definition supplies the actual media type.

Do not base64-encode binary data into LLM-facing Tool arguments. Do not
reinterpret a binary payload as text.

When a callable must construct or parse NAS/NGAP, the codec is an
operation-specific package shipped with that typed ca-client-tools callable.
It is not part of the generic Payload Engine, and it is never implemented in
a Skill Procedure Script or delegated to an LLM.

### 6.2 Canonical HostArtifactRef and legacy migration

A **HostArtifactRef** is an opaque reference to immutable bytes managed by the
CA-Agent Host Artifact Store. The Host owns its storage, access control,
Invocation/producer/consumer binding, expiry, retention, digest and cleanup.
The physical reference grammar and Schema `$id` belong to the separate
versioned Host Artifact Contract; this document does not invent a URI scheme.

The existing `csa:v1:` N1/N2 value is a legacy reference issued by
`ca-provider-5gc`. It is not content-addressed and is not a permanent
cross-Provider artifact type. During migration only, a pinned compatibility
adapter may forward it to the exact legacy operation. New Tools must not mint
or require this reference.

For a HostArtifactRef argument, the Host validates the exact reference against
the current Invocation, producer lineage, allowed consumer callable, expiry
and applicable Scope, then creates private per-dispatch staging files. The Go
process receives a mapping from typed binary arguments to Host-generated
relative file identifiers, not an absolute path. The process reads those files
as streams. It rejects absolute paths, path traversal, symlinks and files
outside the configured roots.

For a legacy `csa:v1:` argument, only the pinned compatibility adapter may
forward it to the old Provider endpoint. New Tool callables accept
HostArtifactRef, which the Host resolves into per-dispatch streams. No
arbitrary reference forwarding or implicit dereference is allowed.

The Provider does not create a persistent artifact database. Its Go process
implements stream input and attachment output over Host-created staging
directories. All CA-generated protocol bytes returned by a Tool are ingested
by Host and returned as HostArtifactRefs; the Provider does not retain a
parallel Provider artifact store.

Illustrative Tool input (the exact Schema reference and token grammar are
owned by the Host Artifact Contract):

~~~json
{
  "n1Payload": "<opaque HostArtifactRef>"
}
~~~

The Host validates every reference before opening it. The Client receives only
the stream and trusted metadata needed for the operation. The Host never
exposes raw bytes, local paths or credentials to the model.

The CLI may support operator-supplied local files for diagnostics and tests.
A local filesystem path is never an Agent-authored artifact reference and must
never be accepted as an unrestricted Agent Tool argument.

### 6.3 Multipart/related wire rules

The implementation follows the applicable 3GPP multipart contract, not
multipart/form-data by default.

For 3GPP multipart/related messages:

1. the JSON root part is the first body part;
2. the multipart Content-Type carries the required root type parameter,
   application/json;
3. each binary body part has its own Content-ID;
4. the JSON root references the matching Content-ID through the
   RefToBinaryData field defined by the operation schema;
5. each part uses the content type declared by the operation OpenAPI
   definition;
6. the number of binary parts is operation-defined and is not hard-coded to
   N1 plus N2.

This is not only an AMF requirement. 3GPP TS 29.518 OpenAPI describes
multiple named N2 information extension parts for AMF operations. 3GPP
TS 29.502 clause 6.1.2.4 defines multipart messages for Nsmf_PDUSession
operations carrying N1/N2 information and N4 information; the specified N16a
cases can contain one, two or three N4 payload parts. The Payload Engine
therefore models an operation-defined collection of typed parts rather than
fixed n1Payload and n2Payload fields.

The Provider Callable Binding Catalog maps OpenAPI multipart part names to typed Tool
arguments and maps each RefToBinaryData field to its corresponding part.
Content-ID values are generated for the individual invocation and inserted
consistently into both the JSON reference and the MIME part header. They must
be unique within the message.

The request writer streams the JSON root and each binary part. It must not
buffer large payloads in memory or convert multipart/related to
multipart/form-data. The response parser handles multipart boundaries,
validates part headers and Content-ID uniqueness, identifies the root part
according to the operation contract, and streams binary response parts to
the Host AttachmentSink.

Enforce maximum part count, per-part and aggregate byte limits, valid media
types, unique Content-ID values, and cancellation. Reject missing, duplicate,
unreferenced or unexpected parts when the operation contract requires a
closed set. Never trust a filename or filesystem path received from a remote
part as a local write destination.

## 7. Provider process protocol

The normative process wire contract is
[Provider Process Protocol v1](provider-protocol-v1.md). This architecture
document does not duplicate its envelope fields, limits or error registry.

The Go Provider is a preinstalled, long-lived executable:
```text
ca-client-tools provider serve
```

The Host starts one process per configured Provider instance, performs the
`initialize` handshake, verifies package and Provider Callable Binding
Catalog digests, and uses bounded NDJSON `catalog`, `invoke` and `cancel`
messages over stdin/stdout. stderr is diagnostics only. The Host owns process
lifecycle, authorization, deadlines, cancellation, draining and shutdown.

The Provider catalog is an exact support projection, not a Capability grant
or source of Agent-facing schemas. The Host builds descriptors from the
Owner-published ToolCatalog and deployment CapabilityContracts.

The first SMF vertical slice exposes eight `kind: observation` callables.
It reuses the existing typed Go client directly, not a CLI subprocess and not
a duplicate query implementation. Public callable identities select fixed
private OpenAMSM bindings; no Agent-supplied MAS selector, URL, method, path,
header or credential is accepted.

Protocol v1 is JSON-only. HostArtifactRef, binary file references,
multipart transfer and NAS/NGAP payloads are deferred to a separately
versioned extension and are not dependencies of the first slice. The Host
v1alpha4 Observation stage and unified callable dispatch contract is tracked
in [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).

## 8. Curl-like raw request mode

A curl-like request facility is useful for operator debugging and protocol
bring-up. It is not the Agent execution model.

A future CLI extension may provide an operator-only command with:

- a configured service key;
- HTTP method and a relative path under that service's configured API root;
- JSON stdin, a single local binary file, or explicitly declared multipart
  file parts;
- response status, safe headers, JSON body and attachment metadata.

It must not accept an absolute URL, override the trusted service origin,
override the configured service or query-token profile, bypass network allowlists, or become a
Tool Provider descriptor. Raw mode is disabled by default and requires an
explicit local operator configuration switch.

The existing CLI v1 command tree and process contract remain unchanged by
this architecture decision. The Provider catalog/invoke process contract is
a separate machine interface and must have its own schemas, exit behavior,
cancellation rules and tests. Raw request mode is not part of CLI v1 or the
Provider contract and must be specified separately before implementation.

## 9. Asynchronous responses and protocol boundaries

The SBI Client performs one operation invocation and returns its actual HTTP
response, including status, declared headers and payloads. For example, a
202 Accepted response remains a 202 result with its correlation/Location
information. The Client does not wait for a notification, poll a resource,
register a callback, resume a Procedure, or maintain a pending-operation
journal.

Host/Runtime own pending-operation correlation, later inbound message
admission, continuation and Procedure resumption. This keeps the Client
consistent with the CA-SL execution model and the Host's lifecycle ownership.

Direct N4 transport between SMF and UPF is PFCP and remains a separate
protocol client concern. Some Nsmf_PDUSession SBI operations can carry one or
more opaque PFCP messages as application/vnd.3gpp.pfcp multipart parts; the
SBI Payload Engine transports those bytes according to the operation contract
but does not open PFCP associations, manage PFCP sequence/timer state or
decode PFCP. The selected OpenAMSM source does not expose a UPF
session-inspection HTTP endpoint, and this repository does not add one.

## 10. Implementation sequence

The work remains ordered to preserve the existing CLI v1 and typed
read-client baseline. Current status is recorded separately from the original
acceptance requirements.

| Step | Work | Acceptance | Current status |
|---|---|---|---|
| 1 | Extract reusable Go h2c HTTP/2 transport and trusted service/header-token configuration | h2c negotiation, HTTP/1 rejection, connection reuse, cancellation, private header-token isolation and bounded I/O tests | **MERGED — PR #2.** Windows-native and WSL local tests/live CLI acceptance PASS; GitHub Actions CI_UNAVAILABLE. |
| 2 | Implement PayloadSource/Resolver and AttachmentSink contracts plus JSON and single-binary bodies | streaming, size limits, cancellation and no-base64 tests | **MERGED — PR #3 (`4c424ee`).** Windows-native and WSL local acceptance PASS; live catalogued payload operation NOT_RUN. |
| 3 | Implement multipart/related request and response support | root-first JSON, Content-ID/RefToBinaryData binding, multiple binary parts, mixed media types, malformed and oversized part tests | **MERGED — PR #3 (`4c424ee`).** Windows-native and WSL local acceptance PASS; live payload-engine operation NOT_RUN. |
| 4 | Add trusted Provider Callable Binding Catalog loading/validation from pinned OpenAPI plus CA metadata | stable operation identity, schema/parameter validation, unknown operation rejection and no caller override of method/path/service/auth | **DESIGN ONLY — NOT IMPLEMENTED.** |
| 5 | Add the long-lived Provider process protocol and descriptors | One resident `provider serve` process; bounded NDJSON catalog/invoke/cancel frames; request correlation, bounded concurrency, cancellation, drain/shutdown and no arbitrary HTTP surface | **DESIGN ONLY — NOT IMPLEMENTED.** |
| 6 | Migrate the existing SMF context-query adapter to shared transport only after exact regression tests pass | preserve private token header, identity keys, AVAILABLE/UNAVAILABLE, ProblemDetails and echoed-key behavior | **NOT STARTED.** PR #2 explicitly deferred this migration pending exact regression coverage. |
| 7 | Add the first source-verified multipart SBI operation when its deployment contract is available | request/response fixture tests plus separate OpenAMSM source and live acceptance | **NOT_AVAILABLE / NOT_RUN.** No callable is currently catalogued for the Payload Engine. |
| 8 | Add the operator-only raw request CLI extension, if still needed | disabled-by-default behavior and proof it is never exposed as an Agent Tool | **DEFERRED.** Not part of CLI v1 or the Agent Tool model. |

Step 6 is an optional transport-consolidation task, not a prerequisite for the
first SMF context Observation Provider/Host vertical slice. That slice may reuse the already
source-verified typed h2c client as-is. Do not rewrite its transport merely to
satisfy package uniformity; migrate only when exact regression tests prove the
existing token, identity, status and ProblemDetails behavior is preserved.

The existing AMF/SMF metrics clients remain separate and continue to use
their own source-verified HTTP contracts. No UPF session-diagnostic client is
implemented. Do not force metrics endpoints through the Provider Callable Binding Catalog.

## 11. Required acceptance gates

- Standard SBI calls negotiate HTTP/2 and reject unintended protocol downgrade.
- The client uses h2c prior knowledge and rejects HTTP/1 fallback.
- The existing OpenAMSM query token is injected only from trusted configuration;
  Skill arguments cannot override the token header.
- Only registered operations can execute; Skill arguments cannot override
  method, path, origin, headers or the trusted query-token value.
- Read and mutation effect metadata is preserved; mutations are not retried
  without an explicit safe retry contract.
- JSON, single-binary and multipart request/response paths are tested.
- Multipart tests cover one and multiple binary parts, correct media types,
  root-first ordering, Content-ID references, duplicate/missing references,
  malformed boundaries, truncation, byte limits and cancellation.
- HostArtifactRef resolution and response attachment storage are Host-owned,
  Invocation-scoped and streaming. The legacy `csa:v1:` path is migration-only
  and is removed after parity acceptance. Staging paths are root-confined and
  no base64 payload is exposed to the model.
- The Host launches one Provider process with a fixed executable/argv; its
  catalog/invoke/cancel protocol frames are never Agent-visible.
- A 202 response is returned as a response; no pending-operation lifecycle is
  created inside the Client.
- Mock tests, pinned source tests/build and live-NF acceptance are reported as
  separate evidence levels.

## 12. Standards references

- 3GPP TS 29.500, clause 5.4, Serialization Protocol:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129500/18.08.00_60/ts_129500v180800p.pdf
- 3GPP TS 29.518, clause 6.1.2.4, HTTP multipart messages:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129518/18.07.00_60/ts_129518v180700p.pdf
- 3GPP TS 29.502, clause 6.1.2.4, Nsmf_PDUSession multipart messages,
  including N1/N2 and one-or-more N4 payload parts:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129502/18.08.00_60/ts_129502v180800p.pdf
- 3GPP TS 29.501, clause 5.3, OpenAPI specification files:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129501/18.06.00_60/ts_129501v180600p.pdf
- Official 3GPP 5G APIs repository:
  https://forge.3gpp.org/rep/all/5G_APIs

The applicable release and document revision must be pinned per operation.
These links establish the design references; they do not establish that a
particular OpenAMSM deployment implements a given operation.

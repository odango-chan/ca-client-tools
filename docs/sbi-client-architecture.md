# CA Client Tools SBI Client Architecture

Status: **SBI Client v1 architecture — partially implemented. PR #1 read-client/CLI baseline and PR #2 shared h2c transport are merged; PR #3 streaming JSON/binary/multipart Payload Engine is open; Operation Binding Catalog and Tool Provider remain design-only.**
Architecture baseline: **SBI Client v1 (separate from CLI v1)**

> Scope notice: PR #1 delivers the OpenAMSM read-only clients and CLI v1.
> PR #2 adds the shared h2c HTTP/2 transport and trusted OpenAMSM query-token
> profile. PR #3 is implementing the streaming JSON/binary/multipart Payload
> Engine and is not yet merged. The Operation Binding Catalog and Tool Provider
> remain future design; TLS, mTLS, OAuth2 and NRF discovery are deferred.
> None of these workstreams changes the CLI v1 contract. The real SMF context
> client uses the existing private query-token mechanism over cleartext HTTP/2.

This document is the normative design for the reusable SBI client, the
SBI Operation Binding Catalog, the CA-Agent Tool Provider contract, and binary/multipart
payload handling. It extends the existing read-client and CLI v1 baseline; it
does not claim that the new runtime components have been implemented.

## 1. Decision

CA Client Tools is not read-only by definition. It provides reusable network
client capabilities for both read and mutation operations, with each
individual operation explicitly described, registered, and authorized.

The repository owns:

- a reusable Go SBI HTTP/2 client and request executor;
- a trusted, versioned SBI Operation Binding Catalog derived primarily from pinned 3GPP OpenAPI;
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

The Tool Provider is a preinstalled Go executable shipped with the CA-Agent
deployment. The current CA-Agent Demo Host is Python; a fixed,
machine-readable subprocess protocol keeps the boundary language-neutral and
avoids coupling future Hosts to Go. It is not an in-process Go library or a
new HTTP/MCP daemon.

The Host dynamically activates only the callable descriptors used by the
active Skill, deriving their schemas and Effect from ToolCatalog and their
authorization from trusted CapabilityContracts. It verifies Provider support
against the pinned SBI Operation Binding Catalog.

For a binary argument typed as a **HostArtifactRef**, the Host validates the
reference against its own Artifact Store, the current Invocation, producer
lineage, allowed consumer callable, expiry and applicable Scope. It stages
the bytes into a private per-dispatch directory and passes only Host-generated
relative file identifiers to the Go process. The process streams those files
into the SBI request and writes response attachments into a separate
Host-created output directory. The Host validates and ingests accepted
attachments as new HostArtifactRefs, then removes the temporary staging
directory.

An **OwnerArtifactRef** is different. Its bytes and lifecycle remain with the
publishing Owner. The generic SBI Provider does not resolve Owner-specific
references such as the existing `csa:v1:` reference. Such a reference can be
used only by a callable whose Owner contract explicitly accepts it. A
cross-Owner transfer requires an explicit Owner export/handoff and Host
ingestion as a new HostArtifactRef; it is never inferred from the reference
string.

This keeps the language boundary explicit, avoids base64 and avoids a second
persistent artifact store in ca-client-tools. The Host must use a fixed
executable and argument vector, never a shell. The Host Artifact Contract is
being reviewed in [ca-agent-demo PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211).

The Host dynamically activates the operation tools needed by the currently
loaded Skill. The SBI Operation Binding Catalog and executors are trusted package content.
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
  checks the Provider SBI Operation Binding Catalog
  builds active-Skill Tool descriptors
          |
          v
ca-runtime
  admits the callable and authorizes its Capability
          |
          v
ca-client-tools Tool Provider
  validates operation identity and arguments
  resolves the trusted SBI Operation Binding Catalog entry
          |
          v
SBI Executor
  h2c HTTP/2 transport + trusted OpenAMSM query-token header + Payload Engine
          |
          v
AMF / SMF / UDM / PCF / AUSF / NRF / other configured NF services
~~~

The Provider process exposes two machine-oriented operations: a catalog
request that accepts exact callable identities already resolved by the Host
and returns a support projection from the Provider's pinned SBI Operation
Binding Catalog, and an invocation request for one exact callable identity.
The catalog response is not a Capability grant and does not return Tool
schemas or Effect. The Host builds Agent-facing Tool descriptors from the
Owner-published ToolCatalog and trusted deployment CapabilityContracts.
The generic invocation command is never itself exposed as an Agent tool.

The process contract uses JSON for operation arguments and metadata, plus
Host-created input/output staging directories for binary data. The Host
controls the paths through trusted process configuration; the Skill cannot
select a path or directory.

Ownership is deliberately split:

| Component | Owns |
|---|---|
| ca-client-tools | SBI transport, SBI Operation Binding Catalog, request/response handling, Payload Engine, Tool Provider implementation and local CLI |
| ca-agent | Preinstalling/integrating the Provider, active-Skill tool registration, trusted configuration injection, Host policy, HostArtifactRef lifecycle and staging, and explicit Owner artifact handoff |
| ca-runtime | Callable admission, Capability / Scope authorization, execution identity, Procedure progression, per-call artifact-reference provenance/admission, Outcome, Evidence and Reconcile |
| ca-skills | Business Procedure and declared callable dependencies |
| env-openamsm | OpenAMSM source, build, deployment, NF lifecycle and deployment-specific source overlays |

CA Client Tools does not own Skill selection, authorization decisions, Runtime
state, pending-operation lifecycle, business orchestration, Evidence, Outcome,
or Reconcile. The Host/Runtime contracts remain authoritative for those
concerns.

## 3. SBI Operation Binding Catalog

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
- the CA Client Tools SBI Operation Binding Catalog maps an exact callable
  identity to its pinned 3GPP OpenAPI operation and trusted service key.

The CA-specific execution binding adds only transport metadata not already
owned by those contracts, for example a trusted service key and operation-
specific multipart/HostArtifactRef bindings. It MUST NOT duplicate Capability
membership, Scope policy, ToolCatalog schemas or Effect.

OpenAMSM-private interfaces, including the existing MAS context-query API,
use separately identified private operation definitions and must not be
mislabeled as standardized 3GPP operations.

### 3.2 Trusted definitions, dynamic registration

Operation definitions and SBI execution bindings are trusted, versioned
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
header-token profile, raw header, content type, service key or Operation
Binding. Unknown, unavailable, unregistered or unauthorized operations fail
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

The minimum SBI execution-binding metadata is:

| Field | Meaning |
|---|---|
| callable identity | Exact Owner, callable kind and exported id |
| API source | Pinned 3GPP release/document/OpenAPI revision and digest, or an explicitly identified private contract |
| source operation | Exact OpenAPI operationId or private-contract operation identity |
| service key | Trusted service entry used to resolve the destination |
| payload bindings | JSON/binary/multipart mappings not derivable from the pinned API definition |
| support evidence | Separate source, client, fixture and live-deployment evidence |

Effect, Input/Output schemas, Owner outcomes, Capability membership and Scope
are owned by ToolCatalog and CapabilityContracts respectively; they MUST NOT
be copied into the SBI execution binding as competing authorities.

A 3GPP definition proves the standardized contract, not that the selected
OpenAMSM deployment implements it. Operation onboarding must separately record
deployment/source evidence in the NF capability map and interface inventory.
## 4. Tool exposure and mutation safety

The Provider exposes concrete registered operations, not a general-purpose
Agent-facing HTTP request tool.

- A registered read operation may be Agent-callable only when its Capability
  is declared and admitted by Host/Runtime policy.
- A registered mutation operation follows the same admission path, with its
  effect explicitly visible in trusted metadata and subject to the applicable
  Host/Runtime policy.
- Host-internal operations may be available to the Host without being exposed
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

The current OpenAMSM SBI deployment uses cleartext HTTP/2 (h2c) with prior
knowledge. The shared client uses the Go standard library HTTP transport and
enables only unencrypted HTTP/2 for configured http:// service origins. It
does not negotiate or fall back to HTTP/1.1.

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

The first version uses static, trusted service configuration. NRF discovery is
not required to use the client and is not implemented as a prerequisite.
Each service key resolves to a configured origin/API root. A future
ServiceResolver may replace static resolution without changing the operation
invocation contract.

Illustrative configuration:

~~~yaml
version: 1

services:
  namf-comm:
    origin: http://amf.sbi.example:7777
    apiRoot: /namf-comm/v1

  smf-mas-query:
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

### 6.2 HostArtifactRef and OwnerArtifactRef

A **HostArtifactRef** is an opaque reference to immutable bytes managed by the
CA-Agent Host Artifact Store. The Host owns its storage, access control,
Invocation/producer/consumer binding, expiry, retention, digest and cleanup.
The physical reference grammar and Schema `$id` belong to the separate
versioned Host Artifact Contract; this document does not invent a URI scheme.

An **OwnerArtifactRef** is an opaque reference managed by a specific Tool
Owner or Provider. Its bytes and lifecycle remain with that Owner. It is not a
HostArtifactRef and is not resolvable by the generic SBI Provider. The
existing `csa:v1:` N1/N2 reference is an example of an OwnerArtifactRef; it
must not be treated as content-addressed or accepted by an unrelated Provider.
The Owner-specific callable that publishes the reference remains responsible
for validating its operation, session/profile binding and expiry.

For a HostArtifactRef argument, the Host validates the exact reference against
the current Invocation, producer lineage, allowed consumer callable, expiry
and applicable Scope, then creates private per-dispatch staging files. The Go
process receives a mapping from typed binary arguments to Host-generated
relative file identifiers, not an absolute path. The process reads those files
as streams. It rejects absolute paths, path traversal, symlinks and files
outside the configured roots.

For an OwnerArtifactRef argument, the Host does not resolve or copy the bytes.
The reference is passed only to the exact Owner callable whose Input Schema
declares that reference class. Cross-Owner use requires an explicit export or
handoff contract; if the Host ingests exported bytes, it issues a new
HostArtifactRef and preserves the source Owner/reference/digest provenance.

The Provider does not create a second persistent Host-style artifact database.
Its Go process implements internal stream resolver and attachment-sink
abstractions over Host-created staging directories. Response attachments are
returned as HostArtifactRefs only when the callable contract assigns their
storage to the Host; an Owner-owned result remains Owner-managed.

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

The SBI Operation Binding Catalog maps OpenAPI multipart part names to typed Tool
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

## 7. Provider process contract

The Provider process contract is separate from the existing CLI v1 resource
commands. The Go Provider is a preinstalled executable, not an Agent-facing
HTTP server, MCP gateway or arbitrary command runner.

The Host uses a fixed executable and fixed argument vector, never a shell.
Its conceptual operations are:

~~~text
ca-client-tools provider catalog
ca-client-tools provider invoke
~~~

The catalog request contains exact callable identities already resolved by
the Host, not Capability IDs:

~~~json
{
  "callables": [
    {
      "owner": "smf-owner",
      "kind": "operation",
      "id": "sbi.smf.example-operation"
    }
  ]
}
~~~

It returns a support projection only:

~~~json
{
  "data": {
    "supported": [
      {
        "owner": "smf-owner",
        "kind": "operation",
        "id": "sbi.smf.example-operation",
        "bindingRef": "sbi.smf.example-operation"
      }
    ],
    "unsupported": []
  }
}
~~~

The Host verifies every returned identity is an exact member of the request
and matches the trusted ToolCatalog and the pinned Provider SBI Operation
Binding Catalog. Duplicate, unknown or altered identities fail
closed.

The Host builds Agent-facing Tool descriptors from the Owner-published
ToolCatalog and deployment CapabilityContracts. The Provider response MUST
NOT supply or override Capability, Effect, Input/Output Schema, URL, HTTP
method, path, headers or credentials.

The invoke command receives one exact callable identity (owner, kind, id),
bounded JSON arguments, Host-generated invocation metadata and Host-staged
relative binary file references where needed. It does not receive an
Agent-authored command line, arbitrary URL, method, path, service key,
credential or absolute filesystem path.

The Provider validates the exact callable and its trusted binding, constructs
the request from pinned metadata, calls the shared SBI Transport and returns
a bounded structured result. The Host/Runtime independently validates output,
Owner Outcome, Evidence and Mutation proof. Provider process success or HTTP
2xx alone MUST NOT be treated as Runtime SUCCEEDED or Mutation COMMITTED.

For HostArtifactRef arguments, the Host resolves the reference into private,
per-dispatch input files. The process streams those files into the SBI request
and writes response attachments into a separate Host-created output directory.
The Host validates and ingests accepted Host-owned attachments as new
HostArtifactRefs. OwnerArtifactRefs are not resolved by this generic Provider;
they remain with their Owner unless an explicit export/handoff is performed.
The Provider does not own a second persistent Host artifact store and does not
base64-encode binary payloads for the model.

The process uses the same one-document stdout / one-document stderr
discipline as CLI v1, with a documented nonzero exit code for operational
failure. Host cancellation terminates the child and aborts its HTTP request;
partial stdout and attachment files are discarded.
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

The following work is ordered to preserve the existing CLI v1 and typed
read-client baseline.

| Step | Work | Acceptance |
|---|---|---|
| 1 | Extract reusable Go h2c HTTP/2 transport and trusted service/header-token configuration | h2c negotiation, HTTP/1 rejection, connection reuse, cancellation, private header-token isolation and bounded I/O tests |
| 2 | Implement PayloadSource/Resolver and AttachmentSink contracts plus JSON and single-binary bodies | streaming, size limits, cancellation and no-base64 tests |
| 3 | Implement multipart/related request and response support | root-first JSON, Content-ID/RefToBinaryData binding, multiple binary parts, mixed media types, malformed and oversized part tests |
| 4 | Add trusted SBI Operation Binding Catalog loading/validation from pinned OpenAPI plus CA metadata | stable operation identity, schema/parameter validation, unknown operation rejection and no caller override of method/path/service/auth |
| 5 | Add Provider catalog/invoke subprocess contract and descriptors | Skill-scoped dynamic registration, fixed executable/argv, JSON schemas, cancellation and Host policy boundary |
| 6 | Migrate the existing SMF context-query adapter to shared transport only after exact regression tests pass | preserve private token header, identity keys, AVAILABLE/UNAVAILABLE, ProblemDetails and echoed-key behavior |
| 7 | Add the first source-verified multipart SBI operation when its deployment contract is available | request/response fixture tests plus separate OpenAMSM source and live acceptance |
| 8 | Add the operator-only raw request CLI extension, if still needed | disabled-by-default behavior and proof it is never exposed as an Agent Tool |

The existing AMF/SMF metrics clients remain separate and continue to use
their own source-verified HTTP contracts. No UPF session-diagnostic client is
implemented. Do not force metrics endpoints through the SBI Operation Binding Catalog.

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
  Invocation-scoped and streaming; OwnerArtifactRefs remain Owner-managed.
  Staging paths are root-confined and no base64 payload is exposed to the model.
- Provider catalog/invoke calls use a fixed executable and argv; the generic
  invoke command itself is never Agent-visible.
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

# CA Client Tools SBI Client Architecture

Status: **DESIGN ACCEPTED — IMPLEMENTATION NOT STARTED**  
Architecture baseline: **SBI Client v1**

This document is the normative design for the reusable SBI client, the
Operation Catalog, the CA-Agent Tool Provider contract, and binary/multipart
payload handling. It extends the existing read-client and CLI v1 baseline; it
does not claim that the new runtime components have been implemented.

## 1. Decision

CA Client Tools is not read-only by definition. It provides reusable network
client capabilities for both read and mutation operations, with each
individual operation explicitly described, registered, and authorized.

The repository owns:

- a reusable Go SBI HTTP/2 client and request executor;
- a trusted, versioned Operation Catalog derived primarily from 3GPP OpenAPI;
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

The Host dynamically activates operation descriptors for the active Skill.
For an invocation, it resolves BinaryRef values in its own Artifact Store and
stages them into a private, invocation-scoped directory. It passes only
Host-generated relative file names to the Go process. The process streams
those files into the SBI request and writes response attachments into a
separate Host-created output directory. The Host ingests those files into its
Artifact Store, issues new BinaryRef values and removes the staging directory.

This keeps the language boundary explicit, avoids base64 and avoids a second
persistent artifact store in ca-client-tools. The Host must use a fixed
executable and argument vector, never a shell.

The Host dynamically activates the operation tools needed by the currently
loaded Skill. The Operation Catalog and executors are trusted package content;
the Skill supplies only declared Capability/operation references and typed
arguments.

~~~text
ca-skills
  declares required Capabilities
          |
          v
ca-agent Host
  resolves the active Skill and deployment policy
  selects matching descriptors from the preinstalled Tool Provider
          |
          v
ca-runtime
  admits the callable and authorizes its Capability
          |
          v
ca-client-tools Tool Provider
  validates operation identity and arguments
  resolves the trusted Operation Catalog entry
          |
          v
SBI Executor
  h2c HTTP/2 transport + trusted OpenAMSM query-token header + Payload Engine
          |
          v
AMF / SMF / UDM / PCF / AUSF / NRF / other configured NF services
~~~

The Provider process exposes two machine-oriented operations: a catalog
request that returns trusted Tool descriptors for a supplied Capability set,
and an invocation request for one fixed operation ID. The Host converts the
returned descriptors into its native Agent tools. The generic
invocation command is never itself exposed as an Agent tool.

The process contract uses JSON for operation arguments and metadata, plus
Host-created input/output staging directories for binary data. The Host
controls the paths through trusted process configuration; the Skill cannot
select a path or directory.

Ownership is deliberately split:

| Component | Owns |
|---|---|
| ca-client-tools | SBI transport, Operation Catalog, request/response handling, Payload Engine, Tool Provider implementation and local CLI |
| ca-agent | Preinstalling/integrating the Provider, active-Skill tool registration, trusted configuration injection, Host policy and BinaryRef resolution |
| ca-runtime | Callable admission, Capability authorization, execution identity, Procedure progression, Outcome, Evidence and Reconcile |
| ca-skills | Business Procedure and declared callable dependencies |
| env-openamsm | OpenAMSM source, build, deployment, NF lifecycle and deployment-specific source overlays |

CA Client Tools does not own Skill selection, authorization decisions, Runtime
state, pending-operation lifecycle, business orchestration, Evidence, Outcome,
or Reconcile. The Host/Runtime contracts remain authoritative for those
concerns.

## 3. Operation Catalog

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

The CA Operation Catalog adds only metadata that is not owned by the 3GPP
OpenAPI document, for example:

- the stable CA Capability binding and public Tool identity;
- the configured service key used to resolve the destination;
- read/mutation effect classification;
- private OpenAMSM header-token selection by trusted deployment configuration;
- operation-specific idempotency/retry constraints;
- bindings between OpenAPI multipart parts, BinaryRef arguments and
  RefToBinaryData fields;
- Host visibility classification and bounded resource limits.

Do not maintain a second hand-written copy of method, path or JSON schema when
the pinned OpenAPI definition already supplies it. OpenAMSM-private interfaces,
including the existing MAS context-query API, use separately identified
private operation definitions and must not be mislabeled as standardized
3GPP operations.

### 3.2 Trusted definitions, dynamic registration

Operation definitions are trusted, versioned package content. They may be
generated from pinned OpenAPI files and completed by reviewed CA metadata.
They are not authored or uploaded by a Skill at invocation time.

Dynamic registration means:

1. the Skill declares the Capabilities it requires;
2. the Host resolves those declarations against installed Provider packages
   and the trusted Operation Catalog;
3. the Host applies deployment availability and policy;
4. only the matching Tool descriptors are activated for that Skill;
5. Runtime admission and Capability authorization still apply to each call.

A Skill must not supply or override an arbitrary URL, HTTP method, path,
header-token profile, raw header, content type, or Operation
Manifest. Unknown, unavailable, unregistered or unauthorized operations fail
closed.

The Provider may expose one Tool per meaningful 3GPP service operation. A Tool
must not hide multiple independently meaningful 3GPP request/response actions
inside a single opaque workflow. This preserves the CA-SL principle that a
Procedure Node corresponds to one meaningful network action.

### 3.3 Operation metadata

The minimum reviewed metadata for an operation is:

| Field | Meaning |
|---|---|
| operation identity | Stable CA operation/Capability ID and source operationId |
| API source | 3GPP release/document or explicitly named private OpenAMSM contract |
| service key | Trusted service entry in the deployment configuration |
| effect | read or mutation |
| schemas | Request and response schemas from the pinned API definition |
| payload bindings | JSON, binary or multipart part mappings, when applicable |
| retry contract | No retry by default; explicit safe retry/idempotency rules only |
| evidence | 3GPP_DEFINED, SOURCE_VERIFIED, CLIENT_IMPLEMENTED, FIXTURE_TESTED, SOURCE_TESTED and LIVE_ACCEPTED as applicable |

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

## 6. Payload model and BinaryRef

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

### 6.2 BinaryRef ownership

BinaryRef is an opaque reference to bytes managed by the CA-Agent Host. The
Host owns:

- storage and durable/temporary retention policy;
- access control and invocation/Skill scoping;
- expiry and cleanup;
- content length, media type and integrity metadata;
- authorization to read or persist an attachment.

CA Client Tools does not create a second artifact database or own BinaryRef
lifecycle. Its Go process implements internal PayloadResolver and
AttachmentSink abstractions over the invocation's trusted staging directories.
The Host resolves each BinaryRef and creates the input files; the Go process
reads them as streams. The Go process writes response attachments to the
output directory; the Host ingests them and returns new BinaryRef values.

The process input includes a mapping from typed binary arguments to relative
file names, not an absolute path. The process receives the trusted input root
and output root through Host-controlled environment configuration. It rejects
absolute paths, path traversal, symlinks and files outside the configured
roots. The Host creates private per-invocation directories, enforces quotas,
and cleans them after success, failure or cancellation.

Illustrative Tool input:

~~~json
{
  "n1Payload": {
    "type": "binary-ref",
    "ref": "artifact://invocation/n1-message-001"
  },
  "n2Payload": {
    "type": "binary-ref",
    "ref": "artifact://invocation/n2-message-001"
  }
}
~~~

The reference is illustrative. The Host validates that a reference belongs
to the active invocation and caller before opening it. The Client receives
only the stream and trusted metadata needed for the operation. Response
attachments are streamed to the Host AttachmentSink and returned to the
caller as new opaque references plus safe metadata (media type, size, digest,
and Content-ID where applicable).

The CLI may support operator-supplied local files for diagnostics and tests.
A local filesystem path is never an Agent-authored BinaryRef and must never be
accepted as an unrestricted Agent Tool argument.

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

The Operation Catalog maps OpenAPI multipart part names to typed Tool
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
commands. Its conceptual operations are:

~~~text
ca-client-tools provider catalog
ca-client-tools provider invoke --operation-id <trusted-operation-id>
~~~

The catalog command accepts one bounded JSON document on stdin:

~~~json
{
  "capabilities": [
    "ca::sbi.namf-comm.n1-n2-message-transfer"
  ]
}
~~~

It returns trusted Tool descriptors. Each descriptor contains a stable Tool
name, description, Capability identity, operation identity, effect
(read/mutation), input schema and output schema. It does not expose the
remote URL, HTTP method, path template or configured token value. The Host
applies deployment policy and Runtime admission before registering any
descriptor in the Python Agent runtime.

~~~json
{
  "data": {
    "tools": [
      {
        "name": "sbi.namf-comm.n1-n2-message-transfer",
        "operationId": "N1N2MessageTransfer",
        "capability": "ca::sbi.namf-comm.n1-n2-message-transfer",
        "effect": "mutation",
        "description": "Transfer N1 and/or N2 information to the serving AMF",
        "inputSchema": {},
        "outputSchema": {}
      }
    ]
  }
}
~~~

The invoke command receives one operation ID selected by the Host from the
trusted catalog. The Host passes the operation ID as a fixed argument; it
must not forward an Agent-authored command line. Its bounded JSON stdin
separates ordinary JSON arguments from binary files staged by the Host:

~~~json
{
  "arguments": {
    "ueContextId": "..."
  },
  "payloads": {
    "n1Payload": {
      "relativePath": "request/part-0001.bin",
      "size": 1024
    },
    "n2Payload": {
      "relativePath": "request/part-0002.bin",
      "size": 2048
    }
  }
}
~~~

The content type and multipart part binding are taken from the trusted
Operation Catalog, not from the Skill or the process request. The Host may
check its stored BinaryRef media-type metadata against the catalog before
staging the file.

The Agent-facing schema may contain BinaryRef values. Before process launch,
the Host resolves each reference in its Artifact Store and stages the bytes
under the invocation's private payload root. The process request contains
only Host-generated relative file names; it does not need the original
artifact:// reference. The Go process uses the trusted Operation Catalog
binding to associate each staged file with its OpenAPI multipart part and
populate the corresponding RefToBinaryData Content-ID in the JSON root.

The process returns one bounded JSON document. The JSON response is returned
as data; binary response parts are written to the Host-created output root and
reported separately:

~~~json
{
  "data": {
    "cause": "N1_N2_TRANSFER_INITIATED"
  },
  "attachments": [
    {
      "contentId": "n2-response-001",
      "relativePath": "part-0001.bin",
      "contentType": "application/vnd.3gpp.ngap",
      "size": 4096,
      "sha256": "..."
    }
  ]
}
~~~

Each output relativePath is resolved under CA_CLIENT_ATTACHMENT_ROOT. The
Host verifies the path remains inside that root, ingests the attachment into
its Artifact Store, creates an opaque BinaryRef and projects that reference
into the Agent-visible Tool result according to the output schema. Remote
filenames are never used as local paths.

The process uses the same one-document stdout / one-document stderr
discipline as CLI v1, with a documented nonzero exit code for operational
failure. Host cancellation terminates the child and aborts its HTTP request;
partial stdout and attachment files are discarded. The Host supplies trusted process configuration through environment variables:
CA_CLIENT_CONFIG_FILE, CA_CLIENT_PAYLOAD_ROOT and
CA_CLIENT_ATTACHMENT_ROOT. CA_CLIENT_PAYLOAD_ROOT contains the Host-staged
request files (for example, request/part-0001.bin); CA_CLIENT_ATTACHMENT_ROOT
is the separate destination for response attachments. These paths are created
and controlled by the Host; they are not accepted from Skill arguments. The
input and output roots are separate and scoped to one invocation.

The Host creates a private directory per invocation, uses restrictive
permissions, rejects symlink and path traversal, enforces per-part and aggregate
quotas, discards partial results after cancellation, and always cleans up.

The Go process reads and writes streams; it does not load whole binary
payloads into memory. The process contract must not accept arbitrary
filesystem paths, raw credentials, URLs or HTTP settings from the Skill.

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
| 4 | Add trusted Operation Catalog loading/validation from pinned OpenAPI plus CA metadata | stable operation identity, schema/parameter validation, unknown operation rejection and no caller override of method/path/service/auth |
| 5 | Add Provider catalog/invoke subprocess contract and descriptors | Skill-scoped dynamic registration, fixed executable/argv, JSON schemas, cancellation and Host policy boundary |
| 6 | Migrate the existing SMF context-query adapter to shared transport only after exact regression tests pass | preserve private token header, identity keys, AVAILABLE/UNAVAILABLE, ProblemDetails and echoed-key behavior |
| 7 | Add the first source-verified multipart SBI operation when its deployment contract is available | request/response fixture tests plus separate OpenAMSM source and live acceptance |
| 8 | Add the operator-only raw request CLI extension, if still needed | disabled-by-default behavior and proof it is never exposed as an Agent Tool |

The existing AMF/SMF metrics clients remain separate and continue to use
their own source-verified HTTP contracts. No UPF session-diagnostic client is
implemented. Do not force metrics endpoints through the SBI Operation Catalog.

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
- BinaryRef resolution and response attachment storage are Host-owned,
  invocation-scoped and streaming; staging paths are root-confined and no
  base64 payload is exposed to the model.
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

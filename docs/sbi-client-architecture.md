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
standardized SBI operations. They remain separate typed HTTP clients. UPF N4
uses PFCP, not SBI, and must not be routed through the SBI HTTP/2 client.

## 2. Ownership and invocation flow

The Tool Provider is preinstalled by CA-Agent as an in-process Go package
dependency. It is not a separate HTTP/MCP daemon. The Host supplies trusted
configuration, the active-Skill context, a BinaryRef PayloadResolver and an
AttachmentSink through the Provider API. Preinstalled means the implementation
is available to the Host; it does not mean every operation is automatically
visible to every Agent.

This in-process boundary lets the Host stream BinaryRef content directly to
the Client without serializing bytes through CLI JSON, base64 or an
unrestricted filesystem path. The CLI remains a separate operator/test
interface and is not the production binary Tool invocation path.

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
  HTTP/2 transport + auth/TLS profiles + Payload Engine
          |
          v
AMF / SMF / UDM / PCF / AUSF / NRF / other configured NF services
~~~

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
- authentication profile selection by trusted deployment configuration;
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
authentication profile, TLS profile, raw header, content type, or Operation
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

### 5.1 Go HTTP/2 client

The standardized SBI transport is implemented in Go and supports:

- HTTP/2 connection reuse and bounded connection management;
- TLS server verification and optional mutual TLS;
- operation-scoped OAuth2 client-credentials authentication;
- request context cancellation and bounded timeouts;
- bounded request and response sizes;
- declared 3GPP headers and API-specific headers;
- structured status, response-header and ProblemDetails handling.

For standard SBI calls, the client must use HTTP/2 as required by the
applicable 3GPP contract. It must not silently downgrade to HTTP/1.1. Any
cleartext HTTP/2 test mode must be explicit deployment configuration and must
not be enabled implicitly. Existing private OpenAMSM adapters retain their
source-specific behavior until migration to the shared transport is verified.

Do not inherit ambient proxy settings by default, follow redirects, accept
absolute request URLs from callers, or allow operation arguments to override
the configured destination. Enforce bounded timeouts, response sizes,
connection lifetimes and cancellation.

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
    origin: https://amf.sbi.example:7777
    apiRoot: /namf-comm/v1
    tlsProfile: plmn-mtls
    authProfile: namf-oauth

  smf-mas-query:
    origin: http://smf:7777
    authProfile: openamsm-query-token

tlsProfiles:
  plmn-mtls:
    caFile: /etc/ca/certs/plmn-ca.pem
    certFile: /etc/ca/certs/client.pem
    keyFile: /etc/ca/secrets/client.key

oauthProfiles:
  namf-oauth:
    tokenURL: https://nrf.sbi.example:443/oauth2/token
    clientID: ca-agent
    clientSecretFile: /etc/ca/secrets/namf-client-secret
    scopes:
      - namf-comm

headerTokenProfiles:
  openamsm-query-token:
    header: X-OpenAMSM-Query-Token
    tokenFile: /etc/ca/secrets/openamsm-query-token

limits:
  requestTimeout: 10s
  maxRequestBytes: 16777216
  maxResponseBytes: 16777216
  maxMultipartParts: 32
~~~

This is an architectural example, not a claim that these endpoint names,
credentials or scopes are deployed. The exact values come from the deployment.

Configuration responsibilities:

- service entries provide static origins and API roots;
- TLS profiles provide trust roots, client certificates and server identity;
- OAuth profiles provide token endpoint, client identity, secret reference and
  scopes; the client manages token acquisition, caching and refresh;
- private OpenAMSM authentication may use a narrowly defined header-token
  profile, such as the existing query token;
- secrets are supplied by secret files or the Host secret facility, not
  embedded in Skill arguments or Operation Manifests.

Do not place plaintext secrets in committed configuration. Do not let a Skill
select or modify a service origin, TLS profile, OAuth profile or token.

## 6. Payload model and BinaryRef

### 6.1 Supported payload classes

The Payload Engine supports the media types declared by an operation's
OpenAPI contract, including:

1. JSON request/response bodies;
2. a single opaque binary request/response body with its declared media type;
3. multipart messages containing a JSON root and one or more binary parts.

Examples of 3GPP binary media types include
application/vnd.3gpp.5gnas and application/vnd.3gpp.ngap. The implementation
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
lifecycle. Its Provider API accepts a Host-supplied PayloadResolver for
request references and an AttachmentSink for response parts. These interfaces
must stream data; they must not require the full payload to be copied into
memory.

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

## 7. Curl-like raw request mode

A curl-like request facility is useful for operator debugging and protocol
bring-up. It is not the Agent execution model.

A future CLI extension may provide an operator-only command with:

- a configured service key;
- HTTP method and a relative path under that service's configured API root;
- JSON stdin, a single local binary file, or explicitly declared multipart
  file parts;
- response status, safe headers, JSON body and attachment metadata.

It must not accept an absolute URL, override the trusted service origin,
override TLS/OAuth credentials, bypass network allowlists, or become a
Tool Provider descriptor. Raw mode is disabled by default and requires an
explicit local operator configuration switch.

The existing CLI v1 command tree and process contract remain unchanged by
this architecture decision. Raw request mode is not part of CLI v1 and must
be specified and tested separately before implementation.

## 8. Asynchronous responses and protocol boundaries

The SBI Client performs one operation invocation and returns its actual HTTP
response, including status, declared headers and payloads. For example, a
202 Accepted response remains a 202 result with its correlation/Location
information. The Client does not wait for a notification, poll a resource,
register a callback, resume a Procedure, or maintain a pending-operation
journal.

Host/Runtime own pending-operation correlation, later inbound message
admission, continuation and Procedure resumption. This keeps the Client
consistent with the CA-SL execution model and the Host's lifecycle ownership.

PFCP/N4 is a separate protocol. The existing UPF /session-info diagnostic
projection is a private HTTP metrics endpoint over PFCP-owned state; it is
not an SBI operation. Any future PFCP client must be a separate protocol
client, not a mode inside the SBI HTTP/2 executor.

## 9. Implementation sequence

The following work is ordered to preserve the existing CLI v1 and typed
read-client baseline.

| Step | Work | Acceptance |
|---|---|---|
| 1 | Extract reusable Go HTTP/2 transport and trusted service/TLS/auth configuration | HTTP/2 negotiation, connection reuse, cancellation, TLS verification, OAuth token refresh, private header-token profile and bounded I/O tests |
| 2 | Implement PayloadSource/Resolver and AttachmentSink contracts plus JSON and single-binary bodies | streaming, size limits, cancellation and no-base64 tests |
| 3 | Implement multipart/related request and response support | root-first JSON, Content-ID/RefToBinaryData binding, multiple binary parts, mixed media types, malformed and oversized part tests |
| 4 | Add trusted Operation Catalog loading/validation from pinned OpenAPI plus CA metadata | stable operation identity, schema/parameter validation, unknown operation rejection and no caller override of method/path/service/auth |
| 5 | Add CA-Agent Tool Provider descriptors and invocation API | Skill-scoped dynamic registration, Host policy boundary, Capability identity and operation-level read/mutation metadata |
| 6 | Migrate the existing SMF context-query adapter to shared transport only after exact regression tests pass | preserve private token header, identity keys, AVAILABLE/UNAVAILABLE, ProblemDetails and echoed-key behavior |
| 7 | Add the first source-verified multipart SBI operation when its deployment contract is available | request/response fixture tests plus separate OpenAMSM source and live acceptance |
| 8 | Add the operator-only raw request CLI extension, if still needed | disabled-by-default behavior and proof it is never exposed as an Agent Tool |

The existing AMF/SMF metrics and UPF diagnostic clients remain separate and
continue to use their own source-verified HTTP contracts. Do not force them
through the SBI Operation Catalog.

## 10. Required acceptance gates

- Standard SBI calls negotiate HTTP/2 and reject unintended protocol downgrade.
- TLS verification and optional mutual TLS work with the configured profile.
- OAuth token acquisition, caching, expiry and refresh work without exposing
  secrets; the private OpenAMSM token header remains isolated to its profile.
- Only registered operations can execute; Skill arguments cannot override
  method, path, origin, headers, auth or TLS.
- Read and mutation effect metadata is preserved; mutations are not retried
  without an explicit safe retry contract.
- JSON, single-binary and multipart request/response paths are tested.
- Multipart tests cover one and multiple binary parts, correct media types,
  root-first ordering, Content-ID references, duplicate/missing references,
  malformed boundaries, truncation, byte limits and cancellation.
- BinaryRef resolution and response attachment storage are Host-owned,
  invocation-scoped and streaming; no base64 payload is exposed to the model.
- A 202 response is returned as a response; no pending-operation lifecycle is
  created inside the Client.
- Mock tests, pinned source tests/build and live-NF acceptance are reported as
  separate evidence levels.

## 11. Standards references

- 3GPP TS 29.500, clause 5.4, Serialization Protocol:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129500/18.08.00_60/ts_129500v180800p.pdf
- 3GPP TS 29.518, clause 6.1.2.4, HTTP multipart messages:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129518/18.07.00_60/ts_129518v180700p.pdf
- 3GPP TS 29.501, clause 5.3, OpenAPI specification files:
  https://www.etsi.org/deliver/etsi_ts/129500_129599/129501/18.06.00_60/ts_129501v180600p.pdf
- Official 3GPP 5G APIs repository:
  https://forge.3gpp.org/rep/all/5G_APIs

The applicable release and document revision must be pinned per operation.
These links establish the design references; they do not establish that a
particular OpenAMSM deployment implements a given operation.

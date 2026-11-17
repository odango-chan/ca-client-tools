# CA Client Tools architecture

Status: **PR #1 read-client/CLI, PR #2 h2c transport and PR #3 JSON/binary/multipart Payload Engine are merged. PR #5 is a Draft documentation PR freezing the first JSON-only SMF Provider profile; Provider runtime and Host dispatch remain unimplemented.**

## 1. Purpose

CA Client Tools provides the network client capabilities used by CA-Agent.
Its target architecture includes a reusable Go SBI HTTP/2 client, a trusted
Operation Catalog, a CA-Agent Tool Provider, OpenAMSM-specific adapters and a
stable local CLI.

The repository is not an Agent, Runtime, workflow engine, Provider business
service or OpenAMSM environment manager. Its transport and payload engine are
designed to support individual 3GPP SBI operations, including read and
mutation operations, without making arbitrary HTTP requests available to the
Agent.

The current deployment/source target remains OpenAMSM. Generic SBI transport
does not imply Open5GS compatibility or prove that every 3GPP API is
implemented by OpenAMSM.

Deployment targets are Windows-native, Linux-native, Docker, Kubernetes and
remote 5GC/NF deployments. WSL is not a product dependency or a deployment
target; it appears only in the optional Windows-native acceptance tooling.
Product code and configuration contain no WSL detection, WSL-specific
configuration fields or WSL-specific network handling.

The detailed normative design is
[docs/sbi-client-architecture.md](sbi-client-architecture.md).

## 2. Invocation and ownership

~~~text
Skill declares Capability
          |
          v
ca-agent Host
  resolves active Skill and deployment policy
  dynamically activates matching descriptors from preinstalled Provider
          |
          v
ca-runtime
  callable admission + Capability authorization
          |
          v
ca-client-tools Tool Provider
  trusted Operation Catalog + typed arguments
          |
          v
SBI Executor
  h2c HTTP/2 + trusted OpenAMSM query-token header + JSON/Binary/Multipart Payload Engine
          |
          v
Configured NF service
~~~

| Component | Owns |
|---|---|
| env-openamsm | OpenAMSM source, build, configuration, deployment and NF lifecycle |
| ca-client-tools | SBI transport, Operation Catalog, Tool Provider implementation, Payload Engine, typed OpenAMSM clients and CLI |
| ca-agent | Preinstalled Provider integration, active-Skill tool registration, trusted configuration, Host policy and BinaryRef storage/resolution |
| ca-runtime | Callable admission, Capability authorization, execution identity, Procedure progression, Outcome, Evidence and Reconcile |
| ca-skills | Business Procedure and declared callable dependencies |

The Provider is a preinstalled Go executable in the CA-Agent deployment. The
current Python Demo Host invokes it through a fixed, machine-readable
subprocess contract; the contract is language-neutral.
The Host activates only the operations required by the active Skill and
permitted by deployment policy. For binary payloads, the Host resolves
BinaryRef values into private invocation-scoped input files and ingests
response attachment files from a private output directory. Runtime admission
and Capability authorization remain mandatory. The CLI remains the local
operator/test interface.

## 3. Operation Catalog

Standardized SBI operation definitions are derived from a pinned,
release-specific 3GPP OpenAPI source. OpenAPI is authoritative for each
operation's operationId, HTTP method, path, parameters, request/response
schemas, media types and status codes.

The CA-owned Provider Backend Binding Catalog is the execution-binding index:
it maps a private backend operation ID to either a pinned 3GPP OpenAPI
operationId or a source-pinned private typed handler, plus the trusted service
key and any payload mapping not already defined by OpenAPI. It must not copy
OpenAPI method/path/schema data into a second executable route registry.

The Owner-published ToolCatalog owns public callable identity, kind, Effect
and input/output schemas. Deployment CapabilityContracts own Capability
membership and Scope policy. The Provider Backend Binding Catalog does not
duplicate either authority. The Host maps an admitted public callable to the
exact private backend operation.

A Skill declares its Capability Grants and references exact callable exports;
it does not author Operation Manifests, URLs, methods, paths, headers,
credentials or payload transport settings. Dynamic registration is Host-side
selection of trusted descriptors, not dynamic execution of Skill-supplied code
or arbitrary HTTP.

OpenAMSM-private APIs such as the SMF context-query contract remain explicitly
private typed bindings. Deployment support is independently verified; a 3GPP
OpenAPI operation is not proof that the selected OpenAMSM source implements it.

See [docs/sbi-client-architecture.md](sbi-client-architecture.md) for the
catalog, registration and security rules.

## 4. Transport and payloads

The standardized SBI executor uses Go HTTP/2 with deployment-provided
static service origins and the existing private OpenAMSM query-token header
profile. The current OpenAMSM transport uses h2c (cleartext HTTP/2 prior
knowledge). The client does not implement TLS/mTLS or OAuth2; deployment
infrastructure owns any network protection outside this client.

The Payload Engine supports JSON, single opaque binary bodies and
multipart/related request and response bodies. Multipart support follows
3GPP TS 29.500 and the applicable service specification: JSON root first,
Content-ID on each binary part, and RefToBinaryData references in JSON.
Declared binary media types include 5GS NAS, NGAP and opaque PFCP payloads
where the relevant SBI operation defines them.

BinaryRef storage, access control, expiry and cleanup belong to the CA-Agent
Host. The Provider consumes Host-supplied streams and streams response
attachments back to the Host; it does not maintain a second artifact store or
base64-encode binary payloads for the model.

The curl-like raw request facility, if added, is operator-only, disabled by
default and never registered as an Agent Tool. The existing CLI v1 contract
remains unchanged.

## 5. Current client baseline

PR #1 contains:

- eight read-only OpenAMSM SMF context-query operations;
- AMF UE and gNB metrics page clients;
- SMF PDU metrics page client;
- the CLI v1 NF/resource command tree and JSON process contract;
- strict request validation and response-key binding for SUPI, PSI and UE IP;
- a typed h2c transport for the SMF SBI listener.

Real acceptance on 2026-11-17: AMF UE List, AMF gNB List, SMF PDU List and
all eight context queries PASS. The context pass follows an operator-authorized
local `env-openamsm` build overlay for the lowercase token lookup; the
imported upstream tree is unchanged. Local Go 1.26.8 test/vet/build and
compiled-CLI contract acceptance PASS.

The AMF/SMF metrics clients are not standardized SBI operations. Direct
N4/PFCP transport is separate from SBI; a standardized SBI operation may
carry an opaque PFCP message as a declared multipart payload, but this
repository does not implement a UPF session-inspection endpoint and does not
carry the server patch; the lab overlay lives in `env-openamsm`.

See [the real acceptance record](evidence/real-openamsm-acceptance-20261117.md)
and [docs/WORK.md](WORK.md).

## 6. Tool visibility

The existence of a client method, CLI command or Operation Catalog entry does
not make it Agent-callable. The Host determines the active Skill's toolset;
Runtime owns admission and Capability authorization.

Each operation is independently classified as Agent-callable or
Host/Tool-internal. Sensitive AMF metrics and raw subscriber projections
remain Host-private unless an explicit contract authorizes a safe projection.
A mutation operation is not implicitly exposed merely because its API
definition exists.

## 7. Procedure and lifecycle boundaries

A Tool represents an individual network operation. It does not implement a
multi-step 3GPP workflow, hide multiple independent request/response actions,
wait for asynchronous notifications, maintain pending-operation journals,
register callbacks, resume a Procedure, or create Runtime Evidence/Outcome.

A 202 Accepted response and its declared correlation headers are returned to
the Host. Host/Runtime own pending-operation correlation and continuation.

## 8. Source and deployment evidence

The canonical NF responsibilities and current source availability are in
[docs/nf-capability-map.md](nf-capability-map.md). Exact OpenAMSM endpoint and
wire facts are in [docs/openamsm-interface-inventory.md](openamsm-interface-inventory.md).

3GPP_DEFINED, SOURCE_VERIFIED, CLIENT_IMPLEMENTED, FIXTURE_TESTED,
SOURCE_TESTED and LIVE_ACCEPTED are separate evidence states. Do not promote a
client to source- or live-supported based only on a mock fixture or the
existence of a 3GPP specification.



## 9. Provider backend and Host-owned Observation integration

ca-client-tools supplies a resident Go Provider backend for the eight private
OpenAMSM SMF context-query routes. The process boundary is
[Provider Process Protocol v1](provider-protocol-v1.md); the typed backend and
route mapping are defined in
[OpenAMSM SMF Query Backend v1](smf-query-backend-contract-v1.md).

The public CA-SL callable Owner is ca-agent-host, not ca-client-tools. The Host
owns one generic SC-backed session-presence Observation
(context.smf.session.read) and eight optional projection-specific Observation
callables. All nine public callables use the ca-client-tools resident Provider.
The generic callable and `smf.mas.sc.context.query` share the private
`openamsm.smf.mas.sc.query` backend operation but retain distinct Host output
Schemas and normalization. The generic public name omits `mas` and
`openamsm`; only projection-specific public names include `mas`. Their
ToolCatalog, normalized Schemas, Owner Outcome mapping and Evidence Producer
are Host-owned proposals in
[ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208).
The Host/Runtime v1alpha4 stage and unified callable dispatch contract is in
[PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).

The Provider catalog lists private backend operation IDs only. It is not a
second ToolCatalog and does not authorize a callable. The Host adapter maps
an admitted Host-owned callable to the exact backend operation after
Capability/Scope checks.

The first SMF Observation delivery is JSON-only because Provider Process
Protocol v1 is frozen for that narrow slice. This is not a limit on the general
SBI architecture: the shared Payload Engine already supports JSON, opaque
binary and multipart/related request/response bodies, and the architecture
permits individually catalogued READ and MUTATION operations. Binary Provider
calls require a separately versioned payload-transfer extension using
Host-owned HostArtifactRef staging; the first source-verified live binary
operation remains NOT_RUN. The source v0.10 response has no observedAt;
Evidence freshness is unresolved and Evidence-dependent PASS remains
NOT_QUALIFIED.

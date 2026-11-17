# CA Client Tools architecture

Status: **Read-client/CLI baseline implemented on PR #1; AMF/SMF list surfaces and all eight SMF context queries live-accepted after an authorized local token-lookup overlay. Shared h2c SBI transport and Payload Engine remain on stacked PRs #2/#3.**

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

The detailed SBI transport and payload design is
[docs/sbi-client-architecture.md](sbi-client-architecture.md). The cross-repository
Capability / callable / Provider identity and authority split is specified in
[docs/sbi-capability-provider-contract.md](sbi-capability-provider-contract.md).

## 2. Invocation and ownership

~~~text
Skill declares Capability
          |
          v
ca-agent Host
  resolves Skill callable refs via ToolCatalog + CapabilityContracts
  checks Provider SBI Operation Binding Catalog
  activates only the callable subset used by this Skill
          |
          v
ca-runtime
  callable admission + Capability authorization
          |
          v
ca-client-tools Tool Provider
  pinned SBI Operation Binding Catalog + typed arguments
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
| ca-client-tools | SBI transport, SBI Operation Binding Catalog, Tool Provider implementation, Payload Engine, typed OpenAMSM clients and CLI |
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
release-specific 3GPP OpenAPI source. The SBI Operation Binding Catalog maps
exact ToolCatalog callable identities to pinned OpenAPI operationIds and
trusted service keys. It does not define Capability membership, Effect,
Schemas or Scope.

Deployment CapabilityContracts separately maps Capability IDs to exact
callables and Scope policies. The Host resolves Skill grants against those
contracts, verifies Provider support, and activates only the exact callables
used by the admitted Skill. Dynamic registration is Host-side Tool exposure,
not dynamic execution of Skill-supplied code or arbitrary HTTP.

The Skill does not author Operation Bindings, URLs, methods, paths, headers,
service keys or credentials.

OpenAMSM-private APIs such as the MAS context-query contract remain explicitly
private definitions. Deployment support is independently verified; a 3GPP
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

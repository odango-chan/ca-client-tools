# CA Client Tools

CA Client Tools is the network client and local tool-provider package used by
CA-Agent. Its target architecture combines a reusable Go SBI HTTP/2 client,
a trusted Operation Catalog, a CA-Agent Tool Provider, typed OpenAMSM adapters
and a stable local CLI.

The transport and payload engine are designed for individual 3GPP SBI
operations, including read and mutation operations. The current deployment
target remains OpenAMSM; this is not an Open5GS compatibility layer and does
not claim that every standardized 3GPP API is available in OpenAMSM.

## Responsibilities

- Provide reusable SBI HTTP/2 transport and trusted TLS/authentication profiles.
- Describe concrete operations using pinned 3GPP OpenAPI definitions plus
  reviewed CA metadata.
- Supply a Tool Provider that CA-Agent can preinstall and dynamically activate
  for the currently loaded Skill.
- Support JSON, opaque binary and multipart/related request and response
  payloads through Host-owned BinaryRef streams.
- Preserve the existing typed OpenAMSM clients and local CLI.
- Keep Runtime authorization, Host policy, business workflow and Evidence
  ownership outside this repository.

## Ownership boundaries

| Component | Owns |
|---|---|
| env-openamsm | OpenAMSM source snapshot, build, configuration, deployment and NF lifecycle |
| ca-client-tools | SBI client, Operation Catalog, Tool Provider implementation, Payload Engine, typed OpenAMSM clients and CLI |
| ca-agent | Host integration, preinstalled Provider activation, active-Skill tool registration, trusted configuration, policy and BinaryRef storage/resolution |
| ca-runtime | Callable admission, Capability authorization, execution, Outcome, Evidence and Reconcile |
| ca-skills | Business Procedures and declared callable dependencies |

The Provider is a preinstalled Go executable in the CA-Agent deployment, but
not all catalog operations are automatically exposed. The Python Host selects
the operation descriptors required by the active Skill and invokes the
Provider through a fixed subprocess protocol. Runtime admission and
Capability authorization remain mandatory. For binary calls, the Host stages
its BinaryRef content into private invocation-scoped files and ingests
response attachments from a private output directory; no base64 payload is
passed to the model. A Skill cannot supply arbitrary URLs, HTTP methods,
paths, authentication settings or Operation Manifests.

## Architecture

See [docs/architecture.md](docs/architecture.md) for repository boundaries and
[docs/sbi-client-architecture.md](docs/sbi-client-architecture.md) for the
normative SBI, dynamic registration, configuration, BinaryRef and
multipart/related design.

The initial transport uses static service endpoints and trusted configuration
files. NRF dynamic service discovery is deferred; it is not required to use the
client. OAuth2, TLS/mTLS and the existing private OpenAMSM query-token
mechanism are configured through separate profiles. Secrets are referenced
from secret files or Host secret facilities, not Skill arguments.

## Current OpenAMSM client baseline

The current review branch contains:

- AMF metrics: GET /ue-info and GET /gnb-info.
- SMF metrics: GET /pdu-info.
- SMF context query: eight read-only OpenAMSM SBI endpoints exposed through
  stable CA context types: session, policy, up-node, signaling-session,
  ip-address, up-selection, charging and session-auth.
- UPF GET /session-info: a CA-owned patch overlay in env-openamsm; this is a
  private HTTP metrics projection over PFCP-owned state, not an SBI operation.

The private OpenAMSM MAS context-query adapter uses its v0.10 wire contract:
X-OpenAMSM-Query-Token, typed query keys, AVAILABLE / UNAVAILABLE and
ProblemDetails errors. The UPF diagnostic endpoint is not present in the
immutable imported source; patch application, native build and live-N4
acceptance remain separate gates.

See [docs/nf-capability-map.md](docs/nf-capability-map.md) and
[docs/openamsm-interface-inventory.md](docs/openamsm-interface-inventory.md)
for exact evidence and endpoint status.

## CLI v1

The current CLI v1 contract is in [docs/cli-contract.md](docs/cli-contract.md).
It remains a fixed, non-interactive process interface with JSON stdin/stdout/
stderr and stable exit codes. Its current command tree is:

~~~text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <TYPE>
ca-client-tools upf session list --page N --page-size N
~~~

The existing CLI v1 commands are not a generic raw HTTP tool. A future
curl-like request mode, if added, is operator-only, disabled by default and
never exposed as an Agent Tool.

## Binary and multipart boundary

BinaryRef values are opaque Host-managed references, not base64 payloads.
CA-Agent owns storage, access control, expiry and cleanup. CA Client Tools
resolves request streams through a Host-provided interface and streams response
attachments back to a Host-provided sink.

For 3GPP multipart/related, the JSON root is first; each binary part has a
Content-ID; JSON references the part through RefToBinaryData. One or more
binary parts and their media types are defined per operation's pinned OpenAPI
contract. N1/N2 is an example, not a hard-coded limit.

## Explicit boundaries

- No Open5GS-specific patch compatibility layer.
- No arbitrary Agent-facing HTTP request tool.
- No Agent-facing MCP server or arbitrary shell execution.
- No Provider Outcome, Predicate verdict, Runtime Run state or Evidence
  authority.
- No multi-step NF workflow, callback ownership, pending-operation journal or
  Procedure resumption.
- PFCP/N4 remains a separate protocol from SBI.
- The repository does not edit the immutable imported OpenAMSM source tree.

## Status

- Existing typed OpenAMSM clients and CLI v1 are implemented on the review
  branch; Go 1.26.2 CI acceptance is pending.
- The generic SBI Client, Operation Catalog, Tool Provider and Payload Engine
  are design-accepted but not yet implemented.
- BinaryRef integration requires the corresponding Host-owned resolver and
  attachment-sink contract in ca-agent.
- The SBI architecture implementation and acceptance sequence is tracked in
  [docs/WORK.md](docs/WORK.md) and
  [docs/DEEPSEEK-HANDOFF.md](docs/DEEPSEEK-HANDOFF.md).

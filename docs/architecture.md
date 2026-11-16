# CA Client Tools architecture

Status: **Current read-client/CLI baseline implemented on the review branch;
SBI Client v1 architecture accepted, implementation not started.**

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
  HTTP/2 + TLS/OAuth profiles + JSON/Binary/Multipart Payload Engine
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

The Provider is preinstalled, but its complete operation catalog is not
automatically exposed. The Host activates only the operations required by the
active Skill and permitted by deployment policy. Runtime admission and
Capability authorization remain mandatory.

## 3. Operation Catalog

Standardized SBI operation definitions are derived from a pinned,
release-specific 3GPP OpenAPI source. A small CA metadata layer binds each
operation to its Capability, service configuration key, read/mutation effect,
idempotency rules and optional payload bindings.

The Skill references declared Capabilities; it does not author Operation
Manifests, URLs, methods, paths, headers, auth profiles or TLS profiles.
Dynamic registration is Host-side selection of trusted operation descriptors,
not dynamic execution of Skill-supplied code or arbitrary HTTP.

OpenAMSM-private APIs such as the MAS context-query contract remain explicitly
private definitions. Deployment support is independently verified; a 3GPP
OpenAPI operation is not proof that the selected OpenAMSM source implements it.

See [docs/sbi-client-architecture.md](sbi-client-architecture.md) for the
catalog, registration and security rules.

## 4. Transport and payloads

The standardized SBI executor uses Go HTTP/2 with deployment-provided
service origins, TLS profiles and authentication profiles. Static service
configuration is sufficient for the first version; NRF dynamic discovery is
not a prerequisite. OAuth2 client credentials and the existing private
OpenAMSM query-token mechanism use separate trusted profiles.

The Payload Engine supports JSON, single opaque binary bodies and
multipart/related request and response bodies. Multipart support follows
3GPP TS 29.500 and the applicable service specification: JSON root first,
Content-ID on each binary part, and RefToBinaryData references in JSON.

BinaryRef storage, access control, expiry and cleanup belong to the CA-Agent
Host. The Provider consumes Host-supplied streams and streams response
attachments back to the Host; it does not maintain a second artifact store or
base64-encode binary payloads for the model.

The curl-like raw request facility, if added, is operator-only, disabled by
default and never registered as an Agent Tool. The existing CLI v1 contract
remains unchanged.

## 5. Current client baseline

The current review branch contains:

- eight read-only OpenAMSM SMF context-query operations;
- AMF UE and gNB metrics page clients;
- SMF PDU metrics page client;
- UPF PFCP session diagnostic client based on the CA-owned env-openamsm patch
  overlay;
- the CLI v1 NF/resource command tree and JSON process contract.

The metrics clients and UPF diagnostic projection are not standardized SBI
operations. The UPF session projection reads PFCP-owned state through a
deployment-specific HTTP metrics endpoint. N4/PFCP is not part of the SBI
HTTP/2 executor.

Go 1.26.2 validation, UPF patch build and live-N4 acceptance remain pending;
see [docs/WORK.md](WORK.md).

## 6. Tool visibility

The existence of a client method, CLI command or Operation Catalog entry does
not make it Agent-callable. The Host determines the active Skill's toolset;
Runtime owns admission and Capability authorization.

Each operation is independently classified as Agent-callable or
Host/Tool-internal. Sensitive AMF/UPF metrics and raw subscriber projections
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

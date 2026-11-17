# Provider Backend and Host Callable Interoperability Contract

**Status:** FROZEN FOR IMPLEMENTATION — JSON-only SMF query backend  
**Implementation status:** Resident Provider and Host dispatch NOT IMPLEMENTED  
**Language baseline:** CA-SL connectionagent.io/v1alpha4 (FROZEN)

## 1. The public Owner and the backend are separate

The public CA-SL callable Owner is ca-agent-host, not ca-client-tools.

The public Host Owner contracts are designed in [ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208):

- generic SC-backed session-presence callable: context.smf.session.read;
- eight optional projection-specific callables: smf.mas.{sc,pa,upn,ssm,iam,ups,ca,saa}.context.query;
- Host-owned ToolCatalog, normalized Output Schemas, Owner Outcome mapping and Evidence Producer;
- deployment-owned CapabilityContracts and Scope policies;
- SMF/OpenAMSM as fact authority.

The Host Runtime integration and current code gaps are tracked in [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214). The Host Artifact Contract in [PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211) is separate and not a dependency of this JSON-only first slice.

ca-client-tools owns the typed Go client and resident Provider backend. It does not publish Agent-facing ToolCatalog callables, Capability IDs, Skill Schemas, Evidence Descriptors or Procedure routes.

## 2. Identity model

| Identity | Authority | Meaning |
|---|---|---|
| Capability ID | Deployment CapabilityContracts | Authorization grant named by the Skill |
| Public callable identity | Host-owned ToolCatalog | Exact (owner, kind, id), with owner ca-agent-host |
| Qualified callable reference | Skill Procedure | Dependency Alias plus exact Host export ID |
| Provider ID | Provider package/deployment | Installed backend executable, e.g. ca-client-tools.sbi |
| Provider backend operation ID | Provider Backend Binding Catalog | Private fixed typed client operation |
| Host adapter binding | Trusted Host package | Maps public callable identity to exact Provider backend operation |
| Invocation/producer ID | Host/Runtime | Execution and Evidence correlation identity |

Capability ID, public callable identity and Provider backend operation ID are distinct and MUST NOT be substituted for one another.

## 3. First-slice public Host callables

The public Host Owner exports are:

| Host callable | Capability ID | Provider backend |
|---|---|---|
| context.smf.session.read | deployment.smf.session-context.read | openamsm.smf.mas.sc.query |
| smf.mas.sc.context.query | deployment.smf.mas.sc.context.read | openamsm.smf.mas.sc.query |
| smf.mas.pa.context.query | deployment.smf.mas.pa.context.read | openamsm.smf.mas.pa.query |
| smf.mas.upn.context.query | deployment.smf.mas.upn.context.read | openamsm.smf.mas.upn.query |
| smf.mas.ssm.context.query | deployment.smf.mas.ssm.context.read | openamsm.smf.mas.ssm.query |
| smf.mas.iam.context.query | deployment.smf.mas.iam.context.read | openamsm.smf.mas.iam.query |
| smf.mas.ups.context.query | deployment.smf.mas.ups.context.read | openamsm.smf.mas.ups.query |
| smf.mas.ca.context.query | deployment.smf.mas.ca.context.read | openamsm.smf.mas.ca.query |
| smf.mas.saa.context.query | deployment.smf.mas.saa.context.read | openamsm.smf.mas.saa.query |

The generic session read is the required initial-presence interface for the PDU Session Skills. It normalizes the SC query into ABSENT, PRESENT+AVAILABLE or PRESENT+UNAVAILABLE. The eight MAS projections are optional compatibility interfaces for Procedures that explicitly need those facts. The generic callable is not an eight-way fan-out and does not imply a cross-MAS atomic snapshot.

Each public callable has a Host-owned ToolCatalog export and Host-owned normalized Schema/Evidence contract. These are not published by this repository.

## 4. Responsibility split

| Component | Owns | Does not own |
|---|---|---|
| ca-skills | Business intent, Capability grants, Procedure stages, typed arguments, Predicates and routes | Provider executable, HTTP paths, credentials or NF state |
| ca-agent-host | Public Host ToolCatalog, normalized Observation outputs, Owner Outcome mapping, Evidence Producer, adapter binding and Provider lifecycle | Deployment Capability grants or SMF fact authority |
| Deployment | CapabilityContracts, exact callable membership and Scope policy bodies | Provider routes, public callable schemas or SMF state |
| ca-client-tools | Typed OpenAMSM client, Provider Backend Binding Catalog, h2c transport, backend process protocol and operator CLI | Public ToolCatalog, Capability grants, Evidence, Procedure routing or HostBusinessCorrelation |
| Runtime | Callable linking/admission, Capability/Scope checks, Invocation identity, Procedure progression, Evidence, Outcome and Reconcile | SBI URL/path construction or query-client implementation |
| SMF/OpenAMSM | Query response and authoritative SMF state | Host authorization or Skill routing |

## 5. Host adapter and Provider process

The Host starts one resident ca-client-tools provider serve process. It verifies the Provider package and Backend Binding Catalog digest, then verifies support for the exact private backend operation IDs required by the active Host adapters.

The Host adapter resolves an admitted public callable through the Host-owned ToolCatalog and deployment CapabilityContracts, applies Scope to the exact materialized SUPI/PSI and HostBusinessInstance, then invokes the corresponding private backend operation. The Provider catalog is support information only; it cannot widen the Host Toolset or grant access.

The Provider returns a typed remote HTTP result or a Provider/backend error. It does not return a CA-SL Owner Outcome. The Host adapter validates the result, performs the public callable's normalization, assigns the declared Owner Outcome and attaches Evidence only when the corresponding Host Owner Evidence Descriptor is valid.

For the generic session read, only exact HTTP 404 + SESSION_NOT_FOUND becomes the successful sessionPresence: ABSENT result. HTTP 200 + UNAVAILABLE is PRESENT with an empty projection. For the eight projection-specific callables, HTTP 404 is not UNAVAILABLE and is not silently normalized into a projection result.

## 6. Authorization and no duplicate registry

- ToolCatalog Owner is ca-agent-host.
- Capability IDs and callable memberships come only from trusted deployment CapabilityContracts.
- The Skill grants Capability IDs only.
- The Host adapter binding is not a ToolCatalog, Capability grant or Agent-facing registry.
- The Provider Backend Binding Catalog is not a public callable catalog.
- Provider support never grants authorization.
- The Host MUST NOT use direct callable-ID fallback as a grant.
- The Host MUST NOT spawn the CLI once per invocation or keep a second Tool registry.
- The Provider MUST NOT create Evidence, choose routes, run a Skill, or own Runtime Outcomes.

## 7. First vertical slice and explicit deferrals

The first combined implementation includes the resident Provider plus the Host adapter/Runtime path for the generic SC session read and all eight optional projection-specific backend operations:

1. implement the Provider process protocol and private backend operation catalog;
2. bind the eight backend operations to the existing typed Go client;
3. install/start the resident Provider in ca-agent-demo;
4. publish and link the Host-owned generic and projection-specific callable contracts;
5. authorize each public callable through the exact deployment CapabilityContracts and Scope policy;
6. execute a real query through admitted Skill/Host/Runtime dispatch;
7. pass positive/negative authorization, Scope, wire-key, output, remote-error and process-lifecycle tests.

The slice is JSON-only. It does not invoke the CLI per call. HostArtifactRef, NAS/NGAP, multipart, binary transport, N1/N2 and mutation callables remain later work.

The current v0.10 SMF response lacks source observedAt and an explicit live-read freshness guarantee. Provider and Host query transport can be implemented and tested, but Evidence-dependent Predicate PASS remains NOT_QUALIFIED until the temporal gate in PR #208 closes.

## 8. Qualification boundary

This contract set is implementation-ready for the Provider/backend boundary. It does not claim that the Provider process, Host Observation stages, trusted Scope policy publication or real Host-dispatch E2E has passed.

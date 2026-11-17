# CA-SL Callable, Capability and Provider Interoperability Contract

**Status:** FROZEN FOR IMPLEMENTATION — first JSON-only SMF Observation slice  
**Implementation status:** Provider process and Host dispatch NOT IMPLEMENTED  
**Language baseline:** CA-SL `connectionagent.io/v1alpha4` (FROZEN)

Field-level process framing is owned by [Provider Process Protocol v1](provider-protocol-v1.md). The eight Owner exports and their Schemas/Evidence are owned by [SMF Context Observation Contract v1](smf-context-observation-contract-v1.md). Host/Runtime integration is owned by [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214). HostArtifactRef in [PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211) is a separate draft, not a first-slice dependency.

## 1. Identity model

| Identity | Authority | Meaning |
|---|---|---|
| Capability ID | Deployment CapabilityContracts | Authorization group granted by a Skill |
| Callable identity | ToolCatalog Owner | Exact `(owner, kind, id)` export |
| Qualified callable reference | Skill Procedure | Dependency Alias plus exact callable export ID |
| Provider ID | Provider package/deployment | Installed executable implementation, e.g. `ca-client-tools.sbi` |
| Provider binding | Provider Callable Binding Catalog | Exact callable to fixed typed Go handler/private binding |
| Invocation/producer ID | Host/Runtime | Execution and Evidence correlation identity |

The Skill's `contract.yaml` lists Capability IDs only. A Procedure uses `operation.tool` or `observation.tool` for the exact callable. Callable IDs are never Capability grants. `kind` is part of identity; Operation and Observation exports cannot substitute for each other.

## 2. Ownership and responsibility

| Component | Owns | Does not own |
|---|---|---|
| `ca-skills` | Business intent, Capability grants, Procedure stages, typed arguments, Predicates and routes | Provider selection, HTTP paths, credentials, transport or NF state |
| ToolCatalog Owner | Exact callable identity/kind, Effect, Owner Outcomes, Input/Output or QueryContext Schemas, Evidence guarantees | Deployment grants or client transport |
| Deployment CapabilityContracts | Capability IDs, exact callable membership, expected Effect and Scope references | Callable schemas, remote routes or credentials |
| `ca-client-tools` | Typed NF clients, Provider Callable Binding Catalog, SBI transport, protocol codecs, CLI and Provider process | Skill routing, Capability grants, Host artifacts, Runtime Outcome/Evidence/Reconcile or NF state authority |
| CA-Agent Host | Provider installation/configuration and process lifecycle, trusted deployment inputs, active-Skill Tool exposure, HostArtifactRef lifecycle/staging | Redefining ToolCatalog or granting a Capability because Provider reports support |
| CA Runtime | Callable linking/admission, Capability/Scope/Effect checks, Invocation identity, Procedure progression, Evidence, Outcome and Reconcile | SBI wire construction, arbitrary URL/path construction or binary persistence |
| NF / OpenAMSM | Authoritative network state and source response | CA Skill routing or Runtime authorization |

### 2.1 Tool Owner versus fact authority

For the first eight context queries, `ca-client-tools` owns the CA-facing callable contract: stable public IDs, normalized Output Schemas, response validation and typed Provider implementation. The source fact authority is the SMF (`smf-owner`), recorded separately in the Evidence Descriptor. The repository containing a contract file does not automatically become its Owner; `ca-client-tools` is Owner because it publishes and maintains this normalized callable contract, not merely because it contains transport code.

## 3. ToolCatalog and CapabilityContracts

ToolCatalog is the source of truth for exact callable contracts. Deployment CapabilityContracts is the source of truth for authorization membership and Scope.

The first SMF slice publishes eight `kind: observation` exports under Owner `ca-client-tools`, Effect `READ`, Owner Outcomes `SUCCEEDED` and `FAILED`, one SUPI/PSI QueryContext Schema, eight closed Output Schemas and one Evidence guarantee per callable.

The deployment publishes Capability ID `ca.smf.context.read` with the eight exact callable members and Scope policy `deployment.smf-context-read`. The Skill grants that Capability ID and references only the exact callable(s) it uses.

Host/Runtime MUST NOT use a direct callable-ID fallback as authorization. Provider support is not authorization. The Host exposes only the exact callable set referenced by the admitted Skill and permitted by deployment policy.

## 4. Provider Callable Binding Catalog

`ca-client-tools` owns one immutable, versioned Provider Callable Binding Catalog. It maps each exact `(owner, kind, id)` identity to exactly one typed Go handler and trusted implementation binding.

It may bind a standardized SBI Operation to a pinned OpenAPI operation, or a private source-pinned Observation query to its fixed typed adapter/private route. For the eight SMF queries, public callable identity selects one fixed OpenAMSM context projection. Private route, private type selector and token header remain Provider details.

The catalog MUST NOT redefine Owner identity, callable kind/ID, ToolCatalog schemas, Effect, Owner Outcomes, Evidence, Capability membership, Scope, credentials or service origin. The process `catalog` method returns a support projection from this same catalog; no second manifest may repeat the support list. Host independently verifies executable/package and catalog digests.

## 5. Host integration and dispatch

Host starts one resident `ca-client-tools provider serve` process per configured Provider instance. The field-level protocol is in [Provider Process Protocol v1](provider-protocol-v1.md): `initialize`, `catalog`, `invoke`, `cancel`.

Provider is not called as a CLI subprocess per invocation. Host MUST connect the process adapter to the existing Tool Dispatch boundary and MUST NOT create a second Tool registry. For CA-SL v1alpha4, Observation is a first-class Procedure stage and uses the same callable admission/dispatch boundary as Operation. The legacy Predicate-side `ObservationSource.acquire()` is not a valid v1alpha4 adapter; see [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).

## 6. First vertical slice and deferrals

The first combined delivery is the eight SMF context Observation callables through the actual Host/Runtime path:

1. publish eight ToolCatalog exports, Schemas and Evidence Descriptors;
2. bind them to the installed Go Provider;
3. install/start the resident Provider in ca-agent-demo;
4. authorize exact callables through Capability ID and Scope;
5. execute a real query through admitted Skill/Host/Runtime dispatch;
6. pass positive and negative authorization, Scope, binding, output and process tests.

This slice is JSON-only. It reuses the existing typed Go SMF client and does not invoke the CLI. HostArtifactRef, NAS/NGAP, multipart, binary transport, N1/N2 and mutation callables are later work.

`UNAVAILABLE` is a successful query result with an empty context, not a negative assertion. The source provides no Owner `observedAt` or Lifecycle identity; Evidence MUST NOT claim freshness or Lifecycle binding.

## 7. No hidden execution or authority

- No Provider-side LLM/Agent or nested Harness.
- No arbitrary URL, method, path, header, token, service key or filesystem path from a Skill.
- No implicit polling or hidden Observation acquisition inside Predicate evaluation.
- No Provider-owned Capability grants or Agent-facing schemas.
- No CLI subprocess per call and no duplicate SMF query client.
- No automatic replay after cancellation, timeout or process restart.
- No Provider-owned NF state store, Host artifact store, Fabric transaction journal or Runtime responsibility journal.

## 8. Qualification boundary

Contracts are ready for implementation review. They do not claim that Provider process, Host v1alpha4 Observation execution or real Host-dispatch E2E has been implemented or qualified.

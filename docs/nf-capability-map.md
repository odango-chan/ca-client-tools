# 5GC NF capability and interface map

Status: **OpenAMSM read-only CLI v1 implemented; AMF/SMF list surfaces and all eight SMF context queries live-accepted at the CLI boundary; ca-client-tools Provider + ca-agent-demo Host/Runtime dispatch E2E NOT YET ACCEPTED**

This document is the canonical NF/capability planning baseline for
`ca-client-tools`. Other repository documents may summarize it but must not
define competing NF scope or invent endpoint availability.

It separates three evidence layers:

1. **3GPP responsibility** — what the standardized NF/service is responsible
   for; this does not prove a particular deployment implements it.
2. **OpenAMSM source surface** — what is actually registered and implemented
   in the pinned source snapshot.
3. **CA integration decision** — who may call it (Agent, Host or Tool), and
   whether a client exists and has been accepted.

The pinned OpenAMSM source snapshot is the authority for what this repository
can call today. 3GPP service/capability names describe network responsibility,
not an invented OpenAMSM route.

### Current implementation and architecture priority

- **Existing baseline:** preserve the eight read-only SMF context-query
  operations, AMF UE/gNB metrics and SMF PDU metrics with their existing
  evidence boundaries.
- **First CA integration acceptance:** deliver the eight SMF context Observation callables backed by private OpenAMSM query routes
  as one end-to-end vertical slice: Host-owned Observation callable contracts and normalized schemas in
  ca-agent-demo PR #208, a long-lived Go Provider backend in ca-client-tools
  using the bounded NDJSON catalog/invoke/cancel process protocol, Provider
  installation and process lifecycle management in ca-agent-demo, Host/Runtime
  dispatch and Capability/Scope admission, then a real query against OpenAMSM
  through an admitted Skill. Host starts one Provider process per configured instance;
  it does not spawn a process per Tool call. The Provider calls typed Go client
  packages internally; the CLI remains for operator/test use. Client/Provider
  work and Host integration are one acceptance goal; CLI-only live acceptance
  does not complete it.
- **SBI foundation:** PR #2 shared Go h2c transport and the static token-header
  profile and PR #3 streaming JSON/binary/multipart Payload Engine are merged.
  PR #5 freezes the first private SMF Provider Backend Binding Catalog entries
  and JSON-only Provider Process Protocol v1 profile; the catalog loader and
  resident Provider process remain unimplemented. The first SMF context-query
  slice reuses the existing JSON/h2c path and does not wait for binary/multipart
  support or HostArtifactRef storage. This first-slice choice does not narrow
  the general SBI architecture, which supports individually bound READ and
  MUTATION operations and JSON, binary and multipart payloads. These
  workstreams do not change the CLI v1 contract.
- **Operation rollout:** add concrete AMF, SMF, UDM, PCF, AUSF, NRF and other
  NF operations only when their 3GPP action, pinned OpenAPI definition,
  OpenAMSM source/deployment support, identity/error semantics and acceptance
  plan are recorded. The generic transport does not make an operation
  available by itself.
- **Deferred per-operation work:** NF responsibilities may be mapped before
  an endpoint is available; do not infer implementation or authorization from
  a 3GPP service name.

## 1. Naming and ownership rules

1. The public model is organized by 3GPP Network Function (NF) and its
   externally meaningful capability: AMF, SMF, UPF, UDM, AUSF, PCF, NRF, CHF,
   and so on.
2. Internal OpenAMSM module names, implementation acronyms and private route
   identifiers are adapter details. They must not become CA Skill parameters,
   Host dispatch names or public CLI flags.
3. A CLI command identifies an NF resource/action, not an OpenAMSM internal
   module. For SMF context reads the public form is
   `smf context query --type <type>`; the private adapter maps the stable type
   to the corresponding OpenAMSM wire endpoint and validates its response.
4. The current direct context-query baseline remains the explicitly admitted
   SMF context types. This does not impose an SMF-only limit on future SBI
   operations: each standardized operation may be exposed to the Agent when
   its Capability is declared and Host/Runtime admission permits it. AMF/UPF
   metrics and sensitive projections remain Host/Tool-internal unless
   separately authorized. Never add arbitrary cross-NF reads outside the
   owning NF's declared operation contract.
5. A source-native diagnostic projection is not automatically a 3GPP SBI
   service, an authoritative business fact, or an Agent-callable Tool.
6. Do not infer support from a network function's 3GPP role. Require a pinned
   OpenAMSM endpoint/schema, owner behavior and tests before implementing a
   client.

## 1.1 Derivation direction: 3GPP → Tool → Skill

The architecture is derived in this direction:

```text
3GPP procedure / message / NF responsibility
                  |
                  v
Owning NF service or protocol contract
                  |
                  v
CA Tool + declared Capability
                  |
                  v
Skill Procedure Node
```

- Start from the 3GPP action and identify the NF that owns it.
- Define the Tool against that NF's real service/protocol contract, including
  the request/response boundary and identity binding.
- Declare the Tool and its Capability in the CA contract; the Skill consumes
  that declared callable.
- A Skill composes multiple Tools into a procedure. It must not bypass the
  Tool boundary and call an NF endpoint directly.
- The Provider Callable Binding Catalog maps pinned source operationIds to exact
  ToolCatalog callable identities and trusted service keys. Deployment
  CapabilityContracts separately maps Capability IDs to exact callable
  membership and Scope policies. The Host combines these authorities with
  the installed Provider binding and activates only callables used by the
  active Skill and permitted by deployment policy.
- Skill files reference declared Capabilities; they do not author raw URLs,
  HTTP methods, paths, headers, authentication profiles or Operation Manifests.
- One Tool/Procedure Node should not conceal several independently meaningful
  3GPP actions. Separate request/response actions where they have distinct
  ownership, outcomes or retry/recovery semantics.

`ca-client-tools` implements the local client side of this chain. It does not
define Skill syntax, ToolCatalog admission, Capability authorization or
Procedure progression.

## 2. Current OpenAMSM read surfaces

| NF | Source-backed read surface | Current client | Intended consumer | Status / limitation |
|---|---|---|---|---|
| AMF | Metrics `GET /ue-info` | AMF UE page client | Host context/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; `LIVE_ACCEPTED`. Host-internal by default; not a replacement for Namf SBI |
| AMF | Metrics `GET /gnb-info` | AMF gNB page client | Host topology/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; `LIVE_ACCEPTED`. Host-internal by default |
| SMF | Metrics `GET /pdu-info` | SMF PDU page client | Host context/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; `LIVE_ACCEPTED`. Host-internal by default; no SMF/UPF N4 SEID pair |
| SMF | Eight read-only private OpenAMSM SMF context-query routes (private OpenAMSM interface) | Typed h2c query adapter; next delivery is exact Observation callables in ca-client-tools | Agent only through declared ToolCatalog/Capability/Runtime admission | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; `FIXTURE_TESTED`; `CLI_LIVE_ACCEPTED`; `HOST_DISPATCH_E2E_NOT_YET_ACCEPTED`. `AVAILABLE` and `UNAVAILABLE` are preserved by the client |
| UPF | No UPF session-inspection endpoint in the selected OpenAMSM source | None | Deferred; no direct UPF query client in this scope | `NOT_AVAILABLE` for `/session-info`; no server patch or inferred cross-NF join |
| UDM | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| AUSF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| PCF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| NRF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| CHF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |

The AMF routes are registered in `src/amf/init.c`; the SMF metrics and
context-query routes are registered in their respective SMF source modules.
The SMF SBI listener is h2c. Its original token lookup used a mixed-case
header name in a case-sensitive hash, so HTTP/2 clients received 403
`FORBIDDEN`. The operator authorized a local `env-openamsm` build overlay that
adds the same lowercase lookup already used by the orchestration handler; all
eight real context queries now pass. The imported upstream tree is unchanged
and this repository does not carry the server patch. The selected source
exposes no UPF session-inspection route; this repository neither infers UPF
state nor adds a UPF client. See the detailed
[OpenAMSM client interface inventory](openamsm-interface-inventory.md) and
[real acceptance evidence](evidence/real-openamsm-acceptance-20261117.md).

## 3. Stable public SMF context types

The public CLI and Host-facing vocabulary is:

| Public type | Meaning | Private OpenAMSM wire code |
|---|---|---|
| `session` | SM context / session projection | SC |
| `policy` | Policy association projection | PA |
| `up-node` | User-plane node selection/binding projection | UPN |
| `signaling-session` | Signaling-session projection | SSM |
| `ip-address` | UE address allocation projection | IAM |
| `up-selection` | User-plane selection projection | UPS |
| `charging` | Charging projection | CA |
| `session-auth` | Session authentication projection | SAA |

The wire-code column is retained only to explain the pinned OpenAMSM adapter.
It must not appear in the public command tree or public success envelope.
Public output contains `type`, `status`, `key` and `context`; the adapter
validates the source `masId` and omits it from the public result.

The public context type is not a new 3GPP NF or SBI service name. It is a
stable CA operation selector for one of the currently available SMF
context projections.

## 4. NF responsibilities relevant to CA workflows

| NF | 3GPP responsibility | Context/data that may matter to CA | CA access rule |
|---|---|---|---|
| AMF | UE registration, reachability, connection and mobility management | Registration/reachability state, serving access, location/TA, gNB association, UE identity projection | Agent does not directly query AMF context in the current architecture; Host/Tool obtains it when required |
| SMF | PDU Session establishment and lifecycle, session control, UPF selection/control | PDU Session identity/state, DNN, S-NSSAI, address, policy association, selected UPF and session procedure state | Agent may query only the declared SMF context capabilities; Host supplies trusted identity binding |
| UPF | User-plane packet forwarding and QoS enforcement | N4 session state, SEID, PDR/FAR/QER, tunnel endpoints, counters and forwarding status | Tool/Host-owned; no current OpenAMSM session inspection endpoint |
| UDM | Subscription data and serving registration data | Subscription/session-related data when explicitly needed | Tool/Host-owned; not queried directly by Agent under current rule |
| AUSF | UE authentication | Authentication procedure/result state | Tool/Host-owned; no client in this repository |
| PCF | Policy authorization and policy decisions | SM policy association and decision data | Prefer SMF-owned context projection when sufficient; direct PCF access requires a separately specified need and source contract |
| NRF | NF registration and discovery | NF profile, service availability and discovery data | Environment/service-discovery concern, not a Skill-authored query |
| CHF | Charging function; example service: `Nchf_ConvergedCharging` | Charging data and charging session state | Tool/Host-owned if a concrete workflow requires it |
| NSSF | Network slice selection; example service: `Nnssf_NSSelection` | Requested/selected S-NSSAI, NSSAI mapping and slice availability | Host/Tool-owned when a workflow needs slice-selection state; not a direct Agent query |
| UDR | Structured data repository; example service: `Nudr_DataRepository` | Subscription, policy and exposure data accessed by owning NFs | Access through UDM/PCF/other owning NF; no direct Skill-authored UDR query |
| BSF | Binding support for PCF discovery; example service: `Nbsf_Management` | UE/session/address-to-PCF binding | Service-discovery/Host concern if required; no direct Agent query |
| NEF | Controlled exposure of network capabilities; `Nnef_*` services | Exposed events, monitoring and AF-facing capabilities | Conditional Tool/Host integration; not part of the current PDU Session client |
| AF | Application influence on traffic and policy (not itself a 5GC NF) | Application session and traffic influence data | External integration; only through an explicitly designed AF/NEF path |
| NWDAF | Network data analytics; example service: `Nnwdaf_AnalyticsInfo` | Analytics about network/user/session behavior | Optional analytics input; never a substitute for authoritative live NF state |
| SCP | Service Communication Proxy | SBI routing, discovery and indirect communication | Deployment transport infrastructure; not a Skill business-context source |
| SEPP | Security Edge Protection Proxy | Inter-PLMN SBI protection across the N32 boundary | Inter-PLMN deployment boundary; not a direct Skill client |
| UDSF | Unstructured Data Storage Function | Unstructured state stored for consuming NFs | Internal dependency of the owning NF; no direct query by Agent |
| SMSF | SMS Function | SMS over NAS procedure state | Only relevant to an SMS-specific Skill; not part of PDU Session establishment |
| 5G-EIR | 5G Equipment Identity Register | Equipment identity status | Registration/equipment-control workflow; not a normal PDU Session context query |
| NSACF | Network Slice Admission Control Function | Slice admission and usage control state | Conditional Host/Tool dependency when slice admission is deployed |

The UE and NG-RAN/gNB participate in 5GC procedures but are not 5GC core
Network Functions. Their data must be identified separately from NF-owned
context.

### 4.1 Example standardized service relationships

The following names are orientation aids, not an endpoint inventory or a
claim of implementation in the pinned OpenAMSM deployment:

| Consumer / provider relationship | Standardized service family (examples) | CA planning implication |
|---|---|---|
| AMF ↔ SMF | `Nsmf_PDUSession` | Session-control request/response actions belong to the AMF/SMF procedure boundary |
| AMF ↔ AUSF | `Nausf_UEAuthentication` | Deferred in this phase; authentication remains an owning-NF procedure |
| SMF ↔ UDM | `Nudm_SDM`, `Nudm_UECM` | Deferred; subscription and serving-registration data stay behind the owning NF contract |
| SMF ↔ PCF | `Npcf_SMPolicyControl` | Deferred; no independent PCF client is added |
| SMF ↔ UPF | N4 / PFCP | No UPF session-read client in this scope; direct N4 remains a separate PFCP protocol concern |
| AMF ↔ NSSF | `Nnssf_NSSelection` | Deferred; slice selection remains a distinct future capability |
| NF ↔ NRF | `Nnrf_NFDiscovery`, `Nnrf_NFManagement` | Deferred; discovery remains deployment/Host service-discovery work |
| SMF ↔ CHF | `Nchf_ConvergedCharging` | Deferred; charging remains a separately owned procedure |
| UDM/PCF/NEF ↔ UDR | `Nudr_DataRepository` | Deferred; UDR remains behind the consuming NF boundary |
| PCF discovery ↔ BSF | `Nbsf_Management` | Deferred; binding data is not a substitute for PCF policy state |

Service names are drawn from the 5GC service-based architecture vocabulary.
For deferred NFs, this is only a `3GPP_DEFINED` classification; their OpenAMSM
interface status is not being expanded in this phase. Exact operations,
versions, resource paths and payload schemas must be taken from the applicable
3GPP specification version and the selected deployment's OpenAPI/source
contract. They must not be inferred from this table.

This is a responsibility map, not an endpoint inventory. No client should be added for a deferred row until its actual OpenAMSM
interface and contract are verified. No source-patch exception is defined for this repository; do not add a client
for an absent endpoint or alter the OpenAMSM server to create one.

## 5. UPF interface boundary

The selected OpenAMSM source does not expose a UPF session-inspection HTTP
endpoint. `ca-client-tools` does not add `/session-info`, patch the server, or
infer UPF PFCP session state by joining unrelated SMF/UPF identifiers. Direct
N4 remains PFCP and is outside this read-client scope.

## 6. Command surface policy

Current CLI v1:

```text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
```

AMF/SMF metrics are Host-internal diagnostics/context inputs by default.
SMF context queries are eligible for Agent exposure only through the paired
ToolCatalog/Capability/Runtime admission. CLI existence does not grant
visibility or authorization.


## 7. Mandatory gate for adding a future NF client

A new NF client, CLI command or Agent-callable Tool must not be added merely
because the corresponding NF or 3GPP service exists. Before implementation,
the capability-map entry must record all of the following:

1. **Use case and procedure action** — the Skill/business reason, the
   corresponding 3GPP procedure action, and the request/response boundary. A
   Procedure Node should correspond to one meaningful 3GPP action; split
   independently meaningful request/response actions rather than hiding a
   multi-step network procedure inside one opaque client call.
2. **NF and service ownership** — the owning NF, provider, standardized
   service family where applicable, and the exact consumer/provider
   relationship.
3. **Deployment evidence** — pinned OpenAMSM commit, source registration and
   handler path, or the actual deployed OpenAPI contract. A 3GPP specification
   alone is not deployment evidence.
4. **Wire contract** — transport, method/path, API version, authentication,
   request/response schemas, identity keys, status/error semantics, paging,
   consistency guarantees, timeout and retry behavior.
5. **CA ownership and exposure** — whether the operation is Agent-callable
   or Host/Tool-internal. There is no blanket rule that all SBI operations
   must be SMF-only: each operation is separately declared and admitted.
   Sensitive context projections may remain Host-internal. Cross-NF access
   must follow the owning NF's contract and must not become arbitrary
   Agent-authored HTTP.
6. **Client boundary** — stable CA-facing NF/resource/action naming. OpenAMSM
   module names, private route IDs and implementation-specific acronyms stay
   inside the adapter.
7. **Verification** — fixture contract tests, source-side tests where
   available, native build and live-NF acceptance. Each result must be
   recorded separately; a fixture test does not prove source or live
   interoperability.

Use these explicit evidence states in the map:

- `3GPP_DEFINED`: standardized responsibility/service identified.
- `SOURCE_VERIFIED`: exact implementation and wire contract inspected in
  the pinned OpenAMSM source.
- `CLIENT_IMPLEMENTED`: client code exists.
- `FIXTURE_TESTED`: local contract tests pass.
- `SOURCE_TESTED`: relevant OpenAMSM source tests/build pass.
- `LIVE_ACCEPTED`: real deployment query has been verified.
- `NOT_AVAILABLE`: the required interface is absent from the inspected
  source snapshot. An absent UPF session-inspection route is not implemented.
- `NOT_INVENTORIED`: a 3GPP responsibility is known, but the selected
  OpenAMSM interface has not yet been inventoried; this is not proof of
  absence.
- `DEFERRED`: a concrete future use case exists, but implementation is
  intentionally postponed.

Do not collapse these states into a single "supported" label. Update this map,
the interface inventory, architecture and traceability together when a client
is added or retired.

## 8. Explicit exclusions

The following remain outside the SBI Client/Provider Callable Binding Catalog scope:

- NW Agent Workflow Planning / Replan and other multi-step planning services;
- private Nsmf PDUSession facade, SM Context orchestration/update
  synchronization and Namf Relay workflow/facade endpoints;
- callback registration, route registration, pending-operation journals,
  business lifecycle and Procedure continuation;
- arbitrary HTTP requests exposed as Agent Tools;
- Open5GS compatibility, inferred N4 joins and direct PFCP transport inside
  the SBI client. Opaque PFCP messages may be transported as declared Nsmf
  multipart payloads without implementing PFCP transport.

Individual standardized 3GPP SBI operations, including mutation operations,
may be added through the trusted Provider Callable Binding Catalog when their use case,
source/deployment support, schemas, identity, effect, idempotency and
verification gates are complete. A mutation operation is not a workflow or
orchestration service.



## First Agent-callable SMF Observation contract

The public callable Owner is ca-agent-host. The generic
context.smf.session.read callable provides the initial SC-backed UE/PSI
presence decision. Eight optional projection-specific callables expose the
individual SMF MAS projections only when a Procedure explicitly needs them.
Their exact Host-owned ToolCatalog and normalized output proposals are in
[ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208).

ca-client-tools owns the typed Go client and private Provider backend
operations only. Its Provider Backend Binding Catalog maps internal backend
operation IDs to fixed OpenAMSM routes; it does not publish public callable
identities or Capability grants. The Evidence authority is SMF/OpenAMSM.

The generic session read distinguishes HTTP 404 SESSION_NOT_FOUND (ABSENT),
HTTP 200 AVAILABLE (PRESENT with projection) and HTTP 200 UNAVAILABLE
(PRESENT with empty projection). These are not interchangeable.

The source response has no observedAt, eventTime or validUntil. The Host Owner
Evidence freshness gate remains open; no freshness or Lifecycle guarantee is
published, and Evidence-dependent PASS remains NOT_QUALIFIED.

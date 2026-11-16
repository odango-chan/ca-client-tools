# 5GC NF capability and interface map

Status: **Existing read-client baseline retained; generic SBI Client v1 architecture accepted; per-operation rollout remains evidence-gated**

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
  operations, AMF UE/gNB metrics, SMF PDU metrics and the UPF diagnostic
  projection with their existing evidence boundaries.
- **SBI foundation:** implement the shared Go HTTP/2 transport, static
  service/TLS/auth profiles, trusted Operation Catalog, CA-Agent Tool Provider
  and JSON/binary/multipart Payload Engine described in
  docs/sbi-client-architecture.md.
- **Operation rollout:** add concrete AMF, SMF, UDM, PCF, AUSF, NRF and other
  NF operations only when their 3GPP action, pinned OpenAPI definition,
  OpenAMSM source/deployment support, identity/error semantics and acceptance
  plan are recorded. The generic transport does not make an operation
  available by itself.
- **UPF:** retain the existing Host/Tool-only PFCP session diagnostic
  projection. PFCP/N4 remains separate from SBI.
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
4. The Agent's direct context-query responsibility remains limited to the
   explicitly admitted SMF context types. AMF/UPF and other NF data needed to
   complete a workflow is acquired by the owning Tool/Host adapter, not by
   adding arbitrary cross-NF reads to the Agent.
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
- The trusted Operation Catalog maps source operationIds to CA Tool and
  Capability descriptors. The Host activates only the descriptors required
  by the active Skill and allowed by deployment policy.
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
| AMF | Metrics `GET /ue-info` | AMF UE page client | Host context/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; Host-internal by default. UE/MM/CM/access/location and PDU summary projection; not a replacement for Namf SBI |
| AMF | Metrics `GET /gnb-info` | AMF gNB page client | Host topology/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; Host-internal by default. Registered gNB diagnostic projection |
| SMF | Metrics `GET /pdu-info` | SMF PDU page client | Host context/default-tool adapter | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; Host-internal by default. Does not expose an exact SMF/UPF N4 SEID pair |
| SMF | Eight read-only context-query SBI routes | Typed SMF context-query adapter | Agent only through declared Tool/Capability admission | `SOURCE_VERIFIED`; `CLIENT_IMPLEMENTED`; stable CA context types map privately to OpenAMSM route IDs. `AVAILABLE` and `UNAVAILABLE` are preserved |
| UPF | CA-owned patch overlay `GET /session-info` (not in immutable source snapshot) | UPF PFCP session page client | Host/Tool internal adapter | `SOURCE_INTERNALS_VERIFIED`; `PATCH_PROPOSED`; `CLIENT_IMPLEMENTED`; fixture coverage added, execution pending. Exact UPF N4 SEID and SMF F-SEID are read from the same UPF PFCP session |
| UDM | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| AUSF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| PCF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| NRF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |
| CHF | No supported client endpoint established by this inventory | None | Deferred | `3GPP_DEFINED`; `DEFERRED`; no endpoint inferred |

The AMF routes are registered in `src/amf/init.c`; the UPF route is not in the
immutable source snapshot. The UPF PFCP session structures do retain both
N4 identities and the associated rule state. The CA-owned patch overlay in
`env-openamsm/patches/openamsm/0001-upf-session-info.patch` adds a read-only
`GET /session-info` projection to a disposable patched source copy. The
original `upstream/openamsm/` tree remains unchanged. See
[OpenAMSM source findings](https://github.com/odango-chan/env-openamsm/blob/feat/upf-session-info-20261116/docs/openamsm-source-findings.md),
the [patch overlay](https://github.com/odango-chan/env-openamsm/blob/feat/upf-session-info-20261116/patches/openamsm/0001-upf-session-info.patch)
and the detailed
[OpenAMSM client interface inventory](openamsm-interface-inventory.md).

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
| SMF ↔ UPF | N4 / PFCP | Active UPF read scope; the UPF patch exposes its own PFCP session and exact N4 identity pair |
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
interface and contract are verified. The UPF patch is the explicitly
documented exception: its internal state is source-verified and the overlay
defines a separate, testable contract.

## 5. UPF PFCP session read contract and N4 evidence boundary

The immutable OpenAMSM snapshot does not register `GET /session-info`.
However, the UPF's own `upf_sess_t` stores both sides of the PFCP association:
the locally allocated UPF N4 SEID and the SMF F-SEID received in the PFCP
Session Establishment Request (SEID plus IPv4/IPv6 address). It also owns the
UE IP allocation, DNN and PFCP PDR/FAR/QER/URR rule lists.

The CA-owned patch overlay in `env-openamsm` adds a read-only, paginated
`GET /session-info?page=N&page_size=N` projection over that existing state.
It is a deployment-specific diagnostic extension, not a 3GPP SBI service and
not an endpoint present in the immutable source snapshot.

The response includes:

- `upf_n4_seid`: UPF-side N4 SEID as a decimal string;
- `smf_n4_f_seid`: SMF-side F-SEID with decimal-string `seid` and available
  IPv4/IPv6 address fields;
- `ue_ip` and optional `dnn`;
- `pdrs`: PDR IDs, source interface, QFI, FAR/QER references, URR IDs and
  assigned/choose F-TEID information;
- `fars`: FAR IDs, destination interfaces and optional outer-header creation
  (downlink N3 GTP-U TEID/address);
- `qers`: QER IDs and QFIs;
- `urrs`: URR IDs and usage counters. 64-bit counters are decimal strings.

The projection does not invent SUPI or PDU Session ID values that UPF does not
own. Correlation with SMF must use the exact SMF F-SEID (SEID plus address),
not a lone UPF SEID or a guessed join. The output remains Host/Tool-internal;
the metrics listener has no endpoint-specific authentication and must be
network-restricted to trusted callers.

The UPF CLI/client is implemented against this patch contract, but it is
usable only after the patch is applied to a disposable OpenAMSM source copy,
the patched source builds, and the endpoint is accepted against a real N4
session. Until those gates pass, report the source extension as proposed,
not as deployed or live-supported.

## 6. Command surface policy

Current CLI v1:

```text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
ca-client-tools upf session list --page N --page-size N
```

AMF/SMF metrics are Host-internal diagnostics/context inputs by default.
SMF context queries are eligible for Agent exposure only through the paired
ToolCatalog/Capability/Runtime admission. CLI existence does not grant
visibility or authorization.

The UPF command is Host/Tool-internal and depends on the separate
`env-openamsm` patch overlay. Its presence in the CLI does not mean the
immutable source snapshot or a deployed UPF already exposes the route.

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
- `SOURCE_INTERNALS_VERIFIED`: the owning NF's internal state required for a
  projection is verified, but no public endpoint exists in the pinned source.
- `PATCH_PROPOSED`: a CA-owned source overlay defines the endpoint, but has
  not yet been applied and built.
- `PATCH_APPLIED`: the overlay has been applied to a disposable source copy.
- `PATCH_BUILT`: the patched OpenAMSM source build passes.
- `CLIENT_IMPLEMENTED`: client code exists.
- `FIXTURE_TESTED`: local contract tests pass.
- `SOURCE_TESTED`: relevant OpenAMSM source tests/build pass.
- `LIVE_ACCEPTED`: real deployment query has been verified.
- `NOT_AVAILABLE`: the required interface is absent from the inspected
  source snapshot (for example, the unpatched UPF session-read route).
- `NOT_INVENTORIED`: a 3GPP responsibility is known, but the selected
  OpenAMSM interface has not yet been inventoried; this is not proof of
  absence.
- `DEFERRED`: a concrete future use case exists, but implementation is
  intentionally postponed.

Do not collapse these states into a single "supported" label. Update this map,
the interface inventory, architecture and traceability together when a client
is added or retired.

## 8. Explicit exclusions

The following remain outside the SBI Client/Operation Catalog scope:

- NW Agent Workflow Planning / Replan and other multi-step planning services;
- private Nsmf PDUSession facade, SM Context orchestration/update
  synchronization and Namf Relay workflow/facade endpoints;
- callback registration, route registration, pending-operation journals,
  business lifecycle and Procedure continuation;
- arbitrary HTTP requests exposed as Agent Tools;
- Open5GS compatibility, inferred N4 joins and PFCP/N4 inside the SBI client.

Individual standardized 3GPP SBI operations, including mutation operations,
may be added through the trusted Operation Catalog when their use case,
source/deployment support, schemas, identity, effect, idempotency and
verification gates are complete. A mutation operation is not a workflow or
orchestration service.

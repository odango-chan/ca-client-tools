# 5GC NF capability and interface map

Status: **planning baseline; source-backed availability is distinguished from future capability needs**

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

Source snapshot:
`env-openamsm` commit
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
The current source findings are static inspection only. Native build, source
tests and live-NF acceptance remain separate work.

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
| UPF | Metrics listener exists; no `GET /session-info` route registered | None | Tool/Host adapter if a future source contract is added | `NOT_AVAILABLE` for the inspected session-read surface. No PDR/FAR/QER session projection or exact SEID pair |
| UDM | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | `3GPP_DEFINED`; `NOT_INVENTORIED` at OpenAMSM interface level; do not infer or invent an endpoint |
| AUSF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | `3GPP_DEFINED`; `NOT_INVENTORIED` at OpenAMSM interface level; do not infer or invent an endpoint |
| PCF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | `3GPP_DEFINED`; `NOT_INVENTORIED` at OpenAMSM interface level; do not infer or invent an endpoint |
| NRF | No supported client endpoint established by this inventory | None | Environment/service discovery layer | `3GPP_DEFINED`; `NOT_INVENTORIED` at OpenAMSM interface level |
| CHF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | `3GPP_DEFINED`; `NOT_INVENTORIED` at OpenAMSM interface level |

The AMF routes are registered in `src/amf/init.c`, the SMF metrics route in
`src/smf/init.c`, and the absence of a UPF session-info route is verified
against `src/upf/init.c` in the pinned source snapshot. See
[OpenAMSM source findings](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md)
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
| AMF ↔ AUSF | `Nausf_UEAuthentication` | Authentication is normally part of UE registration/access, not an SMF session-context read |
| SMF ↔ UDM | `Nudm_SDM`, `Nudm_UECM` | Subscription and serving-registration data should be obtained through the owning NF contract |
| SMF ↔ PCF | `Npcf_SMPolicyControl` | Policy association is an SMF workflow dependency; do not duplicate it with an invented PCF query |
| SMF ↔ UPF | N4 / PFCP | User-plane control is not an HTTP metrics read; exact SEID and PFCP state require an authoritative UPF contract |
| AMF ↔ NSSF | `Nnssf_NSSelection` | Slice selection is a distinct action and should not be hidden inside an unrelated SMF context read |
| NF ↔ NRF | `Nnrf_NFDiscovery`, `Nnrf_NFManagement` | NF discovery belongs to deployment/Host service discovery, not Skill-authored arbitrary lookup |
| SMF ↔ CHF | `Nchf_ConvergedCharging` | Charging integration is a separately owned procedure/interface |
| UDM/PCF/NEF ↔ UDR | `Nudr_DataRepository` | UDR is generally accessed through the consuming NF's service boundary |
| PCF discovery ↔ BSF | `Nbsf_Management` | Binding data is not a substitute for PCF policy state |

Service names are drawn from the 5GC service-based architecture vocabulary.
For NFs not listed as implemented read surfaces in Section 2, this is only a
`3GPP_DEFINED` classification; their OpenAMSM interface status remains
`NOT_INVENTORIED`. Exact operations, versions, resource paths and payload
schemas must be taken from the applicable 3GPP specification version and the
selected deployment's OpenAPI/source contract. They must not be inferred from
this table.

This is a responsibility map, not an endpoint inventory. No client should be
added for a row until its actual OpenAMSM interface and contract are verified.

## 5. UPF and N4 evidence boundary

The inspected OpenAMSM SMF `/pdu-info` projection does not provide the exact
SMF-side and UPF-side N4 SEID pair previously used by an Open5GS-specific
patch. The UPN context's `n4Seid` is one UPF SEID only. OpenAMSM UPF does not
register the old `/session-info` route.

Therefore this repository must not:

- infer an SMF/UPF session join from a single UPF SEID;
- synthesize UPF PDR/FAR/QER state from SMF data;
- interpret missing UPF inspection data as an empty UPF session;
- claim UPF session inspection is implemented.

If a workflow later requires UPF state, first define the authoritative
OpenAMSM UPF read contract (owner, endpoint, schema, identity/SEID binding,
consistency and pagination), implement it as a separate UPF client, and keep
its output Host/Tool-owned. This does not expand the Agent's direct SMF-context
query rule.

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

There is no UPF CLI command in v1 because the inspected source has no supported
UPF session-read route. Do not add a speculative `upf session list` command.

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
5. **CA ownership and exposure** — whether the data is Agent-callable or
   Host/Tool-internal. Under the current architecture, direct Agent context
   queries are limited to explicitly admitted SMF context types. Other NF
   context is obtained by its owning Host/Tool adapter unless a separate
   architecture decision changes that rule.
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
  source snapshot (for example, the UPF session-read route).
- `NOT_INVENTORIED`: a 3GPP responsibility is known, but the selected
  OpenAMSM interface has not yet been inventoried; this is not proof of
  absence.
- `DEFERRED`: a concrete future use case exists, but implementation is
  intentionally postponed.

Do not collapse these states into a single "supported" label. Update this map,
the interface inventory, architecture and traceability together when a client
is added or retired.

## 8. Explicit exclusions

The following remain outside this initial read-only client scope even if related
contracts exist elsewhere in the OpenAMSM source:

- NW Agent Workflow Planning / Replan;
- Nsmf PDUSession facade (create/modify/release);
- SM Context orchestration/update synchronization;
- Namf relay;
- mutation, callback, workflow, route-registration or lifecycle ownership;
- Open5GS compatibility and inferred N4 joins.

These are separate interfaces and ownership domains. They must be specified
and accepted independently before any future client work.

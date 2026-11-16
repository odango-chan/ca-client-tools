# 5GC NF capability and interface map

Status: **planning baseline; source-backed availability is distinguished from future capability needs**

This document defines the CA-facing network-function model for client planning.
It is not a claim that every 3GPP service or every OpenAMSM endpoint is
implemented. The pinned OpenAMSM source snapshot is the authority for what this
repository can call today; 3GPP service/capability names describe the network
responsibility, not an invented OpenAMSM route.

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

## 2. Current OpenAMSM read surfaces

| NF | Source-backed read surface | Current client | Intended consumer | Status / limitation |
|---|---|---|---|---|
| AMF | Metrics `GET /ue-info` | AMF UE page client | Host context/default-tool adapter | Implemented; Host-internal by default. UE/MM/CM/access/location and PDU summary projection; not a replacement for Namf SBI |
| AMF | Metrics `GET /gnb-info` | AMF gNB page client | Host topology/default-tool adapter | Implemented; Host-internal by default. Registered gNB diagnostic projection |
| SMF | Metrics `GET /pdu-info` | SMF PDU page client | Host context/default-tool adapter | Implemented; Host-internal by default. Does not expose an exact SMF/UPF N4 SEID pair |
| SMF | Eight read-only context-query SBI routes | Typed SMF context-query adapter | Agent only through declared Tool/Capability admission | Implemented in client; stable CA context types are mapped privately to OpenAMSM route IDs. `AVAILABLE` and `UNAVAILABLE` are preserved |
| UPF | Metrics listener exists; no `GET /session-info` route registered | None | Tool/Host adapter if a future source contract is added | Not available in inspected source. No PDR/FAR/QER session projection or exact SEID pair |
| UDM | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | Not in current client scope; do not invent an endpoint |
| AUSF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | Not in current client scope; do not invent an endpoint |
| PCF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | Not in current client scope; do not invent an endpoint |
| NRF | No supported client endpoint established by this inventory | None | Environment/service discovery layer | Not in current client scope |
| CHF | No supported client endpoint established by this inventory | None | Host/Tool-owned dependency if required | Not in current client scope |

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
| CHF | Charging function | Charging data and charging session state | Tool/Host-owned if a concrete workflow requires it |

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

## 7. Explicit exclusions

The following remain outside this read-only client scope even if related
contracts exist elsewhere in the OpenAMSM source:

- NW Agent Workflow Planning / Replan;
- Nsmf PDUSession facade (create/modify/release);
- SM Context orchestration/update synchronization;
- Namf relay;
- mutation, callback, workflow, route-registration or lifecycle ownership;
- Open5GS compatibility and inferred N4 joins.

These are separate interfaces and ownership domains. They must be specified
and accepted independently before any future client work.

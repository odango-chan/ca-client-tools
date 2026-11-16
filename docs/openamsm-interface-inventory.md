# OpenAMSM client interface inventory

Status: source snapshot imported and statically inspected; native build,
context-query source tests and live acceptance NOT_RUN.

Source authority: `env-openamsm/main`, archive
`openamsm-personal-z00638432-vngroup1111.zip`, SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
source commit comment `c27e1761`, 11,904 files.
The full tree digest is recorded in `env-openamsm/upstream.lock.yaml`.
The `env-openamsm` repository snapshot used for this inventory is pinned at
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
The source-finding note is
[docs/openamsm-source-findings.md](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md).

## 1. Available read surfaces

| Owner | Method | Path | Source behavior | Client status |
|---|---|---|---|---|
| AMF | GET | `/ue-info` | Connected/retained UE projection; SUPI/SUCI/PEI, MM/CM state, gNB, location, security, slices, AM policy and PDU-session summaries | Client implemented; acceptance NOT_RUN |
| AMF | GET | `/gnb-info` | Registered gNB projection; PLMN, NG/SCTP, supported TA/S-NSSAI and connected UE count | Client implemented; acceptance NOT_RUN |
| SMF | GET | `/pdu-info` | SUPI and PDU list; PSI/DNN or EBI/APN, IP, S-NSSAI, QoS, N3 and PDU/UE activity state | Client implemented; acceptance NOT_RUN |
| SMF (`session`, internal code `SC`) | POST | `/nsmf-sc-context/v1/query` | Session context projection | Client implemented in current PR |
| SMF (`policy`, internal code `PA`) | POST | `/nsmf-pa-context/v1/query` | Policy context projection | Client implemented in current PR |
| SMF (`up-node`, internal code `UPN`) | POST | `/nsmf-upn-context/v1/query` | User-plane node context projection | Client implemented in current PR |
| SMF (`signaling-session`, internal code `SSM`) | POST | `/nsmf-ssm-context/v1/query` | Signaling session context projection | Client implemented in current PR |
| SMF (`ip-address`, internal code `IAM`) | POST | `/nsmf-iam-context/v1/query` | IP address context projection | Client implemented in current PR |
| SMF (`up-selection`, internal code `UPS`) | POST | `/nsmf-ups-context/v1/query` | User-plane selection context projection | Client implemented in current PR |
| SMF (`charging`, internal code `CA`) | POST | `/nsmf-ca-context/v1/query` | Charging context projection | Client implemented in current PR |
| SMF (`session-auth`, internal code `SAA`) | POST | `/nsmf-saa-context/v1/query` | Session authentication context projection | Client implemented in current PR |
| UPF | GET | `/session-info` | CA-owned patch overlay over existing UPF PFCP session state; exact UPF N4 SEID + SMF F-SEID, UE IP, DNN, PDR/FAR/QER/URR | Client implemented; immutable source does not contain route; patch/build/live acceptance pending |

The AMF registers `/ue-info` and `/gnb-info` in `src/amf/init.c`.
The SMF registers `/pdu-info` in `src/smf/init.c`. The immutable UPF
snapshot does not register `/session-info`; the CA-owned patch overlay adds
the handler to a disposable source copy. The original source tree is unchanged.

## 2. Metrics HTTP contract

All three metrics endpoints are GET handlers on each NF's configured metrics
HTTP listener. The source handler does not apply endpoint-specific application
authentication. The Host must inject a trusted origin and the deployment must
restrict network access; clients must not accept arbitrary Agent-authored URLs.

The common response shape is:

- root object with exactly `items` and `pager`;
- `pager.page`: zero-based page index;
- `pager.page_size`: effective page size;
- `pager.count`: number of emitted items in this page;
- optional `pager.next` and `pager.prev` relative links;
- optional `pager.truncated`, which must be treated as an incomplete result.

The AMF/SMF dumpers default to 100 and clamp at 100. The UPF patch defaults
to 20 and clamps at 100 because each item includes nested PFCP rule state.
The shared JSON pager emits a `next` link only when another page exists.
Clients should request explicit `page` and `page_size`, verify the echoed
pager values and item count, follow only the expected next page, cap page count
and total bytes, and fail closed on `truncated=true`. Do not use
`page=-1` for Agent/Host queries because it disables paging and can exceed
the metrics response buffer.

Metrics payloads are source-native diagnostic projections, not atomic
cross-NF snapshots. Separate pages and separate endpoint calls have no shared
snapshot guarantee.

## 2.1 UPF PFCP session projection (CA-owned patch contract)

The UPF endpoint is added by
[`env-openamsm` patch overlay](https://github.com/odango-chan/env-openamsm/blob/feat/upf-session-info-20261116/patches/openamsm/0001-upf-session-info.patch).
It is not present in the immutable imported source until that patch is applied
to a disposable source copy.

`GET /session-info?page=N&page_size=N` returns the standard `items` plus
`pager` envelope. Each item contains:

- `upf_n4_seid`: decimal string for the UPF-side N4 SEID;
- `smf_n4_f_seid.seid`: decimal string for the SMF-side F-SEID;
- `smf_n4_f_seid.ipv4` / `ipv6`: available SMF F-SEID address values;
- `ue_ip.ipv4` / `ipv6` and optional `dnn`;
- `pdrs`: ID, source interface, QFI, FAR/QER references, URR IDs and F-TEID;
- `fars`: ID and destination interface;
- `qers`: ID and QFI;
- `urrs`: ID and usage counters (64-bit counters encoded as decimal strings).

The projection deliberately does not invent SUPI or PDU Session ID. It is
Host/Tool-internal and is not a 3GPP SBI service. The metrics listener has no
endpoint-specific authentication; network access must be restricted. The CLI
uses page size 20 by default (maximum 100). Patch application, native build,
source tests and live-N4 acceptance are tracked separately.

## 3. OpenAMSM SMF context-query wire contract (private adapter detail)

The eight internal context-query paths use the SMF SBI server and
`X-OpenAMSM-Query-Token`. They are read-only and bypass OES workflow
advancement.

The request body supports:

- `supi` and/or `imsi`, together with `pduSessionId`;
- `ueIpv4` and/or `ueIpv6`;
- identity plus PSI combined with one or both IP keys, where identity locates
  the session and each supplied IP is checked for consistency.

When both SUPI and IMSI are supplied, they must identify the same UE. IMSI is
1–15 decimal digits; SUPI uses the `imsi-` prefix for this endpoint. IPv6
consistency uses the configured/default prefix length (normally /64), not the
interface identifier. PSI is checked in the range 0–255 by the QueryContext
resolver. The v0.10 request has no `masKey` or `fieldMask`.

The private OpenAMSM wire response envelope contains:

- `masId` (validated internally and omitted from the public CLI response);
- `status`: `AVAILABLE` or `UNAVAILABLE`;
- `key`: normalized echoed lookup key;
- `context`: context projection, or exactly an empty object when unavailable.

Important source detail: IMSI is request-only. The response key schema contains
`supi`, `pduSessionId`, `ueIpv4` and `ueIpv6`; the handler echoes the
resolved SUPI when available, not an `imsi` property. The client rejects an
unexpected `imsi` field in the response. The Host still validates the echoed
key against its trusted lookup binding.

An existing session with an internal context provider that has not
authoritatively written its state returns HTTP 200 with `status=UNAVAILABLE` and `context={}`. This is not
equivalent to a negative business value.

Relevant HTTP errors:

| HTTP | Cause | Client treatment |
|---|---|---|
| 400 | `INVALID_QUERY_KEY`, `INVALID_PARAM` | Invalid request; do not retry |
| 403 | `FORBIDDEN` | Configuration/authorization failure |
| 404 | `SESSION_NOT_FOUND`, `UNKNOWN_QUERY_SERVICE` | Preserve exact cause |
| 405 | `METHOD_NOT_ALLOWED` | Client contract failure |
| 409 | `KEY_CONFLICT` | Conflicting query identity |
| 503 | `SERVICE_UNAVAILABLE` | Source disabled/unavailable |
| 500 | `INTERNAL` | Source failure |

## 4. OpenAMSM gaps versus the old Open5GS patch set

| Old Open5GS-specific capability | OpenAMSM source | Decision |
|---|---|---|
| SMF `/pdu-info` exports SMF and UPF N4 SEID pair | Pair absent | Do not port the patch or assume a pair |
| UPF `/session-info` exports current PDR/FAR/QER session | Endpoint absent | Do not implement this client |
| UPN context `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

Consequences:

- No exact SMF-to-UPF session join.
- No UPF PDR/FAR/QER observation from `/session-info`.
- No derived N4 initial-resource or forwarding-resource result from these
  missing surfaces.
- Do not fabricate an absent resource as an empty resource list.

## 5. Work still required

1. Run Go 1.26.2 test/vet/build on the declared toolchain; latest client changes
   have not yet been validated by a successful CI run.
2. Run OpenAMSM native build and MAS source tests against the imported snapshot.
3. Integrate the fixed CLI through the Host default-Tool adapter. Keep AMF and
   SMF metrics commands Host-internal; expose SMF context queries only under explicit
   ToolCatalog/Capability policy.
4. Run live NF queries as a separate acceptance level.


## 6. Adjacent NW Agent contracts — explicitly excluded

The same source snapshot contains additional NW Agent contracts:

| Contract | Source path | Behavior | CLI v1 decision |
|---|---|---|---|
| Workflow Planning / Replan | `tools/nw-agent/openapi/Nnwagent_WorkflowPlanning.yaml` | Plan/Replan workflow requests | Excluded |
| Nsmf PDUSession facade | `tools/nw-agent/openapi/Nnwagent_NsmfFacade.yaml` | Create/modify/release SM Context facade | Excluded |
| MAS Orchestration | `tools/nw-agent/openapi/Nsmf_MasOrchestration.yaml` | MAS operation invoke and SM Context Update Sync | Excluded |
| Namf Relay | `tools/nw-agent/openapi/Nnwagent_Relay.md` | Relay standard Namf communication | Excluded |

These are not additional MAS context-query endpoints. They have distinct
request/response, authorization, write, workflow, relay and lifecycle
semantics. They are intentionally outside the initial read-only
`ca-client-tools` deliverable and must not be implemented by the CLI v1
handoff. The exact source contracts are available under the pinned
[OpenAMSM NW Agent OpenAPI directory](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/).

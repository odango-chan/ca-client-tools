# OpenAMSM client interface inventory

Status: source snapshot imported and statically inspected; a native build was
produced from the pinned tree and used for live acceptance. AMF/SMF metrics
surfaces PASS live; the eight SMF context-query paths are live-BLOCKED by an
OpenAMSM token-header defect. The generic h2c SBI transport is on stacked
PR #2 with validation pending; the Operation Catalog is not implemented.

Source authority: `env-openamsm/main`, archive
`openamsm-personal-z00638432-vngroup1111.zip`, SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
source commit comment `c27e1761`, 11,904 files.
The full tree digest is recorded in `env-openamsm/upstream.lock.yaml` and was
re-verified after the deployment build as
`7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`.
The `env-openamsm` repository snapshot used for this inventory is pinned at
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
The source-finding note is
[docs/openamsm-source-findings.md](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md).

## 1. Available read surfaces

| Owner | Method | Path | Source behavior | Client status |
|---|---|---|---|---|
| AMF | GET | `/ue-info` | Connected/retained UE projection; SUPI/SUCI/PEI, MM/CM state, gNB, location, security, slices, AM policy and PDU-session summaries | Client implemented; **live PASS** |
| AMF | GET | `/gnb-info` | Registered gNB projection; PLMN, NG/SCTP, supported TA/S-NSSAI and connected UE count | Client implemented; **live PASS** |
| SMF | GET | `/pdu-info` | SUPI and PDU list; PSI/DNN or EBI/APN, IP, S-NSSAI, QoS, N3 and PDU/UE activity state | Client implemented; **live PASS** |
| SMF (`session`, internal code `SC`) | POST | `/nsmf-sc-context/v1/query` | Session context projection | Client implemented; **live BLOCKED** |
| SMF (`policy`, internal code `PA`) | POST | `/nsmf-pa-context/v1/query` | Policy context projection | Client implemented; **live BLOCKED** |
| SMF (`up-node`, internal code `UPN`) | POST | `/nsmf-upn-context/v1/query` | User-plane node context projection | Client implemented; **live BLOCKED** |
| SMF (`signaling-session`, internal code `SSM`) | POST | `/nsmf-ssm-context/v1/query` | Signaling session context projection | Client implemented; **live BLOCKED** |
| SMF (`ip-address`, internal code `IAM`) | POST | `/nsmf-iam-context/v1/query` | IP address context projection | Client implemented; **live BLOCKED** |
| SMF (`up-selection`, internal code `UPS`) | POST | `/nsmf-ups-context/v1/query` | User-plane selection context projection | Client implemented; **live BLOCKED** |
| SMF (`charging`, internal code `CA`) | POST | `/nsmf-ca-context/v1/query` | Charging context projection | Client implemented; **live BLOCKED** |
| SMF (`session-auth`, internal code `SAA`) | POST | `/nsmf-saa-context/v1/query` | Session authentication context projection | Client implemented; **live BLOCKED** |

The AMF registers `/ue-info` and `/gnb-info` in `src/amf/init.c`.
The SMF registers `/pdu-info` in `src/smf/init.c` and the eight private
context-query routes. The selected source does not expose a UPF
session-inspection endpoint; no UPF client or server patch is included.

## 2. Metrics HTTP contract

All three metrics endpoints (two AMF and one SMF) are GET handlers on the owning NF's configured metrics HTTP
listener. The source handler does not apply endpoint-specific application
authentication. The Host must inject a trusted origin and the deployment must
restrict network access; clients must not accept arbitrary Agent-authored URLs.

The common response shape is:

- root object with exactly `items` and `pager`;
- `pager.page`: zero-based page index;
- `pager.page_size`: effective page size;
- `pager.count`: number of emitted items in this page;
- optional `pager.next` and `pager.prev` relative links;
- optional `pager.truncated`, which must be treated as an incomplete result.

The AMF/SMF dumpers default to 100 and clamp at 100. The shared JSON pager
emits a `next` link only when another page exists.
Clients should request explicit `page` and `page_size`, verify the echoed
pager values and item count, follow only the expected next page, cap page count
and total bytes, and fail closed on `truncated=true`. Do not use
`page=-1` for Agent/Host queries because it disables paging and can exceed
the metrics response buffer.

Metrics payloads are source-native diagnostic projections, not atomic
cross-NF snapshots. Separate pages and separate endpoint calls have no shared
snapshot guarantee.

## 3. OpenAMSM SMF context-query wire contract (private adapter detail)

The eight internal context-query paths use the SMF SBI server and
`X-OpenAMSM-Query-Token`. They are read-only and bypass OES workflow
advancement.

The SMF SBI listener is **cleartext HTTP/2 with prior knowledge (h2c)**. An
HTTP/1.1 request is rejected before routing; the client uses an h2c transport.
The metrics listeners remain plain HTTP/1.1.

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

### 3.1 Live blocker: token header case

The real listener returns HTTP 403 `FORBIDDEN` for a correct configured token:

- `src/smf/mas/common/mas-context-query-sbi.c` defines
  `QC_TOKEN_HDR "X-OpenAMSM-Query-Token"` and looks it up with
  `ogs_sbi_header_get`, a case-sensitive hash lookup.
- `lib/sbi/nghttp2-server.c` stores header names exactly as received.
- HTTP/2 requires lowercase field names, so a compliant client sends
  `x-openamsm-query-token`; the mixed-case lookup always misses.
- Sending the mixed-case name literally is rejected by nghttp2 with
  `GOAWAY PROTOCOL_ERROR`.

The running SMF was inspected with `gdb`; `context_query.enabled` was true and
`allow_tokens[0]` matched the configured token. The token and client are
correct; the lookup is defective for every RFC-compliant HTTP/2 client. No
server patch was applied. Full evidence:
[docs/evidence/real-openamsm-acceptance-20261117.md](evidence/real-openamsm-acceptance-20261117.md).

## 4. OpenAMSM gaps versus the old Open5GS patch set

| Old Open5GS-specific capability | OpenAMSM source | Decision |
|---|---|---|
| SMF `/pdu-info` exports SMF and UPF N4 SEID pair | Pair absent | Do not port the patch or assume a pair |
| UPF `/session-info` exports current PDR/FAR/QER session | Endpoint absent | No client or server patch in this scope |
| UPN context `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

Consequences:

- No exact SMF-to-UPF session join.
- No UPF PDR/FAR/QER observation through the current client surface.
- No derived N4 initial-resource or forwarding-resource result from these
  missing surfaces.
- Do not fabricate an absent resource as an empty resource list.

## 5. Work still required

1. Fix the token header lookup in a deployed OpenAMSM revision (owner:
   `env-openamsm`). Then rerun the eight context queries live and record
   `AVAILABLE` / `UNAVAILABLE`, echoed-key binding and CLI exit codes.
2. Obtain a usable GitHub Actions run, or keep `CI_UNAVAILABLE` recorded.
3. Validate the shared h2c transport on PR #2 and the Payload Engine on PR #3.
4. Integrate the Provider through ca-agent. The Host owns active-Skill
   registration, policy and BinaryRef storage/resolution; Runtime owns
   admission and Capability authorization.
5. Add individual standardized SBI operations only after their exact
   OpenAMSM source/deployment support is verified. A 3GPP OpenAPI definition
   alone does not prove deployment availability.
6. Run live NF queries and mutations as separate acceptance levels, with
   operation-specific safety and idempotency evidence.

## 6. Adjacent private NW Agent contracts — excluded from the SBI Operation Catalog

The same source snapshot contains additional NW Agent contracts:

| Contract | Source path | Behavior | ca-client-tools decision |
|---|---|---|---|
| Workflow Planning / Replan | tools/nw-agent/openapi/Nnwagent_WorkflowPlanning.yaml | Plan/Replan workflow requests | Excluded; workflow-plan service contract, not an individual NF SBI operation |
| Nsmf PDUSession facade | tools/nw-agent/openapi/Nnwagent_NsmfFacade.yaml | Create/modify/release SM Context facade | Excluded; private NF facade/relay behavior |
| MAS Orchestration | tools/nw-agent/openapi/Nsmf_MasOrchestration.yaml | MAS operation invoke and SM Context Update Sync | Excluded; orchestration and lifecycle ownership |
| Namf Relay | tools/nw-agent/openapi/Nnwagent_Relay.md | Relay standard Namf communication | Excluded; relay/deployment behavior |

These are not additional MAS context-query endpoints. They are private
NW-Agent workflow/facade/relay contracts with distinct authorization, write,
workflow and lifecycle semantics. They remain outside the SBI Operation
Catalog and the CLI v1 scope.

This exclusion does not exclude individual standardized 3GPP SBI operations
(for example, a source-verified Namf_Communication operation) when separately
catalogued with a pinned OpenAPI contract and deployment evidence. The exact
private source contracts are available under the pinned
[OpenAMSM NW Agent OpenAPI directory](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/).

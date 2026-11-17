# OpenAMSM client interface inventory

Status: **source snapshot imported and statically inspected; AMF/SMF metrics
surfaces live-accepted; eight SMF context-query paths live-BLOCKED by an
OpenAMSM token-header defect.** The generic SBI transport/Operation Catalog is
architecture-approved but not implemented and is not part of CLI v1.

Source authority: `env-openamsm/main`, archive
`openamsm-personal-z00638432-vngroup1111.zip`, SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
source commit comment `c27e1761`, 11,904 files.
The tree digest is recorded in `env-openamsm/upstream.lock.yaml` and was
re-verified after the local deployment build:
`7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`.
The `env-openamsm` repository snapshot is pinned at
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
Source findings:
[docs/openamsm-source-findings.md](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md).

## 1. Available read surfaces

| Owner | Method | Path | Client status |
|---|---|---|---|
| AMF | GET | `/ue-info` | Implemented; **live PASS** |
| AMF | GET | `/gnb-info` | Implemented; **live PASS** |
| SMF | GET | `/pdu-info` | Implemented; **live PASS** |
| SMF (`session`, internal code `SC`) | POST | `/nsmf-sc-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`policy`, internal code `PA`) | POST | `/nsmf-pa-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`up-node`, internal code `UPN`) | POST | `/nsmf-upn-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`signaling-session`, internal code `SSM`) | POST | `/nsmf-ssm-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`ip-address`, internal code `IAM`) | POST | `/nsmf-iam-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`up-selection`, internal code `UPS`) | POST | `/nsmf-ups-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`charging`, internal code `CA`) | POST | `/nsmf-ca-context/v1/query` | Implemented; **live BLOCKED** |
| SMF (`session-auth`, internal code `SAA`) | POST | `/nsmf-saa-context/v1/query` | Implemented; **live BLOCKED** |

The AMF registers `/ue-info` and `/gnb-info` in `src/amf/init.c`. The SMF
registers `/pdu-info` in `src/smf/init.c`. The context-query handlers are
registered on the SMF SBI server.

The inspected source has **no UPF `/session-info` endpoint**. This repository
does not implement one and does not patch OpenAMSM to add one.

## 2. Metrics HTTP contract

The three metrics endpoints are GET handlers on the owning NF's configured
metrics HTTP listener (plain HTTP/1.1).

The common response shape is:

- root object with exactly `items` and `pager`;
- `pager.page`: zero-based page index;
- `pager.page_size`: effective page size;
- `pager.count`: number of emitted items in this page;
- optional `pager.next` and `pager.prev` relative links;
- optional `pager.truncated`, which must be treated as an incomplete result.

The AMF/SMF dumpers default to 100 and clamp at 100. The JSON pager emits a
`next` link only when another page exists. Clients request explicit `page` and
`page_size`, verify the echoed pager values and item count, follow only the
expected next link, cap page count and total bytes, and fail closed on
`truncated=true`. Do not use `page=-1` for Agent/Host queries because it
disables paging and can exceed the metrics response buffer.

Metrics payloads are source-native diagnostic projections, not atomic
cross-NF snapshots. Separate pages and separate endpoint calls have no shared
snapshot guarantee. The metrics listener does not apply endpoint-specific
application authentication; the origin must be trusted and network-restricted.

## 3. OpenAMSM SMF context-query wire contract (private adapter detail)

The eight internal context-query paths use the SMF SBI server and
`X-OpenAMSM-Query-Token`. They are read-only and bypass OES workflow
advancement.

### 3.1 Transport

The SMF SBI listener is **cleartext HTTP/2 with prior knowledge (h2c)**. An
HTTP/1.1 request is rejected by the SBI server before routing. The client must
use an h2c transport; this is verified against the real listener.

### 3.2 Request body

- `supi` and/or `imsi`, together with `pduSessionId`;
- `ueIpv4` and/or `ueIpv6`;
- identity plus PSI combined with one or both IP keys, where identity locates
  the session and each supplied IP is checked for consistency.

When both SUPI and IMSI are supplied, they must identify the same UE. IMSI is
1–15 decimal digits; SUPI uses the `imsi-` prefix for this endpoint. IPv6
consistency uses the configured/default prefix length (normally /64), not the
interface identifier. PSI is checked in the range 0–255. The v0.10 request has
no `masKey` or `fieldMask`.

### 3.3 Response envelope

- `masId` (validated internally and omitted from the public CLI response);
- `status`: `AVAILABLE` or `UNAVAILABLE`;
- `key`: normalized echoed lookup key;
- `context`: context projection, or exactly an empty object when unavailable.

The response key schema contains `supi`, `pduSessionId`, `ueIpv4` and
`ueIpv6`. Source behavior (`fill_echo` in
`src/smf/mas/common/mas-context-query.c`):

- identity + PSI: echo the resolved UE SUPI and the requested `pduSessionId`;
- each supplied UE IP key: echo the supplied IP;
- IMSI is request-only and is never echoed as an `imsi` field.

The client binds the echoed key to the request before returning a result. A
missing, extra or different field is a Protocol Error, not `UNAVAILABLE`.

An existing session with an internal context provider that has not
authoritatively written its state returns HTTP 200 with `status=UNAVAILABLE`
and `context={}`. This is not a negative business value.

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

### 3.4 Live blocker: token header case

The real listener returns HTTP 403 `FORBIDDEN` for a correct configured token.
Root cause:

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
correct; the server lookup is defective for every RFC-compliant HTTP/2 client.
No server patch was applied. Full evidence:
[docs/evidence/real-openamsm-acceptance-20261117.md](evidence/real-openamsm-acceptance-20261117.md).

## 4. OpenAMSM gaps versus the old Open5GS patch set

| Old Open5GS-specific capability | OpenAMSM source | Decision |
|---|---|---|
| SMF `/pdu-info` exports SMF and UPF N4 SEID pair | Pair absent | Do not port the patch or assume a pair |
| UPF `/session-info` exports current PDR/FAR/QER session | Endpoint absent | Do not implement this client |
| UPN context `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

Consequences:

- No exact SMF-to-UPF session join.
- No UPF PDR/FAR/QER observation from `/session-info`.
- Do not fabricate an absent resource as an empty resource list.

## 5. Work still required

1. Correct the token header lookup in a deployed OpenAMSM revision (owner:
   `env-openamsm`), then rerun the eight context queries live.
2. Observe a real HTTP 200 `AVAILABLE` / `UNAVAILABLE` response and verify the
   live echoed key binding.
3. Obtain a usable GitHub Actions run, or keep `CI_UNAVAILABLE` recorded.
4. The generic SBI HTTP/2 transport, Operation Catalog, Tool Provider and
   payload engine remain a separate, unimplemented workstream; they are not
   part of PR #1.

## 6. Adjacent private NW Agent contracts — excluded from this client

| Contract | Source path | Behavior | Decision |
|---|---|---|---|
| Workflow Planning / Replan | tools/nw-agent/openapi/Nnwagent_WorkflowPlanning.yaml | Plan/Replan workflow requests | Excluded; workflow service contract |
| Nsmf PDUSession facade | tools/nw-agent/openapi/Nnwagent_NsmfFacade.yaml | Create/modify/release SM context facade | Excluded; private facade |
| MAS Orchestration | tools/nw-agent/openapi/Nsmf_MasOrchestration.yaml | MAS operation invoke and SM Context Update Sync | Excluded; orchestration/lifecycle |
| Namf Relay | tools/nw-agent/openapi/Nnwagent_Relay.md | Relay standard Namf communication | Excluded; relay/deployment behavior |

These are not additional context-query endpoints and remain outside this
repository and the CLI v1 scope.

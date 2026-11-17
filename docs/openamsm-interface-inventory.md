# OpenAMSM client interface inventory

Status: source snapshot imported and statically inspected; native build,
context-query source tests and live acceptance NOT_RUN. The generic SBI
transport/Operation Catalog is architecture-approved but not implemented;
this inventory remains the authority for OpenAMSM deployment evidence.

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
| UPF `/session-info` exports current PDR/FAR/QER session | Endpoint absent | No client or server patch in this scope |
| UPN context `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

Consequences:

- No exact SMF-to-UPF session join.
- No UPF PDR/FAR/QER observation through the current client surface.
- No derived N4 initial-resource or forwarding-resource result from these
  missing surfaces.
- Do not fabricate an absent resource as an empty resource list.

## 5. Work still required

1. Run Go 1.26.2 test/vet/build on the declared toolchain; latest client changes
   have not yet been validated by a successful CI run.
2. Run OpenAMSM native build and MAS source tests against the imported snapshot.
3. Implement the shared SBI HTTP/2 transport, trusted service and private header-token
   profiles, Operation Catalog, Tool Provider contract and streaming
   JSON/binary/multipart Payload Engine as specified in
   docs/sbi-client-architecture.md.
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

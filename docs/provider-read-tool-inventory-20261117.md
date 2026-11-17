# Provider read-tool inventory — P0 scope audit

**Date:** 2026-11-17  
**Source pin:** `odango-chan/env-openamsm` commit
`594dec78b95bff4fdc34deac8a3a9681fb41228f` (deployed lab snapshot; imported
`upstream/openamsm` tree unchanged)  
**Scope:** every read-only surface currently implemented by this repository or
source-verified in the pinned OpenAMSM snapshot, plus the previously discussed
UPF question.

This inventory is the audit behind Provider catalog v2. It does not invent an
endpoint: a tool is included only when the pinned OpenAMSM source registers it
and the repository already has a typed client (or a directly reusable generic
client path). Mock/contract tests and live acceptance are recorded separately
and never imply each other.

## 1. Included read-only tools (P0)

All eleven are exposed by the unified Provider through trusted backend
operation IDs. The eight `openamsm.smf.mas.*.query` IDs are unchanged from the
frozen v1 catalog; the three list operations are added by catalog v2.

| # | NF | Operation | HTTP | Path | OpenAPI operationId | Request | Response | Existing implementation | Provider backend ID | P0 |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | AMF | UE list | GET | `/ue-info` | n/a (metrics handler; see §2) | query `page`,`page_size`; no body | `{items[],pager}` | `internal/openamsm/metrics.Client.GetUEInfo` | `openamsm.amf.ue-info.list` | yes |
| 2 | AMF | gNB list | GET | `/gnb-info` | n/a (metrics handler; see §2) | query `page`,`page_size`; no body | `{items[],pager}` | `internal/openamsm/metrics.Client.GetGNBInfo` | `openamsm.amf.gnb-info.list` | yes |
| 3 | SMF | PDU session list | GET | `/pdu-info` | n/a (metrics handler; see §2) | query `page`,`page_size`; no body | `{items[],pager}` | `internal/openamsm/metrics.Client.GetPDUInfo` | `openamsm.smf.pdu-info.list` | yes |
| 4 | SMF | SC context query | POST | `/nsmf-sc-context/v1/query` | `queryScContext` | `{supi?,imsi?,pduSessionId?,ueIpv4?,ueIpv6?}` (Provider profile: `supi`+`pduSessionId`) | `{masId,status,key,context}`; 200 `UNAVAILABLE` has empty `context`; non-200 `application/problem+json` | `internal/openamsm/smf/mas.Client.QueryOutcome` | `openamsm.smf.mas.sc.query` | yes |
| 5 | SMF | PA context query | POST | `/nsmf-pa-context/v1/query` | `queryPaContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.pa.query` | yes |
| 6 | SMF | UPN context query | POST | `/nsmf-upn-context/v1/query` | `queryUpnContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.upn.query` | yes |
| 7 | SMF | SSM context query | POST | `/nsmf-ssm-context/v1/query` | `querySsmContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.ssm.query` | yes |
| 8 | SMF | IAM context query | POST | `/nsmf-iam-context/v1/query` | `queryIamContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.iam.query` | yes |
| 9 | SMF | UPS context query | POST | `/nsmf-ups-context/v1/query` | `queryUpsContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.ups.query` | yes |
| 10 | SMF | CA context query | POST | `/nsmf-ca-context/v1/query` | `queryCaContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.ca.query` | yes |
| 11 | SMF | SAA context query | POST | `/nsmf-saa-context/v1/query` | `querySaaContext` | same as SC | same as SC | same typed client | `openamsm.smf.mas.saa.query` | yes |

Source evidence for the eight context queries: the pinned OpenAPI
`upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml` (Git blob
`46045c2f8f1cae31c6e6c98d5d20addbebe41969`) and the pinned source tests under
`upstream/openamsm/tests/smf-mas-query/`. Source evidence for the three list
operations: `/ue-info` and `/gnb-info` are registered in
`upstream/openamsm/src/amf/init.c` (Git blob
`1ee5c9b148c68ceb7ceb377a29e982b3c9985878`, symbols `amf_dump_ue_info` and
`amf_dump_gnb_info`); `/pdu-info` is registered in
`upstream/openamsm/src/smf/init.c` (Git blob
`ce8aea73ddcc0dddb84f6c839fe2f4b6c096a88b`, symbol `smf_dump_pdu_info`).

The three list endpoints are the OpenAMSM metrics HTTP/1.1 listeners, not
standardized 3GPP SBI operations. They stay on the typed metrics client (plain
HTTP, bounded pagination, no SBI transport); the unified Provider exposes them
as private backend operations with a typed handler. They are not routed through
the h2c SBI transport.

## 2. Request/response schema summary

### 2.1 List operations

- Request: `page` (zero-based integer ≥ 0, default 0) and `page_size`
  (integer 1–100, default 100) query parameters; explicit values are always
  sent. `page=-1` (pagination disabled) is not exposed.
- Response: root object with exactly `items` and `pager`; `pager.page` echoes
  the request, `pager.page_size` is the effective size, `pager.count` equals
  the emitted item count, optional `pager.next`/`pager.prev` are local links,
  and `pager.truncated=true` is treated as an incomplete/unusable result.
- One invocation returns one page only. No cross-page or cross-endpoint
  snapshot is implied.
- Non-200 responses have no declared ProblemDetails contract and are mapped to
  `REMOTE_RESPONSE_INVALID` with only the non-secret HTTP status.

### 2.2 SMF context queries

- Request profile exposed by the Provider: exactly `supi` (`imsi-` + 1–15
  digits) and `pduSessionId` (integer 0–255). The broader typed client also
  supports IMSI aliases and UE-IP lookup keys for the CLI; those are not exposed
  through the Provider backend profile.
- Response: exact typed envelope `{masId,status,key,context}`; `status` is
  `AVAILABLE` or `UNAVAILABLE`; `UNAVAILABLE` requires an empty `context`;
  echoed `key` must identify the requested SUPI/PSI.
- Non-200 responses are remote ProblemDetails outcomes only when they satisfy
  the declared media type/object/status/cause validation; otherwise
  `REMOTE_RESPONSE_INVALID`.

## 3. UPF and other potentially missed read tools

### 3.1 UPF session inspection — NOT INCLUDED (no deployed endpoint)

The previously discussed UPF query capability is **not** an existing deployed
read endpoint:

- The pinned upstream OpenAMSM source does not register a UPF `/session-info`
  handler. `src/upf/init.c` initializes Prometheus metrics only. The deployed
  lab UPF exposes only its metrics listener (`10.30.20.7:9090`), which is not a
  session-projection endpoint and is not reachable through the approved
  acceptance network surface.
- A CA-owned patch overlay adding a read-only paginated UPF session projection
  exists only on the unmerged `env-openamsm` branch
  `feat/upf-session-info-20261116` (`patches/openamsm/0001-upf-session-info.patch`).
  It has not been applied to the deployed snapshot, has no client in this
  repository, and its own branch record states it requires build, source-test
  and real-N4-session acceptance before use.
- This repository's `AGENTS.md` explicitly excludes porting that patch and
  fabricating an N4 SEID join.

Decision: **not in P0**. Reason: no source-verified live endpoint in the
deployed environment, no existing client, and adding one would require an
unapproved deployment patch and new acceptance scope. It is recorded as
`BLOCKED` on the `env-openamsm` decision to build, deploy and accept the
overlay; it must not be described as an empty resource or inferred from SMF
`/pdu-info`/UPN `n4Seid` data.

### 3.2 Other surfaces re-checked

- **CLI command tree:** `amf ue list`, `amf gnb list`, `smf pdu list`,
  `smf context query --type <8 types>`. No additional read commands exist.
- **Existing Go clients:** `internal/openamsm/metrics` (three list calls) and
  `internal/openamsm/smf/mas` (eight context queries). No other NF client
  exists.
- **OpenAMSM OpenAPI:** the pinned tree's NW Agent OpenAPI directory contains
  the eight MAS query operations plus private workflow-planning, PDUSession
  facade, MAS orchestration and Namf relay contracts. Those are orchestration/
  facade/write contracts, not simple read tools; they remain excluded by
  `AGENTS.md` and the SBI Client Architecture.
- **Standardized 3GPP SBI operations** (UDM/PCF/AUSF/NRF/CHF): no
  source-verified deployment evidence and no pinned release-specific OpenAPI
  selected for this repository. Not in P0; they are added only through the
  trusted generic catalog after the per-operation evidence gate.
- **AMF/SMF metrics beyond the three list endpoints:** the metrics handlers
  expose only these three custom endpoints; other Prometheus metrics are
  operational telemetry, not resource projections, and remain Host-internal
  and out of the Provider catalog.

## 4. Provider mapping and dispatch

| Handler profile | Backend operations | Implementation |
|---|---|---|
| `openamsmSMFContextQuery` | 8 `openamsm.smf.mas.*.query` | `providerapp.runtime.invokeSMFContext` → `mas.Client.QueryOutcome` |
| `openamsmAMFMetricsList` | `openamsm.amf.ue-info.list`, `openamsm.amf.gnb-info.list` | `providerapp.runtime.invokeAMFMetrics` → `metrics.Client.GetUEInfo/GetGNBInfo` |
| `openamsmSMFMetricsList` | `openamsm.smf.pdu-info.list` | `providerapp.runtime.invokeSMFMetrics` → `metrics.Client.GetPDUInfo` |

The fixed handler selection is trusted catalog content. The caller supplies only
the backend operation ID and the declared typed arguments; it cannot select a
URL, method, path, header, content type, service key or credential. The CLI and
the Provider reuse the same typed clients and the same response validation.

## 5. Status vocabulary

- All eleven tools: `IMPLEMENTED`; `LOCAL_ACCEPTANCE_PASS` (unit/contract,
  catalog, protocol, CLI regression); `REAL_E2E_PASS` recorded separately in the
  provider acceptance evidence for the pinned lab.
- UPF session inspection: `BLOCKED` (no deployed endpoint; unmerged overlay).
- Standardized 3GPP SBI operations: `NOT_STARTED` / gated by the per-operation
  evidence rule.
- Mutations: abstraction only; `NOT_STARTED` for production dispatch.

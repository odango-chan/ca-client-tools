# OpenAMSM client interface inventory

Status: source snapshot imported and statically inspected; native build, MAS
unit tests and live acceptance NOT_RUN.

Source authority: `env-openamsm/main`, archive
`openamsm-personal-z00638432-vngroup1111.zip`, SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
source commit comment `c27e1761`, 11,904 files.
The full tree digest is recorded in `env-openamsm/upstream.lock.yaml`.

## 1. Available read surfaces

| Owner | Method | Path | Source behavior | Client status |
|---|---|---|---|---|
| AMF | GET | `/ue-info` | Connected/retained UE projection; SUPI/SUCI/PEI, MM/CM state, gNB, location, security, slices, AM policy and PDU-session summaries | Client implemented; acceptance NOT_RUN |
| AMF | GET | `/gnb-info` | Registered gNB projection; PLMN, NG/SCTP, supported TA/S-NSSAI and connected UE count | Client implemented; acceptance NOT_RUN |
| SMF | GET | `/pdu-info` | SUPI and PDU list; PSI/DNN or EBI/APN, IP, S-NSSAI, QoS, N3 and PDU/UE activity state | Client implemented; acceptance NOT_RUN |
| SMF MAS SC | POST | `/nsmf-sc-context/v1/query` | Session Context | Client implemented in current PR |
| SMF MAS PA | POST | `/nsmf-pa-context/v1/query` | Policy Adapter | Client implemented in current PR |
| SMF MAS UPN | POST | `/nsmf-upn-context/v1/query` | UPNode Manager | Client implemented in current PR |
| SMF MAS SSM | POST | `/nsmf-ssm-context/v1/query` | Signaling Session | Client implemented in current PR |
| SMF MAS IAM | POST | `/nsmf-iam-context/v1/query` | IP Address Manager | Client implemented in current PR |
| SMF MAS UPS | POST | `/nsmf-ups-context/v1/query` | UP Selector | Client implemented in current PR |
| SMF MAS CA | POST | `/nsmf-ca-context/v1/query` | Charge Adapter | Client implemented in current PR |
| SMF MAS SAA | POST | `/nsmf-saa-context/v1/query` | Session Auth | Client implemented in current PR |
| UPF | — | `/session-info` | Not registered by OpenAMSM UPF initialization | Excluded |

The AMF registers `/ue-info` and `/gnb-info` in `src/amf/init.c`.
The SMF registers `/pdu-info` in `src/smf/init.c`. The UPF metrics
initialization does not register `/session-info`.

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

The source defaults to page size 100 and clamps the AMF/SMF dumpers to 100.
The shared JSON pager emits a `next` link only when another page exists.
Clients should request explicit `page` and `page_size`, verify the echoed
pager values and item count, follow only the expected next page, cap page count
and total bytes, and fail closed on `truncated=true`. Do not use
`page=-1` for Agent/Host queries because it disables paging and can exceed
the metrics response buffer.

Metrics payloads are source-native diagnostic projections, not atomic
cross-NF snapshots. Separate pages and separate endpoint calls have no shared
snapshot guarantee.

## 3. SMF MAS common request contract

The eight MAS paths use the SMF SBI server and
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

The response envelope contains:

- `masId`;
- `status`: `AVAILABLE` or `UNAVAILABLE`;
- `key`: normalized echoed lookup key;
- `context`: MAS projection, or exactly an empty object when unavailable.

Important source detail: IMSI is request-only. The response key schema contains
`supi`, `pduSessionId`, `ueIpv4` and `ueIpv6`; the handler echoes the
resolved SUPI when available, not an `imsi` property. The client rejects an
unexpected `imsi` field in the response. The Host still validates the echoed
key against its trusted lookup binding.

An existing session with a MAS that has not authoritatively written its state
returns HTTP 200 with `status=UNAVAILABLE` and `context={}`. This is not
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
| UPN MAS `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

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
   SMF metrics commands Host-internal; expose SMF MAS only under explicit
   ToolCatalog/Capability policy.
4. Run live NF queries as a separate acceptance level.

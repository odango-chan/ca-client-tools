# OpenAMSM client interface inventory

Status: static source inspection baseline; build and live acceptance NOT_RUN.

Source authority: `env-openamsm/docs/openamsm-source-findings.md`.
SMF MAS wire authority: implemented SMF MAS QueryContext SBI v0.10 contract.

## 1. Available read surfaces

| Owner | Method | Path | Initial use | Status |
|---|---|---|---|---|
| AMF | GET | `/ue-info` | UE registration/session-related projection | Source-inspected; response schema gate pending |
| AMF | GET | `/gnb-info` | Registered gNB projection | Source-inspected; response schema gate pending |
| SMF | GET | `/pdu-info` | SMF PDU Session projection | Source-inspected; response schema gate pending |
| SMF MAS SC | POST | `/nsmf-sc-context/v1/query` | Session Context | Contract specified |
| SMF MAS PA | POST | `/nsmf-pa-context/v1/query` | Policy Adapter | Contract specified |
| SMF MAS UPN | POST | `/nsmf-upn-context/v1/query` | UPNode Manager | Contract specified |
| SMF MAS SSM | POST | `/nsmf-ssm-context/v1/query` | Signaling Session | Contract specified |
| SMF MAS IAM | POST | `/nsmf-iam-context/v1/query` | IP Address Manager | Contract specified |
| SMF MAS UPS | POST | `/nsmf-ups-context/v1/query` | UP Selector | Contract specified |
| SMF MAS CA | POST | `/nsmf-ca-context/v1/query` | Charge Adapter | Contract specified |
| SMF MAS SAA | POST | `/nsmf-saa-context/v1/query` | Session Auth | Contract specified |

All eight MAS queries use the SMF SBI server and the
`X-OpenAMSM-Query-Token` header. They are read-only and bypass OES workflow
advancement.

## 2. SMF MAS common request contract

The common request body supports exactly one of these key families:

1. `supi` or `imsi`, together with `pduSessionId`;
2. `ueIpv4`;
3. `ueIpv6`.

The v0.10 contract does not include `masKey` or `fieldMask`.

The response envelope contains:

- `masId`;
- `status`: `AVAILABLE` or `UNAVAILABLE`;
- `key`: the actual lookup key echoed by the SMF;
- `context`: MAS projection, or exactly an empty object when unavailable.

An existing session with a MAS that has not authoritatively written its state
returns HTTP 200 with `status=UNAVAILABLE` and `context={}`. This is not
equivalent to a negative business value.

Relevant HTTP errors include:

| HTTP | Cause | Client treatment |
|---|---|---|
| 400 | `INVALID_QUERY_KEY`, `INVALID_PARAM` | Invalid request; do not retry |
| 403 | `FORBIDDEN` | Configuration/authorization failure |
| 404 | `SESSION_NOT_FOUND`, `UNKNOWN_QUERY_SERVICE` | Preserve exact cause |
| 405 | `METHOD_NOT_ALLOWED` | Client contract failure |
| 409 | `KEY_CONFLICT` | Conflicting query identity |
| 503 | `SERVICE_UNAVAILABLE` | Source disabled/unavailable |
| 500 | `INTERNAL` | Source failure |

## 3. OpenAMSM gaps versus the old Open5GS patch set

| Old Open5GS-specific capability | OpenAMSM inspection | Decision |
|---|---|---|
| SMF `/pdu-info` exports SMF and UPF N4 SEID pair | Pair absent | Do not port the patch or assume a pair |
| UPF `/session-info` exports current PDR/FAR/QER session | Endpoint absent | Do not implement this client |
| UPN MAS `n4Seid` | Single decimal UPF SEID | Preserve as UPN context only; never use as a pair |

Consequences:

- No exact SMF-to-UPF session join.
- No UPF PDR/FAR/QER observation.
- No derived N4 initial-resource or forwarding-resource result from these
  missing surfaces.
- Do not fabricate an absent resource as an empty resource list.

## 4. Work still required

Before AMF/SMF metrics clients are implemented, verify the actual OpenAMSM
source schema for:

- exact response envelope and field types;
- page and page-size rules;
- next-page and truncation behavior;
- duplicate/ambiguous item handling;
- listener address and configuration;
- authentication, if any;
- response-size and timeout limits.

Before any live claim, import/build the actual OpenAMSM source and run the
queries against the configured environment.

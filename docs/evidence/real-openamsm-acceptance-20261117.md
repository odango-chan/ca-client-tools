# Real OpenAMSM acceptance — 2026-11-17

Status: **POST-FIX PASS — three list surfaces PASS; eight SMF Context queries
PASS after an authorized build-overlay lowercase token lookup in the local
`env-openamsm` checkout. The pre-fix blocker is preserved in §4–§5.**

This record separates real deployment evidence from the fixture-based local
contract acceptance. A local fixture is not used as real-NF evidence here.

## 1. Environment

| Item | Value |
|---|---|
| Source | `env-openamsm` pinned snapshot `77c0a2f8511c3498d1270f059881b3cf0f7f5eeb` |
| Archive SHA-256 | `6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f` |
| Source commit comment | `c27e1761ae09957a474cc2b0208d227cde64aabf` |
| Tree SHA-256 | `7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d` |
| Host | WSL2 Ubuntu 26.04, kernel `6.18.58.1-microsoft-standard-WSL2` |
| Build | Native out-of-tree meson/ninja build from a disposable staged copy |
| NFs | NRF, SCP, AMF, SMF, UPF, AUSF, UDM, UDR, PCF, NSSF, BSF |
| RAN/UE | UERANSIM v3.3.0 gNB + UE in the sibling `env-ueransim` environment |
| Subscriber store | MongoDB `open5gs.subscribers`, existing lab subscriber |

The imported `upstream/openamsm` tree was verified before and after the build;
the normalized tree hash matched the pinned value. The build staged a copy and
removed only `subdir('tests')` from that disposable copy to shorten the build.
No server source, handler or route was changed.

Local deployment harness: a sibling `env-openamsm` checkout reuses the
proven `env-open5gs` renderer/runtime shape, renders the 11 NF YAML files from
`configs/open5gs/*.yaml.in`, and starts each NF with a PID-scoped lifecycle.
The harness is not part of `ca-client-tools` or PR #1.

## 2. Real UE and session used

| Field | Value |
|---|---|
| SUPI | `imsi-999700000000001` |
| IMSI alias | `999700000000001` |
| PDU Session ID | `1` |
| DNN | `internet` |
| S-NSSAI | SST 1, SD `000001` |
| UE IPv4 | `10.45.0.2` |
| IPv6 | not allocated for this session |

Registration completed (`MM-REGISTERED/NORMAL-SERVICE`) and PDU Session
Establishment succeeded (`PDU Session establishment is successful PSI[1]`).

## 3. Results

| Surface | Real request | Result | Evidence |
|---|---|---|---|
| AMF UE List | `GET http://10.30.20.5:9090/ue-info?page=0&page_size=100` | **PASS** | One item for the real SUPI with `psi: 1`, `cm_state: connected`, `mm_state: registered` |
| AMF gNB List | `GET http://10.30.20.5:9090/gnb-info?page=0&page_size=100` | **PASS** | One UERANSIM gNB, `setup_success: true`, `num_connected_ues: 1` |
| SMF PDU List | `GET http://10.30.20.4:9090/pdu-info?page=0&page_size=100` | **PASS** | One PDU session, `psi: 1`, `ipv4: 10.45.0.2`, `pdu_state: active`, N3 TEIDs present |
| SC, PA, UPN, SSM, IAM, UPS, CA, SAA | `POST http://10.30.20.4:7777/nsmf-<mas>-context/v1/query` with SUPI + PSI | **BLOCKED** | HTTP 403 `FORBIDDEN` before the query resolver runs |

The compiled CLI was used for these checks. The three list commands exited `0`
with one JSON `data` envelope on stdout and empty stderr. For all eight context
types the compiled CLI exited `4` with exactly one JSON error envelope on
stderr and empty stdout:

```json
{"error":{"code":"OPENAMSM_HTTP_ERROR","httpStatus":403,"cause":"FORBIDDEN"}}
```

Direct `curl --http2-prior-knowledge` against the SMF produced the same
ProblemDetails:

```json
{"title":"Forbidden","status":403,"detail":"Missing or invalid query token","cause":"FORBIDDEN"}
```

## 4. Root cause of the context-query blocker

The SMF MAS QueryContext handler is registered on the OpenAMSM SBI server,
which is cleartext HTTP/2 with prior knowledge (h2c). The token check is:

```c
/* src/smf/mas/common/mas-context-query-sbi.c */
#define QC_TOKEN_HDR "X-OpenAMSM-Query-Token"
...
token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR);
```

`ogs_sbi_header_get` is a case-sensitive hash lookup
(`lib/core/ogs-hash.c`: `memcmp(he->key, key, klen) == 0`). The HTTP/2 server
stores header names exactly as received
(`lib/sbi/nghttp2-server.c`, `on_header`: `ogs_sbi_header_set(...,
namestr, valuestr)`). RFC 9113 requires HTTP/2 field names to be lowercase, so
a compliant client sends `x-openamsm-query-token`; the handler looks up
`X-OpenAMSM-Query-Token` and always misses.

Observed probes:

1. `curl --http2-prior-knowledge -H 'X-OpenAMSM-Query-Token: …'` → HTTP 403.
   curl lowercases the field name on the HTTP/2 wire.
2. A raw HTTP/2 client sending the field name literally as
   `X-OpenAMSM-Query-Token` → server `GOAWAY` with `PROTOCOL_ERROR`. nghttp2
   rejects uppercase field names before the application handler.
3. `gdb` on the running SMF confirmed the configuration was loaded:

   ```
   smf_mas_config()->context_query.num_allow_tokens = 1
   smf_mas_config()->context_query.enabled = true
   smf_mas_config()->context_query.allow_tokens[0] = "ca-client-tools-lab-token"
   ```

Therefore the configuration and client token are correct. The defect is the
case-sensitive lookup of a field name that HTTP/2 requires to be lowercase.
There is no RFC-compliant client-side workaround: sending the mixed-case name
is rejected at the HTTP/2 layer, and sending the lowercase name does not match
the handler's hash key.

No server patch was applied. A one-line fix in a later OpenAMSM revision would
be to look up the lowercase field name or perform a case-insensitive header
lookup. That decision belongs to `env-openamsm`, not this repository.

## 5. Consequences for acceptance

- The eight context paths, the `AVAILABLE` / `UNAVAILABLE` live status, and the
  live echoed-key identity binding could not be observed on this deployment.
  They are **BLOCKED**, not PASS. They are also not FAIL: the client reached
  the correct endpoint over the correct protocol and received a valid source
  ProblemDetails response.
- AMF UE List, AMF gNB List and SMF PDU List are **PASS** on the real
  interfaces, including one-page pagination and `pager` validation.
- The client-side response-binding logic is verified by unit tests and by the
  compiled CLI against a fixture that reproduces the pinned handler source
  semantics. That is local contract evidence only and is not substituted for
  the blocked real result.

## 6. Reproduction commands

```bash
# Real list surfaces (PASS)
GOWORK=off go build -o /tmp/ca-client-tools ./cmd/ca-client-tools
CA_OPENAMSM_AMF_METRICS_ORIGIN=http://10.30.20.5:9090 \
  /tmp/ca-client-tools amf ue list --page 0 --page-size 100
CA_OPENAMSM_AMF_METRICS_ORIGIN=http://10.30.20.5:9090 \
  /tmp/ca-client-tools amf gnb list --page 0 --page-size 100
CA_OPENAMSM_SMF_METRICS_ORIGIN=http://10.30.20.4:9090 \
  /tmp/ca-client-tools smf pdu list --page 0 --page-size 100

# Real context query (BLOCKED by the upstream header lookup)
printf '%s' '{"supi":"imsi-999700000000001","pduSessionId":1}' |
  CA_OPENAMSM_SMF_SBI_ORIGIN=http://10.30.20.4:7777 \
  CA_OPENAMSM_QUERY_TOKEN=<configured-lab-token> \
  /tmp/ca-client-tools smf context query --type session
# exit 4, OPENAMSM_HTTP_ERROR 403 FORBIDDEN
```

## 7. Re-acceptance criteria

After the header lookup is corrected in a deployed OpenAMSM build, rerun the
eight context types with the same SUPI + PSI and record:

- HTTP 200 with `AVAILABLE` or `UNAVAILABLE`;
- echoed SUPI/PSI (and IP keys for joint queries) matching the request;
- compiled-CLI stdout, stderr and exit codes;
- a deliberate UE-A request / UE-B response to prove the client rejects it.

Until then, the real end-to-end acceptance remains **BLOCKED**.

## 8. Post-fix joint debugging (authorized server build overlay)

After the investigation above, the operator explicitly authorized changing the
OpenAMSM header lookup to lowercase for joint debugging. The change was applied
as a **build-time overlay in the sibling `env-openamsm` checkout**, not to the
imported `upstream/openamsm` tree:

- Patch: `env-openamsm/patches/openamsm/0001-mas-query-token-lowercase.patch`
- Staged source: `env-openamsm/build/openamsm-source/.../mas-context-query-sbi.c`
- `upstream/openamsm` tree SHA-256 after the build is unchanged:
  `7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`
- `ca-client-tools` code was not changed.

The overlay mirrors the existing orchestration handler:

```c
#define QC_TOKEN_HDR "x-openamsm-query-token"
#define QC_TOKEN_HDR_LEGACY "X-OpenAMSM-Query-Token"
...
token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR);
if (!token)
    token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR_LEGACY);
```

### 8.1 Real results

The same lab UE/session was used: SUPI `imsi-999700000000001`, PSI `1`,
IPv4 `10.45.0.2`. The compiled CLI was rebuilt from the current
`ca-client-tools` branch.

| Public type | Request | Exit | Status | Echoed identity |
|---|---|---:|---|---|
| `session` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `policy` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `up-node` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `signaling-session` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `ip-address` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `up-selection` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `charging` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |
| `session-auth` | SUPI + PSI 1 | 0 | AVAILABLE | SUPI + PSI match |

Additional real checks:

| Variant | Result |
|---|---|
| IMSI + PSI (`999700000000001`, PSI 1) | exit 0, AVAILABLE, echoed `supi=imsi-999700000000001`, PSI 1 |
| IPv4 only (`10.45.0.2`) | exit 0, AVAILABLE, echoed `ueIpv4=10.45.0.2` |
| SUPI + PSI + IPv4 | exit 0, AVAILABLE, all three echoed keys match |
| Wrong PSI 2 | exit 4, HTTP 404 `SESSION_NOT_FOUND` |
| Wrong SUPI | exit 4, HTTP 404 `SESSION_NOT_FOUND` |
| Unallocated IPv4 | exit 4, HTTP 404 `SESSION_NOT_FOUND` |
| Unallocated IPv6 | exit 4, HTTP 404 `SESSION_NOT_FOUND` |

Direct h2c probe:

```text
http=200
masId SC status AVAILABLE key {'supi': 'imsi-999700000000001', 'pduSessionId': 1}
```

### 8.2 Scope of the result

This is a real end-to-end PASS against a real OpenAMSM deployment with the
authorized build overlay. The server transport remains h2c; the client already
sent the header name correctly lowercased on the wire, so no client transport
change was required. The overlay is local to the lab checkout and must be
adopted by `env-openamsm` before a production deployment can pass the same
checks.

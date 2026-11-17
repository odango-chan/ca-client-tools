# Alternative transport investigation for SMF MAS Query — 2026-11-17

Status: **FAILED / BLOCKED — no existing HTTP/1.1 path reaches the eight
MAS QueryContext routes. No `ca-client-tools` code change was made.**

Scope: determine whether the existing OpenAMSM deployment exposes any
HTTP/1.1 listener, alternate HTTP/2 entry, proxy, gateway or other SMF HTTP
interface that can serve the same eight MAS QueryContext routes and read the
static `X-OpenAMSM-Query-Token` with the server's original case-sensitive
lookup.

Constraints observed: no OpenAMSM source change, no new endpoint, no server
patch, no new proxy/gateway, no non-standard HTTP/2 request, no token-check
bypass, no fabricated E2E result.

## 1. Actual listeners in the running deployment

The deployment is the pinned snapshot built in the sibling `env-openamsm`
checkout, with 11 NFs and a UERANSIM UE holding an active PDU session.
`ss -ltnp` (full), filtered to the relevant processes:

| Process | Address | Protocol | Purpose |
|---|---|---|---|
| `open5gs-smfd` | `10.30.20.4:7777` | h2c (nghttp2) | SMF SBI server; all eight MAS Query routes |
| `open5gs-smfd` | `10.30.20.4:9090` | HTTP/1.1 (libmicrohttpd) | SMF metrics; `/metrics` and `/pdu-info` only |
| `open5gs-smfd` | `127.0.0.4:3868`, `127.0.0.4:5868` | Diameter | freeDiameter, not HTTP |
| `open5gs-amfd` | `10.30.20.5:7777` | h2c | AMF SBI |
| `open5gs-amfd` | `10.30.20.5:9090` | HTTP/1.1 | AMF metrics; `/ue-info`, `/gnb-info` |
| `open5gs-scpd` | `10.30.20.200:7777` | h2c | SCP SBI forwarder |
| other NFs | `10.30.20.x:7777` | h2c | SBI |
| other metrics | `10.30.20.7/13:9090` | HTTP/1.1 | UPF/PCF metrics |

There is no listener on ports 80, 443, 8080, 8081, 8090, 8091, 8888, 3000 or
5000 on the SMF, AMF, SCP or owner address. No nginx, HAProxy, Envoy, Traefik,
Caddy, Python HTTP server or Node server is running. The NW agent port 8091
is closed.

SMF UDP sockets (`8805` PFCP, `2123` GTP-C, `2152` GTP-U) are not HTTP.

## 2. Source evidence

### 2.1 Only nghttp2 is selectable for the SBI server

`lib/sbi/server.c` hard-codes the server implementation:

```c
void ogs_sbi_server_init(int num_of_session_pool, int num_of_stream_pool)
{
    if (ogs_sbi_server_actions_initialized == false) {
#if 1 /* Use HTTP2 */
        ogs_sbi_server_actions = ogs_nghttp2_server_actions;
#else
        ogs_sbi_server_actions = ogs_mhd_server_actions;
#endif
    }
    ...
}
```

`mhd-server.c` is compiled into `libsbi` (`lib/sbi/meson.build`), but its
actions are unreachable at runtime. There is no config key, environment
variable or command-line option that selects it. `meson_options.txt` only
defines fuzzing options. Using MHD would require a source change.

### 2.2 The MAS handler is only reachable from the SBI server event

`src/smf/smf-sm.c`:

```c
case OGS_EVENT_SBI_SERVER:
    sbi_request = e->h.sbi.request;
    ...
    if (mas_context_query_sbi_try_handle(stream, sbi_request)) {
        break;
    }
```

`OGS_EVENT_SBI_SERVER` is emitted by the nghttp2 server. The metrics MHD
server emits metrics events and has no SBI callback path.

### 2.3 The HTTP/1.1 metrics server cannot serve MAS routes

`lib/metrics/prometheus/context.c` accepts only `GET`, serves `/`, `/metrics`
and an exact-match list of registered custom endpoints, then returns
`400 Bad Request` for any other URL. The only SMF custom endpoint is
`/pdu-info` (`src/smf/init.c`). There is no config-driven registration and no
catch-all that calls the SBI handler.

### 2.4 The orchestration handler already handles the lowercase field name

`src/smf/mas/common/mas-orchestration-sbi.c` explicitly documents the HTTP/2
constraint:

```c
/* nghttp2 stores request header names in lowercase (RFC 9113). */
#define ORCH_TOKEN_HDR "x-openamsm-orch-token"
#define ORCH_TOKEN_HDR_LEGACY "X-OpenAMSM-Orch-Token"
...
token = ogs_sbi_header_get(request->http.headers, ORCH_TOKEN_HDR);
if (!token)
    token = ogs_sbi_header_get(request->http.headers, ORCH_TOKEN_HDR_LEGACY);
```

The MAS QueryContext handler does not have the lowercase lookup:

```c
#define QC_TOKEN_HDR "X-OpenAMSM-Query-Token"
...
token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR);
```

This is direct source evidence that the server-side fix pattern already exists
in the same codebase for another handler, and that no client-side casing
trick is intended.

### 2.5 No other SMF HTTP entry exposes the eight projections

- `/pdu-info` returns the PDU-session list, not the SC/PA/UPN/SSM/IAM/UPS/CA/SAA
  context projections.
- AMF `/ue-info` and `/gnb-info` contain AMF metrics, not SMF contexts.
- `/nsmf-mas-orchestration/v1/invoke` is a write/orchestration API. Its
  catalog contains lifecycle operations and `QUERY_UP_SESSION` (an N4 UPF
  query), not the eight QueryContext projections. It also uses a separate
  orchestration token.
- The NW agent (`tools/nw-agent/server.py`) has an h2 inbound server and only a
  `/nnwagent-contextquery/v1/iam` route. It forwards IAM over h2 using
  `h2_client.py`, which lowercases header names; it would hit the same SMF
  token-miss. It is not running in this deployment and does not expose the
  other seven contexts.
- The SCP forwards SBI requests but does not rewrite header names; its
  outbound libcurl h2 client sends lowercase names.

## 3. Real probes

All probes used the real SMF at `10.30.20.4` with the real UE
`imsi-999700000000001` / PSI `1` and the configured lab query token.

| Probe | Observed result |
|---|---|
| `curl --http2-prior-knowledge POST http://10.30.20.4:7777/nsmf-sc-context/v1/query` with token | HTTP 403 `FORBIDDEN` (`Missing or invalid query token`) |
| `curl --http1.1 POST http://10.30.20.4:7777/nsmf-sc-context/v1/query` with token | Empty reply; SMF log: `nghttp2_session_mem_recv() failed (-903:Received bad client magic byte string)` |
| `curl --http1.1 POST http://10.30.20.4:9090/nsmf-sc-context/v1/query` | HTTP 400 `Invalid HTTP Method` (metrics server only supports GET) |
| `curl --http1.1 GET http://10.30.20.4:9090/nsmf-sc-context/v1/query` | HTTP 400 `Bad Request` (no matching metrics route) |
| `curl --http2-prior-knowledge POST http://10.30.20.200:7777/nsmf-sc-context/v1/query` with `User-Agent: SMF-ca-client-tools/1` and `3gpp-Sbi-Target-apiRoot: http://10.30.20.4:7777` | SCP forwards to the SMF; SMF returns HTTP 403 `FORBIDDEN` |
| `curl --http1.1 http://127.0.0.4:3868/` | Empty reply (Diameter, not HTTP) |
| `curl --http1.1 http://127.0.0.1:8091/` | Connection refused (NW agent not deployed) |
| mixed-case `X-OpenAMSM-Query-Token` over raw HTTP/2 | nghttp2 `GOAWAY` with `PROTOCOL_ERROR` (HTTP/2 field names must be lowercase) |

The SCP probe proves that an existing HTTP/2 gateway path reaches the SMF, but
the forwarded token is still lowercase and the SMF still returns 403. It does
not solve the problem.

## 4. Excluded paths

1. Direct HTTP/1.1 to the SMF SBI port — protocol mismatch; no HTTP/1.1
   fallback.
2. SMF metrics HTTP/1.1 server — exact route list only; MAS paths are not
   registered and POST is rejected.
3. Other NF SBI listeners — they do not own SMF context state or route MAS
   queries.
4. SCP — forwards over h2 without header-case rewriting; SMF returns 403.
5. NW agent — h2 inbound, only IAM, forwards over lowercase h2; not running.
6. MAS orchestration SBI — write/orchestration API, separate token and
   operation catalog; not the eight QueryContext projections.
7. Failure-dependency ports (3868/5868), GTP-C/PFCP/GTP-U — not HTTP.
8. External proxy/gateway — none exists in the running deployment; no
   nginx/HAProxy/Envoy/Traefik/Caddy or application proxy was found.

## 5. Conclusion

There is **no existing HTTP/1.1 access path** that lets the server's original
mixed-case lookup read the static token. The only HTTP/1.1 listener is the
metrics MHD server, which cannot route the eight MAS QueryContext endpoints.
Every available SBI path is h2c and therefore delivers the header name in
lowercase, which the QueryContext handler does not look up.

The only in-tree alternatives are server-side:

1. Add the same lowercase lookup/fallback that the orchestration handler
   already uses to `mas-context-query-sbi.c`; or
2. Build/select the compiled-out MHD SBI server (`#if 1` in
   `lib/sbi/server.c`), changing the SBI transport to HTTP/1.1.

Both require an OpenAMSM change and are outside this task's constraints.
`ca-client-tools` was therefore **not modified**. Its h2c client already sends
the correctly lowercased field name on the wire, so the minimal
context-query lookup fix above is sufficient; no client transport change is
needed after that fix.

The eight-context real E2E remains **BLOCKED**.

## 6. Minimal fix for the environment owner (not applied)

Mirror the orchestration handler in
`src/smf/mas/common/mas-context-query-sbi.c`:

```c
/* nghttp2 stores request header names in lowercase (RFC 9113). */
#define QC_TOKEN_HDR "x-openamsm-query-token"
#define QC_TOKEN_HDR_LEGACY "X-OpenAMSM-Query-Token"
...
token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR);
if (!token)
    token = ogs_sbi_header_get(request->http.headers, QC_TOKEN_HDR_LEGACY);
```

After the environment rebuild, rerun the eight context types with a known
SUPI + PSI and verify HTTP 200, `AVAILABLE`/`UNAVAILABLE`, and echoed
SUPI/PSI/IP against the request.

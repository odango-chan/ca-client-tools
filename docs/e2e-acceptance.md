# OpenAMSM read-only end-to-end acceptance

Status: **EXECUTED 2026-11-17 — WSL real environment, Linux-hosted CLI, and
Windows-native `.exe` acceptance PASS. The context queries returned `AVAILABLE`
after the authorized local token-lookup overlay; the Windows-native run adds 23
asserted CLI cases and h2c prior-knowledge evidence.**

This plan validates the OpenAMSM read-only API. The pre-fix run showed the
mixed-case token header defect; the operator then authorized a local
`env-openamsm` build overlay for the lowercase lookup. The imported upstream
tree remains unchanged. It covers the eight private SMF MAS Context Query
routes first.
AMF/SMF metrics list commands are supplementary diagnostics, not prerequisites
for the primary SMF query acceptance.

## 1. Trusted deployment configuration

Inject these values through the Host/deployment environment. Do not put the
query token in command arguments, request JSON, source control or test logs.

- `CA_OPENAMSM_SMF_SBI_ORIGIN`: existing SMF SBI server origin, for example
  `http://<smf-host>:<port>`.
- `CA_OPENAMSM_QUERY_TOKEN`: the existing OpenAMSM query token.

The existing typed SMF MAS adapter now uses h2c directly (PR #1); the shared
SBI transport in PR #2 is a separate workstream. Live probes confirmed that the
SMF listener is h2c and that an HTTP/1.1 request is rejected before routing.
No custom TLS/OAuth client or server patch is required.

## 2. Primary scenario: all eight SMF Context Query routes

Use the deployment's real SUPI + PSI. The 2026-11-17 run used the lab UE
`imsi-999700000000001` with PSI `1` (DNN `internet`, IPv4 `10.45.0.2`). The
same lookup is sent to each public CA context type; the adapter maps each
public type to the existing OpenAMSM MAS route.

~~~sh
for type in session policy up-node signaling-session ip-address up-selection charging session-auth; do
  printf '%s\n' '{"supi":"imsi-999700000000001","pduSessionId":1}' |
    ca-client-tools smf context query --type "$type"
done
~~~

For each of the eight calls, verify:

1. The request reaches the existing SMF SBI listener at the operation path
   corresponding to the selected public type.
2. The configured `X-OpenAMSM-Query-Token` is sent by trusted configuration.
3. The response is HTTP 200 with JSON and a normal `data` envelope.
4. `data.type` is the public CA type; OpenAMSM's internal `masId` is not
   exposed.
5. `data.key` echoes the normalized SUPI and PDU Session ID.
6. `data.status` is either `AVAILABLE` or `UNAVAILABLE`. Both are valid
   query outcomes. An `UNAVAILABLE` result must have an empty `context`;
   it is not a transport or CLI failure.
7. Record each type's status without requiring all eight context providers to
   return `AVAILABLE`.
8. Record the protocol actually negotiated. The client must use h2c; HTTP/1.1
   must not be accepted as success.

Do not require an AMF or SMF metrics list query before these calls. The
identity-plus-PSI lookup is the primary acceptance path. IPv4/IPv6 keys may be
used as a separate supplementary test when the deployment has a known,
consistent UE address.

## 3. Error and negative-path checks

- Unknown context type: rejected locally as invalid input; no network request.
- Missing query key or invalid PSI: rejected locally; no network request.
- HTTP 403: query-token/deployment authorization failure.
- HTTP 404 with `SESSION_NOT_FOUND`: preserve as a remote error; do not
  reinterpret it as `UNAVAILABLE`.
- HTTP 200 with `status: UNAVAILABLE` and `context: {}`: successful query.
- Malformed JSON, mismatched `masId`, malformed echoed key or non-empty
  UNAVAILABLE context: protocol failure.
- Cancellation: no partial stdout result and no secret/token in stderr.

## 4. Supplementary metrics checks (not a prerequisite)

When AMF/SMF metrics origins are available, optionally run:

~~~sh
ca-client-tools amf ue list --page 0 --page-size 10
ca-client-tools amf gnb list --page 0 --page-size 10
ca-client-tools smf pdu list --page 0 --page-size 10
~~~

Verify the source pager values and item counts. These are diagnostic
projections and do not establish an atomic cross-NF snapshot.

## 5. Acceptance record

Record the tested repository commit, Go version, OpenAMSM source revision,
whether h2c was negotiated, the eight public types and their returned status,
and any HTTP error cause. Redact the service host if required by deployment
policy. Never record the query token or forward raw subscriber context to an
Agent.

### 2026-11-17 result

A real OpenAMSM deployment was built from the pinned snapshot (verified tree
hash) and run with 11 NFs and a UERANSIM UE holding an active PDU session.

| Surface | Result |
|---|---|
| AMF UE List | PASS |
| AMF gNB List | PASS |
| SMF PDU List | PASS |
| SC, PA, UPN, SSM, IAM, UPS, CA, SAA | PASS — HTTP 200 `AVAILABLE`, echoed identity match |

The pre-fix context calls reached the real SMF over h2c but were rejected with
403 because the handler looked up a mixed-case header name in a
case-sensitive hash while HTTP/2 requires lowercase field names. After the
authorized local overlay, all eight types returned 200 `AVAILABLE` with
matching echoed SUPI/PSI. IMSI + PSI and SUPI + PSI + IPv4 also passed; wrong
PSI/SUPI returned 404 `SESSION_NOT_FOUND`. Full evidence:
[real acceptance record](evidence/real-openamsm-acceptance-20261117.md).

Correction: the unallocated-UE-IP probes in this environment return HTTP 409
`KEY_CONFLICT` (source ProblemDetails for an identity/IP key mismatch), not
404 `SESSION_NOT_FOUND`. The client preserves either status and exit code 4 and
never converts them to `UNAVAILABLE`. See
[Windows/WSL acceptance §5](evidence/windows-wsl-acceptance-20261117.md).

## 6. Windows-native acceptance (2026-11-17)

The compiled Windows `ca-client-tools.exe` (Go 1.26.6, Windows 11) was run
against the real WSL-hosted OpenAMSM deployment. The WSL NFs listen on
loopback aliases that Windows cannot reach directly, so a minimal
loopback-only WSL TCP forwarder exposed exactly the three required services
(SMF SBI `127.0.0.1:17777`, AMF metrics `127.0.0.1:19090`, SMF metrics
`127.0.0.1:29090`). No Docker, firewall change or `netsh portproxy` was used.

Results: 23/23 asserted CLI cases PASS — the three list surfaces, all eight
context types (`AVAILABLE`, echoed identity), IMSI + PSI, IPv4 only,
SUPI + PSI + IPv4, and the negative cases (wrong PSI/SUPI → 404
`SESSION_NOT_FOUND`; unallocated IP → 409 `KEY_CONFLICT`; wrong token → 403
`FORBIDDEN`; missing token → exit 3; unknown type → exit 2). The Windows
client's first bytes on the SBI wire were the HTTP/2 prior-knowledge preface
(`PRI * HTTP/2.0`), and an HTTP/1.1 request to the SBI port was refused; the
metrics HTTP/1.1 control succeeded. The repeatable runner is
`scripts/acceptance/windows-e2e.ps1`. Full record:
[windows-wsl-acceptance-20261117.md](evidence/windows-wsl-acceptance-20261117.md).

Real `UNAVAILABLE` and live UE-A/UE-B fault injection remain **NOT_RUN**;
they are covered by contract tests only. The Payload Engine and shared SBI
transport are Windows/Linux-native test-validated but are not yet wired to a
catalogued live operation.


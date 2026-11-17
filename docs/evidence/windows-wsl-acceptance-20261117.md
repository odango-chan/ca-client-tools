# Windows native + WSL real-environment acceptance — 2026-11-17

Status vocabulary used here follows `docs/TRACEABILITY.md`: PASS, FAIL,
BLOCKED, NOT_RUN, plus the CI state.

| Gate | Final status (round 2) |
|---|---|
| `WINDOWS_NATIVE_ACCEPTANCE_PASS` | **PASS** — Go 1.26.6 `gofmt`/`go test ./...`/`go vet ./...`/native build on validation head `0b80d33` |
| `WSL_LINUX_ACCEPTANCE_PASS` | **PASS** — Go 1.26.8 `gofmt`/`go build`/`go vet`/`go test ./...` and real symlink rejection on the same head |
| WSL environment health | **PASS** — `env-openamsm` 11/11 NFs and `env-ueransim` gNB + registered UE with an active PDU session, re-verified non-destructively |
| `LIVE_E2E_PASS` | **PASS** — Windows-native `.exe` → WSL-hosted real OpenAMSM: 23/23 strict JSON-field assertions |
| `CI_UNAVAILABLE` | **CI_UNAVAILABLE** — the GitHub Actions job never started because of an account billing/spending-limit annotation; this is not a CI PASS and not a test failure |
| Real `UNAVAILABLE` context | **NOT_RUN** — the lab deployment returned `AVAILABLE` for all eight types |
| Live UE-A/UE-B cross-response injection | **NOT_RUN** — requires authorized fault injection; covered by contract tests only |
| Live Payload Engine operation | **NOT_RUN** — the Payload Engine is not yet wired to a catalogued operation |
| Real IPv6 session | **NOT_RUN** — the lab PDU session has no IPv6 address allocated |

This record separates three things that must not be conflated:

1. Windows-native and WSL/Linux source validation (native toolchains on each
   host, real test execution — not a cross-compile substitute).
2. WSL environment health (real NFs, real UE/PDU).
3. Live end-to-end CLI acceptance from the Windows `.exe` over the network
   path to the WSL-hosted NFs.

Mock/fixture tests prove client contract behavior only. The live results below
come from the actual deployment.

## 1. Execution environment

| Item | Value |
|---|---|
| Host OS | Windows 11 专业版, version `10.0.26200.0` |
| Host shell | Windows PowerShell `5.1.26100.9818` |
| Windows Go | `go1.26.6 windows/amd64` (satisfies `go.mod` `go 1.26.2`; not downgraded) |
| WSL distro | `Ubuntu-26.04` (selected via `wsl.exe --list --quiet`; only distribution) |
| WSL kernel | `6.18.58.1-microsoft-standard-WSL2` |
| WSL Go | `go1.26.8 linux/amd64` |
| WSL networking | **mirrored** (`C:\Users\<user>\.wslconfig` → `[wsl2] networkingMode=Mirrored`) |
| Windows Git | used only for the Windows work tree |
| WSL Git | used only for the WSL checkout; the two work trees were never cross-modified |

Repository/branch state at acceptance time:

| Item | Value |
|---|---|
| `ca-client-tools` main | `189deff` — PR #1, PR #4 and PR #2 merged |
| PR #2 `feat/sbi-client-v1-transport-20261116` | merged 2026-11-17T06:43:38Z; merge commit `189deff0d88edb9b5bc64314d2c132408f3deb32`; head merged `5482223`; independent final review COMMENTED with MERGE_READY, no blockers |
| PR #3 `feat/sbi-payload-engine-v1-20261117` | base `main`; validation head `0b80d33a78b900759379ad80e1d7d150e23272a6` — a non-destructive merge of updated main on top of `1cfd69b` (payload fix), `40e3132` (acceptance tooling) and `2e03dc8` (phase-1 docs); the final docs/evidence commit sits immediately above it on the same branch |
| `~/env-openamsm` | `594dec78b95bff4fdc34deac8a3a9681fb41228f`, branch `main`, clean |
| `~/env-ueransim` | `72cd533b48136abb7c6eb4ac10e4709ddb29847a`, branch `main`, clean |

Round 1 used PR #3 code-validation head `d01015c`; the round-1 narrative in
§§4–9 is retained as the historical record. Round 2 (§10) is the final v1
validation round and re-ran the same 23 cases with strict JSON-field
assertions.

Neither environment repository was modified. No NF was restarted. The
OpenAMSM build uses the documented `env-openamsm` patch
`patches/openamsm/0001-mas-query-token-lowercase.patch`; the imported
`upstream/openamsm` tree SHA-256 stayed
`7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`.

## 2. Real UE and PDU session

Verified with the `env-ueransim` / `env-openamsm` stable CLIs and the real AMF
and SMF metrics surfaces; nothing was asserted from the CLI alone.

| Field | Value |
|---|---|
| UE | `UE-A`, RUNNING |
| gNB | UERANSIM gNB, RUNNING |
| Registration | `CM-CONNECTED / RM-REGISTERED / MM-REGISTERED/NORMAL-SERVICE`, `5U1-UPDATED` |
| SUPI | `imsi-999700000000001` |
| SUCI | `suci-0-999-70-0000-0-0-0000000001` |
| PDU Session ID | `1` |
| DNN / S-NSSAI | `internet` / SST 1, SD `000001` |
| UE IPv4 | `10.45.0.2` (real `uesimtun0` in the UE netns) |
| IPv6 | not allocated |
| PDU state | `active`, `pdu_sessions_count=1` |
| N3 | gNB TEID `1` @ `10.30.20.5:2152`; UPF TEID `810` @ `10.30.20.7:2152` |

`env-ueransim wait registered --name UE-A` reports
`UE_REGISTERED_NO_PDU_SESSION`-timeout on this already-established session
because that command asserts the intermediate no-session state; status and AMF
evidence show the UE is registered with an active session. This is expected,
not a registration failure.

## 3. Windows → WSL network path

The NFs listen on loopback aliases in WSL (`lo`: `10.30.20.4:7777`,
`10.30.20.5:9090`, `10.30.20.4:9090`). Those addresses are **not** reachable
from Windows (negative control: `Test-NetConnection 10.30.20.4 -Port 7777` → `TcpTestSucceeded=False`). In mirrored mode, however, Windows and WSL share
`127.0.0.1`.

A minimal loopback-only TCP forwarder (Python 3 stdlib asyncio, byte
transparent so h2c passes through) exposes only the three required services:

| Windows-side origin | WSL listener | Backend | Protocol |
|---|---|---|---|
| `http://127.0.0.1:17777` | `127.0.0.1:17777` | `10.30.20.4:7777` | SMF SBI (HTTP/2 h2c) |
| `http://127.0.0.1:19090` | `127.0.0.1:19090` | `10.30.20.5:9090` | AMF metrics (HTTP/1.1) |
| `http://127.0.0.1:29090` | `127.0.0.1:29090` | `10.30.20.4:9090` | SMF metrics (HTTP/1.1) |

No Docker, no Windows Firewall change, no `netsh portproxy`, no iptables and
no expanded network exposure were used. The forwarder has explicit
start/stop/restart/status and a PID file plus log; its PID changes across
restarts. At acceptance time: RUNNING, pid `57110`. Scripts:
`scripts/acceptance/wsl-tcp-forward.{sh,py}`.

## 4. Windows-native source validation

All commands ran in the Windows work trees with `GOWORK=off` (a parent
`go.work` would otherwise mask this module). Both branches were checked out on
a real Windows NTFS work tree; no cross-compilation was used.

| Check | PR #2 `5482223` | PR #3 `d01015c` |
|---|---|---|
| `gofmt -l .` | empty (PASS) | empty (PASS) |
| `go test ./... -count=1` | PASS (4 packages + cmd) | PASS (5 packages + cmd) |
| `go vet ./...` | PASS | PASS |
| `go build -o dist\ca-client-tools.exe .\cmd\ca-client-tools` | PASS | PASS |
| `.\dist\ca-client-tools.exe --version` | `ca-client-tools dev`, exit 0 | `ca-client-tools dev`, exit 0 |
| `.\dist\ca-client-tools.exe --help` | exit 0 | exit 0 |

`internal/sbi/payload`'s symlink rejection sub-case is skipped on this Windows
account only because creating the symlink fixture needs
`SeCreateSymbolicLinkPrivilege` (`ERROR_PRIVILEGE_NOT_HELD`); traversal and
regular-file checks still run. The real symlink rejection was executed on
Linux/WSL (§6).

A repository-level `.gitattributes` (`* text=auto eol=lf`) was added so that a
Windows checkout produces LF files and `gofmt -l .` is meaningful and stable
across Windows and Linux.

## 5. Live end-to-end acceptance from the Windows `.exe`

The compiled Windows executable was driven only through the documented
trusted configuration environment variables. The lab MAS Query token was read
from the deployment configuration into the test process environment and is
never recorded here, in logs, or in Git.

| Surface / variant | Expected | Observed | Result |
|---|---|---|---|
| AMF UE List | 200, exit 0 | one real SUPI item, `pdu_sessions_count=1` | PASS |
| AMF gNB List | 200, exit 0 | one real UERANSIM gNB, connected UEs 1 | PASS |
| SMF PDU List | 200, exit 0 | PSI 1, `10.45.0.2`, `pdu_state=active` | PASS |
| Context `session` | 200 AVAILABLE, exit 0 | AVAILABLE, key SUPI + PSI echoed | PASS |
| Context `policy` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `up-node` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `signaling-session` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `ip-address` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `up-selection` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `charging` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| Context `session-auth` | 200 AVAILABLE, exit 0 | AVAILABLE, key echoed | PASS |
| IMSI + PSI | 200 AVAILABLE, exit 0 | AVAILABLE; echoed `supi=imsi-999700000000001`, PSI 1 | PASS |
| IPv4 only | 200 AVAILABLE, exit 0 | AVAILABLE; echoed `ueIpv4=10.45.0.2` | PASS |
| SUPI + PSI + IPv4 | 200 AVAILABLE, exit 0 | AVAILABLE; all three echoed keys match | PASS |
| Wrong PSI (2) | 404 `SESSION_NOT_FOUND`, exit 4 | 404 `SESSION_NOT_FOUND`, exit 4 | PASS |
| Wrong SUPI | 404 `SESSION_NOT_FOUND`, exit 4 | 404 `SESSION_NOT_FOUND`, exit 4 | PASS |
| Unallocated IPv4 (`10.45.0.99`) | ProblemDetails, exit 4 | **409 `KEY_CONFLICT`**, exit 4 | PASS (see note) |
| Unallocated IPv6 (`2001:db8:cafe::99`) | ProblemDetails, exit 4 | **409 `KEY_CONFLICT`**, exit 4 | PASS (see note) |
| Wrong token | 403 `FORBIDDEN`, exit 4 | 403 `FORBIDDEN`, exit 4 | PASS |
| Missing token | local config error | exit 3 `CONFIGURATION_MISSING`, no HTTP request | PASS |
| Unknown context type | local invalid input | exit 2 `INVALID_ARGUMENT`, no HTTP request | PASS |

23/23 asserted CLI cases passed. Every positive case wrote exactly one JSON
`data` envelope to stdout with empty stderr; every negative case wrote exactly
one JSON error envelope to stderr with empty stdout. No token, SUPI/IMSI or
raw source body appeared in any error envelope.

Note on unallocated UE IP keys: the pinned source reports an identity/IP key
mismatch as HTTP 409 `KEY_CONFLICT`, not 404 `SESSION_NOT_FOUND`. The client
correctly preserves the source ProblemDetails and never converts it to
`UNAVAILABLE` or to a successful result. An earlier acceptance note had stated
404 for this probe; this record corrects that observation. The 404
`SESSION_NOT_FOUND` semantics were nevertheless observed for wrong PSI and
wrong SUPI.

### 5.1 HTTP/2 prior knowledge (no HTTP/1.1 fallback)

A loopback TCP tap was placed between the Windows CLI and the forwarded SMF
SBI port. The compiled Windows executable's first 24 bytes on the wire were:

``text
client_prefix_ascii="PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
client_prefix_hex=505249202a20485454502f322e300d0a0d0a534d0d0a0d0a
client_prefix_len=24
``

That is the RFC 9113 client connection preface, i.e. HTTP/2 with prior
knowledge (h2c). The same query through the tap returned HTTP 200
`AVAILABLE` with the echoed identity.

Negative probes:

| Probe | Result |
|---|---|
| Windows `curl --http1.1` to the real SMF SBI port (`:17777`) | `curl: (1) Received HTTP/0.9 when not allowed`, exit 1, `http_code=000` — not served as HTTP/1.1 |
| Control: Windows `curl --http1.1` to the AMF metrics listener (`:19090`) | HTTP 200, real AMF JSON |

The CLI code path also cannot fall back: the typed SMF adapter installs a
`golang.org/x/net/http2.Transport` with `AllowHTTP` and a raw-TCP
`DialTLSContext` (h2c only), and the shared `internal/sbi` transport enables
only `UnencryptedHTTP2`. The adapter additionally rejects an `https://` origin
at configuration time.

### 5.2 Reproduction

``powershell
# WSL-side minimal forwarder (see scripts/acceptance/wsl-tcp-forward.sh)
wsl.exe -d Ubuntu-26.04 -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh start

# The token is deployment configuration; inject it into the process
# environment only. Do not put it in arguments, files, logs or docs.
$env:CA_OPENAMSM_QUERY_TOKEN = <read from the env-openamsm deployment config>

.\dist\ca-client-tools.exe amf ue list --page 0 --page-size 100
.\dist\ca-client-tools.exe amf gnb list --page 0 --page-size 100
.\dist\ca-client-tools.exe smf pdu list --page 0 --page-size 100
'{"supi":"imsi-999700000000001","pduSessionId":1}' |
  .\dist\ca-client-tools.exe smf context query --type session

Remove-Item Env:CA_OPENAMSM_QUERY_TOKEN
``

The full repeatable runner is `scripts/acceptance/windows-e2e.ps1`; it asserts
the exit codes/statuses above, stores per-case stdout/stderr/exit files, and
redacts nothing because it never prints the token.

## 6. Linux/WSL source validation

The WSL checkout of the same PR #3 head was validated with the WSL Go
toolchain (native Linux execution, not `/mnt/c` cross-builds):

| Check | Result |
|---|---|
| `gofmt -l .` | empty (PASS) |
| `go build` | PASS |
| `go vet ./...` | PASS |
| `go test ./... -count=1` | PASS (all packages) |
| `TestDirectoryResolverRejectsTraversalAndSymlink` | PASS — the real symlink fixture was created and rejected |

## 7. Review-item closeout covered by this run

- PR #1 review: the h2c-only MAS client now rejects `https://` origins and has
  explicit HTTPS rejection cases. PR #1 was already merged, so the correction
  is carried by PR #2 (next in the stack) and documented in that PR.
- PR #2 review: the branch is synced onto current main (no accidental UPF
  removal or CLI delta); the diff against main is only the new
  `internal/sbi` transport, the h2c-origin fix, its tests, `.gitattributes`
  and one redirect-fixture correction. Metrics clients and SBI transport stay
  separate. The typed SMF adapter migration to the shared transport remains an
  explicit gated follow-up (see `docs/sbi-client-architecture.md` §10).
- PR #3 review: recursive duplicate-JSON-key rejection (request, JSON response
  and multipart root), Windows-only symlink sub-case skip with Linux real
  symlink execution, conservative transport/payload limit composition (payload
  defaults never above the 16 MiB transport bound; 64 KiB multipart framing
  reserve; no transport limit was raised), and a bounded `KindNone` empty-body
  probe. All covered by unit tests plus the native Windows/Linux runs above.

## 8. Architecture boundary check

The delivered stack was re-checked against the agreed ownership table:

| Module | Responsibility | State in this stack |
|---|---|---|
| OpenAMSM NF Client | NF interface adaptation | Typed AMF/SMF clients in `internal/openamsm`; no generic NF framework |
| SBI Transport | HTTP/2 h2c, connection, timeout, cancellation | `internal/sbi` only; metrics clients stay separate |
| Payload Engine | JSON, binary, multipart | `internal/sbi/payload` only; Host owns BinaryRef lifecycle |
| CA-Agent Host | Configuration, Tool registration, BinaryRef lifecycle | Not implemented here; contract only |
| CA Runtime | Capability authorization, execution, Evidence | Not implemented here |
| CA Skills | Business Procedures | Not implemented here |

Confirmed not implemented (kept as separate follow-up work): OAuth2/TLS
management, NRF discovery, UPF `/session-info`, direct PFCP/N4 client,
Operation Catalog, Tool Provider, Host/Runtime integration. The shared
transport is not yet in active use by the typed SMF adapter (explicitly gated),
and no Agent-callable operation was added in this pass.

## 9. Remaining NOT_RUN / follow-up items

1. Real `UNAVAILABLE` context response: all eight types returned `AVAILABLE`;
   the deployment cannot be induced to return `UNAVAILABLE` without changing
   the lab session/state. Remains NOT_RUN. AVAILABLE/UNAVAILABLE handling
   remains covered by contract tests.
2. Live UE-A/UE-B cross-response rejection: requires authorized fault
   injection; contract tests PASS, live NOT_RUN.
3. Live Payload Engine operation: no catalogued operation uses the engine yet
   (Operation Catalog/Tool Provider are separate follow-up work). Engine
   correctness is unit-test level plus Windows/Linux native runs, not live NF
   traffic.
4. SMF MAS adapter migration to the shared SBI transport: deliberately still
   gated; the adapter keeps its private token/identity/ProblemDetails behavior.
5. CI: **CI_UNAVAILABLE**. Local Windows and Linux test/vet/build results are
   PASS; that is not a CI PASS and no CI failure is claimed.
6. IPv6 real session: not allocated by the lab; only unallocated-IPv6 negative
   behavior was exercised.

## 10. Round 2 — PR #3 final closeout (2026-11-17)

Round 2 is the final validation round for the v1 slice. It re-ran the same
23-case acceptance set as round 1, with the Windows-native runner strengthened
to assert the exact JSON fields rather than only exit codes. It supersedes
round 1 in assertion strength only; the case set is unchanged.

### 10.1 Repository and GitHub state

| Item | Value |
|---|---|
| PR #2 | merged to main 2026-11-17T06:43:38Z; merge commit `189deff0d88edb9b5bc64314d2c132408f3deb32`; head merged `5482223`; independent final review COMMENTED with MERGE_READY, no blockers |
| `ca-client-tools` main | contains PR #1, PR #4 and PR #2 |
| PR #3 | base is now `main`; validation head `0b80d33a78b900759379ad80e1d7d150e23272a6` (non-destructive merge of updated main on top of `1cfd69b` payload fix, `40e3132` acceptance tooling and `2e03dc8` phase-1 docs); the final docs/evidence commit sits immediately above it on the same branch |
| PR #3 status | final v1 slice, merged only after the gates in this evidence record; **not merged** at the time of this record |

PR #2 changed 6 files (+897/−3): `.gitattributes`,
`internal/sbi/{client.go,client_test.go,config.go}` and
`internal/openamsm/smf/mas/{client.go,client_test.go}`. PR #3 adds the
Payload Engine (`internal/sbi/payload/{payload.go,payload_test.go,files.go}`
plus `payload_kindnone_test.go`), the acceptance tooling
(`scripts/acceptance/*`) and the documentation.

### 10.2 Environment

| Item | Value |
|---|---|
| Host OS | Windows 11, version `10.0.26200.0` |
| Host shell | Windows PowerShell `5.1.26100.9818` |
| Windows Go | `go1.26.6 windows/amd64` (not downgraded); `GOWORK=off`; real NTFS worktree |
| WSL distro | `Ubuntu-26.04` |
| WSL Go | `go1.26.8 linux/amd64`; real Linux execution, not a `/mnt/c` cross-build |
| Deployment | real OpenAMSM in WSL, 11 NFs; real UERANSIM gNB + registered UE with an active PDU session |
| Windows-side origins | `http://127.0.0.1:17777` (SMF SBI h2c), `:19090` (AMF metrics), `:29090` (SMF metrics) |
| Forwarder | optional, hardened, loopback-only; restarted from `scripts/acceptance/wsl-tcp-forward.sh` before the run and verified to listen only on the three `127.0.0.1` ports |
| Environment stability | no 5GC NF was rebuilt or restarted |
| Token handling | read from the deployment configuration into the test process environment only; never printed, stored or committed |

### 10.3 Final source validation (validation head `0b80d33`)

Windows-native:

| Check | Result |
|---|---|
| `gofmt -l .` | empty (PASS) |
| `go test ./... -count=1` | PASS — 5 packages ok (`internal/cli`, `internal/openamsm/metrics`, `internal/openamsm/smf/mas`, `internal/sbi`, `internal/sbi/payload`) + `cmd` no tests |
| `go vet ./...` | PASS |
| `go build -o .\dist\ca-client-tools.exe .\cmd\ca-client-tools` | PASS |
| exe SHA-256 | `60E892EC5824C1A3DCAE07E1940166D8E2DD387BB8693A4BE7D7B959AB577B35` |
| `.\dist\ca-client-tools.exe --version` | `ca-client-tools dev`, exit 0 |
| `.\dist\ca-client-tools.exe --help` | exit 0 |

WSL/Linux (same head):

| Check | Result |
|---|---|
| `gofmt -l .` | empty (PASS) |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./... -count=1` | PASS (all five packages) |
| `TestDirectoryResolverRejectsTraversalAndSymlink/symlink` | PASS — real symlink fixture created and rejected |

The payload symlink fixture is skipped on this Windows account only because
creating it needs `SeCreateSymbolicLinkPrivilege`
(`ERROR_PRIVILEGE_NOT_HELD`); traversal and regular-file checks still run.
Windows symlink rejection is **not** claimed as PASS; the real symlink
rejection is the WSL/Linux result.

### 10.4 LIVE E2E — strict assertions (PASS 23/23)

Runner: `scripts/acceptance/windows-e2e.ps1` on Windows PowerShell 5.1. Output
directory `acceptance-out-final` (gitignored; holds subscriber data and is not
committed). Summary: **Total=23, PASS=23, FAIL=0, NOT_RUN=0, Result=PASS.**

Negative matrix — actual observed and asserted values:

| Probe | Exit | `error.code` | `error.httpStatus` | `error.cause` |
|---|---|---|---|---|
| Wrong PSI | 4 | `OPENAMSM_HTTP_ERROR` | 404 | `SESSION_NOT_FOUND` |
| Wrong SUPI | 4 | `OPENAMSM_HTTP_ERROR` | 404 | `SESSION_NOT_FOUND` |
| Unallocated IPv4 (`10.45.0.99`) | 4 | `OPENAMSM_HTTP_ERROR` | 409 | `KEY_CONFLICT` |
| Unallocated IPv6 (`2001:db8:cafe::99`) | 4 | `OPENAMSM_HTTP_ERROR` | 409 | `KEY_CONFLICT` |
| Wrong token | 4 | `OPENAMSM_HTTP_ERROR` | 403 | `FORBIDDEN` |
| Missing token | 3 | `CONFIGURATION_MISSING` | none | none — origins pointed at closed `http://127.0.0.1:1`, proving no request |
| Unknown context type | 2 | `INVALID_ARGUMENT` | none | none — same closed-origin no-request proof |

Positive identity binding — all eight types (`session`, `policy`, `up-node`,
`signaling-session`, `ip-address`, `up-selection`, `charging`,
`session-auth`): exit 0, `data.status` `AVAILABLE`, `data.type` equal to the
requested type, `data.key.supi` = `imsi-999700000000001` and
`data.key.pduSessionId` = `1`. The IMSI + PSI variant echoes the normalized
SUPI with the matching PSI; SUPI + PSI + IPv4 (`10.45.0.2`) echoes all three
key fields; IPv4 only echoes `data.key.ueIpv4` = `10.45.0.2`.

List membership and pager assertions:

| Surface | Asserted content |
|---|---|
| AMF UE List | target SUPI item, `pdu_sessions_count=1`, `pdu_sessions[].psi` contains `1`, `pager` present |
| AMF gNB List | expected name `UERANSIM-gnb-999-70-1`, `num_connected_ues=1`, `pager` present |
| SMF PDU List | target SUPI, `pdu` entry `psi=1` with `pdu_state=active` and `ipv4=10.45.0.2`, `pager` present |

Stream discipline and leakage: every case emitted exactly one JSON document on
the expected stream with the opposite stream empty; no token or subscriber
value leaked into any error text.

Offline runner evidence (self-test level, **not** live evidence): the runner
parses cleanly under PowerShell 5.1 (6261 tokens, 0 errors); `-SelfTest` passes
23 assertions; the runner exit codes 2 (missing required parameters) and 3
(missing token) were verified. A 23-case rehearsal against a temporary fixture
executable built from prior captured JSON was also exercised during tooling
development, but its artifacts are not retained, so it is not claimed as
independently reproducible evidence; the live 23/23 matrix above is the
acceptance evidence.

### 10.5 Forwarder hardening and tests

The acceptance forwarder listener is the module constant `127.0.0.1`. The
`--bind` flag was deleted and `CA_WSL_FORWARD_BIND` is not read at all.
Defense-in-depth validation rejects `0.0.0.0`, `::`, `::1`, non-loopback IPv4
and IPv6 addresses and hostnames before any listener is created. Exactly the
three declared services (17777/19090/29090) are exposed; there is no fourth
listener and no general proxy behavior.

`test_wsl_tcp_forward.py`: 13/13 PASS on Windows (Python 3.14) and WSL
(`python3`) — `127.0.0.1` accepted; wildcard/non-loopback IPv4 and IPv6, `::`,
`::1`, hostname and empty rejected; `--bind` rejected by argparse; help does
not advertise `--bind`; `CA_WSL_FORWARD_BIND=0.0.0.0` still yields exactly
three loopback listeners; a mutated constant fails closed with exit 2 and no
listener; a non-3-service table is rejected.

`test-wsl-forwarder-lifecycle.sh` was executed for real in WSL with an
isolated state directory, free high non-default ports and
`CA_WSL_FORWARD_BIND=0.0.0.0` deliberately set: start → RUNNING; exactly three
`127.0.0.1` listeners; status → RUNNING exit 0 with the same PID; restart →
new PID; stop → stopped; status after stop → exit 3 NOT RUNNING with no
listeners; the pre-existing live acceptance forwarder was untouched. Exit 0,
`RESULT: PASS`.

### 10.6 CI

**CI_UNAVAILABLE.** The GitHub Actions `Go CI` job for the PR #2 head (run
`44020384806`) shows conclusion failure, but the job never started: the
annotation is "The job was not started because recent account payments have
failed or your spending limit needs to be increased." This is GitHub
billing/infrastructure state — not a test failure and not a CI PASS. All prior
runs, including main, show the same state.

### 10.7 Still NOT_RUN / out of scope

1. Real SMF context `UNAVAILABLE` — the lab always returns `AVAILABLE`.
2. Live UE-A/UE-B cross-response injection — requires authorized fault
   injection; contract tests only.
3. Live Payload Engine operation — not wired to a catalogued operation.
4. SMF MAS adapter migration to the shared transport — deliberately still
   gated; the adapter keeps its private token/identity/ProblemDetails behavior.
5. Real IPv6 PDU session — the lab has none; only the unallocated-IPv6
   negative case was exercised.
6. Operation Catalog, Tool Provider, OAuth2/TLS/NRF dynamic discovery, direct
   PFCP/N4 and Wait/Ingress lifecycle — out of scope / follow-up work.

These items remain `NOT_RUN` and are never weakened to `PASS`.

### 10.8 Known non-blocking follow-ups

1. `probeEmptyBody` may return the bounded `ErrInvalidPayload` instead of
   `ctx.Err()` when cancellation coincides exactly with the no-progress read
   bound. It still fails closed; no response byte can be accepted.
2. The optional forwarder exits 1 with a Python traceback for a non-numeric
   `CA_WSL_FORWARD_*_PORT` value instead of the documented configuration
   exit 2 (the listener boundary itself is unaffected).


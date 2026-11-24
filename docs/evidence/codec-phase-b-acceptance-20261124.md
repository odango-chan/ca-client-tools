# Codec Phase B acceptance evidence (2026-11-24, closeout)

Status: LOCAL ACCEPTANCE PASS / CI recorded below
Branch: `feat/free5gc-codec-phase-b-20261124`
PR: [#39](https://github.com/odango-chan/ca-client-tools/pull/39) (OPEN)

This document records the Phase B closeout: the NAS qualification accounting
gap, runtime qualification semantics, JSON/IPC contract review and Python worker
concurrency/lifecycle fixes. Phase C migration has **not** started.

## 1. Scope of the changes

- `protocol/codec/evidence.go` (new): one evidence registry. It is the only
  source for `caTested` / `caQualified` in the generated manifest and for the
  runtime NAS, N2 and NGAP qualification gates.
- `protocol/codec/capability.go`: manifest entries derive from the registry and
  carry `evidenceId`, `evidenceSource` and `qualifiedScope`.
- `protocol/codec/nas.go`, `n2.go`, `ngap.go`: byte-identical re-encode no
  longer implies `qualified`. NAS requires a message-type evidence record plus
  no partial/unknown/duplicate/lossy projection. N2 additionally requires the
  input to stay inside the evidenced IE/vector scope. NGAP stays
  `qualified=false` and `complete=false` for every full PDU.
- `protocol/codec/json.go`: reject trailing JSON values, null for non-nilable
  targets, integer/float overflow, and BIT STRING lengths larger than their
  payload; no reflection panic on untrusted input.
- `protocol/codec/worker/worker.go`: compare the declared frame length in
  uint32 space before allocating.
- `python/ca_codec_worker/client.py`: one lock covers the complete serial
  request-response cycle; per-generation queue and reader; immediate
  cross-thread `cancel()` / `close()`; duplicate request-ID rejection.
- Tests: `business_test.go`, `codec_test.go`, `json_test.go`,
  `python/tests/test_codec_worker.py`.

No APER patch, fork, new Runtime, new version system or consumer migration is
introduced.

## 2. Qualification result

| Namespace | Enumerated | CA tested | CA qualified |
| --- | --- | --- | --- |
| NAS 5GSM | 20 | 8 | 8 |
| NGAP PDU | 120 | 1 (`NGReset`, self round-trip) | 0 |
| N2 transfer roots | 41 | 7 | 7 (scoped) |

NAS qualification:

| MT | Name | Tested/qualified | Evidence ID |
| --- | --- | --- | --- |
| `0xC1` | PDUSessEstReq | yes/yes | `NAS-C1-CA-ENC-001` |
| `0xC2` | PDUSessEstAccept | yes/yes | `NAS-C2-OPENAMSM-ENC-001` |
| `0xC3` | PDUSessEstRej | yes/yes | `NAS-C3-OPENAMSM-ENC-001` |
| `0xC9` | PDUSessModReq | yes/yes | `NAS-C9-CA-ENC-001` |
| `0xCB` | PDUSessModCmd | yes/yes | `NAS-CB-OPENAMSM-ENC-001` |
| `0xD1` | PDUSessRelReq | yes/yes | `NAS-D1-CA-ENC-001` |
| `0xD3` | PDUSessRelCmd | yes/yes | `NAS-D3-OPENAMSM-ENC-001` |
| `0xD4` | PDUSessRelComplete | yes/yes | `NAS-D4-CA-ENC-001` |
| `0xCA` | PDUSessModRej | no/no | none — accounting gap closed by downgrade |
| `0xCC` | PDUSessModComplete | no/no | none — accounting gap closed by downgrade |
| `0xCD` | PDUSessModCmdRej | no/no | none — accounting gap closed by downgrade |
| `0xD2` | PDUSessRelRej | no/no | none — accounting gap closed by downgrade |
| other 8 | Auth/Status/Remote UE | no/no | none |

The four downgraded messages remain fully callable through the Free5GC decoder.
Their decode is `SUCCESS` with `reencodeVerified=true`, `reencodeEligible=true`
and `qualified=false`; `complete=true` when the parser reported no partial data.
`TestNASUnqualifiedMessagesStillDecode` pins this behaviour.

N2 qualified roots (all 7 scoped): Setup Request, Modify Request, Setup
Response, Setup Unsuccessful, Modify Response, Release Command, Release
Response. The two container roots are restricted to the evidenced Protocol IE
IDs (Setup Request `{130,134,136,139}`, Modify Request `{135}`); the five
non-container roots must equal an evidenced vector. All other successfully
decoded inputs report `N2_NOT_QUALIFIED`.

## 3. Windows amd64 (native)

- Windows 11 (10.0.26200.0), PowerShell 5.1
- Go 1.26.6 windows/amd64, Python 3.14.6
- `CGO_ENABLED=0`, `GOWORK=off`
- `gofmt -l` clean for the committed branch files (an unrelated untracked
  `.agent-tmp/` scratch file in the local checkout was excluded)
- `go test <committed packages> -count=1`: PASS (all packages)
- `go vet <committed packages>`: PASS
- `go build <committed packages>`: PASS
- `CGO_ENABLED=0 go build -o ca-client-tools.exe ./cmd/ca-client-tools`: PASS
- `CA_CLIENT_TOOLS_BIN=... python -m unittest discover -s python/tests -v`:
  21 tests PASS

Codec-specific Windows results:

- real OpenAMSM NAS vectors `0xC2/0xCB/0xD3/0xC3` decode and re-encode
  byte-identically and are qualified;
- CA TS 24.501 vectors `0xC1/0xC9/0xD1/0xD4` decode, re-encode and are
  qualified;
- `0xCA/0xCC/0xCD/0xD2` minimal vectors decode with re-encode identity but
  `qualified=false` and a `NAS_NOT_QUALIFIED` diagnostic;
- ePCO decodes as projection-only and `EncodeNAS` refuses to emit bytes;
- NAS PARTIAL is reached only through `*message.Error.IEToDoList`;
- N2 OpenAMSM/asn1c vectors, IPv4/IPv6 and the IPv6 two-QFI vector decode,
  re-encode byte-identically and are qualified inside the evidenced scope;
- N2 `ext=1` root vectors are `NOT_QUALIFIED`; trailing octets are `INVALID`;
- NGAP `NGReset` self round-trip returns `SUCCESS` with
  `qualified=false`, `complete=false` and `NGAP_NOT_QUALIFIED`;
- Go worker and native facade report identical status/qualification flags.

## 4. Linux amd64

- WSL2 Ubuntu 26.04 LTS, x86_64, Go 1.26.8 linux/amd64, Python 3.14.4
- `GOWORK=off`, `CGO_ENABLED=0`
- `go test <committed packages> -count=1`: PASS
- `go vet <committed packages>`: PASS
- `go build <committed packages>`: PASS
- `CGO_ENABLED=0 go build -o /tmp/ca-client-tools ./cmd/ca-client-tools`: PASS
- `CA_CLIENT_TOOLS_BIN=/tmp/ca-client-tools python3 -m unittest discover -s
  python/tests -v`: 21 tests PASS

WSL is an acceptance environment only; the product has no WSL dependency.
The authoritative Linux run remains the `ubuntu-latest` CI job on the PR head.

## 5. Python worker concurrency and lifecycle evidence

All tests are deterministic: `threading.Barrier` orders the racing calls and a
fake framed worker provides explicit `sleep`, `exit` and `wrong_id` behaviour.
No test relies on a sleep to "probably" hit a race.

| Test | What it proves |
| --- | --- |
| `test_two_threads_do_not_mismatch` | Two threads released by one barrier each receive their own request ID, tag and binary payload. |
| `test_many_threads_sequential_requests` | 8 threads × 5 requests: no deadlock, no loss, 40 unique request IDs, correct association. |
| `test_slow_request_does_not_receive_fast_response` | A fast request cannot overtake the slow request and steal its response; elapsed time proves serialisation. |
| `test_worker_exit_during_request_then_restart` | In-flight process exit raises `CodecWorkerExited`, clears process state, and the next request starts a fresh worker. |
| `test_cancel_interrupts_in_flight_request` | `cancel()` from another thread wakes the wait with `CodecWorkerCancelled` instead of blocking for the 60 s timeout. |
| `test_close_interrupts_in_flight_request` | `close()` wakes the wait with `CodecWorkerExited`, leaves no running process, and late requests are rejected as closed. |
| `test_timeout_then_restart` | Timeout stops the worker; the next request starts a new PID and succeeds. |
| `test_restart_isolates_old_response_queue` | The restarted generation has a different queue; the old queue contains only failure markers, never a reusable response. |
| `test_duplicate_request_id_is_rejected` | A reused caller request ID raises `CodecWorkerProtocolError` before it is sent. |
| `test_response_request_id_mismatch_is_rejected` | A wrong response ID is a protocol error and stops the desynchronised process. |
| `test_request_order_and_association` | 25 sequential requests preserve order and payload association. |
| `test_frame_bounds_are_rejected_before_allocation` | Oversized request/response frames are rejected before large allocation. |

Existing lifecycle tests remain: real-binary timeout and abnormal exit.

## 6. CI

GitHub Actions `Go CI` run
[45353264623](https://github.com/odango-chan/ca-client-tools/actions/runs/45353264623)
on commit `b62d863847cff29ee639c12afb6255b5ded55e32`:

- `linux`: **PASS**
- `windows`: **PASS**

Both native runners executed `gofmt`, the Go suite, `go vet`, `go build`, the
race suites, the codec worker build and the 21-test Python IPC/concurrency
acceptance. The later documentation-only commit on the same branch is validated
by its own `Go CI` run, reported in the PR #39 checks; the code under test is
identical to `b62d863`.

A GitHub Actions job that fails before executing steps is `CI_UNAVAILABLE`,
never PASS. No such result occurred or is claimed here.

## 7. What is not proven

- No 3GPP Release 19 conformance claim for all 120 NGAP messages, all NAS IEs or
  all 41 N2 roots.
- No live SCTP/N2 capture. OpenAMSM/asn1c/CA vectors are independent of
  Free5GC and remain the interoperability evidence.
- Nested APER extension additions are not independently proven; the unpatched
  Free5GC APER limitation remains.
- `0xCA/0xCC/0xCD/0xD2` need an independent vector before they can be
  qualified. A self round-trip is not accepted as that evidence.

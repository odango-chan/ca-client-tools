# Provider acceptance evidence — 2026-11-17

**Branch:** `feat/unified-provider-read-tools-20261117`  
**Validation head:** `dfb761c` (the implementation tree tested below; this
evidence document is committed immediately above it)  
**Scope:** unified resident Provider for all eleven existing read-only
OpenAMSM tools, Provider Process Protocol v1 + additive v2 read-tool extension,
trusted catalog v1/v2, shared-transport migration, generic SBI executor,
trusted service configuration, CLI regression.  
**Status vocabulary:** `IMPLEMENTED`, `LOCAL_ACCEPTANCE_PASS`,
`REAL_E2E_PASS`, `BLOCKED`, `NOT_RUN`, `CI_UNAVAILABLE`.

> This record is for the implementation branch's validation head. It is not a
> product qualification, not a Host/Runtime integration acceptance, and not a
> claim that standardized 3GPP SBI operations or mutations are enabled.

## 1. Environment

- Host OS: Windows-native (Go 1.26.6 windows/amd64).
- Linux environment: WSL2 Ubuntu 26.04, Go 1.26.8 linux/amd64 (acceptance and
  Linux-native gates only; not a product dependency).
- Live NF environment: pinned `odango-chan/env-openamsm` commit
  `594dec78b95bff4fdc34deac8a3a9681fb41228f`, deployed in WSL with a real
  UERANSIM UE and an active PDU session.
- Live accepted origins (loopback-only acceptance network):
  SMF SBI `http://127.0.0.1:17777` (h2c), AMF metrics
  `http://127.0.0.1:19090`, SMF metrics `http://127.0.0.1:29090`.
- Query token: read from the deployed env-openamsm config at run time through
  `scripts/acceptance/wsl-provider-e2e.sh` / the existing get-token helper;
  never printed or committed. No token, SUPI or raw remote body is stored in
  this document.
- Test UE: `imsi-999700000000001`, PDU session ID `1`.

## 2. Source pins and digests

| Item | Value |
|---|---|
| env-openamsm commit | `594dec78b95bff4fdc34deac8a3a9681fb41228f` |
| MAS OpenAPI blob | `upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml` @ `46045c2f8f1cae31c6e6c98d5d20addbebe41969` |
| AMF init source blob | `upstream/openamsm/src/amf/init.c` @ `1ee5c9b148c68ceb7ceb377a29e982b3c9985878` |
| SMF init source blob | `upstream/openamsm/src/smf/init.c` @ `ce8aea73ddcc0dddb84f6c839fe2f4b6c096a88b` |
| v1 catalog SHA-256 | `sha256:cbb8de356531c1f5e43c558a59eaa4bec94bd3d074505307ed99fc09fd42ac57` (frozen artifact, unchanged bytes) |
| v2 catalog SHA-256 | `sha256:b2de218a63a596de0b948617c8e864c8e6e4bf8bc8758e6652cb5d0e7f4bfd50` |

## 3. Local gates (Windows-native and Linux-native)

Commands, from the repository root with `GOWORK=off` (a parent `go.work` excludes
this module):

```bash
gofmt -l <all .go files>            # empty
go build ./...                      # exit 0
go vet ./...                        # exit 0
go test ./... -count=1              # all packages ok
go test -race ./internal/provider/ ./internal/providerapp/ -count=1   # ok
```

Results:

- Windows-native Go 1.26.6: `gofmt` clean, build PASS, vet PASS, all package
  tests PASS; Provider protocol suite 204 cases PASS incl. `-race` (89.3%
  statement coverage of `internal/provider`).
- Linux-native Go 1.26.8 (same working tree mounted in WSL): `gofmt` clean,
  build PASS, vet PASS, all package tests PASS
  (`scripts/acceptance/wsl-go-gates.sh` → `ALL_LINUX_GATES_PASS`).

New P1 package test results (loopback-only): `internal/schemacheck`,
`internal/sbi/executor`, `internal/sbi/serviceconfig`, `internal/catalog`,
`internal/providerapp` all PASS. The generic executor covers JSON, single
binary and multipart/related request/response payloads, header isolation, path
escaping, ProblemDetails vs malformed responses, cancellation, idempotency
gate and the catalog→executor bridge.

## 4. Real Provider E2E (live OpenAMSM)

Runner: `scripts/acceptance/provider_e2e_test.go` (build tag `acceptance`),
driven on Windows by

```powershell
# CA_ACCEPTANCE_* env vars set by the operator from the deployed config
go test -tags acceptance ./scripts/acceptance/ -count=1 -v
```

and on Linux by `scripts/acceptance/wsl-provider-e2e.sh`. The runner starts the
compiled binary as a real child process (`ca-client-tools provider serve`),
drives its NDJSON stdin/stdout protocol, and separately runs the compiled CLI.

Windows-native run (compiled `.exe` from the branch, WSL-hosted NFs through the
validated loopback topology):

```
PASS TestProviderV2AllReadTools
PASS TestProviderV2ProblemDetailsAndErrorBoundaries
PASS TestProviderV2CancellationDeadlineAndFraming
PASS TestProviderV2ConcurrentCalls
PASS TestProviderV1Compatibility
PASS TestProviderCleanEOFShutdown
PASS TestCLIRegressionLive
PASS TestCLIHelpSurfaceUnchanged
8/8 PASS
```

Linux-native run (compiled Linux binary, same live NFs): the same eight tests
`8/8 PASS`.

Assertions covered:

- **All eleven tools through the Provider (v2 protocol):** catalog returns all
  eleven supported IDs in request order with descriptors that contain no
  origin/URL/method/path/header/token/serviceKey; every invoke returns HTTP 200
  with the typed response (eight MAS envelopes with exact echoed
  SUpi/PSI/masId and AVAILABLE/UNAVAILABLE semantics; three list pages with
  validated `items`/`pager`); live AMF UE, AMF gNB and SMF PDU list membership
  for the lab UE.
- **Remote ProblemDetails:** wrong PSI → protocol-success
  `httpStatus:404, problem:{status:404, cause:"SESSION_NOT_FOUND"}` with no
  owner-outcome fabrication.
- **Error boundaries:** invalid PSI (>255) and undeclared argument →
  `BACKEND_ARGUMENTS_INVALID`; unknown backend ID →
  `BACKEND_OPERATION_UNSUPPORTED`; a subsequent valid invoke still succeeds
  (process survives request failures).
- **Deadline/cancel:** expired absolute deadline → `CALL_DEADLINE_EXCEEDED`
  without dispatch; in-flight cancel → `CANCEL_REQUESTED`, original request
  exactly one terminal response (`CALL_CANCELLED` or a valid completed result
  in the allowed race), second cancel → `ALREADY_COMPLETED`, unknown target →
  `NOT_FOUND`.
- **Framing/idempotency of IDs:** unknown envelope field →
  `PROTOCOL_FRAME_INVALID`; duplicate request ID → `PROTOCOL_REQUEST_ID_INVALID`
  for the second frame while the first still succeeds; service continues after
  both.
- **Concurrency:** eight simultaneous invokes (four MAS + four list) complete
  with one correlated terminal response each.
- **v1 compatibility:** v1 initialize/catalog returns only the eight frozen
  SMF backend IDs, the AMF list ID is `NOT_IMPLEMENTED`, no v2 fields appear,
  invoke works, and a v2 frame in a v1 process fails with
  `PROTOCOL_VERSION_UNSUPPORTED`.
- **Lifecycle:** closing stdin exits `0`; stdout contains protocol frames only.
- **CLI regression:** `--help` surface/tree unchanged (Provider command hidden);
  AMF UE/gNB and SMF PDU list; all eight SMF context queries with exact
  `type`/`status`/key echo; wrong-PSI negative returns exit `4`,
  `OPENAMSM_HTTP_ERROR`, HTTP 404, cause `SESSION_NOT_FOUND`; invalid PSI
  returns exit `2`.

## 5. Explicit non-claims

- Host/Runtime integration, public ToolCatalog publication, Capability/Scope
  authorization, Owner Outcomes and Evidence are **not** exercised here; those
  are separate repositories. The Provider is a deterministic private backend.
- No standardized 3GPP SBI operation was executed end-to-end: the catalog ships
  no production `genericSBI` binding. The generic executor is
  `LOCAL_ACCEPTANCE_PASS` only.
- No mutation was dispatched. The executor/catalog have the effect/idempotency
  abstraction; no production mutation operation or authorization exists.
- Binary/multipart **Provider** payload dispatch is `NOT_IMPLEMENTED` and
  `BLOCKED` on the Host Artifact Store/HostArtifactRef contract. The payload
  engine and generic executor payload paths are separately tested at library
  level; this is not a live binary/multipart acceptance.
- The real lab always returned `AVAILABLE` for the eight context projections;
  live `UNAVAILABLE` remains `NOT_RUN` (no approved fault injection), as does a
  cross-UE fault-injection rehearsal (client contract tests cover rejection).
- GitHub Actions: **`CI_UNAVAILABLE`** — the job does not start because of the
  recorded GitHub account billing/spending-limit annotation. This is neither a
  CI PASS nor a code failure. Local gates above are the acceptance evidence.

## 6. Per-tool status

| Tool | Implementation | Local | Real E2E |
|---|---|---|---|
| AMF `GET /ue-info` via Provider | `openamsm.amf.ue-info.list` → metrics client | PASS | PASS |
| AMF `GET /gnb-info` via Provider | `openamsm.amf.gnb-info.list` → metrics client | PASS | PASS |
| SMF `GET /pdu-info` via Provider | `openamsm.smf.pdu-info.list` → metrics client | PASS | PASS |
| SMF SC context | `openamsm.smf.mas.sc.query` → typed MAS client | PASS | PASS |
| SMF PA context | `openamsm.smf.mas.pa.query` | PASS | PASS |
| SMF UPN context | `openamsm.smf.mas.upn.query` | PASS | PASS |
| SMF SSM context | `openamsm.smf.mas.ssm.query` | PASS | PASS |
| SMF IAM context | `openamsm.smf.mas.iam.query` | PASS | PASS |
| SMF UPS context | `openamsm.smf.mas.ups.query` | PASS | PASS |
| SMF CA context | `openamsm.smf.mas.ca.query` | PASS | PASS |
| SMF SAA context | `openamsm.smf.mas.saa.query` | PASS | PASS |
| UPF session inspection | NOT_IMPLEMENTED (no deployed endpoint; unmerged env patch only) | — | BLOCKED |

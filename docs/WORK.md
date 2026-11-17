# Work

## Overall

Design: ACCEPTED for the existing OpenAMSM client/CLI v1 baseline and the
generic SBI Client v1 architecture (h2c HTTP/2 transport, static service
configuration and the private OpenAMSM header-token profile, trusted Operation Catalog, CA-Agent Tool Provider,
Host-owned BinaryRef and JSON/binary/multipart support).

Implementation: COMPLETE for the PR #1 read-only scope and the unified
Provider/Catalog/transport convergence in branch
`feat/unified-provider-read-tools-20261117`. SMF context-query, AMF/SMF metrics
clients and CLI v1 are implemented; the typed SMF adapter now uses the shared
`internal/sbi` h2c transport; the JSON/binary/multipart Payload Engine is
merged via PR #3. The trusted Operation Catalog (frozen v1 + additive v2
covering all eleven read-only tools), the resident `ca-client-tools provider
serve` process (Provider Process Protocol v1 + v2 read-tool extension), the
generic SBI executor, the schema validator and the trusted multi-service
configuration loader are implemented and locally tested on this branch. The
Provider was exercised against the live pinned OpenAMSM lab for all eleven
tools (v1 and v2 modes) plus cancellation, deadline, concurrency, framing,
ProblemDetails and CLI regression. Binary/multipart Provider payloads remain a
design-only extension blocked on the Host Artifact Store contract.

Acceptance (2026-11-17, two hosts; final round on PR #3 validation head
`0b80d33`): Go 1.26.6 `gofmt`, `go test ./...`, `go vet ./...` and native build
PASS natively on Windows (5 packages ok + `cmd`), and Go 1.26.8 runs the same
gates in the Linux/WSL checkout including a real symlink fixture. Compiled-CLI
contract acceptance PASS (29 cases). A real OpenAMSM deployment built from the
pinned snapshot, with 11 NFs and a real UERANSIM UE, was queried with the
compiled CLI: AMF UE List, AMF gNB List, SMF PDU List and all eight SMF
context queries PASS. The Windows-native `.exe` was additionally run against
the WSL-hosted NFs through the hardened optional loopback forwarder: 23/23
strict cases PASS, asserting exact JSON status/type, echoed identity keys and
list membership for positive cases and HTTP status/cause/exit plus no-request
semantics for the negative matrix (`scripts/acceptance/windows-e2e.ps1`; see
[docs/e2e-acceptance.md](e2e-acceptance.md) §3 and §7). The optional
forwarder's fixed loopback binding is unit-tested (13/13 on Windows and WSL)
and its lifecycle test PASSes in WSL. The context pass follows an
operator-authorized lowercase token lookup in a local `env-openamsm` build
overlay; the imported upstream tree remains unchanged. GitHub Actions:
CI_UNAVAILABLE — the job never started because of a GitHub billing/spending
limit annotation, which is not a test failure. Other NF operations are not
implicitly enabled by the generic architecture. Evidence:
[Windows/WSL acceptance](evidence/windows-wsl-acceptance-20261117.md) §10.

**Read-only delivery status:** the source implementation and fixture
coverage are complete for AMF /ue-info and /gnb-info, SMF /pdu-info, and all
eight OpenAMSM SMF context-query routes. The unsupported UPF `/session-info`
extension is removed. The typed SMF adapter speaks h2c directly and binds the
echoed SUPI/PSI/IP to the request. Live list acceptance PASS. Live context
acceptance PASS after the local build overlay added the lowercase token
lookup; all eight types returned AVAILABLE with matching echoed identity.
Windows-native acceptance PASS from the compiled `.exe` (lists, eight
contexts, identity/IP variants and negative token/identity probes, h2c prior
knowledge confirmed). The overlay is in `env-openamsm` and is not part of
this repository. Evidence: [real acceptance](evidence/real-openamsm-acceptance-20261117.md),
[Windows/WSL acceptance](evidence/windows-wsl-acceptance-20261117.md).

PR #1 (CLI v1/read clients) and PR #4 are merged to main. PR #2 (shared h2c SBI
transport and static token-header profile) merged to main as `189deff`
(independent final review COMMENTED with MERGE_READY, no blockers). PR #3
(Payload Engine, acceptance tooling and docs) is the final v1 slice; its base
is now `main` and its validation head is `0b80d33`, with the final docs commit
immediately above it. PR #3 merges only after the gates in the evidence
record. CI remains unavailable. The SBI architecture document is the normative
baseline; its operation catalog and Tool Provider slices are future work.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM deployment target and repository ownership | ACCEPTED | DOCUMENTED | Review pending | README.md; docs/architecture.md |
| Existing SMF context query client | ACCEPTED (OpenAMSM v0.10 wire contract) | IMPLEMENTED with typed h2c transport and response binding | Local contract PASS; live PASS (post-fix, authorized overlay) | internal/openamsm/smf/mas |
| AMF /ue-info and /gnb-info metrics clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| SMF /pdu-info metrics client | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED | Go 1.26.8 + local PASS; CI_UNAVAILABLE | docs/cli-contract.md; internal/cli |
| Generic Go h2c SBI HTTP/2 transport | ACCEPTED | IMPLEMENTED on PR #2 (merged, `189deff`) | Windows-native + WSL/Linux format/test/vet/build PASS; strict Windows E2E PASS | internal/sbi/client.go; internal/sbi/client_test.go |
| SMF context-query migration to shared h2c transport | ACCEPTED | IMPLEMENTED on `feat/unified-provider-read-tools-20261117`; existing regression suite passes unchanged and a transport-parity test proves the shared client is used | Windows-native + WSL/Linux go test PASS | internal/openamsm/smf/mas/transport_sbi.go |
| Static service configuration and OpenAMSM query-token header profile | ACCEPTED; h2c only; custom TLS/OAuth excluded | IMPLEMENTED on PR #2 (merged, `189deff`); trusted multi-service YAML loader added on this branch | Windows-native + WSL/Linux format/test/vet/build PASS; strict Windows E2E PASS | internal/sbi/config.go; internal/sbi/serviceconfig |
| Trusted Operation Catalog and OpenAPI binding | ACCEPTED | IMPLEMENTED on this branch: frozen v1 catalog + additive v2 (11 read-only bindings), strict YAML loader, digest and frozen-inventory validation | Catalog tests PASS; provider catalog E2E PASS live | contracts/provider-bindings; internal/catalog |
| CA-Agent preinstalled Go Tool Provider subprocess contract and Skill-scoped dynamic registration | ACCEPTED | IMPLEMENTED on this branch: `provider serve` Factor v1/v2, initialize/catalog/invoke/cancel, concurrency, deadline, cancellation, drain | 204 protocol cases PASS incl. `-race`; live Provider E2E all 11 tools PASS | internal/provider; internal/providerapp |
| Generic SBI executor, JSON-Schema subset validator and trusted service configuration | ACCEPTED | IMPLEMENTED on this branch | Loopback h2c tests PASS; live standardized-SBI operation NOT_RUN (no source-verified deployment operation) | internal/sbi/executor; internal/schemacheck; internal/sbi/serviceconfig |
| Provider binary/multipart payload extension | DESIGN ONLY | NOT_IMPLEMENTED; blocked on Host Artifact Store contract | NOT_RUN | docs/provider-payload-extension-v1.md |
| BinaryRef resolver/sink contract | ACCEPTED; Host owns storage/lifecycle | IMPLEMENTED on PR #3 (final v1 slice) | Windows-native + WSL/Linux format/test/vet/build PASS | internal/sbi/payload/files.go |
| JSON, binary and multipart/related Payload Engine | ACCEPTED | IMPLEMENTED on PR #3 (final v1 slice) | Windows-native + WSL/Linux format/test/vet/build PASS; live operation NOT_RUN | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go |
| Optional WSL loopback acceptance forwarder | ACCEPTED; acceptance-only, WSL-mirrored networking only, fixed `127.0.0.1` | IMPLEMENTED on PR #3 | Binding unit tests 13/13 PASS (Windows + WSL); WSL lifecycle test PASS | scripts/acceptance/wsl-tcp-forward.{sh,py}; scripts/acceptance/test_wsl_tcp_forward.py |
| Windows-native CLI → WSL-hosted real OpenAMSM E2E | ACCEPTED (strict assertion matrix, docs/e2e-acceptance.md §3) | N/A (acceptance of the existing CLI) | PASS — 23/23 strict JSON assertions | scripts/acceptance/windows-e2e.ps1 |
| Operator-only curl-like raw request CLI extension | OPTIONAL FUTURE WORK; not part of CLI v1 | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| PFCP/N4 | Direct N4 transport is separate from SBI | No direct PFCP client in this repository | Out of scope | docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md |

## Unified Provider / Catalog implementation (2026-11-17 branch)

Delivered on `feat/unified-provider-read-tools-20261117` from latest `main`:

- **Process**: `ca-client-tools provider serve` (hidden machine subcommand; CLI
  v1 help/tree unchanged) implements Provider Process Protocol v1 exactly and
  the additive v2 read-tool extension. Strict NDJSON framing, duplicate-key and
  unknown-field rejection, negotiated frame/in-flight/duration limits,
  initialize/catalog/invoke/cancel, out-of-order correlation, cancellation,
  deadline, bounded concurrency, safe error registry, EOF drain and no replay.
- **Catalog**: frozen `providers-bindings/v1` artifact preserved byte-for-byte
  (SHA-256 `cbb8de35…`), plus `provider-bindings/v2` covering all eleven
  read-only backend operations (SHA-256 `b2de218a…`). Fault-injection and
  tamper tests cover strict YAML decoding, source pins, argument profiles,
  schema keyword closure, generic binding validation and digest stability.
- **Tools**: AMF UE/gNB list, SMF PDU list and all eight SMF context queries
  dispatch to the existing typed clients (`internal/openamsm/*`). The CLI and
  the Provider share the same clients and validation; there is no duplicate
  business path.
- **Transport**: the SMF MAS client now uses the shared `internal/sbi`
  transport (h2c, HTTP/2 required, disabled redirects/proxy, bounded I/O,
  cancellation); the existing regression suite passes unchanged and a parity
  test proves the shared transport is in use and connection reuse works.
- **P1 infrastructure**: generic SBI executor (JSON/binary/multipart, h2c),
  JSON-Schema-subset validator and trusted multi-service configuration loader
  are implemented and locally tested. No production `genericSBI` binding is
  shipped: a standardized 3GPP operation requires pinned-OpenAPI and
  deployment/source evidence first. The catalog→executor bridge is implemented
  and tested, including a mutation-idempotency gate; no mutation is enabled.
- **Payload extension**: binary/multipart Provider protocol is design-only and
  blocked on the Host-owned `HostArtifactRef`/Artifact Store contract (no such
  API is invented or claimed implemented).

Validation (recorded in [provider acceptance evidence](evidence/provider-acceptance-20261117.md)):
Windows-native (Go 1.26.6) and Linux/WSL-native (Go 1.26.8)
`gofmt`/`go test ./... -count=1`/`go vet ./...`/`go build ./...` PASS; Provider
protocol suite 204 cases PASS including `-race`; real live Provider E2E against
the pinned OpenAMSM lab PASS for all eleven tools in v2 mode plus v1
compatibility, ProblemDetails, invalid arguments, expired deadline,
cancel/race, framing errors, duplicate request IDs, concurrency and clean EOF
shutdown; live CLI regression PASS (help/tree unchanged, lists + eight context
queries + negative matrix). Both the Windows-native binary and a Linux-native
binary were exercised. CI remains `CI_UNAVAILABLE` (GitHub billing/spending
limit annotation), not a PASS.

## Existing blockers

- **SMF context-query token header (resolved in the local lab).** The original
  handler looked up `X-OpenAMSM-Query-Token` in a case-sensitive header hash,
  while HTTP/2 requires lowercase field names. The operator authorized a
  lowercase lookup in a local `env-openamsm` build overlay; all eight real
  context queries now PASS. The overlay must be adopted by `env-openamsm` for
  any other deployment. The alternative-transport investigation confirmed
  there was no existing HTTP/1.1, proxy or gateway workaround. See the
  [real acceptance evidence](evidence/real-openamsm-acceptance-20261117.md)
  and [transport investigation](evidence/transport-alternative-investigation-20261117.md).
- GitHub Actions: **CI_UNAVAILABLE**. The `Go CI` job never started; the
  annotation is "The job was not started because recent account payments have
  failed or your spending limit needs to be increased." This is GitHub
  billing/infrastructure state, not a test failure and not a CI PASS. Local
  Windows-native and WSL/Linux validation is recorded as PASS.

## SBI architecture implementation handoff

[docs/sbi-client-architecture.md](sbi-client-architecture.md) is the
normative design. [docs/DEEPSEEK-HANDOFF.md](DEEPSEEK-HANDOFF.md) records the
implementation sequence and acceptance criteria.

The implementation must preserve these boundaries:

- ca-client-tools provides the preinstallable Tool Provider and concrete
  operation executors; it does not own Host policy or Runtime authorization.
- the Host dynamically activates only trusted operations required by the
  active Skill; a Skill cannot submit arbitrary Operation Manifests or URLs.
- static service configuration and the existing OpenAMSM header-token profile
  are sufficient for this scope; custom TLS and OAuth clients are excluded.
- BinaryRef storage, access control, expiry and cleanup are Host-owned.
- the Client streams binary payloads; it does not base64-encode them for the
  model or create a second artifact store.
- multipart/related follows 3GPP root-first JSON, Content-ID and
  RefToBinaryData semantics.
- individual SBI mutations are allowed when explicitly catalogued and
  authorized; multi-step orchestration, pending-operation lifecycle,
  callbacks and Runtime state remain outside this repository.
- PFCP/N4 remains separate from SBI.

## Next actions

1. Adopt the lowercase token lookup in the `env-openamsm` release (the local
   lab uses the documented build overlay).
2. Keep the real acceptance evidence current when the lab is rebuilt; the
   eight context types and identity checks already PASS.
3. The live server cannot be induced to return another UE's data without a
   fault injection; UE-A/UE-B rejection remains covered by the client contract
   tests. Add a fault-injection rehearsal only if a separate test environment
   is approved.
4. ~~Validate the shared h2c transport on PR #2 and the Payload Engine on PR #3
   with Go 1.26.8 tests, vet, formatting and native build.~~ DONE: PR #2 is
   merged and the PR #3 validation head passes Windows-native (Go 1.26.6) and
   WSL/Linux (Go 1.26.8) format/test/vet/build plus the strict Windows-native
   E2E 23/23; see
   [Windows/WSL acceptance](evidence/windows-wsl-acceptance-20261117.md) §10.
5. Complete the final review of PR #3 and merge it after the gates in the
   evidence record.
6. ~~Migrate the existing SMF context-query adapter to the shared transport only
   after exact regression tests preserve its token, identity, status and
   ProblemDetails behavior.~~ DONE on
   `feat/unified-provider-read-tools-20261117`: existing regression suite
   unchanged + shared-transport parity tests.
7. ~~Implement the trusted Operation Catalog and CA-Agent Tool Provider
   descriptor/invocation contract.~~ DONE on the same branch: catalog v1/v2,
   resident Provider v1/v2, all eleven read-only tools, live E2E, generic
   executor, service config. Binary/multipart Provider payloads and Host
   artifact dispatch remain blocked/design-only.
8. Add a first multipart operation only after its exact pinned 3GPP OpenAPI
   definition, OpenAMSM deployment support and Host artifact contract are
   verified.
9. Integrate the Provider and Host-owned BinaryRef resolver in ca-agent as a
   separate repository workstream.

## Authority / evidence links

- [OpenAMSM source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/docs/openamsm-source-findings.md)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- [3GPP TS 29.500](https://www.etsi.org/deliver/etsi_ts/129500_129599/129500/18.08.00_60/ts_129500v180800p.pdf)
- [3GPP TS 29.518](https://www.etsi.org/deliver/etsi_ts/129500_129599/129518/18.07.00_60/ts_129518v180700p.pdf)
- [3GPP TS 29.502](https://www.etsi.org/deliver/etsi_ts/129500_129599/129502/18.08.00_60/ts_129502v180800p.pdf)
- [Official 3GPP 5G APIs repository](https://forge.3gpp.org/rep/all/5G_APIs)
- docs/architecture.md
- docs/sbi-client-architecture.md
- docs/cli-contract.md
- docs/openamsm-interface-inventory.md

The current canonical `env-openamsm` pin is
`594dec78b95bff4fdc34deac8a3a9681fb41228f` (verified clean in WSL).
`docs/openamsm-interface-inventory.md` and
`docs/evidence/real-openamsm-acceptance-20261117.md` record the earlier
`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb` inspection and are retained as
historical inspection records.

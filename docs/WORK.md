# Work

## Overall

Design: ACCEPTED for the existing OpenAMSM client/CLI v1 baseline and the
generic SBI Client v1 architecture (h2c HTTP/2 transport, static service
configuration and the private OpenAMSM header-token profile, trusted Operation Catalog, CA-Agent Tool Provider,
JSON/binary/multipart Payload Engine support; HostArtifactRef Store/API remains a separate draft contract).

Implementation: COMPLETE for the PR #1 read-only CLI scope. SMF context-query,
AMF/SMF metrics clients and CLI v1 are implemented; the typed SMF adapter uses
cleartext HTTP/2 prior knowledge (h2c) and strict response-key binding. The
shared h2c SBI transport and trusted OpenAMSM token-header profile are merged
via PR #2. The streaming JSON/binary/multipart Payload Engine and root-confined
staged-file Resolver/Attachment Sink primitives are merged via PR #3
(`4c424ee`); the HostArtifactRef Store/API is not implemented. The resident Provider process,
general 3GPP operation-binding loader and Host backend integration are not yet
implemented. PR #5 is a Draft documentation PR freezing the first private SMF
backend bindings and JSON-only Provider Process Protocol v1 profile. The CLI
v1 remains an operator/test interface; the Host MUST NOT spawn it per Tool
invocation.

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

PR #1 (CLI v1/read clients), PR #2 (shared h2c SBI transport and trusted
token-header profile, merge `189deff`), PR #3 (Payload Engine and acceptance
tooling, merge `4c424ee`) and PR #4 are merged to main. PR #3 passed the
recorded Windows-native and WSL/Linux local gates; the live Payload Engine
operation remains NOT_RUN because no operation is yet wired through the
Provider. GitHub Actions remains CI_UNAVAILABLE for the recorded billing/
spending-limit reason. PR #5 is open and Draft; it adds documentation contracts
for the first private SMF Provider slice only. The normative SBI architecture
continues to support individual READ and MUTATION operations and JSON, binary
and multipart payloads; the Provider runtime and general 3GPP operation
bindings remain implementation work.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM deployment target and repository ownership | ACCEPTED | DOCUMENTED | Review pending | README.md; docs/architecture.md |
| Existing SMF context query client | ACCEPTED (OpenAMSM v0.10 wire contract) | IMPLEMENTED with typed h2c transport and response binding | Local contract PASS; live PASS (post-fix, authorized overlay) | internal/openamsm/smf/mas |
| AMF /ue-info and /gnb-info metrics clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| SMF /pdu-info metrics client | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED | Go 1.26.8 + local PASS; CI_UNAVAILABLE | docs/cli-contract.md; internal/cli |
| Generic Go h2c SBI HTTP/2 transport | ACCEPTED | IMPLEMENTED on PR #2 (merged, `189deff`) | Windows-native + WSL/Linux format/test/vet/build PASS; strict Windows E2E PASS | internal/sbi/client.go; internal/sbi/client_test.go |
| SMF context-query migration to shared h2c transport | P1 convergence gate before general Operation Catalog; not a first-slice blocker | NOT_STARTED; transport validated, exact regression/migration review not done | NOT_RUN (gated) | docs/sbi-client-architecture.md §10; internal/openamsm/smf/mas |
| Static service configuration and OpenAMSM query-token header profile | ACCEPTED; h2c only; custom TLS/OAuth excluded | IMPLEMENTED on PR #2 (merged, `189deff`) | Windows-native + WSL/Linux format/test/vet/build PASS; strict Windows E2E PASS | internal/sbi/config.go; internal/sbi/client.go |
| Multi-service trusted configuration-file schema/loader | Illustrative architecture example only; field-level contract NOT_FROZEN | NOT_IMPLEMENTED; Provider v1 uses allowlisted environment variables | NOT_RUN | docs/sbi-client-architecture.md §5.2; overnight execution goal |
| General 3GPP Operation Catalog / pinned OpenAPI bindings | Architecture ACCEPTED; field-level machine contract NOT_FROZEN | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md; docs/sbi-implementation-backlog-20261117.md |
| Provider Backend Binding Catalog v1 schema and first private SMF bindings | FROZEN FOR IMPLEMENTATION in PR #5 | Exact schema/semantic rules documented; YAML artifact present; loader NOT_STARTED | NOT_RUN | docs/provider-backend-binding-catalog-v1.md; contracts/provider-bindings/v1/provider-bindings.yaml |
| Provider Process Protocol v1 (first SMF JSON Observation profile) | FROZEN FOR IMPLEMENTATION in PR #5 | Resident Provider NOT_STARTED | NOT_RUN | docs/provider-protocol-v1.md |
| Host-owned public callable-to-backend adapter and Host/Runtime dispatch | ca-agent-demo PR #208/#214 contracts | Host integration NOT_IMPLEMENTED | NOT_RUN | ca-agent-demo PR #214 |
| Provider staged-file Resolver/Sink primitives | ACCEPTED; Host supplies private roots and owns durable artifact lifecycle | IMPLEMENTED and merged on PR #3 | Windows-native + WSL/Linux format/test/vet/build PASS | internal/sbi/payload/files.go |
| HostArtifactRef Schema, Store/API and lifecycle | ca-agent-demo PR #211 design draft; physical schema/API not frozen | NOT_IMPLEMENTED | NOT_RUN | ca-agent-demo PR #211 |
| JSON, binary and multipart/related Payload Engine | ACCEPTED | IMPLEMENTED and merged on PR #3 | Windows-native + WSL/Linux format/test/vet/build PASS; live operation NOT_RUN | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go |
| Optional WSL loopback acceptance forwarder | ACCEPTED; acceptance-only, WSL-mirrored networking only, fixed `127.0.0.1` | IMPLEMENTED on PR #3 | Binding unit tests 13/13 PASS (Windows + WSL); WSL lifecycle test PASS | scripts/acceptance/wsl-tcp-forward.{sh,py}; scripts/acceptance/test_wsl_tcp_forward.py |
| Windows-native CLI → WSL-hosted real OpenAMSM E2E | ACCEPTED (strict assertion matrix, docs/e2e-acceptance.md §3) | N/A (acceptance of the existing CLI) | PASS — 23/23 strict JSON assertions | scripts/acceptance/windows-e2e.ps1 |
| Operator-only curl-like raw request CLI extension | OPTIONAL FUTURE WORK; not part of CLI v1 | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| PFCP/N4 | Direct N4 transport is separate from SBI | No direct PFCP client in this repository | Out of scope | docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md |

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

The checked [cross-repository SBI implementation backlog](sbi-implementation-backlog-20261117.md) tracks WP0–WP15, priority, implementation gaps and local acceptance. The Host-owned [Public Callable ID Naming Contract v1](https://github.com/odango-chan/ca-agent-demo/blob/docs/v1alpha4-callable-runtime-contract-20261117/docs/design/public-callable-naming-contract-v1.md) now defines the general NF-first grammar, the generic `smf.session.context.query` ID, the eight `smf.mas.<projection>.context.query` projections and inactive future 5GC naming candidates. The retired `context.smf.session.read` is rejected; private Provider backend IDs, Capability IDs, package aliases and 3GPP source `operationId` values are not renamed.

[docs/sbi-client-architecture.md](sbi-client-architecture.md) is the
normative design. The h2c transport and JSON/binary/multipart Payload Engine
are merged. The PR #5 Provider v1 contract is limited to the first JSON-only
SMF Observation slice; it does not narrow the general SBI architecture.
[docs/DEEPSEEK-HANDOFF.md](DEEPSEEK-HANDOFF.md) records the implementation
sequence and acceptance criteria.

The implementation must preserve these boundaries:

- ca-client-tools provides the preinstallable Tool Provider and concrete
  operation executors; it does not own Host policy or Runtime authorization.
- the Host dynamically activates only trusted operations required by the
  active Skill; a Skill cannot submit arbitrary Operation Manifests or URLs.
- static service configuration and the existing OpenAMSM header-token profile
  are sufficient for this scope; custom TLS and OAuth clients are excluded.
- HostArtifactRef storage, access control, expiry and cleanup are Host-owned; the Host Store/API remains a draft contract in ca-agent-demo PR #211.
- the Client streams binary payloads; it does not base64-encode them for the
  model or create a second artifact store.
- multipart/related follows 3GPP root-first JSON, Content-ID and
  RefToBinaryData semantics.
- the JSON-only Provider Process Protocol v1 is a first-SMF-slice boundary;
  binary operations use a separately versioned Provider payload extension
  over Host-owned staging and the already merged Payload Engine.
- individual SBI mutations are allowed when explicitly catalogued and
  authorized; multi-step orchestration, pending-operation lifecycle,
  callbacks and Runtime state remain outside this repository.
- PFCP/N4 remains separate from SBI.

## Next actions

The execution authority is [the overnight implementation goal](overnight-execution-goal-20261117.md). Keep the first deliverable narrowly focused on `PROVIDER_HOST_DISPATCH_IMPLEMENTATION_PASS`.

1. **P0 — Provider:** implement the strict private binding-catalog loader and resident `provider serve` process using Provider Process Protocol v1; reuse the typed SMF client and preserve CLI v1.
2. **P0 — Runtime:** implement the frozen synchronous RuntimeHost.v2 HostBusinessCorrelation contract; preserve RuntimeHost.v1 and do not add Temporal/async continuation.
3. **P0 — Host:** implement the inherited Operation/Observation callable admission and Scope path for the v1alpha5 target. Use ca-agent-demo PR #214 only as the v1alpha4 inherited-stage design reference; if v1alpha5 artifact/admission changes are needed, define them in the Host-owned repository and do not claim full v1alpha5 support from #214 alone.
4. **P0 — acceptance:** execute a real diagnostic query through Host → Runtime → Provider → pinned OpenAMSM, plus negative authorization, identity, cancellation and process-lifecycle tests.
5. **P1 — transport convergence:** migrate the typed MAS client to shared `internal/sbi` only after exact parity tests. This is required before general 3GPP Operation Catalog onboarding, not a first-slice blocker.
6. **P1 — general SBI design:** define the field-level OpenAPI-derived Operation Catalog and trusted multi-service configuration-file contracts. Do not implement guessed schemas or generic HTTP dispatch.
7. **P1 — binary design:** complete the separately versioned Provider payload-transfer proposal only after the HostArtifactRef physical contract is accepted. Reuse the merged Payload Engine; do not add binary fields to Provider Protocol v1.
8. **Gated follow-ons:** add a Mutation or live binary operation only after exact pinned OpenAPI, selected OpenAMSM deployment support, Owner/Capability/Scope and idempotency/reconciliation contracts exist. Keep raw curl operator-only.
9. **External deployment follow-up:** adopt the lowercase query-token header lookup in the `env-openamsm` release. The current real CLI PASS uses the authorized local build overlay; this server-side change is not a ca-client-tools code task.
10. Keep real acceptance evidence current when the lab is rebuilt. Cross-UE live fault injection remains optional and requires a separately approved test environment; contract tests remain the current negative evidence.

## Authority / evidence links

- [OpenAMSM source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/docs/openamsm-source-findings.md)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- [3GPP TS 29.500 Release 19.7.1](https://www.etsi.org/deliver/etsi_ts/129500_129599/129500/19.07.01_60/ts_129500v190701p.pdf)
- [3GPP TS 29.518 Release 19.8.0](https://www.etsi.org/deliver/etsi_ts/129500_129599/129518/19.08.00_60/ts_129518v190800p.pdf)
- [3GPP TS 29.502 Release 19.7.0](https://www.etsi.org/deliver/etsi_ts/129500_129599/129502/19.07.00_60/ts_129502v190700p.pdf)
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


## Provider / Host Observation vertical slice (2026-11-17)

| Contract | Owner / design status | Implementation / acceptance |
|---|---|---|
| CA-SL v1alpha5 | ca-skills PR #61; full single-file Preview candidate, NOT RELEASED / NOT QUALIFIED | Next language target; v1alpha4 remains immutable inherited semantic baseline |
| Provider Process Protocol v1 | ca-client-tools; FROZEN FOR IMPLEMENTATION | Resident process NOT_IMPLEMENTED |
| SMF query backend | ca-client-tools; FROZEN FOR IMPLEMENTATION | Eight typed backend operations reuse the existing Go client; the generic session read and SC projection callable share the SC operation; Provider process NOT_IMPLEMENTED |
| Public generic session read | ca-agent-host; proposal in [PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208) | ToolCatalog publication, adapter and Evidence NOT_IMPLEMENTED / NOT_QUALIFIED |
| Eight projection-specific Observation callables | ca-agent-host; proposal in [PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208) | Host adapters and Evidence NOT_IMPLEMENTED / NOT_QUALIFIED |
| Host v1alpha5 inherited Observation stage and unified callable admission | ca-agent-demo PR #214 is v1alpha4-scoped inherited-stage design only; v1alpha5 admission/artifact support not specified there | NOT_IMPLEMENTED / contract gap must be tracked by Host owner |
| HostBusinessCorrelation / RuntimeHost.v2 propagation | PR #208 and cross-repository Runtime contract | Typed boundary not yet implemented |
| Node-entry Event Wait / Integration Ingress | ca-skills PR #61/#62; ca-agent-demo PR #210/#213 | v1alpha5 Preview contracts; not owned by ca-client-tools Provider; Compiler/Host/Runtime/Integration implementation NOT_QUALIFIED |
| HostArtifactRef | Separate draft [ca-agent-demo PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211) | Not required by JSON-only first slice |

The public Owner for all first-slice Observation callables is ca-agent-host.
ca-client-tools provides only the private Provider backend. The generic
`smf.session.context.query` callable is the initial UE/PSI presence check; it and
`smf.mas.sc.context.query` share the same private SC query operation but retain
distinct Host output/normalization contracts. The generic public name omits
`mas` and `openamsm`; only projection-specific callable IDs include `mas`, and
`openamsm` is reserved for private backend operation IDs. The other seven MAS
projection callables are optional compatibility interfaces.
HTTP 404 SESSION_NOT_FOUND is distinct from HTTP 200 UNAVAILABLE. The generic
callable may normalize the exact 404 to ABSENT; UNAVAILABLE means a session
exists but the queried projection has not been written.

The v0.10 SMF response has no source observedAt, eventTime or validUntil.
Host queryStartedAt and receivedAt are audit timestamps only. The Host Owner
Evidence Descriptors are not publishable and Evidence-dependent Predicate PASS
remains NOT_QUALIFIED. This does not block Provider process implementation
or raw query transport acceptance.

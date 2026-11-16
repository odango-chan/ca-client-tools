# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary, AMF/UPF read scope,
existing SMF context-query baseline and CLI v1 process contract. Implementation:
PARTIAL (AMF/SMF clients and the UPF client/CLI are staged; UPF source patch is
proposed). Acceptance: NOT_RUN. Other NF clients: DEFERRED. Roll-up: DEFERRED.

The read-only SMF context-query adapter, AMF `/ue-info` and `/gnb-info`,
SMF `/pdu-info`, and UPF `/session-info` client are implemented on the review
branch. The UPF endpoint depends on the CA-owned env-openamsm patch overlay;
it is not in the immutable source snapshot. Fixture coverage is authored for
the UPF client and CLI command. The CLI exposes the stable NF/resource command
tree, JSON stdin, success/error envelopes, stderr separation, stable exit
codes and cancellation handling. Go 1.26.2 CI, patch build and live-N4
acceptance are still pending.

The repository declares Go 1.26.2. A previous local run passed an earlier
revision under Go 1.23.2 with a temporary go.mod version override; latest
changes have not been validated on the declared toolchain. GitHub Actions
attempts have failed before exposing runner step records/logs. The authoritative
OpenAMSM source is imported in env-openamsm/main; metrics response shapes and
paging are source-verified.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM-only repository boundary | ACCEPTED | IMPLEMENTED (docs) | NOT_RUN | docs/architecture.md |
| Canonical 5GC NF map and 3GPP → Tool → Skill derivation | 3GPP responsibility mapped; source availability explicitly classified | DOCUMENTED; mandatory future-client gate defined | native/live acceptance NOT_RUN | docs/nf-capability-map.md |
| SMF context query client | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | Prior revision LOCAL_PASS (Go 1.23.2 override); latest changes NOT_RUN; Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas |
| AMF `/ue-info` and `/gnb-info` clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| SMF `/pdu-info` client | SOURCE CONTRACT VERIFIED (no N4 pair) | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED (`docs/cli-contract.md`) | IMPLEMENTED on review branch; validation pending | Go 1.26.2 CI / local test-vet-build NOT_VERIFIED | docs/cli-contract.md · internal/cli |
| UPF `/session-info` / exact N4 pair join | CA-owned patch contract defined from UPF internal PFCP state | CLIENT_IMPLEMENTED; patch overlay proposed in env-openamsm | PATCH_APPLIED / PATCH_BUILT / LIVE_ACCEPTED pending | internal/openamsm/metrics; upf session list |
| Other NF clients (UDM/AUSF/PCF/NRF/NSSF/CHF/UDR/BSF/NEF/AF/NWDAF/SCP/SEPP/UDSF/SMSF/5G-EIR/NSACF) | DEFERRED by user scope | NOT_STARTED | NOT_APPLICABLE in this phase | docs/nf-capability-map.md |

## Blockers

- The source archive is imported and hashed in `env-openamsm/main`; native
  build, OpenAMSM context-query source tests and live NF acceptance remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- The UPF read projection is staged as
  [env-openamsm PR #3](https://github.com/odango-chan/env-openamsm/pull/3).
  Its patch-application workflow failed before exposing any job steps/logs;
  patch application is therefore NOT_VERIFIED, not a source failure or a pass.
- Latest Go test/vet/build validation is outstanding because GitHub Actions
  jobs failed before exposing runner steps/logs; the latest branch changes
  have not been rerun on the declared Go toolchain.

## Implementation handoff

[`docs/DEEPSEEK-HANDOFF.md`](DEEPSEEK-HANDOFF.md) is now a reproducible
implementation and validation record for the CLI v1 work already staged. It
is not an open delegation to redesign the command tree. It does not authorize
Host/Runtime changes, NW Agent workflow/facade/orchestration/relay work, or
changes to typed client semantics.

The OpenAMSM source evidence is pinned to
[`env-openamsm` snapshot 77c0a2f8511c3498d1270f059881b3cf0f7f5eeb](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
The imported source has been statically inspected; no native build or live-NF
acceptance is implied by this documentation handoff.

## Next actions

1. Obtain a successful Go 1.26.2 `gofmt` / `go test ./...` / `go vet ./...` / build run.
2. Apply the env-openamsm patch overlay to a disposable source copy and build it.
3. Verify UPF `/session-info` against a real N4 session, including exact SMF
   F-SEID, UPF N4 SEID and PDR/FAR/QER/URR state.
4. Perform live-NF acceptance for the AMF and UPF read surfaces.
5. Integrate the CLI through the Host adapter under the existing ownership
   contract; expose only declared SMF context types to the Agent.
6. Keep all other NF client work deferred.

## Authority / evidence links

- [env-openamsm source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md)
- [env-openamsm UPF session patch PR #3](https://github.com/odango-chan/env-openamsm/pull/3)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- [SMF context source tests](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tests/smf-mas-query/)
- `docs/architecture.md`
- `docs/cli-contract.md`
- `docs/DEEPSEEK-HANDOFF.md`
- `docs/openamsm-interface-inventory.md`

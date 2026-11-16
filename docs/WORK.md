# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary, NF capability map,
SMF context query, source-native AMF/SMF metrics reads and CLI v1 process
contract. Implementation: PARTIAL (typed clients and CLI v1 command surface
are staged). Acceptance: NOT_RUN. Roll-up: DEFERRED.

The read-only SMF context-query adapter and AMF `/ue-info`, `/gnb-info` and
SMF `/pdu-info` page clients are implemented on the review branch with fixture
tests. The public CLI uses stable context types rather than OpenAMSM module IDs. The CLI now exposes the stable NF/resource command tree and public SMF
context types. JSON stdin, success/error envelopes, stderr separation, stable
exit codes and cancellation handling are staged. The latest Go 1.26.2 CI and
local validation are still pending.

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
| SMF context query client | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | Prior revision LOCAL_PASS (Go 1.23.2 override); latest changes NOT_RUN; Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas |
| AMF `/ue-info` and `/gnb-info` clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| SMF `/pdu-info` client | SOURCE CONTRACT VERIFIED (no N4 pair) | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED (`docs/cli-contract.md`) | IMPLEMENTED on review branch; validation pending | Go 1.26.2 CI / local test-vet-build NOT_VERIFIED | docs/cli-contract.md · internal/cli |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The source archive is imported and hashed in `env-openamsm/main`; native
  build, OpenAMSM context-query source tests and live NF acceptance remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- Latest Go test/vet/build validation is outstanding because GitHub Actions
  jobs failed before exposing runner steps/logs; the latest branch changes
  have not been rerun on the declared Go toolchain.

## Implementation handoff

The implementation handoff remains as a reproducible task/evidence record:
[`docs/DEEPSEEK-HANDOFF.md`](DEEPSEEK-HANDOFF.md). It is a single Goal-mode
implementation brief for CLI v1. It does not authorize Host/Runtime changes,
NW Agent workflow/facade/orchestration/relay work, or changes to typed client
semantics.

The OpenAMSM source evidence is pinned to
[`env-openamsm` snapshot 77c0a2f8511c3498d1270f059881b3cf0f7f5eeb](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).
The imported source has been statically inspected; no native build or live-NF
acceptance is implied by this documentation handoff.

## Next actions

1. Run `gofmt`, `go test ./...`, `go vet ./...` and native build with Go 1.26.2.
2. Obtain a successful CI run for the current branch.
3. Run OpenAMSM native build and context-query source tests against the imported snapshot.
4. Integrate the fixed CLI through the Host adapter; expose only declared SMF
   context types to the Agent through explicit Host/Runtime policy.
5. Keep UPF session inspection and N4 pair-dependent features out of scope
   unless OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- [env-openamsm source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- [SMF context source tests](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tests/smf-mas-query/)
- `docs/architecture.md`
- `docs/cli-contract.md`
- `docs/DEEPSEEK-HANDOFF.md`
- `docs/openamsm-interface-inventory.md`

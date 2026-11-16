# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary, SMF MAS Query,
source-native metrics reads and CLI v1 process contract. Implementation:
PARTIAL (typed clients and the initial flag-based CLI exist; CLI v1 migration
is pending). Acceptance: NOT_RUN. Roll-up: DEFERRED.

The read-only SMF MAS QueryContext client and AMF `/ue-info`, `/gnb-info`
and SMF `/pdu-info` page clients are implemented on the review branch with
fixture tests. An initial flag-based CLI is present, but it is not the final
CLI contract. `docs/cli-contract.md` defines CLI v1 using Cobra, a stable
resource/action command tree, JSON stdin for sensitive MAS lookup values,
success/error JSON envelopes, stderr separation and stable exit codes.
CLI v1 is DESIGN ACCEPTED and IMPLEMENTATION NOT_STARTED.

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
| SMF MAS Query client | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | Prior revision LOCAL_PASS (Go 1.23.2 override); latest changes NOT_RUN; Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas |
| AMF `/ue-info` and `/gnb-info` clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| SMF `/pdu-info` client | SOURCE CONTRACT VERIFIED (no N4 pair) | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED (`docs/cli-contract.md`) | NOT_STARTED (current CLI is pre-v1) | NOT_RUN | docs/cli-contract.md · internal/cli |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The source archive is imported and hashed in `env-openamsm/main`; native
  build, MAS source tests and live NF acceptance remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- Latest Go test/vet/build validation is outstanding because GitHub Actions
  jobs failed before exposing runner steps/logs; the latest branch changes
  have not been rerun on the declared Go toolchain.

## Next actions

1. Implement CLI v1 from `docs/cli-contract.md`: Cobra command tree, strict
   JSON stdin for MAS queries, JSON success/error envelopes, stderr separation
   and exit-code mapping. Do not change typed OpenAMSM client semantics.
2. Obtain a usable Go 1.26.2 test/vet/build run for the migrated branch.
3. Run OpenAMSM native build and MAS source tests against the imported snapshot.
4. Integrate the fixed CLI through the Host adapter; expose only SMF MAS
   callables to the Agent by explicit Host/Runtime policy.
5. Keep UPF session-info and N4 pair dependent features out of scope unless
   OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- `env-openamsm/docs/openamsm-source-findings.md`
- SMF MAS QueryContext SBI v0.10 design specification
- `docs/architecture.md`
- `docs/cli-contract.md`
- `docs/openamsm-interface-inventory.md`

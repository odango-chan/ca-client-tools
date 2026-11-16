# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary, SMF MAS Query and
source-native metrics reads. Implementation: PARTIAL (SMF MAS, AMF metrics and
SMF metrics clients/CLI implemented on the current branch). Acceptance:
NOT_RUN. Roll-up: DEFERRED.

The read-only SMF MAS QueryContext CLI and AMF `/ue-info`, `/gnb-info` and SMF
`/pdu-info` page clients are implemented with fixture tests. The repository
declares Go 1.26.2. A previous local run passed an earlier revision under Go
1.23.2 with a temporary go.mod override; the latest code changes have not yet
been rerun locally. GitHub Actions attempts failed before exposing any step
records or logs. The authoritative OpenAMSM source is imported in
env-openamsm/main; metrics response shapes and paging are source-verified.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM-only repository boundary | ACCEPTED | IMPLEMENTED (docs) | NOT_RUN | docs/architecture.md |
| SMF MAS Query client and CLI | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | Prior revision LOCAL_PASS (Go 1.23.2 override); latest changes NOT_RUN; Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas · internal/cli |
| AMF `/ue-info` and `/gnb-info` clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics · internal/cli |
| SMF `/pdu-info` client | SOURCE CONTRACT VERIFIED (no N4 pair) | IMPLEMENTED (current branch) | NOT_RUN | internal/openamsm/metrics · internal/cli |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The source archive is imported and hashed in `env-openamsm/main`; native
  build, MAS source tests and live NF acceptance remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- Latest Go test/vet/build validation is outstanding because GitHub Actions
  jobs currently fail before exposing runner steps/logs; the latest branch
  changes have not been locally rerun.

## Next actions

1. Obtain a usable Go 1.26.2 test/vet/build run for the current branch.
2. Run OpenAMSM native build and MAS source tests against the imported snapshot.
3. Integrate the fixed CLI through the Host default-Tool adapter; expose only
   SMF MAS callables to the Agent by explicit Host/Runtime policy.
4. Keep UPF session-info and N4 pair dependent features out of scope unless
   OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- `env-openamsm/docs/openamsm-source-findings.md`
- SMF MAS QueryContext SBI v0.10 design specification
- `docs/architecture.md`
- `docs/openamsm-interface-inventory.md`

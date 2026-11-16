# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary and initial SMF MAS
Query slice. Implementation: PARTIAL (SMF MAS client/CLI committed on the
current branch). Acceptance: NOT_RUN. Roll-up: DEFERRED.

The first read-only SMF MAS QueryContext CLI is implemented with fixture tests.
The repository declares Go 1.26.2. A previous local run passed gofmt/test/vet/build
under Go 1.23.2 with a temporary go.mod override, but the latest code changes
have not yet been rerun locally. GitHub Actions attempts failed before exposing
any step records or logs. The authoritative OpenAMSM source is now imported in
env-openamsm/main; AMF/SMF metrics response shapes and paging are source-verified.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM-only repository boundary | ACCEPTED | IMPLEMENTED (docs) | NOT_RUN | docs/architecture.md |
| SMF MAS Query client and CLI | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | LOCAL_PASS (Go 1.23.2 override); Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas · internal/cli |
| AMF `/ue-info` and `/gnb-info` clients | SOURCE CONTRACT VERIFIED | NOT_STARTED | NOT_RUN | docs/openamsm-interface-inventory.md |
| SMF `/pdu-info` client | SOURCE CONTRACT VERIFIED (no N4 pair) | NOT_STARTED | NOT_RUN | docs/openamsm-interface-inventory.md |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The source archive is imported and hashed in `env-openamsm/main`; native
  build, MAS source tests and live NF acceptance remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- AMF/SMF metrics clients remain unimplemented; source schema/paging review is
  complete, so implementation can proceed.

## Next actions

1. Obtain an executable Go test/vet/build run for the current SMF MAS client/CLI.
2. Add AMF `/ue-info`, `/gnb-info` and SMF `/pdu-info` bounded paginated clients.
4. Keep UPF session-info and N4 pair dependent features out of scope unless
   OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- `env-openamsm/docs/openamsm-source-findings.md`
- SMF MAS QueryContext SBI v0.10 design specification
- `docs/architecture.md`
- `docs/openamsm-interface-inventory.md`

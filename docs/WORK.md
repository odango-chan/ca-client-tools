# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary and initial SMF MAS
Query slice. Implementation: PARTIAL (SMF MAS client/CLI committed on the
current branch). Acceptance: NOT_RUN. Roll-up: DEFERRED.

The first read-only SMF MAS QueryContext CLI is implemented with fixture tests.
Local `gofmt`, `go test ./...`, `go vet ./...` and native build pass under Go
1.23.2 using a temporary local go.mod version override. The repository declares
Go 1.26.2; exact target-toolchain validation is still NOT_VERIFIED. GitHub
Actions attempts failed before exposing any step records or logs. AMF/SMF metrics clients await exact
response-schema verification against the OpenAMSM source archive.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM-only repository boundary | ACCEPTED | IMPLEMENTED (docs) | NOT_RUN | docs/architecture.md |
| SMF MAS Query client and CLI | ACCEPTED (v0.10 wire contract) | IMPLEMENTED (current branch) | LOCAL_PASS (Go 1.23.2 override); Go 1.26.2 / GitHub CI NOT_VERIFIED | internal/openamsm/smf/mas · internal/cli |
| AMF `/ue-info` and `/gnb-info` clients | PROPOSED | NOT_STARTED | NOT_RUN | OpenAMSM response schema pending |
| SMF `/pdu-info` client | PROPOSED | NOT_STARTED | NOT_RUN | OpenAMSM response schema pending |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The OpenAMSM archive is not yet imported into
  `env-openamsm/upstream/openamsm`; static source findings are not build or
  live acceptance evidence. Tracked by [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- AMF/SMF metrics response schemas and pagination must be verified from the
  actual archive before their clients are implemented.

## Next actions

1. Obtain an executable Go test/vet/build run for the current SMF MAS client/CLI.
2. Verify the archive import/source status with env-openamsm.
3. Add AMF/SMF metrics clients after exact source-schema inspection.
4. Keep UPF session-info and N4 pair dependent features out of scope unless
   OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- `env-openamsm/docs/openamsm-source-findings.md`
- SMF MAS QueryContext SBI v0.10 design specification
- `docs/architecture.md`
- `docs/openamsm-interface-inventory.md`

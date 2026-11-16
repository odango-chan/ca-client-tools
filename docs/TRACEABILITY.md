# Traceability

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM-only boundary and ownership | README.md; AGENTS.md; docs/architecture.md | Review | NOT_RUN |
| Eight SMF MAS Query paths and request/response envelope | internal/openamsm/smf/mas/client.go | internal/openamsm/smf/mas/client_test.go | Prior local Go 1.23.2 check PASS (temporary go.mod override); latest changes NOT_RUN; Go 1.26.2 / GitHub CI NOT_VERIFIED; live acceptance NOT_RUN |
| v0.10 identity/IP key combinations, 1–15 digit IMSI validation and PSI bounds | Query.requestBody | TestInvalidQueryKeysFailBeforeNetwork; TestQueryAllowsConsistentIdentityAndBothIPKeys; TestQueryAcceptsOpenAMSMIMSIUpToFifteenDigits; TestIMSIRejectsMoreThanFifteenDigits | latest local/CI validation NOT_RUN; live acceptance NOT_RUN |
| Query token handling, redirect rejection and bounded SBI transport | NewClient / Client.Query | TestQueryBuildsExactRequestAndPreservesAvailableResult; TestRedirectIsNotFollowed; TestOriginConfigurationIsStrict | latest local/CI validation NOT_RUN; live acceptance NOT_RUN |
| AVAILABLE / UNAVAILABLE distinction and source-accurate echoed key | parseResult / validateEchoKey | TestUnavailableIsSuccessfulAndRequiresEmptyContext; TestResponseKeyRejectsRequestOnlyIMSIField | latest local/CI validation NOT_RUN; live acceptance NOT_RUN |
| CLI v1 process contract (Cobra command tree, strict JSON stdin/stdout/stderr, exit codes) | docs/cli-contract.md | implementation pending; required test matrix in docs/cli-contract.md §8 | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED; acceptance NOT_RUN |
| CLI process cancellation and deadline propagation | docs/cli-contract.md §5–§6, §8–§9 | implementation and cancellation tests pending | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED; acceptance NOT_RUN |
| AMF and SMF metrics read clients | internal/openamsm/metrics/client.go; internal/cli/run.go | TestGetUEInfoPreservesSourceItems; TestGetGNBInfoValidatesSequentialPagination; TestGetPDUInfoPreservesOpenAMSMShapeWithoutN4Pair; TestMetricsRejectsTruncatedAndDuplicateJSON; TestRunAMFUEInfo; TestRunSMFPDUInfo | latest local/CI validation NOT_RUN; live acceptance NOT_RUN |
| UPF session-info and N4 pair join | Excluded | OpenAMSM source findings | NOT_APPLICABLE |

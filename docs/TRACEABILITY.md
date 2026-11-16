# Traceability

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM-only boundary and ownership | README.md; AGENTS.md; docs/architecture.md | Review | NOT_RUN |
| Eight SMF MAS Query paths and request/response envelope | internal/openamsm/smf/mas/client.go | internal/openamsm/smf/mas/client_test.go | CI pending; live acceptance NOT_RUN |
| Query-key exclusivity and PSI bounds | Query.requestBody | TestInvalidQueryKeysFailBeforeNetwork | CI pending; live acceptance NOT_RUN |
| Query token handling, redirect rejection and bounded SBI transport | NewClient / Client.Query | TestQueryBuildsExactRequestAndPreservesAvailableResult; TestRedirectIsNotFollowed; TestOriginConfigurationIsStrict | CI pending; live acceptance NOT_RUN |
| AVAILABLE / UNAVAILABLE distinction | parseResult | TestUnavailableIsSuccessfulAndRequiresEmptyContext | CI pending; live acceptance NOT_RUN |
| CLI environment injection and machine output | internal/cli/run.go | internal/cli/run_test.go | CI pending; Host integration NOT_RUN |
| AMF and SMF metrics read clients | Not implemented | Source schema review pending | NOT_RUN |
| UPF session-info and N4 pair join | Excluded | OpenAMSM source findings | NOT_APPLICABLE |

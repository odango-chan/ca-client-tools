# Traceability

Status vocabulary: PASS, FAIL, BLOCKED, NOT_RUN.

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM deployment target and ownership boundaries | README.md; AGENTS.md; docs/architecture.md | Repository review | PASS |
| Eight SMF context paths and public CA type mapping | internal/openamsm/smf/mas/client.go; internal/cli/run.go | `TestEveryContextTypeUsesItsExactOpenAMSMRoute`; `TestSMFContextQueryRoutesAllEightTypesFromTheCLI` | Local PASS; live BLOCKED |
| SUPI + PSI and IMSI + PSI direct queries | Query validation and request builder | `TestKnownSUPIAndPSIQueryDirectlyWithoutListLookup`; `TestKnownIMSIAndPSIQueryDirectly`; compiled-CLI fixture | Local PASS; live BLOCKED |
| IPv4, IPv6 and joint identity + IP lookup | Query validation and canonicalization | `TestQuerySupportsIPv4IPv6AndJointLookups`; `TestJointIPLookupResponseBinding` | Local PASS; live BLOCKED |
| Invalid identity, PSI and IP rejection before network | Client validation | `TestInvalidQueryKeysFailBeforeNetwork`; CLI invalid-input tests | PASS |
| Response SUPI / PSI / IP binding; cross-UE and cross-session rejection | `bindEchoKey` in internal/openamsm/smf/mas/client.go | `TestResponseIdentityMismatchIsAProtocolError`; `TestUNAVAILABLEWithMismatchedIdentityIsAlsoRejected`; `TestJointIPLookupResponseBinding`; CLI cross-UE tests | PASS (contract); live BLOCKED |
| `AVAILABLE` / `UNAVAILABLE` status semantics | `parseResult` and CLI mapping | `TestUnavailableIsSuccessfulAndRequiresEmptyContext`; CLI UNAVAILABLE test | PASS (contract); live BLOCKED |
| HTTP errors, timeout, redirect, size limit, content type | SMF client | `TestHTTPErrorIsNotUNAVAILABLE`; `TestTimeoutIsReportedAsTransportFailure`; `TestRedirectIsNotFollowed`; `TestResponseSizeLimitIsEnforced`; live 403/404 probes | PASS |
| Cleartext HTTP/2 prior-knowledge transport | `http2.Transport{AllowHTTP: true}` in SMF client | `r.ProtoMajor == 2` assertions in h2c fixtures; real listener probe | PASS |
| CLI v1 command tree, JSON stdin/stdout/stderr, exit codes, cancellation | internal/cli/run.go; cmd/ca-client-tools/main.go | internal/cli/run_test.go | PASS |
| AMF `/ue-info` and `/gnb-info` | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real AMF | PASS (live) |
| SMF `/pdu-info` | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real SMF | PASS (live) |
| Metrics pagination and pager validation | `parsePage` and CLI `--page` / `--page-size` | Metrics fixture tests; real page 0/1 queries | PASS |
| Sensitive-value protection in errors | CLI error envelopes and remote cause filtering | SUPI/token/raw-detail assertions in client and CLI tests | PASS |
| Real eight-context end-to-end acceptance | N/A | Real OpenAMSM deployment | BLOCKED (upstream token header case; see evidence) |
| Generic SBI transport / Operation Catalog / Payload Engine | Separate workstream | Not in PR #1 | NOT_RUN |
| UPF `/session-info` | Explicitly out of scope | Not implemented | NOT_RUN |

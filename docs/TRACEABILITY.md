# Traceability

Status vocabulary: PASS, FAIL, BLOCKED, NOT_RUN.

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM deployment target and ownership boundaries | README.md; AGENTS.md; docs/architecture.md | Review | PASS |
| Canonical NF map; 3GPP → Tool → Skill derivation; operation evidence gate | docs/nf-capability-map.md; docs/sbi-client-architecture.md | Pinned OpenAMSM source findings and release-pinned 3GPP OpenAPI references | PASS (static) |
| SMF context-query paths and public CA context mapping | internal/openamsm/smf/mas/client.go; internal/cli/run.go | `TestEveryContextTypeUsesItsExactOpenAMSMRoute`; `TestSMFContextQueryRoutesAllEightTypesFromTheCLI`; real eight-type run | PASS (local + live) |
| SMF identity/IP request keys and token framing | Query validation and request builder | `TestKnownSUPIAndPSIQueryDirectlyWithoutListLookup`; `TestKnownIMSIAndPSIQueryDirectly`; `TestQuerySupportsIPv4IPv6AndJointLookups`; `TestInvalidQueryKeysFailBeforeNetwork` | PASS (contract) |
| SMF response identity binding (SUPI/PSI/IP, cross-UE rejection) | `bindEchoKey` in internal/openamsm/smf/mas/client.go | Contract mismatch tests; real echoed SUPI/PSI/IP checks on all eight types | PASS (contract + live) |
| AVAILABLE / UNAVAILABLE and ProblemDetails semantics | Client parser and CLI error mapping | `TestUnavailableIsSuccessfulAndRequiresEmptyContext`; `TestRemoteProblemDetailsCauseIsPreserved`; `TestHTTPErrorIsNotUNAVAILABLE`; real AVAILABLE + 404 probes | PASS (contract + live) |
| Cleartext HTTP/2 prior-knowledge transport and redirect/timeout/size limits | SMF client `http2.Transport` | h2c fixture assertions (`ProtoMajor == 2`); timeout, redirect, size-limit tests; real listener reached | PASS |
| CLI v1 process contract, JSON envelopes, exit codes and cancellation | internal/cli/run.go; cmd/ca-client-tools/main.go | internal/cli/run_test.go | PASS (local) |
| AMF `/ue-info` and `/gnb-info` metrics | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real AMF | PASS (live) |
| SMF `/pdu-info` metrics | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real SMF | PASS (live) |
| Metrics pagination and pager validation | `parsePage`; CLI page flags | Metrics fixtures; real page 0/1 queries | PASS |
| Sensitive-value protection | CLI error envelopes; safe remote cause filter | SUPI/token/raw-detail assertions in client and CLI tests | PASS |
| Eight-context real OpenAMSM CLI acceptance | Compiled CLI v1 | Real OpenAMSM deployment with the authorized local token-lookup overlay | PASS at CLI boundary (all eight AVAILABLE, identity match); not Host/Runtime dispatch evidence |
| Generic h2c SBI HTTP/2 transport | internal/sbi/client.go; internal/sbi/client_test.go (PR #2, merged as `189deff`) | h2c negotiation, HTTP/1 rejection, connection reuse, cancellation and bounded I/O fixtures; Windows + WSL acceptance | PASS (local Windows + WSL); GitHub Actions CI_UNAVAILABLE |
| Static service configuration and private OpenAMSM header-token profile | internal/sbi/config.go; internal/sbi/client.go (PR #2, merged as `189deff`) | Trusted-origin, token-header isolation and h2c transport tests; Windows + WSL acceptance | PASS (local Windows + WSL); GitHub Actions CI_UNAVAILABLE |
| Trusted SBI Operation Binding Catalog and long-lived Go Tool Provider process protocol | docs/sbi-capability-provider-contract.md §7; docs/sbi-client-architecture.md §7 | Exact callable support projection; one resident `provider serve` process; NDJSON `catalog`/`invoke`/`cancel`; request correlation, bounded concurrency, cancellation and drain | DESIGN DRAFT in PR #5; NOT_IMPLEMENTED / NOT_RUN |
| Combined SMF MAS client + Host Dispatch vertical slice | ca-client-tools PR #5; ca-agent-demo PR #211 | Eight private OpenAMSM Observation callables through installed Provider and actual Host/Runtime Capability/Scope admission | DESIGN TARGET; HOST_DISPATCH_E2E NOT_RUN |
| Host-owned HostArtifactRef and streaming payload interfaces | internal/sbi/payload/files.go (PR #3, merged as `4c424ee`) | Root-confined resolver, generated-name sink, quotas and digest fixtures; Windows + WSL acceptance | Payload primitives PASS (local Windows + WSL); Host Artifact Store/staging integration NOT_IMPLEMENTED |
| JSON, binary and multipart/related request/response | internal/sbi/payload/payload.go; payload_test.go (PR #3, merged as `4c424ee`) | JSON/binary and multipart fixtures based on TS 29.500 / 29.502 / 29.518; Windows + WSL acceptance | PASS (local Windows + WSL); live catalogued binary operation NOT_RUN |
| Operator-only raw request mode | docs/sbi-client-architecture.md | Disabled-by-default and non-Agent-exposure test plan | NOT_RUN |
| Host-side resident Provider process manager | ca-agent-demo `docs/design/generic-agent-host-architecture.md` (PR #211) | Starts one Provider process per configured instance; existing Tool Dispatch seam; readiness, cancellation, drain and shutdown | DESIGN TARGET; NOT_IMPLEMENTED / NOT_RUN |
| Async SBI response boundary | docs/sbi-client-architecture.md | 202 response preservation; Host/Runtime owns continuation | DESIGN ONLY; NOT_RUN |
| Direct PFCP/N4 transport outside SBI; opaque PFCP parts may use multipart | docs/architecture.md; docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md | Protocol and ownership review | DESIGN ONLY; NOT_RUN |

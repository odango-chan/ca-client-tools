# Traceability

Existing evidence below is revision-scoped. See [WORK](WORK.md) for current status and closure gates.

Status vocabulary: PASS, FAIL, BLOCKED, NOT_RUN.

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM deployment target and ownership boundaries | README.md; AGENTS.md; docs/architecture.md | Review | PASS |
| Canonical NF map; 3GPP → Tool → Skill derivation; operation evidence gate | docs/nf-capability-map.md; docs/sbi-client-architecture.md | Pinned OpenAMSM source findings and release-pinned 3GPP OpenAPI references | PASS (static) |
| SMF context-query paths and public CA context mapping | internal/openamsm/smf/mas/client.go; internal/cli/run.go | `TestEveryContextTypeUsesItsExactOpenAMSMRoute`; `TestSMFContextQueryRoutesAllEightTypesFromTheCLI`; real eight-type run | PASS (local + live) |
| SMF identity/IP request keys and token framing | Query validation and request builder | `TestKnownSUPIAndPSIQueryDirectlyWithoutListLookup`; `TestKnownIMSIAndPSIQueryDirectly`; `TestQuerySupportsIPv4IPv6AndJointLookups`; `TestInvalidQueryKeysFailBeforeNetwork` | PASS (contract) |
| SMF response identity binding (SUPI/PSI/IP, cross-UE rejection) | `bindEchoKey` in internal/openamsm/smf/mas/client.go | Contract mismatch tests; real echoed SUPI/PSI/IP checks on all eight types | PASS (contract + live) |
| AVAILABLE / UNAVAILABLE and ProblemDetails semantics | Client parser and CLI error mapping | `TestUnavailableIsSuccessfulAndRequiresEmptyContext`; `TestRemoteProblemDetailsCauseIsPreserved`; `TestHTTPErrorIsNotUNAVAILABLE`; real AVAILABLE + 404 probes | PASS (contract + live) |
| Cleartext HTTP/2 prior-knowledge transport and redirect/timeout/size limits | Shared `internal/sbi` transport used by the SMF client | h2c fixture assertions (`ProtoMajor == 2`); timeout, redirect, size-limit tests; HTTP/1 downgrade rejection; real listener reached | PASS |
| CLI v1 process contract, JSON envelopes, exit codes and cancellation | internal/cli/run.go; cmd/ca-client-tools/main.go | internal/cli/run_test.go | PASS (local) |
| AMF `/ue-info` and `/gnb-info` metrics | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real AMF | PASS (live) |
| SMF `/pdu-info` metrics | internal/openamsm/metrics/client.go | Fixture tests; compiled CLI against real SMF | PASS (live) |
| Metrics pagination and pager validation | `parsePage`; CLI page flags | Metrics fixtures; real page 0/1 queries | PASS |
| Sensitive-value protection | CLI error envelopes; safe remote cause filter | SUPI/token/raw-detail assertions in client and CLI tests | PASS |
| Eight-context real end-to-end acceptance | N/A | Real OpenAMSM deployment with the authorized local token-lookup overlay | PASS (all eight AVAILABLE, identity match) |
| Generic h2c SBI HTTP/2 transport | internal/sbi/client.go; internal/sbi/client_test.go (PR #2, merged `189deff`) | h2c negotiation, HTTP/1 rejection, connection reuse, cancellation and bounded I/O fixtures; Windows-native + WSL/Linux go test/vet/build on validation head `0b80d33`; strict Windows E2E 23/23 | PASS |
| Static service configuration and private OpenAMSM header-token profile | internal/sbi/config.go; internal/sbi/client.go (PR #2, merged `189deff`) | Trusted-origin, token-header isolation and h2c transport tests; Windows-native + WSL/Linux go test/vet/build; strict Windows E2E 23/23 | PASS |
| SMF context-query adapter migration to the shared h2c transport | internal/openamsm/smf/mas/transport_sbi.go; internal/sbi (PR #2) | Existing MAS regression suite unchanged; shared-transport parity, HTTP/1 rejection and h2c connection-reuse tests; live eight-context queries via both CLI and Provider | PASS |
| Eleven read-only tools through the unified Provider (AMF UE/gNB, SMF PDU, eight SMF context) | internal/providerapp/runtime.go; internal/openamsm/* | Provider catalog partition + live invoke assertions for every tool; live list membership; ProblemDetails/error matrix | PASS (real E2E) |
| JSON, binary and multipart/related request/response | internal/sbi/payload/payload.go; payload_test.go; payload_kindnone_test.go (PR #3) | JSON/binary and multipart fixtures based on TS 29.500 / 29.502 / 29.518; duplicate-key, KindNone body and limit-composition tests; Windows-native + WSL/Linux run | PASS (live operation NOT_RUN) |
| Optional WSL loopback acceptance forwarder binding boundary | scripts/acceptance/wsl-tcp-forward.{py,sh} (PR #3) | Fixed `127.0.0.1` listener; `--bind` deleted and `CA_WSL_FORWARD_BIND` not read; wildcard/non-loopback IPv4+IPv6/`::`/`::1`/hostname/empty rejected before listening; exactly three declared services; 13/13 unit tests PASS on Windows and WSL | PASS (acceptance tooling) |
| Optional WSL forwarder lifecycle (start/status/restart/stop) | scripts/acceptance/test-wsl-forwarder-lifecycle.sh | Executed for real in WSL with isolated state dir and non-default high ports; three loopback listeners, new PID on restart, stop then status exit 3; live forwarder untouched | PASS (acceptance tooling) |
| Strict negative matrix and positive identity/list assertions | scripts/acceptance/windows-e2e.ps1 | Exact `httpStatus`/`cause`/exit for wrong PSI/SUPI, unallocated IPv4/IPv6 and wrong token; local `CONFIGURATION_MISSING`/`INVALID_ARGUMENT` with closed-origin no-request proof; `data.type`/`status`/`key` echo; AMF UE/gNB and SMF PDU list membership plus pager; single-JSON-document and leak checks; 23/23 PASS live on validation head `0b80d33` | PASS (live) |
| Windows-native CLI against real WSL-hosted OpenAMSM | internal/cli; internal/openamsm/smf/mas; internal/openamsm/metrics | `scripts/acceptance/windows-e2e.ps1` — 23 live cases with strict JSON assertions, h2c preface capture and HTTP/1.1 rejection | PASS |
| Real `UNAVAILABLE` context, live UE-A/UE-B injection, live Payload Engine operation, real IPv6 session | N/A | Lab always returns `AVAILABLE`; fault injection not authorized; no catalogued live operation; no IPv6 address allocated | NOT_RUN |
| Operator-only raw request mode | docs/sbi-client-architecture.md | Disabled-by-default and non-Agent-exposure test plan | NOT_RUN |
| Async SBI response boundary | docs/sbi-client-architecture.md | 202 response preservation; Host/Runtime owns continuation | DESIGN ONLY; NOT_RUN |
| Direct PFCP/N4 transport outside SBI; opaque PFCP parts may use multipart | docs/architecture.md; docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md | Protocol and ownership review | DESIGN ONLY; NOT_RUN |

## Consolidated target

| Target | Authority and implementation surface | Evidence required | Status |
|---|---|---|---|
| One Provider protocol (sole `ca-client-tools.provider/v2`) | [Protocol](provider-protocol-v2.md), `internal/provider` | P0 process test, artifact identity, rejection of the removed v1 ID and regression coverage for all eleven typed reads | PASS (unit + live + process); v1 ID rejected with `PROTOCOL_VERSION_UNSUPPORTED` |
| One embedded trusted Catalog | [Catalog](provider-backend-binding-catalog-v2.md), `internal/catalog`, `contracts/provider-bindings/v2/provider-bindings.yaml` | Embed/digest exact bytes, reject legacy v1 artifact, shipped inventory of eleven read-only bindings, acceptance-only fixture bindings absent from production builds | PASS (digest `sha256:79e0ea5c1d733357fcb98f11e16cfaee5e2ff00af2e4362060c72211edd2ea85` in the current tree) |
| Nested JSON and Generic SBI dispatch | [Executor](generic-sbi-executor.md), `internal/sbi/executor`, `internal/schemacheck`, `internal/sbi/serviceconfig`, `internal/providerapp` | Recursive pre-I/O validation of the closed argument schema; process-level invoke through trusted service configuration; nested objects/arrays, precision, defaults, duplicate keys, invalid/failed arguments, trusted routing, status/ProblemDetails, deadline/cancellation | PASS (process-level acceptance with test-only trusted binding and deterministic loopback h2c upstream) |
| Shared transport and Payload Engine | [SBI architecture](sbi-client-architecture.md), `internal/sbi`, `internal/sbi/payload` | One trusted registry and one shared SBI transport instance for typed MAS and generic dispatch; metrics HTTP/1.1 path preserved; P0 JSON only | PASS (unit + live typed regression; generic process acceptance) |
| Production Mutation remains gated | [WORK](WORK.md) | Pre-I/O rejection in production configuration | PASS (unit + process acceptance with zero upstream requests; reason `MUTATION_GATED`) |
| Host-owned references and streaming | [Host integration](host-integration.md), [payload requirements](provider-payload-extension-v1.md) | P1 blocked on published Host contract; cross-repository invocation and cleanup tests required | BLOCKED (no accepted/pinned Host Artifact Store contract at `ca-agent` `main` `72842a5`; P1 disabled) |

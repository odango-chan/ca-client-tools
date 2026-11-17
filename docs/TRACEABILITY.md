# Traceability

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM deployment target and ownership boundaries | README.md; AGENTS.md; docs/architecture.md | Review | NOT_RUN |
| Canonical NF map; 3GPP → Tool → Skill derivation; operation evidence gate | docs/nf-capability-map.md; docs/sbi-client-architecture.md | Pinned OpenAMSM source findings and release-pinned 3GPP OpenAPI references | Static source inspection only; source/live acceptance tracked per operation |
| Existing SMF context-query wire paths and public CA context mapping | internal/openamsm/smf/mas/client.go; internal/cli/run.go | internal/openamsm/smf/mas/client_test.go | Latest Go 1.26.2 / CI NOT_VERIFIED; live acceptance NOT_RUN |
| Existing SMF identity/IP keys, query token, AVAILABLE/UNAVAILABLE and ProblemDetails | Query request validation and response parser | SMF adapter fixture tests | Latest Go 1.26.2 / CI NOT_VERIFIED; live acceptance NOT_RUN |
| CLI v1 process contract and cancellation | internal/cli/run.go; cmd/ca-client-tools/main.go | internal/cli/run_test.go | Implemented on review branch; Go 1.26.2 acceptance NOT_VERIFIED |
| AMF and SMF metrics read clients | internal/openamsm/metrics/client.go | AMF/SMF metrics fixture tests | Latest Go 1.26.2 / CI NOT_RUN; live acceptance NOT_RUN |
| Generic SBI HTTP/2 transport design | docs/sbi-client-architecture.md | Architecture review | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Static service configuration and private OpenAMSM header-token profile | internal/sbi/config.go; internal/sbi/client.go | Trusted-origin, token-header isolation and h2c transport tests | IMPLEMENTED ON FEATURE BRANCH; GO TEST/CI PENDING |
| Trusted Operation Catalog and preinstalled Go Tool Provider subprocess with Skill-scoped dynamic registration | docs/sbi-client-architecture.md | Catalog validation, process schemas, staging-root security and Host/Runtime boundary test plan | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Host-owned BinaryRef and streaming payload interfaces | docs/sbi-client-architecture.md | Resolver/sink contract and lifecycle test plan | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| JSON, binary and multipart/related request/response | docs/sbi-client-architecture.md | Multipart conformance test plan based on TS 29.500 / TS 29.502 / TS 29.518 | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Operator-only raw request mode | docs/sbi-client-architecture.md | Disabled-by-default and non-Agent-exposure test plan | OPTIONAL; NOT_STARTED |
| Async SBI response boundary | docs/sbi-client-architecture.md | 202 response preservation; Host/Runtime owns continuation | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Direct PFCP/N4 transport remains outside SBI executor; opaque PFCP message parts may be carried by Nsmf SBI multipart | docs/architecture.md; docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md | Protocol and ownership review | DESIGN ACCEPTED |

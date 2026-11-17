# Traceability

| Contract / behavior | Implementation | Verification | Acceptance |
|---|---|---|---|
| OpenAMSM deployment target and ownership boundaries | README.md; AGENTS.md; docs/architecture.md | Review | NOT_RUN |
| Canonical NF map; 3GPP → Tool → Skill derivation; operation evidence gate | docs/nf-capability-map.md; docs/sbi-client-architecture.md | Pinned OpenAMSM source findings and release-pinned 3GPP OpenAPI references | Static source inspection only; source/live acceptance tracked per operation |
| Existing SMF context-query wire paths and public CA context mapping | internal/openamsm/smf/mas/client.go; internal/cli/run.go | internal/openamsm/smf/mas/client_test.go | Latest Go 1.26.2 / CI NOT_VERIFIED; live acceptance NOT_RUN |
| Existing SMF identity/IP keys, query token, AVAILABLE/UNAVAILABLE and ProblemDetails | Query request validation and response parser | SMF adapter fixture tests | Latest Go 1.26.2 / CI NOT_VERIFIED; live acceptance NOT_RUN |
| CLI v1 process contract and cancellation | internal/cli/run.go; cmd/ca-client-tools/main.go | internal/cli/run_test.go | Implemented on review branch; Go 1.26.2 acceptance NOT_VERIFIED |
| AMF and SMF metrics read clients | internal/openamsm/metrics/client.go | AMF/SMF metrics fixture tests | Latest Go 1.26.2 / CI NOT_RUN; live acceptance NOT_RUN |
| UPF PFCP session diagnostic projection | env-openamsm CA-owned patch overlay; internal/openamsm/metrics/client.go | UPF fixture tests authored | Fixture execution, patch application/build and live-N4 acceptance NOT_RUN |
| Generic SBI HTTP/2 transport | internal/sbi/client.go | HTTP/2 negotiation/downgrade, connection reuse, cancellation, bounded streaming and response preservation tests | IMPLEMENTED ON FEATURE BRANCH; GO TEST/CI PENDING |
| Static service, TLS/mTLS, OAuth client-credentials and private header-token profiles | internal/sbi/config.go; internal/sbi/auth.go; internal/sbi/client.go | Trusted-origin validation, CA/mTLS, OAuth token cache, private header-token and no-secret-override tests | IMPLEMENTED ON FEATURE BRANCH; GO TEST/CI PENDING |
| Trusted Operation Catalog and preinstalled Go Tool Provider subprocess with Skill-scoped dynamic registration | docs/sbi-client-architecture.md | Catalog validation, process schemas, staging-root security and Host/Runtime boundary test plan | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Host-owned BinaryRef and streaming payload interfaces | internal/sbi/payload/payload.go; internal/sbi/payload/files.go | Root-confined staged-file resolver, generated-name attachment sink, quota/digest tests | IMPLEMENTED ON FEATURE BRANCH; GO TEST/CI PENDING |
| JSON, binary and multipart/related request/response | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go | JSON/binary streaming, root-first multipart, Content-ID binding, mixed media types, malformed/missing/duplicate parts, limits and cancellation fixtures | IMPLEMENTED ON FEATURE BRANCH; GO TEST/CI PENDING |
| Operator-only raw request mode | docs/sbi-client-architecture.md | Disabled-by-default and non-Agent-exposure test plan | OPTIONAL; NOT_STARTED |
| Async SBI response boundary | docs/sbi-client-architecture.md | 202 response preservation; Host/Runtime owns continuation | DESIGN ACCEPTED; IMPLEMENTATION NOT_STARTED |
| Direct PFCP/N4 transport remains outside SBI executor; opaque PFCP message parts may be carried by Nsmf SBI multipart | docs/architecture.md; docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md | Protocol and ownership review | DESIGN ACCEPTED |

# Work

## Overall

Design: ACCEPTED for the existing OpenAMSM client/CLI v1 baseline and the
generic SBI Client v1 architecture (h2c HTTP/2 transport, static service
configuration and the private OpenAMSM header-token profile, trusted Operation Catalog, CA-Agent Tool Provider,
Host-owned BinaryRef and JSON/binary/multipart support).

Implementation: PARTIAL. Existing SMF context-query, AMF/SMF metrics clients
and CLI v1 are staged on PR #1. The h2c SBI transport and static OpenAMSM
token-header profile are implemented on PR #2; the JSON/binary/multipart
Payload Engine and Host-staged file boundary are implemented on PR #3.
Operation Catalog and Tool Provider remain NOT_STARTED.

Acceptance: NOT_RUN for the latest Go 1.26.2 branch state, live OpenAMSM
acceptance, and all new SBI foundation components. Other NF operations are
not implicitly enabled by the generic architecture; each requires its own
source/deployment evidence and operation entry.

The current branch is
feat/openamsm-client-tools-foundation-20261116, with Draft PR #1. Do not merge
until the existing CLI/client validation gates are satisfied. The SBI architecture document is the normative baseline; its implementation
slices are tracked separately in stacked PRs #2 and #3.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM deployment target and repository ownership | ACCEPTED | DOCUMENTED | Review pending | README.md; docs/architecture.md |
| Existing SMF context query client | ACCEPTED (OpenAMSM v0.10 wire contract) | IMPLEMENTED on review branch | Latest Go 1.26.2 / CI / live acceptance NOT_VERIFIED | internal/openamsm/smf/mas |
| AMF /ue-info and /gnb-info metrics clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED on review branch | Go 1.26.2 / live acceptance NOT_RUN | internal/openamsm/metrics |
| SMF /pdu-info metrics client | SOURCE CONTRACT VERIFIED | IMPLEMENTED on review branch | Go 1.26.2 / live acceptance NOT_RUN | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED on review branch | Go 1.26.2 test/vet/build and current CI NOT_VERIFIED | docs/cli-contract.md; internal/cli |
| Generic Go h2c SBI HTTP/2 transport | ACCEPTED | IMPLEMENTED on stacked PR #2 | Go test/vet/build pending | internal/sbi/client.go; internal/sbi/client_test.go |
| Static service configuration and OpenAMSM query-token header profile | ACCEPTED; h2c only; custom TLS/OAuth excluded | IMPLEMENTED on stacked PR #2 | Go test/vet/build pending | internal/sbi/config.go; internal/sbi/client.go |
| Trusted Operation Catalog and OpenAPI binding | ACCEPTED | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| CA-Agent preinstalled Go Tool Provider subprocess contract and Skill-scoped dynamic registration | ACCEPTED | Provider commands/Host integration NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| BinaryRef resolver/sink contract | ACCEPTED; Host owns storage/lifecycle | IMPLEMENTED on stacked PR #3 | Go test/vet/build pending | internal/sbi/payload/files.go |
| JSON, binary and multipart/related Payload Engine | ACCEPTED | IMPLEMENTED on stacked PR #3 | Go test/vet/build pending | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go |
| Operator-only curl-like raw request CLI extension | OPTIONAL FUTURE WORK; not part of CLI v1 | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| PFCP/N4 | Direct N4 transport is separate from SBI | No direct PFCP client in this repository | Out of scope | docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md |

## Existing blockers

- The OpenAMSM source archive is imported and hashed in env-openamsm/main;
  native build, OpenAMSM context-query source tests and live NF acceptance
  remain NOT_RUN. Tracked by
  [env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1).
- Latest Go test/vet/build validation is outstanding because recent GitHub
  Actions jobs did not expose usable runner step logs. Do not attribute this
  to source or infrastructure without evidence.

## SBI architecture implementation handoff

[docs/sbi-client-architecture.md](sbi-client-architecture.md) is the
normative design. [docs/DEEPSEEK-HANDOFF.md](DEEPSEEK-HANDOFF.md) records the
implementation sequence and acceptance criteria.

The implementation must preserve these boundaries:

- ca-client-tools provides the preinstallable Tool Provider and concrete
  operation executors; it does not own Host policy or Runtime authorization.
- the Host dynamically activates only trusted operations required by the
  active Skill; a Skill cannot submit arbitrary Operation Manifests or URLs.
- static service configuration and the existing OpenAMSM header-token profile
  are sufficient for this scope; custom TLS and OAuth clients are excluded.
- BinaryRef storage, access control, expiry and cleanup are Host-owned.
- the Client streams binary payloads; it does not base64-encode them for the
  model or create a second artifact store.
- multipart/related follows 3GPP root-first JSON, Content-ID and
  RefToBinaryData semantics.
- individual SBI mutations are allowed when explicitly catalogued and
  authorized; multi-step orchestration, pending-operation lifecycle,
  callbacks and Runtime state remain outside this repository.
- PFCP/N4 remains separate from SBI.

## Next actions

1. Complete Go 1.26.2 gofmt, go test ./..., go vet ./... and native build for
   the existing CLI/client review branch.
2. Run the existing OpenAMSM AMF/SMF query acceptance against the unmodified server.
3. Implement the shared Go SBI HTTP/2 transport and static trusted service,
   the OpenAMSM header-token profile.
4. Implement streaming PayloadResolver/AttachmentSink interfaces, then JSON,
   binary and multipart/related request/response handling.
5. Implement the trusted Operation Catalog and CA-Agent Tool Provider
   descriptor/invocation contract.
6. Migrate the existing SMF context-query adapter only after regression tests
   preserve its private token, identity, status and ProblemDetails behavior.
7. Add a first multipart operation only after its exact pinned 3GPP OpenAPI
   definition and OpenAMSM deployment support are verified.
8. Integrate the Provider and Host-owned BinaryRef resolver in ca-agent as a
   separate repository workstream.

## Authority / evidence links

- [OpenAMSM source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- [3GPP TS 29.500](https://www.etsi.org/deliver/etsi_ts/129500_129599/129500/18.08.00_60/ts_129500v180800p.pdf)
- [3GPP TS 29.518](https://www.etsi.org/deliver/etsi_ts/129500_129599/129518/18.07.00_60/ts_129518v180700p.pdf)
- [3GPP TS 29.502](https://www.etsi.org/deliver/etsi_ts/129500_129599/129502/18.08.00_60/ts_129502v180800p.pdf)
- [Official 3GPP 5G APIs repository](https://forge.3gpp.org/rep/all/5G_APIs)
- docs/architecture.md
- docs/sbi-client-architecture.md
- docs/cli-contract.md
- docs/openamsm-interface-inventory.md

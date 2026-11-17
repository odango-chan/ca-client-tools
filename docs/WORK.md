# Work

## Overall

Design: ACCEPTED for the existing OpenAMSM client/CLI v1 baseline and the
generic SBI Client v1 architecture (h2c HTTP/2 transport, static service
configuration and the private OpenAMSM header-token profile, trusted Operation Catalog, CA-Agent Tool Provider,
Host-owned BinaryRef and JSON/binary/multipart support).

Implementation: COMPLETE for the PR #1 read-only scope. SMF context-query,
AMF/SMF metrics clients and CLI v1 are implemented; the typed SMF adapter now
uses cleartext HTTP/2 prior knowledge (h2c) and strict response-key binding.
The shared h2c SBI transport and static OpenAMSM token-header profile are on
PR #2; the JSON/binary/multipart Payload Engine and Host-staged file boundary
are on PR #3. Operation Catalog and Tool Provider remain NOT_STARTED.

Acceptance (2026-11-17): Go 1.26.8 `gofmt`, `go test ./...`, `go vet ./...`
and `go build ./...` PASS. Compiled-CLI contract acceptance PASS (29 cases).
A real OpenAMSM deployment built from the pinned snapshot, with 11 NFs and a
real UERANSIM UE, was queried with the compiled CLI: AMF UE List, AMF gNB List
and SMF PDU List PASS; the eight SMF context queries are BLOCKED by an
OpenAMSM token-header defect. GitHub Actions: CI_UNAVAILABLE. Other NF
operations are not implicitly enabled by the generic architecture.

**Read-only delivery status:** the source implementation and fixture
coverage are complete for AMF /ue-info and /gnb-info, SMF /pdu-info, and all
eight OpenAMSM SMF context-query routes. The unsupported UPF `/session-info`
extension is removed; no OpenAMSM server patch is needed. The typed SMF
adapter now speaks h2c directly, so it works against the real listener without
waiting for the shared transport migration. Live list acceptance PASS; live
context acceptance is BLOCKED by the server-side token-header case defect. The
primary live scenario remains all eight context types using a known SUPI +
PSI. Evidence: [real acceptance](evidence/real-openamsm-acceptance-20261117.md).

The current branch is
feat/openamsm-client-tools-foundation-20261116, with Draft PR #1. Do not merge
until the existing CLI/client validation gates are satisfied. The SBI architecture document is the normative baseline; its implementation
slices are tracked separately in stacked PRs #2 and #3.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM deployment target and repository ownership | ACCEPTED | DOCUMENTED | Review pending | README.md; docs/architecture.md |
| Existing SMF context query client | ACCEPTED (OpenAMSM v0.10 wire contract) | IMPLEMENTED with typed h2c transport and response binding | Local contract PASS; live BLOCKED by token-header case | internal/openamsm/smf/mas |
| AMF /ue-info and /gnb-info metrics clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| SMF /pdu-info metrics client | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED | Go 1.26.8 + local PASS; CI_UNAVAILABLE | docs/cli-contract.md; internal/cli |
| Generic Go h2c SBI HTTP/2 transport | ACCEPTED | IMPLEMENTED on stacked PR #2 | Go test/vet/build pending | internal/sbi/client.go; internal/sbi/client_test.go |
| SMF context-query migration to shared h2c transport | GATED on exact regression tests | NOT_STARTED; retain existing adapter until transport is validated | NOT_RUN | docs/sbi-client-architecture.md §10; internal/openamsm/smf/mas |
| Static service configuration and OpenAMSM query-token header profile | ACCEPTED; h2c only; custom TLS/OAuth excluded | IMPLEMENTED on stacked PR #2 | Go test/vet/build pending | internal/sbi/config.go; internal/sbi/client.go |
| Trusted Operation Catalog and OpenAPI binding | ACCEPTED | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| CA-Agent preinstalled Go Tool Provider subprocess contract and Skill-scoped dynamic registration | ACCEPTED | Provider commands/Host integration NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| BinaryRef resolver/sink contract | ACCEPTED; Host owns storage/lifecycle | IMPLEMENTED on stacked PR #3 | Go test/vet/build pending | internal/sbi/payload/files.go |
| JSON, binary and multipart/related Payload Engine | ACCEPTED | IMPLEMENTED on stacked PR #3 | Go test/vet/build pending | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go |
| Operator-only curl-like raw request CLI extension | OPTIONAL FUTURE WORK; not part of CLI v1 | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| PFCP/N4 | Direct N4 transport is separate from SBI | No direct PFCP client in this repository | Out of scope | docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md |

## Existing blockers

- **SMF context-query token header (BLOCKED).** The OpenAMSM handler looks up
  `X-OpenAMSM-Query-Token` in a case-sensitive header hash, while HTTP/2
  requires lowercase field names. Every compliant client receives HTTP 403
  `FORBIDDEN`. This blocks all eight live context queries and the final
  delivery criterion. The defect is owned by `env-openamsm`; no server patch
  is applied here. See the real acceptance evidence. The alternative-transport
  investigation confirmed there is no existing HTTP/1.1, proxy or gateway path
  to the same eight routes; every SBI path is h2c. See
  [the transport investigation](evidence/transport-alternative-investigation-20261117.md).
- GitHub Actions: **CI_UNAVAILABLE** (no usable runner logs). Local Go 1.26.8
  validation is recorded as PASS.

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

1. Fix the token header lookup in a deployed OpenAMSM revision (lowercase
   lookup or case-insensitive comparison). Do not patch the server inside this
   repository.
2. Rerun all eight SMF context types with the known SUPI + PSI and record real
   `AVAILABLE` / `UNAVAILABLE`, echoed-key binding and CLI exit codes.
3. Prove a live UE-A request / UE-B response rejection.
4. Validate the shared h2c transport on PR #2 and the Payload Engine on PR #3
   with Go 1.26.8 tests, vet, formatting and native build.
5. Migrate the existing SMF context-query adapter to the shared transport only
   after exact regression tests preserve its token, identity, status and
   ProblemDetails behavior.
6. Implement the trusted Operation Catalog and CA-Agent Tool Provider
   descriptor/invocation contract.
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

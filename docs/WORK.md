# Work

## Overall

The CLI/client foundation is merged to `main`:

- **PR #1** `f191e2f` — OpenAMSM AMF/SMF read clients and CLI v1.
- **PR #2** `189deff` — shared h2c HTTP/2 transport and trusted OpenAMSM token-header profile.
- **PR #3** `4c424ee` — streaming JSON/binary/multipart Payload Engine, Host-owned file boundary and Windows/WSL acceptance tooling.

The delivered CLI v1 remains read-only. The eight SMF MAS context queries are
private OpenAMSM query interfaces served by the SMF SBI listener; they are not
standardized 3GPP SBI operations.

**Windows/WSL acceptance (2026-11-17):** Windows-native Go 1.26.6 formatting,
tests, vet, build and executable smoke checks PASS. WSL Go 1.26.8 formatting,
tests, vet, build and real symlink rejection checks PASS. The Windows-to-WSL
lab path completed 23/23 CLI E2E cases, including all eight SMF contexts,
identity binding and expected negative cases. The real OpenAMSM context
queries returned AVAILABLE with matching SUPI/PSI (and applicable IP) keys.
GitHub Actions remains CI_UNAVAILABLE; local acceptance is recorded separately.

The OpenAMSM lowercase query-token lookup is currently a build-time overlay in
the local `env-openamsm` workspace. The imported upstream source remains
unchanged; other deployments need the overlay adopted by their environment
release before reproducing the same h2c behavior.

**Provider architecture status:** the cross-repository design is documented in
[PR #5](https://github.com/odango-chan/ca-client-tools/pull/5), but the SBI
Operation Binding Catalog, long-lived Go Tool Provider process and Host
integration are not implemented. The agreed boundary is one resident
`ca-client-tools provider serve` process per configured Provider instance,
with bounded NDJSON `catalog`/`invoke`/`cancel` messages over stdin/stdout.
The Provider calls typed Go client/codec packages internally; the CLI remains
an operator/test interface and is not the Host dispatch path.

The first integration delivery combines the former client-package goal and
Host-integration goal into one SMF MAS vertical slice: move/reuse the eight
query clients as exact Observation callables, install/manage the resident
Provider in `ca-agent-demo`, connect the existing Host/Runtime dispatch and
Capability/Scope admission path, and prove a real query through an admitted
Skill against OpenAMSM. CLI live acceptance is not Host/Runtime dispatch
acceptance. The first slice is JSON/h2c only and does not wait for NAS/NGAP,
multipart or HostArtifactRef.

The Host-side artifact lifecycle is tracked separately as a design draft in
[ca-agent-demo PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211);
HostArtifactRef storage/staging is not yet implemented.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM deployment target and repository ownership | ACCEPTED | DOCUMENTED | Review pending | README.md; docs/architecture.md |
| Existing SMF context query client | ACCEPTED (OpenAMSM v0.10 wire contract) | IMPLEMENTED with typed h2c transport and response binding | Local contract PASS; live PASS (post-fix, authorized overlay) | internal/openamsm/smf/mas |
| AMF /ue-info and /gnb-info metrics clients | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| SMF /pdu-info metrics client | SOURCE CONTRACT VERIFIED | IMPLEMENTED | Live PASS | internal/openamsm/metrics |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED | Go 1.26.8 + local PASS; CI_UNAVAILABLE | docs/cli-contract.md; internal/cli |
| Generic Go h2c SBI HTTP/2 transport | ACCEPTED | MERGED on PR #2 | Windows + WSL Go tests/vet/build PASS; CI_UNAVAILABLE | internal/sbi/client.go; internal/sbi/client_test.go |
| SMF context-query migration to shared h2c transport | GATED on exact regression tests | NOT_STARTED; current typed adapter remains source of CLI behavior | NOT_RUN | docs/sbi-client-architecture.md §10; internal/openamsm/smf/mas |
| Static service configuration and OpenAMSM query-token header profile | ACCEPTED; h2c only; custom TLS/OAuth excluded | MERGED on PR #2 | Windows + WSL local acceptance PASS; CI_UNAVAILABLE | internal/sbi/config.go; internal/sbi/client.go |
| Trusted SBI Operation Binding Catalog | DESIGN DRAFT in PR #5 | NOT_IMPLEMENTED | NOT_RUN | docs/sbi-capability-provider-contract.md; docs/sbi-client-architecture.md |
| Long-lived Go Tool Provider process protocol | DESIGN DRAFT in PR #5: `provider serve`, bounded NDJSON `catalog`/`invoke`/`cancel` | NOT_IMPLEMENTED | NOT_RUN | docs/sbi-capability-provider-contract.md §7 |
| Host Provider process manager and existing Tool Dispatch integration | DESIGN TARGET; combined with the eight-query SMF MAS vertical slice | NOT_IMPLEMENTED | HOST_DISPATCH_E2E NOT_RUN | ca-agent-demo PR #211; ca-client-tools PR #5 |
| HostArtifactRef resolver/sink contract | DESIGN DRAFT in ca-agent-demo PR #211; Host owns storage/lifecycle | Payload primitives MERGED on PR #3; Host store/staging NOT_IMPLEMENTED | Windows + WSL local acceptance PASS for payload primitives; Host E2E NOT_RUN | internal/sbi/payload/files.go; ca-agent-demo PR #211 |
| JSON, binary and multipart/related Payload Engine | ACCEPTED | MERGED on PR #3 | Windows + WSL local acceptance PASS; live catalogued binary operation NOT_RUN | internal/sbi/payload/payload.go; internal/sbi/payload/payload_test.go |
| Operator-only curl-like raw request CLI extension | OPTIONAL FUTURE WORK; not part of CLI v1 | NOT_STARTED | NOT_RUN | docs/sbi-client-architecture.md |
| PFCP/N4 | Direct N4 transport is separate from SBI | No direct PFCP client in this repository | Out of scope | docs/sbi-client-architecture.md; docs/openamsm-interface-inventory.md |

## Existing blockers

- **SMF context-query token header (resolved in the local lab).** The original
  handler looked up `X-OpenAMSM-Query-Token` in a case-sensitive header hash,
  while HTTP/2 requires lowercase field names. The operator authorized a
  lowercase lookup in a local `env-openamsm` build overlay; all eight real
  context queries now PASS. The overlay must be adopted by `env-openamsm` for
  any other deployment. The alternative-transport investigation confirmed
  there was no existing HTTP/1.1, proxy or gateway workaround. See the
  [real acceptance evidence](evidence/real-openamsm-acceptance-20261117.md)
  and [transport investigation](evidence/transport-alternative-investigation-20261117.md).
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

1. Keep the lowercase query-token build overlay documented and arrange its
   adoption by the `env-openamsm` deployment/release owner before reproducing
   the lab on another environment.
2. Preserve the existing Windows/WSL acceptance evidence. Real UE-A/UE-B
   service-side fault injection remains NOT_RUN; client contract tests cover
   identity mismatch rejection.
3. Synchronize Draft PR #5 with the now-merged PR #2/#3 `main` base and review
   the cross-repository Capability / callable / Provider contract.
4. Freeze the field-level NDJSON v1 envelope, startup handshake, error registry
   and cancellation acknowledgement. The architecture decision is already a
   long-lived `provider serve` process with bounded `catalog`/`invoke`/`cancel`
   frames, not a CLI launch per Tool call.
5. Implement the first combined SMF MAS vertical slice: exact Observation
   callable contracts and private OpenAMSM bindings in ca-client-tools; Host
   installation and resident process lifecycle in ca-agent-demo; existing
   Tool Dispatch and Runtime Capability/Scope admission; real OpenAMSM query
   through an admitted Skill; negative authorization/binding tests.
6. Do not count CLI PASS, Provider-local PASS or mocked Host dispatch as the
   vertical-slice acceptance. Require real Host/Runtime dispatch evidence.
7. Add NAS/NGAP codecs, HostArtifactRef binary staging and N1/N2 migration only
   after the JSON-only SMF vertical slice is accepted. Add other NF callables
   only after their exact source/deployment contracts are verified.

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

# Work

## Runtime-only Development bindings (v1.2.0 candidate)

`ca-client-tools#37` is implemented on the release path. The pinned OpenAMSM
Owner `v1alpha4` publication is vendored byte-for-byte at
`contracts/owner/openamsm-mas/v1alpha4/` and verified against its manifest on
every generation. The generator cross-checks the Owner Callables against the
reviewed Go typed closure, proves the audited legacy exclusions, emits 25
Runtime-only bindings (21 MUTATION + 4 READ) with their Owner Schema exports,
and writes `provider.yaml.runtimeBindingsPath` / `runtimeBindingsDigest`. The
public Interface Package is unchanged (11 READ Tools, 2 Capabilities,
4 Schemas).

Contract: [Runtime-only Provider binding sidecar](runtime-bindings-v1.md).
Local gates: `go test ./...`, `go vet`, the real `skillc v2.1.0` Interface link
gate, and `ca-client-tools-release generate`/`verify` produce a byte-stable
package. The Host consumes the package: 11 public + 25 Runtime-only bindings
resolve in `catalog` with matching Effects. Publication and the live Scheme C
regression are tracked separately; this repository claims no live MAS mutation.

## Current release: v1.1.0 (published)

[v1.1.0](https://github.com/odango-chan/ca-client-tools/releases/tag/v1.1.0) is
published from the annotated tag `v1.1.0` (tag object `b399abb`, commit
`69541eca7eb1513d6d991b5dd8504702fb29ea44`). The release publishes:

- `ca-client-tools_1.1.0_linux_amd64.tar.gz`
- `ca-client-tools_1.1.0_linux_arm64.tar.gz`
- `ca-client-tools_1.1.0_windows_amd64.zip`
- `ca-client-tools_interface_1.1.0.zip`
- `openamsm-mas-compiler-contract-v0.3.0.zip` (`NOT_LINKABLE` candidate)
- `mas-runtime-compatibility.json` and `SHA256SUMS` covering every published
  ZIP/TAR.GZ and the compatibility record

Post-publication verification: all three platform `interface/` trees and the
standalone ZIP are byte-identical (7 files); `ca-client-tools-release verify`
accepts the extracted package; the published Windows binary reports
`ca-client-tools v1.1.0 (commit 69541eca…)` and its `initialize` identity
(`providerId`, `providerVersion=1.1.0`, `backendBindingCatalogDigest`) matches
`provider.yaml`; the published Interface Package links through the real
`ca-skill-compiler v2.1.0` as `VALIDATED / LINKED / BUILT / NOT_ADMITTED`.

## v1.1.0 release package (implemented)

`v1.1.0` publishes the Compiler Interface Package and the Provider runtime
binding generated from the single canonical Tool definition. The scope, exact
asset names and acceptance gates are in [RELEASE](RELEASE.md); the format and
its implemented boundaries are in
[the release package contract](tool-provider-release-package-vnext.md).

Implemented in this release:

1. **One canonical Tool source.**
   `contracts/provider-bindings/v2/provider-bindings.yaml` declares
   `metadata.interface`, `spec.schemaFiles` and `spec.tools` in addition to the
   private bindings. `internal/catalog` validates the Tool identity, kind,
   Effect agreement with the binding, Owner outcomes, Capability membership and
   every Schema `$ref`; a MUTATION Tool must declare `receiptSchema` and
   `mutation.commitProof`. There is no second authored ToolCatalog.
2. **Deterministic generation.** `internal/interfacepkg` and
   `cmd/ca-client-tools-release` emit `interface/package.json`,
   `interface/tool-catalog.yaml`, `interface/capabilities.json`,
   `interface/schemas/*.schema.json` and `provider.yaml`. The snapshot digest
   uses the frozen `CA-SKILL-DEPENDENCY-SNAPSHOT` v1 framing; JSON key order,
   indentation and file order are fixed, so repeated generation is
   byte-identical. `verify` re-generates and byte-compares a staged package.
3. **Published scope.** Eleven READ Tools in two Capabilities and four
   standalone Schemas. No MUTATION Tool is exported: the Owner publishes no
   receipt, commit-proof or reconcile contract for a Provider mutation binding,
   so exporting one would require a fabricated receipt. The typed MAS mutation
   callables stay runtime `Development` bindings (see below).
4. **Provider manifest.** `provider.yaml` carries the Provider ID, the release
   version, the protocol, the stdio argv, the interface path and snapshot
   digest, the extended backend catalog digest and the exact public-to-private
   binding map. A process-level test starts the real binary and compares
   `initialize` identity/version/digest and the `catalog` response.
5. **Release assets.** Three platform archives each contain the binary,
   `provider.yaml` and the identical `interface/` tree; the standalone
   `ca-client-tools_interface_<version>.zip` contains only the Interface
   Package; `SHA256SUMS` covers every asset. The workflow verifies the archive
   and ZIP identity before upload and re-verifies the published checksums.
6. **Compiler link gate.** The release and CI workflows download the pinned
   `ca-skill-compiler v2.1.0` binary (SHA-256 verified) and run
   `build-development` against the generated package and the example Skill in
   `examples/interface-package-skill/`. A broken `$ref` negative case must fail.

### Typed MAS execution (Development, not Interface-published)

The Provider process has a fixed typed OpenAMSM MAS orchestration closure
(31 callables, 29 MUTATION and 2 READ) installed from Go source
(`internal/openamsm/smf/mas/bindings.go`). It is available only when the
private orchestration endpoint is configured, requires the exact Host
occurrence scope, never retries and never converts a result into `COMMITTED`.
The read-only Owner receipt and exact-original reconcile resources
(`openamsm.smf.mas.receipt.query`, `openamsm.smf.mas.reconcile`) are served by
the same private transport.

These callables are **not** exported by the Compiler Interface Package: their
invocation requires a Host-private scope that must not appear in a public
Schema, and the Owner publication required for a valid MUTATION ToolCatalog
entry is not part of this release. They are `Development`-capable, not
Production-qualified. Do not describe a successful Scheme C run as production
qualification.

## Verified historical baseline

The v1.0.0 baseline remains the earlier accepted Provider code head
`5b93842035c72042b51e5a225da88bd21813c6e4`, validated against merged
`ca-agent/main` `447d0cce7e42c7c1184b11fb2563792eead68d51`. Cross-repository
acceptance and exact gates are in
[Provider final acceptance](evidence/provider-final-acceptance-20261118.md).
Earlier P0/P1 records remain revision-scoped history.

The P0 single-protocol/single-Catalog execution path and P1 Host-owned artifact
channel integration are implemented and accepted within the limits below.

## P0 - consolidate and connect read execution (IMPLEMENTED / ACCEPTED)

1. **One Provider protocol.** `provider serve` accepts exactly
   `ca-client-tools.provider/v2`. The v1 identity, version negotiation, version
   lock and compatibility branches were deleted. Any frame with another
   identity is rejected with `PROTOCOL_VERSION_UNSUPPORTED`, before and after
   initialize; a legacy initialize creates no runtime.
2. **One embedded Catalog.** `contracts/provider-bindings/v2/provider-bindings.yaml`
   is the single artifact and the only production load path (`catalog.Load`).
   The SHA-256 of the exact embedded bytes is the base catalog digest; the
   Provider reports it extended by the fixed typed MAS closure. Only the
   explicit `providerfixture` build tag may append acceptance-only test
   bindings; production binaries never contain them.
3. **Unified runtime.** `providerapp` builds one trusted service registry keyed
   by the Catalog `serviceKey` (typed `CA_OPENAMSM_*` settings plus the optional
   trusted `CA_PROVIDER_SERVICE_CONFIG` service file) and one shared
   `internal/sbi` transport instance. Typed MAS, genericSBI dispatch and the
   typed metrics handlers all use that instance, with metrics on an explicit
   source-verified HTTP/1.1 transport profile and unchanged paging/error
   semantics. The eleven typed read bindings are unchanged.
4. **Generic SBI through the Provider.** Each cataloged `genericSBI` binding is
   bridged to the existing executor at initialization, with its closed
   `generic.argumentSchema` compiled once. Invocation arguments are validated
   recursively before any network I/O; the executor owns method, path,
   parameters, trusted headers, body framing and response decoding. Valid
   remote ProblemDetails remains a successful Provider result.
5. **Mutation gate.** A cataloged `MUTATION` binding whose handler is not the
   typed MAS handler is rejected with `BACKEND_OPERATION_UNSUPPORTED`
   (`detail.reason: MUTATION_GATED`) before any remote I/O. The typed MAS
   handler is the only bypass and exists only with the private endpoint and
   Host occurrence scope. An idempotency key alone authorizes neither dispatch
   nor retries.
6. **Generic SBI response projection boundary.** Generic response projection
   rejects any binary body or attachment before the JSON success branch. A
   `genericSBI` binding whose declared response is binary or
   `multipart/related` (with parts) is reported `NOT_IMPLEMENTED` by
   availability and rejected at the execution entrance, before any remote I/O.
7. **Metrics shared transport.** The typed AMF/SMF metrics handlers and the
   standalone CLI metrics commands run over the one shared `internal/sbi`
   transport through an explicit `metrics-http1` trusted profile.

P0 is closed at the validation head recorded in
[the P0 closeout record](evidence/provider-p0-closeout-20261118.md).

## P1 - Host artifact streaming (IMPLEMENTED / ACCEPTED)

P1 is implemented at `4448379a324bccd44a7155c6cd337385d377eaf0` (tested head
`0135418e032ebddc2c861cd4f52591137bd9ba70`) against the Host Artifact Store
contract `ca-host.artifact-store/v1`:

- Contract commit `2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae`, fixture
  `MANIFEST.sha256`
  `59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4`.
- The exact contract fixture bytes are pinned under
  `contracts/host-artifact-store/v1/` and verified by a Go test, together with
  the reference/slot/channel-config vectors.
- `internal/artifactchannel` implements the channel client: bootstrap
  validation, input GET resolution with size/digest verification, output POST
  staging with exact Content-Length and digest, quota enforcement, and the
  contract section 4.4 mapping to `ARTIFACT_REFERENCE_INVALID` /
  `ARTIFACT_TRANSFER_FAILED`. References, tokens, paths and bytes are never
  logged.
- `initialize` accepts and validates the optional `artifactChannel`. Artifact
  references are validated at declared slots only and resolved before any
  remote I/O.
- Full record: [P1 closeout evidence](evidence/provider-p1-closeout-20261118.md).
- **Real-NF multipart: NOT_VERIFIED** - the pinned deployment exposes no
  source-verified deployed multipart operation; none was invented.

## Final acceptance against merged Host main (v1.0.0)

Final acceptance at Provider `5b93842035c72042b51e5a225da88bd21813c6e4`
(**ACCEPTED**) against the merged `ca-agent/main`
`447d0cce7e42c7c1184b11fb2563792eead68d51`:

- Provider full repository gates PASS; cross-repository E2E 43/43 top-level
  PASS; real OpenAMSM 11/11 read operations PASS through the real Host entry on
  the pinned `env-openamsm` `594dec78` lab.
- Production/fixture isolation PASS; contract pin re-verified.
- GitHub CI for `5b93842` was `CI_UNAVAILABLE` (run `44263932341`: zero-step
  failure). That is not a CI pass and not a test failure.
- Real-NF multipart remains `NOT_VERIFIED`.

Full record: [final acceptance evidence](evidence/provider-final-acceptance-20261118.md).

## Scoped 5GSM codec (Issue #18 first slice)

`protocol/nas5gsm` is IMPLEMENTED and OWNER-LOCAL PASS. It provides a
protocol-only, cross-repo importable encoder/decoder for the PDU Session
Establishment Request with fail-closed malformed/unsupported handling. It has
no Provider, Host, Runtime, Artifact Store or network dependency and is not an
Agent-callable tool. Full Host ingress and the original-N1 Owner binding remain
#18 work and are NOT claimed here.

## Production Mutation gate

Enabling a concrete mutation operation requires pinned authoritative
source/deployment evidence, explicit Host and Runtime authorization,
operation-specific idempotency/reconciliation semantics and reviewed acceptance.
No automatic retry or implicit polling exists in the Provider or the executor.
The typed MAS mutation callables are a `Development` capability only; they are
not published through the Compiler Interface Package and are not
Production-qualified.

## Open items

Issue [#8](https://github.com/odango-chan/ca-client-tools/issues/8) remains
open. It covers mixed/non-projectable response media for artifact-bearing
genericSBI bindings. It does not affect the shipped production Catalog: that
Catalog contains eleven typed read bindings and no production genericSBI
binding. Keep the issue as a prerequisite before introducing such a production
binding.

Host-side generic Tool package loading and the removal of the Python MAS
adapters remain [ca-agent-demo#241](https://github.com/odango-chan/ca-agent-demo/issues/241).
Skill Studio Interface Package import remains
[ca-skill-studio-demo#261](https://github.com/odango-chan/ca-skill-studio-demo/issues/261).

Do not widen the current release scope to production Mutation, a new NF
operation, MCP integration or real-NF multipart. These require their own
source, contract, authorization and acceptance gates.

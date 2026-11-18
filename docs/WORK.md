# Work

## Verified starting point

On 2026-11-18, [draft PR #7](https://github.com/odango-chan/ca-client-tools/pull/7)
targets `main` from `feat/unified-provider-read-tools-20261117`. The inspected
starting remote head was the architecture-document commit
`dbb3cfe059b24c440aee8bf85e0a522c2f1101e6`; the implementation below continues
from that head. The P0 closeout (response projection boundary and metrics
transport convergence) was implemented and validated at head
`72fd75ea6a1ab8df5295a7bb0a835e4edd970a10`; that round and its exact per-gate
evidence are recorded in [the P0 closeout record](evidence/provider-p0-closeout-20261118.md).
The earlier P0 slice is recorded in
[the P0 acceptance record](evidence/provider-p0-acceptance-20261118.md).
Earlier results in [Provider acceptance 2026-11-17](evidence/provider-acceptance-20261117.md)
and [Windows/WSL acceptance](evidence/windows-wsl-acceptance-20261117.md) are
revision-scoped history and do not establish this consolidated target.

## P0 — consolidate and connect read execution (IMPLEMENTED / ACCEPTED)

1. **One Provider protocol.** `provider serve` accepts exactly
   `ca-client-tools.provider/v2`. The v1 identity, version negotiation, version
   lock and compatibility branches were deleted. Any frame with another
   identity is rejected with `PROTOCOL_VERSION_UNSUPPORTED`, before and after
   initialize; a legacy initialize creates no runtime.
2. **One embedded Catalog.** `contracts/provider-bindings/v2/provider-bindings.yaml`
   is the single artifact and the only production load path (`catalog.Load`).
   The v1 artifact, v1 structures, v1 loader and version dispatch were deleted.
   The SHA-256 of the exact embedded bytes is reported as
   `backendBindingCatalogDigest`. Only the explicit `providerfixture` build tag
   may append acceptance-only test bindings; production binaries never contain
   them and the reported digest still covers the production artifact.
3. **Unified runtime.** `providerapp` builds one trusted service registry keyed
   by the Catalog `serviceKey` (typed `CA_OPENAMSM_*` settings plus the optional
   trusted `CA_PROVIDER_SERVICE_CONFIG` service file) and one shared
   `internal/sbi` transport instance. Typed MAS, genericSBI dispatch and the
   typed metrics handlers all use that instance, with metrics on an explicit
   source-verified HTTP/1.1 transport profile and unchanged paging/error
   semantics (see item 7). The eleven typed read bindings are unchanged.
4. **Generic SBI through the Provider.** Each cataloged `genericSBI` binding is
   bridged to the existing executor at initialization, with its closed
   `generic.argumentSchema` compiled once. Invocation arguments are validated
   recursively (nested objects, arrays, numbers, closed objects) before any
   network I/O; the executor owns method, path, parameters, trusted headers,
   body framing and response decoding. Valid remote ProblemDetails remains a
   successful Provider result with `httpStatus`/`problem`; malformed responses,
   transport failures and cancellation map to their documented faults.
5. **Mutation gate.** The deployed Catalog remains read-only. A cataloged
   `MUTATION` binding is rejected with `BACKEND_OPERATION_UNSUPPORTED`
   (`detail.reason: MUTATION_GATED`) before any remote I/O; the acceptance
   upstream proves zero mutation requests. An idempotency key alone authorizes
   neither dispatch nor retries.
6. **Generic SBI response projection boundary.** Generic response projection
   rejects any binary body or attachment before the JSON success branch, so a
   multipart result that carries a JSON root and attachments is never returned
   as a partial success with the attachments dropped. A `genericSBI` binding
   whose declared response is binary or `multipart/related` (with parts) is
   reported `NOT_IMPLEMENTED` by availability and rejected with
   `BACKEND_OPERATION_UNSUPPORTED` (`detail.reason: NOT_IMPLEMENTED`) at the
   execution entrance, before any remote I/O. The fault carries no artifact
   content. The executor's independent binary/multipart capability and its
   Payload Engine fixtures remain intact for P1.
7. **Metrics shared transport.** The typed AMF/SMF metrics handlers and the
   standalone CLI metrics commands now run over the one shared `internal/sbi`
   transport through an explicit `metrics-http1` trusted profile, instead of a
   separate `http.Client`. The standard SBI services keep `sbi-h2c` prior
   knowledge with no HTTP/1.1 fallback; the metrics listeners keep HTTP/1.1 and
   their source-verified paging/shape/error semantics. Connection lifecycle,
   cancellation, timeout, redirect, proxy and size bounds are shared and
   configured from the trusted service registry.

P0 is closed at the validation head recorded in the evidence: one protocol and
one Catalog are observable in the binary and acceptance runner, the legacy
identity is rejected, all eleven typed reads pass through the real Provider on
Windows-native and Linux-native against the pinned lab, and a process-level
generic invoke reaches the executor through trusted configuration with
deterministic loopback coverage of nested arguments, precision, defaults,
duplicate keys, invalid arguments, trusted routing, status/ProblemDetails,
timeouts, cancellation and the Mutation gate. Local gates are recorded for the
same head. GitHub CI cannot run for that head because the job never starts on
this account (billing/spending-limit annotation); that is `CI_UNAVAILABLE`, not
a passing check and not a test failure.

Closeout at `72fd75e`: the projection defect is covered by unit tests for
JSON-root-plus-attachments, binary-only, multipart-only, normal JSON and 204;
artifact-bearing acceptance-only bindings are rejected pre-I/O at the runtime
and through the real Provider process with zero upstream requests; and the
metrics transport convergence is covered by profile, isolation, configuration,
cancellation, timeout, size-bound and connection-reuse tests. Windows-native and
Linux-native live acceptance reran all eleven typed reads plus metrics and the
generic fixture suite (16/16 top-level tests and 8/8 invalid-argument subtests
on each platform). GitHub CI for `72fd75e` is `CI_UNAVAILABLE` for the same
zero-step job reason. See
[the P0 closeout record](evidence/provider-p0-closeout-20261118.md).

## P1 — Host artifact streaming (IMPLEMENTED / LOCAL_CROSS_REPO_ACCEPTANCE_PASS)

P1 is implemented at `4448379a324bccd44a7155c6cd337385d377eaf0` (tested head
`0135418e032ebddc2c861cd4f52591137bd9ba70`) against the Host owner (`ca-agent`)
published and pinned Host Artifact Store contract `ca-host.artifact-store/v1`:

- Contract commit `2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae` (documentation-only
  update of accepted `3a0d140401109d19152e253cb0b89b59feef7bfb`), document
  <https://github.com/odango-chan/ca-agent/blob/2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae/docs/design/host-artifact-store-v1.md>,
  fixture `MANIFEST.sha256`
  `59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4`.
- The exact contract fixture bytes are pinned under
  `contracts/host-artifact-store/v1/` and verified by a Go test, together with
  the reference/slot/channel-config vectors.
- `internal/artifactchannel` implements the channel client: bootstrap
  validation, input GET resolution with size/digest verification, output POST
  staging with exact Content-Length and digest, quota enforcement, and the
  contract §4.4 mapping to `ARTIFACT_REFERENCE_INVALID` /
  `ARTIFACT_TRANSFER_FAILED`. References, tokens, paths and bytes are never
  logged.
- `initialize` accepts and validates the optional `artifactChannel` (malformed
  or partial => `PROVIDER_CONFIGURATION_INVALID`) and reports the effective
  artifact limits. Artifact-bearing `genericSBI` bindings are `NOT_CONFIGURED`
  without a channel and available with one; the closed `artifactSlots`
  descriptor extension is published. This intentionally replaces the P0
  `NOT_IMPLEMENTED` projection boundary.
- Generic invoke validates the closed reference value at declared slots only,
  rejects references at undeclared non-slot locations, resolves every input
  reference through the channel before any remote I/O, maps inputs to the
  executor resolver (`body`, `payloads.<partName>`) and streams response parts
  to the invocation-scoped Host sink. Binary results project `response` as
  exactly the reference object; multipart results replace declared
  nested/array JSON-pointer values with reference objects and keep `httpStatus`.
  No output reference is emitted on failure, cancellation or deadline.
- The executor gained an additive optional streaming-sink seam; the memory sink
  remains the default and all P0 behavior/tests are unchanged.
- The `providerfixture` build adds the Appendix A acceptance-only bindings
  `acceptance.artifact.binary.up`, `acceptance.artifact.binary.down` and
  `acceptance.artifact.multipart.roundtrip` under service key
  `acceptance-artifact`, with the declared multipart parts and array-index
  JSON pointers. Production catalog/digest is unchanged.
- Gates at the implementation head: `gofmt -l .` empty, `go build ./...`,
  `go vet ./...`, `go test ./... -count=1`, `go test -tags providerfixture
  ./internal/providerapp/ ./internal/catalog/ ./internal/provider/`,
  `go vet -tags providerfixture ...`, `go vet -tags acceptance
  ./scripts/acceptance/`, and the non-live process acceptance subset against
  the compiled `providerfixture` binary all PASS. Exact commands and outputs
  are in [the P1 evidence](evidence/provider-p1-20261118.md).

Lead-run acceptance at Provider `0135418e032ebddc2c861cd4f52591137bd9ba70`
and Host `18eb562bd96ed1faccf94c6d0791236a53cc2b30`:

- **Cross-repository E2E PASS**: the real `ca-agent` Host adapter and artifact
  channel drove the real `ca-client-tools provider serve` child through binary
  up/down and multipart round trip (two inputs, two outputs, nested array JSON
  pointers), optional-part omission, upstream missing/duplicate parts, malformed
  root and truncated MIME, size/quota bounds, cancellation, deadline, Provider
  exit, pre-I/O invalid/foreign/expired/missing reference and unauthorized-token
  rejection, partial-write cleanup and no output reference on failure — 39/39
  top-level tests PASS.
- **Live P0 regression PASS**: the running pinned lab
  (`env-openamsm` `594dec78`) through the real Provider binary — 21/21 top-level
  tests PASS, including the eleven typed reads, CLI, one protocol/one Catalog,
  generic JSON, Mutation gate, deadline/cancellation and the P1 process-level
  artifact suite.
- **Real-NF multipart: NOT_VERIFIED** — the pinned deployment exposes no
  source-verified deployed multipart operation; none was invented.
- **GitHub CI: CI_UNAVAILABLE** for the tested head (run `44249002299`, job
  `134614799098`: failure in ~2 s with an empty `steps` list; the job never
  starts). Not a CI pass and not a test failure.

Exact commands, coverage and caveats are in
[the P1 evidence](evidence/provider-p1-20261118.md) §7 (and the Host repository's
own acceptance record).

P1 closeout at Provider code head `4c135ab0ac449b5315e493899ee46734242eb115`
and Host closeout code head `c9d2dcd` (`ca-agent` PR #27):

- **Reference-scope defect fixed.** A `genericSBI` binding with no declared
  artifact slots skipped reference-location validation, so a Host-minted
  reference smuggled through an open nested argument object could reach the
  remote NF. Validation now runs for every generic invoke before remote/channel
  I/O; reproducing test
  `TestGenericNonArtifactInvocationRejectsReferenceAtUndeclaredLocation`
  (fails before, passes after; zero upstream requests). Tracked by PR #7;
  latent mixed-response projection edge tracked separately as
  [ca-client-tools#8](https://github.com/odango-chan/ca-client-tools/issues/8).
- **Contract pin re-verified:** all 29 `contracts/host-artifact-store/v1/`
  files byte-identical to `ca-agent@2c0b07f:conformance/artifact-store/v1`;
  manifest digest `59238a43…` verified, no drift.
- **Local gates at the closeout head:** `gofmt -l .` empty; `go build ./...`;
  `go vet ./...`; `go test ./... -count=1` (13/13 packages); focused `-race`
  (5/5); `providerfixture` suite (13/13) and fixture-tag race; non-live
  process acceptance subset 13/13 PASS; production/fixture binaries built and
  byte-scanned.
- **Lead cross-repository acceptance PASS:** Host normal-entrypoint binary
  started the real `providerfixture` `provider serve` child through the Host
  adapter (binary up/down, multipart two-in/two-out, optional-part omission,
  activation boundary, crash cleanup) and all eleven typed reads through the
  same Host entry against the pinned `env-openamsm` lab; Host E2E 43/43
  top-level PASS at the closeout heads.
- **Real-NF multipart NOT_VERIFIED**; GitHub CI for the closeout head is
  `CI_UNAVAILABLE` (job never starts on this account).

Full record: [P1 closeout evidence](evidence/provider-p1-closeout-20261118.md).

## Production Mutation gate

Enabling a concrete mutation operation requires pinned authoritative
source/deployment evidence, explicit Host and Runtime authorization,
operation-specific idempotency/reconciliation semantics and reviewed acceptance.
No automatic retry or implicit polling exists in the Provider or the executor.

# Release process

Status: **v1.1.0 release package implemented.** The latest published stable
release is [v1.0.0](https://github.com/odango-chan/ca-client-tools/releases/tag/v1.0.0);
v1.1.0 is the next stable version and adds the generated Compiler Interface
Package, the runtime `provider.yaml` and the standalone interface ZIP. Re-check
the Git tag and GitHub Release inventory immediately before publication.

This document defines how to build and publish CA Client Tools. The release
scope is limited to the implementation and acceptance scope recorded in
[WORK](WORK.md) and [the release package contract](tool-provider-release-package-vnext.md).

## Release scope

`v1.1.0` contains:

- The resident Provider process using only `ca-client-tools.provider/v2`.
- The single embedded production Catalog: eleven typed OpenAMSM read bindings
  and the fixed typed MAS orchestration closure.
- The canonical public Tool definitions in
  `contracts/provider-bindings/v2/provider-bindings.yaml` and their standalone
  Schema sources.
- The generated CA-SL Interface Package
  (`connectionagent.io/interface-package/v1alpha1`) and the generated runtime
  binding `provider.yaml`.
- The shared SBI transport, typed OpenAMSM adapters and the frozen local CLI v1.
- The generic SBI executor and Payload Engine implementation.
- Host-owned artifact reference/streaming integration conforming to the pinned
  `ca-host.artifact-store/v1` contract.
- The read-only Owner durable receipt / exact-original reconcile Provider
  handlers (`openamsm.smf.mas.receipt.query`, `openamsm.smf.mas.reconcile`).

The Interface Package exports **READ Tools only**. The eleven public Tools are
the eight OpenAMSM SMF MAS context queries and the three AMF/SMF metrics lists,
grouped into two Capabilities. No MUTATION Tool is exported: the Owner publishes
no receipt/commit-proof/reconcile contract for a Provider mutation binding, so a
mutation export would require an invented receipt. The typed MAS mutation
callables stay available as runtime `Development` bindings behind the private
orchestration configuration and are never claimed as production-qualified.

It does **not** claim that every generic SBI operation or every 3GPP NF is
implemented. The production Catalog contains no production genericSBI binding.
Host/Runtime continue to own callable admission, Capability authorization,
execution identity, Outcome, Evidence and Reconcile. A compiled Skill is
`NOT_ADMITTED`; `BUILT` is not a Host admission.

The binary/multipart executor and Host channel are covered by deterministic
fixtures and cross-repository Host acceptance. A real-NF multipart operation
has not been verified and must not be claimed as live-supported.

Issue [#8](https://github.com/odango-chan/ca-client-tools/issues/8) remains
open. It does not affect the current production Catalog because no production
genericSBI binding is shipped. Close it before adding a production
artifact-bearing genericSBI operation whose response media could be mixed or
otherwise non-projectable.

## Release artifacts

The tag-triggered workflow publishes:

| Target | Archive |
|---|---|
| Windows amd64 | `ca-client-tools_1.1.0_windows_amd64.zip` |
| Linux amd64 | `ca-client-tools_1.1.0_linux_amd64.tar.gz` |
| Linux arm64 | `ca-client-tools_1.1.0_linux_arm64.tar.gz` |
| OS-neutral interface | `ca-client-tools_interface_1.1.0.zip` |
| Checksums | `SHA256SUMS` |

Each platform archive contains:

```text
ca-client-tools_<version>_<os>_<arch>/
  ca-client-tools[.exe]
  provider.yaml
  interface/
    package.json
    tool-catalog.yaml
    capabilities.json
    schemas/*.schema.json
  README.md
  CHANGELOG.md
```

The standalone interface ZIP contains only the Interface Package, with
`package.json` at the archive root: `package.json`, `tool-catalog.yaml`,
`capabilities.json` and `schemas/*.schema.json`. It never contains the Go
executable, `provider.yaml`, credentials or private backend bindings.

Requirements enforced by the workflow:

- The Interface Package is generated **once** per tagged release and the same
  directory is copied into every platform staging tree.
- Each archive's `interface/` tree and `provider.yaml` are byte-identical to the
  canonical generation, and the standalone ZIP's uncompressed bytes are
  identical to every platform archive's `interface/` tree.
- `provider.yaml` identity/version/catalog digest match the binary's
  `initialize` response, and every published binding resolves in `catalog`.
- `SHA256SUMS` covers all archives, the standalone interface ZIP and the MAS
  runtime compatibility record. A generation, identity or consistency failure
  fails the whole release.

The workflow also downloads the published assets after upload and verifies
`SHA256SUMS` and the presence of all four archives.

Release binaries embed:

- `Version`: the exact semantic version tag, including the leading `v`;
- `GitCommit`: the full commit SHA from which the binary was built;
- `BuildDate`: the UTC build timestamp;
- `ProviderVersion`: the version without the leading `v`, reported by
  `initialize` and equal to `provider.yaml.providerVersion`.

The executable is both the operator CLI and the resident Provider process.
Use `ca-client-tools --version` to inspect the build identity.

## Local verification

Run from the repository root with the Go version declared in `go.mod`:

```bash
test -z "$(gofmt -l .)"
go test ./... -count=1
go test -tags providerfixture ./... -count=1
go vet ./...
go vet -tags providerfixture ./...
go vet -tags acceptance ./scripts/acceptance/
go test -race -count=1 ./internal/artifactchannel/ ./internal/provider/ ./internal/providerapp/ ./internal/sbi/executor/ ./internal/catalog/ ./internal/interfacepkg/
go test -tags providerfixture -race -count=1 ./internal/providerapp/ ./internal/catalog/ ./internal/artifactchannel/
go build ./...
```

Generate and verify the release package:

```bash
go run ./cmd/ca-client-tools-release generate --out /tmp/release --version 1.1.0 --provider-version 1.1.0
go run ./cmd/ca-client-tools-release verify   --out /tmp/release --version 1.1.0 --provider-version 1.1.0
```

Run the real Compiler link gate (downloads the pinned `ca-skill-compiler
v2.1.0` binary; verify its SHA-256 before use):

```bash
curl -sSLo /tmp/skillc https://github.com/odango-chan/ca-skill-compiler/releases/download/v2.1.0/skillc-v2.1.0-linux-amd64
echo "11da056ae5fff416de1fcfacd0a3535a4392a2b2d7349d928f41e1928398b568  /tmp/skillc" | sha256sum -c -
chmod +x /tmp/skillc
CA_SKILLC=/tmp/skillc go test ./internal/interfacepkg -count=1 -v
```

Live acceptance is separate from these local gates. Use the exact procedures and
pinned inputs in the dated evidence records. Never convert a missing lab,
skipped vector or unavailable GitHub Actions job into PASS.

## Publication procedure

1. Merge the reviewed release-preparation changes into `main`.
2. Re-read this document and [WORK](WORK.md); verify the intended release scope
   and all external contract pins.
3. Confirm the required checks are green. If GitHub Actions is unavailable,
   obtain and record a fresh full local acceptance run on the exact main SHA;
   report the CI limitation separately.
4. Inspect the existing Git tags and GitHub Releases. If the intended version
   already exists, do not move, delete or reuse it; select the next available
   version. Otherwise create the annotated tag on the approved `main` commit and
   push it. Do not tag a feature branch or an unmerged PR.
5. The tag-triggered Release workflow re-runs format, unit, fixture, vet, race
   and build gates; runs the Compiler link gate; cross-compiles the production
   binaries; generates the Interface Package and `provider.yaml`; packages and
   verifies the archives; and publishes the GitHub Release with SHA-256
   checksums.
6. Verify each asset, verify `SHA256SUMS`, and run `--version` on the Windows
   and Linux amd64 binaries before announcing the release.

The release workflow accepts stable `vMAJOR.MINOR.PATCH` annotated tags only,
and rejects a tag whose commit is not already reachable from `origin/main`. It
does not publish prereleases. No tag is created by the preparation PR itself.

## MAS contract archives

Stable runtime tags with MAS packaging build the candidate from the same
tagged commit. The historical `v1.0.0` tag predates MAS packaging.
The contract package has its own version (`0.3.0`) and the stable URI
`wtlab:packages:openamsm:mas-compiler-contract`. The Owner and declared
Publisher ID is `wtlab`. Downloads currently use ca-client-tools GitHub
Release assets. A WTLab HTTPS publication base is unconfirmed and independent
of the stable URI.

- The runtime release carrying the lifecycle candidate uses package
  version `0.3.0` and carries `openamsm-mas-compiler-contract-v0.3.0.zip`.
  It contains a 22-entry draft ToolCatalog and 42 distinct JSON Schemas:
  25 inherited byte-identically from v0.2.0 plus 17 new lifecycle projections.
  It also contains the source map, plan provenance, Schema inheritance record,
  static closure, Owner outcome contract and digest manifests. The earlier
  v0.2.0 archive remains immutable and is still reproducible from its versioned
  source directory.
- Every runtime release carries `mas-runtime-compatibility.json`, which
  records the runtime tag and source commit, along with the independent
  contract version, content digest, archive SHA-256 and the release hosting it.
  `SHA256SUMS` covers this record and the runtime archives. Runtime support
  for the candidate is marked `NOT_QUALIFIED`.
- Release jobs serialize lookup and upload. An existing package version is
  reused only after downloading it and matching its exact generated bytes.
  Preserve the first release and its archive. An ambiguous, incomplete or
  different archive fails the release. If GitHub replaces a queued release
  run, dispatch the workflow again for the existing annotated tag.
- For runtime-only changes, retain the contract version. For changed payloads,
  add a new contract source version, update the generator selection and add
  version-specific validation. Retain existing versioned Schema IDs only
  where their definitions stay byte-identical. v0.3.0 reuses unchanged v0.2.0
  Schema IDs and assigns v0.3.0 IDs only to new lifecycle projections. Keep
  prior archives immutable.

The draft catalog is marked `NOT_LINKABLE` and must not be confused with the
linkable Interface Package published by this release. Exports with formal
mutation semantics require verified runtime receipts and Reconcile support.
Evidence guarantees require verified evidence producers. CapabilityContracts and
EvidenceTrustBindings are Deployment-owned. Deployment must authorize the actual
WTLab publisher principal and bind its verified credentials and scope to the
package and digest before accepting a trusted snapshot.

The archive does not change MAS admission. Host ingress correlation continues
without an extra AMF registration query, CM CONNECTED gate or generic SMF
Session ABSENT gate.

## Repository licensing

The repository is public but currently has no `LICENSE` file. Public
visibility alone does not grant an open-source license. This release does not
imply permission to reuse or redistribute the code under an open-source
license; the repository owner should select an appropriate license separately.

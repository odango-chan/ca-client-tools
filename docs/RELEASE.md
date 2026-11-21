# Release process

Status: **v1.0.0 preparation. As checked on 2026-11-18, the repository has no
Git tag refs and no GitHub Releases. Re-check both immediately before
publication; this preparation does not create either.**

This document defines how to build and publish CA Client Tools. The first
stable release is intentionally limited to the implementation and acceptance
scope recorded in [WORK](WORK.md) and [the final acceptance evidence](evidence/provider-final-acceptance-20261118.md).

## Release scope

The proposed `v1.0.0` release contains:

- The resident Provider process using only
  `ca-client-tools.provider/v2`.
- The single embedded production Catalog and its eleven typed OpenAMSM read
  bindings.
- The shared SBI transport, typed OpenAMSM adapters and local CLI v1.
- The generic SBI executor and Payload Engine implementation.
- Host-owned artifact reference/streaming integration conforming to the pinned
  `ca-host.artifact-store/v1` contract.

It does **not** claim that every generic SBI operation or every 3GPP NF is
implemented. The production Catalog contains no production genericSBI binding.
Production Mutation dispatch remains disabled. Host/Runtime continue to own
callable admission, Capability authorization, execution identity, Outcome,
Evidence and Reconcile.

The binary/multipart executor and Host channel are covered by deterministic
fixtures and cross-repository Host acceptance. A real-NF multipart operation
has not been verified and must not be claimed as live-supported.

Issue [#8](https://github.com/odango-chan/ca-client-tools/issues/8) remains
open. It does not affect the current production Catalog because no production
genericSBI binding is shipped. Close it before adding a production
artifact-bearing genericSBI operation whose response media could be mixed or
otherwise non-projectable.

## Release artifacts

The tag-triggered workflow publishes these production binaries:

| Target | Archive |
|---|---|
| Windows amd64 | `ca-client-tools_1.0.0_windows_amd64.zip` |
| Linux amd64 | `ca-client-tools_1.0.0_linux_amd64.tar.gz` |
| Linux arm64 | `ca-client-tools_1.0.0_linux_arm64.tar.gz` |

Each archive contains the production executable, README and CHANGELOG. The
release also publishes `SHA256SUMS` covering all archives. Acceptance-only
`providerfixture` binaries are never published.

Release binaries embed:

- `Version`: the exact semantic version tag, including the leading `v`;
- `GitCommit`: the full commit SHA from which the binary was built;
- `BuildDate`: the UTC build timestamp.

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
go test -race -count=1 ./internal/artifactchannel/ ./internal/provider/ ./internal/providerapp/ ./internal/sbi/executor/ ./internal/catalog/
go test -tags providerfixture -race -count=1 ./internal/providerapp/ ./internal/catalog/ ./internal/artifactchannel/
go build ./...
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
4. Inspect the existing Git tags and GitHub Releases. If `v1.0.0` already
   exists, do not move, delete or reuse it; reconcile the release history and
   select the next available version. Otherwise create the annotated tag
   `v1.0.0` on the approved `main` commit and push it. Do not tag a feature
   branch or an unmerged PR.
5. The tag-triggered Release workflow re-runs format, unit, fixture, vet, race
   and build gates; cross-compiles the production binaries; packages them; and
   publishes the GitHub Release with SHA-256 checksums.
6. Verify each asset, verify `SHA256SUMS`, and run `--version` on the Windows
   and Linux amd64 binaries before announcing the release.

For this first `v1.0.0` publication only, the reviewed
`.github/workflows/bootstrap-v1.0.0.yml` automatically performs the annotated
main-commit tagging step after PR #9's merge commit passes the full main-push
`Go CI` workflow. It checks the exact CI head, requires PR #9's merge commit,
and does not move an existing tag. A tag push made by `GITHUB_TOKEN` does not
start another workflow, so this bootstrap explicitly dispatches `release.yml`
on the new annotated tag. The Release workflow independently repeats all gates
before publishing binaries. Later versions follow the ordinary tagging process.

A workflow dispatch from `main` may supply `release_tag=v1.0.0` for an
existing immutable annotated tag. The release job explicitly re-fetches the
remote tag object before validating its type, because checkout of a dispatched
tag can locally peel its tag reference to a commit. It then verifies the exact
tagged source commit and main ancestry before running release gates and building
assets. This is a recovery mechanism, not permission to retag or move a release.

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

- The next runtime release carrying the lifecycle candidate uses package
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

The draft catalog is marked `NOT_LINKABLE`. The v0.3.0 Release source plan still
has a blocking defect: r2/r3 reference `trigger` while declaring no start input.
That upstream plan must be corrected before linking. Exports with formal mutation
semantics require verified runtime receipts and Reconcile support. Evidence
guarantees require verified evidence producers. CapabilityContracts and
EvidenceTrustBindings are Deployment-owned. Build provenance describes the
tagged source, artifact and digests. Deployment must authorize the actual
WTLab publisher principal and bind its verified credentials and scope to
the package and digest before accepting a trusted snapshot.

The archive does not change MAS admission. Host ingress correlation continues
without an extra AMF registration query, CM CONNECTED gate or generic SMF
Session ABSENT gate.

## Repository licensing

The repository is public but currently has no `LICENSE` file. Public
visibility alone does not grant an open-source license. This release does not
imply permission to reuse or redistribute the code under an open-source
license; the repository owner should select an appropriate license separately.

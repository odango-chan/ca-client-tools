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

The release workflow accepts stable `vMAJOR.MINOR.PATCH` annotated tags only,
and rejects a tag whose commit is not already reachable from `origin/main`. It
does not publish prereleases. No tag is created by the preparation PR itself.

## Repository licensing

The repository is currently private and has no `LICENSE` file. This release
preparation does not invent a license or imply an open-source grant. If the
repository is made public, the owner must select and add an appropriate license
before presenting the project as open source.

# Provider P1 closeout evidence — 2026-11-18

**Branch:** `feat/unified-provider-read-tools-20261117` (Draft PR #7)
**Provider closeout code head:** `4c135ab0ac449b5315e493899ee46734242eb115`
**Previous head:** `f2e37d0e3a3f0ad75c02ce3861c6f8155b11c9be`
**Host counterpart:** `odango-chan/ca-agent` Draft PR #27,
`feat/host-artifact-store-p1-20261118`, closeout code head
`c9d2dcd` (see the Host evidence record).
**Pinned Host contract:** `ca-host.artifact-store/v1` at `ca-agent`
`2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae`
**Fixture identity:** `MANIFEST.sha256` SHA-256
`59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4`

Status vocabulary: `PASS`, `FAIL`, `NOT_RUN`, `NOT_VERIFIED`,
`CI_UNAVAILABLE`, `BLOCKED_ENV`.

> Scope: P1 closeout audit, one contract-conformance defect fix, a full local
> gate re-run at the closeout head, and the Lead-run cross-repository
> acceptance. It is not a real-NF multipart acceptance and it grants no Runtime
> admission/Capability authorization or production mutation.
> The earlier P1 implementation/acceptance record remains authoritative for its
> own pinned heads: [P1 evidence](provider-p1-20261118.md).

## 1. Contract pin and no-drift re-verification

| Item | Value |
|---|---|
| Contract identity | `ca-host.artifact-store/v1` |
| Contract document | `ca-agent@2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae` `docs/design/host-artifact-store-v1.md` |
| Pinned fixture copy | `contracts/host-artifact-store/v1/` (29 files) |
| Fixture manifest SHA-256 | `59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4` |
| Drift check | 29/29 files byte-identical to `ca-agent@2c0b07f:conformance/artifact-store/v1` (`git hash-object --no-filters` vs `rev-parse commit:path`); no missing/extra file; `TestPinnedManifest` verifies the same bytes in-repo |

No contract, schema or fixture change was made in this closeout.

## 2. Defect found and fixed

**Severity:** LOW-MEDIUM, contract §2.4/§5.3. **Reproduced before fixing.**

`internal/providerapp/artifacts.go` skipped reference-location validation
entirely for `genericSBI` bindings that declare no artifact slots
(`validateArtifactArguments` returned early when the binding had no slots), and
`internal/providerapp/runtime.go` only ran the check inside the artifact-only
branch. A Host-minted artifact reference smuggled through an open nested
argument object (a trusted catalog schema with `additionalProperties: true` at
a nested level) was forwarded verbatim to the remote NF — violating "a
reference is valid only at one declared slot of one invocation" and able to
disclose a Host invocation-bound token to the remote peer.

Fix (3 files, +24/-15):

- a no-slot binding is treated as an empty declared set and arguments are
  scanned for reference values anywhere;
- the check runs for every `genericSBI` invoke after argument-schema validation
  and before any remote or channel I/O;
- the `internal/artifactchannel` package doc now names the actual pinned
  contract commit `2c0b07f` (documentation-only update of accepted `3a0d140`;
  fixture bytes unchanged).

Reproducing test:
`internal/providerapp/artifact_reference_scope_test.go`
`TestGenericNonArtifactInvocationRejectsReferenceAtUndeclaredLocation` asserts
`BACKEND_ARGUMENTS_INVALID`, **zero upstream requests**, no token echo in the
fault, and that a reference-free invoke of the same binding still dispatches
once. Verified failing before the fix
(`invoke error = <nil>, want BACKEND_ARGUMENTS_INVALID`) and passing after.

A latent, currently unreachable projection edge found by the same audit is
tracked separately as **ca-client-tools#8** (artifact-bearing binding whose
declared response media types cannot be projected into closed output slots).
It is not fixed here because no shipped catalog entry can express it.

## 3. Local gates at the closeout code head

Windows-native, Go `go1.26.6 windows/amd64`, `GOWORK=off`, from the repository
root at `4c135ab`:

```text
gofmt -l .                                   # empty
go build ./...                               # exit 0
go vet ./...                                 # exit 0
go vet -tags providerfixture ./...           # exit 0
go vet -tags acceptance ./scripts/acceptance # exit 0
go test ./... -count=1                       # 13/13 packages ok, 0 fail
go test -race -count=1 ./internal/artifactchannel/ ./internal/provider/ ./internal/providerapp/ ./internal/sbi/executor/ ./internal/catalog/   # ok
go test -tags providerfixture ./...          # 13/13 packages ok, 0 fail
go test -tags providerfixture -race ./internal/providerapp/ ./internal/catalog/ ./internal/artifactchannel/  # ok
```

Build artifacts used for acceptance:

| Binary | Build | SHA-256 |
|---|---|---|
| `ca-client-tools-provider-fixture.exe` | `go build -tags providerfixture -o … ./cmd/ca-client-tools` | `1e289f86a5c0f9aa55d8e023faaae70804d0ace2293b5a2a3f7bcb0019a2249b` |
| `ca-client-tools-provider.exe` | `go build -o … ./cmd/ca-client-tools` | `4d15f4589ee6759728a3945d3b8e18e7212b9ec8fa0826ec178fe2164375d474` |

Byte-scan of the production binary: no `acceptance.artifact.*`,
`acceptance.generic.*`, `acceptance-artifact`, `acceptance-fixture-sbi`,
`providerfixture` or fixture-process-token strings; the same strings are
present in the `providerfixture` build. Production P1 code symbols are present
as expected.

Process-level acceptance (fixture binary, deterministic local loopback h2c
upstream, `-tags acceptance`):

```text
CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE=<fixture binary> \
go test -tags acceptance ./scripts/acceptance -run 'TestProviderGeneric|TestProviderArtifact' -count=1
# PASS 13/13 top-level, 13/13 subtests, 0 fail
```

## 4. Cross-repository acceptance at the closeout heads

Run by the Team Lead with the Host repository's acceptance suite:

| Item | Value |
|---|---|
| Host entrypoint acceptance | **PASS** — the real `cmd/producttask-server` Host binary started the real `providerfixture` `provider serve` child through the Host adapter; binary upload/download, two-in/two-out multipart round trip, optional-part omission, activation boundary, unauthorized/private status, crash cleanup (`TestNormalHostEntryArtifactAcceptance`) |
| Host live read acceptance through the normal entry | **PASS** — all eleven typed read operations through the same Host entry against the pinned `env-openamsm` `594dec78` lab (`TestNormalHostEntryLiveReadOnlyAcceptance`) |
| Host cross-repository E2E at the closeout heads | **PASS 43/43 top-level**, 0 fail — includes the prior 39 artifact/store/channel/process vectors plus the two normal-entry vectors and the new channel-timeout / negotiated-deadline regressions |
| Host closeout code head | `c9d2dcd` (`feat/host-artifact-store-p1-20261118`) |
| Provider closeout code head | `4c135ab0ac449b5315e493899ee46734242eb115` |
| Provider fixture binary | SHA-256 `1e289f86a5c0f9aa55d8e023faaae70804d0ace2293b5a2a3f7bcb0019a2249b` |

The detailed Host-side record (full-repository gates, the Host-internal
wiring, the audit fixes and the exact commands) is
`ca-agent:docs/evidence/host-artifact-store-p1-20261118.md`.

## 5. Real-NF multipart — NOT_VERIFIED

The pinned deployment exposes no source-verified, deployed multipart SBI
operation for this Provider. No operation was invented, no NF was modified and
no live multipart request was attempted. The deterministic local upstream plus
the real Host/Provider channel path remain the acceptance basis for the
transfer mechanism; a real-NF multipart operation remains an external
prerequisite.

## 6. GitHub CI

The closeout Provider head was pushed after these local gates. The Provider
repository's GitHub Actions job has never started on this account (billing /
spending-limit annotation, empty `steps` list), so the recorded status for this
head is `CI_UNAVAILABLE`; it is not a CI pass and not a test failure. The Host
repository has no GitHub Actions workflow. See the PR body for the exact run
id at the pushed head.

## 7. Explicit non-claims

No Runtime admission, Capability/Scope authorization, Owner Outcome, Evidence,
journal or reconcile state was exercised here. No production `genericSBI`
binding was added, no production mutation was dispatched and no real-NF
multipart operation was onboarded. Bytes still never travel in NDJSON, and no
caller value can select an origin, method, path, header or credential. The
Provider keeps no persistent artifact namespace; all artifact issuance,
storage, quota and lifecycle remain Host-owned.

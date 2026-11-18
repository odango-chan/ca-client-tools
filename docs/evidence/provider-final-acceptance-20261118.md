# Provider final acceptance and merge evidence — 2026-11-18

**Branch:** `feat/unified-provider-read-tools-20261117` (Draft PR #7)
**Provider acceptance head:** `5b93842035c72042b51e5a225da88bd21813c6e4`
**Host baseline:** `odango-chan/ca-agent` `main` at
`447d0cce7e42c7c1184b11fb2563792eead68d51` (merge commit of PR #27;
tree identical to the accepted branch head `bf9f730`)
**Pinned Host contract:** `ca-host.artifact-store/v1` at `ca-agent`
`2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae`
**Fixture identity:** `MANIFEST.sha256` SHA-256
`59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4`

Status vocabulary: `PASS`, `FAIL`, `NOT_RUN`, `NOT_VERIFIED`,
`CI_UNAVAILABLE`, `BLOCKED_ENV`.

> Scope: the final acceptance round for PR #7 against merged Host `main`. It
> re-runs the Provider repository gates, the merged-Host repository gates, the
> cross-repository E2E suite and the live Provider acceptance; it does not
> onboard a new operation, change the protocol/Catalog or open production
> mutation. Historical records remain authoritative for their own pinned
> heads and are not modified by this record.

## 1. Environment and exact inputs

| Item | Value |
|---|---|
| OS | Windows 11, Windows-native process execution (`cmd`/PowerShell) |
| Go | `go1.26.6 windows/amd64` |
| Workspace mode | `GOWORK=off` for both repositories |
| Host checkout | detached worktree at `ca-agent` `main` `447d0cc` |
| Host sibling SDK | `../tool-mcp-gw/sdk/go` at `bdded53` (`v0.1.0` development replacement) |
| Provider checkout | PR #7 branch at `5b93842` |
| Lab | pinned `env-openamsm` `594dec78`; loopback forwarder for SMF SBI `127.0.0.1:17777`, AMF metrics `19090`, SMF metrics `29090` |
| Token | injected only through the process environment from the deployment render; never printed, written to a file, logged or committed |

The Host worktree resolves the `tool-mcp-gw/sdk/go` development replacement
through a directory junction to the real sibling checkout; no module content
was copied or modified.

The acceptance head `5b93842` is the last code-changing commit. The
documentation commit that records this evidence changes only Markdown and does
not alter any Go input: rebuilding the binaries from the final pushed tree with
the flags in §7 reproduces the same digests.

## 2. Provider repository gates — PASS

At `5b93842`, from the repository root:

```text
gofmt -l .                                      # empty
go build ./...                                  # exit 0
go vet ./...                                    # exit 0
go vet -tags providerfixture ./...              # exit 0
go vet -tags acceptance ./scripts/acceptance    # exit 0
go test ./... -count=1                          # 13/13 packages ok
go test -tags providerfixture ./... -count=1    # 13/13 packages ok
go test -race -count=1 ./internal/artifactchannel/ ./internal/provider/ ./internal/providerapp/ ./internal/sbi/executor/ ./internal/catalog/   # ok
go test -tags providerfixture -race -count=1 ./internal/providerapp/ ./internal/catalog/ ./internal/artifactchannel/                         # ok
```

## 3. Merged Host `main` gates — PASS

At `ca-agent` `main` `447d0cc`:

```text
go build ./...                            # exit 0
go vet ./...                              # exit 0
go test ./... -count=1                    # 6/6 packages ok
go test -race ./artifactstore/ ./toolprovider/ ./cmd/producttask-server/ -count=1   # ok
```

The six packages are `ca-agent`, `artifactstore`, `internal/producttaskapi`,
`cmd/producttask-server`, `mcphost` and `toolprovider`. This is the merged
Host `main`, not the earlier PR branch.

## 4. Cross-repository E2E — PASS 43/43

Run against the merged Host `main` and the Provider PR head, with both
binaries built from the Provider source tree:

```powershell
$env:CA_CLIENT_TOOLS_PROVIDER_BIN = <providerfixture binary>
$env:CA_ACCEPTANCE_PROVIDER_EXE  = <production Provider binary>
$env:CA_ACCEPTANCE_QUERY_TOKEN   = <deployment-injected token>
GOWORK=off go test -tags e2e ./toolprovider/ -count=1 -v
```

```text
PASS 43/43 top-level, 0 FAIL, 0 SKIP
```

The suite includes `TestNormalHostEntryArtifactAcceptance` (real
`cmd/producttask-server` entry, real `provider serve` child, binary upload and
download, two-in/two-out multipart round trip, optional-part omission,
unactivated-operation rejection, crash cleanup, no orphan child) and
`TestNormalHostEntryLiveReadOnlyAcceptance` (the eleven typed read operations
through the same Host entry against the pinned lab). No test was skipped for a
missing environment variable.

## 5. Real OpenAMSM — 11/11 read operations PASS

`TestNormalHostEntryLiveReadOnlyAcceptance` activates exactly these eleven
operations on the real Host entry and invokes each one, failing on any
non-200 remote status or malformed projection:

| # | Operation | Result |
|---|---|---|
| 1 | `openamsm.amf.ue-info.list` | PASS |
| 2 | `openamsm.amf.gnb-info.list` | PASS |
| 3 | `openamsm.smf.pdu-info.list` | PASS |
| 4 | `openamsm.smf.mas.sc.query` | PASS |
| 5 | `openamsm.smf.mas.pa.query` | PASS |
| 6 | `openamsm.smf.mas.upn.query` | PASS |
| 7 | `openamsm.smf.mas.ssm.query` | PASS |
| 8 | `openamsm.smf.mas.iam.query` | PASS |
| 9 | `openamsm.smf.mas.ups.query` | PASS |
| 10 | `openamsm.smf.mas.ca.query` | PASS |
| 11 | `openamsm.smf.mas.saa.query` | PASS |

Context results are accepted only with `status` `AVAILABLE` or `UNAVAILABLE`;
every context result is required to echo the SUPI/PSI key and every list
result to contain `items` and `pager`. The direct Provider live vector
`TestProviderV2AllReadTools` in §6 independently invokes the same eleven
operations through the resident `provider serve` process and asserts the MAS
`masId`, echoed key and paged list shape.

## 6. Provider acceptance — PASS 21/21

With the deployment origins, token, SUPI `imsi-999700000000001` and PSI `1`
injected through the environment:

```text
GOWORK=off go test -tags acceptance ./scripts/acceptance/ -count=1 -v
# PASS 21/21 top-level, 0 FAIL, 0 SKIP
```

This includes the eleven live typed reads, one-protocol/one-Catalog,
legacy-identity rejection, the generic JSON vector, the Mutation gate,
deadline/cancellation and the P1 process-level artifact suite.

## 7. Binaries and fixture isolation

Built from the Provider source tree with
`-trimpath -buildvcs=false` so the digest is reproducible from source content
alone (documentation-only commits do not change the compiled inputs):

| Binary | Command | SHA-256 |
|---|---|---|
| `ca-client-tools-provider.exe` | `go build -trimpath -buildvcs=false ./cmd/ca-client-tools` | `2d43e32a6596240df8b461b69cd4bc9053385a0c6e071bf785c8348b1d0d2d70` |
| `ca-client-tools-providerfixture.exe` | `go build -trimpath -buildvcs=false -tags providerfixture ./cmd/ca-client-tools` | `964049d22d2e3dc9ba954d4ad32f76bad7776da40c873321ad09046ba91cd7d7` |
| `ca-client-tools.exe` (CLI) | `go build -trimpath -buildvcs=false ./cmd/ca-client-tools` | `2d43e32a6596240df8b461b69cd4bc9053385a0c6e071bf785c8348b1d0d2d70` |

Byte scan of the binaries:

- production build: none of `acceptance.artifact.*`,
  `acceptance.generic.*`, `acceptance-artifact`, `acceptance-fixture-sbi`,
  `providerfixture` present;
- fixture build: those markers present.

The `providerfixture` bindings are compiled only under the explicit
`providerfixture` tag (`internal/catalog/fixture.go`,
`internal/providerapp/fixture.go`; production uses the no-op
`fixture_stub.go`). The reported `backendBindingCatalogDigest` covers the
production artifact in both builds.

## 8. Contract pin — no drift

All 29 files under `contracts/host-artifact-store/v1/` are byte-identical to
`ca-agent@2c0b07f:conformance/artifact-store/v1` (`git hash-object
--no-filters` versus `git rev-parse`), with no missing or extra file. The
manifest digest is `59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4`.

## 9. GitHub CI — CI_UNAVAILABLE

For the acceptance head `5b93842`, GitHub Actions run
[44263932341](https://github.com/odango-chan/ca-client-tools/actions/runs/44263932341)
job `134663706743` concluded `failure` in ~2 s with an **empty `steps` list**
and no logs: the job never starts on this account (billing/spending-limit
annotation). This is `CI_UNAVAILABLE`; it is not a CI pass and not a test
failure. `ca-agent` has no GitHub Actions workflow.

## 10. Real-NF multipart — NOT_VERIFIED

The pinned deployment still exposes no source-verified, deployed multipart SBI
operation. No operation was invented, no NF was modified and no live multipart
request was attempted. The deterministic local upstream plus the real
Host/Provider channel path remain the acceptance basis for the transfer
mechanism; real-NF multipart is an external prerequisite.

## 11. Explicit non-claims

No Runtime admission, Capability/Scope authorization, Owner Outcome, Evidence,
journal or reconcile state was exercised. No production mutation was
dispatched and no production `genericSBI` binding was added. Bytes still never
travel in NDJSON, and no caller value can select an origin, method, path,
header or credential. All artifact issuance, storage, quota and lifecycle
remain Host-owned.

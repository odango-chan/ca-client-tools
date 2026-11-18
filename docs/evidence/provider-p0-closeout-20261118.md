# Provider P0 closeout evidence — 2026-11-18

**Branch:** `feat/unified-provider-read-tools-20261117` (draft PR #7)  
**Implementation/validation head:** `72fd75ea6a1ab8df5295a7bb0a835e4edd970a10`  
**Previous reviewed head:** `f592e48a3ad88fc0a5bfed7560f59cd2a45e5403`  
**Scope:** P0 closeout only: (1) the Generic SBI multipart/binary response
projection boundary, and (2) convergence of the typed metrics handlers onto the
one shared `internal/sbi` transport with an explicit trusted HTTP/1.1 profile.
The eleven typed reads, one protocol, one embedded Catalog, nested generic
dispatch and the production Mutation gate are unchanged. P1 Host artifact
streaming remains blocked and disabled.

Status vocabulary: `PASS`, `FAIL`, `BLOCKED`, `NOT_RUN`, `CI_UNAVAILABLE`.

> This record is for the implementation branch's validation head. It is not a
> product qualification, not a Host/Runtime integration acceptance, and not a
> claim that a standardized production 3GPP SBI operation or mutation is enabled.

## 1. Environment

- Host OS: Windows-native (Go 1.26.6 windows/amd64).
- Linux environment: WSL2 Ubuntu 26.04, Go 1.26.8 linux/amd64 (acceptance and
  Linux-native gates only; not a product dependency).
- Live NF environment: pinned `odango-chan/env-openamsm` commit
  `594dec78b95bff4fdc34deac8a3a9681fb41228f`, deployed in WSL with a real
  UERANSIM UE and an active PDU session.
- Live accepted origins (loopback-only acceptance network): SMF SBI
  `http://127.0.0.1:17777` (h2c), AMF metrics `http://127.0.0.1:19090`, SMF
  metrics `http://127.0.0.1:29090`.
- Query token: read at run time from the deployment helper; never printed or
  committed. No token, SUPI or raw remote body is stored here.
- Test UE: `imsi-999700000000001`, PDU session ID `1`.

## 2. Artifact identity (unchanged)

| Item | Value |
|---|---|
| Embedded Catalog | `contracts/provider-bindings/v2/provider-bindings.yaml` |
| Catalog SHA-256 (exact embedded bytes) | `sha256:79e0ea5c1d733357fcb98f11e16cfaee5e2ff00af2e4362060c72211edd2ea85` |
| Provider ID / protocol | `ca-client-tools.sbi` / `ca-client-tools.provider/v2` |
| Catalog version | `"2"` |

The closeout changes no catalog binding, no wire identity and no descriptor
shape. The only catalog change is build-tagged acceptance tooling
(`providerfixture`): two additional acceptance-only `genericSBI` bindings
(`acceptance.generic.binary`, `acceptance.generic.multipart`) that are absent
from every production build and never affect the reported digest.

## 3. Fix 1 — Generic SBI response projection boundary

### Defect

`invokeGeneric` checked `executed.JSON != nil` before `Body`/`Attachments`. A
multipart execution result can legitimately carry both a JSON root and
attachments, so the old order returned a successful JSON result and silently
dropped the attachments. Binary results were only rejected as a side effect of
the branch order.

### Resolution

- `projectGenericResult` now rejects any `Body` or `Attachments` result
  (`REMOTE_RESPONSE_INVALID`) **before** the JSON success branch. The fault
  carries no attachment bytes, metadata or remote content.
- `responseProjectionSupported` classifies a generic operation as projectable
  only when it declares no response parts and every declared response media
  type is JSON (`application/json` or a `+json` suffix). Empty response
  declarations (204) remain projectable.
- Catalog **availability** reports an artifact-bearing `genericSBI` binding as
  `available: false`, `reason: NOT_IMPLEMENTED`. The protocol layer therefore
  rejects an invoke with `BACKEND_OPERATION_UNSUPPORTED`
  (`detail.reason: NOT_IMPLEMENTED`) before any remote I/O.
- The **execution entrance** (`invokeGeneric`) repeats the same check before
  schema validation and before `Executor.Execute`, so availability reporting
  cannot be bypassed.
- The executor's independent binary/multipart request and response decoding,
  the Payload Engine and the executor fixtures are unchanged for P1.

### Verification

Unit (Windows-native and Linux-native `go test ./...`):

- `TestProjectGenericResultBoundaries` — JSON root plus attachments, attachments
  only, binary body, JSON 200/202 and empty 204. Asserts no partial result, no
  status, and no attachment/body content in the fault text.
- `TestResponseProjectionSupported` — JSON, `+json`, empty, binary, multipart
  and response-parts classification.
- `TestInvokeGenericRejectsArtifactOperationBeforeExecutor` — the runtime is
  constructed without an executor, so a bypass would panic; the fault is
  `BACKEND_OPERATION_UNSUPPORTED` / `NOT_IMPLEMENTED`.
- `TestRuntimeArtifactBindingsRejectedBeforeIO` (`providerfixture`) — both
  artifact fixture bindings are unavailable as `NOT_IMPLEMENTED`; invoking them
  returns the documented fault and the loopback upstream records zero requests.

Provider process (`providerfixture` binary, real NDJSON protocol):

- `TestProviderGenericArtifactBindingsRejectedBeforeIO` — catalog reports the
  binary and multipart fixture bindings in `unsupported` with
  `NOT_IMPLEMENTED`; both invokes return `BACKEND_OPERATION_UNSUPPORTED` with
  exactly one detail field (`reason`); the upstream observes zero requests; the
  process still serves a normal JSON invoke afterwards.

## 4. Fix 2 — Metrics shared transport convergence

### Design

`internal/sbi` now owns two reviewed `TransportProfile` values, built from the
same trusted `Limits` and trusted service registry:

| Profile | Wire behavior | Used by |
|---|---|---|
| `sbi-h2c` (default) | cleartext HTTP/2 prior knowledge; duplicate/non-HTTP/2 negotiation is a transport error; no HTTP/1.1 fallback | typed SMF MAS, genericSBI executor |
| `metrics-http1` | HTTP/1.1 only (cleartext or TLS); HTTP/2 response is a transport error | AMF/SMF metrics handlers, CLI metrics commands |

Each profile has one `http.Transport` and one `http.Client`; the shared client
selects it per trusted service key. Connection lifecycle, per-request timeout,
context cancellation, response size bounds, redirect refusal, disabled ambient
proxies, dial limits and idle/connection caps are therefore shared. The typed
metrics client applies its own bounded request context (5 s default, at most
30 s) and its source-verified 2 MiB default / 4 MiB maximum response bound on
top of the transport bounds.

The typed metrics handlers keep their behavior: `/ue-info`, `/gnb-info`,
`/pdu-info`, zero-based paging, `items`+`pager` validation, sequential page
links, unique-key JSON, status/`Content-Type` checks and the `InputError` /
`ConfigurationError` / `ProtocolError` / `RemoteError` taxonomy. The Provider
builds one `sbi.Client` from the merged registry and one
`metrics.NewClientWithSBI`; `ProviderRuntime.Close` closes only that shared
client. The standalone CLI still uses `metrics.NewClient`, which now also builds
the same shared transport/profile implementation instead of a separate
`http.Client`.

### Verification

- `internal/sbi/client_test.go`: profile protocol enforcement for both
  profiles, real HTTP/1.1 metrics transport, per-profile transport isolation
  (h2c and metrics services in one client), profile/origin validation
  (unknown profile rejected, TLS allowed for metrics, HTTPS rejected for h2c),
  service/profile lookup, and idempotent `CloseIdleConnections` across profiles
  followed by successful requests.
- `internal/openamsm/metrics/client_test.go`: typed metrics over the shared
  transport, shared-client configuration validation (nil transport, no keys,
  unknown key, wrong profile, invalid timeout/response bound), response-size
  limit, cancellation, timeout, and connection reuse (one connection across two
  calls). All pre-existing metrics paging/shape/redirect/origin tests pass
  unchanged.
- Existing MAS, Provider, CLI and generic executor suites pass unchanged.

## 5. Local gates at `72fd75e`

All commands from the repository root with `GOWORK=off`:

```text
gofmt -l .                                   # empty
go build ./...                               # exit 0
go vet ./...                                 # exit 0
go test ./... -count=1                       # all packages ok
go test -race -count=1 ./internal/provider/ ./internal/providerapp/ ./internal/catalog/ ./internal/sbi/... ./internal/openamsm/...
go test -tags providerfixture ./internal/providerapp/ ./internal/catalog/ -count=1   # ok
go vet -tags providerfixture ./internal/providerapp/ ./internal/catalog/             # exit 0
go vet -tags acceptance ./scripts/acceptance/                                        # exit 0
go build -o dist/ca-client-tools.exe ./cmd/ca-client-tools                            # exit 0
go build -tags providerfixture -o dist/ca-client-tools-provider-fixture.exe ./cmd/ca-client-tools  # exit 0
```

- Windows-native Go 1.26.6: all of the above **PASS**.
- Linux-native Go 1.26.8 (`scripts/acceptance/wsl-go-gates.sh`): `gofmt` clean,
  build/vet/test **PASS** → `ALL_LINUX_GATES_PASS`.

## 6. Live acceptance at `72fd75e`

Runner: `scripts/acceptance/windows-provider-e2e.ps1` and
`scripts/acceptance/wsl-provider-e2e.sh`, driving compiled production and
`providerfixture` Provider binaries over the real NDJSON protocol against the
pinned lab. The metrics handlers therefore run over the new shared
`metrics-http1` profile, and the typed MAS/generic paths over the unchanged
`sbi-h2c` profile.

### Windows-native (production + fixture binaries, real live lab)

`powershell ... windows-provider-e2e.ps1 -SmfSbiOrigin ... -AmfMetricsOrigin ...
-SmfMetricsOrigin ... -Supi imsi-999700000000001 -Psi 1` → **PASS**
(16/16 top-level tests, plus 8 invalid-argument subtests):

```text
PASS TestProviderV2AllReadTools
PASS TestProviderV2ProblemDetailsAndErrorBoundaries
PASS TestProviderV2CancellationDeadlineAndFraming
PASS TestProviderV2ConcurrentCalls
PASS TestProviderLegacyProtocolRejected
PASS TestProviderCleanEOFShutdown
PASS TestCLIRegressionLive
PASS TestCLIHelpSurfaceUnchanged
PASS TestProviderGenericInitializeCatalogAndDispatch
PASS TestProviderGenericUnconfiguredBindings
PASS TestProviderGenericArtifactBindingsRejectedBeforeIO
PASS TestProviderGenericArgumentsRejectedBeforeIO        (8/8 subtests)
PASS TestProviderGenericDuplicateKeyRejected
PASS TestProviderGenericMutationGate
PASS TestProviderGenericDeadlineExceeded
PASS TestProviderGenericCancellation
```

### Linux-native (compiled Linux binaries, same live NFs and loopback fixture)

`scripts/acceptance/wsl-provider-e2e.sh` → **PASS** with the same 16/16
top-level tests and 8/8 invalid-argument subtests.

The eleven typed reads (eight SMF context queries and the three AMF/SMF list
operations) ran through the real Provider on both platforms against the pinned
lab, including the refactored metrics transport. The process-level artifact
projection boundary, Mutation gate, deadline and cancellation assertions ran on
both platforms.

## 7. GitHub CI

For implementation head `72fd75e`, GitHub Actions run
[44233975105](https://github.com/odango-chan/ca-client-tools/actions/runs/44233975105)
concluded `failure` in ~2 seconds with **zero executed steps** (job
`134565525075` has an empty `steps` list and no logs). This is the previously
recorded GitHub account billing/spending-limit annotation: the job never starts.
It is **`CI_UNAVAILABLE`**, not a CI PASS and not a test failure. Local
Windows-native and Linux-native gates above are the acceptance evidence for this
head.

## 8. P1 dependency check (unchanged)

Checked on 2026-11-18 against `ca-agent` remote `main` head
`72842a50101f65a20b7335b6815f0f7255af16f9`: there is still no published/pinned
Host Artifact Store / BinaryRef / attachment contract for this Provider, and no
explicit authorization for the cross-repository integration. P1 remains
**BLOCKED and DISABLED**: no Host-owned reference is accepted at declared slots,
no invocation-scoped reader/sink is connected, and Provider binary/multipart
response projection is not implemented. The executor and Payload Engine keep
their independent binary/multipart capability as library-level evidence only.

## 9. Explicit non-claims

- No production `genericSBI` binding is shipped; the artifact fixture bindings
  exist only under `-tags providerfixture`.
- No mutation was dispatched. The Mutation gate was proven by pre-I/O rejection.
- No real-NF multipart operation was executed; P0 rejects artifact-bearing
  responses before remote I/O rather than projecting them.
- Host/Runtime integration, public ToolCatalog publication, Capability/Scope
  authorization, Owner Outcomes and Evidence are not exercised here.
- The imported OpenAMSM source snapshot was not modified.

# Provider P0 acceptance evidence — 2026-11-18

> This record covers the P0 slice at `dbcbd8ac84c625d28853b3cfa201838bfdac97b3`.
> The later P0 closeout (generic response projection boundary and metrics
> transport convergence at `72fd75ea6a1ab8df5295a7bb0a835e4edd970a10`) is
> recorded in [the P0 closeout record](provider-p0-closeout-20261118.md).

**Branch:** `feat/unified-provider-read-tools-20261117` (draft PR #7)  
**Validation head:** `dbcbd8ac84c625d28853b3cfa201838bfdac97b3` (the implementation tree tested below)  
**Starting head:** `dbb3cfe059b24c440aee8bf85e0a522c2f1101e6` (architecture-document commit, verified in branch history)  
**Scope:** P0 consolidated Provider: one process protocol, one embedded trusted
Catalog, unified trusted service registry/transport, genericSBI dispatch through
the existing executor and Payload Engine, preserved eleven typed reads and
typed MAS/metrics semantics, pre-I/O Mutation gate, process-level acceptance.
P1 Host artifact streaming was dependency-checked and remains disabled.

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
- Query token: read at run time through the existing get-token helper; never
  printed or committed. No token, SUPI or raw remote body is stored here.
- Test UE: `imsi-999700000000001`, PDU session ID `1`.

## 2. Artifact identity

| Item | Value |
|---|---|
| Single embedded Catalog | `contracts/provider-bindings/v2/provider-bindings.yaml` |
| Catalog SHA-256 (exact embedded bytes) | `sha256:79e0ea5c1d733357fcb98f11e16cfaee5e2ff00af2e4362060c72211edd2ea85` |
| Provider ID | `ca-client-tools.sbi` |
| Protocol identity | `ca-client-tools.provider/v2` (sole accepted identity) |
| Removed legacy identity | `ca-client-tools.provider/v1` — rejected with `PROTOCOL_VERSION_UNSUPPORTED` |
| Removed legacy Catalog | `contracts/provider-bindings/v1/provider-bindings.yaml` — deleted with its loader and version dispatch |

The live `initialize` result reported
`backendBindingCatalogDigest = sha256:79e0ea5c1d733357fcb98f11e16cfaee5e2ff00af2e4362060c72211edd2ea85`
and `catalogVersion = "2"`.

## 3. Local gates at the validation head

All commands from the repository root with `GOWORK=off`:

```text
gofmt -l .                                   # empty
go build ./...                               # exit 0
go vet ./...                                 # exit 0
go test ./... -count=1                       # all packages ok
go test -race -count=1 ./internal/provider/ ./internal/providerapp/ ./internal/catalog/   # ok
go test -tags providerfixture ./internal/providerapp/ -count=1                            # ok
go vet -tags acceptance ./scripts/acceptance/                                             # exit 0
```

- Windows-native Go 1.26.6: all of the above PASS.
- Linux-native Go 1.26.8 (`scripts/acceptance/wsl-go-gates.sh`): `gofmt` clean,
  build/vet/test PASS → `ALL_LINUX_GATES_PASS`.
- Unit coverage added for: legacy protocol rejection before/after initialize and
  before runtime creation; single-catalog load and legacy `/v1` artifact
  rejection; generic `argumentSchema` structural rules (closed object, required
  properties, path/query/body/idempotency agreement, scalar-argument
  rejection); recursive nested validation before I/O; 204 without a `response`
  member; Mutation gate; trusted service registry conflicts and invalid service
  files; shared-transport typed MAS and generic executor wiring under the
  `providerfixture` build tag.

## 4. Process-level acceptance (real `provider serve` child process)

Runner: `scripts/acceptance/provider_e2e_test.go` and
`scripts/acceptance/provider_generic_e2e_test.go` (build tag `acceptance`),
driving compiled binaries over the real NDJSON stdin/stdout protocol. Two
binaries are used: the production build (no tags) and the explicit
`-tags providerfixture` build whose test-only trusted bindings never appear in
a production binary. The generic cases use a deterministic loopback h2c
upstream inside the test process and a trusted `serviceconfig` v1 file for the
acceptance-only service key.

### Windows-native (production + fixture binaries, real live lab)

`go test -tags acceptance ./scripts/acceptance/ -count=1 -v` → **PASS**
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
PASS TestProviderGenericArgumentsRejectedBeforeIO        (8/8 subtests)
PASS TestProviderGenericDuplicateKeyRejected
PASS TestProviderGenericMutationGate
PASS TestProviderGenericDeadlineExceeded
PASS TestProviderGenericCancellation
```

### Linux-native (compiled Linux binaries, same live NFs and loopback fixture)

`scripts/acceptance/wsl-provider-e2e.sh` → **PASS** with the same 16/16 tests
and 8/8 invalid-argument subtests.

### Assertions covered

- **Single protocol / single Catalog:** initialize reports the sole protocol,
  `catalogVersion: "2"` and the exact embedded-artifact digest; a v1 initialize
  is rejected with `PROTOCOL_VERSION_UNSUPPORTED` and creates no runtime, then
  the same process initializes with the sole identity; v1 frames after
  initialize are rejected; the acceptance runner contains no v1 compatibility
  test path.
- **Catalog partition and hygiene:** all eleven production read IDs plus the two
  fixture IDs are partitioned in request order with descriptors; descriptors
  expose no origin/URL/method/path/serviceKey/token/trusted header; generic
  descriptors expose `argumentSchema` and omit scalar `arguments`; typed
  descriptors keep their two scalar arguments; a fixture-only run proves the
  eleven typed IDs are `NOT_CONFIGURED` without typed deployment settings.
- **Eleven typed reads:** every `openamsm.smf.mas.*.query` returns a typed
  `{masId,status,key,context}` envelope with exact SUpi/PSI echo and
  AVAILABLE/UNAVAILABLE semantics; AMF `/ue-info`, AMF `/gnb-info` and SMF
  `/pdu-info` return `items`/`pager` pages with live membership assertions.
- **Generic dispatch:** nested object and array arguments round-trip; integer
  `9007199254740993` is preserved exactly; the trusted optional query default
  `mode=ok` is applied; trusted `POST /acceptance/v1/echo` and the fixed
  `X-Acceptance-Trusted: fixture-7` header are observed on the upstream while
  caller arguments cannot add headers/URLs/methods; JSON response preserved;
  202 preserved with body; 204 serialized as exactly `{"httpStatus":204}`;
  valid `application/problem+json` 404 is a successful protocol result
  `{httpStatus:404,problem:{status:404,cause}}`; malformed problem and
  non-problem non-200 map to `REMOTE_RESPONSE_INVALID`.
- **Before-I/O rejection:** eight nested/top-level invalid argument cases
  (undeclared `header`/`url`/`method`, undeclared nested field, wrong nested
  type, missing required field, below minimum, wrong array shape) return
  `BACKEND_ARGUMENTS_INVALID` with zero upstream requests; a nested duplicate
  JSON key returns `PROTOCOL_FRAME_INVALID` with zero upstream requests.
- **Deadline / cancellation:** a 100 ms negotiated call bound on a slow upstream
  yields `CALL_DEADLINE_EXCEEDED`; an in-flight slow invoke plus `cancel` yields
  `CANCEL_REQUESTED` and exactly one terminal `CALL_CANCELLED` response.
- **Mutation gate:** `acceptance.generic.mutation` is rejected with
  `BACKEND_OPERATION_UNSUPPORTED` / `detail.reason: "MUTATION_GATED"` and the
  upstream observes zero mutation requests.
- **Typed regressions:** remote ProblemDetails as a successful result, invalid
  typed arguments before dispatch, unknown IDs, duplicate request IDs, malformed
  envelopes, clean EOF shutdown and the unchanged CLI v1 surface/commands.

## 5. P1 dependency check (Host Artifact Store)

Checked on 2026-11-18 against the Host owner repository `ca-agent` remote `main`
head `72842a50101f65a20b7335b6815f0f7255af16f9`: there is no published/pinned
Artifact Store / BinaryRef / attachment contract for this Provider and no
accepted contract URL+commit recorded in this repository. The four required
contract areas in [Host integration](../host-integration.md) (reference shape
and stream handoff; cross-process transfer/adapter/stream/sink interfaces and
quotas; attachment publication/finalization and cleanup; stable error mapping
and shared conformance fixtures) are all absent, and no explicit authorization
for the cross-repository integration exists.

P1 therefore remains **BLOCKED and DISABLED**: no Host-owned reference is
accepted at declared slots, no invocation-scoped reader/sink is connected, and
Provider binary/multipart response projection is not implemented. This is an
external owner dependency, not a P0 defect; P0 does not depend on it.

## 6. GitHub CI

For validation head `dbcbd8ac84c625d28853b3cfa201838bfdac97b3`, GitHub Actions
run [44224604766](https://github.com/odango-chan/ca-client-tools/actions/runs/44224604766)
concluded `failure` in ~2 seconds with **zero executed steps** (job
`134534578118` has an empty `steps` list and no logs). This is the previously
recorded GitHub account billing/spending-limit annotation: the job never
started. It is **`CI_UNAVAILABLE`**, not a CI PASS and not a test failure. Local
Windows-native and Linux-native gates above are the acceptance evidence for this
head.

## 7. Explicit non-claims

- Host/Runtime integration, public ToolCatalog publication, Capability/Scope
  authorization, Owner Outcomes and Evidence are not exercised here; those are
  separate repositories.
- No production `genericSBI` binding is shipped. The generic process acceptance
  uses acceptance-only trusted bindings in the explicit `providerfixture` build
  plus a deterministic loopback upstream; no standardized 3GPP SBI operation was
  executed end-to-end.
- No mutation was dispatched. The Mutation gate was proven by pre-I/O rejection.
- Live `UNAVAILABLE` SMF context responses remain `NOT_RUN` (the lab always
  returns `AVAILABLE`; no approved fault injection). Real-NF multipart
  operations and binary/multipart Provider projection remain `NOT_RUN`/P1.
- The imported OpenAMSM source snapshot was not modified.

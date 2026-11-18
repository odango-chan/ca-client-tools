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

## P1 — Host artifact streaming (BLOCKED, DISABLED)

P1 requires the Host owner (`ca-agent`) to publish and pin the accepted Host
Artifact Store contract described in [Host integration](host-integration.md):
reference JSON shape/scope/authentication and the reference-to-stream handoff;
the cross-process transfer mechanism plus adapter bootstrap/configuration,
stream and sink interfaces, size/digest metadata and quotas; attachment
publication/finalization including cancellation, timeout, partial writes,
expiry and crash cleanup; and stable error mapping plus shared conformance
fixtures.

Checked on 2026-11-18: `ca-agent` remote `main` head
`72842a50101f65a20b7335b6815f0f7255af16f9` contains no published Artifact
Store/BinaryRef/attachment contract for this Provider, and no accepted contract
URL plus commit is recorded here. There is no explicit authorization for the
required cross-repository integration. Therefore P1 remains disabled: no
Host-owned reference is accepted at declared slots, no invocation-scoped
reader/sink is connected, and Provider binary/multipart response projection is
not implemented. The local payload engine and generic executor binary/multipart
paths remain library-level evidence only. Enabling P1 requires the Host contract
publication/pin and integration authorization first; mock executor fixtures
alone do not complete P1.

## Production Mutation gate

Enabling a concrete mutation operation requires pinned authoritative
source/deployment evidence, explicit Host and Runtime authorization,
operation-specific idempotency/reconciliation semantics and reviewed acceptance.
No automatic retry or implicit polling exists in the Provider or the executor.

# CA Client Tools architecture

Accepted target for demo/internal infrastructure. Optimize for one Provider protocol, one Catalog and a unified execution stack, with no backward-compatibility requirement. [WORK](WORK.md) records current implementation.

## 1. Purpose

CA Client Tools provides the network client capabilities used by CA-Agent.
Its target architecture includes a reusable Go SBI HTTP/2 client, a trusted
Operation Catalog, a CA-Agent Tool Provider, OpenAMSM-specific adapters and a
stable local CLI.

The repository is not an Agent, Runtime, workflow engine, Provider business
service or OpenAMSM environment manager. Its transport and payload engine are
designed to support individual 3GPP SBI operations, including read and
mutation operations, without making arbitrary HTTP requests available to the
Agent.

The current deployment/source target remains OpenAMSM. Generic SBI transport
does not imply Open5GS compatibility or prove that every 3GPP API is
implemented by OpenAMSM.

Deployment targets are Windows-native, Linux-native, Docker, Kubernetes and
remote 5GC/NF deployments. WSL is not a product dependency or a deployment
target; it appears only in the optional Windows-native acceptance tooling.
Product code and configuration contain no WSL detection, WSL-specific
configuration fields or WSL-specific network handling.

The detailed normative design is
[docs/sbi-client-architecture.md](sbi-client-architecture.md).

## 2. Invocation and ownership

```text
ca-agent Host → resident Provider → trusted Catalog → Provider Runtime
                                                   ├─ typed MAS / metrics handlers
                                                   └─ Generic SBI executor
                                                               ↓
                                          shared transport and Payload Engine → NF
```

See [Host integration](host-integration.md) for repository boundaries and process ownership. The separate local operator CLI uses typed clients under its own contract.

## 3. Trusted dispatch

The sole [Provider protocol](provider-protocol-v2.md) resolves a private backend operation ID through the sole [Catalog](provider-backend-binding-catalog-v2.md). Validate nested JSON against the trusted invocation schema and dispatch to the binding's declared handler. The Runtime must connect the [Generic SBI executor](generic-sbi-executor.md), using reviewed wire definitions and trusted service configuration. Keep typed handlers for MAS identity binding and remote semantics, and metrics paging, response shape and error semantics. Keep production Mutation gated under [WORK](WORK.md).

## 4. Transport and payloads

Reuse the [shared transport and Payload Engine](sbi-client-architecture.md). Transport policy is trusted per service through one of two reviewed profiles: `sbi-h2c` (cleartext HTTP/2 prior knowledge, no HTTP/1.1 fallback) and `metrics-http1` (the source-verified OpenAMSM metrics listener path). Both share lifecycle, cancellation, timeout, redirect, proxy and size bounds; the typed metrics decoding and error semantics stay source-specific. P0 accepts nested JSON. P1 uses Host Artifact Store references and invocation-scoped streaming through the pinned [Host contract](host-integration.md): a binary or `multipart/related` response is projected as reference objects only when an `artifactChannel` is configured, and artifact-bearing bindings are `NOT_CONFIGURED` otherwise.

## 6. Tool visibility

The existence of a client method, CLI command or Operation Catalog entry does
not make it Agent-callable. The Host determines the active Skill's toolset;
Runtime owns admission and Capability authorization.

Each operation is independently classified as Agent-callable or
Host/Tool-internal. Sensitive AMF metrics and raw subscriber projections
remain Host-private unless an explicit contract authorizes a safe projection.
A mutation operation is not implicitly exposed merely because its API
definition exists.

## 7. Procedure and lifecycle boundaries

A Tool represents an individual network operation. It does not implement a
multi-step 3GPP workflow, hide multiple independent request/response actions,
wait for asynchronous notifications, maintain pending-operation journals,
register callbacks, resume a Procedure, or create Runtime Evidence/Outcome.

A 202 Accepted response and its declared correlation headers are returned to
the Host. Host/Runtime own pending-operation correlation and continuation.

## 8. Source and deployment evidence

The canonical NF responsibilities and current source availability are in
[docs/nf-capability-map.md](nf-capability-map.md). Exact OpenAMSM endpoint and
wire facts are in [docs/openamsm-interface-inventory.md](openamsm-interface-inventory.md).

3GPP_DEFINED, SOURCE_VERIFIED, CLIENT_IMPLEMENTED, FIXTURE_TESTED,
SOURCE_TESTED and LIVE_ACCEPTED are separate evidence states. Do not promote a
client to source- or live-supported based only on a mock fixture or the
existence of a 3GPP specification.

## 9. Legacy `ca-provider-5gc` migration boundary

`ca-provider-5gc` is a migration source and transitional implementation, not
a permanent parallel 5GC tool plane. New client-side NF callables and
reusable operation-specific protocol codec **adapters** belong in this repository;
prefer reviewed upstream NAS/NGAP libraries to locally authored binary codecs; the old Provider should receive only
the safety fixes needed to keep the bridge working and is retired after the
migration gates pass.

Do not copy the Provider HTTP server, Runtime ObservationSource, Runtime
correlation/Evidence handling, CreatedSession workflow state, PCF/UDM
orchestration, Fabric journals or mutation adapters. A low-level reader or
parser may be reused only after separating transport and source parsing from
Provider/Runtime semantics and verifying its exact compatibility with
OpenAMSM. Package layout uses NF/protocol names (AMF, SMF, NAS, NGAP); do not
create a `fivegc/` namespace or an Open5GS compatibility layer.

## 10. NGAP/NAS upstream codec decision (2026-11-23)

**Status: accepted target, not implemented.** For the protocol-only layer, use
the **pure-Go** [free5gc/ngap](https://github.com/free5gc/ngap) and
[free5gc/nas](https://github.com/free5gc/nas) modules as the first candidates.
Pin reviewed tagged versions/commit SHAs and preserve applicable upstream
notices. Do not add the C `oai-libngapcodec`, `asn1c`, CGO, a C compiler or a
Linux/WSL runtime dependency to the Provider. A Go dependency is not considered
qualified merely because its `go.mod` resolves: actual required 3GPP wire
profiles, extensions, failure behavior, and both target platforms must pass
tests before replacing existing code.

The architecture distinguishes the **NGAP-PDU** between AMF and gNB from the
**N2 SM Transfer container** transported inside SBI `multipart/related`.
The CA Agent Host does not become an NGAP endpoint and does not need a complete
RAN NGAP transport stack. It must retain exact original N1 NAS/N2 SM bytes,
Content-ID, Content-Type, digest and private Owner correlation. Only operations
that genuinely require typed fields invoke a reviewed codec; decoding does not
authorize mutation, replace raw bytes, or invent a QoS/business delta.

### Ownership and migration scope

| Layer | Target responsibility | Current migration candidate |
| --- | --- | --- |
| `ca-client-tools` | Common pure-Go codec dependency + narrow, versioned NAS/NGAP protocol adapters; any needed trusted Provider-private parse/validate entry points | Replace local `protocol/nas5gsm` hand codec once parity is proven |
| `ca-agent-demo` | Ingress multipart preservation, request classification, Tool/Runtime dispatch; no second Python binary protocol implementation | `integrations/fivegc/nas5gsm.py`, NAS/N2-specific portions of `nsmf_n1.py` |
| `env-5gc-sim` Mock AMF | Build/inspect its simulated AMF-owned N1/N2 payloads with the shared Go libraries; retain Mock AMF state machine and realistic AMF behavior | `internal/mockamf/n2transfer/*` and any duplicate NAS framing |
| `env-openamsm` | Independent native Owner's existing NAS/N2/NGAP semantics and authoritative execution | **Not** a migration target; native C implementation is an interoperability oracle |

Do not expose protocol helper functions as general Agent-callable Tools by default.
If the Host requires NAS/N2 decode *before* Skill admission (e.g. Nsmf
front-door routing), provide a tightly scoped **Host-private** callable via the
already selected Provider process or a shared reviewed package boundary. This
must not produce a circular Host/Provider bootstrap dependency or require a
separate CLI process per message. Runtime and Skill ToolCatalog stay distinct
from protocol-only helper API. Whether a particular parsed field belongs in a
published Tool interface is a separate explicit decision.

### Cross-platform contract

The Provider must build and execute its codec conformance tests on
**Windows-native amd64** and **Linux amd64** with `CGO_ENABLED=0`, without
WSL, MinGW, Visual Studio C build tools, `asn1c`, Linux headers or kernel
modules. Add macOS native tests when a runner is available; a darwin
cross-build is additional evidence, **not** a native test. The full
OpenAMSM/UPF/RAN lab is allowed to run on Linux or remote hosts; only the
codec and Client Tools binary are required to run natively on Windows.
No new platform support is claimed until the actual gates pass.

### Mandatory compatibility and removal gates

1. **Inventory and pin** every hand-written NAS and N2 Transfer parser/encoder
   and related tests in all three repos. Verify upstream license, exact source
   version, required Release/IE/extension coverage and dependencies (including
   build tags). Record explicitly unsupported inputs.
2. **Decode + encode real vectors** captured from admitted OpenAMSM, Mock AMF
   and 5GC Create / Modify / Release traces. Include establishment, QoS modify,
   release request/complete, N2 Setup/Modify/Release Transfer and failure/extension
   variants. Assert semantic field parity, correct identity binding, correct
   APER/NAS wire output for approved profiles and deliberate preservation of
   unrecognized original bytes; do not assume arbitrary re-encoding is
   byte-for-byte invariant.
3. **Negative and fuzz/property gates:** truncated/duplicate/invalid IE,
   unknown critical extension, overlong lengths, unsupported encoding, wrong
   message type, PSI/PTI mismatch, malformed APER and payload-bomb cases must
   fail closed. Existing Host routing/Owner semantics and Mock AMF behavior
   must not regress.
4. **Platform gates:** `CGO_ENABLED=0 go test ./...`, `go vet ./...`,
   native Windows build and focused codec test, native Linux build and focused
   codec test; optionally macOS native. No claim of Windows-native
   interoperability based solely on cross-compilation.
5. **Cutover one owner at a time** after equivalence: Go codec adapter first,
   Mock AMF consumers second, Python Host-specific parsing last. Remove replaced
   code and redundant fixtures; keep multipart/raw preservation and necessary
   business routing. No parallel long-lived fallback parser or new compatibility
   switch. Update release/license notices and do not claim live E2E without
   corresponding real environment evidence.

This decision changes neither the frozen CA-SL Interface Package schema nor
the [vNext Provider release package](tool-provider-release-package-vnext.md).
Protocol codec packages are implementation dependencies, not automatically
Compiler-visible ToolCatalog entries. The initial design and accepted
target are tracked here; implementation is a separate cross-repository task.

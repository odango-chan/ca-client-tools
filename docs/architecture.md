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
reusable operation-specific protocol codec **adapters** belong in this
repository. Prefer reviewed upstream NAS/NGAP libraries to locally authored
binary codecs. The old Provider should receive only
the safety fixes needed to keep the bridge working and is retired after the
migration gates pass.

Do not copy the Provider HTTP server, Runtime ObservationSource, Runtime
correlation/Evidence handling, CreatedSession workflow state, PCF/UDM
orchestration, Fabric journals or mutation adapters. A low-level reader or
parser may be reused only after separating transport and source parsing from
Provider/Runtime semantics and verifying its exact compatibility with
OpenAMSM. Package layout uses NF/protocol names (AMF, SMF, NAS, NGAP); do not
create a `fivegc/` namespace or an Open5GS compatibility layer.

## 10. Native dynamic NGAP/NAS codec modules (2026-11-23)

**Status: design direction; implementation, ABI and module coverage are not
yet qualified.** CA needs generic, replaceable protocol codec modules. Full
coverage of an entire 3GPP Release is not a prerequisite. Each module must
declare exact coverage; current Create / Modify / Release flows must not
define the maximum protocol surface.

### 10.1 Standards baseline and coverage declaration

Use 3GPP Release 19 as the current reference baseline:

- [3GPP TS 38.413](https://portal.etsi.org/webapp/WorkProgram/Report_WorkItem.asp?WKI_ID=79727):
  NG Application Protocol (NGAP), reference version v19.4.0.
- [3GPP TS 24.501](https://portal.etsi.org/webapp/WorkProgram/Report_WorkItem.asp?WKI_ID=79501):
  5GS Non-Access-Stratum (NAS), reference version v19.8.0.

A codec module may implement a subset of a Release or multiple Release
profiles. That is acceptable only when the module declares exact coverage and
unsupported behavior. Never infer "full R19 support" from a handful of working
messages.

Coverage must be machine-readable and traceable to:

- protocol and exact 3GPP TS / version / Release baseline;
- module ID, module version and stable Codec ABI version;
- upstream project, exact version and source commit, plus license;
- every implemented NGAP PDU outcome / procedure / message and IE/nested
  type, with separate encode and decode status;
- every implemented 5GMM / 5GSM message and IE, with separate encode and
  decode status;
- constraints, extension / unknown-IE behavior and negative cases;
- test-vector, round-trip, fuzz and interoperability evidence;
- explicitly unsupported messages, IEs, options and known limitations.

Use explicit states such as `ENCODE_DECODE`, `DECODE_ONLY`,
`ENCODE_ONLY`, `NOT_IMPLEMENTED`, `UNVERIFIED`, and
`UNSUPPORTED_BY_DESIGN`. Summary percentages never replace the itemized
inventory. A message being unused by the current demo is not grounds to omit
it from the inventory.

### 10.2 Module artifact and stable C ABI

The primary module artifact is a native shared library:

- Linux: `.so`
- Windows: `.dll`
- macOS: `.dylib`

The module boundary is a **versioned C ABI**, not Go's `plugin` mechanism.
This allows the implementation to be written in Go, C, C++ or Rust without
exposing language-specific objects to the caller.

The ABI must use only fixed-width integers, opaque instance handles and
caller-owned byte buffers. No Go structs/slices, C++ classes/exceptions,
Rust-owned objects, or allocator-owned memory may cross the boundary. The
initial symbol set is:

```c
uint32_t ca_codec_abi_version(void);

int32_t ca_codec_create(
    const uint8_t *config, uint32_t config_len,
    uint64_t *instance);

int32_t ca_codec_invoke(
    uint64_t instance,
    const uint8_t *request, uint32_t request_len,
    uint8_t *response, uint32_t *inout_response_len);

void ca_codec_destroy(uint64_t instance);
```

The call payload is a versioned, language-neutral envelope (Protobuf is the
current candidate) supporting `DECODE`, `ENCODE`, `VALIDATE` and
`GET_COVERAGE`. A decode request carries original wire bytes and a selected
spec profile; its response carries message identity, structured fields,
validation diagnostics and coverage status. Encode accepts a schema-addressed
typed message representation. The exact canonical field-tree schema is a
separate ABI contract deliverable; it must support nested CHOICE/SEQUENCE,
lists, enums, bit strings, octet strings and extension/open-type data without
loss.

The ABI must define calling convention, symbol visibility, thread-safety,
error codes, maximum request/response sizes and buffer-too-small behavior.
The module does not allocate memory for the caller: the caller supplies the
output buffer and retries with the reported required size. ABI major changes
are incompatible; minor additions must remain backward compatible.

A Go codec can be built as a C shared library with Go's
`-buildmode=c-shared`; this requires `CGO_ENABLED=1` and a suitable C
toolchain for that **module build**. Go documents C-shared build support for
Windows DLLs as well as Linux shared libraries. Windows Go 1.25+ CGO builds
also require a sufficiently recent GCC/binutils toolchain. See
[Go build modes](https://go.dev/cmd/go/#hdr-Build_modes) and
[Go minimum requirements](https://golang.google.cn/wiki/MinimumRequirements).
A pure-Go executable worker over the same logical RPC contract is an allowed
artifact fallback when producing a shared library is not worthwhile.

### 10.3 Loading and process isolation

Do not load arbitrary codec libraries directly into the CA Agent or the
long-lived Go Provider process by default. The default is:

```text
CA Host / Go Provider / Mock AMF
              |
       CodecClient (private API)
              |
      Protocol Codec Runtime
       manifest + profile router
              |
       framed local IPC (resident)
              |
       Native Codec Loader worker
          LoadLibrary / dlopen
              |
       +------+-------+
       |              |
   ngap.dll        libnas.so
```

The Runtime and loader worker are long-lived; **never start a process per
message**. The worker loads the C ABI library and exchanges length-framed
request/response envelopes over inherited stdio pipes or an equivalent local
transport. No public network listener is required. Prefer one worker per
module version when independent failure isolation is needed.

This keeps the existing Go Provider v2 public process contract
(`initialize/catalog/invoke/cancel`) and ToolCatalog unchanged. Codec modules
are not Agent-callable Tools, do not create Capability grants, and are not a
second Tool Provider or a new public Tool registry. Host-private codec calls
must use an explicitly private interface and must not be admitted as model
operations. The same module contract can be consumed by the Python Host and
Go Mock AMF without duplicating protocol code.

A direct in-process C ABI loader may be considered later as an optimization,
but it would introduce CGO/FFI and share crash/memory risk with the caller.
Go's `plugin` package is not the selected mechanism: it is not supported on
Windows and has lifecycle/toolchain limitations
([Go plugin documentation](https://pkg.go.dev/plugin)).

### 10.4 Manifest and profile routing

Each module package contains a manifest (for example `codec.yaml`) with:

- `moduleId`, `moduleVersion`, `codecAbiVersion`;
- protocol (`ngap` or `nas`);
- 3GPP TS, Release and exact specification version profiles;
- operations and itemized message/IE coverage;
- upstream provenance, commit and license;
- target OS/architecture and artifact name;
- checksums/signature, dependencies and resource limits.

Protocol Release, module version and ABI version are independent dimensions.
NGAP and NAS modules may be versioned independently, while a peer profile
binds the compatible pair:

```yaml
protocolCodecs:
  profiles:
    openamsm-r19:
      ngap:
        module: ngap-free5gc
        spec: TS 38.413@19.4.0
      nas:
        module: nas-free5gc
        spec: TS 24.501@19.8.0
  peers:
    openamsm:
      profile: openamsm-r19
```

This is illustrative, not a committed configuration schema. Do not assume
that an individual NGAP or NAS packet always identifies its 3GPP Release.
Select a profile from trusted peer/deployment configuration or explicit
session context; if unknown, fail clearly rather than guess or silently fall
back to another Release.

### 10.5 Safe module replacement

Modules are installed into immutable versioned directories; never overwrite a
loaded DLL/SO in place. Replacement is:

1. Verify package signature/checksum, platform, dependencies and manifest.
2. Verify ABI compatibility and declared coverage.
3. Start a new loader worker with the new module.
4. Run ABI handshake, health check and module conformance smoke tests.
5. Atomically route new requests to the new worker.
6. Allow in-flight calls on the old worker to finish, then terminate it.

Terminate the old worker process rather than relying on in-process
`dlclose` / `FreeLibrary` for Go runtimes or arbitrary native libraries.
A failed upgrade leaves the old worker/profile active. Modules are trusted
executable code: install only trusted/signed artifacts, apply least privilege,
request-size limits and timeouts. Process separation improves fault isolation
but is not, by itself, a complete security sandbox.

### 10.6 Upstream candidates

First candidates remain the pure-Go modules
[free5gc/ngap](https://github.com/free5gc/ngap) and
[free5gc/nas](https://github.com/free5gc/nas). Candidate tags inspected on
2026-11-23 include NGAP v1.2.0 and NAS v1.3.1. They are not pre-qualified for
complete Release 19 coverage. Audit actual implementation and tests against
the selected specification versions; record gaps accurately. Evaluate
maintained alternatives or standards-derived generation where useful. Do not
create scattered handwritten codecs in Host, Provider and Mock AMF to fill
gaps.

### 10.7 Ownership and qualification

| Layer | Responsibility | Migration candidate |
| --- | --- | --- |
| `ca-client-tools` | Own Codec ABI/header, module manifest, Codec Runtime/client, loader worker and independently versioned module artifacts | Replace scoped `protocol/nas5gsm` with generic module contract |
| `ca-agent-demo` | Select trusted profile and call private Codec Runtime; preserve original N1/N2 bytes and multipart metadata; no duplicate Python codec | `integrations/fivegc/nas5gsm.py`, NAS/N2 parsing in `nsmf_n1.py` |
| `env-5gc-sim` Mock AMF | Consume same module contract for NAS/NGAP/N2 operations; retain simulator state machine | `internal/mockamf/n2transfer/*` and duplicate NAS framing |
| `env-openamsm` | Keep native Owner implementation and provide independent interoperability vectors | Not a migration target |

Qualification gates:

1. Generate the exact itemized coverage inventory and compare it with actual
   encoder/decoder behavior. Full Release coverage is not required; truthful,
   reproducible coverage is.
2. Test every declared supported message/IE, and explicitly test unsupported
   behavior, extensions, unknown IEs, malformed lengths, duplicates and
   criticality handling.
3. Include standard/upstream vectors, semantic round-trips, fuzz tests and
   negative tests. Build success is not protocol qualification.
4. Keep real OpenAMSM / Mock AMF Create / Modify / Release interoperability as
   a separate E2E status, not a substitute for codec coverage.
5. Test Runtime/Host on native Windows amd64 and Linux amd64. Test each native
   library on its declared OS/architecture. Test macOS when a native runner
   is available.
6. Verify module provenance, signatures/checksums, ABI compatibility,
   crash/timeout behavior and safe upgrade/drain.

Report three independent statuses: declared standard coverage, codec
implementation qualification, and interoperability with a specific
OpenAMSM/RAN deployment. This design does not change the frozen CA-SL
Interface Package or Provider v2 ToolCatalog; protocol codec modules are
implementation dependencies, not Compiler-visible Tools.

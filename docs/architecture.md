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
- for NAS v1, every implemented 5GSM message and IE visible in the AMF-SMF
  N1 SM payload, with separate encode and decode status. 5GMM is outside the
  v1 AMF-SMF codec scope;
- constraints, extension / unknown-IE behavior and negative cases;
- test-vector, round-trip, fuzz and interoperability evidence;
- explicitly unsupported messages, IEs, options and known limitations.

Use explicit states such as `ENCODE_DECODE`, `DECODE_ONLY`,
`ENCODE_ONLY`, `NOT_IMPLEMENTED`, `UNVERIFIED`, and
`UNSUPPORTED_BY_DESIGN`. Summary percentages never replace the itemized
inventory. A message being unused by the current demo is not grounds to omit
it from the inventory.

### 10.2 Module artifact, embedded manifest, C ABI and JSON contract

The primary module artifact is a native shared library:

- Linux: `.so`
- Windows: `.dll`
- macOS: `.dylib`

The module boundary is a **versioned C ABI**, not Go's `plugin`
mechanism. Implementations may use Go, C, C++ or Rust without exposing
language-specific objects to the caller.

**Do not require a separate `codec.yaml` beside every library.** The full
codec capability manifest is embedded in the module and returned by an
exported query function, `ca_codec_get_manifest_json`. This keeps identity,
protocol version and exact coverage next to the implementation that owns
them. The Runtime queries it after loading the library and validates it
against the requested profile.

There is still a small external package/distribution index for locating an
artifact before it can be loaded. It contains only module ID/version,
OS/architecture, artifact location, dependencies, SHA-256 and package
signature. It is not a second copy of the detailed protocol coverage
manifest. Verify the artifact signature/checksum **before loading** the
library; an embedded manifest is descriptive metadata, not a trust anchor.

#### Shared header

Keep one shared `ca_codec.h` in the CA Codec SDK / ABI contract. It is
necessary as the canonical declaration of exported symbols, calling
convention, fixed-width types, status codes and buffer ownership for native
module authors and the loader. It is not a per-module metadata file and does
not describe NGAP/NAS message fields. Modules consume the same header; do
not fork or copy-paste it.

The ABI uses fixed-width integers, opaque instance handles and caller-owned
byte buffers. No Go structs/slices, C++ classes/exceptions, Rust-owned
objects, `size_t`-dependent wire fields or allocator-owned memory cross the
boundary. The v1 ABI exposes separate directional functions:

```c
uint32_t ca_codec_abi_version(void);

int32_t ca_codec_get_manifest_json(
    uint8_t *out_json,
    uint32_t *inout_json_len);

int32_t ca_codec_create(
    const uint8_t *config_json, uint32_t config_len,
    uint64_t *instance);

int32_t ca_codec_decode(
    uint64_t instance,
    const uint8_t *wire_data, uint32_t wire_length,
    const uint8_t *options_json, uint32_t options_length,
    uint8_t *message_json, uint32_t *inout_message_json_length);

int32_t ca_codec_encode(
    uint64_t instance,
    const uint8_t *message_json, uint32_t message_json_length,
    const uint8_t *options_json, uint32_t options_length,
    uint8_t *wire_data, uint32_t *inout_wire_length);

void ca_codec_destroy(uint64_t instance);
```

The data contract is intentionally directional:

- **DECODE:** input is the original binary protocol PDU / transfer payload;
  output is a typed UTF-8 JSON Protocol Message.
- **ENCODE:** input is a typed UTF-8 JSON Protocol Message; output is the
  original protocol's binary wire encoding.
- `options_json` carries non-payload context such as exact specification
  version, root type when the input is a standalone transfer, and codec
  options. It does not wrap or Base64-encode the wire bytes.
- The module identity and protocol are established by the module manifest;
  callers do not need to redundantly identify the module in every call.

For output buffers, the caller supplies memory. If the buffer is too small
(or NULL for a sizing query), the function returns `BUFFER_TOO_SMALL` and
reports the required length; the caller allocates and retries. The module
never transfers allocator ownership to the caller. ABI-level return codes
are reserved for loader/ABI/instance/buffer failures. Malformed wire data,
unsupported messages and semantic validation failures are returned as
structured diagnostics in the JSON result, not collapsed into a generic
process error. The ABI also defines calling convention, symbol visibility,
thread-safety, maximum request/response sizes and error semantics. ABI major
changes are incompatible; minor additions remain backward compatible.

#### Protocol message JSON and wire data

**Use UTF-8 JSON for structured messages and options; use raw binary for
protocol wire data.** Do not Base64-encode the protocol PDU at the C ABI.
NGAP/NAS wire bytes are passed directly as byte buffers.

The canonical JSON value model must be lossless for ASN.1 / NAS structures:
tagged CHOICE values; nested SEQUENCE objects; ordered SEQUENCE OF arrays;
integer and enumerated values (preserving numeric code and known name);
BIT STRING with both Base64 bytes and bit length; OCTET STRING as Base64;
and extension/open-type values with raw bytes where the standard and
implementation permit preservation. Optional/absent values must remain
distinct from explicit zero/empty values. Unknown fields must follow the
relevant 3GPP criticality and forwarding rules; JSON must not silently discard
them. This typed JSON schema is a versioned CA Protocol Message contract, not
an ad-hoc serialization of private Go structs.

For local IPC between the application and the resident Codec Host, preserve
the same directional semantics: metadata/options are UTF-8 JSON and the
wire payload is a separate length-delimited binary segment. Use a framed
transport with explicit metadata and payload lengths; do not convert the
binary payload to Base64 merely to put it inside JSON. Encode responses return
a binary payload segment; Decode requests carry a binary payload segment.
Request/response limits apply to both segments.

For NAS v1, the codec scope is the **SMF-visible N1 SM NAS payload carried
across the AMF-SMF SBI boundary** (the 5GSM message content). The AMF owns
UE-facing NAS security processing. The v1 NAS codec does not implement or
require a NAS Security Service, 5GMM security-header processing, integrity
protection, ciphering, deciphering, NAS security-context management or key
lifecycle. Those are outside this AMF-SMF protocol codec boundary. If input
is still a UE-AMF protected NAS envelope rather than the SMF-visible N1 SM
payload, v1 reports it as outside scope / unsupported and does not claim to
decode its protected inner message. Preserve the original N1 SM bytes and
SBI metadata.

A standalone NGAP PDU is decoded from APER. A standalone N2 SM Transfer
payload is decoded using its explicitly selected TS 38.413 transfer root
type; it is not misidentified as a complete NGAP PDU. NAS v1 accepts the
SMF-visible 5GSM N1 SM payload, not 5GMM NAS messages.

Do not log raw NAS, NGAP, SUPI or decoded subscriber-sensitive IEs by
default; diagnostics should redact payloads and sensitive values. A future
binary fast path is unnecessary for v1 because wire payloads already use
binary buffers. JSON remains the structured message representation.

A Go codec can be built as a C shared library with Go's
`-buildmode=c-shared`; this requires `CGO_ENABLED=1` and a suitable C
toolchain for that **module build**. Go documents C-shared build support for
Windows DLLs as well as Linux shared libraries. Windows Go 1.25+ CGO builds
also require a sufficiently recent GCC/binutils toolchain. See
[Go build modes](https://go.dev/cmd/go/#hdr-Build_modes) and
[Go minimum requirements](https://golang.google.cn/wiki/MinimumRequirements).
A pure-Go executable worker using the same directional JSON/binary contract
is an allowed artifact fallback when a shared library is not worthwhile.

### 10.3 Loading and process isolation

Do not load native codec libraries directly into the CA Agent or
long-lived Go Provider process. The default is one isolated, long-lived
`ca-codec-host` process containing the profile router, module manager and
native ABI loader:

```text
CA Host / Go Provider / Mock AMF
              |
       CodecClient (private API)
              |
        ca-codec-host
       +------+-------+
       |              |
   Profile Router   ABI Loader
                      |
                LoadLibrary / dlopen
                      |
                NGAP / NAS modules
```

The application-to-`ca-codec-host` IPC is long-lived and framed; **never
start a process per message**. The host loads C ABI modules and passes raw
wire bytes as a binary payload segment plus JSON options/message data. No
public network listener is required. A per-module worker process can be added
only when stronger crash isolation between independently sourced modules is
needed; it is not a mandatory extra layer for the first implementation.
A codec crash may restart `ca-codec-host`, but must not crash the Agent or
Provider; the client must detect disconnection and reconnect safely.

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

### 10.4 Embedded manifest and profile routing

Each module embeds its detailed manifest and exposes it through
`ca_codec_get_manifest_json`. Do not require a duplicate `codec.yaml`.
The embedded manifest contains:

- `moduleId`, `moduleVersion`, `codecAbiVersion`;
- protocol (`ngap` or `nas`);
- 3GPP TS, Release and exact specification version profiles;
- operations and itemized message/IE coverage;
- upstream provenance, commit and license;
- supported platforms and known limitations.

The external package index contains only pre-load distribution information:
artifact location, OS/architecture, dependencies, SHA-256 and package
signature. Verify this index and artifact before loading; the embedded
manifest is not a trust anchor.

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

1. Verify package signature/checksum, platform, dependencies and embedded
   manifest before loading.
2. Verify ABI compatibility and declared coverage.
3. Start a new `ca-codec-host` generation with the requested module set.
4. Run ABI handshake, health check and module conformance smoke tests.
5. Atomically route new requests to the new host generation.
6. Allow in-flight calls on the old host to finish, then terminate the old
   host process, releasing all its loaded libraries.

Do not rely on in-process `dlclose` / `FreeLibrary` for Go runtimes or arbitrary
native libraries. A failed upgrade leaves the old host/profile active.
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
| `ca-agent-demo` | Select trusted profile and call private Codec Runtime; NAS v1 covers the SMF-visible 5GSM N1 SM payload; preserve original N1/N2 bytes and multipart metadata; no duplicate Python codec | `integrations/fivegc/nas5gsm.py`, NAS/N2 parsing in `nsmf_n1.py` |
| `env-5gc-sim` Mock AMF | Consume the same module contract for supported NAS 5GSM / NGAP / N2 operations; retain simulator state machine | `internal/mockamf/n2transfer/*` and duplicate NAS framing |
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

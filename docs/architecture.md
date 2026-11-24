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

## 10. Free5GC-backed NGAP/NAS codec integration (2026-11-24)

**Status: accepted integration direction; implementation and protocol coverage are
not yet qualified.** Use maintained, reviewed upstream Go implementations as
the protocol engine. CA owns a small integration facade and the cross-language
worker contract, not a second NGAP/NAS implementation.

### 10.1 Architecture decision

Use these upstream pure-Go modules as the initial pinned candidates:

- [free5gc/ngap v1.2.0](https://github.com/free5gc/ngap/tree/v1.2.0)
  — source commit \`7c6125004f5fa914412a4a87de3d48f0ca2ab1c8\`.
- [free5gc/nas v1.3.1](https://github.com/free5gc/nas/tree/v1.3.1)
  — source commit \`ff5ea67383b7b5841247aab712889890b4c67d55\`.

Both declare Go 1.26.2, matching this repository's current Go toolchain.
Both are Apache-2.0 licensed; retain upstream license and copyright notices.
These are pinned integration candidates, not a claim of complete Release 19
support. Upgrade only after re-running the coverage and interoperability gates.

The target deliberately does **not** include:

- C ABI, \`ca_codec.h\`, CGO, native \`.so\` / \`.dll\` / \`.dylib\` modules;
- Go \`plugin\`, \`dlopen\`, \`LoadLibrary\`, dynamic module replacement or a
  native module package registry;
- a separate \`ca-codec-host\` product or a new public network service;
- handwritten APER / NAS wire codecs duplicated in Host, Mock AMF and Client
  Tools.

The Go module dependency is the implementation. Do not wrap it in a native
loader merely to create an additional versioning layer.

### 10.2 Ownership and call paths

\`ca-client-tools\` is the single owner of the CA-facing codec integration:

- Go consumers import the facade and call it in-process.
- Python consumers use one long-lived worker subcommand of the existing
  \`ca-client-tools\` executable. This is a private local IPC adapter, not a
  separate service or product.
- The facade depends directly on the pinned Free5GC modules. It does not copy
  upstream message structs or maintain a parallel protocol schema.
- CA-specific validation, explicit root-type selection, input-size bounds,
  error normalization and tested projections belong in the facade.
- Protocol parsing is not a business workflow and does not authorize an
  operation. Codec APIs are private implementation infrastructure, not
  Agent-callable Tools, Capability grants or ToolCatalog entries.

Consumers:

| Consumer | Required integration | Must remain consumer-owned |
| --- | --- | --- |
| \`ca-agent-demo\` | Python Host calls the resident Client Tools worker for NAS / NGAP decoding where needed; preserves original SBI N1/N2 bytes and Content-ID metadata | SBI transport, Host routing, Skill projection, Run lifecycle, Owner correlation and authorization |
| \`env-5gc-sim\` Mock AMF | Go code calls the Client Tools facade directly for NAS / NGAP / standalone N2 Transfer | Mock AMF state machine, scenario behavior, RAN/UPF resource projection and test controls |
| \`env-openamsm\` | Supplies independent real-protocol interoperability vectors and remains the real Owner implementation | No codec migration or business-logic rewrite |

Do not introduce a \`fivegc/\` namespace or an Open5GS compatibility layer.
Use the repository's existing NF / protocol naming.

### 10.3 Codec API boundary

Use the upstream APIs as the source of protocol types:

- 5GSM NAS: \`github.com/free5gc/nas/message.ParseGSM\`, returning the
  upstream \`GSMMessage\` interface and its message-specific types.
- Complete NGAP PDU: \`github.com/free5gc/ngap/message.ParseMessageType\`
  and the upstream message \`Write\` / Marshal APIs.
- Standalone N2 SM Transfer: \`github.com/free5gc/ngap/ie.UnmarshalBinary\`
  and \`ie.MarshalBinary\`, with the exact target transfer type selected by
  trusted SBI context such as \`n2SmInfoType\`.

The CA facade must provide a small stable entry point around these APIs. It
must not recreate every upstream message type or add a second generic ASN.1
model. For standalone N2 Transfer, the caller supplies an explicit root type;
never guess a transfer root from arbitrary bytes.

The facade must also define and test:

- maximum input and output sizes;
- exact-input-consumption / trailing-octet behavior;
- malformed length and invalid mandatory-field behavior;
- duplicate IE and unknown IE / criticality behavior;
- extension and open-type handling;
- consistent error categories without hiding the upstream cause.

A successful upstream decode is not proof that every IE was understood or
preserved. Where the upstream API reports unimplemented NAS IEs, unknown
NGAP IEs, or extension limitations, the facade must expose that result or
fail closed. It must never silently turn a partial decode into a complete
message.

### 10.4 Python worker contract

Expose the worker as a subcommand of the existing executable, for example:

\`\`\`text
ca-client-tools codec worker
\`\`\`

The Python Host starts it once during Host startup, reuses it for requests and
closes it during Host shutdown. Do not start one process per message. The
worker uses stdin/stdout only; no listening socket, HTTP endpoint or separate
daemon is introduced.

Use a bounded, versioned binary frame so protocol wire bytes are not wrapped
in JSON or Base64:

\`\`\`text
uint32_be frame_length
"CCW1"                 4-byte magic
uint16_be protocol_version
uint8     frame_kind
uint8     flags
uint32_be metadata_length
uint32_be payload_length
metadata_json          UTF-8 JSON bytes
payload                raw binary bytes
\`\`\`

The frame length must equal the exact number of bytes following the length
prefix. Reject unknown versions, frame kinds, non-zero reserved flags,
inconsistent lengths, oversized frames, truncated frames and trailing bytes.

- Decode request: metadata identifies request ID, protocol, operation and
  explicit N2 root type when applicable; payload contains the original raw
  NAS / NGAP bytes.
- Decode response: metadata contains the typed message representation and
  diagnostics; payload is empty.
- Encode request: metadata contains the typed message representation and
  explicit protocol / root type; payload is empty.
- Encode response: metadata contains status and diagnostics; payload contains
  raw protocol bytes.
- Error response: metadata contains a stable error code and safe diagnostic;
  do not echo subscriber-sensitive input or raw payloads.

The worker's typed JSON projection must be deterministic and versioned with
the facade. It must represent CHOICE selection, optional-field presence,
ordered arrays, integer / enum values and binary-valued ASN.1 fields without
loss. Do not hand-maintain a second per-message schema. Before claiming a
generic JSON Encode/Decode contract, prove the mapping with round-trip vectors
for every declared message. If a particular upstream type cannot be mapped
losslessly, expose a clearly named projection-only operation and preserve the
original wire bytes; do not claim generic re-encoding for that type.

A single worker may process requests serially. Add a bounded worker pool only
if measured concurrency requires it. Keep request IDs for correlation, enforce
timeouts and size limits, and ensure a worker crash cannot corrupt Host state.
The Go facade remains directly callable in-process by Go consumers.

### 10.5 Upstream coverage audit and known limitations

The initial source audit on 2026-11-24 found:

- NGAP \`ParseMessageType\` has explicit dispatch branches for 120
  PDU outcome / procedure combinations in the pinned source snapshot.
- NAS \`ParseGSM\` has explicit dispatch branches for 20 5GSM message types.
- These counts describe dispatch entries only. They do not establish complete
  per-IE support, correct extension handling, full Release coverage or
  successful encode/decode for every branch.
- NGAP APER currently contains TODO paths for extensible SEQUENCE handling;
  the decoder reads but discards the extension bit, while the encoder always writes false. The decoder also documents
  unsupported fragmentation for large \`SEQUENCE OF\` values.
- NGAP's unknown-IE helper can read the IE identity / criticality and skip an
  open value, but does not provide a lossless unknown-value round-trip
  contract.
- NAS message decoders can report recognized-but-unimplemented IEs through
  \`IEToDoList\`; an unknown IEI whose framing cannot be determined fails
  decoding.

The current 3GPP reference baselines remain TS 38.413 v19.4.0 and TS 24.501
v19.8.0. They are comparison baselines, not a full-R19 support claim. The
audit must map actual upstream message and IE behavior to the exact selected
OpenAMSM / Mock AMF use cases and to itemized 3GPP requirements.

The initial pinned-source inventory is recorded in
[free5gc-upstream-audit.md](free5gc-upstream-audit.md). It found 120 NGAP
message dispatch targets, 20 NAS 5GSM dispatch branches, 13 standalone
PDU Session Resource Transfer roots (six with dedicated test files), and
recognized-but-unimplemented NAS IE paths in all 15 PDU Session message
decoders. These are inventory counts and risk indicators, not coverage proof.
The initial verdict is REUSE CANDIDATE / SOURCE AUDIT PARTIAL / NOT QUALIFIED.


Do not patch around these limitations with new handwritten APER or NAS
parsers in each consumer. If a limitation blocks required coverage:

1. reproduce it with a minimal standard-derived vector;
2. identify whether the issue is upstream API usage, an upstream defect or
   genuinely missing support;
3. prefer an upstream fix / contribution or a narrowly pinned upstream fork;
4. otherwise evaluate a maintained alternative or standards-derived generator;
5. record the blocked coverage explicitly until verified.

### 10.6 Raw N1/N2 preservation and SBI multipart

The codec never replaces transport retention. Host ingress must retain the
exact original N1/N2 part bytes, Content-ID, Content-Type, digest and the
associated SBI JSON reference.

Multipart parsing must:

- remove only the MIME framing CRLF immediately before a boundary; never use
  unrestricted \`strip\` / \`rstrip\` on a binary payload;
- resolve \`n1SmMsg.contentId\` and \`n2SmInfo.contentId\` to the exact MIME
  part, rather than selecting the first part with a matching Content-Type;
- reject missing, duplicate or mismatched references and Content-IDs;
- preserve payload bytes byte-for-byte, including terminal \`0D\`, \`0A\` and
  \`0D 0A\` octets.

This is a prerequisite to codec migration and is tracked as a separate
\`ca-agent-demo\` change. Codec tests must consume the exact retained bytes.

### 10.7 Migration sequence

1. Fix SBI multipart byte preservation and Content-ID resolution in
   \`ca-agent-demo\`; add regression vectors before replacing any codec.
2. Complete a pinned upstream coverage audit for NAS, NGAP and each N2
   Transfer root used by the real Create / Modify / Release flows.
3. Add direct Free5GC dependencies and a thin, tested Go facade in
   \`ca-client-tools\`; keep the legacy \`protocol/nas5gsm\` API until all
   consumers migrate.
4. Add the long-lived stdio worker to the existing Client Tools executable
   and prove the framed request/response contract from Python.
5. Migrate \`env-5gc-sim\` to the Go facade; remove its handwritten APER
   reader only after equivalent or stronger tests pass.
6. Migrate \`ca-agent-demo\` to the worker; remove Python protocol parsers only
   after exact behavior and raw-byte retention are verified.
7. Delete the legacy scoped NAS codec and duplicate parsers after all
   consumers and cross-repository tests pass.
8. Run real OpenAMSM / Mock AMF Create, Modify and Release interoperability
   tests with captured or independently generated vectors.

### 10.8 Qualification and non-claims

Keep these statuses separate:

1. **Upstream source coverage** — exact message / IE / extension support in
   the pinned dependency.
2. **CA facade qualification** — adapter behavior, strictness, framing,
   round-trip, malformed input, fuzz and regression tests.
3. **Cross-language qualification** — Go direct API and Python worker produce
   equivalent projections and wire bytes.
4. **Live interoperability** — a named OpenAMSM / Mock AMF / RAN deployment
   passes the relevant procedure.

Required tests include upstream golden vectors, independent standard-derived
vectors, Encode→Decode→Encode where the upstream API supports it, exact raw
byte preservation, unknown / unsupported IE cases, extension cases,
trailing-octet rejection, fuzzing and real E2E Create / Modify / Release.

Do not claim full TS 38.413 / TS 24.501 Release 19 coverage from the pinned
Free5GC versions or from the number of dispatch branches. Do not change the
frozen CA-SL Interface Package, Provider v2 process contract or ToolCatalog
for this internal protocol integration.

The previous native dynamic-module / C ABI / \`ca-codec-host\` proposal is
superseded by this section.

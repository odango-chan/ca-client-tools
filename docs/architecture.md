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

## 10. Full-standard NGAP/NAS codec decision (2026-11-23)

**Status: accepted architecture target; implementation and full-standard
qualification are not yet complete.** The requirement is a reusable 3GPP
protocol implementation, not a codec limited to the messages currently used
by Create / Modify / Release or by today's OpenAMSM deployment.

### Normative coverage baseline

Use the current **3GPP Release 19** protocol baseline as of 2026-11-23:

- [3GPP TS 38.413 v19.4.0](https://www.etsi.org/deliver/etsi_ts/138400_138499/138413/19.04.00_60/ts_138413v190400p.pdf):
  NG Application Protocol (NGAP).
- [3GPP TS 24.501 v19.8.0](https://www.etsi.org/deliver/etsi_ts/124500_124599/124501/19.08.00_60/ts_124501v190800p.pdf):
  5GS Non-Access-Stratum (NAS) protocol.

This baseline is deliberately versioned. Later Release 19 corrections and
future 3GPP releases must be added as explicit, reviewed protocol profiles;
do not silently mix definitions from different releases. Keep a traceable
standard-version → upstream-code-version → implementation-coverage matrix.

**Full protocol codec coverage means:**

| Protocol | Required default coverage |
| --- | --- |
| NGAP, TS 38.413 | All NGAP-PDU alternatives (initiating message, successful outcome, unsuccessful outcome); every standardized procedure/message, IE, nested type, extension container and constraint in the pinned release; APER decode and encode. |
| NGAP N2 SM Transfer | All standardized N2 SM Transfer structures and their nested IEs, not only the PDU Session Setup / Modify / Release transfer currently used by the demo. |
| NAS 5GMM, TS 24.501 | All standardized 5GMM message types, mandatory/optional/conditional IEs, IE value formats and constraints in the pinned release. |
| NAS 5GSM, TS 24.501 | All standardized 5GSM message types, mandatory/optional/conditional IEs, IE value formats and constraints in the pinned release. |
| NAS envelope | All standardized EPD/security-header forms and correct framing. Ciphered/integrity-protected payloads must be represented without corruption; cryptographic algorithms and security-context/key lifecycle are a separately owned security service, not silently claimed by a message codec. |

The codec must not have a hand-maintained allowlist derived from current Skill
procedures or current OpenAMSM call paths. A message or IE being unused by the
demo is **not** grounds to omit it from the generic protocol layer.

This is a codec requirement, **not** a requirement to implement every network
function or protocol state machine. It does not make CA Client Tools an AMF,
SMF, gNB, NAS mobility/session manager, NGAP SCTP endpoint or general 5GC
stack. SBI transport, NGAP transport, business procedures, authorization and
Runtime state remain in their existing owners.

### Upstream implementation selection

First candidates are the pure-Go modules
[free5gc/ngap](https://github.com/free5gc/ngap) and
[free5gc/nas](https://github.com/free5gc/nas). They avoid the C `asn1c`
runtime and are suitable for Windows-native builds in principle. Candidate
versions visible on 2026-11-23 include NGAP v1.2.0 and NAS v1.3.1; these are
**candidate versions, not a declaration that they completely cover TS 38.413
v19.4.0 and TS 24.501 v19.8.0**.

Before adoption, compare their actual exported message/IE types, encode/decode
implementations, extension handling and tests against the complete normative
baseline above. A large generated source tree or successful import is not
proof of full coverage.

If either candidate has a gap:
1. Check another maintained, license-compatible implementation or a
   standards-derived generated codec.
2. Prefer extending/generating the common protocol definitions from the
   normative ASN.1 / message definitions, with reproducible generation and
   upstream provenance.
3. Do not shrink the CA protocol contract to the subset that the selected
   library happens to implement. Do not fill gaps with scattered hand-written
   message encoders in Host, Provider or Mock AMF.
4. If full coverage cannot yet be achieved, publish the exact gap matrix and
   mark full-standard coverage **NOT QUALIFIED**. Do not label a partial
   library integration as full NGAP/NAS support.

Pin exact module versions and source commits, preserve upstream license
notices, and keep the protocol version mapping explicit. The C
`oai-libngapcodec` / `asn1c` implementation may be evaluated as a reference
or fallback candidate if necessary, but it is not the default because the
selected Provider must remain Windows-native and `CGO_ENABLED=0`.

### API and ownership

The common protocol layer must expose generic message-level operations, not
only helpers such as `EncodeEstablishmentRequest`:

- decode a byte stream into the appropriate typed NGAP PDU or NAS message;
- encode any supported typed message into its standardized wire form;
- expose message/procedure/IE identity and typed fields without requiring
  consumers to know codec internals;
- preserve unknown or unrecognized extension/open-type data where the
  applicable standard's criticality and forwarding rules permit it;
- apply each protocol's specified handling for unknown, duplicate, malformed,
  out-of-range and unsupported fields. Do not globally reject every unknown IE
  or silently discard one that must be reported/rejected.

A thin CA adapter may provide ergonomic domain helpers, but it must be layered
on the complete generic codec. Such helpers may not narrow the protocol
surface, replace raw Owner-facing bytes by default, or invent business meaning.

| Layer | Target responsibility | Migration candidate |
| --- | --- | --- |
| `ca-client-tools` | Own/pin the common full-standard pure-Go protocol codec and stable generic Go API; provide Host-private access where needed | Replace scoped `protocol/nas5gsm` and add full NGAP/NAS API |
| `ca-agent-demo` | Preserve original N1/N2 bytes and multipart metadata; use the generic codec through an approved in-process/shared boundary or resident Provider; no second Python codec | `integrations/fivegc/nas5gsm.py`, NAS/N2-specific portions of `nsmf_n1.py` |
| `env-5gc-sim` Mock AMF | Use the same generic codec for any NAS/NGAP/N2 messages it constructs or inspects; retain simulator state machine and scenario behavior | `internal/mockamf/n2transfer/*` and duplicate NAS framing |
| `env-openamsm` | Keep the native Owner implementation; use as an independent interoperability oracle | Not a migration target |

Protocol codec APIs are not automatically Agent-callable Tools and do not
grant Capability authorization. They are shared protocol infrastructure.
If the Host must decode before Skill admission, use a Host-private interface
that does not create a circular bootstrap dependency or start a new process per
message. The Host must retain original bytes, Content-ID, Content-Type,
digest and Owner correlation; decoding is inspection/projection, not
authorization to rewrite the wire payload.

### Cross-platform contract

The codec and CA Client Tools binary must build and run natively on
**Windows amd64** and **Linux amd64** with `CGO_ENABLED=0`, without WSL,
MinGW, Visual Studio C build tools, `asn1c`, Linux headers or kernel modules.
Add native macOS tests when a runner is available. A cross-build is not a
substitute for a native test. The full OpenAMSM/UPF/RAN lab may remain on Linux
or remote hosts.

### Mandatory qualification and migration gates

1. **Complete standards inventory:** derive machine-checkable inventories of
   every NGAP procedure/message, PDU outcome, IE, nested type and extension in
   TS 38.413 v19.4.0, and every 5GMM/5GSM message and IE in TS 24.501 v19.8.0.
   Compare every entry with the selected upstream codec's exported types,
   encoder, decoder and tests. Record exact coverage and gaps; no sample-only
   or current-use-only inventory.
2. **Generic codec tests:** exercise every declared message type and IE
   family, required/optional/conditional fields, constraints, extension
   containers, malformed and unknown-field behavior. Include standards-based
   vectors, upstream vectors, property/round-trip tests and fuzzing. Test
   decode→encode semantically; only require byte identity where the standard
   or preserved raw open type requires it.
3. **Real interoperability:** verify the Create / Modify / Release flows and
   real OpenAMSM/Mock AMF vectors as a subset of the generic suite. These E2E
   cases do not replace the full protocol coverage inventory.
4. **Negative/security gates:** malformed APER/NAS, invalid lengths, duplicate
   IEs, invalid constraints, unknown IE criticality, extension handling,
   truncated protected NAS envelopes, payload-size abuse and identity
   mismatches must follow the relevant 3GPP rules and fail safely.
5. **Platform gates:** `CGO_ENABLED=0 go test ./...`, `go vet ./...`, native
   Windows build + codec suite and native Linux build + codec suite. Report
   macOS separately when unavailable.
6. **Cutover:** after full-standard API and parity gates pass, migrate Go
   Provider/Mock AMF consumers, then remove Python Host-specific codecs and
   old narrow local implementations. No permanent duplicate codec, per-flow
   message allowlist or compatibility switch. Preserve raw transport bytes
   and business/Runtime ownership.

Full-standard protocol coverage, implementation compatibility with a specific
OpenAMSM build, and real 5GC E2E qualification are three separate statuses.
Do not conflate them.

This decision changes neither the frozen CA-SL Interface Package nor the
[vNext Provider release package](tool-provider-release-package-vnext.md).
Protocol codecs are implementation dependencies, not Compiler-visible
ToolCatalog entries.

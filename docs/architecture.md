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

## 10. Pluggable NGAP/NAS codec modules (2026-11-23)

**Status: accepted architecture direction; module implementation and coverage
inventory are not yet complete.** CA requires reusable, generic protocol
codec capabilities whose exact coverage is declared per module. Full coverage
of an entire 3GPP Release is **not** a prerequisite. The current Create /
Modify / Release flows are consumers and must not define the maximum protocol
surface.

### Standards baseline and coverage declaration

Use 3GPP Release 19 as the current reference baseline:

- [3GPP TS 38.413](https://portal.etsi.org/webapp/WorkProgram/Report_WorkItem.asp?WKI_ID=79727):
  NG Application Protocol (NGAP), reference version v19.4.0.
- [3GPP TS 24.501](https://portal.etsi.org/webapp/WorkProgram/Report_WorkItem.asp?WKI_ID=79501):
  5GS Non-Access-Stratum (NAS), reference version v19.8.0.

A codec module may implement only a subset of a Release, or support multiple
Release profiles. That is acceptable **provided the module declares its exact
coverage and unsupported behavior**. Do not claim "full R19 support" based on
a handful of working messages. Prefer wording such as "TS 38.413 v19.4.0,
decode: messages A/B/C; encode: A/B; IE coverage: ...".

Coverage must be machine-readable and traceable at least to:

- protocol and exact 3GPP TS / version / Release baseline;
- codec module ID, module version and stable Codec ABI version;
- upstream project, exact version and source commit, plus license;
- each NGAP PDU outcome / procedure / message, and each implemented IE or
  nested type, with separate encode and decode status;
- each 5GMM / 5GSM message and IE, with separate encode and decode status;
- constraints, extension / unknown-IE behavior and relevant negative cases;
- test-vector, round-trip, fuzz and interoperability evidence;
- explicitly unsupported messages, IEs, options and known limitations.

Use explicit states such as `ENCODE_DECODE`, `DECODE_ONLY`,
`ENCODE_ONLY`, `NOT_IMPLEMENTED`, `UNVERIFIED`, and
`UNSUPPORTED_BY_DESIGN`. Aggregate percentages may be reported as a
summary, but never replace the itemized inventory. A message being unused by
the current demo is not a reason to omit it from the inventory.

### Pluggable module architecture

Separate the stable CA-facing Codec Runtime / ABI from each protocol codec
implementation:

```text
CA Host / ca-agent-demo / Mock AMF
              |
              v
     Protocol Codec Runtime
       stable ABI + router
              |
       persistent local RPC
              |
       +------+-------+--------+
       |              |        |
   NGAP R17        NGAP R19  NAS R19
   module          module    module
```

The Runtime owns module discovery, manifest validation, routing, lifecycle,
timeouts and diagnostics. Each module owns protocol-specific encode/decode
and reports its exact coverage. The module can be implemented in Go, C/C++ or
another suitable language; consumers must not depend on its implementation
language or private data structures.

The default cross-platform integration should use a **long-lived local codec
worker process with a versioned RPC contract**, not a new process per message.
Use a stable schema (for example Protobuf) and a local transport supported on
Windows, Linux and macOS. The Host and Provider can remain
`CGO_ENABLED=0`; the worker can use the toolchain appropriate to its
implementation. Keep module process startup separate from per-message calls.

A native shared library can be an optional platform-specific adapter:
Linux `.so`, Windows `.dll`, macOS `.dylib`. Do not make `.so` the
only artifact or the mandatory cross-platform ABI. In particular, Go's
`plugin` package is not suitable as the default: the Go documentation lists
support only for Linux, FreeBSD and macOS, and notes that plugins cannot be
closed and require tightly matched toolchains and dependencies
([Go plugin documentation](https://pkg.go.dev/plugin)). A Go `c-shared`
library also introduces a C ABI / cgo build boundary.

### Runtime selection and replacement

A module manifest (for example `codec.yaml`) declares its identity,
`codecAbiVersion`, protocol, 3GPP specification baseline, module version,
supported operations, coverage inventory, upstream provenance and artifact
platforms.

The Runtime configuration maps an explicit peer / deployment / network
profile to codec modules. It must support multiple protocol profiles
simultaneously, for example:

```yaml
protocolCodecs:
  profiles:
    ran-lab-r17:
      ngap: ngap-r17
      nas: nas-r17
    ran-lab-r19:
      ngap: ngap-r19
      nas: nas-r19
  peerProfiles:
    ran-lab-a: ran-lab-r17
    ran-lab-b: ran-lab-r19
```

This is illustrative configuration, not a committed schema.

Do **not** assume the 3GPP Release can always be inferred from an individual
NGAP or NAS packet. Release is a specification baseline, not a universal
per-message negotiation field. Select the profile from trusted deployment /
peer configuration or explicit session context. If no profile is known, fail
with a clear diagnostic or use an explicitly configured default; do not guess.

Replacing a module must not interrupt an in-flight decode / encode operation.
Install and verify the new module, validate its manifest and ABI, run health
and conformance smoke tests, atomically route new calls to it, then drain the
old worker. Keep protocol codec modules stateless with respect to UE/session
business state; NAS security context and key lifecycle remain separately
owned. Preserve original N1/N2 wire bytes and transport metadata regardless
of the selected decoder.

### Upstream implementation candidates

First candidates remain the pure-Go modules
[free5gc/ngap](https://github.com/free5gc/ngap) and
[free5gc/nas](https://github.com/free5gc/nas). Candidate tags inspected on
2026-11-23 include NGAP v1.2.0 and NAS v1.3.1. These are candidates only:
their actual Release 19 message / IE coverage must be measured, not assumed.
Evaluate maintained alternatives or standards-derived generation where
useful. Do not create scattered handwritten codecs in Host, Provider and
Mock AMF to fill gaps.

### Ownership

| Layer | Responsibility | Migration candidate |
| --- | --- | --- |
| `ca-client-tools` | Own the stable Codec ABI, module manifest contract, Runtime/router and reference worker; publish codec modules as independently versioned artifacts | Replace scoped `protocol/nas5gsm` with generic module contract |
| `ca-agent-demo` | Select configured profile and call the Runtime; preserve original N1/N2 bytes and multipart metadata; no duplicate Python codec | `integrations/fivegc/nas5gsm.py`, NAS/N2 parsing in `nsmf_n1.py` |
| `env-5gc-sim` Mock AMF | Call the same Codec Runtime/module contract for supported NAS/NGAP/N2 operations; retain simulator state machine | `internal/mockamf/n2transfer/*` and duplicate NAS framing |
| `env-openamsm` | Keep native Owner implementation and provide independent interoperability vectors | Not a migration target |

Codec APIs are implementation dependencies, not Agent-callable Tools. They
do not create Capability grants or Runtime authorization. If Host ingress
must decode before Skill admission, the Runtime must be Host-private and must
not create a circular dependency on an Agent-callable Provider Tool.

### Qualification gates

1. Generate the exact itemized coverage inventory from the selected TS
   versions and compare it with module declarations and actual encoder /
   decoder behavior. Full Release coverage is not required; truthful,
   reproducible coverage is.
2. Test every declared supported message / IE and explicitly test unsupported
   behavior. Include mandatory / optional / conditional fields, constraints,
   extensions, unknown IEs, malformed lengths, duplicates and criticality.
3. Include standard / upstream vectors, semantic round-trips, fuzz tests and
   negative tests. A successful build or import is not protocol qualification.
4. Keep real OpenAMSM / Mock AMF Create / Modify / Release interoperability as
   a separate E2E qualification dimension, not a substitute for coverage.
5. Test the Host / Runtime natively on Windows amd64 and Linux amd64 with
   `CGO_ENABLED=0`; add macOS when a native runner is available. Test each
   worker artifact on its declared target platform.
6. Verify module provenance, checksums/signatures and trusted installation.
   Native in-process modules execute with Host privileges; process isolation
   is preferred for independently replaceable modules.

Report three independent statuses: declared standard coverage, codec
implementation qualification, and interoperability with a particular
OpenAMSM / RAN deployment. Never conflate them.

This architecture does not change the frozen CA-SL Interface Package or the
[vNext Provider release package](tool-provider-release-package-vnext.md).
Protocol codec modules are implementation dependencies, not Compiler-visible
ToolCatalog entries.

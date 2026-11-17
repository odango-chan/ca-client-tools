# CA Client Tools

CA Client Tools is the repository for a reusable, NF/service-agnostic 3GPP SBI HTTP/2 client and Payload Engine, typed OpenAMSM adapters, a preinstallable Go Tool Provider backend, and a fixed local CLI.

The delivered CLI v1 surface is read-only. The broader SBI architecture supports
individually registered READ and MUTATION operations; this does not expose
arbitrary HTTP requests to an Agent. Provider runtime implementation is tracked
separately from the already merged transport and payload foundation.

The repository targets **OpenAMSM only**. It is not a generic 5GC provider
framework, not an Open5GS compatibility layer, and not the CA Runtime or
CA Agent Host.

## Platform targets and optional WSL tooling

The product targets **Windows-native, Linux-native, Docker, Kubernetes and
remote 5GC/NF deployments**. WSL is **not** a product dependency or a
deployment target: the Go client, shared SBI transport, Payload Engine and
CLI contain no WSL detection, no WSL-specific configuration field and no
WSL-specific network handling. Without WSL, an operator simply configures a
real remote SBI origin.

WSL appears only in the optional Windows-native acceptance tooling for this
laboratory. The acceptance-only forwarder is specific to WSL2 mirrored
networking, binds loopback only (`127.0.0.1`), exposes exactly the three
acceptance services (SMF SBI `17777`, AMF metrics `19090`, SMF metrics
`29090`), is not a general proxy, and is never imported by product code. See
[scripts/acceptance/README.md](scripts/acceptance/README.md).

## Delivered scope (CLI v1)

| Owner | Operation | Command |
|---|---|---|
| AMF | `GET /ue-info` | `ca-client-tools amf ue list` |
| AMF | `GET /gnb-info` | `ca-client-tools amf gnb list` |
| SMF | `GET /pdu-info` | `ca-client-tools smf pdu list` |
| SMF | Eight private OpenAMSM SMF context-query routes | `ca-client-tools smf context query --type <TYPE>` |

The eight SMF context types are `session`, `policy`, `up-node`,
`signaling-session`, `ip-address`, `up-selection`, `charging` and
`session-auth`. Each maps to one existing OpenAMSM v0.10 read-only SBI route.

The primary workflow needs no list lookup first. When the Agent already knows
the SUPI and PDU Session ID, it queries the target session context directly:

```bash
export CA_OPENAMSM_SMF_SBI_ORIGIN=http://127.0.0.1:7777
export CA_OPENAMSM_QUERY_TOKEN=...

echo '{
  "supi": "imsi-001010000000001",
  "pduSessionId": 5
}' | ca-client-tools smf context query --type session
```

Identity input is one JSON object on stdin. Supported locating keys are
SUPI + PSI, IMSI + PSI, IPv4, IPv6, and identity + PSI combined with IPv4
and/or IPv6 consistency checks. Subscriber identity is never passed as a
command-line argument. AMF UE List, AMF gNB List and SMF PDU List are
independent auxiliary reads; they are not prerequisites for a context query.

The client binds the response to the request:

- the echoed SUPI must match the requested SUPI (or `imsi-<IMSI>` when the
  request used the IMSI alias);
- the echoed `pduSessionId` must match the requested PSI;
- every requested UE IP lookup key must be echoed and match;
- a missing, extra or different key field is a Protocol Error, not a
  transport error and not `UNAVAILABLE`.

`AVAILABLE` returns the source context. `UNAVAILABLE` is a successful query
with an empty context. HTTP failures keep their status and safe
ProblemDetails `cause`. No context is inferred or fabricated.

## Responsibilities

- Provide the shared SBI transport and JSON/binary/multipart Payload Engine,
  typed OpenAMSM clients, and the fixed local CLI.
- Provide a trusted Provider Backend Binding Catalog and a preinstallable Go
  Provider backend for individually registered operations; the resident
  Provider runtime is not yet implemented.
- Keep service origins, credentials, HTTP methods, paths and payload bindings
  in trusted deployment/catalog/client configuration, never Skill-authored
  arguments.
- Keep Host policy, Runtime authorization, business workflow, Evidence and
  Procedure lifecycle outside this repository.

The shared h2c SBI transport and static token-header profile (PR #2, merged to
main as `189deff`) and the JSON/binary/multipart Payload Engine with its
Host-staged file boundary and acceptance tooling (PR #3, merged as
`4c424ee`) are implemented and validated on Windows-native and WSL/Linux.
They are independent of the CLI v1 read-only command surface. PR #1's typed SMF
client still has its own bounded h2c transport; migration to the shared
transport remains gated on exact regression tests and is required before
general 3GPP Operation Catalog onboarding. The PR #3 file primitives do not
implement the HostArtifactRef Store/API; that Host-owned contract remains a
draft in ca-agent-demo PR #211.

PR #5 is a Draft documentation PR. It freezes the first private SMF backend
binding set and Provider Process Protocol v1 for the initial JSON-only
Observation slice. The general 3GPP Operation Catalog loader and resident
Provider runtime are not implemented.

## Ownership boundaries

| Component | Owns |
|---|---|
| `env-openamsm` | OpenAMSM source snapshot, build, configuration and NF lifecycle |
| `ca-client-tools` | Shared SBI transport, Payload Engine, trusted Provider backend bindings, typed OpenAMSM clients and the fixed local CLI |
| `ca-agent` | Host integration, trusted configuration, Tool registration and exposure policy |
| `ca-runtime` | Callable admission, Capability authorization, execution, Outcome, Evidence and Reconcile |
| `ca-skills` | Business Procedures and declared callable dependencies |

Raw CLI stdout is Host-private because it may contain subscriber identifiers,
IP addresses and topology data. The Host must redact/project results before
returning them to an Agent, and must not treat the presence of a CLI command
as authorization.

## CLI contract

[docs/cli-contract.md](docs/cli-contract.md) is normative for the process
interface: command tree, stdin JSON, JSON stdout/stderr envelopes, exit codes,
cancellation and trusted environment configuration. The CLI is a fixed,
non-interactive local process interface. It is not an arbitrary shell and not
a generic raw HTTP tool.

The initial transport uses h2c (cleartext HTTP/2 prior knowledge), static
service endpoints and a trusted OpenAMSM query-token header profile. The
client does not implement TLS/mTLS or OAuth2; network protection and any
external transport termination belong to deployment infrastructure. The query
token is injected through trusted configuration, never Skill arguments.
The shared client's configuration is currently supplied in memory; no frozen
on-disk multi-service configuration schema or file loader is implemented.
Provider Process Protocol v1 uses only its allowlisted OpenAMSM environment
configuration. The YAML example in the architecture document is illustrative,
not a normative file format.

## Explicit exclusions

- No UPF `/session-info` client and no server patch in this repository.
- The only OpenAMSM change used for acceptance is a local `env-openamsm`
  build overlay for the query-token lookup; `upstream/openamsm` stays pristine.
- No TLS/mTLS, OAuth2 token acquisition/refresh or NRF discovery in the
  current h2c OpenAMSM baseline; deployment-specific profiles remain future
  work and are not prerequisites for the current client.
- CLI v1 has no mutation/write commands. This is a CLI-scope statement, not a
  restriction on the general SBI layer: an individual MUTATION operation may
  be added only through a trusted binding, explicit Host/Runtime authorization,
  source-backed request/response semantics and an operation-specific
  idempotency/reconciliation policy.
- No Open5GS-specific compatibility layer.
- No inferred SMF/UPF N4 SEID pair join.
- The general 3GPP Operation Catalog loader and resident Provider runtime are
  not implemented. PR #5 freezes only the first private SMF backend bindings
  and the JSON-only Provider Process Protocol v1 profile.
- No Agent-facing MCP server, arbitrary shell execution or Agent-visible
  generic curl/raw HTTP tool.
- No Provider Outcome, Runtime state or Evidence authority.

The generic SBI client architecture described in
[docs/sbi-client-architecture.md](docs/sbi-client-architecture.md) is normative
for the reusable transport, Operation/Provider binding model and Payload
Engine. The h2c transport and JSON/binary/multipart Payload Engine are
implemented; the general OpenAPI-derived operation catalog and resident
Provider runtime remain future implementation work.

## Implementation backlog

See [SBI Implementation Backlog](docs/sbi-implementation-backlog-20261117.md) for the checked cross-repository status and source-verified implementation queue, including the common READ/MUTATION execution model, binary/multipart Provider extension gate and local-only acceptance when Actions is unavailable.

## Public Callable IDs and implementation handoff

The canonical public Callable ID grammar is Host-owned in [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214). The first SMF public set is `smf.session.context.query` plus the eight `smf.mas.<projection>.context.query` Observation IDs. Private Provider backend IDs such as `openamsm.smf.mas.sc.query`, dependency aliases and deployment Capability IDs remain separate identities. See the [Public Callable ID Naming Contract v1](https://github.com/odango-chan/ca-agent-demo/blob/docs/v1alpha4-callable-runtime-contract-20261117/docs/design/public-callable-naming-contract-v1.md).

For the current cross-repository implementation order and the overnight Harness task, start at [docs/overnight-execution-goal-20261117.md](docs/overnight-execution-goal-20261117.md), then follow [the SBI backlog](docs/sbi-implementation-backlog-20261117.md). The first target is Provider + Host/Runtime diagnostic dispatch, not general curl or production Skill qualification.

## Provider integration (design only)

The resident Go Provider is a private backend process for the CA-Agent Host.
The public CA-SL Observation Owner is ca-agent-host; deployment owns
CapabilityContracts and Scope policies; SMF/OpenAMSM is the fact authority.
ca-client-tools owns the typed Go client and Provider backend operations only.

The Host-owned callable design is tracked in
[ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208),
and Host/Runtime v1alpha4 dispatch integration is tracked in
[PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).
This repository defines the private
[Provider Process Protocol v1](docs/provider-protocol-v1.md),
[Provider Backend Binding Catalog v1](docs/provider-backend-binding-catalog-v1.md)
and [OpenAMSM SMF Query Backend Contract v1](docs/smf-query-backend-contract-v1.md).
It does not publish ToolCatalog exports, Skill Schemas, Capability grants or
Evidence Descriptors.

The first backend slice supports the generic SC session read and eight
projection-specific Host adapters. Provider Process Protocol v1 is
intentionally JSON-only for this SMF Observation slice; that protocol boundary
does not limit the general SBI client or the already implemented Payload
Engine. Binary operations will use a separately versioned Provider payload
extension over Host-owned HostArtifactRef staging, streaming into the existing
JSON/binary/multipart Payload Engine. Mutation callables are outside this first
slice, but remain supported by the general architecture when individually
bound, source-verified and authorized. Source freshness is not claimed;
Evidence-dependent PASS remains NOT_QUALIFIED until the SMF temporal gate is
closed.
 
## Status

| Area | Status |
|---|---|
| AMF UE List, AMF gNB List, SMF PDU List | PASS (real OpenAMSM environment) |
| Eight SMF Context queries by SUPI/IMSI + PSI | PASS (real OpenAMSM, post-fix) |
| IPv4, IPv6 and joint lookup | PASS (real IPv4/joint; local IPv6 contract; real IPv6 session NOT_RUN) |
| Response identity binding | PASS (unit + compiled-CLI contract acceptance) |
| Go format/test/vet/build (Windows-native and WSL/Linux) | PASS — Go 1.26.6 Windows-native and Go 1.26.8 Linux/WSL on validation head `0b80d33` |
| Windows-native CLI → WSL-hosted real 5GC E2E | PASS — 23/23 strict JSON assertions; h2c prior-knowledge evidence captured separately |
| Shared h2c SBI transport (PR #2) and Payload Engine (PR #3) | PASS — implemented and Windows/Linux gates PASS; live operation NOT_RUN |
| GitHub Actions | CI_UNAVAILABLE — job never started (account billing/spending-limit annotation); not a CI PASS or a test failure |
| PR status | PR #2 merged (`189deff`); PR #3 merged (`4c424ee`); PR #5 remains Draft documentation-only |

A real OpenAMSM environment was built from the pinned snapshot and queried with
the compiled CLI. The AMF and SMF list surfaces and all eight SMF context
queries PASS. The initial context queries were blocked by a mixed-case token
header lookup over HTTP/2; after operator authorization, a build-time overlay
in the local `env-openamsm` checkout added the same lowercase lookup already
used by the orchestration handler. The upstream source tree remains unchanged
and `ca-client-tools` needed no code change for that defect. Details and
reproduction steps are in
[docs/evidence/real-openamsm-acceptance-20261117.md](docs/evidence/real-openamsm-acceptance-20261117.md).

A second acceptance run executed the compiled Windows `.exe` from Windows 11
against the same real 5GC running in WSL2 (mirrored networking, using the
optional acceptance-only loopback forwarder that exposes only the SMF SBI and
AMF/SMF metrics services). The final round re-ran the same 23 cases with the
runner asserting exact JSON fields: status and type plus echoed identity keys
and list membership for positive cases; HTTP status, ProblemDetails `cause`
and exit code for negative cases; and no-request semantics for local
rejections. It also confirmed the Windows-native build and the HTTP/2
prior-knowledge preface with no HTTP/1.1 fallback. See
[docs/evidence/windows-wsl-acceptance-20261117.md](docs/evidence/windows-wsl-acceptance-20261117.md).

Local acceptance uses the compiled CLI against a fixture that reproduces the
pinned OpenAMSM v0.10 handler semantics. Real acceptance now covers the lists
and all eight context queries; the fixture remains the reproducible contract
test for identity mismatch and error paths.

See [docs/WORK.md](docs/WORK.md), [docs/TRACEABILITY.md](docs/TRACEABILITY.md),
[docs/openamsm-interface-inventory.md](docs/openamsm-interface-inventory.md)
and [docs/DEEPSEEK-HANDOFF.md](docs/DEEPSEEK-HANDOFF.md) for evidence.

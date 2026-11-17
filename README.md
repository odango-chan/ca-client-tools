# CA Client Tools

CA Client Tools is the read-only client and local command-line integration
layer for the OpenAMSM environment used by CA-Agent.

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

- Provide the typed OpenAMSM read clients and the fixed local CLI.
- Use cleartext HTTP/2 prior knowledge (h2c) for the SMF SBI listener and the
  existing `X-OpenAMSM-Query-Token` header profile.
- Keep transport details, endpoint configuration and authentication outside
  Skill-authored arguments.
- Keep Runtime authorization, Host policy, business workflow and Evidence
  ownership outside this repository.

The shared h2c SBI transport and static token-header profile (PR #2, merged to
main) and the JSON/binary/multipart Payload Engine with its Host-staged file
boundary and acceptance tooling (PR #3, the final v1 slice) are implemented and
validated on Windows-native and WSL/Linux. They are not part of PR #1 and are
not needed by the CLI v1 read-only surface. PR #1's typed SMF client keeps its
own small h2c transport for the real SBI listener; its migration to the shared
transport remains explicitly gated. The Operation Catalog and Tool Provider are
not yet implemented.

## Ownership boundaries

| Component | Owns |
|---|---|
| `env-openamsm` | OpenAMSM source snapshot, build, configuration and NF lifecycle |
| `ca-client-tools` | Typed OpenAMSM read clients and the fixed local CLI |
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

## Explicit exclusions

- No UPF `/session-info` client and no server patch in this repository.
- The only OpenAMSM change used for acceptance is a local `env-openamsm`
  build overlay for the query-token lookup; `upstream/openamsm` stays pristine.
- No TLS, mTLS, OAuth2, token refresh or NRF discovery requirement for CLI v1.
- No mutation or write operations.
- No Open5GS-specific compatibility layer.
- No inferred SMF/UPF N4 SEID pair join.
- No generic Operation Catalog or Tool Provider is implemented in the v1
  slices.
- No Agent-facing MCP server or arbitrary shell execution.
- No Provider Outcome, Runtime state or Evidence authority.

The generic SBI client architecture described in
[docs/sbi-client-architecture.md](docs/sbi-client-architecture.md) is the
normative design for the shared transport and Payload Engine, which are
implemented. Operation Catalog and Tool Provider work remains future work
outside CLI v1 and the PR #3 v1 slice.

## Provider integration (design only)

The Go Provider is a separate, preinstalled, long-lived process. The field-level
[Provider Process Protocol v1](docs/provider-protocol-v1.md) and the eight
[SMF Context Observation contracts](docs/smf-context-observation-contract-v1.md)
are frozen for implementation. The machine-readable ToolCatalog, QueryContext
Schema, eight Output Schemas and Evidence Descriptors are under
`contracts/smf-context/v1alpha4/`. The single immutable Provider binding
catalog is `contracts/provider-bindings/v1/provider-bindings.yaml`.

The Host/Runtime v1alpha4 integration contract is tracked in
[ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).
These are documentation contracts only: Provider process, Host Observation
stage and real Host-dispatch E2E are not implemented or qualified. The first
slice is JSON-only and does not depend on HostArtifactRef or NAS/NGAP work.

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
| PR status | PR #2 merged to main (`189deff`); PR #3 is the final v1 slice, merged only after the gates in the evidence record |

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

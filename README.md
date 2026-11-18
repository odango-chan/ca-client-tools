# CA Client Tools

CA Client Tools is the OpenAMSM client and Tool Provider integration layer for
CA-Agent. It provides the reusable SBI transport, a trusted Operation Catalog, a
resident Go Tool Provider process, typed OpenAMSM read clients and a stable local
CLI.

The repository targets **OpenAMSM only**. It is not a generic 5GC provider
framework, not an Open5GS compatibility layer, and not the CA Runtime or
CA Agent Host.

## Provider architecture

The internal demo has exactly one resident Provider protocol (`ca-client-tools.provider/v2`) and one trusted Catalog, with no backward-compatibility requirement. Launch through `ca-client-tools provider serve`. The eleven typed OpenAMSM read bindings are preserved, Generic SBI is connected with recursively validated nested JSON arguments, and dispatch uses the shared trusted transport and Payload Engine. The legacy v1 Provider protocol and v1 Catalog were removed; a legacy frame is rejected. Host-owned artifact references and streaming depend on the published cross-repository Host contract and remain disabled. Production Mutation stays gated.

Start with [AGENTS](AGENTS.md) for the required reading order, [DeepSeek handoff](docs/DEEPSEEK-HANDOFF.md) for execution, and [WORK](docs/WORK.md) for implementation status and gates.

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
| SMF | Eight MAS QueryContext routes | `ca-client-tools smf context query --type <TYPE>` |

The eight SMF context types are `session`, `policy`, `up-node`,
`signaling-session`, `ip-address`, `up-selection`, `charging` and
`session-auth`. Each maps to one existing OpenAMSM v0.10 read-only SBI route.

The same eleven tools are also reachable through the resident Provider machine
interface; the CLI v1 command tree, help surface and JSON stdin/stdout/stderr
contract are unchanged by the Provider addition.

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

See [SBI client architecture](docs/sbi-client-architecture.md) for the transport and Payload Engine.

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
- No Agent-facing MCP server or arbitrary shell execution.
- No Provider Outcome, Runtime state or Evidence authority.

Production access remains read-only under the gate in [WORK](docs/WORK.md).

## Status

See [WORK](docs/WORK.md) for the inspected PR #7 head, current implementation and pending work.

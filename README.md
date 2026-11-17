# CA Client Tools

CA Client Tools is the read-only client and local command-line integration
layer for the OpenAMSM environment used by CA-Agent.

The repository targets **OpenAMSM only**. It is not a generic 5GC provider
framework, not an Open5GS compatibility layer, and not the CA Runtime or
CA Agent Host.

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

The shared h2c SBI transport, static token-header profile and the payload
foundation are implemented on stacked PRs #2 and #3. They are not part of
PR #1 and are not needed by the CLI v1 read-only surface. PR #1's typed SMF
client has its own small h2c transport for the real SBI listener.

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

- No UPF `/session-info` client and no OpenAMSM server patch.
- No TLS, mTLS, OAuth2, token refresh or NRF discovery requirement for CLI v1.
- No mutation or write operations.
- No Open5GS-specific compatibility layer.
- No inferred SMF/UPF N4 SEID pair join.
- No generic Operation Catalog or Tool Provider in this PR.
- No Agent-facing MCP server or arbitrary shell execution.
- No Provider Outcome, Runtime state or Evidence authority.

The generic SBI client architecture described in
[docs/sbi-client-architecture.md](docs/sbi-client-architecture.md) is a
separate workstream. Operation Catalog and Tool Provider work is not part of
CLI v1 or PR #1.

## Status

| Area | Status |
|---|---|
| AMF UE List, AMF gNB List, SMF PDU List | PASS (real OpenAMSM environment) |
| Eight SMF Context queries by SUPI/IMSI + PSI | BLOCKED on real NF; PASS on local contract fixture |
| IPv4, IPv6 and joint lookup | PASS (local contract fixture) |
| Response identity binding | PASS (unit + compiled-CLI contract acceptance) |
| Go format/test/vet/build | PASS (Go 1.26.8) |
| GitHub Actions | CI_UNAVAILABLE |
| Real OpenAMSM end-to-end acceptance | PARTIAL — lists PASS, contexts BLOCKED |

A real OpenAMSM environment was built from the pinned snapshot and queried with
the compiled CLI. The AMF and SMF list surfaces worked. The eight SMF context
paths are **BLOCKED** by an OpenAMSM defect: the handler looks up the token
header with a mixed-case name while HTTP/2 requires lowercase field names, so
every compliant client receives HTTP 403 `FORBIDDEN`. No server patch was
applied. Details and reproduction steps are in
[docs/evidence/real-openamsm-acceptance-20261117.md](docs/evidence/real-openamsm-acceptance-20261117.md).

Local acceptance uses the compiled CLI against a fixture that reproduces the
pinned OpenAMSM v0.10 handler semantics. It proves client/CLI behavior only and
is never substituted for the blocked real result.

See [docs/WORK.md](docs/WORK.md), [docs/TRACEABILITY.md](docs/TRACEABILITY.md),
[docs/openamsm-interface-inventory.md](docs/openamsm-interface-inventory.md)
and [docs/DEEPSEEK-HANDOFF.md](docs/DEEPSEEK-HANDOFF.md) for evidence.

# CA Client Tools

CA Client Tools is the local client and command-line integration layer for the
OpenAMSM environment.

The repository targets **OpenAMSM only**. It is not a generic 5GC provider
framework and does not maintain Open5GS compatibility.

## Responsibilities

- Call the externally exposed OpenAMSM interfaces.
- Provide typed, bounded client operations and a stable CLI entry point.
- Keep transport details, endpoint configuration and authentication outside
  Skill-authored arguments.
- Return structured results for integration by the CA Agent Host.

## Ownership boundaries

| Component | Owns |
|---|---|
| `env-openamsm` | OpenAMSM source snapshot, build, configuration and NF lifecycle |
| `ca-client-tools` | OpenAMSM client implementations and CLI |
| `ca-agent` | Host integration, default-tool registration and Agent exposure policy |
| `ca-runtime` | Callable admission, Capability authorization, execution, Outcome, Evidence and Reconcile |
| `ca-skills` | Business Procedure and declared callable dependencies |

The CLI is a local invocation surface. It does not replace the network protocol
exposed by OpenAMSM: for example, SMF MAS Query remains an HTTP/SBI request from
the CLI process to the SMF.

## Initial OpenAMSM interface inventory

The current static source findings identify:

- AMF metrics: `GET /ue-info` and `GET /gnb-info`.
- SMF metrics: `GET /pdu-info`.
- SMF MAS Query: eight read-only SBI endpoints for SC, PA, UPN, SSM, IAM, UPS,
  CA and SAA.
- No UPF `/session-info` endpoint in the inspected OpenAMSM source.

The SMF MAS Query wire contract is the implemented OpenAMSM v0.10 contract:
`X-OpenAMSM-Query-Token`, typed query keys, `AVAILABLE` /
`UNAVAILABLE`, and ProblemDetails errors.

## Explicit exclusions

- No Open5GS-specific patch compatibility layer.
- No synthetic UPF `/session-info` client.
- No inference of an SMF/UPF N4 SEID pair from the single UPN `n4Seid` value.
- No Provider Outcome, Predicate verdict, Runtime Run state, or Evidence
  authority.
- No Agent-facing MCP server and no arbitrary shell execution.

The OpenAMSM source archive has not yet been imported into
`env-openamsm/upstream/openamsm`; current endpoint inventory is a static
source-inspection baseline and is not a build or live-acceptance claim.

## Status

Repository bootstrap only. Implementation and acceptance are not yet started.

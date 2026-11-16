# CA Client Tools

CA Client Tools is the local client and command-line integration layer for the
OpenAMSM environment.

The repository targets **OpenAMSM only**. It is not a generic 5GC provider
framework and does not maintain Open5GS compatibility.

## Responsibilities

- Call the externally exposed OpenAMSM interfaces.
- Provide typed SMF context-query operations, bounded source-native metrics page clients and a stable CLI entry point.
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
exposed by OpenAMSM: for example, SMF context query remains an HTTP/SBI request from
the CLI process to the SMF.

## NF capability and interface map

See [docs/nf-capability-map.md](docs/nf-capability-map.md) for the current AMF/SMF/UPF capability map, including implemented, unavailable and future integration surfaces.

## Initial OpenAMSM interface inventory

The current static source findings identify:

- AMF metrics: `GET /ue-info` and `GET /gnb-info`.
- SMF metrics: `GET /pdu-info`.
- SMF context query: eight read-only OpenAMSM SBI endpoints exposed through stable CA context types: `session`, `policy`, `up-node`, `signaling-session`, `ip-address`, `up-selection`, `charging` and `session-auth`.
- No UPF `/session-info` endpoint in the inspected OpenAMSM source.

The private OpenAMSM adapter follows its v0.10 wire contract:
`X-OpenAMSM-Query-Token`, typed query keys, `AVAILABLE` /
`UNAVAILABLE`, and ProblemDetails errors.

## Explicit exclusions

- No Open5GS-specific patch compatibility layer.
- No synthetic UPF `/session-info` client.
- No inference of an SMF/UPF N4 SEID pair from the single UPN `n4Seid` value.
- No Provider Outcome, Predicate verdict, Runtime Run state, or Evidence
  authority.
- No Agent-facing MCP server and no arbitrary shell execution.

The authoritative OpenAMSM snapshot is imported in `env-openamsm/main` under
`upstream/openamsm/`: archive `openamsm-personal-z00638432-vngroup1111.zip`,
archive SHA-256 `6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
source commit comment `c27e1761`, 11,904 files. The source is statically
inspected; native build, context-query source tests and live-NF acceptance remain NOT_RUN.

## CLI v1 implementation (CI acceptance pending)

The normative CLI design is in [docs/cli-contract.md](docs/cli-contract.md).
The executable implementation handoff is
[docs/DEEPSEEK-HANDOFF.md](docs/DEEPSEEK-HANDOFF.md); it scopes the current
work to CLI v1 and explicitly excludes NW Agent planning, facade, orchestration
and relay endpoints.
The selected framework is Cobra, confined to `internal/cli`; OpenAMSM client
packages remain independent of CLI libraries.

The resource/action command tree is:

```bash
# Host-internal metrics pages
CA_OPENAMSM_AMF_METRICS_ORIGIN=http://amf:9090 ./ca-client-tools amf ue list --page 0
CA_OPENAMSM_AMF_METRICS_ORIGIN=http://amf:9090 ./ca-client-tools amf gnb list --page 0
CA_OPENAMSM_SMF_METRICS_ORIGIN=http://smf:9090 ./ca-client-tools smf pdu list --page 0

 # SMF context Observation candidate; subscriber lookup values are supplied on stdin
CA_OPENAMSM_SMF_SBI_ORIGIN=http://smf:7777 \
CA_OPENAMSM_QUERY_TOKEN="$OPENAMSM_QUERY_TOKEN" \
  ./ca-client-tools smf context query --type up-node <<'JSON'
{
  "supi": "imsi-001010000000001",
  "pduSessionId": 5
}
JSON
```

SMF context input is one bounded JSON object on stdin; SUPI, IMSI and UE IP keys must
never be process arguments. Endpoint origins and the query token are trusted
environment configuration. Metrics use zero-based `--page` and `--page-size`
(1–100); each invocation returns one page and the Host controls traversal.

CLI v1 writes one `{"data": ...}` JSON document to stdout on success, or one
`{"error": ...}` JSON document to stderr on failure. Exit codes are stable and
classified by argument, configuration, remote/transport and protocol/internal
failure. Help and version output are the only human-readable stdout exceptions.
An SMF context `status=UNAVAILABLE` response remains a successful result (exit 0). HTTP 404 `SESSION_NOT_FOUND` remains a CLI error; only the separate CA Agent Host generic session-context Observation adapter may normalize that exact response to a typed `ABSENT` business result. The CLI never fabricates a successful absence payload.

**Implementation status:** the review branch now exposes the CLI v1 resource/action tree, including `smf context query --type <TYPE>`. Public output uses stable CA context types; OpenAMSM's internal route identifiers remain inside the adapter.

## Status

- SMF context-query and AMF/SMF metrics clients: implemented on the current review branch with fixture tests. Latest local test/vet/build has not yet been rerun. The declared Go 1.26.2 toolchain and GitHub Actions remain NOT_VERIFIED (jobs exposed no step records or logs).
- CLI implementation: the Cobra-based v1 command tree, strict JSON stdin, output envelopes, stable exit mapping and cancellation handling are implemented. The latest CI run failed; a compile-time `emitData` call-site mismatch has been corrected, and a fresh Go 1.26.2 CI pass is required before acceptance.
- AMF `/ue-info`, AMF `/gnb-info` and SMF `/pdu-info`: bounded, paginated
  source-native page clients implemented on the current review branch. Item JSON
  is preserved rather than converted into Provider or Skill business semantics.
- UPF session inspection and exact N4 pair joins: not currently exposed by OpenAMSM; excluded.
- OpenAMSM build and live-NF acceptance: not run.

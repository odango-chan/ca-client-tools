# CA Client Tools repository rules

## Project identity

This repository contains local client implementations and a stable CLI for
**OpenAMSM**. It is not a generic 5GC adapter framework and does not support
Open5GS as a second backend.

Do not create a `fivegc/` package or directory. OpenAMSM is the selected
upstream environment and source of interface behavior.

## Required ownership boundaries

- `env-openamsm` owns the OpenAMSM source snapshot, build, configuration,
  deployment and NF process lifecycle.
- `ca-client-tools` owns typed OpenAMSM client calls and the local CLI.
- `ca-agent` owns Host integration, default-tool registration, configuration
  injection and the policy that distinguishes Agent-callable tools from
  Host-internal tools.
- `ca-runtime` owns callable admission, Capability authorization, Runtime
  execution identity, Procedure progression, Outcome, Evidence and Reconcile.
- `ca-skills` owns business Procedures and declared callable dependencies.

The CLI is a fixed local process interface, not an arbitrary shell. A Host
adapter must invoke typed commands with a fixed executable and validated
arguments. Endpoint configuration and credentials are trusted deployment
configuration, never Skill-authored arguments. Implement the CLI only against
`docs/cli-contract.md`: Cobra at the CLI layer, JSON stdin for sensitive MAS
lookup data, JSON success on stdout, JSON errors on stderr, and the documented
exit-code mapping. Do not treat the current flag-based CLI as the final style.

The CLI does not replace the remote protocol. A local CLI invocation of an
SMF MAS query still sends the documented HTTP/SBI request to OpenAMSM SMF.
Cobra must not be imported by `internal/openamsm/**`; keep the CLI process
boundary thin and deterministic.

## OpenAMSM source and interface evidence

Read these before changing client behavior:

- `docs/architecture.md`
- `docs/cli-contract.md` (normative CLI v1 contract)
- `docs/openamsm-interface-inventory.md`
- `env-openamsm/docs/openamsm-source-findings.md`
- the implemented SMF MAS QueryContext SBI v0.10 contract
  (`SMF-MAS查询上下文SBI接口设计说明书.md`)

The authoritative OpenAMSM source archive is imported under
`env-openamsm/upstream/openamsm` on `env-openamsm/main` (archive SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`, source
commit comment `c27e1761ae09957a474cc2b0208d227cde64aabf`, 11,904 files). The
inventory is source-inspected; native build, source unit tests and live-NF
acceptance are not yet claimed.

Do not infer an endpoint merely because Open5GS or another NF exposes it.
In particular:

- OpenAMSM has AMF `/ue-info` and `/gnb-info`, SMF `/pdu-info`, and eight
  SMF MAS Query SBI paths. Metrics responses use `items` plus a `pager` object;
  page indexes are zero-based and page size is capped at 100. The metrics
  listener is plain HTTP and these custom handlers do not apply application
  authentication, so origins must be trusted and network-restricted.
- The inspected OpenAMSM source does not expose UPF `/session-info`.
- OpenAMSM SMF `/pdu-info` does not expose the SMF/UPF N4 SEID pair.
- UPN MAS `n4Seid` is a single UPF SEID and is not an N4 pair.

Do not port the Open5GS SMF N4-pair or UPF session-info patches into this
repository. Do not recreate those endpoints in OpenAMSM as a client workaround.
Do not join SMF and UPF sessions without an exact authoritative pair.

## Client behavior

- Use explicit deployment-configured endpoint origins.
- Reject credentials, query strings, fragments and unexpected base paths in
  configured origins.
- Disable ambient proxy inheritance and redirects unless a reviewed contract
  explicitly requires otherwise.
- Apply bounded timeouts and response sizes.
- Never log query tokens, SUPI/IMSI, raw subscriber responses or raw error
  bodies.
- Preserve source status and error semantics. In particular,
  `AVAILABLE` and `UNAVAILABLE` are distinct successful SMF MAS responses;
  `UNAVAILABLE` must not be converted to a negative business fact.
- Do not retry mutations. The initial scope is read-only client operations.
- Do not create Runtime Evidence, Predicate verdicts, business lifecycle
  records, journals or reconciliation state here.

## Tool visibility

This repository implements client operations; it does not decide who may call
them.

- SMF MAS Query operations are candidates for explicitly declared
  Agent-callable Tools.
- AMF `/ue-info`, AMF `/gnb-info` and SMF `/pdu-info` may be registered as Host
  defaults but remain Host-internal unless the Host/Runtime contract explicitly
  exposes them. Their source payloads may contain SUPI, SUCI, PEI, location and
  network topology data; never pass raw metrics pages directly to the Agent.
- Tool visibility and Capability authorization must not be inferred from a
  CLI subcommand's existence.
- The SMF MAS request accepts SUPI and/or IMSI, but the OpenAMSM v0.10 response
  echoes the resolved SUPI when available, `pduSessionId` and supplied IP keys;
  IMSI is not a response-key property. Host adapters must verify that echo
  against the trusted lookup binding, normalize IMSI bindings to SUPI for
  comparison, then redact/replace subscriber identifiers before returning an
  Agent-visible Observation. Raw CLI stdout is Host-private and must not be
  passed directly to the Agent.

## Reuse from ca-provider-5gc

Do not copy the Provider HTTP server, Runtime ObservationSource, Runtime
correlation/evidence handling, CreatedSession workflow state, PCF/UDM
orchestration, Fabric journals or mutation adapters wholesale.

A low-level reader/parser may be reused only after separating transport and
source parsing from Provider/Runtime semantics and verifying its exact
compatibility with OpenAMSM.

## Delivery and validation

- Keep implementation small and OpenAMSM-specific.
- Add focused tests for every supported endpoint and error/status distinction.
- Run `gofmt`, `go test ./...`, `go vet ./...`, and a native build.
- Mock HTTP tests prove client contract behavior only. They do not prove
  OpenAMSM source compatibility or live-NF acceptance.
- Never claim source build, live query or end-to-end acceptance without
  evidence from the actual OpenAMSM environment.

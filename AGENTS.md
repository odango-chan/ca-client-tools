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
`docs/cli-contract.md`: stable NF/resource command tree and Cobra at the CLI layer, JSON stdin for sensitive SMF context lookup data, JSON success on stdout, JSON errors on stderr, and the documented
exit-code mapping. The CLI v1 command tree is implemented on the current
review branch; do not reintroduce the pre-v1 flag-only interface.

The CLI does not replace the remote protocol. A local CLI invocation of an
SMF context query still sends the documented HTTP/SBI request to OpenAMSM SMF.
Cobra must not be imported by `internal/openamsm/**`; keep the CLI process
boundary thin and deterministic.

## OpenAMSM source and interface evidence

Before changing or adding any client behavior, read these documents in order:

1. `docs/nf-capability-map.md` — canonical NF responsibilities, CA ownership,
   3GPP → Tool → Skill derivation, client scope and evidence states.
2. `docs/openamsm-interface-inventory.md` — source-backed endpoint and wire
   contracts for the pinned OpenAMSM snapshot.
3. `docs/architecture.md` — repository boundaries and Agent/Host exposure.
4. `docs/cli-contract.md` — normative local CLI v1 process contract.
5. The pinned OpenAMSM source findings and imported source snapshot linked
   below.
6. The OpenAMSM v0.10 SMF context-query OpenAPI contract and its source tests
   (the exact paths and repository commit are recorded in
   `docs/openamsm-interface-inventory.md`).

The authoritative OpenAMSM source archive is imported under
`env-openamsm/upstream/openamsm` on `env-openamsm/main` (archive SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`, source
commit comment `c27e1761ae09957a474cc2b0208d227cde64aabf`, 11,904 files). The corresponding `env-openamsm` repository snapshot is pinned at
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb). The source findings are at
[docs/openamsm-source-findings.md](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md);
the OpenAMSM context-query wire contract is
[Nsmf_MasContextQuery.yaml](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml);
and source tests are under
[tests/smf-mas-query/](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tests/smf-mas-query/).
The inventory is source-inspected; native build, source unit tests and live-NF
acceptance are not yet claimed.

Do not infer an endpoint merely because Open5GS or another NF exposes it. The public command vocabulary is NF/resource/capability based; OpenAMSM implementation identifiers remain private to the adapter.
In particular:

- OpenAMSM has AMF `/ue-info` and `/gnb-info`, SMF `/pdu-info`, and eight
  SMF context-query SBI paths. Metrics responses use `items` plus a `pager` object;
  page indexes are zero-based and page size is capped at 100. The metrics
  listener is plain HTTP and these custom handlers do not apply application
  authentication, so origins must be trusted and network-restricted.
- The inspected OpenAMSM source does not expose UPF `/session-info`.
- OpenAMSM SMF `/pdu-info` does not expose the SMF/UPF N4 SEID pair.
- UPN context `n4Seid` is a single UPF SEID and is not an N4 pair.

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
  `AVAILABLE` and `UNAVAILABLE` are distinct successful SMF context responses;
  `UNAVAILABLE` must not be converted to a negative business fact.
- Do not retry mutations. The initial scope is read-only client operations.
- Do not create Runtime Evidence, Predicate verdicts, business lifecycle
  records, journals or reconciliation state here.

## Tool visibility

This repository implements client operations; it does not decide who may call
them.

- SMF context-query operations are candidates for explicitly declared
  Agent-callable Tools.
- AMF `/ue-info`, AMF `/gnb-info` and SMF `/pdu-info` may be registered as Host
  defaults but remain Host-internal unless the Host/Runtime contract explicitly
  exposes them. Their source payloads may contain SUPI, SUCI, PEI, location and
  network topology data; never pass raw metrics pages directly to the Agent.
- Tool visibility and Capability authorization must not be inferred from a
  CLI subcommand's existence.
- The SMF context request accepts SUPI and/or IMSI, but the OpenAMSM v0.10 response
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

## Current implementation state and future client gate

The CLI v1 NF/resource command tree, strict JSON stdin, JSON stdout/stderr
envelopes, exit-code mapping and cancellation propagation are implemented on
the review branch. Do not treat the former pre-v1 flag CLI migration as an
open implementation task. Go 1.26.2 test/vet/build and live-NF acceptance
remain pending; see `docs/WORK.md` and `docs/TRACEABILITY.md`.

The next client implementation is not pre-authorized by this repository
guidance. A new NF client or command may start only after its use case,
3GPP action, owning NF, pinned OpenAMSM endpoint/schema, identity and error
semantics, CA caller, and verification plan have been added to
`docs/nf-capability-map.md` and `docs/openamsm-interface-inventory.md`.

Explicitly out of scope:
- changes to `ca-agent`, `ca-runtime`, `ca-skills`, Host adapters or ToolCatalog;
- NW Agent Workflow Planning / Replan, Nsmf PDUSession Facade, SM Context
  Orchestration Invoke / Update Sync, or Namf Relay endpoints;
- mutation APIs, orchestration, callbacks, route registration, journals,
  business lifecycle, Evidence, Outcome or Reconcile;
- Open5GS compatibility, UPF `/session-info`, or inferred N4 SEID joins;
- building or modifying the imported `env-openamsm` source snapshot.

The NW Agent endpoints above are real, separately specified interfaces in the
OpenAMSM source tree. Their exclusion here is intentional: this repository's
initial deliverable is a bounded read-only client/CLI surface, not another
workflow service or NF facade. Do not add them merely because their OpenAPI
files are present in the source snapshot.

If a requirement appears to conflict across documents, use
`docs/cli-contract.md` for the local CLI process contract and the pinned
OpenAMSM OpenAPI/source for remote wire behavior. Do not silently invent a
resolution; report the exact conflict and stop that item.

## Delivery and validation

- Keep implementation small and OpenAMSM-specific.
- Add focused tests for every supported endpoint and error/status distinction.
- Run `gofmt`, `go test ./...`, `go vet ./...`, and a native build.
- For every future NF client, complete the capability-map gate before coding.
- Mock HTTP tests prove client contract behavior only. They do not prove
  OpenAMSM source compatibility or live-NF acceptance.
- Never claim source build, live query or end-to-end acceptance without
  evidence from the actual OpenAMSM environment.

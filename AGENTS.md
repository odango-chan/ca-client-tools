# CA Client Tools repository rules

## Project identity

This repository provides a reusable 3GPP SBI client/tool-provider foundation,
typed OpenAMSM clients and a stable local CLI. The SBI transport and payload
engine are designed to be NF/service-agnostic; the current deployment/source
target remains OpenAMSM. This is not an Open5GS compatibility layer.

Do not create a `fivegc/` package or directory. OpenAMSM is the selected
upstream environment and source of interface behavior.

## Required ownership boundaries

- `env-openamsm` owns the OpenAMSM source snapshot, build, configuration,
  deployment and NF process lifecycle.
- `ca-client-tools` owns the reusable SBI transport, trusted Operation Catalog,
  Tool Provider implementation, Payload Engine, typed OpenAMSM client calls
  and the local CLI.
- `ca-agent` owns Host integration, default-tool registration, configuration
  injection and the policy that distinguishes Agent-callable tools from
  Host-internal tools.
- `ca-runtime` owns callable admission, Capability authorization, Runtime
  execution identity, Procedure progression, Outcome, Evidence and Reconcile.
- `ca-skills` owns business Procedures and declared callable dependencies.

The CLI v1 remains a fixed, non-interactive local process interface governed
by docs/cli-contract.md. It is not an arbitrary shell and is not the Agent Tool
Provider interface. Keep Cobra confined to internal/cli; never import it from
client packages. Preserve the existing CLI v1 command tree and JSON
stdin/stdout/stderr contract.

The SBI Tool Provider is a preinstalled Go executable in the ca-agent
deployment. The current Python Demo Host dynamically activates only trusted
operation descriptors required by the active Skill and permitted by
deployment policy.
The Host resolves BinaryRef values into private invocation-scoped input files
and ingests response attachments from a private output directory. Do not route
binary bytes through CLI JSON or base64. Skill-authored arguments must never supply
arbitrary URLs, methods, paths, headers, credentials or Operation
Manifests. Runtime admission and Capability authorization remain
mandatory. See docs/sbi-client-architecture.md.

The optional curl-like raw request mode is operator-only, disabled by default
and never registered as an Agent Tool. It is not part of CLI v1.

The generic SBI executor is for standard SBI HTTP/2 operations. Existing
AMF/SMF metrics endpoints remain separate typed HTTP clients. Direct N4 PFCP transport is separate from SBI; an Nsmf SBI
operation may carry an opaque PFCP message as a declared multipart payload.
Other NF operations are added through the trusted Operation Catalog only
after the per-operation evidence gate.

## OpenAMSM source and interface evidence

Before changing or adding any client behavior, read these documents in order:

1. `docs/nf-capability-map.md` — canonical NF responsibilities, CA ownership,
   3GPP → Tool → Skill derivation, client scope and evidence states.
2. `docs/openamsm-interface-inventory.md` — source-backed endpoint and wire
   contracts for the pinned OpenAMSM snapshot.
3. `docs/architecture.md` — repository boundaries and Agent/Host exposure.
4. `docs/sbi-client-architecture.md` — normative SBI transport, Operation
   Catalog, Tool Provider, configuration and BinaryRef/multipart contract.
5. `docs/cli-contract.md` — normative local CLI v1 process contract.
6. The pinned OpenAMSM source findings and imported source snapshot linked
   below.
7. The OpenAMSM v0.10 SMF context-query OpenAPI contract and its source tests
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
The imported source is immutable. The inspected source does not expose a UPF
`/session-info` endpoint; no UPF session client or server patch is part of the
current scope. The inventory is source-inspected; native build, source unit
tests and live-NF acceptance are not yet claimed.

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
- Do not automatically retry mutations. A retry is allowed only when the
  trusted operation definition specifies an idempotency/reconciliation
  contract and Host/Runtime policy permits it.
- Do not create Runtime Evidence, Predicate verdicts, business lifecycle
  records, journals or reconciliation state here.

## Tool visibility

This repository provides operation descriptors and executors; it does not
decide who may call them.

- The CA-Agent Tool Provider is preinstalled, while the Host dynamically
  activates only operations required by the active Skill and permitted by
  deployment policy.
- Runtime owns callable admission and Capability authorization.
- Each operation is independently classified as Agent-callable or
  Host/Tool-internal. A catalog entry or CLI command never grants permission.
- Raw HTTP request mode is never an Agent Tool.
- Sensitive AMF metrics and raw subscriber projections remain Host-private
  unless a separate contract authorizes a safe projection.
- BinaryRef storage, access control, expiry and cleanup are Host-owned. The
  Client consumes Host-provided streams and returns response attachments to a
  Host-provided sink.
- The SMF context-query adapter must continue validating the echoed lookup
  key against the Host's trusted binding, normalize IMSI bindings to SUPI for
  comparison, and redact/replace subscriber identifiers before returning an
  Agent-visible Observation. Raw CLI stdout is Host-private.

## Reuse from ca-provider-5gc

Do not copy the Provider HTTP server, Runtime ObservationSource, Runtime
correlation/evidence handling, CreatedSession workflow state, PCF/UDM
orchestration, Fabric journals or mutation adapters wholesale.

A low-level reader/parser may be reused only after separating transport and
source parsing from Provider/Runtime semantics and verifying its exact
compatibility with OpenAMSM.

## Current implementation state and future client gate

The CLI v1 NF/resource command tree, strict JSON stdin, JSON stdout/stderr
envelopes, exit-code mapping, response identity binding and cancellation
propagation are implemented on the review branch. Do not treat the former
pre-v1 flag CLI migration as an open implementation task. Go 1.26.8
test/vet/build and local contract acceptance PASS. AMF/SMF list surfaces and
all eight SMF context queries PASS against a real OpenAMSM deployment; the
context pass uses an authorized local `env-openamsm` build overlay for the
lowercase token lookup, with the imported upstream tree unchanged. See
`docs/WORK.md`,
`docs/TRACEABILITY.md` and
`docs/evidence/real-openamsm-acceptance-20261117.md`.

The h2c SBI transport and static OpenAMSM token-header profile are
implemented on stacked PR #2; the JSON/binary/multipart Payload Engine and
Host-staged file boundary are implemented on stacked PR #3. Both await Go
test/vet/build acceptance. The Operation Catalog and Tool Provider are not
yet implemented. A new Agent-callable operation may be added only
after its use case, 3GPP action, owning NF, pinned release-specific OpenAPI,
selected OpenAMSM source/deployment support, identity and error semantics,
Capability binding, effect/idempotency metadata and verification plan have
been added to docs/nf-capability-map.md and docs/openamsm-interface-inventory.md.
Operation definitions are trusted package content, never Skill-authored input.

Explicitly out of scope for this repository:
- implementation changes to ca-agent, ca-runtime, ca-skills, Host adapters or
  Runtime ToolCatalog; this repository defines and supplies the Provider
  contract, while Host/Runtime integration is a separate workstream;
- NW Agent Workflow Planning / Replan, Nsmf PDUSession Facade, SM Context
  Orchestration Invoke / Update Sync, and Namf Relay workflow/facade endpoints;
- multi-step NF orchestration, callbacks, route registration, pending-operation
  journals, business lifecycle, Evidence, Outcome or Reconcile;
- arbitrary HTTP requests exposed as Agent Tools;
- Open5GS compatibility, inferred N4 SEID joins, and direct PFCP transport
  inside the SBI executor. Opaque PFCP message parts declared by an Nsmf SBI
  operation are handled as binary multipart payloads, not as PFCP transport;
- building or modifying the immutable imported env-openamsm source snapshot.

Individual 3GPP SBI read and mutation operations are not excluded merely
because they mutate state. They must be separately catalogued, source-verified,
authorized and tested. The private NW Agent workflow/facade/relay endpoints
remain excluded; do not add them merely because OpenAPI files exist in the
source snapshot.

If a requirement appears to conflict across documents, use
`docs/cli-contract.md` for the local CLI process contract and the pinned
OpenAMSM OpenAPI/source for remote wire behavior. Do not silently invent a
resolution; report the exact conflict and stop that item.

## Delivery and validation

- Keep OpenAMSM deployment adapters and private endpoint definitions source-specific. Keep the shared SBI transport standards-based and NF/service-agnostic; do not turn it into a generic 5GC workflow framework.
- Add focused tests for every supported endpoint and error/status distinction.
- Run `gofmt`, `go test ./...`, `go vet ./...`, and a native build.
- For every future NF client, complete the capability-map gate before coding.
- Mock HTTP tests prove client contract behavior only. They do not prove
  OpenAMSM source compatibility or live-NF acceptance.
- Never claim source build, live query or end-to-end acceptance without
  evidence from the actual OpenAMSM environment.

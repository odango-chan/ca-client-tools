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

WSL is an **optional acceptance environment only, never a product dependency**.
The product targets Windows-native, Linux-native, Docker, Kubernetes and remote
5GC/NF deployments. Product code — the Go client, shared SBI transport, Payload
Engine and CLI — must not detect or depend on WSL and must not add WSL-specific
configuration fields or WSL network handling. Without WSL, an operator
configures a real remote SBI origin and does not start the forwarder. The only
WSL-specific code is the optional acceptance tooling under `scripts/acceptance/`:
the forwarder is WSL-mirrored-networking-specific, binds loopback only
(`127.0.0.1`), exposes exactly the three acceptance services (SMF SBI `17777`,
AMF metrics `19090`, SMF metrics `29090`), is not a general proxy, and must
never be imported by product code.

The CLI v1 remains a fixed, non-interactive local process interface governed
by docs/cli-contract.md. It is not an arbitrary shell and is not the Agent Tool
Provider interface. Keep Cobra confined to internal/cli; never import it from
client packages. Preserve the existing CLI v1 command tree and JSON
stdin/stdout/stderr contract.

The SBI Tool Provider is a preinstalled Go executable in the ca-agent
deployment. The current Python Demo Host dynamically activates only trusted
operation descriptors required by the active Skill and permitted by
deployment policy.
The Host owns Artifact Store reference issuance, resolution and lifecycle. Use [Host integration](docs/host-integration.md) and the [payload contract](docs/provider-payload-extension-v1.md). Keep bytes outside NDJSON. Invocation arguments never supply arbitrary URLs, methods, paths, headers, credentials or Operation Manifests. Runtime admission and Capability authorization apply.

The optional curl-like raw request mode is operator-only, disabled by default
and never registered as an Agent Tool. It is not part of CLI v1.

The generic SBI executor is for standard SBI HTTP/2 operations. Existing
AMF/SMF metrics endpoints remain separate typed HTTP clients. Direct N4 PFCP transport is separate from SBI; an Nsmf SBI
operation may carry an opaque PFCP message as a declared multipart payload.
Other NF operations are added through the trusted Operation Catalog only
after the per-operation evidence gate.

## Authoritative read order

For every Harness run, read in this order:

1. `AGENTS.md` — authority, repository boundaries and delivery rules.
2. `docs/DEEPSEEK-HANDOFF.md` — execution entry point.
3. `docs/OVERNIGHT-GOAL.md` — runnable scope.
4. `docs/WORK.md` — status, ordered P0/P1 and acceptance gates.
5. `docs/architecture.md` — consolidated target.
6. `docs/provider-protocol-v2.md` — sole process wire contract.
7. `docs/provider-backend-binding-catalog-v2.md` and then `contracts/provider-bindings/v2/provider-bindings.yaml` — target rules and current embedded artifact.
8. `docs/generic-sbi-executor.md`.
9. `docs/sbi-client-architecture.md` and then `docs/service-configuration.md`.
10. `docs/host-integration.md` and then `docs/provider-payload-extension-v1.md`.
11. `docs/nf-capability-map.md`, `docs/openamsm-interface-inventory.md`, then `docs/provider-read-tool-inventory-20261117.md`.
12. Pinned OpenAMSM sources and tests linked below.
13. `docs/cli-contract.md`.
14. `docs/TRACEABILITY.md`, `docs/e2e-acceptance.md`, then the applicable dated evidence under `docs/evidence/`.

The architecture decision is one Provider protocol and one Catalog for the demo, with no backward-compatibility requirement. The protocol and Catalog documents own their respective contracts. Pinned OpenAMSM source/OpenAPI owns remote behavior, the CLI contract owns the local operator CLI, and the published Host contract owns the Artifact Store interface. WORK owns status. Historical evidence applies to its pinned revision only. Escalate unresolved same-scope conflicts instead of inventing wire behavior.

## OpenAMSM source and interface evidence

The authoritative OpenAMSM source archive is imported under
`env-openamsm/upstream/openamsm` on `env-openamsm/main` (archive SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`, source
commit comment `c27e1761ae09957a474cc2b0208d227cde64aabf`, 11,904 files). The corresponding `env-openamsm` repository main is pinned at
[`594dec78b95bff4fdc34deac8a3a9681fb41228f`](https://github.com/odango-chan/env-openamsm/tree/594dec78b95bff4fdc34deac8a3a9681fb41228f). The source findings are at
[docs/openamsm-source-findings.md](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/docs/openamsm-source-findings.md);
the OpenAMSM context-query wire contract is
[Nsmf_MasContextQuery.yaml](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml);
and source tests are under
[tests/smf-mas-query/](https://github.com/odango-chan/env-openamsm/tree/594dec78b95bff4fdc34deac8a3a9681fb41228f/upstream/openamsm/tests/smf-mas-query/).
The imported source is immutable. The inspected source does not expose a UPF
`/session-info` endpoint; no UPF session client or UPF/N4 server patch is part
of the current scope. Native build and live-NF acceptance are recorded in
[env-openamsm acceptance](https://github.com/odango-chan/env-openamsm/blob/594dec78b95bff4fdc34deac8a3a9681fb41228f/docs/acceptance-20261117.md)
and `docs/evidence/real-openamsm-acceptance-20261117.md`. The only OpenAMSM
build overlay is the explicit MAS Query token lowercase lookup; it is owned by
`env-openamsm` and is not part of this repository.

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

See [WORK](docs/WORK.md) for verified implementation status. A new Agent-callable operation requires a documented use case, owning NF, pinned release-specific OpenAPI and deployment evidence, identity/error semantics, Capability binding, effect/idempotency metadata and test plan. Record these in the capability map and interface inventory before implementation.

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

## GitHub workflow

Fetch and verify the current draft PR #7 head and branch before changes. Deliver on `feat/unified-provider-read-tools-20261117` with ordinary commits and fast-forward pushes. If the head moves, reconcile against the latest head and verify again. Keep PR #7 draft/open. Do not merge, push to main, force-push, create a replacement PR, or update PR #5. Report exact pushed SHA and file list. Distinguish local tests, live tests and GitHub CI. A job that failed to start is unavailable, never a passing check.

## Delivery and validation

- Keep OpenAMSM deployment adapters and private endpoint definitions source-specific. Keep the shared SBI transport standards-based and NF/service-agnostic; do not turn it into a generic 5GC workflow framework.
- Add focused tests for every supported endpoint and error/status distinction.
- Run `gofmt`, `go test ./...`, `go vet ./...`, and a native build.
- For every future NF client, complete the capability-map gate before coding.
- Mock HTTP tests prove client contract behavior only. They do not prove
  OpenAMSM source compatibility or live-NF acceptance.
- Never claim source build, live query or end-to-end acceptance without
  evidence from the actual OpenAMSM environment.

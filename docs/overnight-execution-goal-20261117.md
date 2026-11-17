# CA Client Tools — overnight implementation goal

**Date:** 2026-11-17  
**Status:** IMPLEMENTATION HANDOFF — not a new protocol or language specification  
**Primary acceptance target:** `PROVIDER_HOST_DISPATCH_IMPLEMENTATION_PASS`  
**CA-SL baseline:** v1alpha4 — FROZEN; do not change its grammar or semantics

This document is the execution entry point for an overnight DeepSeek Harness run. The detailed contracts linked below remain authoritative. This document orders the work, identifies what is implementable now, and explicitly fences off work whose contracts are still drafts or incomplete.

## 1. The goal

Complete the first real **Host → Runtime → resident Provider → OpenAMSM** vertical slice for SMF context Observation.

The deliverable is not merely a Go Provider that can answer a local request. The implementation is accepted only when:

1. `ca-client-tools` runs one resident Go Provider process using Provider Process Protocol v1;
2. it exposes the eight private SMF backend operation IDs from the frozen binding file and reuses the existing typed SMF client;
3. `ca-runtime` implements the narrow RuntimeHost.v2 HostBusinessCorrelation contract while preserving v1 unchanged;
4. `ca-agent-demo` admits v1alpha4 Operation/Observation callables through one exact identity and authorization boundary, starts the resident Provider, and maps admitted Host callables to private backend IDs;
5. a real query succeeds through the actual Host/Runtime/Provider path against the pinned OpenAMSM deployment, with negative authorization and Scope tests proving fail-closed behavior.

The public surface contains **nine Host callables mapped to eight private Provider operations**. The generic `smf.session.context.query` and projection-specific `smf.mas.sc.context.query` share the private `openamsm.smf.mas.sc.query` backend operation, but retain distinct Host output Schemas and normalization.

This first slice is intentionally JSON-only. It is the shortest path to a working, verifiable Provider integration. It does not finish the broader 3GPP Operation Catalog, generic mutation execution, or binary Provider protocol extension.

## 2. Repository and contract snapshot

The following are the current review sources; do not overwrite their documentation branches with implementation commits.

| Repository | Current review item | Role / state |
|---|---|---|
| `ca-client-tools` | [PR #5](https://github.com/odango-chan/ca-client-tools/pull/5) | Draft, documentation-only: Provider Process Protocol v1, first private binding set and SMF backend contract |
| `ca-agent-demo` | [PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208) | Draft: SUPI/PSI context boundary, Host-owned SMF Observation proposals and existing 5GC Host changes |
| `ca-agent-demo` | [PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214) | Draft, documentation-only: v1alpha4 callable stages, authorization, Provider integration and exact code-entrypoint gaps |
| `ca-runtime` | [PR #176](https://github.com/odango-chan/ca-runtime/pull/176) | Draft: contains the separately frozen synchronous RuntimeHost.v2 HostBusinessCorrelation sub-contract; its implementation is not started |
| `ca-skills` | [PR #58](https://github.com/odango-chan/ca-skills/pull/58) | Draft: v1alpha4 PDU/MAS examples; the generic session read uses the canonical public ID |
| `ca-agent-demo` | [PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211) | Draft Host Artifact Contract; not a first-slice dependency |
| `ca-agent-demo` | [PR #213](https://github.com/odango-chan/ca-agent-demo/pull/213) | Separate asynchronous SBI continuation design; not a first-slice dependency |
| `ca-skills` | [PR #61](https://github.com/odango-chan/ca-skills/pull/61) / [PR #62](https://github.com/odango-chan/ca-skills/pull/62) | Next-version Event Wait / Integration Ingress design; not part of frozen v1alpha4 implementation |

The older `ca-agent-demo#209` continuation PR is closed; #213 is the current follow-up. Do not treat the asynchronous continuation proposal as a dependency of the synchronous Provider/Observation slice.

### Normative inputs — read in this order

1. [CA-SL v1alpha4 frozen specification](https://github.com/odango-chan/ca-skills/blob/main/docs/skill-language-spec.v1alpha4.zh-CN.md) — language semantics remain unchanged.
2. [Public Callable ID Naming Contract v1](https://github.com/odango-chan/ca-agent-demo/blob/docs/v1alpha4-callable-runtime-contract-20261117/docs/design/public-callable-naming-contract-v1.md) — public IDs, dependency aliases and identity-layer separation.
3. [CA-SL v1alpha4 Callable Runtime Integration Contract](https://github.com/odango-chan/ca-agent-demo/blob/docs/v1alpha4-callable-runtime-contract-20261117/docs/design/v1alpha4-callable-runtime-integration-contract-v1.md) — especially §§2–8.
4. [Provider Process Protocol v1](provider-protocol-v1.md) — exact private process boundary; first slice only.
5. [OpenAMSM SMF Query Backend Contract v1](smf-query-backend-contract-v1.md) — exact eight backend IDs and response semantics.
6. [Provider Backend / Host Callable Interoperability Contract](sbi-capability-provider-contract.md) — ownership and public-to-private mapping.
7. [Provider Backend Binding Catalog v1](provider-backend-binding-catalog-v1.md) — exact private catalog structure and validation rules.
8. [Provider Backend Binding Catalog YAML](../contracts/provider-bindings/v1/provider-bindings.yaml) — machine-readable first-slice binding set.
9. [SBI Client Architecture](sbi-client-architecture.md) — normative transport, payload and long-term operation-binding architecture.
10. [SBI Implementation Backlog](sbi-implementation-backlog-20261117.md) — WP0–WP15 and independent follow-on lanes.
11. [RuntimeHost.v2 HostBusinessCorrelation Contract](https://github.com/odango-chan/ca-runtime/blob/docs/operation-completion-continuity-20261116/docs/design/runtime-host-v2-host-business-correlation-contract.md) — synchronous v2 envelope only.
11. [Host 5GC Context Contract](https://github.com/odango-chan/ca-agent-demo/blob/docs/5gc-agent-context-boundary-20261116/docs/5gc-agent-context-contracts-v1alpha4.md) — SUPI/PSI and HostBusinessInstance binding.

If a local checkout contains a newer reviewed contract revision, compare it to the linked PR head and use the latest explicit owner decision. Do not infer approval from a draft branch.

## 3. Existing implementation — reuse, do not rewrite

### ca-client-tools

Already implemented and merged to `main`:

- CLI v1 and typed OpenAMSM AMF/SMF clients;
- bounded shared h2c SBI transport in `internal/sbi`;
- JSON, opaque binary and multipart/related Payload Engine in `internal/sbi/payload`;
- root-confined staged-file Resolver and generated-name Attachment Sink primitives;
- Windows-native and WSL/Linux validation plus real CLI acceptance (23/23 strict live cases).

The current CLI and live OpenAMSM evidence do **not** prove Provider or Host dispatch acceptance.

The existing `internal/openamsm/smf/mas.Client.Query` is the source-verified typed query implementation. Provider v1 MUST call it directly. Do not spawn the CLI, write a second query client, or replace its response validation with a weaker generic parser.

The existing SMF query client still has its own h2c transport. Its migration to the shared `internal/sbi` transport is a later convergence task after exact parity tests; it is not a blocker for this first Provider slice.

### Cross-repository

- RuntimeHost.v1 already exists. RuntimeHost.v2 is an additive, explicitly selected contract and must not silently alter v1.
- Host currently has legacy Operation-centric and ObservationSource code paths. PR #214 §6 lists the exact current code structures and required v1alpha4 changes. Do not wrap the legacy `ObservationSource.acquire()` path and call it v1alpha4 Observation support.
- The Host Artifact Store and physical HostArtifactRef contract are not implemented. The low-level Client Payload Engine file interfaces do not imply that the Host Artifact Store exists.

## 4. P0 implementation lanes

Use separate implementation branches and Draft PRs based on each repository's current `main`. Do not commit implementation changes to the existing documentation PR branches.

### Lane A — ca-client-tools Provider runtime (WP1 + WP2)

Implement the separate machine command `ca-client-tools provider serve`. Keep the existing CLI v1 command tree and stdin/stdout contract unchanged.

Required behavior:

- one long-lived process per configured Provider instance; fixed executable/argv, no shell;
- bounded NDJSON frames exactly as Provider Protocol v1 specifies; stdout contains protocol frames only, stderr contains safe diagnostics only;
- strict UTF-8/JSON framing, duplicate-key rejection, unknown-field rejection, frame-size enforcement and unique per-process request IDs;
- `initialize` must be the first request; validate trusted environment configuration and negotiated limits;
- embed and load the exact versioned `provider-bindings.yaml`; calculate `bindingCatalogDigest` as SHA-256 over the exact embedded UTF-8 bytes, including the final LF;
- `catalog` returns an exact supported/unsupported partition of only the requested private backend IDs; it never returns public callable IDs, Tool schemas or Capability decisions;
- `invoke` accepts only an exact registered backend ID and the typed SUPI + PDU Session ID arguments defined by the contract; use the corresponding fixed MAS type and existing typed Go client;
- return valid remote HTTP ProblemDetails as a typed remote result; malformed remote responses become Provider/backend errors; never convert them into Owner Outcomes or Evidence;
- implement `cancel`, bounded concurrency, deadlines, out-of-order response correlation, one terminal response per accepted invocation, drain and EOF shutdown;
- no automatic retries, no replay after process restart, no raw remote bodies, no token/SUPI leakage;
- HostBusinessCorrelation is never forwarded to Provider Protocol v1.

Use a maintained YAML parser and strict typed decoding/semantic validation; do not write a hand-rolled YAML parser. Keep Cobra confined to the CLI layer. Add focused tests for every protocol method, malformed frame, duplicate key, unknown field, boundary limit, concurrency/cancellation race and process lifecycle.

### Lane B — ca-runtime RuntimeHost.v2 (WP3)

Implement only the frozen synchronous HostBusinessCorrelation contract:

- add the complete versioned v2 schema at the path specified by the contract;
- preserve RuntimeHost.v1 schema and behavior unchanged, including rejection of the v2-only field;
- validate the exact two-member opaque object (`ingressRef`, `lifecycleRef`), with no partial/null/empty/unknown fields;
- include both values in request fingerprint and immutable Run identity/RunRecord persistence and restoration;
- add the exact `run.started.payload.hostBusinessCorrelationDigest` calculation specified by the contract, outside the existing Runtime `correlation` object;
- test same-request replay, changed-content conflict, Run rebinding rejection, restart restore and v1 regression.

Do not implement Temporal, pending-Operation continuation, callbacks, inbound SBI, a new Wait model or new Run control states in this lane.

### Lane C — ca-agent-demo Host callable integration (WP4)

Follow ca-agent-demo PR #214 §6 and §8.1. The required changes include:

- preserve ordered v1alpha4 Node stages: Precondition → Operation → Observation → Postcondition;
- represent Operation and Observation with exact callable identity `(owner, kind, id)` and one admission/dispatch boundary, preserving their distinct Kind, Schema, Effect and Outcome semantics;
- link `operation.tool` only to Operation exports and `observation.tool` only to Observation exports;
- derive Capability membership only from Skill-granted Capability IDs plus trusted deployment CapabilityContracts; remove direct callable-ID grant fallback;
- apply exact SUPI/PSI and HostBusinessInstance Scope before dispatch;
- make Predicate evaluation consume explicit Observation output/Evidence; do not use legacy `ObservationSource.acquire()` as the v1alpha4 acquisition path;
- start one resident Go Provider, verify executable/package and binding-catalog digests, send only the exact private backend IDs in `catalog`, and correlate invoke/cancel by requestId;
- keep HostBusinessCorrelation in the Host-to-Runtime v2 envelope only; do not put it in Skill input, Observation QueryContext or Provider Protocol v1;
- map each admitted public callable to exactly one private backend operation; generic and MAS-SC public callables share the SC backend but keep separate Host output schemas and normalization.

Do not create a second Tool registry, expose Provider backend IDs as public callables, or fall back to the CLI when the Provider is unavailable.

### Lane D — cross-repository implementation acceptance (WP5)

The implementation acceptance fixture may use test-only, non-published ToolCatalog/CapabilityContracts and test-only Evidence fixtures to exercise Runtime stage plumbing. Those fixtures MUST remain test-only; they are not production Owner publications or a source freshness guarantee.

Run the real diagnostic query through the installed Provider and actual Host/Runtime dispatch path against the pinned OpenAMSM deployment. Verify:

- generic HTTP 404 + exact `SESSION_NOT_FOUND` → generic session-presence result `ABSENT`;
- HTTP 200 + `UNAVAILABLE` → session `PRESENT` with an empty projection;
- MAS projection-specific 404 → its declared Owner failure, never `UNAVAILABLE`;
- exact SUPI/PSI binding and cross-UE/cross-PSI rejection;
- wrong callable kind, missing Capability, wrong Scope, unsupported backend ID and malformed catalog partition fail before any unauthorized NF request;
- malformed ProblemDetails, mismatched response key, deadline, cancellation, process exit and restart-no-replay fail closed.

A direct CLI query, Provider-local unit test or mocked Host dispatch is intermediate evidence, not this acceptance.

## 5. P1 follow-on lanes — do not block P0

| Work | Current boundary | Tonight's allowed work |
|---|---|---|
| Shared transport convergence (WP6) | Existing typed MAS client has a separate h2c transport; `internal/sbi` is already implemented | Add exact parity tests and, if safe, migrate the typed client without changing status, echo, ProblemDetails, timeout or cancellation behavior. This is a convergence requirement before general Operation Catalog onboarding, but not a P0 Provider blocker. |
| General 3GPP Operation Catalog (WP7) | Architecture is accepted, but a field-level machine contract and generic Provider invoke schema are not frozen | Produce a design proposal for the pinned OpenAPI-derived binding, generic typed arguments, service-key resolution, response/media-type mapping and READ/MUTATION metadata. Because Provider Protocol v1 `invoke.arguments` is deliberately SMF-specific, define a separately versioned generic invocation protocol/extension; do not add generic fields to v1. Do not invent a loader format or treat the private SMF v1 catalog as the general catalog. |
| Multi-service trusted configuration | `internal/sbi.Config` supports service maps in memory; Provider v1 accepts only the two allowlisted OpenAMSM environment variables | Propose a versioned trusted config-file schema, secret-file handling and Host injection model. Do not add config paths/secrets to Skill arguments or Provider v1 frames. |
| Provider binary/multipart extension (WP9) | Payload Engine is implemented; Provider v1 is intentionally JSON-only; HostArtifactRef physical contract remains draft in ca-agent-demo PR #211 | Audit the exact cross-repository extension requirements and produce a separately versioned protocol proposal. Do not add binary fields to Provider v1 or invent HostArtifactRef grammar/storage APIs. |
| First outbound Mutation (WP8) | No selected operation has a complete pinned source + Owner + deployment + idempotency/reconciliation contract | Build a source-backed candidate matrix only. Do not enable mutation dispatch or automatic retries. |
| NAS/NGAP codec and N1/N2 migration (WP10) | Migration design only; Host Artifact and Fabric authority gates remain open | Inventory reusable codecs and byte-level fixtures; do not migrate the binary execution path before the Host Artifact and Provider extension contracts are frozen. |
| NF coverage (WP11) | NF roles are mapped; deployment support is operation-specific | Update source-backed inventory only; do not infer AMF/UDM/PCF/UPF operations from 3GPP names. |
| Event Wait / Integration Ingress (WP12) | Separate next-version design in ca-skills #61/#62 and ca-agent-demo #210/#213 | Compatibility audit only. Do not backport into CA-SL v1alpha4 or this Provider slice. |
| TLS/mTLS, OAuth2, NRF discovery (WP13) | Not required for the selected OpenAMSM h2c/token profile | Defer; do not add complexity to the current deployment profile. |
| Legacy ca-provider-5gc cleanup (WP14) | Requires exact Provider/Host parity | Do not remove old code before migration acceptance. |

## 6. Non-negotiable architecture rules

- Public Callable IDs, dependency aliases, Capability IDs, Provider IDs, private backend operation IDs and 3GPP source `operationId` values are different identity layers.
- Use the Host-owned NF-first public ID grammar. First SMF public IDs are the generic `smf.session.context.query` plus the eight `smf.mas.<projection>.context.query` IDs. The additional AMF/SMF/PCF/UDM names are naming candidates only, not implemented callables.
- `context.smf.session.read` is retired; reject it, do not alias it.
- Never derive a Capability ID by prefixing a Callable ID or dependency alias.
- `openamsm` appears only in private backend/source identifiers, never in public Callable IDs or Skill references.
- The Host owns ToolCatalog, public schemas, Owner Outcomes, Evidence Producer and public-to-private adapter mapping. Deployment owns CapabilityContracts and Scope. Runtime owns admission, execution identity, Outcome/Evidence/Reconcile. Client Tools owns typed client execution and private backend bindings.
- The Provider is deterministic. It does not run an LLM, execute a Skill, select routes, authorize Capabilities, create Evidence or own workflow state.
- No generic arbitrary-URL/method/path/header/curl Agent Tool. No Skill-authored Operation Manifest.
- No direct PFCP/N4 transport inside the SBI executor. Opaque PFCP bytes may be a declared multipart part of an SBI operation; they are not PFCP transport.
- Do not modify frozen CA-SL v1alpha4, fabricate SMF `observedAt`/`validUntil`/`maxAgeMs`, or publish production Evidence descriptors from test fixtures.

## 7. Validation and reporting

### Local validation

- ca-client-tools: `gofmt`, `go test ./... -count=1`, `go vet ./...`, `go build ./...`, plus protocol/process-specific tests.
- ca-runtime: follow its current `AGENTS.md` and `docs/WORK.md`; the documented full local gate is `GOWORK=off bash scripts/verify-local.sh`.
- ca-agent-demo: follow its current `AGENTS.md`, `Makefile` and PR #214 acceptance list; run the repository's full local test and validation commands.
- Cross-repository: preserve reproducible command output, tested commit SHAs, OpenAMSM source pin, sanitized environment description and live request/response assertions. Never store tokens or raw subscriber responses in committed evidence.

GitHub Actions may remain `CI_UNAVAILABLE` because of the recorded account billing/spending-limit condition. Do not loop on unavailable Actions runs. Local acceptance is reported separately and is not called CI PASS.

### Required final status vocabulary

Report every work package separately as one of:

- `IMPLEMENTED`
- `LOCAL_ACCEPTANCE_PASS`
- `REAL_E2E_PASS`
- `PROVIDER_HOST_DISPATCH_IMPLEMENTATION_PASS`
- `BLOCKED`
- `NOT_STARTED`
- `NOT_QUALIFIED`
- `CI_UNAVAILABLE`

Every PASS requires a commit, PR, exact test command and reproducible evidence. A repository's unit tests passing does not complete the cross-repository Goal.

### Product qualification remains separate

The pinned SMF v0.10 response has no source timestamp and no accepted live-authoritative-read freshness guarantee. Therefore production ToolCatalog/CapabilityContracts publication, real v1alpha4 Skill qualification and Evidence-dependent Predicate PASS remain `NOT_QUALIFIED`. This does not block Provider implementation or diagnostic Host-dispatch acceptance. Never fabricate freshness metadata.

## 8. Harness execution rules

1. Start from each repository's current `main`; inspect its `AGENTS.md`, active PRs, worktree and tests before editing.
2. Create one implementation branch/PR per repository. Keep the existing documentation PR branches untouched.
3. Work through independent lanes in parallel where dependencies permit. A missing contract blocks only its affected lane; write an exact BLOCKED record and continue other lanes.
4. Do not push directly to `main`, force-push shared branches, silently merge existing PRs or rewrite another agent's work.
5. Do not claim completion from a design document, mock, CLI acceptance or Provider-local test alone.
6. Update each repository's WORK/TRACEABILITY/README and handoff only after the implementation and evidence actually exist.
7. At the end, provide a per-repository PR list, per-WP status, commits, local commands/results, real E2E evidence, unresolved blockers and the next safe action.

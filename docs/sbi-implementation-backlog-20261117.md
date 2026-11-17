# CA SBI Unified Client / Provider — Implementation Backlog

**Status:** IMPLEMENTATION HANDOFF — inventory verified against 2026-11-17 repository main snapshots and active design PRs  
**Primary language target:** CA-SL `connectionagent.io/v1alpha5` Preview (NOT RELEASED / NOT QUALIFIED). v1alpha4 is the immutable inherited semantic baseline, not the next implementation target.  
**Qualification:** NONE of the pending Host/Provider/Runtime slices is claimed implemented by this document.

This register is an execution queue, not a parallel architecture. Its core goal is **one reusable outbound SBI Client, one typed Provider backend system, and one Host callable dispatch plane** for both READ and MUTATION operations. The first released CLI is read-only; this does not restrict the underlying SBI Client. A generic Agent-facing curl Tool is not authorized.

## 1. Verified existing foundation

| Item | Evidence / current repository location | Status |
|---|---|---|
| Typed OpenAMSM SMF MAS query client and fixed CLI | `internal/openamsm/smf/mas/client.go`, `cmd/ca-client-tools`; PR #1 | IMPLEMENTED; real CLI query previously accepted |
| Shared bounded SBI HTTP/2 transport | `internal/sbi/client.go`; merged PR #2 | IMPLEMENTED |
| JSON / opaque binary / multipart-related Payload Engine, streaming file Resolver and Attachment Sink | `internal/sbi/payload/payload.go` and `files.go`; merged PR #3 | IMPLEMENTED library; no catalogued live binary Provider operation |
| Eight private SMF backend operation bindings | `contracts/provider-bindings/v1/provider-bindings.yaml` on PR #5 | CONTRACT ONLY; no resident Provider execution |
| NDJSON `initialize/catalog/invoke/cancel` protocol | `docs/provider-protocol-v1.md` on PR #5 | CONTRACT FROZEN FOR FIRST JSON OBSERVATION SLICE; runtime NOT IMPLEMENTED |
| Host v1alpha5 inherited Observation stage / unified callable dispatcher | ca-agent-demo PR #214; current main `runtime/frozen.py`, `capability_contract.py` still expose old Operation-centric model | DESIGN; NOT IMPLEMENTED |
| HostBusinessCorrelation | ca-runtime PR #176 RuntimeHost.v2 narrow contract | DESIGN FROZEN; v2 schema / persistence NOT IMPLEMENTED |
| Host-managed binary artifacts | ca-agent-demo PR #211 | DESIGN DRAFT; HostArtifactRef implementation NOT CLAIMED |
| Inbound SBI event Wait / Integration ingress | ca-skills PR #61/#62; ca-agent-demo PR #210/#213 | v1alpha5 Preview integration design; not a Client Tools Provider responsibility |

## 2. Canonical public naming

The Host-owned [public naming contract](https://github.com/odango-chan/ca-agent-demo/blob/docs/v1alpha4-callable-runtime-contract-20261117/docs/design/public-callable-naming-contract-v1.md) applies to all public ToolCatalog Callable IDs.

- **Generic:** `smf.session.context.query`
- **MAS-specific:** `smf.mas.{sc,pa,upn,ssm,iam,ups,ca,saa}.context.query`
- **Private Provider:** `openamsm.smf.mas.{sc,pa,upn,ssm,iam,ups,ca,saa}.query`

Public IDs use the Host-owned NF-first semantic grammar: `<nf>.<resource-path>.<action>` or, for an explicitly published interface/projection, `<nf>.<interface>.<projection>.<resource-path>.<action>`. The `.context.query` suffix is used consistently for this SMF context-query family. `context.smf.session.read` is retired and MUST NOT become a compatibility alias. The generic and MAS-SC public callables share one private SC operation with distinct Host output normalization. Future 5GC IDs in the Host contract are naming candidates only, not implementation claims. No 3GPP OpenAPI `operationId`, Capability ID, evidence guarantee ID, package alias, Schema ID or backend operation ID is automatically renamed.

## 3. Work packages and dependencies

| WP | Repository / owner | Deliverable | State / gate | Priority |
|---|---|---|---|---|
| WP0 | ca-client-tools / ca-agent-demo / ca-skills | Enforce naming, remove old public ID references, exact-link negative fixtures | Design commits present; implementation tests still required | P0 |
| WP1 | ca-client-tools | One resident `provider serve`, strict NDJSON framing, initialized catalog, invoke/cancel, correlation, limits, safe errors, process drain | Contract in PR #5; implementation NOT_STARTED | P0 |
| WP2 | ca-client-tools | Eight private SMF handlers mapped to pinned source `operationId`, reusing `internal/openamsm/smf/mas.Client.Query` with 404/200/error validation | Contract in PR #5; implementation NOT_STARTED | P0 |
| WP3 | ca-runtime | RuntimeHost.v2 opaque HostBusinessCorrelation, fingerprint, immutable Run metadata, `run.started` digest; v1 unchanged | Narrow v2 contract in PR #176; implementation NOT_STARTED | P0 |
| WP4 | ca-agent-demo | Host resident process manager; trusted backend mapping; v1alpha5 inherited Observation stage, Kind-aware Capability/Scope checks and dispatch | PR #208/#214 design; implementation NOT_STARTED | P0 |
| WP5 | ca-agent-demo + ca-runtime + ca-client-tools | Real diagnostic Host→Runtime→Provider→OpenAMSM query; nine public callables to eight private handlers; negative Scope/error/lifecycle tests | NOT_RUN | P0 |
| WP6 | ca-client-tools | Migrate typed MAS client to shared `internal/sbi` transport with strict parity tests; preserve h2c/token/echo/ProblemDetails/cancellation | Shared transport and typed client separately implemented; migration NOT_DONE; required before general Operation Catalog onboarding, not a first-slice blocker | P1 |
| WP7 | ca-client-tools | General pinned 3GPP OpenAPI Operation Catalog loader and trusted Backend Binding validation; READ/MUTATION typed dispatch foundation | Architecture defined; general source/Schema/runtime not frozen at field level; do not invent external wire formats | P1 / gated |
| WP8 | ca-client-tools | First source-verified outbound SBI **Mutation** operation using existing Request Executor/HTTP2 and declared Effect, no arbitrary URL/Method; preserve Fabric journal/authorization | NOT_IMPLEMENTED; requires exact pinned operation and Owner/deployment contract | P1 / gated |
| WP9 | ca-agent-demo + ca-client-tools | HostArtifactRef + streamed JSON/binary/multipart Provider payload extension; reuse merged Payload Engine, no Base64 in NDJSON | Draft design PR #211; protocol v1 intentionally JSON-only | P1 / separately versioned |
| WP10 | ca-client-tools + ca-agent-demo | Rehome reusable NAS/NGAP codecs and N1/N2 SBI transfer builders as typed operations; preserve Fabric mutation journal/sender authority | Migration design PR #211 and ca-provider-5gc map; not accepted | P2 / gated |
| WP11 | ca-client-tools | NF coverage inventory for AMF, SMF, UDM, PCF, NSSF and other required SBI services; map source implementations to typed operations without implementing duplicate transports | AMF/SMF CLI baseline exists; not all NF operations catalogued | P1 audit; selective P2 code |
| WP12 | ca-agent-demo / ca-runtime / ca-skills | Node-entry Event Wait, Integration IngressCatalog, durable ingress/response obligation and same-Run continuation | PR #61/#62/#210/#213 designs; separate language/version and protocol owner | P2 / NOT current first slice |
| WP13 | ca-client-tools + deployment | Future TLS/mTLS, OAuth2 and NRF discovery profiles behind trusted config | Current OpenAMSM h2c/token profile sufficient; future implementation | P2 / deployment-dependent |
| WP14 | ca-agent-demo + ca-client-tools | Remove legacy ca-provider-5gc generic-query/adapter code **only after** exact Host/Runtime Provider parity passes | Conditional; no early deletion | P1 cleanup |
| WP15 | All relevant repos | README, docs, traceability, local test matrix, source pins and implementation evidence | Continuous; documentation must reflect actual code, not claim work done | P0 supporting |

## 4. Independent overnight execution lanes

These lanes can be implemented in separate Draft PRs; avoid pushing into existing document PRs except for a deliberate contract correction.

**Lane A — Provider core (WP1→WP2).** Start with strict command/protocol parser and fake local typed backend tests; then use the existing SMF typed client. Do not implement a new HTTP client, introduce JSON-only as an architectural restriction, or redefine public ToolCatalog IDs inside the Provider.

**Lane B — RuntimeHost.v2 (WP3).** Build schema, validators, request replay fingerprint, immutable RunRecord and event digest; keep RuntimeHost.v1 unchanged. This is synchronous metadata flow, not Temporal or pending-operation continuation.

**Lane C — Host integration (WP4).** Implement the inherited Observation stage inside the v1alpha5 Procedure Node; PR #214 is only the v1alpha4-scoped design reference. Both Operation and Observation share admitted Callable identity and authorization infrastructure while preserving Kind/Effect differences. Host owns public ToolCatalog, normalized Schema, Owner Outcome/Evidence and trusted adapter mapping. Provider `catalog` takes private backend IDs only.

**Lane D — Source-verified SBI follow-on (WP6, WP7, WP11).** After lanes A–C pass local unit checks, converge the existing typed MAS client on the shared `internal/sbi` transport with exact parity tests; this must precede general 3GPP Operation Catalog onboarding. In parallel, prepare the field-level Operation Catalog contract and source inventory. Implement only field-complete, source-pinned work; produce a concrete BLOCKED record for missing Operation Catalog/Owner/Scope contract rather than guessing.

**Lane E — HostArtifactRef / mutations (WP8–WP10).** The library-level Payload Engine is already merged and must not be rewritten. Prepare fixtures, source-verified operation manifests and staged file security tests from PR #211. Do not wire new binary fields into Provider Protocol v1; a separate versioned payload extension and Host-controlled file lifecycle are required. Do not enable a live mutation without explicit Effect/authorization/reconcile controls and deployment evidence.

**Lane F — Event Wait/Ingress (WP12).** Track the v1alpha5 Appendix L / IngressCatalog contract gap across Compiler, Host, Runtime and Integration. Do not implement ingress delivery inside ca-client-tools; the Provider remains an outbound backend. Do not claim production Owner qualification from draft contracts or synthetic fixtures.

## 5. Required acceptance boundaries

### Implementation acceptance (eligible to PASS after code and real test evidence)

- `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt`/diff checks for ca-client-tools, plus exact Provider protocol and backend tests.
- RuntimeHost.v1/v2 schema/fixture tests, request replay conflict, persistence/restart and digest tests, existing full local ca-runtime verification.
- ca-agent-demo v1alpha5 inherited Observation semantics and negative authorization/Scope tests; first Host process lifecycle and Provider E2E.
- Real OpenAMSM diagnostic query through the **actual** admitted Host/Runtime/Provider path, not merely CLI or mocked dispatch.
- Generic 404 SESSION_NOT_FOUND → ABSENT, 200 UNAVAILABLE → PRESENT, MAS 404 → declared Owner failure; invalid wire/transport failures remain UNKNOWN.
- Multi-UE/PSI/lifecycle mismatches fail closed; HostBusinessCorrelation never leaks into Provider Protocol v1 or Skill inputs.

When all pass, report `PROVIDER_HOST_DISPATCH_IMPLEMENTATION_PASS`. GitHub Actions being unavailable because of billing/limits is `CI_UNAVAILABLE`, neither CI PASS nor a code failure. Local acceptance must include exact commands/output and environment constraints.

### Product qualification — separate, currently blocked

- Public Host ToolCatalog and deployment CapabilityContracts must be real, trusted publications, not the `docs/` proposals or test-only fixtures.
- SMF v0.10 response lacks source `observedAt` and no accepted live-authoritative-read freshness guarantee has been published. Never invent source time, `maxAgeMs` or production Evidence Descriptors.
- Real v1alpha5 Skill linking, Evidence-dependent Predicate PASS and product qualification remain `NOT_QUALIFIED` until those explicit gates close.

### No-go activities

Do not force-push shared branches, push directly to `main`, treat v1alpha4 as the next target, claim v1alpha5 release/qualification, publish synthetic Evidence, spawn the CLI per Tool invocation, duplicate the HTTP2 Client/Payload Engine, create a generic Agent-facing curl capability, bypass Fabric authority, or silently merge existing PRs.

## 6. Harness stop / continue rules

- Work autonomously through independent lanes while a separate lane is blocked; never mark the entire Goal complete because a single repository passed unit tests.
- Before each implementation PR, check current `main`, `AGENTS.md`, applicable existing PRs and conflict risk. Create one implementation Draft PR per repository. Do not overwrite concurrently edited document branches.
- If a required field-level contract is missing, stop **that affected lane only**, write exact missing field/owner/source and a BLOCKED report, and continue other independent lanes. Do not silently invent a protocol.
- Exhaust meaningful local test, negative-test, build and traceability work; make no CI rerun loops when Actions are billing-blocked.
- Final report distinguishes `IMPLEMENTED`, `LOCAL_ACCEPTANCE_PASS`, `REAL_E2E_PASS`, `BLOCKED`, `NOT_STARTED`, `NOT_QUALIFIED` and `CI_UNAVAILABLE` per work package. Every completed claim requires commit, PR and reproducible evidence.

## 7. Cross-repository contract references

- ca-client-tools PR #5: Provider Process Protocol v1, Backend Binding Catalog, SMF backend contract and the general SBI Client architecture.
- ca-agent-demo PR #208: Host Business Context, SUPI/PSI, generic and MAS Observation ToolCatalog proposals.
- ca-agent-demo PR #214: v1alpha4-scoped inherited Operation/Observation Host design reference; v1alpha5 Host admission and Wait integration still require explicit Host-owned contract.
- ca-runtime PR #176: synchronous RuntimeHost.v2 HostBusinessCorrelation sub-contract; asynchronous continuity proposal separately.
- ca-agent-demo PR #211: HostArtifactRef binary lifecycle **design**, not implemented.
- ca-agent-demo PR #210/#213 and ca-skills PR #61/#62: Node-entry Event Wait / Integration Ingress are v1alpha5 Preview design inputs; Host/Runtime/Integration implementation and qualification remain open.
- ca-skills PR #58: v1alpha4 PDU/MAS examples are historical/inherited-baseline fixtures, not the next-version sample target; the v1alpha5 sample work is visible in Studio PR #199 and must be checked against the current #61/#62 heads before use.

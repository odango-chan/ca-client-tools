# CLI v1 implementation and validation record

Status: **IMPLEMENTED — LOCAL AND REAL E2E PASS (post-fix, authorized local build overlay)**
Repository: `odango-chan/ca-client-tools`
Branch: `feat/openamsm-client-tools-foundation-20261116`
Normative contract: [`docs/cli-contract.md`](cli-contract.md)
NF capability baseline: [`docs/nf-capability-map.md`](nf-capability-map.md)

This document records the current implementation state and remaining
acceptance work. It is not a request to redesign or delegate the CLI.

## Implemented command surface

```text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
```

The SMF context query accepts one bounded JSON request object on stdin.
Public output exposes stable CA-facing `type`, `status`, `key` and
`context`. OpenAMSM internal route identifiers and wire `masId` remain
inside the private SMF adapter; they are validated but not exposed in the
public CLI result.

The SMF adapter now uses cleartext HTTP/2 prior knowledge (h2c), matching the
real SBI listener, and binds the echoed response key to the request: SUPI,
PSI and any supplied UE IP must match. A mismatch is a Protocol Error, never
`UNAVAILABLE`.

Tests cover help/version, all eight context routes, SUPI + PSI, IMSI + PSI,
IPv4, IPv6, joint lookup, cross-UE and cross-session responses,
AVAILABLE/UNAVAILABLE, 4xx/5xx, timeout, redirect, response size, malformed
JSON, duplicate keys, sensitive-value redaction, AMF UE/gNB and SMF PDU lists,
and cancellation.

## NF scope

This section records the current CLI v1 implementation scope only. It is not
a permanent limitation on the generic SBI Client architecture described in
the follow-on workstream below.

See [`docs/nf-capability-map.md`](nf-capability-map.md) for the source-backed
NF capability map.

- AMF: `GET /ue-info` and `GET /gnb-info`; Host-internal by default.
- SMF: `GET /pdu-info` plus eight read-only context-query routes; Agent
  exposure only through explicit ToolCatalog/Capability admission.
- UDM/AUSF/PCF/NRF/CHF and the other adjacent NFs: deferred by current scope.

## Architecture boundaries

- `env-openamsm`: OpenAMSM source, build, deployment and NF lifecycle.
- `ca-client-tools`: typed OpenAMSM clients and fixed local CLI; the
  follow-on SBI Client/Operation Catalog/Tool Provider architecture is
  documented separately and is not implemented in this CLI v1 workstream.
- `ca-agent`: Host integration, trusted configuration and exposure policy.
- `ca-runtime`: callable admission, Capability authorization, execution,
  Outcome, Evidence and Reconcile.
- `ca-skills`: business Procedures and declared callable dependencies.

This CLI v1 workstream does not implement Host/Runtime integration, the
follow-on SBI mutation-operation engine, NW Agent workflow
planning/facade/orchestration/relay, Open5GS compatibility, or additional NF
clients beyond the existing SMF baseline and AMF read scope.

## Validation results

| Check | Result |
|---|---|
| `gofmt -w .` (Linux/WSL and Windows native) | PASS |
| `go test ./...` (Go 1.26.8 Linux/WSL, Go 1.26.6 Windows native) | PASS |
| `go vet ./...` (Linux/WSL and Windows native) | PASS |
| `go build ./...` + native `ca-client-tools.exe` (Linux/WSL and Windows) | PASS |
| Compiled-CLI fixture acceptance (29 cases) | PASS |
| Windows-native live CLI acceptance vs WSL 5GC (23 cases) | PASS (23/23 strict JSON assertions) |
| GitHub Actions | CI_UNAVAILABLE (job never started; billing/spending-limit annotation) |

Real OpenAMSM deployment (pinned snapshot, verified tree hash, 11 NFs,
UERANSIM UE with an active PDU session):

| Surface | Result |
|---|---|
| AMF UE List | PASS |
| AMF gNB List | PASS |
| SMF PDU List | PASS |
| Eight SMF Context queries | PASS (all AVAILABLE, identity match) |

The pre-fix run returned HTTP 403 `FORBIDDEN` because the handler looked up
`X-OpenAMSM-Query-Token` in a case-sensitive hash while HTTP/2 requires
lowercase field names. With operator authorization, a local `env-openamsm`
build overlay added the same lowercase lookup already used by the
orchestration handler. The imported upstream tree remains unchanged; no
`ca-client-tools` code change was needed.

Real post-fix checks:

- all eight context types: exit 0, `AVAILABLE`, echoed SUPI/PSI match;
- IMSI + PSI: exit 0, `AVAILABLE`, echoed `supi=imsi-999700000000001`, PSI 1;
- IPv4-only and SUPI + PSI + IPv4: exit 0, echoed `ueIpv4=10.45.0.2`;
- wrong PSI/SUPI: exit 4, HTTP 404 `SESSION_NOT_FOUND`;
- unallocated UE IP: exit 4, HTTP 409 `KEY_CONFLICT`.

Evidence:
[real acceptance](evidence/real-openamsm-acceptance-20261117.md).

Remaining work:

1. Adopt the lowercase token lookup in the `env-openamsm` release.
2. Keep CI_UNAVAILABLE recorded until a usable Actions run exists.
3. UE-A/UE-B rejection remains covered by client contract tests; a live
   fault-injection rehearsal would require a separately approved test setup.
4. Keep the Windows-native + WSL acceptance record current when the lab or the
   forwarder topology is rebuilt; see
   [Windows/WSL acceptance](evidence/windows-wsl-acceptance-20261117.md).

## Source authority

OpenAMSM snapshot:
[`594dec78b95bff4fdc34deac8a3a9681fb41228f`](https://github.com/odango-chan/env-openamsm/tree/594dec78b95bff4fdc34deac8a3a9681fb41228f).

See [`docs/openamsm-interface-inventory.md`](openamsm-interface-inventory.md)
and [`docs/architecture.md`](architecture.md) for source paths, request and
response semantics, and exclusions.

PR #1 now meets the stated delivery criterion in the local lab: known SUPI +
PSI directly returns all eight SMF contexts and the echoed identity matches.
The server-side overlay must be adopted by `env-openamsm` before another
deployment can reproduce the result. CI remains unavailable.


## SBI Client v1 — foundation merged; Provider runtime pending

The normative architecture is
[docs/sbi-client-architecture.md](sbi-client-architecture.md). The current
repository state is:

- PR #2 merged: shared h2c HTTP/2 transport and trusted OpenAMSM token-header
  profile (`189deff`).
- PR #3 merged: streaming JSON, opaque binary and multipart/related Payload
  Engine plus Host-staged file boundary (`4c424ee`). Windows-native and
  WSL/Linux local acceptance PASS. No catalogued live binary operation has
  been run through the Provider yet.
- PR #5 is open and Draft. It freezes the private SMF backend binding set and
  Provider Process Protocol v1 for the first JSON-only SMF Observation slice.
  It does not implement the resident Provider or Host dispatch.
- The current CLI v1 remains read-only. That does not restrict the general
  SBI architecture, which supports individually registered READ and MUTATION
  operations with explicit authorization and retry/idempotency policy.
- The general 3GPP Operation Catalog loader and resident Provider runtime
  remain unimplemented. GitHub Actions remains CI_UNAVAILABLE for the
  previously recorded account billing/spending-limit reason.

Implementation order:

1. Implement the trusted Provider Backend Binding Catalog loader and the
   resident `provider serve` process using the frozen JSON-only Provider
   Process Protocol v1. The first vertical slice is the generic SC-backed
   session read plus the eight optional projection-specific SMF queries.
2. Reuse `internal/openamsm/smf/mas.Client.Query` directly. Do not spawn the
   CLI per invocation, duplicate the SMF HTTP client, or expose private
   OpenAMSM routes/MAS selectors to Skill source.
3. Complete the Host adapter and Host/Runtime dispatch in ca-agent-demo.
   The Host maps its admitted public callable to one exact private backend
   operation; ToolCatalog, CapabilityContracts and Provider bindings remain
   separate authorities. Verify Scope, output normalization and the
   SESSION_NOT_FOUND versus UNAVAILABLE distinction.
4. Preserve the protocol-v1 JSON-only boundary for this first SMF slice.
   Do not retrofit binary fields into the frozen v1 envelope.
5. Define a separately versioned Provider payload-transfer extension for
   binary operations. It must pass Host-generated relative file identifiers
   over invocation-scoped HostArtifactRef staging, stream request and response
   attachments, and reuse the already merged Payload Engine. Never Base64
   binary payloads into Agent-visible JSON.
6. Add the first source-verified binary/multipart SBI operation only after
   its pinned 3GPP OpenAPI operation and selected OpenAMSM source/deployment
   support are verified. TS 29.502 §6.1.2.4 explicitly defines multipart
   transfer of N1/N2 SM payloads and N4 information; that standard support
   does not by itself prove deployment implementation.
7. Add individual READ and MUTATION operations only through trusted bindings,
   per-operation source/fixture/live evidence and Host/Runtime authorization.
   Mutations are not automatically retried; require explicit idempotency or
   reconciliation semantics.
8. Keep any curl-like raw request mode operator-only, disabled by default,
   and outside the Agent Tool Provider. It is not part of CLI v1.

Do not implement Host/Runtime authorization, Skill selection, Artifact Store,
pending-operation lifecycle, callbacks or Procedure resumption in this
repository. Those remain ca-agent / ca-runtime responsibilities.


## Next implementation handoff — Provider + Host SMF Observation slice

The ownership split is normative. Do not collapse the Host Tool Owner and
the Go Provider backend:

1. ca-agent-host owns the public generic callable
   context.smf.session.read and the eight optional
   smf.mas.<type>.context.query Observation callables. The generic callable name omits `mas` and `openamsm`; only projection-specific public names include `mas`, while `openamsm` is reserved for private backend IDs. See
   [ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208).
2. ca-client-tools owns the typed Go backend and resident process for all SMF context-query callables. The generic `context.smf.session.read` and MAS-specific `smf.mas.sc.context.query` callables share the private `openamsm.smf.mas.sc.query` backend operation but retain separate Host output/normalization contracts. `ca-provider-5gc` is a transitional migration source, not a parallel query backend. Use
   [Provider Process Protocol v1](provider-protocol-v1.md),
   [OpenAMSM SMF Query Backend v1](docs/smf-query-backend-contract-v1.md),
   and contracts/provider-bindings/v1/provider-bindings.yaml.
3. Host/Runtime integration, unified callable dispatch, Scope and code-entrypoint
   migration are tracked in
   [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).
4. HostArtifactRef in PR #211 is separate and not a first-slice dependency.
5. CA-SL v1alpha4 remains FROZEN. Do not modify language semantics.

Implementation order: Provider process/handshake; eight private backend
handlers reusing internal/openamsm/smf/mas.Client.Query; Host resident process
manager and adapter binding; Host-owned generic and projection-specific
Observation stages; positive/negative Scope and wire-binding tests; real
Host-dispatch query acceptance.

The generic Host session read (ca-client-tools backend `openamsm.smf.mas.sc.query`) maps only exact HTTP 404 + SESSION_NOT_FOUND to ABSENT. HTTP 200 UNAVAILABLE is PRESENT with an empty projection. The MAS-specific projection callables, including the SC callable sharing this backend operation, must not map 404 to UNAVAILABLE. Provider returns
typed remote HTTP results, not CA-SL Owner Outcomes.

Do not publish ToolCatalog or Evidence Descriptor files from ca-client-tools.
Do not invoke the CLI per call, duplicate the SMF client, route Observation
through ObservationSource.acquire(), accept direct callable IDs as grants,
or add binary/HostArtifactRef/NAS/NGAP work to the first slice.

The v0.10 response has no source observedAt. Do not invent freshness or claim
Evidence-dependent PASS. Provider transport can be implemented while the
Host Evidence descriptors remain NOT_PUBLISHED / NOT_QUALIFIED.

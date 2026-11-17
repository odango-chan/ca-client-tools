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


## Next workstream — SBI Client v1 (implemented; PR #2 merged, PR #3 final v1 slice)

The normative design is in
[docs/sbi-client-architecture.md](sbi-client-architecture.md). The h2c
transport and trusted OpenAMSM token-header profile are merged to main via
PR #2 (`189deff`). The JSON/binary/multipart Payload Engine, Host-staged file
boundary and strict acceptance tooling are implemented on PR #3, the final v1
slice, at validation head `0b80d33`. Windows-native (Go 1.26.6) and WSL/Linux
(Go 1.26.8) format/test/vet/build PASS, and the strict Windows-native E2E
runner passes 23/23 on the real deployment. The transport and Payload Engine
are not accepted as a live operation until the Operation Catalog/Tool Provider
wires them to a real SBI operation. CI remains unavailable: the GitHub Actions
job never started because of an account billing/spending-limit annotation.
Custom TLS/mTLS and OAuth2 client support are explicitly excluded. See
[docs/evidence/windows-wsl-acceptance-20261117.md](evidence/windows-wsl-acceptance-20261117.md).

Continue in this order:

1. ~~Validate PR #2 h2c negotiation, HTTP/1 rejection, connection reuse,
   cancellation, bounded I/O and trusted token-header isolation.~~ DONE on
   Windows-native and WSL/Linux runs.
2. ~~Validate PR #3 JSON, binary and multipart/related handling, Content-ID /
   RefToBinaryData binding, malformed parts, size limits, cancellation and
   Host-root confinement.~~ DONE, including duplicate-key rejection, KindNone
   body probing, limit composition and real Linux symlink rejection.
3. Migrate the existing SMF context-query adapter to the shared transport only
   after exact regression tests preserve its private token, identity, status
   and ProblemDetails behavior. Until then, retain the current typed adapter.
4. Implement the trusted Operation Catalog derived from pinned release-specific
   3GPP OpenAPI plus small CA metadata. Do not duplicate method/path/schema
   where OpenAPI already defines them. Keep OpenAMSM-private APIs separately named.
5. Implement the CA-Agent Tool Provider catalog/invoke subprocess contract.
   The Provider is a preinstalled Go executable; the Host dynamically activates
   only trusted operations required by the active Skill and resolves BinaryRef
   values into private per-invocation input/output directories.
6. Keep read and mutation operations at individual operation granularity.
   Mutations are not automatically retried; require an explicit
   idempotency/reconciliation contract and Host/Runtime permission.
7. Add a first multipart operation only after its pinned 3GPP OpenAPI and
   selected OpenAMSM source/deployment support are verified.
8. Keep any curl-like raw request mode operator-only, disabled by default and
   outside the Agent Tool Provider. It is not part of CLI v1.

Do not implement Host/Runtime authorization, Skill selection, Artifact Store,
pending-operation lifecycle, callbacks or Procedure resumption in this
repository. Those remain ca-agent / ca-runtime responsibilities.


## Next implementation handoff — Provider + Host SMF Observation slice

Use these documents as the implementation contract set; do not ask the
implementation agent to invent protocol fields or authorization semantics:

1. `docs/provider-protocol-v1.md` — field-level resident NDJSON process protocol.
2. `docs/smf-context-observation-contract-v1.md` and
   `contracts/smf-context/v1alpha4/` — eight exact Observation exports,
   schemas, Evidence guarantees, private route mapping and status semantics.
3. [ca-agent-demo PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214)
   — Host v1alpha4 Observation stage, unified callable dispatch, Capability/
   Scope admission and exact code-entrypoint changes.
4. [ca-agent-demo PR #211](https://github.com/odango-chan/ca-agent-demo/pull/211)
   — separate HostArtifactRef draft; not a dependency of this JSON-only slice.
5. CA-SL v1alpha4 remains FROZEN. Do not modify the language or invent compatibility.

Implementation order: Provider process/handshake; typed Provider handlers
reusing `internal/openamsm/smf/mas.Client.Query`; Host resident process manager
and callable dispatch adapter; v1alpha4 Observation stage, CapabilityContracts
and pure Predicate evaluation; real Host-dispatch E2E and negative tests.

Do not invoke the CLI per call, duplicate the SMF client, route Observation
through `ObservationSource.acquire()`, accept direct callable IDs as grants,
or add binary/HostArtifactRef/NAS/NGAP work to the first slice. This is a
documentation-only handoff; no Host E2E is claimed.

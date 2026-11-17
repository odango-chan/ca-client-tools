# CLI v1 implementation and validation record

Status: **IMPLEMENTED — LOCAL ACCEPTANCE PASS; REAL CONTEXT E2E BLOCKED**
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
| `gofmt -w .` | PASS |
| `go test ./...` (Go 1.26.8) | PASS |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| Compiled-CLI fixture acceptance (29 cases) | PASS |
| GitHub Actions | CI_UNAVAILABLE |

Real OpenAMSM deployment (pinned snapshot, verified tree hash, 11 NFs,
UERANSIM UE with an active PDU session):

| Surface | Result |
|---|---|
| AMF UE List | PASS |
| AMF gNB List | PASS |
| SMF PDU List | PASS |
| Eight SMF Context queries | BLOCKED |

The eight context types return HTTP 403 `FORBIDDEN` from both the compiled CLI
and a direct h2c `curl` call. The token was confirmed loaded in the running SMF
(`gdb`: `num_allow_tokens=1`, matching value). Root cause: the handler looks up
`X-OpenAMSM-Query-Token` in a case-sensitive header hash, but HTTP/2 requires
lowercase field names; a literal mixed-case field name is rejected by nghttp2
with `GOAWAY PROTOCOL_ERROR`. No compliant client can satisfy the check. No
server patch was applied. Evidence:
[real acceptance](evidence/real-openamsm-acceptance-20261117.md).

Remaining work:

1. Fix the token header lookup in a deployed OpenAMSM revision (owner:
   `env-openamsm`; not this repository).
2. Rerun the eight context types with a known SUPI + PSI and record live
   `AVAILABLE` / `UNAVAILABLE`, echoed-key binding and CLI exit codes.
3. Prove a live UE-A request / UE-B response rejection.
4. Obtain a usable CI run, or keep `CI_UNAVAILABLE` recorded.

## Source authority

OpenAMSM snapshot:
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).

See [`docs/openamsm-interface-inventory.md`](openamsm-interface-inventory.md)
and [`docs/architecture.md`](architecture.md) for source paths, request and
response semantics, and exclusions.

PR #1 does not meet the stated final delivery criterion while live context
queries are blocked. Keep it open until the upstream lookup is fixed and the
eight contexts are accepted, unless the reviewer explicitly narrows delivery
to the list surfaces plus client-side contract evidence.


## Next workstream — SBI Client v1 (implementation staged; validation pending)

The normative design is in
[docs/sbi-client-architecture.md](sbi-client-architecture.md). The h2c
transport and trusted OpenAMSM token-header profile are implemented in stacked
PR #2. The JSON/binary/multipart Payload Engine and Host-staged file boundary
are implemented in stacked PR #3. Neither is accepted until Go 1.26.8 local
acceptance or usable CI evidence is available. Custom TLS/mTLS and OAuth2
client support are explicitly excluded.

Continue in this order:

1. Validate PR #2 h2c negotiation, HTTP/1 rejection, connection reuse,
   cancellation, bounded I/O and trusted token-header isolation.
2. Validate PR #3 JSON, binary and multipart/related handling, Content-ID /
   RefToBinaryData binding, malformed parts, size limits, cancellation and
   Host-root confinement.
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

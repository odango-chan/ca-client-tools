# CLI v1 implementation and validation record

Status: **IMPLEMENTED ON REVIEW BRANCH — VALIDATION PENDING**
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

Fixture coverage is authored for help/version, context AVAILABLE/UNAVAILABLE/404,
invalid input, sensitive-value redaction, AMF UE/gNB and SMF PDU list
operations, and cancellation of an in-flight context query.
Execution of the latest test set remains pending.

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

## Remaining validation

1. Run `gofmt` on changed Go files.
2. Run `go test ./...`, `go vet ./...` and native build with Go 1.26.2.
3. Obtain a successful current-head GitHub Actions run.
4. Run all eight SMF Context Query public types against the unmodified
   OpenAMSM deployment with the known SUPI + PSI test lookup. AMF/SMF metrics
   lists are supplementary, not prerequisites; follow
   [docs/e2e-acceptance.md](e2e-acceptance.md).
5. For this CLI v1 slice, keep other NF client implementations deferred.
   Future individual SBI operations follow the evidence gate and design in
   docs/sbi-client-architecture.md; do not infer deployment support.

Latest Actions jobs fail before exposing step records or logs. This is
insufficient evidence to attribute the failure to source code or runner
infrastructure. Go 1.26.2 local validation has not been performed in this
environment. Do not mark these checks PASS without new evidence.

## Source authority

OpenAMSM snapshot:
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).

See [`docs/openamsm-interface-inventory.md`](openamsm-interface-inventory.md)
and [`docs/architecture.md`](architecture.md) for source paths, request and
response semantics, and exclusions.

Do not merge PR #1 until validation is complete.


## Next workstream — SBI Client v1 (implementation staged; validation pending)

The normative design is in
[docs/sbi-client-architecture.md](sbi-client-architecture.md). The h2c
transport and trusted OpenAMSM token-header profile are implemented in stacked
PR #2. The JSON/binary/multipart Payload Engine and Host-staged file boundary
are implemented in stacked PR #3. Neither is accepted until Go 1.26.2 local
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

# CLI v1 implementation and validation record

Status: **IMPLEMENTED ON REVIEW BRANCH — VALIDATION PENDING**
Repository: `odango-chan/ca-client-tools`
Branch: `feat/openamsm-client-tools-foundation-20261116`
Normative contract: [`docs/cli-contract.md`](cli-contract.md)
NF capability baseline: [`docs/nf-capability-map.md`](nf-capability-map.md)

This document records the current implementation state and the remaining
acceptance work. It is no longer a request to redesign the CLI or delegate the
implementation to another Harness.

## Implemented command surface

```text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
```

The SMF context query accepts one bounded JSON request object on stdin.
Public output exposes the stable CA-facing `type`, `status`, `key` and
`context`. OpenAMSM's internal route identifiers and wire `masId` remain
inside the private SMF adapter; they are validated but not exposed in the
public CLI result.

The current fixture tests cover:

- CLI help/version and the NF/resource command tree;
- context-query AVAILABLE, UNAVAILABLE and 404 handling;
- invalid stdin, duplicate/unknown fields and sensitive-value redaction;
- AMF UE/gNB and SMF PDU metrics list commands;
- cancellation of an in-flight context query.

## NF scope

The source-backed NF capability and availability map is
[`docs/nf-capability-map.md`](nf-capability-map.md).

- AMF: `GET /ue-info`, `GET /gnb-info`; Host-internal by default.
- SMF: `GET /pdu-info` plus eight read-only context-query routes; Agent exposure
  only through explicit ToolCatalog/Capability admission.
- UPF: no `/session-info` route in the pinned OpenAMSM source; no UPF session
  client or inferred SMF/UPF N4 join.
- UDM/AUSF/PCF/NRF/CHF: responsibilities are mapped, but no client endpoint is
  claimed without a source-backed contract.

## Architecture boundaries

- `env-openamsm`: OpenAMSM source, build, deployment and NF lifecycle.
- `ca-client-tools`: typed OpenAMSM clients and fixed local CLI.
- `ca-agent`: Host integration, trusted configuration and exposure policy.
- `ca-runtime`: callable admission, Capability authorization, execution,
  Outcome, Evidence and Reconcile.
- `ca-skills`: business Procedures and declared callable dependencies.

This repository does not implement Host/Runtime integration, NF mutation,
NW Agent workflow planning/facade/orchestration/relay, Open5GS compatibility,
or speculative UPF inspection.

## Remaining validation

1. Run `gofmt` on changed Go files.
2. Run `go test ./...`, `go vet ./...` and native build with the declared
   Go 1.26.2 toolchain.
3. Obtain a successful current-head GitHub Actions run.
4. Separately run the pinned OpenAMSM native build and context-query source
   tests; then perform live-NF acceptance.

Latest Actions jobs currently fail before exposing step records or logs.
This is not enough evidence to attribute the failure to source code or runner
infrastructure. No Go 1.26.2 local validation has been performed in this
environment. Do not mark those checks PASS without new evidence.

## Source authority

OpenAMSM snapshot:
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb).

See [`docs/openamsm-interface-inventory.md`](openamsm-interface-inventory.md)
and [`docs/architecture.md`](architecture.md) for source paths, request and
response semantics, and exclusions.

Do not merge PR #1 until the validation above is complete.

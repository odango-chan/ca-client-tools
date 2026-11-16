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
ca-client-tools upf session list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
```

The SMF context query accepts one bounded JSON request object on stdin.
Public output exposes stable CA-facing `type`, `status`, `key` and
`context`. OpenAMSM internal route identifiers and wire `masId` remain
inside the private SMF adapter; they are validated but not exposed in the
public CLI result.

Fixture coverage is authored for help/version, context AVAILABLE/UNAVAILABLE/404,
invalid input, sensitive-value redaction, AMF UE/gNB, SMF PDU and UPF PFCP
session list operations, and cancellation of an in-flight context query.
Execution of the latest test set remains pending.

## NF scope

See [`docs/nf-capability-map.md`](nf-capability-map.md) for the source-backed
NF capability map.

- AMF: `GET /ue-info` and `GET /gnb-info`; Host-internal by default.
- SMF: `GET /pdu-info` plus eight read-only context-query routes; Agent
  exposure only through explicit ToolCatalog/Capability admission.
- UPF: the immutable source has no `/session-info` route. A CA-owned
  `env-openamsm` patch overlay adds the read-only PFCP session projection;
  the CLI/client and fixture coverage are staged. Patch application, native
  build and live-N4 acceptance remain pending. The projection reads the exact
  SMF F-SEID and UPF N4 SEID from the same UPF session; no inferred join is used.
- UDM/AUSF/PCF/NRF/CHF and the other adjacent NFs: deferred by current scope.

## Architecture boundaries

- `env-openamsm`: OpenAMSM source, build, deployment and NF lifecycle.
- `ca-client-tools`: typed OpenAMSM clients and fixed local CLI.
- `ca-agent`: Host integration, trusted configuration and exposure policy.
- `ca-runtime`: callable admission, Capability authorization, execution,
  Outcome, Evidence and Reconcile.
- `ca-skills`: business Procedures and declared callable dependencies.

This repository does not implement Host/Runtime integration, NF mutation,
NW Agent workflow planning/facade/orchestration/relay, Open5GS compatibility,
or additional NF clients beyond the existing SMF baseline and AMF/UPF read
scope. The UPF source patch is maintained in `env-openamsm`, not here.

## Remaining validation

1. Run `gofmt` on changed Go files.
2. Run `go test ./...`, `go vet ./...` and native build with Go 1.26.2.
3. Obtain a successful current-head GitHub Actions run.
4. Apply the `env-openamsm` UPF patch overlay to a disposable source copy and
   run the OpenAMSM native build.
5. Verify the UPF `/session-info` response against a real PFCP session,
   including both N4 identities and the PDR/FAR/QER/URR projection.
6. Keep all other NF client work deferred.

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

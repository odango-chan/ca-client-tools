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
4. Run the existing AMF and SMF queries against the unmodified OpenAMSM
   deployment and record their real response acceptance.
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


## Next workstream — SBI Client v1 (design accepted, not implemented)

The next architecture work is specified in
[docs/sbi-client-architecture.md](sbi-client-architecture.md). This is a new
workstream after the current CLI v1 baseline; it does not rewrite the current
CLI command tree or claim that the generic SBI engine is already implemented.

Implement in this order:

1. Shared Go HTTP/2 transport with static trusted service configuration,
   h2c HTTP/2, static service configuration and the private OpenAMSM
   query-token header profile. Custom TLS/mTLS and OAuth2 client support are
   excluded; NRF discovery is deferred.
2. Streaming payload contracts: Host-supplied PayloadResolver for request
   BinaryRef values and Host-supplied AttachmentSink for response attachments.
   The Host owns storage, authorization, expiry and cleanup.
3. JSON, single-binary and multipart/related request/response support.
   Follow TS 29.500 clause 5.4 and the applicable service specifications,
   including TS 29.502 and TS 29.518: root JSON first, Content-ID per binary
   part, RefToBinaryData references, multiple operation-defined binary parts
   (including N4 payload combinations) and operation-declared media types.
   For application/vnd.3gpp.pfcp, carry the PFCP application-layer message
   only; do not include IP/UDP headers.
4. Trusted Operation Catalog derived from pinned release-specific 3GPP
   OpenAPI plus small CA metadata. Do not duplicate method/path/schema where
   OpenAPI already defines them. Keep OpenAMSM-private APIs separately named.
5. CA-Agent Tool Provider catalog/invoke subprocess contract. The Provider is
   a preinstalled Go executable; the current Python Demo Host dynamically
   activates only trusted operations required by the active Skill. The Host resolves
   BinaryRef values into private per-invocation input files and ingests
   response attachments from a private output directory. Skill content cannot
   submit arbitrary manifests, URLs, methods, paths, headers or auth settings.
6. Keep read and mutation operations at individual operation granularity.
   Mutations are not automatically retried; require an explicit
   idempotency/reconciliation contract and Host/Runtime permission.
7. Migrate the existing SMF context-query adapter to shared transport only
   after exact regression tests preserve its token, identity, status and
   ProblemDetails behavior.
8. Add a first multipart SBI operation only after its pinned 3GPP OpenAPI and
   selected OpenAMSM source/deployment support are verified.
9. Keep any curl-like raw request mode operator-only, disabled by default and
   outside the Agent Tool Provider. It is not part of CLI v1.

Do not implement Host/Runtime authorization, Skill selection, Artifact Store,
pending-operation lifecycle, callbacks or Procedure resumption in this
repository. Those remain ca-agent / ca-runtime responsibilities.

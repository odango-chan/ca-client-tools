# Changelog

All notable changes to CA Client Tools are documented here. Provider protocol
versioning and the local CLI contract are separate; this changelog records
product releases, not wire-protocol compatibility promises.

## [Unreleased] - Free5GC codec Phase B

### Included

- `protocol/codec`: NAS 5GSM and NGAP/N2 facade over the pinned
  `free5gc/nas v1.3.1` and `free5gc/ngap v1.2.0` modules.
- Explicit status model, raw-byte preservation, ePCO decode-projection-only
  handling, typed NAS `IEToDoList` detection and N2 explicit-root/strict-
  consumption checks.
- Canonical JSON projection for the upstream Go types and a generated type
  registry.
- `ca-client-tools codec worker`: long-lived framed stdin/stdout IPC with raw
  binary segments and no network listener.
- Python `ca_codec_worker` process client with lifecycle, request IDs,
  timeout/cancel and abnormal-exit handling.
- Generated `docs/codec-capabilities.json` coverage manifest.

### Deliberate limitations

- The APER fail-closed patch is not applied in Phase B. Root extension bits are
  `NOT_QUALIFIED`; nested extension additions remain unproven and the raw bytes
  stay authoritative.
- JSON Encode is gated to types with dedicated projection round-trip tests.
- Phase C consumer migration is not part of this change.

## [1.1.0] — Compiler Interface Package and runtime binding

### Included

- Single canonical Go-owned Tool definition: the public Callable ID, Schema
  references, Effect, Owner outcomes, Capability membership and private backend
  binding now live in `contracts/provider-bindings/v2/provider-bindings.yaml`
  with standalone YAML Schema sources.
- Deterministic release generator (`ca-client-tools-release generate|verify`)
  emitting a frozen `connectionagent.io/interface-package/v1alpha1` package
  (`package.json`, `tool-catalog.yaml`, `capabilities.json`, standalone JSON
  Schemas) and the Host runtime binding `provider.yaml`.
- Eleven public READ Tools in two Capabilities (`openamsm.smf.mas.context.read`,
  `openamsm.nf.metrics.read`); linkable by `ca-skill-compiler v2.1.0`.
- New release asset `ca-client-tools_interface_<version>.zip`. Every platform
  archive now contains `provider.yaml` and the byte-identical `interface/` tree.
- Read-only Owner durable receipt and exact-original reconcile Provider handlers
  (`openamsm.smf.mas.receipt.query`, `openamsm.smf.mas.reconcile`).
- `providerVersion` is injected from the release tag and agrees with
  `provider.yaml` and the running binary's `initialize` response.

### Deliberate limitations

- The Interface Package exports no MUTATION Tool: the Owner publishes no
  receipt/commit-proof/reconcile contract for a mutation binding, so a mutation
  export would require an invented receipt. The typed MAS mutation callables
  remain runtime `Development` bindings and are not production-qualified.
- Host-side generic package loading remains `ca-agent-demo#241`; Skill Studio
  ZIP import remains `ca-skill-studio-demo#261`.
- Production genericSBI mutation dispatch stays disabled; no production
  genericSBI binding is shipped.
- Real-NF multipart remains unverified.

## [1.0.0] — first stable release

This release baseline is limited to the eleven production read bindings and
the verified Provider/Host integration scope below.

### Included

- One resident Provider process protocol: `ca-client-tools.provider/v2`.
- One embedded trusted Provider Catalog with eleven typed OpenAMSM read
  bindings.
- Reusable SBI transport with explicit HTTP/2 prior-knowledge and metrics
  HTTP/1.1 profiles.
- Typed AMF/SMF read clients and the non-interactive CLI v1 process contract.
- Generic SBI executor and JSON/binary/multipart Payload Engine foundations.
- Host-owned Artifact Store reference and streaming integration under the
  pinned `ca-host.artifact-store/v1` contract.
- Windows amd64 and Linux amd64/arm64 release build targets.

### Deliberate limitations

- Production Mutation dispatch remains disabled.
- No production genericSBI operation is shipped in the embedded Catalog.
- Real-NF multipart remains unverified because the pinned deployment exposes no
  source-verified deployed multipart operation.
- Issue #8 remains a prerequisite before adding artifact-bearing production
  genericSBI bindings with mixed or otherwise non-projectable response media.
- Runtime admission, Capability authorization and business Outcome/Evidence
  remain owned by Host/Runtime, not this repository.

## [Unreleased]

Changes after the first stable release will be recorded here.

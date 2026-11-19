# Changelog

All notable changes to CA Client Tools are documented here. Provider protocol
versioning and the local CLI contract are separate; this changelog records
product releases, not wire-protocol compatibility promises.

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

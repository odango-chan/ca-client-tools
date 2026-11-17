# Provider Payload Extension v1 — design proposal (binary / multipart)

**Status:** **DESIGN PROPOSAL — NOT IMPLEMENTED, NOT ENABLED, NOT ACCEPTED.**
This document is not a protocol version that any Host can use today.

**Implemented libraries it builds on:** the JSON/binary/multipart Payload Engine
(`internal/sbi/payload`, PR #3), the streamed root-confined staging
`DirectoryResolver`/`DirectorySink` primitives, and the generic SBI executor
(`internal/sbi/executor`, this branch) which supports JSON, single-binary and
multipart/related request bodies and multipart/related responses against the
shared transport with loopback tests.

**Explicitly not claimed:**

- no Host Artifact Store or `HostArtifactRef` implementation exists in this
  repository or in any accepted Host contract (ca-agent-demo PR #211 is a draft
  design only);
- no real Host binary dispatch has passed;
- no Provider process can accept or return binary/multipart payloads today —
  Provider Process Protocol v1 and its v2 read-tool extension are JSON-only;
- no real-NF binary or multipart operation has been executed.

## 1. Why a separate versioned extension

Provider Protocol v1 froze a JSON-only process boundary on purpose. Adding
binary fields to v1 or v2 would change a frozen contract, and inserting
base64/URLs into NDJSON would violate the architecture rule that binary bytes
are never projected through model-facing JSON. A separate, versioned payload
extension is therefore required.

## 2. Non-negotiable boundaries (already established)

- Binary bytes NEVER travel in NDJSON frames, as base64 or otherwise.
- The Host owns artifact storage, access control, invocation scoping, expiry,
  cleanup, quota enforcement and reference validity. The Provider never creates
  a second artifact store.
- The Skill supplies typed arguments declared by the trusted catalog; it never
  supplies a URL, method, path, header, credential, absolute filesystem path or
  artifact-store API parameter.
- The Provider receives only Host-generated opaque references and/or
  Host-created private staging roots. It streams bytes; it does not buffer large
  payloads in memory.
- The operation contract (media types, part names, Content-ID/RefToBinaryData
  binding, part count, per-part and aggregate limits) comes from the trusted
  Operation Catalog, not from the caller.
- Remote filenames are never used as local write paths.

## 3. Proposed shape (for review, not implementation)

A future `ca-client-tools.provider-payload/v1` extension would be negotiated as
part of a future Provider process version (not v1, not v2) and would add only
attachment descriptors to the existing JSON envelope:

```json
{
  "protocolVersion": "ca-client-tools.provider/<future>",
  "type": "request",
  "requestId": "host-invoke-001",
  "method": "invoke",
  "params": {
    "backendOperationId": "<trusted private id>",
    "arguments": { "...": "typed JSON arguments only" },
    "attachments": {
      "inputs": [
        {"artifactRef": "<Host-opaque reference>", "role": "<trusted part/argument name>"}
      ],
      "expectedOutputRoles": ["<trusted part name>"]
    },
    "invocation": { "...": "unchanged" }
  }
}
```

The Host resolves each opaque `artifactRef` in its own store, validates that it
belongs to the active invocation and caller, stages the bytes into a
Host-created private input root, and supplies that root plus the reference→
relative-name mapping through trusted process configuration. The Provider maps
the relative name to a trusted catalog part binding and streams it via the
Payload Engine.

The response would describe output attachments without embedding bytes:

```json
{
  "result": {
    "httpStatus": 200,
    "response": { "...": "JSON root only" },
    "attachments": [
      {
        "role": "<trusted part name>",
        "contentId": "<generated or source value>",
        "contentType": "application/vnd.3gpp.ngap",
        "size": 4096,
        "sha256": "<hex>"
      }
    ]
  }
}
```

The Provider writes response attachment bytes into a Host-created private output
root using generated relative names; the Host verifies root confinement,
ingests them into its Artifact Store and issues new opaque references. The
protocol result never contains a filesystem path or an artifact-store URL.

## 4. Unresolved Host-owned decisions (blockers for implementation)

The following must be published by the Host/artifact owner before this
extension can be implemented against a real system. This repository MUST NOT
invent them:

1. the opaque `HostArtifactRef` reference grammar and its trust properties;
2. Artifact Store create/read/write/ingest API and authorization model;
3. invocation-scoped staging layout, root names and cleanup guarantee;
4. per-artifact and aggregate sizes, quotas, expiry and content-digest policy;
5. how a stored artifact's declared media type is validated against the trusted
   operation catalog binding;
6. cancellation/partial-output semantics: when the Host may discard staged
   output, and how a cancelled invocation's artifacts are revoked;
7. whether the Host passes staging roots by environment variables
   (`CA_CLIENT_PAYLOAD_ROOT`, `CA_CLIENT_ATTACHMENT_ROOT`) or another trusted
   channel, and the exact process-level isolation rules.

Until these are accepted, only library-level payload work is in scope.

## 5. Catalog requirements for a binary/multipart operation

A future binary/multipart backend operation requires, in trusted catalog
content:

- a pinned OpenAPI operation and the exact request/response media types;
- request payload kind (`application/json`, a declared opaque binary type, or
  `multipart/related`);
- for multipart: root-first JSON type, the closed set of part names, each
  part's declared media type, the JSON-Pointer/RefToBinaryData field for each
  part, required/optional part flags, maximum part count, per-part and
  aggregate byte limits;
- response handling: expected root and attachment parts, how response
  Content-ID values are bound, whether unreferenced or duplicate parts fail
  closed;
- effect classification, and for mutations an explicit idempotency/
  reconciliation contract in addition to Host/Runtime authorization.

No such production binding is shipped. The `genericSBI` loader and executor can
represent these fields today, and the Payload Engine tests already cover the
wire behavior; the Provider process extension + Host artifact lifecycle are the
missing pieces.

## 6. Acceptance that would be required before enabling it

- A separately versioned Provider payload protocol with strict framing,
  duplicate-key/unknown-field rejection and negotiated attachment limits.
- Host Artifact Store implementation and an accepted cross-repository contract.
- Local tests: JSON + one binary part + multiple binary parts, mixed JSON +
  binary, Content-ID/RefToBinaryData binding, duplicate/missing/unreferenced
  parts, malformed boundaries, truncation, per-part/aggregate limits,
  cancellation, path-traversal/symlink rejection, digest and cleanup.
- Source-verified operation evidence against the deployed NF; a real
  binary/multipart SBI operation executed with Host-staged artifacts and a
  recorded result. Recorded as `REAL_E2E_PASS` only then.
- Mutation payloads additionally require authorization, idempotency and
  reconciliation evidence; no automatic retries.

**Current status:** `NOT_IMPLEMENTED` / `BLOCKED` on Host-owned artifact
contract; library-level payload engine and generic executor payload paths are
`IMPLEMENTED` / `LOCAL_ACCEPTANCE_PASS` (loopback).

# Provider payload and artifact contract

This is part of the single Provider protocol target. P0 supports inline nested JSON. P1 adds Host-owned artifact references and invocation-scoped streaming within that same protocol. The historical filename is retained for inbound links.

Status: **P1 IMPLEMENTED** against the pinned Host Artifact Store contract
`ca-host.artifact-store/v1` (ca-agent commit
`2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae`; fixtures pinned byte-for-byte under
[`contracts/host-artifact-store/v1/`](../contracts/host-artifact-store/v1)).
Local unit and process-level evidence is in
[the P1 record](evidence/provider-p1-20261118.md); the P1 closeout (audit,
reference-scope fix and Lead-run acceptance) is in
[the P1 closeout record](evidence/provider-p1-closeout-20261118.md).
Cross-repository acceptance is recorded by the Lead.

## Reference value, slots and direction

The only approved reference value is the closed JSON object
`{"artifactRef":"art_<43 unpadded base64url characters>"}` (grammar
`^art_[A-Za-z0-9_-]{43}$`). It is opaque, location-bound and direction-bound:

| Direction | Catalog declaration | Invocation/result location |
|---|---|---|
| Input, binary request body | `generic.body.kind: binary` | `arguments.body` = reference object |
| Input, multipart part | `generic.body.parts[*].name` | `arguments.payloads[<part name>]` = reference object |
| Output, binary response body | non-JSON, non-multipart response media type | `response` = reference object |
| Output, multipart response part | `generic.responseParts[*].jsonPointerField` (private Content-ID pointer) | value at the published slot pointer inside `response` = reference object; the published pointer is the parent `RefToBinaryData` object |

Catalog `responseParts[*].jsonPointerField` remains the private MIME Content-ID
location (for example `/messages/0/n2MessageContent/contentId`) and is never
published. The descriptor's `artifactSlots.outputs[*].pointer` (for example
`/messages/0/n2MessageContent`) is the publication location where the whole
value is replaced by the reference object; nested objects and array indexes are
preserved. The Provider generates the outbound MIME Content-IDs, writes them at
the declared private pointers, and overwrites any trusted-root placeholder; a
caller-supplied Content-ID is never trusted or echoed.

## Transfer and publication rules

- Byte streams use only the Host-owned loopback HTTP/1.1 channel of the pinned
  contract: GET `/v1/artifacts/{artifactRef}` for inputs and POST `/v1/artifacts`
  for outputs, with the contract headers and exact size/digest verification.
  Bytes never travel in NDJSON, never use a filesystem path supplied as JSON,
  never use base64 and never use an arbitrary URL.
- Input references are validated and resolved before any remote I/O. Missing or
  malformed references, references at undeclared locations or at non-slot
  values are `BACKEND_ARGUMENTS_INVALID`; a channel 401/403/404/409/410 is
  `ARTIFACT_REFERENCE_INVALID`; a channel 400/413/429/503, transport/timeout,
  size/digest/quota failure is `ARTIFACT_TRANSFER_FAILED`. The upstream sees
  zero requests for any pre-I/O rejection. Reference-location validation runs
  for **every** `genericSBI` invoke: a binding that declares no artifact slots
  has an empty declared set, so any reference value in its arguments is also an
  undeclared-location `BACKEND_ARGUMENTS_INVALID` (a Host token is never
  forwarded to the upstream NF).
- Response parts stream through the invocation-scoped Host sink; the executor's
  in-memory sink remains the default for non-artifact execution. Per-artifact,
  aggregate byte and attachment-count bounds come from the effective channel
  limits and are enforced independently of the Host.
- No output reference is emitted on failure, cancellation or deadline, and
  in-flight uploads are abandoned; the Host owns staging cleanup and publishes
  only an accepted result. The Provider keeps no persistent artifact namespace
  and never reuses an observed token.
- Diagnostics never contain references, tokens, paths, media-type-sensitive
  payload values or bytes. Allowed diagnostics are the opaque invocation ID,
  catalog slot name, size class and fault code.
- The optional `payload.AttachmentSink` streaming seam is additive: P0
  behavior, the memory sink and all existing executor/Payload Engine fixtures
  are unchanged when no Host sink is supplied.

## Acceptance status

P1 acceptance requires cross-repository binary and multipart invoke tests with
streamed bytes, nested pointer round trips, content type/size/digest checks, and
negative cases for foreign, expired or missing references, undeclared/missing/
duplicate parts, malformed/truncated MIME, limits, cancellation and partial
output cleanup. The Provider side of that matrix is implemented and covered by
local unit and process-level tests against a contract-faithful Host channel test
double; the real Host adapter run is recorded by the Lead. A real-NF multipart
operation is not available in the pinned deployment and remains `NOT_VERIFIED`:
no operation is invented for it.

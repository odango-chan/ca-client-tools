# Provider Process Protocol

Implemented for the internal demo. The sole accepted wire identity is `ca-client-tools.provider/v2`. The suffix identifies the consolidated contract. The v1 path, version selection and fallback were removed in P0: a frame carrying any other identity is rejected with `PROTOCOL_VERSION_UNSUPPORTED`. P1 adds Host Artifact Store streaming inside this same protocol and Catalog; there is no second protocol, Catalog or compatibility layer. See [WORK](WORK.md) for the verified implementation state.

## 1. Framing, envelope, correlation and lifecycle

Required process rules:

Launch the fixed executable with argv `provider serve`. A response carries
`protocolVersion`, `type: response`, `requestId` and `ok`. When `ok` is true
include only `result`; when false include only `error`.

- one UTF-8 JSON object per LF-terminated line; compact single-line frames;
- `protocolVersion`, `type=request`, `requestId`, `method`, `params`;
- unique per-process request IDs (at most 128 printable ASCII characters);
- frame size, in-flight and call-duration bounds negotiated in `initialize`;
- out-of-order responses correlated only by `requestId`;
- exactly one terminal response per accepted request;
- `cancel` targeting `params.targetRequestId` with `CANCEL_REQUESTED` / `ALREADY_COMPLETED` / `NOT_FOUND`;
- EOF-after-drain shutdown, no replay, no automatic retries;
- stdout protocol frames only; stderr safe diagnostics only.

The first request must be `initialize`. Every frame must use the sole wire identity. Reject any other identity with `PROTOCOL_VERSION_UNSUPPORTED`. Limits are negotiated independently of protocol identity.

### Malformed frames

Emit a bounded frame or request-ID fault using the recovered ID when valid, or an empty ID otherwise. The Host treats an uncorrelated fault as a process failure and restarts without replay.

## 2. initialize

Initialize accepts `params.hostInstanceId`, `params.providerInstanceId` and `params.limits` and returns the effective bounds and artifact identity:

```json
{
  "providerId": "ca-client-tools.sbi",
  "providerVersion": "1.0.0",
  "buildDigest": "sha256:<64 lowercase hex>",
  "backendBindingCatalogDigest": "sha256:<64 lowercase hex>",
  "catalogVersion": "2",
  "limits": {
    "maxFrameBytes": 8388608,
    "maxInFlight": 8,
    "maxCallDurationMs": 5000
  }
}
```

`backendBindingCatalogDigest` is the SHA-256 over the exact embedded v2 catalog
bytes. The Host independently verifies the executable/package digest and the
catalog digest against trusted deployment metadata. The reported values are consistency checks. Initialization performs no
implicit live health query.

Resolve services from [trusted deployment configuration](service-configuration.md). Fail initialization on malformed or partial configured profiles or when no trusted service is configured. Missing services make their bindings `NOT_CONFIGURED`. Validate package and catalog identity independently in the Host.

The Provider builds one trusted service registry at startup keyed by the
Catalog `serviceKey`: the typed OpenAMSM settings (`CA_OPENAMSM_*`) are
translated into it, and the optional trusted `CA_PROVIDER_SERVICE_CONFIG`
service file contributes additional h2c services and token profiles. Typed MAS,
genericSBI dispatch and the typed metrics handlers share one `internal/sbi`
transport instance built from that registry; the metrics listeners keep their
source-verified HTTP/1.1 behavior through an explicit trusted transport profile.
The same initialization fails closed on duplicate service keys, conflicting
token profiles or an invalid service file.

### 2.1 artifactChannel bootstrap (P1)

`initialize` optionally accepts `params.artifactChannel`: the Host Artifact
Store channel bootstrap of the pinned cross-repository contract
`ca-host.artifact-store/v1` ([Host integration](host-integration.md)):

```json
{
  "baseUrl": "http://127.0.0.1:53211",
  "processToken": "<opaque bearer token>",
  "requestTimeoutMs": 3000,
  "maxArtifactBytes": 1048576,
  "maxAggregateBytes": 4194304,
  "maxAttachments": 8
}
```

- The object is closed: exactly these six members. `baseUrl` is
  `http://127.0.0.1:<port>` only (no path, query, fragment, userinfo or other
  scheme), `processToken` is an opaque secret of 1-512 printable ASCII
  characters, `requestTimeoutMs` is a positive integer bounded by the
  negotiated `maxCallDurationMs`, and the three artifact bounds are positive
  integers. The Provider enforces the bounds independently and never exceeds
  its own payload/transport bounds. A malformed channel is
  `PROVIDER_CONFIGURATION_INVALID` at `initialize`; it is never partially
  applied. The token is never logged or echoed.
- Absent `artifactChannel` means artifact streaming is not configured:
  artifact-bearing bindings are `NOT_CONFIGURED` and artifact-bearing invokes
  are rejected before any remote I/O.
- On success, initialize reports the effective bounds it enforces:

```json
"artifactChannel": {
  "requestTimeoutMs": 3000,
  "maxArtifactBytes": 1048576,
  "maxAggregateBytes": 4194304,
  "maxAttachments": 8
}
```

The member is omitted when no channel is configured. Initialization still
performs no implicit live health query.

## 3. catalog

`params.backendOperationIds` is a unique array of exact
private backend operation IDs. An empty array is valid and returns empty
`supported` and `unsupported` arrays. The Provider partitions the requested
IDs exactly, preserving request order:

```json
{
  "supported": ["openamsm.smf.mas.sc.query", "openamsm.amf.ue-info.list"],
  "unsupported": [
    {"backendOperationId": "openamsm.unknown", "reason": "NOT_IMPLEMENTED"}
  ],
  "catalogVersion": "2",
  "bindings": [ /* descriptors for exactly the supported IDs, same order */ ]
}
```

`reason` is `NOT_IMPLEMENTED` (no such backend binding, or its declared response
shape is neither projectable JSON nor an artifact-bearing contract) or
`NOT_CONFIGURED` (binding exists but its trusted service is not configured, or
the binding is artifact-bearing and the Host did not configure an
`artifactChannel`). An artifact-bearing `genericSBI` binding is `NOT_CONFIGURED`
without a channel and available with one; a `genericSBI` binding whose declared
response is neither JSON nor an artifact shape is reported `NOT_IMPLEMENTED` and
rejected with `BACKEND_OPERATION_UNSUPPORTED`
(`detail.reason: "NOT_IMPLEMENTED"`) before any remote I/O.

An artifact-bearing `genericSBI` descriptor adds the closed `artifactSlots`
extension fixed by the Host contract §3. It is derived from the trusted Catalog
and contains no origin, path, token or Content-ID:

```json
"artifactSlots": {
  "inputs": [
    {"slot": "n1Message", "pointer": "/payloads/n1Message", "mediaType": "application/vnd.3gpp.5gnas", "required": true}
  ],
  "outputs": [
    {"slot": "n2Response", "pointer": "/messages/0/n2MessageContent", "mediaType": "application/vnd.3gpp.ngap", "required": true}
  ]
}
```

Input `pointer` is the invocation-argument location of the reference object
(`/body` or `/payloads/<escaped part name>`). Output `pointer` is the location
inside `response` of the decoded part's `RefToBinaryData` object; the private
catalog content-ID pointer is never published. The empty output pointer `""`
denotes the whole `response` value for a binary response body. `artifactSlots`
is present only when at least one direction declares a part.

A descriptor contains only private binding metadata. Typed bindings publish their scalar profiles below. Generic bindings publish the closed nested schema as `argumentSchema` and omit the scalar `arguments` field:

```json
{
  "backendOperationId": "openamsm.amf.ue-info.list",
  "effect": "READ",
  "request": {"mediaType": "application/json"},
  "response": {"mediaTypes": ["application/json"]},
  "arguments": [
    {"name": "page", "type": "integer", "required": false, "minimum": 0, "default": 0},
    {"name": "pageSize", "type": "integer", "required": false, "minimum": 1, "maximum": 100, "default": 100}
  ],
  "source": {"sourceId": "openamsm-amf-init-c", "operationId": "listAmfUEInfo"}
}
```

Descriptors MUST NOT contain service origins, URLs, HTTP methods, route
templates, header names or values, token values, public callable IDs,
Capability IDs, Owner Outcomes or Evidence guarantees. A descriptor is not an
authorization grant: the Host applies ToolCatalog visibility, deployment policy,
Runtime admission and Capability/Scope authorization before an invoke is sent.

## 4. invoke

Typed metrics example:

```json
{
  "backendOperationId": "openamsm.amf.gnb-info.list",
  "arguments": {"page": 0, "pageSize": 100},
  "invocation": {
    "invocationId": "runtime-invocation-01",
    "producerInvocationId": "producer-01",
    "deadline": "2026-11-17T09:30:00Z"
  }
}
```

- `arguments` is a JSON object. For `genericSBI`, it can contain nested objects and arrays, including `body` and schema-declared attachment slots. Preserve JSON null, booleans, strings and number precision. Reject duplicates at every depth, undeclared fields and invalid types before network I/O. Apply only declared defaults.
- Typed MAS requires `supi` and `pduSessionId`. Typed lists take optional `page` and `pageSize` as specified by the Catalog. Each invocation returns one page.
- Invocation metadata contains two opaque IDs and an absolute RFC 3339 UTC deadline. The effective deadline is bounded by the configured maximum duration.
- P1 carries Host-owned opaque artifact references only at declared
  input/output attachment locations. Byte streams travel through the pinned
  Host Artifact Store channel outside NDJSON. Before any remote I/O the Provider
  validates the channel is configured, every required declared input slot holds
  exactly the closed reference object `{"artifactRef":"art_<43 base64url>"}`, no
  reference value appears at an undeclared location or at a non-slot value, and
  it resolves every input reference through the channel with the declared media
  type, exact size and verified SHA-256. A shape/location violation is
  `BACKEND_ARGUMENTS_INVALID`; an unusable reference is
  `ARTIFACT_REFERENCE_INVALID`; a transfer, size, digest or quota failure is
  `ARTIFACT_TRANSFER_FAILED`. Arguments are mapped to the executor's resolver
  seam (`body` for a binary body, `payloads.<partName>` for multipart) and
  response parts are streamed to the invocation-scoped Host sink. Any failure,
  cancellation or deadline emits **no** output reference and abandons in-flight
  uploads. See [payload contract](provider-payload-extension-v1.md) and
  [Host integration](host-integration.md).

### Results

Successful remote 200 responses use this shape:

```json
{"httpStatus": 200, "response": { /* typed OpenAMSM result */ }}
```

A successful 2xx response with no projected JSON body (for example a 204 or a
bodyless generic result) is serialized as `{"httpStatus": 204}` with no
`response` member. A validated remote ProblemDetails result never carries a
`response` member.

- SMF context responses keep the typed envelope
  (`masId`, `status`, `key`, `context`) and echo/identity validation.
- List responses are the source-native paginated object validated by the typed
  metrics client (`items`, `pager`), preserved without reinterpretation.

A valid non-200 `application/problem+json` response remains a **successful
Provider protocol result**:

```json
{"httpStatus": 404, "problem": {"status": 404, "cause": "SESSION_NOT_FOUND"}}
```

For an artifact-bearing `genericSBI` binding, the successful result keeps
`httpStatus` unchanged and projects references instead of bytes:

- a binary response body makes `response` exactly the reference object:
  `{"httpStatus": 200, "response": {"artifactRef": "art_..."}}`;
- a `multipart/related` response keeps its JSON root and replaces each declared
  output slot value with its reference object, preserving nested objects,
  escaped pointer segments and array indexes: for example
  `/messages/0/n2MessageContent` becomes `{"artifactRef": "art_..."}` while the
  rest of the root is unchanged. The upstream MIME Content-ID is never echoed as
  a reference. On failure the result carries no `response` reference at all.

ProblemDetails is returned only when the media type is
`application/problem+json`, the body is a strict unique-key JSON object, the
problem status is an integer equal to the HTTP status and `cause` is a safe
machine token (`^[A-Z][A-Z0-9_]{0,63}$`). Anything else is
`REMOTE_RESPONSE_INVALID`. The three list endpoints have no ProblemDetails
contract: a non-200 list response is `REMOTE_RESPONSE_INVALID` with the
non-secret HTTP status in error details.

The Provider never maps a remote result to a CA-SL Owner Outcome, business
absence/presence, Evidence or Runtime state. It never returns raw remote
bodies, headers, query tokens, SUPI values in errors or subscriber projections
in diagnostics, and it never retries automatically.

### Production Mutation gate

A cataloged `MUTATION` binding is rejected before any remote I/O with
`BACKEND_OPERATION_UNSUPPORTED` and `detail.reason: "MUTATION_GATED"`. An
idempotency contract neither authorizes dispatch nor creates a retry path; the
deployed Catalog remains read-only until the operation-specific evidence gate,
Host/Runtime authorization and reviewed acceptance in [WORK](WORK.md) are
satisfied.

## 5. Error registry

Required fault mapping:

| Code | Meaning |
|---|---|
| `PROTOCOL_VERSION_UNSUPPORTED` | Unknown frame version or version change after initialize |
| `PROTOCOL_FRAME_INVALID` | Envelope/params/frame violations, unknown fields, duplicate keys |
| `PROTOCOL_FRAME_TOO_LARGE` / `PROTOCOL_METHOD_UNSUPPORTED` | Frame exceeds the active bound / unknown method |
| `PROTOCOL_REQUEST_ID_INVALID` | Invalid, reused or unrecoverable request ID |
| `PROVIDER_NOT_INITIALIZED` / `PROVIDER_ALREADY_INITIALIZED` | Handshake order |
| `PROVIDER_CONFIGURATION_INVALID` | Trusted environment missing/partial/invalid at initialize |
| `PROVIDER_BINDING_CATALOG_INVALID` | Embedded catalog or digest invalid |
| `BACKEND_OPERATION_UNSUPPORTED` | Unknown, `NOT_CONFIGURED` (including an artifact-bearing binding without a channel) or `NOT_IMPLEMENTED` backend operation |
| `BACKEND_ARGUMENTS_INVALID` | Typed argument or closed artifact-reference validation failed before remote dispatch |
| `CALL_DEADLINE_EXCEEDED` / `CALL_CANCELLED` | Per-call context outcome |
| `REMOTE_TRANSPORT_ERROR` | No trustworthy remote HTTP response |
| `REMOTE_RESPONSE_INVALID` | Malformed/undeclared remote response |
| `ARTIFACT_REFERENCE_INVALID` | Host channel 401/403/404/409/410: unusable, foreign, expired or unknown reference |
| `ARTIFACT_TRANSFER_FAILED` | Host channel 400/413/429/503, transport/timeout, size/digest/quota failure |
| `PROVIDER_INTERNAL` / `PROVIDER_SHUTTING_DOWN` | Unexpected failure / draining |

Every protocol fault uses an error object with a stable `code`, a required safe
`message`, and optional code-specific `detail`. Messages are fixed by the
Provider error registry and never contain caller values, internal errors,
credentials, remote bodies or subscriber identifiers.

A Provider protocol failure has no remote Owner Outcome; Host/Runtime maps it
to the appropriate Runtime UNKNOWN/failure path. A valid remote ProblemDetails
result is not a protocol failure.

## 6. Generic responses and authority

Preserve status, including 202 and 204, without polling. Project declared JSON, safe response metadata and, in P1, Host-issued attachment references. Map valid remote ProblemDetails to the remote-result envelope and map invalid upstream responses, transport failures and cancellation to distinct faults. Never expose upstream bodies in faults or let the caller select an origin, method, header, path template or credentials. A valid catalog binding establishes executability only. The production Mutation gate applies before dispatch.

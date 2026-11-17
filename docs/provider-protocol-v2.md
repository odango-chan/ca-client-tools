# Provider Process Protocol v2 — additive read-tool extension

**Status:** IMPLEMENTED AND LOCAL-ACCEPTANCE-PASSED in this branch; real live
acceptance against the pinned OpenAMSM lab is recorded separately in
[provider-acceptance-20261117.md](evidence/provider-acceptance-20261117.md).
**Protocol ID:** `ca-client-tools.provider/v2`  
**Relationship to v1:** additive and version-selected. The frozen
[Provider Process Protocol v1](https://github.com/odango-chan/ca-client-tools/blob/2804adc4f9b109dd10f5998bc816dd372c479ca5/docs/provider-protocol-v1.md)
is unchanged. A v1 Host sees exactly the v1 behavior; it must select v2
explicitly through the initialize `protocolVersion`.

## 1. Why v2 exists

Protocol v1 covers the eight private SMF context-query backend operations and a
fixed `arguments` object of exactly `supi` and `pduSessionId`. The repository
already contains three more read-only OpenAMSM surfaces (`GET /ue-info`,
`GET /gnb-info`, `GET /pdu-info`). Exposing them through v1 would change a
frozen contract, so v2 adds exactly what is needed:

1. a catalog that covers all eleven source-verified read-only operations;
2. catalog descriptors that carry effect, typed argument schema, media types
   and private source identity so the Host can bind active Skill operations
   without guessing;
3. argument validation driven by the trusted catalog binding instead of a
   hard-coded SMF argument object.

v2 does not add generic HTTP execution, caller-selected URLs/methods/paths/
headers, credentials, binary/base64 payloads or any authorization decision.

## 2. Framing, envelope, correlation and lifecycle

All v1 rules apply unchanged, including:

- one UTF-8 JSON object per LF-terminated line; compact single-line frames;
- `protocolVersion`, `type=request`, `requestId`, `method`, `params`;
- unique per-process request IDs (at most 128 printable ASCII characters);
- frame size, in-flight and call-duration bounds negotiated in `initialize`;
- out-of-order responses correlated only by `requestId`;
- exactly one terminal response per accepted request;
- `cancel` with `CANCEL_REQUESTED` / `ALREADY_COMPLETED` / `NOT_FOUND`;
- EOF-after-drain shutdown, no replay, no automatic retries;
- stdout protocol frames only; stderr safe diagnostics only.

A process negotiates **one** protocol version in the first `initialize` frame.
Every later frame must carry that exact version; a mismatch is
`PROTOCOL_VERSION_UNSUPPORTED`. Changing version requires a new process.

### 2.1 Malformed frames

v1 requires responses to correlate by `requestId`, but a frame can be so
malformed that no request ID is recoverable. v2 states the behavior the
implementation already uses: the Provider emits a best-effort
`PROTOCOL_FRAME_INVALID` / `PROTOCOL_REQUEST_ID_INVALID` response using the
`requestId` if it is syntactically recoverable and `""` otherwise, then keeps
serving. A conforming Host treats an uncorrelated error response as a protocol
failure and restarts the process.

## 3. initialize

Request and response are identical to v1 except for the accepted
`protocolVersion` and one additional response field. A v2 initialize result is:

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
catalog digest against trusted deployment metadata; the reported values are
consistency checks, not self-authenticating trust. Initialization performs no
implicit live health query.

Trusted configuration is read only from the allowlisted environment:

- `CA_OPENAMSM_SMF_SBI_ORIGIN` + `CA_OPENAMSM_QUERY_TOKEN` — SMF context
  queries (required for v1; optional for v2 if other services are configured);
- `CA_OPENAMSM_AMF_METRICS_ORIGIN` — AMF UE/gNB list operations;
- `CA_OPENAMSM_SMF_METRICS_ORIGIN` — SMF PDU list operation.

v2 requires at least one trusted service to be configured (otherwise
`PROVIDER_CONFIGURATION_INVALID`). A partially configured service pair is a
configuration error, not a silent partial start. A service that is simply not
configured makes its bindings `NOT_CONFIGURED` in `catalog`; the process still
starts for the configured services. v1 keeps its stricter requirement: both SMF
SBI origin and query token must be present.

## 4. catalog

Request is unchanged: `params.backendOperationIds` is a non-empty-unique array
of exact private backend operation IDs. The Provider partitions the requested
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

`reason` is `NOT_IMPLEMENTED` (no such backend binding) or `NOT_CONFIGURED`
(binding exists but its trusted service is not configured).

A v2 descriptor contains only private binding metadata:

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

## 5. invoke

Request shape is unchanged:

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

v2 differences:

- `arguments` is validated against the trusted catalog binding's declared
  argument schema: exact member set, required members, `string`/`integer`/
  `boolean` types, integer ranges and string patterns. Unknown, missing,
  mistyped or out-of-range arguments fail with `BACKEND_ARGUMENTS_INVALID`
  before any remote request.
- Defaults are trusted catalog content applied by the Provider for absent
  optional arguments. A caller can never supply an argument not declared for
  that backend operation.
- MAS argument profile is byte-for-byte the v1 profile: exactly `supi` and
  `pduSessionId`. IMSI aliases and UE-IP lookup keys are not exposed.
- List argument profile: optional `page` (integer ≥ 0, default 0) and
  `pageSize` (integer 1–100, default 100). One invocation returns one page.
- `invocation` validation is unchanged (two opaque IDs, absolute RFC 3339
  deadline with UTC `Z`). The deadline and the configured maximum call duration
  bound the Go call context.

### 5.1 Results

Successful remote 200 responses use the same shape as v1:

```json
{"httpStatus": 200, "response": { /* typed OpenAMSM result */ }}
```

- SMF context responses keep the exact typed v1 envelope
  (`masId`, `status`, `key`, `context`) and all v1 echo/identity validation.
- List responses are the source-native paginated object validated by the typed
  metrics client (`items`, `pager`), preserved without reinterpretation.

A valid non-200 `application/problem+json` response remains a **successful
Provider protocol result**:

```json
{"httpStatus": 404, "problem": {"status": 404, "cause": "SESSION_NOT_FOUND"}}
```

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

## 6. Error registry

v2 reuses the frozen v1 code registry with no additions. In particular:

| Code | v2 use |
|---|---|
| `PROTOCOL_VERSION_UNSUPPORTED` | Unknown frame version or version change after initialize |
| `PROTOCOL_FRAME_INVALID` | Envelope/params/frame violations, unknown fields, duplicate keys |
| `PROTOCOL_REQUEST_ID_INVALID` | Invalid, reused or unrecoverable request ID |
| `PROVIDER_NOT_INITIALIZED` / `PROVIDER_ALREADY_INITIALIZED` | Handshake order |
| `PROVIDER_CONFIGURATION_INVALID` | Trusted environment missing/partial/invalid at initialize |
| `PROVIDER_BINDING_CATALOG_INVALID` | Embedded v2 catalog or digest invalid |
| `BACKEND_OPERATION_UNSUPPORTED` | Unknown or `NOT_CONFIGURED` backend operation |
| `BACKEND_ARGUMENTS_INVALID` | Typed argument validation failed before remote dispatch |
| `CALL_DEADLINE_EXCEEDED` / `CALL_CANCELLED` | Per-call context outcome |
| `REMOTE_TRANSPORT_ERROR` | No trustworthy remote HTTP response |
| `REMOTE_RESPONSE_INVALID` | Malformed/undeclared remote response |
| `PROVIDER_INTERNAL` / `PROVIDER_SHUTTING_DOWN` | Unexpected failure / draining |

A Provider protocol failure has no remote Owner Outcome; Host/Runtime maps it
to the appropriate Runtime UNKNOWN/failure path. A valid remote ProblemDetails
result is not a protocol failure.

## 7. Explicit v2 exclusions

v2 remains JSON-only. It defines no:

- binary, multipart, base64, Content-ID, filesystem-path or HostArtifactRef
  fields (see the separate Payload Extension design;
  `ca-agent-demo` PR #211 is design-only and the Host Artifact Store does not
  exist);
- generic HTTP/curl execution or caller authority over service, method, path,
  headers, content type or credentials;
- standardized 3GPP Operation Catalog execution. The generic executor
  infrastructure exists in this repository, but v2 ships no production
  `genericSBI` binding because no standardized operation has the required
  pinned-source/deployment evidence;
- MUTATION bindings in the shipped catalog. The catalog schema can represent a
  MUTATION binding only with an explicit idempotency/reconciliation contract,
  but no mutation is enabled for dispatch;
- OAuth, TLS/mTLS, NRF discovery, public ToolCatalog publication, Capability
  authorization or Provider-side workflow orchestration.

## 8. Versioning rules

- v1 remains frozen and supported for its eight SMF operations.
- v2 is selected only by the initialize `protocolVersion`; there is no
  fallback, negotiation loop or per-frame upgrade.
- A future extension must use a new versioned catalog/protocol artifact. Adding
  fields to v2 or changing the interpretation of an existing field is not
  permitted.
- Every protocol/catalog version is embedded trusted package content with a
  published SHA-256 digest.

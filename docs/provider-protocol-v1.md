# CA Client Tools Provider Process Protocol v1

**Status:** FROZEN FOR IMPLEMENTATION  
**Implementation status:** NOT IMPLEMENTED  
**Protocol ID:** `ca-client-tools.provider/v1`

This is the sole normative definition of the Host ↔ Provider process wire protocol. CLI v1 is a separate operator/test interface.

## 1. Process model

The Provider is a preinstalled Go executable in `ca-client-tools`. The Host starts one long-lived process per configured Provider instance:

```text
ca-client-tools provider serve
```

The Host uses a fixed trusted executable and argument vector, never a shell. It MUST NOT start a process or invoke a CLI command per callable invocation. The process is not an Agent-facing HTTP server, MCP server, generic command runner or second Runtime.

Host writes request frames to stdin; Provider writes response frames to stdout; diagnostics go to stderr only. stdout MUST contain protocol frames only. stderr MUST NOT contain credentials, query tokens, raw sensitive bodies or binary payload contents.

For the first OpenAMSM SMF slice, the Host passes only these trusted, allowlisted environment entries:

- `CA_OPENAMSM_SMF_SBI_ORIGIN`
- `CA_OPENAMSM_QUERY_TOKEN`

The Host MUST construct a sanitized environment rather than inheriting the full Agent/Host environment. Tokens MUST NOT appear in argv, frames, errors or logs.

The existing SMF typed client uses h2c HTTP/2 prior knowledge, disables redirects, defaults to a 5-second timeout (maximum 30 seconds), and defaults to a 1 MiB response limit (maximum 4 MiB). These are first-binding properties, not generic protocol fields.

## 2. NDJSON framing and envelope

Each frame is exactly one UTF-8 JSON object followed by LF (`\n`). JSON MUST NOT be pretty-printed across lines.

| Limit | Default | Hard maximum |
|---|---:|---:|
| Frame size, including newline | 8 MiB | 16 MiB |
| In-flight Provider requests | 8 | 16 |
| First SMF call duration | 5,000 ms | 30,000 ms |

The Host supplies limits in `initialize`. The Provider rejects values above hard maxima and returns accepted limits no greater than those requested. Both sides enforce the negotiated bounds.

The Provider rejects malformed UTF-8, blank frames, non-object JSON, multiple JSON values in one frame, duplicate object keys, oversized frames and unknown envelope fields. It MUST NOT emit banners, progress text or blank stdout lines.

Every request has exactly:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `protocolVersion` | string | yes | `ca-client-tools.provider/v1` |
| `type` | string | yes | `request` |
| `requestId` | string | yes | Host-generated unique correlation ID |
| `method` | string | yes | `initialize`, `catalog`, `invoke`, `cancel` |
| `params` | object | yes | Method-specific fields |

`requestId` is non-empty, at most 128 ASCII characters and unique for the process lifetime.

Every response has exactly `protocolVersion`, `type: response`, `requestId`, `ok`, and exactly one of `result` or `error`. The response `requestId` MUST equal its request. `ok=true` means the protocol method completed; a valid Owner Outcome `FAILED` is still `ok=true`.

The Provider may complete concurrent requests out of order. The Host correlates only by `requestId`. Each accepted request receives one terminal response unless the process exits first.

## 3. Startup handshake

The first Host frame MUST be `initialize`. No other method is valid before successful initialization.

```json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-init-001",
  "method": "initialize",
  "params": {
    "hostInstanceId": "host-instance-01",
    "providerInstanceId": "ca-client-tools.sbi",
    "limits": {
      "maxFrameBytes": 8388608,
      "maxInFlight": 8,
      "maxCallDurationMs": 5000
    }
  }
}
```

`hostInstanceId` and `providerInstanceId` are non-empty trusted deployment identifiers. No URL, token, arbitrary environment variable, filesystem path or Agent configuration is carried in this frame.

Successful result:

```json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-init-001",
  "ok": true,
  "result": {
    "providerId": "ca-client-tools.sbi",
    "providerVersion": "1.0.0",
    "buildDigest": "sha256:<provider-build-digest>",
    "bindingCatalogDigest": "sha256:<provider-binding-catalog-digest>",
    "limits": {
      "maxFrameBytes": 8388608,
      "maxInFlight": 8,
      "maxCallDurationMs": 5000
    }
  }
}
```

The Host independently verifies executable/package and Provider Callable Binding Catalog digests against trusted deployment metadata. Process-reported values are consistency checks, not self-authenticating trust. Missing/invalid configuration fails initialization with `PROVIDER_CONFIGURATION_INVALID`. Initialization does not perform an implicit live health query. Version negotiation is not supported.

## 4. Methods

### 4.1 catalog

The Host supplies exact callable identities already resolved from ToolCatalog. Capability IDs MUST NOT be sent as callable identities.

Request parameters: `callables`, a non-empty unique array of objects containing exactly `owner`, `kind`, `id`. `kind` is `operation` or `observation`.

```json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-catalog-001",
  "method": "catalog",
  "params": {
    "callables": [
      {"owner":"ca-client-tools","kind":"observation","id":"smf.context.session.query"}
    ]
  }
}
```

Result:

```json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-catalog-001",
  "ok": true,
  "result": {
    "supported": [
      {"owner":"ca-client-tools","kind":"observation","id":"smf.context.session.query"}
    ],
    "unsupported": []
  }
}
```

An unsupported entry has the same identity and a `reason` of `NOT_IMPLEMENTED` or `NOT_CONFIGURED`. The response MUST partition the requested identities exactly: no duplicate, unrequested, altered or omitted identity. Any mismatch fails closed.

The Provider MUST NOT return or override Capability IDs, Effect, schemas, Evidence guarantees, URL, HTTP method, path, headers, service discovery policy or credentials. Host Tool descriptors come from ToolCatalog and deployment CapabilityContracts.

### 4.2 invoke

Request parameters:

- `callable`: exact `owner`, `kind`, `id`;
- `arguments`: materialized, schema-validated Operation arguments or Observation QueryContext;
- `invocation`: `invocationId`, `producerInvocationId`, and absolute RFC 3339 `deadline`.

```json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-invoke-001",
  "method": "invoke",
  "params": {
    "callable": {"owner":"ca-client-tools","kind":"observation","id":"smf.context.session.query"},
    "arguments": {"supi":"imsi-001010000000001","pduSessionId":5},
    "invocation": {
      "invocationId":"invocation-01",
      "producerInvocationId":"producer-01",
      "deadline":"2026-11-17T09:30:00Z"
    }
  }
}
```

Valid Owner success:

```json
{
  "protocolVersion":"ca-client-tools.provider/v1",
  "type":"response",
  "requestId":"host-invoke-001",
  "ok":true,
  "result":{
    "ownerOutcome":"SUCCEEDED",
    "output":{
      "type":"session",
      "status":"AVAILABLE",
      "key":{"supi":"imsi-001010000000001","pduSessionId":5},
      "context":{
        "lifecycle":"OPERATIONAL","dnn":"internet","sst":1,"sd":"",
        "subscriptionValidated":true,"secondaryAuthRequired":false,"uecmRegistered":true
      }
    }
  }
}
```

Valid Owner failure is also `ok=true`:

```json
{
  "protocolVersion":"ca-client-tools.provider/v1",
  "type":"response",
  "requestId":"host-invoke-001",
  "ok":true,
  "result":{"ownerOutcome":"FAILED","failure":{"code":"SESSION_NOT_FOUND","httpStatus":404}}
}
```

For the first eight Observation callables, Owner Outcomes are only `SUCCEEDED` and `FAILED`. Provider/Runtime values `DENIED`, `UNKNOWN`, `OUTCOME_UNKNOWN`, `COMMITTED` and `NOT_COMMITTED` MUST NOT be returned as Owner Outcomes.

The Provider validates the exact callable, typed arguments, private binding, response identity echo and normalized output. It MUST call the existing typed Go client directly; it MUST NOT spawn the CLI or reimplement the SMF query client. Reuse `internal/openamsm/smf/mas.Client.Query`.

### 4.3 cancel

A cancel request has its own request ID and a `targetRequestId` identifying an in-flight `invoke`:

```json
{
  "protocolVersion":"ca-client-tools.provider/v1",
  "type":"request",
  "requestId":"host-cancel-001",
  "method":"cancel",
  "params":{"targetRequestId":"host-invoke-001"}
}
```

The result `disposition` is `CANCEL_REQUESTED`, `ALREADY_COMPLETED` or `NOT_FOUND`.

The cancel acknowledgement is not the terminal response for the original invoke. The original invocation MUST complete exactly once if the process remains alive; a race may return the valid result instead. Cancellation propagates to the Go `context.Context` and active HTTP request. A cancel acknowledgement does not prove that a remote operation did not complete. Host/Runtime owns v1alpha4 cancellation and Mutation/Reconcile semantics. The Provider MUST NOT replay a call after cancellation or restart.

## 5. Errors and Owner failures

Provider/protocol error envelope:

```json
{
  "protocolVersion":"ca-client-tools.provider/v1",
  "type":"response",
  "requestId":"host-invoke-001",
  "ok":false,
  "error":{"code":"REMOTE_TRANSPORT_ERROR","message":"SMF context query transport failed"}
}
```

The error object contains stable `code`, safe `message`, and optional code-specific safe `details`. It MUST NOT contain a raw remote body or secret.

| Code | Meaning |
|---|---|
| `PROTOCOL_VERSION_UNSUPPORTED` | Unsupported protocol version; fail initialization |
| `PROTOCOL_FRAME_INVALID` | Invalid UTF-8/JSON, duplicate keys, wrong envelope or unknown fields |
| `PROTOCOL_FRAME_TOO_LARGE` | Frame exceeds negotiated maximum |
| `PROTOCOL_REQUEST_ID_INVALID` | Missing, malformed or reused request ID |
| `PROTOCOL_METHOD_UNSUPPORTED` | Unknown method |
| `PROVIDER_NOT_INITIALIZED` | Request before initialize |
| `PROVIDER_ALREADY_INITIALIZED` | Second initialize |
| `PROVIDER_CONFIGURATION_INVALID` | Required trusted configuration absent/invalid |
| `PROVIDER_BINDING_CATALOG_INVALID` | Embedded binding catalog invalid or digest mismatch |
| `CALLABLE_NOT_SUPPORTED` | No implementation for exact callable |
| `CALL_ARGUMENTS_INVALID` | Typed input invalid before remote dispatch |
| `CALL_DEADLINE_EXCEEDED` | Per-call deadline elapsed |
| `CALL_CANCELLED` | Per-call context cancelled |
| `REMOTE_TRANSPORT_ERROR` | No trustworthy remote response |
| `REMOTE_RESPONSE_INVALID` | Malformed response, wrong private type, key mismatch or invalid output |
| `PROVIDER_INTERNAL` | Unexpected Provider failure |
| `PROVIDER_SHUTTING_DOWN` | Provider is draining and no longer admits work |

A valid OpenAMSM HTTP ProblemDetails response is not a Provider protocol error. It is normalized to Owner `FAILED` with the safe cause/status. Source causes include `INVALID_QUERY_KEY`, `INVALID_PARAM`, `FORBIDDEN`, `SESSION_NOT_FOUND`, `UNKNOWN_QUERY_SERVICE`, `KEY_CONFLICT`, `METHOD_NOT_ALLOWED`, `SERVICE_UNAVAILABLE` and `INTERNAL`. An unrecognized valid HTTP error becomes Owner `FAILED` with generic `HTTP_ERROR` and status; raw body is never returned.

A valid HTTP 200 with `status: UNAVAILABLE` is Owner `SUCCEEDED`. Transport failures and invalid responses are not Owner failures: for an Observation they map to Runtime `UNKNOWN`. `CALL_CANCELLED` and `CALL_DEADLINE_EXCEEDED` map to global Runtime cancellation/deadline, not route keys. A valid remote 403 is Owner `FAILED`, not Host `DENIED`.

## 6. Correlation, concurrency and lifecycle

- Host owns admission limits; Provider enforces accepted `maxInFlight` and `maxFrameBytes`.
- Host supplies an absolute deadline; Provider uses it as the upper bound for the Go call context.
- Provider MUST NOT retry automatically.
- Responses may be out of order but MUST be complete single-line frames correlated by request ID.
- Normal Owner `FAILED` does not terminate the process.
- Host stops admitting work, drains/cancels in-flight calls, then closes stdin.
- EOF after drain is the normal shutdown signal; v1 has no `shutdown` method.
- Provider exits normally after EOF and drain; Host may force-terminate after a bounded grace period.
- Unexpected process exit leaves outstanding Observation calls without valid Owner results as Runtime UNKNOWN. A dispatched Mutation remains OUTCOME_UNKNOWN and follows frozen v1alpha4 Reconcile/responsibility rules.
- Process restart MUST NOT replay any request. `requestId` is correlation, not an idempotency key.

## 7. v1 exclusions

Protocol v1 is JSON-only. It has no binary file references, HostArtifactRef, multipart channel, filesystem paths, Content-ID or base64 payload fields. NAS/NGAP codecs, HostArtifactRef staging and binary/multipart SBI payloads require a separately reviewed extension/version. They MUST NOT be smuggled into v1 as optional fields.

Protocol v1 also does not define OAuth, TLS identity, NRF discovery, arbitrary service selection, arbitrary HTTP methods/paths, generic curl requests or Provider-side workflow orchestration.

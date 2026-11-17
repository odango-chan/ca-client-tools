# CA Client Tools Provider Process Protocol v1

**Status:** FROZEN FOR IMPLEMENTATION — Provider/backend boundary only  
**Implementation status:** Resident Provider NOT IMPLEMENTED  
**Protocol ID:** ca-client-tools.provider/v1

This protocol is the private process boundary between the CA-Agent Host adapter and the installed Go Provider. It is not a CA-SL ToolCatalog, not an Agent-facing Tool API, and not an authorization protocol.

## 1. Ownership and process model

- ca-agent-host owns the public CA-SL Observation callables, their ToolCatalog exports, normalized Output Schemas, Owner Outcomes and Evidence Producer behavior.
- Deployment owns CapabilityContracts and Scope policies.
- SMF is the authority for the queried network state.
- ca-client-tools owns the typed OpenAMSM Go client and its resident Provider process backend.
- The Host adapter maps an already-admitted public callable to one exact Provider backend operation. The Provider does not receive Capability IDs and does not authorize public callables.

The Provider is a preinstalled Go executable in ca-client-tools:

~~~text
ca-client-tools provider serve
~~~

The Host starts one long-lived process per configured Provider instance. It uses a fixed trusted executable and argument vector, never a shell. It MUST NOT spawn the CLI or a new Provider process per query.

Host writes frames to child stdin; Provider writes response frames to stdout; diagnostics go to stderr only. stdout MUST contain protocol frames only. stderr MUST NOT contain credentials, query tokens, raw response bodies, or binary payload contents.

The Host passes a sanitized environment containing only trusted deployment configuration required by the Provider:

- CA_OPENAMSM_SMF_SBI_ORIGIN
- CA_OPENAMSM_QUERY_TOKEN

The token MUST NOT appear in argv, NDJSON frames, response bodies, errors or logs.

The Provider reuses internal/openamsm/smf/mas.Client.Query directly. It MUST NOT spawn the CLI, duplicate the HTTP/2 client, or implement a second OpenAMSM query path.

## 2. Framing and common envelope

Each frame is exactly one UTF-8 JSON object followed by LF (\n). JSON MUST NOT be pretty-printed across lines.

| Limit | Default | Hard maximum |
|---|---:|---:|
| Frame size, including newline | 8 MiB | 16 MiB |
| In-flight Provider requests | 8 | 16 |
| Query duration | 5,000 ms | 30,000 ms |

The Host supplies limits in initialize. Provider accepts only values within hard maxima and returns accepted values no greater than those requested. Both sides enforce the accepted bounds.

Provider rejects malformed UTF-8, blank frames, non-object JSON, multiple JSON values in one frame, duplicate object keys, oversized frames, unknown envelope fields and reused request IDs. It MUST NOT emit banners, progress text or blank stdout lines.

Every request has exactly:

| Field | Type | Required |
|---|---|---|
| protocolVersion | string | yes |
| type | string | yes; exact value request |
| requestId | string | yes |
| method | string | yes |
| params | object | yes |

protocolVersion is exactly ca-client-tools.provider/v1. requestId is non-empty, at most 128 ASCII characters, unique for the process lifetime, and never reused.

Every response has exactly protocolVersion, type=response, requestId, ok, and exactly one of result or error. The response requestId MUST equal its request. ok=true means the Provider protocol operation completed; it does not mean the remote HTTP query was successful. A valid remote HTTP ProblemDetails response is returned inside result, not as a Provider protocol error.

Provider may complete concurrent requests out of order. Host correlates only by requestId. Each accepted request receives one terminal response unless the process exits first.

## 3. Startup handshake

The first Host frame MUST be initialize. No other method is valid before successful initialization.

~~~json
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
~~~

The instance IDs are non-empty trusted deployment identifiers. No URL, token, arbitrary environment variable, filesystem path or Skill-supplied configuration is carried in this frame.

Successful result:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-init-001",
  "ok": true,
  "result": {
    "providerId": "ca-client-tools.sbi",
    "providerVersion": "1.0.0",
    "buildDigest": "sha256:<provider-build-digest>",
    "backendBindingCatalogDigest": "sha256:<backend-binding-catalog-digest>",
    "limits": {
      "maxFrameBytes": 8388608,
      "maxInFlight": 8,
      "maxCallDurationMs": 5000
    }
  }
}
~~~

Host independently verifies executable/package and backend-binding-catalog digests against trusted deployment metadata. Values reported by the process are consistency checks, not self-authenticating trust. Missing or invalid configuration fails initialization with PROVIDER_CONFIGURATION_INVALID. Initialization MUST NOT perform an implicit live health query. Version negotiation is not supported in v1.

## 4. Private backend operation catalog

The catalog method describes Provider backend operations only. It MUST NOT contain public ToolCatalog callable identities, Capability IDs, Owner Outcomes, Skill Schemas, Evidence guarantees or authorization decisions.

The Host supplies the exact backend operation IDs required by its trusted adapter bindings:

- openamsm.smf.mas.sc.query
- openamsm.smf.mas.pa.query
- openamsm.smf.mas.upn.query
- openamsm.smf.mas.ssm.query
- openamsm.smf.mas.iam.query
- openamsm.smf.mas.ups.query
- openamsm.smf.mas.ca.query
- openamsm.smf.mas.saa.query

Request:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-catalog-001",
  "method": "catalog",
  "params": {
    "backendOperationIds": [
      "openamsm.smf.mas.sc.query",
      "openamsm.smf.mas.pa.query"
    ]
  }
}
~~~

Result:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-catalog-001",
  "ok": true,
  "result": {
    "supported": [
      "openamsm.smf.mas.sc.query",
      "openamsm.smf.mas.pa.query"
    ],
    "unsupported": []
  }
}
~~~

An unsupported entry contains the exact requested ID and reason NOT_IMPLEMENTED or NOT_CONFIGURED. The response MUST partition the requested IDs exactly: no duplicate, unrequested, altered or omitted ID. Any mismatch fails closed.

The immutable Provider Backend Binding Catalog maps each backend operation ID to one fixed MAS selector, route and typed Go client call. The Host's separate trusted adapter binding maps Host-owned ToolCatalog callables to these backend operation IDs. Neither catalog grants Capability access or changes ToolCatalog contracts.

## 5. invoke

Request parameters:

- backendOperationId: one exact ID from §4;
- arguments: exactly supi and pduSessionId;
- invocation: Host/Runtime execution metadata.

The Provider backend profile accepts formatted SUPI and PDU Session ID. The 5GC Skill-facing Host QueryContext Schema narrows PSI to 1–15; the underlying OpenAMSM/CLI client retains its broader 0–255 API validation. IMSI alias and UE-IP lookup modes are not exposed through this Provider backend profile.

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-invoke-001",
  "method": "invoke",
  "params": {
    "backendOperationId": "openamsm.smf.mas.sc.query",
    "arguments": {
      "supi": "imsi-001010000000001",
      "pduSessionId": 5
    },
    "invocation": {
      "invocationId": "runtime-invocation-01",
      "producerInvocationId": "producer-01",
      "deadline": "2026-11-17T09:30:00Z"
    }
  }
}
~~~

A valid HTTP 200 response is returned as a Provider result:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-invoke-001",
  "ok": true,
  "result": {
    "httpStatus": 200,
    "response": {
      "masId": "SC",
      "status": "AVAILABLE",
      "key": {
        "supi": "imsi-001010000000001",
        "pduSessionId": 5
      },
      "context": {
        "lifecycle": "OPERATIONAL",
        "dnn": "internet",
        "sst": 1,
        "sd": "",
        "subscriptionValidated": true,
        "secondaryAuthRequired": false,
        "uecmRegistered": true
      }
    }
  }
}
~~~

A valid remote ProblemDetails response is also a successful Provider protocol result:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "response",
  "requestId": "host-invoke-001",
  "ok": true,
  "result": {
    "httpStatus": 404,
    "problem": {
      "status": 404,
      "cause": "SESSION_NOT_FOUND"
    }
  }
}
~~~

Provider returns the typed remote HTTP result; it MUST NOT map it to CA-SL Owner Outcome or Runtime Outcome. Host-owned Observation adapters perform business normalization. The generic Host callable `context.smf.session.read` is dispatched through this resident ca-client-tools Provider using the same private backend operation, `openamsm.smf.mas.sc.query`, as the MAS-specific `smf.mas.sc.context.query` callable. The two public callables have distinct Host output Schemas and normalization: only the generic adapter maps exact HTTP 404 + SESSION_NOT_FOUND to the successful `sessionPresence: ABSENT` result; the MAS-specific SC callable returns its declared Owner failure. None of the projection-specific callables convert 404 to UNAVAILABLE.

The Provider MUST validate the exact backend operation ID, typed arguments, private route/type binding, remote HTTP status and response. For HTTP 200 it reuses the existing typed client's exact MAS type, SUPI/PSI echo binding, status and empty-UNAVAILABLE-context validation. For non-200 it accepts a ProblemDetails result only when the response has the declared ProblemDetails media type, a valid object, matching HTTP/problem status and a safe machine cause. Malformed/non-ProblemDetails responses are REMOTE_RESPONSE_INVALID, not remote Owner failures.

The Provider MUST NOT return raw remote bodies, response headers containing secrets, query tokens or arbitrary transport metadata. It MUST NOT retry automatically.

## 6. cancel

A cancel request has its own request ID and a targetRequestId identifying an in-flight invoke request:

~~~json
{
  "protocolVersion": "ca-client-tools.provider/v1",
  "type": "request",
  "requestId": "host-cancel-001",
  "method": "cancel",
  "params": {
    "targetRequestId": "host-invoke-001"
  }
}
~~~

The result disposition is one of CANCEL_REQUESTED, ALREADY_COMPLETED or NOT_FOUND.

The cancel acknowledgement is not the terminal response for the original invoke. The original request MUST complete exactly once if the process remains alive; a completion/cancellation race may return the valid remote result instead. Cancellation propagates to the Go context.Context and active HTTP request. A cancel acknowledgement does not prove that a remote query did not execute. Provider MUST NOT replay after cancellation or process restart.

## 7. Provider/protocol error registry

A Provider error uses ok=false and an object containing stable code, safe message, and optional code-specific safe details. It MUST NOT include secrets or raw remote bodies.

| Code | Meaning |
|---|---|
| PROTOCOL_VERSION_UNSUPPORTED | Unsupported protocol version |
| PROTOCOL_FRAME_INVALID | Invalid UTF-8/JSON, duplicate keys, invalid envelope or unknown fields |
| PROTOCOL_FRAME_TOO_LARGE | Frame exceeds negotiated maximum |
| PROTOCOL_REQUEST_ID_INVALID | Missing, malformed or reused request ID |
| PROTOCOL_METHOD_UNSUPPORTED | Unknown method |
| PROVIDER_NOT_INITIALIZED | Request before initialize |
| PROVIDER_ALREADY_INITIALIZED | Second initialize |
| PROVIDER_CONFIGURATION_INVALID | Required trusted configuration absent/invalid |
| PROVIDER_BINDING_CATALOG_INVALID | Embedded backend binding catalog invalid or digest mismatch |
| BACKEND_OPERATION_UNSUPPORTED | Exact backend operation has no implementation |
| BACKEND_ARGUMENTS_INVALID | Typed input invalid before remote dispatch |
| CALL_DEADLINE_EXCEEDED | Per-call deadline elapsed |
| CALL_CANCELLED | Per-call context cancelled |
| REMOTE_TRANSPORT_ERROR | No trustworthy remote HTTP response |
| REMOTE_RESPONSE_INVALID | Malformed response, invalid ProblemDetails, wrong private type, key mismatch or invalid response shape |
| PROVIDER_INTERNAL | Unexpected Provider failure |
| PROVIDER_SHUTTING_DOWN | Provider is draining and no longer admits calls |

A Provider transport/protocol failure has no remote Owner Outcome. Host/Runtime maps it to Runtime UNKNOWN; it MUST NOT fabricate a valid Owner FAILED response or Evidence. A valid remote ProblemDetails response is returned in result; the Host Owner adapter decides its declared Owner Outcome.

## 8. Correlation, concurrency and lifecycle

- Host owns admission and supplies negotiated limits; Provider enforces them.
- Host supplies an absolute deadline; Provider uses it as the upper bound for the Go call context.
- Provider does not generate Invocation IDs, Evidence, Capability decisions or Procedure routes.
- Responses may be out of order but MUST be complete single-line frames correlated by requestId.
- A valid remote HTTP error does not terminate the Provider process.
- Host stops admitting work, drains/cancels in-flight calls, then closes stdin.
- EOF after drain is the normal shutdown signal; v1 has no shutdown method.
- Provider exits normally after EOF and drain; Host may force-terminate after a bounded grace period.
- Unexpected process exit fails outstanding calls without a trustworthy remote response. Host/Runtime handles the result as UNKNOWN.
- Process restart MUST NOT replay any request. requestId is correlation, not an idempotency key.

## 9. Explicit v1 exclusions

Protocol v1 is JSON-only. It has no HostArtifactRef, binary file references, multipart channel, filesystem paths, Content-ID or base64 payload fields. NAS/NGAP codecs, HostArtifactRef staging and binary/multipart SBI payloads require a separately reviewed extension/version and are not first-slice dependencies.

Protocol v1 also does not define OAuth, TLS identity, NRF discovery, arbitrary service selection, arbitrary HTTP methods/paths, generic curl requests, public ToolCatalog publication, Capability authorization or Provider-side workflow orchestration.

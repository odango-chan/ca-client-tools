# Generic SBI Executor

Status: **IMPLEMENTED / LOCAL_ACCEPTANCE_PASS.** Package
`internal/sbi/executor` plus its supporting schema validator
(`internal/schemacheck`). Mock h2c tests only; no live-NF acceptance was run.

## Purpose

The executor runs one trusted SBI operation descriptor through the existing
shared transport (`internal/sbi.Client`) and the Payload Engine
(`internal/sbi/payload`). It is NF/service-agnostic: an `Operation` is trusted
package content derived from the Operation Catalog, never Skill-authored input.
The caller supplies typed arguments only; the executor owns the service key,
method, path template, query names, trusted headers, media types and body
framing.

The executor does not retry. A mutation is attempted exactly once, even when an
idempotency contract is declared. It creates no Runtime Evidence, Procedure,
journal or callback state and does not wait for asynchronous notifications.

## API

```go
func New(client *sbi.Client, resolver ArgumentResolver, limits payload.Limits) (*Executor, error)
func (e *Executor) Execute(ctx context.Context, op Operation, args map[string]any) (Result, error)
```

`Operation`, `BodySpec`, `MultipartPart`, `Parameter`, `HeaderBinding`,
`Idempotency` and `Result` are the binding types documented in the package
source. The catalog layer maps trusted catalog entries onto these types, including
multipart request `parts`, multipart response `responseParts`, parameter
constraints and optional-query defaults. `Operation.Effect` is `READ` or
`MUTATION`.

`ArgumentResolver` returns a streaming reader for one Host-staged binary input
by Host-generated relative name. The executor never resolves a filesystem path
and never writes response bytes to disk.

Additional fields beyond the original executor brief (safe additive
extensions for the catalog wiring):

- `MultipartPart.Required` — declared parts default to optional; the payload
  engine enforces the flag.
- `Operation.ResponseParts []MultipartPart` — declared binary response parts
  for a `multipart/related` response. Without it the multipart response root
  JSON references cannot be decoded.
- `Idempotency.Mode == "REQUIRED"` — the key argument's top-level name is an
  allowed argument; the key is validated but never sent on the wire (there is
  no generic idempotency header contract in the catalog).

`BodySpec.ContentTypeParam` and `MultipartPart.ContentID` are reserved catalog
metadata. The payload engine always generates a unique Content-ID per
invocation and always emits `type=application/json` for the multipart root, so
configured values are ignored.

## Validation before any network I/O

Operation (fail closed, `*ConfigurationError`):

- operation ID, service key, `READ`/`MUTATION` effect, method in
  GET/POST/PUT/PATCH/DELETE;
- absolute path template, no `?#\%`, no relative/empty segments, balanced
  `{param}` tokens;
- every token has a `PathParameter` of type `string` and every `PathParameter`
  appears in the template; parameter names are unique across path and query;
- parameters have supported types (`string|integer|boolean`) and consistent
  constraints;
- header bindings use valid names, reject reserved transport headers
  (`Authorization`, `Host`, `Content-Length`, `Connection`,
  `Transfer-Encoding`, `Cookie`, `Content-Type`, proxy/upgrade/TE/trailer
  headers) and reject CR/LF or control characters;
- body kind (`json|binary|multipart`) matches its media type, schemas and part
  declarations; declared part count fits `MaxParts`;
- `MUTATION` requires `Idempotency`; `READ` must not declare one; `REQUIRED`
  idempotency needs a valid JSON-pointer key argument;
- response media types are valid and unique; a response schema requires a JSON
  or `multipart/related` declaration; multipart response declarations need
  matching `ResponseParts`.

Arguments (fail closed, `*InputError`):

- unknown argument names are rejected; only declared path/query parameters,
  `body`, `payloads` (multipart only) and the idempotency key argument are
  accepted;
- path values are substituted with `url.PathEscape` and rejected when they
  contain `/`, `?`, `#`, `\`, control characters or are `.`/`..`;
- query values are type-checked and constrained (minimum/maximum for integers,
  minLength/maxLength/pattern/enum for strings); trusted defaults on optional
  query parameters are applied when omitted;
- `args["body"]` is required for JSON and multipart bodies (JSON `null` is a
  valid present value), and optional `null` for query parameters means absent
  (a trusted default is applied when declared);
- JSON/multipart root bodies are validated against `Body.Schema`;
- binary body and multipart part values are non-empty Host payload names;
  multipart `args["payloads"]` keys must be declared parts and required parts
  must be present.

## Body handling

- JSON: `payload.BuildJSON` after schema validation.
- Binary: the resolver is opened once; the already-open stream and its size are
  passed to `payload.BuildBinary` (no buffering, exactly-once open).
- Multipart: each supplied part is opened once, then
  `payload.BuildMultipartRelated` writes the JSON root first, generates a
  unique Content-ID per binary part and inserts it into the declared
  `RefToBinaryData` JSON pointer. The multipart media type and boundary come
  from the payload engine; caller arguments cannot set them.

## Response handling

- Only declared response media types are accepted; a non-empty body without a
  declared media type or `Content-Type` is a `*ProtocolError`.
- JSON: one strictly decoded value (`json.Number`, duplicate object keys and
  trailing data rejected), optionally validated against `ResponseSchema`.
- `multipart/related`: decoded by `payload.DecodeResponse` through an
  in-memory, bounded `AttachmentSink`; root JSON is validated and attachments
  are returned with Content-ID, media type, size, SHA-256 and bytes.
- Other media types: one bounded in-memory body (`Result.Body`).
- `Result.Headers` contains only a fixed safe allowlist (`Location`,
  `Retry-After`, `3gpp-Sbi-*` metadata headers). Cookies, credentials and the
  configured header token are never projected.
- 2xx: `Result` with actual HTTP status (202/204 preserved, no polling).
- Non-2xx with a well-formed `application/problem+json` whose `status` matches
  the HTTP status and whose `cause` matches `^[A-Z][A-Z0-9_]{0,63}$`:
  `*RemoteError{HTTPStatus, Problem, Cause}`. Title/detail/instance are retained
  on the `ProblemDetails` value but never included in error text.
- Any other non-2xx response: `*ProtocolError`. Remote bodies are not logged.

## Error taxonomy

`*InputError`, `*ConfigurationError`, `*ProtocolError`, `*RemoteError`,
`*TransportError`. Transport failures and cancellation/timeout preserve
`errors.Is(err, context.Canceled)` / `errors.Is(err, context.DeadlineExceeded)`.

## Limits

`New` composes the supplied `payload.Limits` against the shared transport
defaults (`payload.ComposeLimits`). A caller that lowers
`sbi.Limits.MaxRequestBytes`/`MaxResponseBytes` must lower the payload limits to
the same bounds. Oversized requests fail before the transport; oversized
responses are a `*ProtocolError`.

## Tests (LOCAL_ACCEPTANCE_PASS)

Commands (Windows-native, Go 1.26.6):

```text
GOWORK=off go test ./internal/schemacheck/... ./internal/sbi/executor/... ./internal/sbi/serviceconfig/... -count=1
GOWORK=off go vet ./internal/schemacheck/... ./internal/sbi/executor/... ./internal/sbi/serviceconfig/...
GOWORK=off go build ./internal/schemacheck/... ./internal/sbi/executor/... ./internal/sbi/serviceconfig/...
```

Covered with loopback-only, deterministic h2c tests (`http.Protocols`
unencrypted HTTP/2, the same native prior-knowledge path used by the transport
tests):

- GET path escaping and query encoding, trusted header delivery, token profile
  header injection while caller arguments cannot add headers;
- JSON POST request/response and response schema mismatch;
- binary request from an in-memory fake resolver, single binary response;
- multipart request: root first, generated unique Content-IDs, root JSON
  `RefToBinaryData` references inserted, multiple mixed-type parts;
- multipart response attachment decoding (Content-ID, media type, size,
  SHA-256, bytes) and missing declared part rejection;
- ProblemDetails vs malformed problem/non-problem errors;
- cancellation and request timeout;
- mutation without idempotency rejected before network I/O, missing
  idempotency key rejected, no automatic retry;
- reserved header binding and header CRLF injection rejected, unknown
  arguments rejected, unsafe path values rejected;
- oversized request/response limits.

## Not run / not claimed

- Live OpenAMSM/NF end-to-end acceptance: **NOT_RUN**.
- Catalog generation from OpenAPI and the Provider process: owned by the
  catalog/Provider workstream, **NOT_RUN** here.
- Real-NF multipart operations (for example N1/N2 transfer): **NOT_RUN**.

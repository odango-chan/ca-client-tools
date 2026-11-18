# Trusted Service Configuration v1

Status: **IMPLEMENTED / LOCAL_ACCEPTANCE_PASS.** Package
`internal/sbi/serviceconfig`. YAML loading and validation tests only; no
network and no live-NF acceptance.

## Provider integration target

P0 implements one trusted service registry keyed by the Catalog's `serviceKey`
for typed, generic and metrics dispatch. `providerapp.loadTrustedServices`
translates the typed OpenAMSM deployment settings into the registry and also
loads the optional trusted `CA_PROVIDER_SERVICE_CONFIG` file (this loader) for
additional h2c services and token profiles. One `internal/sbi` transport
instance is built from the merged configuration and shared by the typed MAS
client, the generic executor and the typed metrics handlers. Standard SBI
services use the `sbi-h2c` profile; the typed `CA_OPENAMSM_*_METRICS_ORIGIN`
settings register their services with the explicit `metrics-http1` profile so
the metrics HTTP/1.1 behavior is preserved without a second HTTP client. This
loader file itself remains h2c-only (`protocol: h2c`). Incomplete, duplicate or
conflicting configuration fails Provider initialization. Origins and secrets
come from trusted deployment configuration; the registry is server-side only.
See [WORK](WORK.md).

## Purpose

The loader reads the deployment's trusted multi-service SBI configuration and
returns an `sbi.Config` accepted by `sbi.NewClient`. Configuration is trusted
package/deployment content: a Skill, caller or operation argument can never
supply or override an origin, API root, header token profile or token.

## Schema version 1

```yaml
version: 1
services:
  namf-comm:
    nfType: AMF
    origin: http://amf.sbi.example:7777
    apiRoot: /namf-comm/v1
    protocol: h2c            # h2c only for now
    headerTokenProfile: openamsm-query-token
    discovery: none          # reserved; only "none", null or {} is accepted
    requestTimeout: 10s
limits:
  requestTimeout: 10s
  dialTimeout: 5s
  maxRequestBytes: 16777216
  maxResponseBytes: 16777216
  maxIdleConns: 64
  maxIdleConnsPerHost: 8
  maxConnsPerHost: 32
  idleConnTimeout: 90s
headerTokenProfiles:
  openamsm-query-token:
    header: x-openamsm-query-token
    tokenFile: /etc/ca/secrets/openamsm-query-token   # mutually exclusive with tokenEnv
    # tokenEnv: CA_OPENAMSM_QUERY_TOKEN
```

## API

```go
func Load(path string, lookupEnv func(string) (string, bool)) (sbi.Config, error)
func LoadDocument(path string, lookupEnv func(string) (string, bool)) (Document, error)
func Redacted(config sbi.Config) map[string]any
func (d Document) Redacted() map[string]any
```

`Load` returns exactly the `sbi.Config` the shared transport consumes. A
per-service `requestTimeout` cannot be applied per service by the v1 transport,
which uses one global `limits.requestTimeout`; `LoadDocument` therefore exposes
the validated per-service values (and `nfType`) as metadata for callers that can
use them. This is a deliberate extension, not a transport feature.

`lookupEnv == nil` uses `os.LookupEnv`.

## Strictness rules

The loader rejects:

- unknown fields at any level (`yaml.KnownFields(true)`), including `token`,
  `tls`, OAuth or token-endpoint style fields, so no literal secret can be
  embedded in the file;
- duplicate mapping keys, YAML aliases and YAML merge keys (`<<`);
- multiple YAML documents;
- a missing or unsupported `version` (only `1` is accepted);
- empty `services`, an unsupported `protocol` (anything other than `h2c`), a
  malformed `nfType`, non-empty or unknown `discovery` content;
- invalid durations (must be strings such as `10s`), out-of-bounds limits,
  values rejected by `sbi.NewClient`, and origins with a path, query, fragment,
  userinfo or a non-`http` scheme;
- a service that references an unknown header token profile;
- a token profile that sets both `tokenFile` and `tokenEnv`, neither, a
  relative `tokenFile`, a missing/unreadable file, a file over 4 KiB, or an
  empty/multi-line token (a single trailing newline is trimmed and allowed);
- an invalid environment-variable name or an unset environment variable.

Secrets are never included in error text or in the redacted views; the redacted
views replace every token value with `<redacted>`.

## Test coverage (LOCAL_ACCEPTANCE_PASS)

```text
GOWORK=off go test ./internal/sbi/serviceconfig/... -count=1
GOWORK=off go vet ./internal/sbi/serviceconfig/...
```

Covered: valid configurations with a file token and with an environment token;
unknown fields; duplicate keys; aliases; merge keys; multiple documents; bad
origins and API roots; missing token profile; missing/relative/oversized token
file; multi-line and empty tokens; mutually exclusive token sources; unsupported
version; TLS/OAuth/unknown field rejection; per-service timeout metadata;
redaction proving the token substring never appears. Tests use `t.TempDir` and do
not require network access.

## Not run / not claimed

- Live-NF acceptance: **NOT_RUN**.
- TLS/mTLS, OAuth2, NRF discovery: **NOT_IMPLEMENTED** by design in this slice.
- Per-service timeout application in the transport: **NOT_IMPLEMENTED**; values
  are validated and exposed by `LoadDocument` only.

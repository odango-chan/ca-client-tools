# CA Client Tools CLI Contract

Status: **DESIGN ACCEPTED — IMPLEMENTATION PENDING**  
Contract version: **CLI v1**  
Scope: local, non-interactive process interface used by the CA Agent Host and
available for operator diagnostics.

This document is normative for the CLI surface. It does not redefine the
OpenAMSM HTTP/SBI contracts or grant any caller Agent/Tool authorization.

## 1. Design decisions

1. Use **Cobra** (`github.com/spf13/cobra`) for the command tree, help,
   argument parsing and shell completion. Go's standard-library `flag` package
   is suitable for simple single-command programs, but does not provide the
   nested command tree and completion experience required here. Cobra is a
   widely used Go community convention, not an official Go standard-library
   component.
2. Keep Cobra confined to `internal/cli`. OpenAMSM client packages must not
   import Cobra, pflag or any CLI package.
3. Keep the CLI non-interactive and machine-oriented. A normal command emits
   exactly one JSON success document to stdout, or exactly one JSON error
   document to stderr. It never prompts.
4. Use command/flags for operation selection and non-sensitive controls.
   Send subscriber lookup values to `smf mas query` as one JSON object on
   stdin, never as process arguments.
5. Inject endpoint origins and credentials only through trusted deployment
   configuration. Never accept arbitrary endpoint URLs or authentication
   tokens from command arguments or stdin.
6. Keep the CLI a thin process adapter over typed client operations. It does
   not own ToolCatalog visibility, Capability authorization, Runtime state,
   business lifecycle, Evidence, Outcome or Reconcile.

Do not add Viper, a second CLI framework, a custom shell parser, an interactive
prompt framework or a general-purpose command/plugin system for this scope.
The Go standard library remains responsible for JSON encoding, context,
process I/O and the underlying client implementation.

## 2. Command tree

The executable name is `ca-client-tools`. Command names are lowercase;
multiword flags use kebab-case. Do not add aliases in CLI v1.

| Command | OpenAMSM operation | Input |
|---|---|---|
| `ca-client-tools amf ue list` | AMF `GET /ue-info` | `--page`, `--page-size` |
| `ca-client-tools amf gnb list` | AMF `GET /gnb-info` | `--page`, `--page-size` |
| `ca-client-tools smf pdu list` | SMF `GET /pdu-info` | `--page`, `--page-size` |
| `ca-client-tools smf mas query --mas <ID>` | One of the eight SMF MAS QueryContext SBI paths | JSON request object on stdin |

MAS identifiers are exactly `SC`, `PA`, `UPN`, `SSM`, `IAM`, `UPS`,
`CA` and `SAA`.

Metrics list commands use zero-based `--page` (default `0`) and
`--page-size` (default `100`, valid range `1..100`). One invocation returns
one source page only. The Host owns any multi-page traversal and must not infer
an atomic snapshot across pages or endpoints.

The old spellings `amf ue-info`, `amf gnb-info` and `smf pdu-info` are
superseded by the resource/action command form above. This is a demo-stage
interface; no compatibility aliases are required.

## 3. Input contract

### 3.1 Command-line arguments

Arguments identify the command and carry non-sensitive options only:

- `--mas` selects one of the eight MAS identifiers.
- `--page` and `--page-size` control one metrics page.
- `--help` / `-h` and `--version` are standard root/command controls.

Unknown flags, unexpected positional arguments, duplicate singleton flags,
invalid enum values and out-of-range pagination values fail before network
access. Cobra must not print its default usage/error text for an operational
failure; the CLI maps the failure to the JSON error contract in §5.

Do not add flags for `--supi`, `--imsi`, UE IP addresses, SBI origin, metrics
origin or query token.

### 3.2 SMF MAS JSON input on stdin

`smf mas query` reads exactly one JSON object from stdin. The object contains
the OpenAMSM v0.10 request fields:

```json
{
  "supi": "imsi-001010000000001",
  "pduSessionId": 5
}
```

Allowed fields:

| Field | Type | Meaning |
|---|---|---|
| `supi` | string | SUPI using the OpenAMSM `imsi-` form |
| `imsi` | string | 1–15 decimal IMSI digits; request-only alias |
| `pduSessionId` | integer | Source contract range 0–255 |
| `ueIpv4` | string | UE IPv4 lookup/consistency key |
| `ueIpv6` | string | UE IPv6 lookup/consistency key |

The existing OpenAMSM v0.10 key-combination rules remain authoritative:

- SUPI and/or IMSI must be supplied together with `pduSessionId`; when both
  identities are supplied they must identify the same UE.
- One or both UE IP keys may be supplied.
- Identity plus PSI may be combined with one or both UE IP keys for consistency
  checking.
- The selected `--mas` identifier is not repeated in the JSON body.

Input is limited to 64 KiB. It must be valid UTF-8 and exactly one JSON object.
Reject duplicate object keys, unknown fields, malformed values, trailing
non-whitespace data and multiple JSON documents. Do not silently ignore
unrecognized input.

Only the MAS query command consumes stdin in CLI v1. Metrics list commands do
not read stdin and do not wait for interactive input.

### 3.3 Trusted configuration

The following values are deployment configuration, not Skill or Agent input:

| Environment variable | Used by |
|---|---|
| `CA_OPENAMSM_AMF_METRICS_ORIGIN` | AMF UE and gNB list |
| `CA_OPENAMSM_SMF_METRICS_ORIGIN` | SMF PDU list |
| `CA_OPENAMSM_SMF_SBI_ORIGIN` | SMF MAS query |
| `CA_OPENAMSM_QUERY_TOKEN` | SMF MAS query |

Origins must pass the existing strict client configuration validation.
Credentials, arbitrary URLs and query tokens must never be accepted as command
arguments or request JSON fields. Do not print environment values.

## 4. Success output

A successful operation writes exactly one UTF-8 JSON document followed by a
newline to stdout:

```json
{
  "data": {
    "masId": "PA",
    "status": "AVAILABLE",
    "key": {
      "supi": "imsi-001010000000001",
      "pduSessionId": 5
    },
    "context": {}
  }
}
```

The `data` value preserves the typed client result and source semantics. For
metrics list commands, `data` contains the validated source page, including
`items` and `pager`. Do not rename source fields or turn them into Provider,
Skill or business-lifecycle facts in this repository.

An HTTP 200 MAS response with `status: "UNAVAILABLE"` is still a successful
client invocation: exit code 0 and a normal `data` envelope. It must not be
converted into a CLI error or a negative business conclusion.

On success, stderr is empty. Do not print progress, banners, logs or human
summaries to stdout. CLI v1 has JSON as its only operational output format;
do not add a separate human/table renderer.

The MAS response `key` and metrics pages may contain subscriber identifiers,
IP addresses, location or topology data. CLI stdout is Host-private process
output. The Host must validate the echoed lookup binding, redact/project
subscriber identity and construct the approved Agent-visible Observation
before returning any result to the Agent. Never forward raw CLI stdout directly
to the Agent.

## 5. Error output and exit codes

An operational failure writes exactly one JSON document to stderr and writes
nothing to stdout:

```json
{
  "error": {
    "code": "OPENAMSM_HTTP_ERROR",
    "httpStatus": 503,
    "cause": "SERVICE_UNAVAILABLE"
  }
}
```

The error object may include safe, stable fields such as `httpStatus`,
`cause` or `field`. It must not include raw request values, SUPI/IMSI, IP
lookup values, tokens, response bodies, stack traces or unfiltered remote
error text. Error codes are stable machine identifiers; prose diagnostics are
not part of the contract.

| Exit code | Error codes | Meaning |
|---:|---|---|
| `0` | — | Operation succeeded, including MAS `UNAVAILABLE` |
| `2` | `INVALID_ARGUMENT`, `COMMAND_UNSUPPORTED` | Command, flag or stdin input invalid |
| `3` | `CONFIGURATION_MISSING`, `CONFIGURATION_INVALID` | Required trusted configuration absent or invalid |
| `4` | `OPENAMSM_HTTP_ERROR`, `OPENAMSM_TRANSPORT_ERROR` | Remote HTTP or transport failure |
| `5` | `OPENAMSM_PROTOCOL_INVALID`, `INTERNAL_ERROR` | Invalid source response or local internal failure |

`--help` and `--version` write their human-readable text to stdout and exit
with code 0. Operational failures must not also print Cobra's usage text.
Signal termination follows the host operating system's conventional process
status; the Host must still treat cancellation/timeout as an incomplete
invocation, not as a business result.

When an active command receives `SIGINT` or `SIGTERM` (where supported), the
CLI cancels the root request context and lets the in-flight HTTP request stop.
A signal-driven termination is not a source response: emit neither a success
`data` envelope nor a synthetic `UNAVAILABLE` result. The process may exit
without a JSON envelope and uses the platform's conventional signal status.
The Host must classify that invocation as cancelled/incomplete. Do not add a
signal or cancellation condition to the MAS `AVAILABLE`/`UNAVAILABLE`
business status model.

## 6. Process and security requirements

- The Host invokes a fixed executable with an argument vector; never construct
  a shell command or allow arbitrary executable/subcommand selection from a
  Skill.
- The Host writes the MAS JSON object to stdin and supplies only the required
  trusted environment variables.
- Do not place subscriber identity, UE IP lookup keys, credentials or tokens
  in process arguments.
- The CLI uses bounded input, HTTP timeouts and response sizes. It does not
  retry requests implicitly in CLI v1.
- The executable creates a root context cancelled by process termination
  signals supported by the target platform (at minimum interrupt and, on
  Unix-like targets, `SIGTERM`). Every outbound HTTP request must use that
  context, so cancellation aborts the request rather than allowing it to finish
  in the background.
- A Host deadline/cancellation is process control, not a remote MAS status.
  The Host launches the fixed executable directly (never through a shell),
  applies its own invocation deadline, and terminates the child when that
  context ends. A cancelled/timed-out invocation is incomplete even if a
  process race produces exit code 0 or partial stdout; the Host must discard
  partial output and must not parse it as a business result.
- The Host process wrapper should bound process and pipe shutdown (for Go
  callers, use a context-bound process invocation and a non-zero bounded
  wait/termination policy). Do not let a child or inherited pipe keep an
  invocation alive indefinitely.
- A timeout raised by the client's own bounded HTTP timeout is an operational
  transport failure and maps to `OPENAMSM_TRANSPORT_ERROR` (exit 4). A Host
  cancellation or OS termination is not converted to that error envelope.
- Never log tokens, subscriber identifiers, raw query JSON, raw response
  bodies or remote error bodies.
- stdout and stderr are separate protocol channels. No debug output may be
  mixed into stdout.
- The CLI does not persist state, create lifecycle identifiers, register
  callbacks/routes, or own continuation. Those are Host/Runtime concerns.

## 7. Go implementation structure

Use Cobra only in the CLI layer:

```text
cmd/ca-client-tools/
  main.go

internal/cli/
  root.go
  amf.go
  smf.go
  input.go
  output.go

internal/openamsm/
  amf/
  metrics/
  smf/mas/
```

The exact file split may be adjusted without changing this contract.

Implementation requirements:

- Pin a reviewed stable Cobra version in `go.mod` and commit `go.sum`.
  Do not depend on an unpinned latest version.
- Do not add Viper or another configuration framework; the existing explicit
  environment-variable contract is sufficient.
- Construct commands with injected stdin/stdout/stderr and dependencies so
  tests do not require global process state.
- Use Cobra's command tree and standard flag parsing; keep command handlers
  thin and call typed OpenAMSM clients.
- Centralize success/error serialization and exit-code mapping.
- Keep `os.Exit` at the executable boundary only. Client and CLI packages
  return errors/results; they must not terminate the process.
- Do not import Cobra from `internal/openamsm/**`.

## 8. Required tests and acceptance

The implementation is not complete until tests cover:

1. Exact command tree and generated help for root and each leaf command.
2. Unknown commands, unknown flags, extra positional arguments and invalid
   flag values.
3. Required MAS identifier and all existing v0.10 query-key combinations.
4. Stdin JSON: valid request, empty input, malformed JSON, duplicate keys,
   unknown fields, invalid UTF-8, trailing JSON, multiple documents and size
   limit.
5. Sensitive values never appear in process arguments, error output or logs.
6. Success envelope and exactly one JSON document on stdout.
7. Error envelope, exactly one JSON document on stderr, empty stdout and the
   correct exit code for every error class.
8. MAS `UNAVAILABLE` remains exit code 0 and appears inside `data`.
9. Metrics page output preserves `items` / `pager` and the one-page-per-call
   behavior.
10. Injected writers/readers and environment configuration work without
    touching global process state.
11. `gofmt`, `go test ./...`, `go vet ./...` and native build pass using the
    repository's declared Go toolchain.

12. Cancellation: a cancelled command context aborts an in-flight HTTP request;
    a signal-driven termination cannot produce a success envelope or be
    translated into MAS `UNAVAILABLE`; Host-side timeout/cancellation discards
    partial stdout and is recorded as an incomplete invocation.

Mock HTTP tests prove client behavior only. OpenAMSM source build, MAS source
tests and live-NF acceptance remain separate evidence levels.

## 9. Host integration contract

The Host adapter is the only supported production caller for Runtime/Agent
workflows. It must:

1. Map an admitted Tool invocation to one fixed CLI command.
2. Validate and serialize the typed request into stdin.
3. Inject endpoint origins and credentials from trusted Host configuration.
4. apply an invocation deadline and cancellation.
5. parse the success envelope only when exit code is 0.
6. parse the error envelope only when exit code is nonzero.
7. validate the MAS response echo against the Host's trusted UE/PSI binding.
8. keep raw subscriber identifiers and raw metrics pages Host-private.
9. project only the declared Tool output Schema to the Agent.
10. launch the executable directly with a fixed executable path and argument
    vector (never a shell), pass the MAS request only through stdin, and apply
    an invocation deadline/cancellation to the child process. On cancellation
    or timeout, discard partial stdout and treat the invocation as incomplete,
    regardless of any apparent partial response or racing exit status.

The existence of a CLI command does not register a Tool, make it Agent-callable,
or grant a Capability. AMF/SMF metrics list commands remain Host-internal by
default. SMF MAS operations are Agent-callable only when separately declared
and admitted through the Host/Runtime Tool and Capability contracts.

# DeepSeek Harness handoff — CA Client Tools CLI v1

Status: **IMPLEMENTATION STAGED — CI ACCEPTANCE PENDING**
Repository: `odango-chan/ca-client-tools`
Base branch: `feat/openamsm-client-tools-foundation-20261116`
Current handoff scope: **CLI v1 only**
Normative contract: [`docs/cli-contract.md`](cli-contract.md)

This file is the complete task brief. Start in Goal mode and execute the task
end-to-end within the scope below. Do not spend the task redesigning the
architecture or asking for approval for decisions already frozen here.

## Goal

Migrate the existing pre-v1, Go `flag`-based CLI to the accepted CLI v1
contract using Cobra. Preserve the existing typed OpenAMSM client behavior.
Deliver a deterministic, non-interactive, machine-readable process interface
that the CA Agent Host can invoke safely as a fixed local executable.

The goal is complete only when the implementation, required tests, Go
formatting, tests, vet and native build all pass on the repository's declared
Go toolchain, and the resulting changes are committed to this branch.

## Read first

Read these files in this order:

1. `AGENTS.md` — repository rules and ownership boundaries.
2. `docs/cli-contract.md` — normative CLI v1 process contract.
3. `docs/architecture.md` — component and ownership boundaries.
4. `docs/openamsm-interface-inventory.md` — source-backed remote API scope.
5. `docs/TRACEABILITY.md` and `docs/WORK.md` — implementation and evidence status.

The OpenAMSM source snapshot and exact source references are pinned in
`docs/openamsm-interface-inventory.md`. Do not substitute Open5GS behavior,
another source version, or assumptions from a different environment.

## Staged implementation and verification

- Cobra command tree is implemented in `internal/cli/run.go`.
- Strict SMF context JSON stdin parsing includes UTF-8, size, duplicate-key, unknown-field and trailing-document rejection.
- JSON success/error envelopes, stable exit classes, sensitive-value redaction and cancellation handling are implemented.
- Fixture tests cover CLI help/version, MAS AVAILABLE/UNAVAILABLE/404 and invalid input, plus metrics list operations.
- A compile-time `emitData` signature mismatch in the MAS success path was corrected in the latest commit; fresh CI is required.

## In-scope implementation

The following CLI v1 implementation has been staged. Review and complete only the following:

1. Review the staged Cobra CLI command tree and close any remaining gaps against `docs/cli-contract.md`.
2. Implement exactly this command tree, with no compatibility aliases:
   - `ca-client-tools amf ue list`
   - `ca-client-tools amf gnb list`
   - `ca-client-tools smf pdu list`
   - `ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>`
3. Use zero-based metrics pagination: `--page` defaults to 0;
   `--page-size` defaults to 100 and accepts only 1–100.
4. Read the SMF context lookup request from exactly one JSON object on stdin.
   Enforce the 64 KiB limit, UTF-8 validity, one-document-only rule, duplicate
   key rejection, unknown-field rejection and all v0.10 key-combination rules.
   Sensitive lookup values must never appear in argv.
5. Read endpoint origins and query token only from the trusted environment
   variables specified in `docs/cli-contract.md`. Do not add Viper or another
   configuration framework.
6. Emit exactly one `{"data": ...}` JSON document to stdout on success.
   Emit exactly one `{"error": ...}` JSON document to stderr on operational
   failure; leave the other stream empty. Preserve SMF context `UNAVAILABLE` as a
   successful exit-0 result.
7. Implement the documented stable exit-code mapping, safe error fields,
   help/version exceptions, and no-progress-output rule.
8. Propagate process cancellation to every in-flight HTTP request. Do not
   retry. A cancellation or signal is not SMF context `UNAVAILABLE` and must not
   produce a success envelope.
9. Keep Cobra and all CLI concerns inside `internal/cli`. Keep
   `internal/openamsm/**` independent of Cobra and CLI packages.
10. Refactor the CLI entry point as needed so stdin, stdout, stderr,
    environment lookup, context and typed client dependencies are injectable
    for tests. Keep `os.Exit` at the executable boundary only.

## Frozen architecture and explicit exclusions

Do not change these ownership boundaries:

- `env-openamsm`: OpenAMSM source snapshot, build, configuration and NF
  process lifecycle.
- `ca-client-tools`: typed OpenAMSM client operations and the fixed local CLI.
- `ca-agent`: Host process adapter, trusted configuration injection,
  default-Tool registration and Agent-versus-Host-only exposure policy.
- `ca-runtime`: callable admission, Capability authorization, execution
  identity, Procedure progression, Outcome, Evidence and Reconcile.
- `ca-skills`: business Procedures and declared callable dependencies.

This task is read-only client/CLI work. The only remote SMF SBI operations
in scope are the eight MAS QueryContext operations (SC, PA, UPN, SSM, IAM,
UPS, CA and SAA). AMF UE/gNB and SMF PDU metrics commands remain Host-internal
by default. CLI command existence does not grant Tool visibility or
Capability authorization.

Explicitly do **not** implement or migrate:

- NW Agent Workflow Planning / Replan;
- NW Agent Nsmf PDUSession Facade (create/modify/release);
- NW Agent MAS Orchestration Invoke or SM Context Update Sync;
- NW Agent Namf Relay;
- any mutation, orchestration, callback, route registration, lifecycle,
  journal, Evidence, Outcome or Reconcile behavior;
- Host adapter or Runtime integration code in other repositories;
- Open5GS compatibility, a generic `fivegc/` package, UPF
  `/session-info`, or inferred SMF/UPF N4 SEID joins;
- changes to the imported OpenAMSM source snapshot.

These adjacent NW Agent APIs exist in the pinned OpenAMSM source, but they
have separate write, workflow, authorization and relay semantics. Their
presence in the source tree is not authorization to add them to this CLI.

Do not redesign the accepted CLI contract. If the current typed client code
cannot satisfy a normative v0.10 behavior, first establish the exact mismatch
from the pinned OpenAMSM source and tests. Do not silently change source
semantics; report the conflict with file/line evidence and stop only the
blocked item.

## Cancellation and timeout rules

- The CLI root context must be cancelled by supported process termination
  signals (at minimum interrupt and Unix-like `SIGTERM`).
- Every outbound HTTP request must be bound to that context.
- Host-side deadline/cancellation is process control. The Host will invoke a
  fixed executable directly, with an argument vector and no shell, under a
  bounded process deadline.
- If the process is cancelled or terminated, partial stdout is discarded and
  the invocation is incomplete, regardless of a racing exit status.
- Do not map cancellation to SMF context `UNAVAILABLE`, a negative business fact,
  or a successful `data` result.
- A timeout from the client's own bounded HTTP timeout is an operational
  transport error under the CLI contract; Host cancellation is not.
- Do not persist state, resume operations, or introduce background workers.

## Required tests

Implement every case in `docs/cli-contract.md` §8. In particular verify:

- exact Cobra command tree and help for root and leaf commands;
- unknown commands/flags, extra positional args, invalid flags and missing
  SMF context type;
- all supported and rejected OpenAMSM v0.10 query-key combinations;
- stdin empty/malformed/invalid UTF-8/duplicate-key/unknown-field/trailing
  JSON/multiple-document/oversize cases;
- sensitive values absent from argv, logs and error output;
- success and error envelopes, stream separation and every exit code;
- SMF context `UNAVAILABLE` remains exit 0 and is not treated as a business negative;
- source-native metrics page preservation and pagination bounds;
- dependency injection without global process state;
- cancellation of an in-flight HTTP request and the absence of a success
  envelope after cancellation;
- `gofmt`, `go test ./...`, `go vet ./...` and native build.

Mock HTTP tests prove client/CLI contract behavior only. They do not prove
OpenAMSM source build or live-NF acceptance.

## Execution sequence

1. Inspect the existing CLI and typed client packages; record the baseline.
2. Implement Cobra command construction and dependency injection.
3. Implement strict SMF context stdin decoding and validation.
4. Implement centralized JSON output, safe error mapping and exit codes.
5. Implement cancellation propagation.
6. Add/adjust tests until the full required matrix passes.
7. Run all required commands with the declared Go 1.26.2 toolchain. Do not
   temporarily lower or override the `go.mod` language/toolchain declaration
   to make CI pass.
8. Update `docs/WORK.md` and `docs/TRACEABILITY.md` with actual results,
   separating local test, CI, source-build and live-NF evidence.
9. Commit the implementation to the current feature branch and report the
   commit SHA and test output. Do not merge PR #1.

## Completion report

Return:

- summary of implemented CLI v1 behavior;
- files changed;
- commit SHA;
- exact Go version used;
- exact commands run and pass/fail results;
- any test not run and why;
- confirmation that typed client semantics and all out-of-scope boundaries
  were preserved.

Do not claim acceptance levels that were not actually executed.

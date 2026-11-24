# Goal — Replace duplicated CA NAS / NGAP codecs with Free5GC

## Current execution tracking

- SBI multipart byte-preservation prerequisite: [ca-agent-demo PR #245](https://github.com/odango-chan/ca-agent-demo/pull/245), head `f048e99a`. Locally verified: targeted 34 passed, integration 60 passed / 12 skipped (missing external binaries), full unit 2041 passed / 88 skipped. The two Actions runs still fail before any step (`runner_id=0`, `steps=[]`), which is CI_UNAVAILABLE, not a test result.
- Initial pinned-source inventory: [free5gc-upstream-audit.md](free5gc-upstream-audit.md). Verdict superseded by the follow-up below.
- Per-IE and vector audit (2026-11-24):
  - [NAS 5GSM coverage matrix](free5gc-nas-coverage-matrix-20261124.md): real message inventory, per-message / per-IE tables, IEToDoList paths and byte-level decode evidence. The ePCO validation gap in the current CA codec is recorded there.
  - [NGAP N2 Transfer coverage matrix](free5gc-ngap-coverage-matrix-20261124.md): used / future / not-needed root classification, per-IE tables, QoS flow evidence, unknown IE and strict-consumption findings.
  - [APER extension-bit investigation](free5gc-aper-extension-report-20261124.md): source facts, independent standard vectors, observed Free5GC behavior, impact and fix options.
  - [Codec selection decision](free5gc-codec-selection-20261124.md): REUSE FREE5GC WITH PATCH, facade contract and remaining blockers.
- The facade and the Python worker remain unimplemented by design. Do not begin consumer migration until the blockers in the selection document are closed.


## Objective

Move CA's protocol implementation onto the pinned, maintained Free5GC Go
modules. Keep protocol implementation in one place, expose a small Go facade
for Go consumers, and expose the same implementation to the Python Host through
one long-lived worker process owned by the existing \`ca-client-tools\`
executable.

This Goal is an implementation task across three repositories:

- \`odango-chan/ca-client-tools\`
- \`odango-chan/ca-agent-demo\`
- \`odango-chan/env-5gc-sim\`

Do not change \`env-openamsm\` business implementation, frozen CA-SL contracts,
Provider v2 protocol, ToolCatalog, Compiler or Skill Studio.

## Non-negotiable architecture

1. Use direct Go module dependencies:
   - \`github.com/free5gc/ngap v1.2.0\`, source commit
     \`7c6125004f5fa914412a4a87de3d48f0ca2ab1c8\`.
   - \`github.com/free5gc/nas v1.3.1\`, source commit
     \`ff5ea67383b7b5841247aab712889890b4c67d55\`.
2. Both pinned candidates declare Go 1.26.2, matching the current CA Go
   toolchain. Preserve Apache-2.0 notices.
3. \`ca-client-tools\` owns the CA-facing codec facade and worker command.
4. Go consumers call the facade directly in-process.
5. Python consumers use one long-lived \`ca-client-tools codec worker\`
   process. Do not start a process per message.
6. The worker uses framed stdin/stdout. JSON metadata and raw binary payload
   are separate segments. Never Base64-wrap the protocol wire payload.
7. Use upstream public protocol types and APIs. Do not copy Free5GC source,
   hand-maintain duplicate message structs or create a parallel ASN.1 model.
8. Standalone N2 Transfer requires an explicit root type selected by trusted
   SBI context (for example \`n2SmInfoType\`). Never guess a root type.
9. Preserve exact original N1/N2 bytes, Content-ID, Content-Type and digest.
   A decoded projection never replaces the original wire payload.
10. No C ABI, CGO, native shared library, Go plugin, dynamic module loader,
    \`ca-codec-host\` product, public network service or codec ToolCatalog entry.
11. Do not claim full R19 coverage. The pinned Free5GC versions are candidates
    whose exact message / IE / extension coverage must be measured.

## Phase 0 — Establish exact baselines

- Read the current main branches and open PRs in all three repositories.
- Record the exact source commit, Go / Python versions and relevant tests.
- Preserve unrelated work. Do not force-push or overwrite another task's
  branch.
- Keep the existing \`protocol/nas5gsm\` package until every consumer is
  migrated and cross-repository gates pass.

## Phase 1 — Fix SBI multipart byte preservation first

Repository: \`ca-agent-demo\`.

Inspect \`nsmf_http.py\`, \`nsmf_n1.py\` and
\`tests/unit/test_nsmf_http.py\`.

Required changes:

- Remove unrestricted \`strip(b"\\r\\n")\` and \`rstrip(b"\\r\\n")\` from
  multipart parsing.
- Parse MIME boundaries and remove only the framing CRLF immediately before
  the next boundary. Never mutate the payload bytes.
- Resolve \`n1SmMsg.contentId\` to the exact MIME part. Do not select the first
  part with Content-Type \`application/vnd.3gpp.5gnas\`.
- Resolve \`n2SmInfo.contentId\` to the exact N2 MIME part and validate its
  expected Content-Type.
- Reject absent references, missing parts, duplicate Content-IDs, duplicate
  JSON roots, mismatched Content-Types, malformed boundaries and truncated
  multipart bodies.
- Keep the exact original N1/N2 bytes and compute the digest over those exact
  bytes.

Add regression tests proving:

- N1 payloads ending in \`0D\`, \`0A\` and \`0D 0A\` survive byte-for-byte.
- A decoy N1 part placed first is not selected when JSON references a later
  Content-ID.
- Missing / wrong N1 Content-ID fails closed.
- N2 selection follows \`n2SmInfo.contentId\`, not MIME part order.
- Existing Create / Modify / Release multipart tests remain green.

Open a focused PR in \`ca-agent-demo\`. Do not mix Free5GC migration into this
multipart fix. Run the full relevant unit and integration tests and report
exact commands and results.

## Phase 2 — Complete the upstream coverage audit

Repository: \`ca-client-tools\`; audit both pinned upstream repositories.

For NAS, inventory every 5GSM message branch in \`message.ParseGSM\` and every
IE used by the AMF-SMF N1 SM boundary. For each message / IE, record separately:

- decode implementation and evidence;
- encode implementation and evidence;
- mandatory / optional / conditional status;
- recognized-but-unimplemented \`IEToDoList\` cases;
- unknown IEI behavior and whether framing can be determined;
- duplicate, truncation, length and enum validation;
- test vectors and known limitations.

For NGAP, inventory all PDU outcome / procedure / message dispatch branches,
all required N2 Transfer roots, nested IE types and extension containers.
Record separately:

- decode and encode support;
- unknown IE identity, criticality and raw-value preservation;
- unknown extension / open-type behavior;
- extensible SEQUENCE behavior;
- large \`SEQUENCE OF\` fragmentation;
- exact-input-consumption / trailing-octet behavior;
- malformed length, duplicate IE and criticality behavior;
- test vectors and known limitations.

At minimum, reproduce and classify the already observed risks:

- NGAP APER extensible SEQUENCE handling is marked TODO and treats the
  extension bit as absent.
- NGAP APER documents unsupported large \`SEQUENCE OF\` fragmentation.
- The NGAP unknown-IE helper does not establish lossless unknown-value
  round-trip preservation.
- NAS decoders can report recognized but unimplemented IEs through
  \`IEToDoList\`; unknown IEIs can fail when their framing is unknown.

Compare these results to the exact OpenAMSM / Mock AMF procedures and selected
3GPP TS 38.413 / TS 24.501 baselines. Do not treat source file counts or
dispatch branch counts as coverage proof.

Deliver a machine-readable inventory and a concise gap report. If a gap blocks
required real procedures, stop that path and propose an upstream contribution,
a narrowly pinned fork, or a maintained alternative. Do not write a replacement
APER/NAS parser in a consumer repository.

## Phase 3 — Build the CA Go facade

Repository: \`ca-client-tools\`.

- Add the two pinned modules to \`go.mod\` / \`go.sum\`.
- Add a small public package boundary for NAS, NGAP PDU and standalone N2
  Transfer operations.
- Use upstream message / IE types directly; do not duplicate their structs.
- Add explicit root-type registration for each N2 Transfer operation required
  by the selected SBI profile.
- Enforce input/output bounds, strict error handling and trailing-octet
  policy. Where upstream APIs cannot prove complete consumption, document
  the limitation and add a safe wrapper or block the unsupported operation.
- Keep raw wire bytes separate from decoded semantic projections.
- Do not remove \`protocol/nas5gsm\` yet.

Required Go tests:

- upstream golden vectors and independently generated vectors;
- all declared message / IE Encode and Decode cases;
- round-trip tests where supported;
- unknown IE, extension and unsupported-case tests;
- malformed/truncated/duplicate/trailing-data tests;
- fuzz tests for message and Transfer entry points;
- no panics on malformed input.

## Phase 4 — Add the Python worker to the existing executable

Repository: \`ca-client-tools\`.

Add a long-lived subcommand such as:

\`\`\`text
ca-client-tools codec worker
\`\`\`

Use a bounded frame:

\`\`\`text
uint32_be frame_length
"CCW1"                 4-byte magic
uint16_be protocol_version
uint8     frame_kind
uint8     flags
uint32_be metadata_length
uint32_be payload_length
metadata_json          UTF-8 JSON bytes
payload                raw binary bytes
\`\`\`

The frame length must equal all bytes after the prefix. Validate magic, version,
kind, reserved flags, segment lengths, request size, truncation and trailing
bytes. Include a request ID in metadata.

- Decode request: metadata identifies protocol and optional N2 root type;
  payload is raw wire bytes.
- Decode response: metadata carries the deterministic typed JSON projection;
  no binary payload.
- Encode request: metadata carries typed JSON message; no binary payload.
- Encode response: metadata carries status; payload is raw wire bytes.
- Error response: metadata carries a stable error code and safe diagnostic.

The JSON projection must preserve CHOICE, optional presence, ordered arrays,
integer / enum values and binary-valued fields. Prove Encode→Decode round trips
before advertising generic encoding. Do not hand-maintain a second full
protocol schema. If a type cannot be mapped losslessly, expose projection-only
behavior and keep its original bytes.

Add Python client code that starts the worker once, reuses it, correlates
request IDs, enforces timeouts and shuts it down cleanly. A worker crash must
not corrupt Host state. Test malformed frames and worker restart behavior.

## Phase 5 — Migrate Go and Python consumers

### env-5gc-sim

- Replace handwritten N2 APER bit readers with the CA Go facade.
- Replace the local NAS codec with the facade where used.
- Preserve Mock AMF state transitions, scenario semantics and business
  projections.
- Remove one-QoS-flow assumptions only when upstream structures and the
  simulator's business logic can represent multiple flows correctly.
- Keep unsupported protocol cases explicit and fail closed.

### ca-agent-demo

- Replace Python NAS / NGAP protocol parsers with worker calls only after
  Phase 1 and worker parity tests pass.
- Keep SBI multipart parsing, transport, N1/N2 retention, Owner routing and
  lifecycle behavior in Python Host.
- Delete duplicated Python codec code after all production call sites and tests
  use the worker.
- Never convert a typed projection back into wire bytes unless an Encode path
  has been independently qualified.

## Phase 6 — Cross-repository and real E2E qualification

Run and report separately:

1. \`ca-client-tools\`: build, vet, unit, fuzz seed and worker tests.
2. \`env-5gc-sim\`: Mock AMF tests using the direct Go facade.
3. \`ca-agent-demo\`: Host tests using the long-lived worker and exact
   multipart byte-preservation regressions.
4. Cross-language parity: Go and Python decode the same vectors to equivalent
   typed values; encode outputs match golden bytes where supported.
5. Real OpenAMSM / Mock AMF Create, Modify and Release E2E with exact N1/N2
   payload retention and independently captured protocol vectors.

Report four separate statuses:

- upstream source coverage;
- CA facade qualification;
- Go/Python cross-language qualification;
- live interoperability for the named deployment.

Do not claim full R19 support unless the complete itemized coverage evidence
actually proves it.

## Required final report

Include:

- repositories, branches, PR links and exact HEAD SHAs;
- pinned dependency versions and source commits;
- the per-message / per-IE coverage inventory;
- known upstream limitations and blocked items;
- files changed and duplicate implementations removed;
- exact test commands and PASS / FAIL counts;
- Go/Python parity evidence;
- real E2E evidence and explicit non-claims.

Keep PRs open for review. Do not merge, tag or publish without explicit
approval.
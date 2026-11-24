# Codec selection decision

**Date:** 2026-11-24  
**Inputs:** [NAS matrix](free5gc-nas-coverage-matrix-20261124.md),
[NGAP matrix](free5gc-ngap-coverage-matrix-20261124.md),
[APER report](free5gc-aper-extension-report-20261124.md),
[source audit](free5gc-upstream-audit.md).

## 1. Decision

**REUSE FREE5GC WITH PATCH.**

- Use the pinned Go modules (`nas v1.3.1`, `ngap v1.2.0`) as the protocol
  implementation.
- Add a thin facade in `ca-client-tools` that:
  - decodes and re-encodes the audited message/IE subset;
  - treats any `ParseGSM` error, including `IEToDoList`, as PARTIAL and never
    promotes it to a successful decode;
  - rejects standalone N2 transfers whose root extension bit is 1 until the
    upstream APER fix lands;
  - enforces strict consumption (`ByteOffset` == input length) on standalone
    N2 transfers;
  - always preserves the original bytes; the typed projection is a view only.
- Pursue an upstream patch for the APER extension-addition gap. A minimal
  fail-closed patch is already implemented in a scratch copy and validated (see
  the [APER report](free5gc-aper-extension-report-20261124.md) section 5.2); it
  is the first upstream proposal. If upstream will not accept the localized
  change, move to **MAINTAIN NARROW FORK** limited to `aper/{un}marshal.go` and
  its tests.

Do not start from **REPLACE UPSTREAM**: no tested case requires it.

## 2. Why not the other options

| Option | Verdict | Evidence |
| --- | --- | --- |
| REUSE FREE5GC (unpatched) | Rejected | ext=1 additions are silently dropped; trailing octets are accepted; unknown IEs abort regardless of criticality |
| REUSE FREE5GC WITH PATCH | **Selected** | all current OpenAMSM traffic round-trips byte-identically; the defects are narrow and the facade can gate them |
| MAINTAIN NARROW FORK | Fallback | only if upstream rejects the APER fail-closed patch; the validated patch is ~15 changed lines plus tests |
| REPLACE UPSTREAM | Not justified | no audited vector fails in a way another implementation would fix; pycrate agrees with Free5GC on all valid OpenAMSM vectors |

## 3. Facade contract (what must be true before consumers migrate)

1. Raw N1/N2 bytes, Content-ID, Content-Type and digest are retained; a decoded
   projection never replaces the wire payload.
2. N2 root selection is explicit, from trusted SBI `n2SmInfoType`, never by
   guessing from bytes.
3. NAS decode returns one of: SUCCESS, PARTIAL (IEToDoList/unknown-new-IE),
   UNSUPPORTED MESSAGE, INVALID. PARTIAL must never be reported as PASS.
4. Standalone N2 transfer decode rejects: ext bit 1 (until fixed), trailing
   octets, unknown IE IDs, truncated open types.
5. A message with an unimplemented IE that the business needs (for example an
   Authorized QoS flow description) blocks the business step; the facade must
   surface the IE list, not swallow it.
6. The codec is not an Agent Tool, not a network service, and owns no business
   state.

## 4. Remaining blockers before migration

The encoder-level evidence gap is closed: 0xC2, 0xCB, 0xD3 and 0xC3 decode and
re-encode byte-identically from real OpenAMSM NAS encoder output, and all seven
used N2 roots do the same from OpenAMSM asn1c output (see the two matrices).
What remains:

- Fix `ValidateEPCO` (start at offset 1) or stop advertising that validation.
- Obtain or generate a Modify Request / Release Command Transfer with extension
  bit 1 for per-root APER sign-off, and a nested-extension vector.
- File the validated fail-closed APER patch upstream (or use the narrow fork if
  rejected).
- Optionally capture the same messages from a live SBI/SCTP exchange as a
  deployment check; this no longer blocks the protocol decision.

## 5. What is explicitly not started

- No Free5GC dependency is added to `go.mod`.
- No existing codec is deleted or replaced.
- No Host/NAS worker or Python worker is implemented.
- No CA-SL, Provider v2, ToolCatalog or Skill contract change.

## 6. Next-phase plan

### Phase A - close the evidence gaps (no product dependency)

Done on 2026-11-24:

1. A real OpenAMSM build was produced in WSL2 and a harness generated the 0xC2 /
   0xCB / 0xD3 / 0xC3 NAS messages and the missing N2 roots; Free5GC decodes and
   re-encodes every one byte-identically.
2. The 0xC2 SSC/PST octet decode is confirmed with the real Accept vector.
3. Every used N2 root now has an OpenAMSM asn1c vector (Setup/Modify Request,
   Setup/Modify/Release Response, Setup Unsuccessful, Release Command).

Still open:

4. Generate a Modify Request / Release Command Transfer with extension bit 1
   and a full nested extension-addition vector (the current nested test is a
   targeted ext-bit mutation, not a complete addition).
5. Re-derive the Mock AMF "ipv6 two qfi" Setup Response vector, which two
   independent decoders reject.
6. File the upstream APER fail-closed patch (already implemented and validated
   in a scratch copy), or fall back to the narrow fork.

### Phase B - facade and worker (after Phase A)

1. Add the Free5GC module requirements and the facade package with the contract
   in section 3.
2. Add the strict-consumption and extension-bit guards with tests built from
   the Phase A vectors.
3. Add the long-lived worker over framed stdin/stdout with separate binary
   segments; no Base64, one process per Host, no new service.
4. Run Go and Python cross-language tests against the same vectors.

### Phase C - consumer migration (after Phase B)

1. Switch the Mock AMF to the facade and remove its handwritten N2 subset only
   after the Go facade passes the same vectors.
2. Switch the Host's NAS projections to the worker, keeping raw N1/N2 bytes and
   Content-ID metadata authoritative.
3. Delete the old codecs only after every consumer and cross-repository gate
   passes.

Nothing in Phase B or C starts while Phase A is open. The current PR stays a
documentation PR; implementation gets its own focused change.


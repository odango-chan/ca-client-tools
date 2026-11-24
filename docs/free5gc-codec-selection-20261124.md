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
- Pursue an upstream patch for the APER extension-addition gap. If upstream
  will not accept the localized change, move to **MAINTAIN NARROW FORK** limited
  to `aper/{un}marshal.go` and its tests.

Do not start from **REPLACE UPSTREAM**: no tested case requires it.

## 2. Why not the other options

| Option | Verdict | Evidence |
| --- | --- | --- |
| REUSE FREE5GC (unpatched) | Rejected | ext=1 additions are silently dropped; trailing octets are accepted; unknown IEs abort regardless of criticality |
| REUSE FREE5GC WITH PATCH | **Selected** | all current OpenAMSM traffic round-trips byte-identically; the defects are narrow and the facade can gate them |
| MAINTAIN NARROW FORK | Fallback | only if upstream rejects the APER change; full fork is not justified |
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

- Capture one real OpenAMSM Establishment Accept (0xC2), Modification Command
  (0xCB) and Release Command (0xD3), and prove byte-level decode/re-encode.
- Resolve the 0xC2 Selected SSC Mode octet layout question with that vector.
- Fix `ValidateEPCO` (start at offset 1) or stop advertising that validation.
- Obtain or generate a Release Command Transfer vector and a Modify Request
  ext=1 vector for per-root APER sign-off.
- Decide the upstream patch plan (issue + PR) for the APER extension bit.

## 5. What is explicitly not started

- No Free5GC dependency is added to `go.mod`.
- No existing codec is deleted or replaced.
- No Host/NAS worker or Python worker is implemented.
- No CA-SL, Provider v2, ToolCatalog or Skill contract change.

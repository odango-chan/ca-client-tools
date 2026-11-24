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

### 3.1 Raw payload authority

1. Raw N1/N2 bytes, Content-ID, Content-Type and digest are retained; a decoded
   projection never replaces the wire payload and is never re-emitted as the
   message. The facade returns the original bytes unchanged plus a typed view.
2. N2 root selection is explicit, from trusted SBI `n2SmInfoType`, never by
   guessing from bytes.
3. The codec is not an Agent Tool, not a network service, and owns no business
   state.

### 3.2 NAS decoder outcome states

The states are defined by the Go values the pinned `free5gc/nas v1.3.1` API
actually returns. Only `PARTIAL` has a dedicated Go error type; the two
"unsupported" states are decided structurally by the facade, never by matching
error text.

| State | Decision rule (actual Go surface) | Business handling |
| --- | --- | --- |
| `SUCCESS` | `ParseGSM` returns `(m != nil, nil)`; the facade's IEI walk found no duplicate and no unknown IE | Complete message; the projection may be consumed |
| `PARTIAL` | `ParseGSM` returns `(m != nil, err)` where `err` is `*message.Error` (package `github.com/free5gc/nas/message`) with `len(IEToDoList) > 0` - one or more recognized IEIs map to `*ie.IEToDo`. Use `errors.As`, not text matching | Recognized but incomplete. `complete=false`, `IEToDoList` exposed; must never be reported or logged as a successful decode, and a business step that needs an IE in that list blocks |
| `UNSUPPORTED_MESSAGE` | `b[3]` is outside the audited message-type allow-list. The decoder agrees (`nil, "unknown GSM Msg type: %d"`) but that error is an untyped `errors.Errorf`, so the facade decides from the octet before parsing | Fail closed; no projection |
| `UNSUPPORTED_IE` | the facade's own IEI walk proves the IEI is unknown/unmodelled while the framing is well formed. The decoder returns `(nil, "<Msg> unknown iei[%d]")`, again untyped, so the IEI and offset must come from the facade's walk | Fail closed; report the raw IEI and offset; never skip silently |
| `INVALID` | everything else: `(nil, err)` for a message type inside the allow-list with structurally valid framing - lengths that overrun, truncated TLVs, zero-length ePCO, the recovered-panic path, and any `PARTIAL` candidate that also fails the structural walk | Fail closed; raw bytes retained for evidence |

Evidence for the rules (NAS coverage matrix §6):

- `PARTIAL` is typed: `ParseGSM` returns the raw `*message.Error` value (it is
  not wrapped) when `IEToDoList` is non-empty. Verified with `...66 03 112233`
  (`IEToDoList=[102]`), `...72 0003 112233` (`[114]`) and `...70 0003 112233`
  (`[112]`).
- `UNSUPPORTED_MESSAGE` and `UNSUPPORTED_IE` are **not** typed: the decoder
  returns `errors.Errorf("unknown GSM Msg type: %d")` and
  `errors.Errorf("<Msg> unknown iei[%d]")`, wrapped by `errors.Wrap(err,
  "ParseGSM()")`. String matching would silently break on an upstream wording
  change, so the facade must not do it.
- A 4-octet `2e0101c1` returns a non-nil message *plus* a recovered-panic error;
  that is `INVALID`, never `SUCCESS`.

### 3.3 Hard rules that follow from the decoder's actual behavior

1. **Not every `ParseGSM` error is `PARTIAL`.** Only `*message.Error` with a
   non-empty `IEToDoList` is `PARTIAL`. The previous wording ("treats any
   `ParseGSM` error, including `IEToDoList`, as PARTIAL") is withdrawn: it would
   map unknown IEIs, length errors and panic recovery onto a partial decode.
2. **Duplicate IEs are not silently accepted.** `free5gc/nas` silently skips a
   repeated recognized IE (`if m.X != nil { reader.Next(ieLen); break }`), and
   duplicate PDU Session Type / ePCO vectors decode with no error. The facade
   runs its own IEI scan and reports a duplicate as `INVALID` with reason
   `DUPLICATE_IE`, preserving the raw bytes.
3. **A partial result is never a complete message.** The projection carries
   `complete=false` and `IEToDoList`; consumers that need an unimplemented IE
   (for example an Authorized QoS flow description) must block their step. The
   facade surfaces the IE list instead of swallowing it.
4. **Unknown IEs are never dropped silently.** An unknown IEI is
   `UNSUPPORTED_IE` with its IEI and offset; there is no "skip and continue"
   path in the facade, and no attempt to re-encode without it.
5. **Re-encode is not a correctness oracle.** `ParseGSM` can succeed and
   `MarshalBinary` still be lossy: a well-formed ePCO message
   (`2e0203c1ffff93a37b000e00010401020304000d0303080808`) decodes with no error
   but re-encodes as `2e0203c1ffff93a37b000180`, dropping every unmodelled
   option and rewriting the configuration-protocol octet. The facade re-encodes
   only for the audited subset and only as an internal check compared
   byte-for-byte with a stored expectation; a mismatch is a hard failure, and
   re-encoded NAS is never emitted downstream.
6. **Original bytes are always preserved**, including for `INVALID` and
   `PARTIAL`, so evidence and later re-analysis never depend on a lossy
   projection.

### 3.4 N2 transfer contract

1. Standalone N2 transfer decode rejects: ext bit 1 (the fail-closed APER patch
   makes this an `errors.Is(err, aper.ErrExtensibleSequenceUnsupported)` result
   at the root and at nested extensible SEQUENCEs), trailing octets, unknown IE
   IDs, and truncated open types.
2. Strict consumption is a facade check, because `ie.UnmarshalBinary` does not
   check the final bit offset. The correct predicate uses the real API shape:
   `consumed := pd.ByteOffset()*8 + pd.BitOffset()` - `ByteOffset()` is in
   **octets**, not bits - and the decode is strict when
   `len(input)*8 - consumed < 8` and the remaining bits are zero. The IPv6
   two-QFI Setup Response is the calibration vector: it legitimately consumes
   210 of 216 bits.
3. A message with an unimplemented IE that the business needs blocks the
   business step; the facade surfaces the IE list, not a guess.

### 3.5 Test plan that proves the contract

Each row is a test the facade must own before any consumer migrates. Vectors
come from the two coverage matrices; none is invented here.

| # | Case | Input | Expected |
| --- | --- | --- | --- |
| 1 | NAS success, byte-identical re-encode | `2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001` (0xC2) and the 0xCB / 0xD3 / 0xC3 encoder vectors | `SUCCESS`, `reencode_verified=true` |
| 2 | NAS partial | `2e0101c1ffff91a16603112233` and the `0x72` / `0x70` variants | `PARTIAL`, `errors.As` to `*message.Error`, `IEToDoList` non-empty, `complete=false` |
| 3 | Unsupported message type | a 5GSM octet outside the allow-list (for example `2e0101ee...`) | `UNSUPPORTED_MESSAGE`, no projection |
| 4 | Unknown IEI | `2e0101c1ffff91a17603112233` | `UNSUPPORTED_IE`, IEI 118 and offset reported |
| 5 | Duplicate recognized IE | `2e0101c1ffff91a191` (duplicate PDU Session Type) and a duplicate-ePCO vector | `INVALID` with `DUPLICATE_IE` |
| 6 | Length / truncation | `2e0101c1` (4 octets), the audit ePCO vector `2e0203c1ffff93a37b000d...`, `...7b0000`, trailing bytes `...deadbeef` | `INVALID`, raw bytes retained |
| 7 | Lossy re-encode gate | `2e0203c1ffff93a37b000e00010401020304000d0303080808` | `SUCCESS` decode but `reencode_verified=false`; no re-encoded bytes emitted |
| 8 | N2 ext=1 root and nested | Setup Request `80...00`, `80...0202aabb`, Release Command `9000`, nested `804000091d0000` | rejected with `errors.Is(err, aper.ErrExtensibleSequenceUnsupported)` |
| 9 | N2 strict consumption | two-QFI IPv6 Setup Response (210/216 bits) versus the same bytes plus one octet | accepted versus rejected |
| 10 | N2 valid round net | the ten audited ext=0 roots | decoded and re-encoded byte-identically |

Until the facade exists and tests 1-10 pass, no consumer may read a projection
as a complete message.

## 4. Remaining blockers before migration

The encoder-level evidence gap is closed: 0xC2, 0xCB, 0xD3 and 0xC3 decode and
re-encode byte-identically from real OpenAMSM NAS encoder output, and all seven
used N2 roots do the same from OpenAMSM asn1c output (see the two matrices).

Status of the three Phase A items:

- `ValidateEPCO` off-by-one: **fixed** (`ca-client-tools#38`), the audit vector
  is now rejected at `ValidateEPCO`, `EncodeEstablishmentRequest` and
  `DecodeEstablishmentRequest`.
- Mock AMF "ipv6 two qfi" vector: **closed as an audit error** - the vector is
  valid and verified by asn1c, free5gc/ngap v1.2.0 and pycrate 0.8.1
  (`env-5gc-sim#83`).
- APER fail-closed patch: **prepared and re-verified** (revision 2, clean-copy
  apply plus full `go test ./...`); submission is awaiting authorization.
  Materials: [upstream PR bundle](evidence/free5gc-aper-upstream-pr-20261124/README.md).
- Optional and non-blocking: capture the same messages from a live SBI/SCTP
  exchange as a deployment check.

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
4. Legal ext=1 vectors exist for the remaining roots (Modify Request
   `800001...000`, Release Command `9000`) and for the nested
   `QosFlowSetupRequestItem` (`804000091d0000`, `804000091d008002aabb`), all
   validated by pycrate and rejected by the patched build.

Still open at the end of Phase A (2026-11-24):

5. ~~Re-derive the Mock AMF "ipv6 two qfi" Setup Response vector, which two
   independent decoders reject.~~ Closed: the rejection claim was an audit
   error. asn1c, free5gc/ngap v1.2.0 and pycrate 0.8.1 all accept the vector and
   agree on `2001:db8::2`, TEID `0x10203040` and QFI `[7 63]`; boundary vectors
   were added around it (`env-5gc-sim#83`).
6. ~~File the upstream APER fail-closed patch, or fall back to the narrow
   fork.~~ Prepared, not filed: revision 2 applies cleanly to a clean
   `free5gc/ngap` v1.2.0 copy and passes the full `go test ./...`; the complete
   upstream PR bundle is
   [evidence/free5gc-aper-upstream-pr-20261124/](evidence/free5gc-aper-upstream-pr-20261124/README.md).
   Opening the upstream PR needs explicit authorization; until then the facade
   depends on the local patch (or the narrow fork) and must reject ext=1 itself.
   The narrow-fork maintenance assessment is in that bundle (§7).
7. ~~Fix `ValidateEPCO`.~~ Fixed by `ca-client-tools#38`; the audit vector is
   rejected at all three entry points.

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


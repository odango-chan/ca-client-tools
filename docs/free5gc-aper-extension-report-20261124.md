# Free5GC APER extension-bit investigation

**Audit date:** 2026-11-24  
**Pinned source:** `github.com/free5gc/ngap v1.2.0`, commit
`7c6125004f5fa914412a4a87de3d48f0ca2ab1c8`  
**Status:** REPRODUCED / IMPACT BOUNDED / FIX REQUIRED FOR GENERAL R19 USE

## 1. What the source does

`aper/unmarshal.go:1087` `ReadSequencePreambleBitMap` reads the extension bit
through `ReadExtensible()` and discards the returned boolean
(`_, err := pd.ReadExtensible()`), then reads the optional bitmap. The comment
says "Extensible Sequence is not supported. Extensible bit is always 0."

`aper/marshal.go:1036` `WriteSequencePreambleBitMap` calls
`WriteExtensible(false)` when `valueExtensible` is true, i.e. it always emits
extension bit 0, and has no representation for extension additions.

The three request/command transfer roots call this helper with
`valueExtensible=true`:

- `ie/pdu_session_resource_setup_request_transfer.go:28,54`
- `ie/pdu_session_resource_modify_request_transfer.go:28,54`
- `ie/pdu_session_resource_release_command_transfer.go:35,70`
- and the response transfers (`..._setup_response_transfer.go:56,118`,
  `..._modify_response_transfer.go:65,139`,
  `..._release_response_transfer.go:30,59`).

Nested extensible SEQUENCEs (QosFlowSetupRequestItem, QosFlowLevelQosParameters,
QosCharacteristics alternatives, ...) use the same helper, so the defect is not
limited to the root.

## 2. Answers to the required questions

1. **Do the used roots have extensible SEQUENCEs?** Yes - every root and many
   nested structures. The root preamble is 1 extension bit; nested preambles add
   optional bits.
2. **What is the extension bit in current OpenAMSM traffic?** 0. The captured
   OpenAMSM Setup Request, Modify Request and Setup Response vectors all start
   with a zero extension bit and Free5GC re-encodes them byte-for-byte.
3. **What is it in the Mock AMF vectors?** 0. The Mock AMF decoders explicitly
   reject a set root extension bit (`n2transfer/request.go:102-108`,
   `response.go:124-130`, `modify.go:68-74`, `modify.go:304-310`).
4. **Do nested SEQUENCEs have the same problem?** Yes - same helper and same
   `valueExtensible=true` call sites. The root-level reproduction below proves
   the mechanism; nested vectors are still required to sign off per structure
   because an addition that precedes a parent field can shift the parent parse
   (worse than the root case, where the additions are simply left as trailing
   bytes).
5. **What does Release 19 define?** The audited TS 38.413 transfer types carry
   `...` with no named extension additions in the pinned ASN.1. R19 evolves the
   content through ProtocolIE-Containers (new IE IDs) rather than through
   SEQUENCE extension additions for these roots. That is why the ext bit has
   been 0 in practice.
6. **Is ext bit 0 handled correctly?** Yes for the audited traffic: OpenAMSM's
   asn1c vectors decode and re-encode byte-identically.
7. **What happens when ext bit is 1?** The bit is ignored. With an empty
   extension-addition list the decode succeeds; with addition bytes the bytes
   are left unconsumed and the decode still succeeds (silent data loss); a
   truncated addition list is also accepted. Encode always writes 0, so the
   additions cannot be reproduced. This is silent data loss plus a strictness
   hole, not a hard failure.

## 3. Standards vectors and observed behavior

Base vector (OpenAMSM asn1c-compatible Setup Request, 45 octets, ext bit 0):

```
00000400820008080f4240200f4240008b000a01f0ac101f3d263f259100860001000088000700010000091d00
```

Derived vectors (first octet extension bit set to 1):

| ID | Bytes | Meaning |
| --- | --- | --- |
| V1 | base with `80` first octet + `00` suffix (46) | ext=1, empty extension-addition list |
| V2 | base with `80` + `02 02 aa bb` (49) | ext=1, one 2-octet extension addition |
| V3 | base with `80`, no additions octet (45) | ext=1, truncated addition list |
| V4 | base + `ff ff` (47) | ext=0, two trailing octets |

The `00` / `02 02 aa bb` suffix shape is the X.691 extension-addition encoding
as validated by the independent asn1tools aligned-PER oracle (a zero-addition
list is one `00` octet; a one-addition list starts with `02`, then an open type
length `02` and two value octets). pycrate decodes V1 and V2 to exactly the same
IE values as the base vector and rejects V3.

Observed with Free5GC v1.2.0:

| Vector | Result |
| --- | --- |
| base | OK, IEs `[130 139 134 136]` |
| V1 | OK, same IEs |
| V2 | OK, same IEs; re-encode produces the base vector (additions dropped) |
| V3 | OK, same IEs (truncation not detected) |
| V4 | OK, same IEs (trailing octets not detected) |

The same preamble implementation is used by the Modify Request and Release
Command transfers; the root-level behavior is therefore shared. Section 3.1
records that every used root's ext=0 form has been byte-verified; a Release
Command / Modify Request vector with ext=1 is still required for per-root
sign-off of the extension path.

### 3.1 Ext=0 verification across the used roots

The OpenAMSM asn1c encoder produced ext=0 vectors for every root used by the
audited flow; Free5GC decodes and re-encodes each byte-identically (details and
provenance in the [NGAP matrix](free5gc-ngap-coverage-matrix-20261124.md)):

```
Setup Request       000003008b000a01f00a0a0001000007d100860001000088000700010000091c00
Modify Request      000001008700170100a000011c0040030d4020030d40200186a0200186a0
Setup Response      0003e00a0a0008000003e90001
Setup Unsuccessful  0800
Modify Response     100004
Release Command     10
Release Response    00
```

This is the exact subset the business exercises. The extension-bit defect
therefore does not affect current traffic; it is a qualification limit for
future or third-party peers.

## 4. Impact on the real business

- **Current OpenAMSM / Mock AMF traffic:** no impact - ext bit is 0 and all
  audited vectors round-trip byte-identically.
- **A future or third-party peer that uses an extension addition:** the CA
  facade would decode a partial message and silently lose the addition; a
  re-encode would not reproduce the peer's bytes. For an "exact byte
  preservation" codec this is a correctness failure.
- **A malformed/truncated addition list:** accepted as valid, so a facade
  cannot rely on Free5GC alone to reject malformed APER.
- **Extension additions on nested structures:** potential misparse of following
  fields, not just data loss. This is the highest-risk unverified case.

## 5. Fix options

| Option | Cost | Notes |
| --- | --- | --- |
| A. Reuse as-is + facade pre-scan | low | The facade can inspect the first bit of each standalone transfer and reject ext=1, and can enforce `ByteOffset()` strictness. It cannot inspect nested preambles without a full APER walk, so nested risk remains. |
| B. Upstream patch | medium | Read and retain the extension bit; decode/skip extension additions per X.691 and expose them (or at least consume them); write them back on encode. Localized to `aper/{un}marshal.go` plus generated preamble calls that need an additions field. Needs discussion with Free5GC. |
| C. Narrow fork | medium-high | Pin a fork containing only the APER preamble patch + tests. Keep the rest of the module on the upstream tag. |
| D. Replace implementation | high | Only if Free5GC also fails on used-edge cases; not justified by the current evidence. |

**Recommendation:** REUSE FREE5GC WITH PATCH. Implement option A now (fail
closed on ext=1 at the facade, strict-consumption guard), pursue option B
upstream, and keep option C as the fallback if upstream will not take the
localized APER change. Do not hand-write a replacement APER stack.

## 6. Reproduction

The probe programs used for this report live outside the repository (they must
not add a Free5GC dependency to the product):

- Go: `github.com/free5gc/ngap v1.2.0` with `ie.UnmarshalBinary` /
  `ie.MarshalBinary` on the vectors above.
- Python: `pycrate_asn1dir.NGAP.NGAP_IEs.PDUSessionResourceSetupRequestTransfer`
  with `from_aper`, and `asn1tools.compile_string(spec, "per")` for the preamble
  encoding shape.

Results are reproducible with the byte strings in section 3; no generated
encoder output is used as its own oracle. The base vector is the OpenAMSM
asn1c-generated vector stored by the Mock AMF tests
(`ca-5gc-sim/internal/mockamf/n2transfer/n2transfer_test.go:99`), and pycrate
is a different implementation from both Free5GC and asn1c.

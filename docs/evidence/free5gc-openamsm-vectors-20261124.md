# OpenAMSM encoder vectors used by the Free5GC audit (2026-11-24)

**Status:** EVIDENCE / REPRODUCIBLE

These vectors were produced by the pinned OpenAMSM snapshot
(`env-openamsm@594dec78b95bff4fdc34deac8a3a9681fb41228f`, source tree hash
`7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`) and
validated against `free5gc/nas v1.3.1` / `free5gc/ngap v1.2.0`. They are
independent of Free5GC: OpenAMSM uses its own hand-written 5GSM encoder and its
own asn1c-generated NGAP codec (R19.2.0, `38413-j20.asn`).

## 1. Provenance

- OpenAMSM was built in WSL2 Ubuntu 26.04 from a native-filesystem copy of the
  repository using `scripts/build.sh` (Meson/Ninja, installed under
  `.runtime/install`). Only the disposable staged source and the Meson build
  directory were touched: `-Werror` was relaxed to compile under GCC 15, and
  the scripts' CRLF line endings were normalized. The tracked snapshot was not
  modified.
- NAS vectors: a C harness linked the built `libogscore` and `libogsnas-5gs`
  and called `ogs_nas_5gs_plain_encode()` on the same structures and field
  values that `src/smf/gsm-build.c` builds (`gsm_build_pdu_session_establishment_accept`,
  `gsm_build_pdu_session_modification_command`,
  `gsm_build_pdu_session_release_command`, `gsm_build_pdu_session_establishment_reject`).
- NGAP vectors: a C harness linked `libogsasn1c-ngap` / `libogsasn1c-util` and
  called `ogs_asn_encode()` on the same asn1c types that
  `src/smf/ngap-build.c` encodes. The Cause-only roots mirror the builder
  exactly; the Modify Response vector carries one
  `QosFlowAddOrModifyResponseItem` (QFI 1).
- Validation: each vector was decoded and re-encoded with the pinned Free5GC
  modules and compared byte-for-byte. pycrate (an independent ASN.1/APER
  implementation) was used as a second decoder for the NGAP vectors.

## 2. NAS vectors

| Message | Hex |
| --- | --- |
| 0xC2 PDU Session Establishment Accept | `2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001` |
| 0xCB PDU Session Modification Command | `2e0101cb7a000901000631310101ff01790006012041010109` |
| 0xD3 PDU Session Release Command | `2e0101d324` |
| 0xC3 PDU Session Establishment Reject | `2e0101c324` |

Accept contents: header (PSI 1, PTI 1); combined SSC/PST octet `0x11` (SSC1,
IPv4); Authorized QoS rules LV-E (default match-all rule, QFI 1); Session-AMBR
LV (100 Mbit/s UL and DL); PDU Address TLV (`10.10.0.8`); S-NSSAI TLV (SST 1,
SD 000001). Command contents: header; Authorized QoS rules (0x7A); Authorized
QoS flow descriptions (0x79, 5QI 9). Release Command / Establishment Reject:
header plus 5GSM cause `0x24` (regular deactivation).

Result: Free5GC `message.ParseGSM` returns the message with no error and the
message's `MarshalBinary` reproduces the exact bytes.

## 3. N2 Transfer vectors

| Root | Hex | Producer |
| --- | --- | --- |
| PDUSessionResourceSetupRequestTransfer | `000003008b000a01f00a0a0001000007d100860001000088000700010000091c00` | OpenAMSM asn1c (stored by Mock AMF tests) |
| PDUSessionResourceModifyRequestTransfer | `000001008700170100a000011c0040030d4020030d40200186a0200186a0` | live D5 SMF capture stored by Mock AMF tests |
| PDUSessionResourceSetupResponseTransfer | `0003e00a0a0008000003e90001` | OpenAMSM asn1c |
| PDUSessionResourceSetupResponseTransfer | `0003e0c0a80a010102030404010080` | OpenAMSM asn1c |
| PDUSessionResourceSetupResponseTransfer | `000fe020010db8000000000000000000000001000003eb0001` | OpenAMSM asn1c |
| PDUSessionResourceSetupResponseTransfer | `0003e00a141e28123456780801014090` | OpenAMSM asn1c |
| PDUSessionResourceSetupUnsuccessfulTransfer | `0800` | OpenAMSM asn1c harness |
| PDUSessionResourceModifyResponseTransfer | `100004` | OpenAMSM asn1c harness |
| PDUSessionResourceReleaseCommandTransfer | `10` | OpenAMSM asn1c harness |
| PDUSessionResourceReleaseResponseTransfer | `00` | OpenAMSM asn1c |

All decode in Free5GC and re-encode byte-identically. pycrate decodes the same
values (Cause nas/normal-release for `0800` and `10`; QFI 1 for `100004`).

One stored Mock AMF vector, `000fe020010db80000000000000000000000021020304004070fc0`
("ipv6 two qfi" Setup Response), is rejected by both Free5GC and pycrate. It
must be re-derived in `env-5gc-sim`; it is not used as evidence here.

## 4. Reproducing the validation

The Free5GC side needs no harness in this repository: decode with
`ie.UnmarshalBinary` / `ie.MarshalBinary` for the NGAP roots and
`message.ParseGSM` plus `GSMMessage.MarshalBinary` for NAS. Any scratch module
can import the pinned versions; the audit deliberately did not add them to the
product `go.mod`.

Example (Go, scratch module):

```go
in, _ := hex.DecodeString("2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001")
msg, err := nas.ParseGSM(in)       // must be nil error
out, err := msg.MarshalBinary()    // out must equal in
```

The OpenAMSM harness code is intentionally kept out of the repositories; this
document records the exact field content and the encoder entry points above so
the vectors can be regenerated from the pinned snapshot.

## 5. APER extension-bit patch evidence

- Patch: [`free5gc-ngap-aper-ext-failclosed-20261124.patch`](free5gc-ngap-aper-ext-failclosed-20261124.patch)
- It makes `ReadSequencePreambleBitMap` return
  `aper.ErrExtensibleSequenceUnsupported` when the extension bit is 1, instead
  of discarding it. Encoding is unchanged (writes 0, as today).
- Patched module results: `go test ./...` passes (ngap, aper, ie, message,
  nrppa, util). The ext=0 Setup Request and all OpenAMSM vectors still decode
  and re-encode byte-identically; the ext=1 variants (empty additions, one
  addition, truncated additions) fail closed.
- Nested probe: `QosFlowSetupRequestItem` base `004000091d00` (pycrate-validated
  fields: QFI 1, non-dynamic 5QI 9, ARP 8) decodes in both builds; the targeted
  ext-bit mutation `804000091d00` decodes silently in unpatched v1.2.0 and
  fails closed in the patched build.

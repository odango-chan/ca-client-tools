# NGAP N2 Transfer coverage matrix for the CA Create / Modify / Release subset

**Audit date:** 2026-11-24  
**Pinned source:** `github.com/free5gc/ngap v1.2.0`, commit
`7c6125004f5fa914412a4a87de3d48f0ca2ab1c8`  
**Status:** SOURCE + VECTOR AUDIT / N2 SUBSET CONFIRMED / R19 UNQUALIFIED

## 1. Which N2 Transfer roots the flows really use

The SMF builds and parses the NGAP `*Transfer` PDUs itself with asn1c
(`ogs_asn_encode` / `ogs_asn_decode`); the transfer bytes are not relayed from
the AMF or UPF. Evidence:

| Direction | n2SmInfoType | Transfer root | OpenAMSM evidence |
| --- | --- | --- | --- |
| SMF -> AMF | `PDU_RES_SETUP_REQ` | PDUSessionResourceSetupRequestTransfer | `src/smf/ngap-build.c:101,404` |
| SMF -> AMF | `PDU_RES_MOD_REQ` | PDUSessionResourceModifyRequestTransfer | `ngap-build.c:408,554` |
| SMF -> AMF | `PDU_RES_MOD_REQ` (release of QoS flows inside Modify) | PDUSessionResourceModifyRequestTransfer with QosFlowToReleaseList | `ngap-build.c:558,639` |
| SMF -> AMF | `PDU_RES_REL_CMD` | PDUSessionResourceReleaseCommandTransfer | `ngap-build.c:643,665` |
| AMF -> SMF | `PDU_RES_SETUP_RSP` | PDUSessionResourceSetupResponseTransfer | `ngap-handler.c:35,67` |
| AMF -> SMF | `PDU_RES_SETUP_FAIL` | PDUSessionResourceSetupUnsuccessfulTransfer | `ngap-handler.c:390,408` |
| AMF -> SMF | `PDU_RES_MOD_RSP` | PDUSessionResourceModifyResponseTransfer | `ngap-handler.c:527,549` |
| AMF -> SMF | `PDU_RES_REL_RSP` | PDUSessionResourceReleaseResponseTransfer | no ASN.1 decode; state check only (`gsm-sm.c:2647-2693`) |
| AMF -> SMF | `PDU_RES_MOD_FAIL` | PDUSessionResourceModifyUnsuccessfulTransfer | listed in skip logic but **no FSM handler** (`gsm-sm.c:2722`) |

Classification:

- **Actually used:** Setup Request, Setup Response, Setup Unsuccessful, Modify
  Request, Modify Response, Release Command, Release Response.
- **Future / not yet implemented by OpenAMSM:** Modify Unsuccessful (the
  `n2SmInfoType` is recognised but falls through to "Unknown message").
- **Not needed for this subset:** Modify Confirm, Modify Indication,
  Modify Indication Unsuccessful, Notify, Notify Released, Path Switch,
  Handover, Secondary RAT Usage, UE Context Resume/Suspend.

The Mock AMF only decodes Setup Request and Modify Request, encodes Setup
Response, Modify Response and Release Response, and only checks the
`PDU_RES_REL_CMD` type string without decoding the transfer
(`ca-5gc-sim/internal/mockamf/namf.go:275-281`, `:381-393`,
`internal/mockamf/n2transfer/*`). It never produces or consumes a failure
transfer.

## 2. ProtocolIE support per used root

The request transfers carry a `ProtocolIE-Container`. The container struct is
the complete list of accepted IE IDs: any other ID aborts the decode with
`unknown reference field value <id>`, regardless of criticality
(`ie/protocol_ie_field.go` generated `...IEs.Read` branches).

### 2.1 PDUSessionResourceSetupRequestTransfer - 15 IEs

| IE ID | IE | OpenAMSM sets it? |
| --- | --- | --- |
| 130 | PDUSessionAggregateMaximumBitRate | yes (`ngap-build.c:133`) |
| 139 | UL-NGU-UP-TNLInformation | yes (`:153`) |
| 126 | AdditionalUL-NGU-UP-TNLInformation | no |
| 127 | DataForwardingNotPossible | yes (`:179`) |
| 134 | PDUSessionType | yes (`:195`) |
| 138 | SecurityIndication | yes (`:227`) |
| 129 | NetworkInstance | no |
| 136 | QosFlowSetupRequestList | yes (`:310`) |
| 166 | CommonNetworkInstance | no |
| 22 | DirectForwardingPathAvailability | no |
| 195 | RedundantUL-NGU-UP-TNLInformation | no |
| 186 | AdditionalRedundantUL-NGU-UP-TNLInformation | no |
| 190 | RedundantCommonNetworkInstance | no |
| 197 | RedundantPDUSessionInformation | no |
| 318 | MBSSessionSetupRequestList | no |

### 2.2 PDUSessionResourceModifyRequestTransfer - 13 IEs

| IE ID | IE | OpenAMSM sets it? |
| --- | --- | --- |
| 130 | PDUSessionAggregateMaximumBitRate | no |
| 140 | UL-NGU-UP-TNLModifyList | not in the captured D5 vector |
| 129 | NetworkInstance | no |
| 135 | QosFlowAddOrModifyRequestList | yes (`ngap-build.c:432`) |
| 137 | QosFlowToReleaseList | yes (`:584`, UE-requested QoS release) |
| 126 | AdditionalUL-NGU-UP-TNLInformation | no |
| 166 / 186 / 190 / 195 | common/redundant TNL variants | no |
| 138 | SecurityIndication | no |
| 319 | MBSSessionSetuporModifyRequestList | no |
| 317 | MBSSessionToReleaseList | no |

The live D5 vector (`000001008700170100a000011c0040030d4020030d40200186a0200186a0`)
contains only IE 135.

### 2.3 Roots without a ProtocolIE container

These are extensible SEQUENCEs; their "IE set" is the fixed root field list.

| Root | Root fields | Optional fields implemented |
| --- | --- | --- |
| PDUSessionResourceSetupResponseTransfer | DLQosFlowPerTNLInformation (mandatory) | AdditionalDLQosFlowPerTNLInformation, SecurityResult, QosFlowFailedToSetupList, IEExtensions |
| PDUSessionResourceModifyResponseTransfer | none mandatory | DL/UL NGU-UP-TNLInformation, QosFlowAddOrModifyResponseList, AdditionalDLQosFlowPerTNLInformation, QosFlowFailedToAddOrModifyList, IEExtensions |
| PDUSessionResourceReleaseCommandTransfer | Cause (mandatory) | IEExtensions |
| PDUSessionResourceReleaseResponseTransfer | none mandatory | IEExtensions |
| PDUSessionResourceSetupUnsuccessfulTransfer | Cause (mandatory) | CriticalityDiagnostics, IEExtensions |
| PDUSessionResourceModifyUnsuccessfulTransfer | Cause (mandatory) | CriticalityDiagnostics, IEExtensions |

All of these fields are implemented; the optional fields are decoded from the
sequence preamble bitmap.

## 3. QoS Flow support

- The Free5GC `QosFlowSetupRequestList` is a `SEQUENCE OF` with the upstream
  container bound 1..64 (`ie/protocol_ie_container.go` analog). A two-flow
  vector was round-tripped and independently decoded by pycrate:

  ```
  00000400820008080f4240200f4240008b000a01f0ac101f3d263f259100860001000088000d04010000091d00200000091d00
  ```

  Free5GC decodes two items; pycrate decodes QFIs `[1, 2]`.
- The current Mock AMF decoders accept exactly one flow and fail closed on a
  second (`n2transfer/request.go:295`, `modify.go:399`). Its encoders accept
  1..64. This is a Mock AMF limit, not a Free5GC limit; the CA Host treats N2
  as opaque bytes, so it does not impose a flow count.
- The QoS parameter tree (5QI, ARP, GBR/MBR) is fully typed in Free5GC
  (`QosFlowLevelQosParameters`, `QosCharacteristics` CHOICE, non-dynamic and
  dynamic 5QI descriptors) and present in the captured Setup Request vector.

## 4. Unknown IE and strict-consumption behavior

Source facts:

- The generated ProtocolIE `Read` switch ends in
  `errors.Errorf("unknown reference field value %d", id)` for any unknown ID.
  There is no criticality-based Reject/Ignore/Notify handling at this level and
  the raw open-type value is discarded.
- `ie.UnmarshalBinary` constructs a `PerBitData`, calls the root `Read` and
  returns the error; it does not check the final bit offset. `PerBitData`
  exposes `ByteOffset()` / `BitOffset()`, so a facade-side strict-consumption
  check is possible.

Empirical results (Setup Request Transfer):

| Input | Free5GC result |
| --- | --- |
| valid 45-octet vector | decoded, 4 IEs |
| same vector + two trailing octets `ff ff` | **decoded, no error** - trailing octets are not detected |
| unknown IE 999 with criticality reject (`00`) | error `unknown reference field value 999` |
| unknown IE 999 with criticality ignore (`40`) | same error - criticality ignored |
| unknown IE 999 with criticality notify (`80`) | same error |

This is stricter than the NGAP message-level unknown-IE helper (which skips
Ignore/Notify IEs) but it is not protocol-aware: a real R19 peer that adds an
IE with Ignore criticality cannot be decoded at all. For the CA subset this is
acceptable only if the facade fails closed and preserves the raw bytes; it
prevents "unknown IE preservation" as a feature.

## 5. Independent interoperability evidence

The Mock AMF repository stores vectors generated by the pinned OpenAMSM asn1c
`NGAP` codec (R19.2.0, `38413-j20.asn`) - a different implementation from
Free5GC:

| Vector | Free5GC decode | Free5GC re-encode |
| --- | --- | --- |
| Setup Request `000003008b000a01f00a0a0001000007d100860001000088000700010000091c00` | OK: IEs 139, 134, 136 | byte-identical |
| Modify Request `000001008700170100a000011c0040030d4020030d40200186a0200186a0` | OK: IE 135 | byte-identical |
| Setup Response `0003e00a0a0008000003e90001` | OK | byte-identical |
| Setup Response `0003e0c0a80a010102030404010080` | OK | byte-identical |
| Setup Response `000fe020010db8000000000000000000000001000003eb0001` | OK | byte-identical |
| Setup Response `0003e00a141e28123456780801014090` | OK | byte-identical |
| Release Response `00` | OK (empty root) | byte-identical |

pycrate (independent ASN.1/APER implementation) decodes the same vectors with
the same values, including the two-flow vector.

One stored Mock AMF vector is rejected by **both** decoders:
`000fe020010db80000000000000000000000021020304004070fc0` ("ipv6 two qfi").
Two independent decoders rejecting the same bytes points at the stored vector
(or its generation), not at Free5GC. It must be re-derived in `env-5gc-sim`
before it is used as a golden vector.

## 6. Gaps

1. No OpenAMSM-generated Release Command Transfer vector exists; the root is
   simple (Cause + optional extensions) but is not byte-verified.
2. The Setup/Modify **Unsuccessful** transfers have no dedicated upstream test
   files and are not produced by the Mock AMF; only source inspection exists.
3. Unknown IEs abort the standalone transfer decode; raw unknown IE retention is
   impossible with the public API.
4. Trailing octets are not rejected by `ie.UnmarshalBinary`; the facade needs a
   strict-consumption wrapper (`ByteOffset() == len(bytes)*8`).
5. The APER extension-bit gap applies to every root here - see
   [the APER report](free5gc-aper-extension-report-20261124.md).

**NGAP verdict: USED ROOTS SUPPORTED / EXT BIT UNQUALIFIED / STRICTNESS GAP.**

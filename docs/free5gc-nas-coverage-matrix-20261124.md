# NAS 5GSM coverage matrix for the CA Create / Modify / Release subset

**Audit date:** 2026-11-24  
**Pinned source:** `github.com/free5gc/nas v1.3.1`, commit
`ff5ea67383b7b5841247aab712889890b4c67d55`  
**Status:** SOURCE + VECTOR AUDIT / NOT A RELEASE 19 QUALIFICATION

This matrix is scoped to what the CA Host actually sees on the Nsmf and Namf
boundaries. It does not enumerate every 5GSM IE in TS 24.501.

## 1. Where the real messages come from

The real protocol producer/consumer for the current deployment is
`env-openamsm` (an Open5GS-derived C implementation); the Mock AMF is a test
peer, and `ca-agent-demo` is the Host. Message facts therefore come from:

| Source | Evidence |
| --- | --- |
| OpenAMSM SMF builds the SMF-originated 5GSM messages | `upstream/openamsm/src/smf/gsm-build.c:68` (0xC2 Accept), `:354` (0xCB Command), `:603` (0xCA Reject), `:626` (0xD3 Release Command), `:648` (0xD2 Release Reject); encoded by `ogs_nas_5gs_plain_encode` via `src/smf/gsmue-message.c:1309` |
| OpenAMSM decodes UE-originated 5GSM | `src/smf/gsmue-message.c:1409` dispatcher; per-message decoders `:575` (0xC1), `:818` (0xC9), `:1123` (0xD1), `:1045` (0xCC), `:1260` (0xD4) |
| The Nsmf N1 SM part Content-ID is always `5gnas-sm` | `lib/sbi/message.h:394`; `src/smf/sbi-path.c:772-782` |
| The Host retains raw N1 and never re-encodes it | `ca-agent-demo/src/ca_agent_host/integrations/fivegc/nsmf_n1.py:26` (`SbiPart.payload`), `nsmf_http.py:268` etc. |
| Mock AMF only parses the 4-octet 5GSM header itself | `ca-5gc-sim/internal/mockamf/namf_wire.go:253-292` |

## 2. Real message inventory

| Procedure | SBI operation | Direction | NAS message | MT | Evidence | CA role today |
| --- | --- | --- | --- | --- | --- | --- |
| Create | `POST /nsmf-pdusession/v1/sm-contexts` | UE -> SMF | PDU Session Establishment Request | 0xC1 | OpenAMSM `nsmf-handler.c:126-159`, `gsmue-message.c:575` | decodes the request (type/SSC/ePCO presence) |
| Create | Namf N1N2MessageTransfer (after PFCP) | SMF -> UE | PDU Session Establishment Accept | 0xC2 | OpenAMSM `n4-handler.c:551-564`, `gsm-build.c:68` | opaque forward |
| Create | Nsmf Create error / Namf | SMF -> UE | PDU Session Establishment Reject | 0xC3 | OpenAMSM `nsmf-handler.c:163-167`, `gsm-build.c:331` | opaque |
| Create | Namf / Update response | SMF -> AMF | PDU Session Resource Setup Request Transfer | N2 | OpenAMSM `ngap-build.c:101` | opaque |
| Create | UpdateSMContext | AMF -> SMF | Setup Response / Setup Unsuccessful Transfer | N2 | OpenAMSM `ngap-handler.c:35`, `:390` | opaque |
| Modify | `POST .../modify` | UE -> SMF | PDU Session Modification Request | 0xC9 | OpenAMSM `nsmf-handler.c:907-950`, `gsmue-message.c:818` | framing projection (QoS-rule / QoS-flow presence) |
| Modify | Update response / Namf | SMF -> UE | PDU Session Modification Command | 0xCB | OpenAMSM `gsm-build.c:354`, `n4-handler.c:997-1067` | opaque |
| Modify | Update response | SMF -> UE | PDU Session Modification Reject | 0xCA | OpenAMSM `gsm-build.c:603` | not used |
| Modify | `POST .../modify` | UE -> SMF | PDU Session Modification Complete | 0xCC | OpenAMSM `gsmue-message.c:1045`, `nsmf-handler.c:753` | header/PTI only |
| Modify | `POST .../modify` | UE -> SMF | PDU Session Modification Command Reject | 0xCD | OpenAMSM `gsmue-message.c:1084` | not used |
| Release | `POST .../release` | UE -> SMF | PDU Session Release Request | 0xD1 | OpenAMSM `gsmue-message.c:1123` | header projection |
| Release | Namf N1N2 / Update response | SMF -> UE | PDU Session Release Command | 0xD3 | OpenAMSM `ssm_ue.c:151-171`, `gsm-build.c:626` | opaque |
| Release | Update response | SMF -> UE | PDU Session Release Reject | 0xD2 | OpenAMSM `gsm-build.c:648` | not used |
| Release | `POST .../release` | UE -> SMF | PDU Session Release Complete | 0xD4 | OpenAMSM `gsmue-message.c:1260` | header/PTI only |

`0xCA`, `0xCD` and `0xD2` exist in TS 24.501 and in both implementations, but
they are not part of the current E2E evidence. They remain audit candidates,
not exercised messages.

## 3. Decode / encode support in free5gc/nas v1.3.1

`message.ParseGSM` (`message/message.go:614-679`) dispatches 20 branches: the
fifteen PDU Session messages, 5GSM Status, Service-Level Authentication
Command/Complete and Remote UE Report/Response. All twelve messages in this
matrix have a decoder and an encoder.

Every PDU Session message follows the same pattern: mandatory IEs first, then an
optional-IEI loop. Recognized but unimplemented IEs append to
`umerr.IEToDoList` and the decoder returns the IE name in an `*Error`; the
partially populated message is returned together with that error
(`message.go:671-678`). Unknown IEIs whose framing is unknown abort with a
wrapped error and a nil message.

**Status vocabulary used below**

- `PASS` - decoder/encoder implemented; no `IEToDo` path in the IE type.
- `PARTIAL` - the IEI is recognized but the IE type returns `*ie.IEToDo`
  (`IEToDoList`); decode is incomplete.
- `FAIL` - the IE cannot be encoded (its `MarshalBinary` returns `*ie.IEToDo`).
- `PASS*` - `ExtendedProtCfgOpts` is parsed through the direction-specific
  `UnmarshalFromMs` / `UnmarshalFromNw` methods, not `UnmarshalBinary`.

Mandatory IEs (decoded before the IEI loop) are listed per message:

| Message | Mandatory IEs |
| --- | --- |
| 0xC1 | Integrity Protection Maximum Data Rate (V, 2B) |
| 0xC2 | Selected PDU Session Type (V), Selected SSC Mode (V), Authorized QoS Rules (LV-E), Session-AMBR (LV) |
| 0xC3 | 5GSM Cause (V, 1B) |
| 0xD3 | 5GSM Cause (V, 1B) |
| 0xCD / 0xD2 | 5GSM Cause (V, 1B) |
| others | none |

All mandatory IEs above are implemented. `0xC2` is decoded as one octet split
into two 4-bit halves (Selected PDU Session Type / Selected SSC Mode,
`pdu_session_establishment_accept.go:99-116`), while the audited OpenAMSM
builder sets only `selected_pdu_session_type` (`gsm-build.c:138`) and its Accept
struct has no selected SSC mode field (`message.h:1266`). Whether the two
layouts agree is **unproven**; a captured OpenAMSM Accept is required before the
0xC2 decode path can be qualified - see section 7.

## 4. Per-message IE matrix


### 0xC1 -- PDU Session Establishment Request

File: `message/pdu_session_establishment_request.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x90 | PDUSessType | TV | 9.11.4.11 | PASS | PASS | yes |
| 0xA0 | SSCMode | TV | 9.11.4.16 | PASS | PASS | yes |
| 0x28 | Capability5GSM | TLV | 9.11.4.1 | PASS | PASS | yes |
| 0x55 | MaxNumOfSupportedPktFilters | TV | 9.11.4.9 | PASS | PASS | yes |
| 0xB0 | AlwaysonPDUSessReq | TV | 9.11.4.4 | PASS | PASS | yes |
| 0x39 | SMPDUDNReqCntr | TLV | 9.11.4.15 | PARTIAL | FAIL | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x66 | IPHdrCompressionCfg | TLV | 9.11.4.24 | PARTIAL | FAIL | yes |
| 0x6E | DSTTEthPortMACAddr | TLV | 9.11.4.25 | PASS | PASS | yes |
| 0x6F | UEDSTTResidenceTime | TLV | 9.11.4.26 | PASS | PASS | yes |
| 0x74 | PortMgmtInfoCntr | TLV-E | 9.11.4.27 | PASS | PASS | yes |
| 0x1F | EthHdrCompressionCfg | TLV | 9.11.4.28 | PARTIAL | FAIL | yes |
| 0x29 | SuggestedIfId | TLV | 9.11.4.10 | PASS | PASS | yes |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |
| 0x70 | ReqMBSCntr | TLV-E | 9.11.4.30 | PARTIAL | FAIL | no |
| 0x34 | PDUSessPairID | TLV | 9.11.4.32 | PARTIAL | FAIL | no |
| 0x35 | RSN | TLV | 9.11.4.33 | PARTIAL | FAIL | no |

### 0xC2 -- PDU Session Establishment Accept

File: `message/pdu_session_establishment_accept.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x59 | Cause5GSM | TV | 9.11.4.2 | PASS | PASS | yes |
| 0x29 | PDUAddr | TLV | 9.11.4.10 | PASS | PASS | yes |
| 0x56 | RQTimerValue | TV | 9.11.2.3 | PASS | PASS | no |
| 0x22 | SNSSAI | TLV | 9.11.2.8 | PASS | PASS | yes |
| 0x80 | AlwaysonPDUSessInd | TV | 9.11.4.3 | PASS | PASS | yes |
| 0x75 | MappedEPSBearerCtxs | TLV-E | 9.11.4.8 | PASS | PASS | yes |
| 0x78 | EAPMsg | TLV-E | 9.11.2.2 | PASS | PASS | yes |
| 0x79 | AuthoQosFlowDescs | TLV-E | 9.11.4.12 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x25 | DNN | TLV | 9.11.2.1B | PASS | PASS | yes |
| 0x17 | NwFeatureSupport5GSM | TLV | 9.11.4.18 | PASS | PASS | yes |
| 0x18 | ServingPLMNRateCtrl | TLV | 9.11.4.20 | PARTIAL | FAIL | yes |
| 0x77 | ATSSSCntr | TLV-E | 9.11.4.22 | PARTIAL | FAIL | yes |
| 0xC0 | CtrlPlaneOnlyInd | TV | 9.11.4.23 | PARTIAL | FAIL | yes |
| 0x66 | IPHdrCompressionCfg | TLV | 9.11.4.24 | PARTIAL | FAIL | yes |
| 0x1F | EthHdrCompressionCfg | TLV | 9.11.4.28 | PARTIAL | FAIL | yes |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |
| 0x71 | ReceivedMBSCntr | TLV-E | 9.11.4.31 | PARTIAL | FAIL | no |

### 0xC3 -- PDU Session Establishment Reject

File: `message/pdu_session_establishment_reject.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x37 | BackoffTimerValue | TLV | 9.11.2.5 | PASS | PASS | yes |
| 0xF0 | AllowedSSCMode | TV | 9.11.4.5 | PASS | PASS | yes |
| 0x78 | EAPMsg | TLV-E | 9.11.2.2 | PASS | PASS | yes |
| 0x61 | CongestionReattemptIndicator5GSM | TLV | 9.11.4.21 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x1D | ReattemptIndicator | TLV | 9.11.4.17 | PARTIAL | FAIL | yes |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |

### 0xC9 -- PDU Session Modification Request

File: `message/pdu_session_modification_request.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x28 | Capability5GSM | TLV | 9.11.4.1 | PASS | PASS | yes |
| 0x59 | Cause5GSM | TV | 9.11.4.2 | PASS | PASS | yes |
| 0x55 | MaxNumOfSupportedPktFilters | TV | 9.11.4.9 | PASS | PASS | yes |
| 0xB0 | AlwaysonPDUSessReq | TV | 9.11.4.4 | PASS | PASS | yes |
| 0x13 | IntegrityProtectionMaxDataRate | TV | 9.11.4.7 | PASS | PASS | yes |
| 0x7A | ReqQosRules | TLV-E | 9.11.4.13 | PASS | PASS | yes |
| 0x79 | ReqQosFlowDescs | TLV-E | 9.11.4.12 | PASS | PASS | yes |
| 0x75 | MappedEPSBearerCtxs | TLV-E | 9.11.4.8 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x74 | PortMgmtInfoCntr | TLV-E | 9.11.4.27 | PASS | PASS | yes |
| 0x66 | IPHdrCompressionCfg | TLV | 9.11.4.24 | PARTIAL | FAIL | yes |
| 0x1F | EthHdrCompressionCfg | TLV | 9.11.4.28 | PARTIAL | FAIL | yes |
| 0x70 | ReqMBSCntr | TLV-E | 9.11.4.30 | PARTIAL | FAIL | no |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |

### 0xCA -- PDU Session Modification Reject

File: `message/pdu_session_modification_reject.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x37 | BackoffTimerValue | TLV | 9.11.2.5 | PASS | PASS | yes |
| 0x61 | CongestionReattemptIndicator5GSM | TLV | 9.11.4.21 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x1D | ReattemptIndicator | TLV | 9.11.4.17 | PARTIAL | FAIL | yes |

### 0xCB -- PDU Session Modification Command

File: `message/pdu_session_modification_command.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x59 | Cause5GSM | TV | 9.11.4.2 | PASS | PASS | yes |
| 0x2A | SessAMBR | TLV | 9.11.4.14 | PASS | PASS | yes |
| 0x56 | RQTimerValue | TV | 9.11.2.3 | PASS | PASS | yes |
| 0x80 | AlwaysonPDUSessInd | TV | 9.11.4.3 | PASS | PASS | yes |
| 0x7A | AuthoQosRules | TLV-E | 9.11.4.13 | PASS | PASS | yes |
| 0x75 | MappedEPSBearerCtxs | TLV-E | 9.11.4.8 | PASS | PASS | yes |
| 0x79 | AuthoQosFlowDescs | TLV-E | 9.11.4.12 | PASS | PASS | no |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x77 | ATSSSCntr | TLV-E | 9.11.4.22 | PARTIAL | FAIL | yes |
| 0x66 | IPHdrCompressionCfg | TLV | 9.11.4.24 | PARTIAL | FAIL | yes |
| 0x74 | PortMgmtInfoCntr | TLV-E | 9.11.4.27 | PASS | PASS | yes |
| 0x1E | ServingPLMNRateCtrl | TLV | 9.11.4.20 | PARTIAL | FAIL | yes |
| 0x1F | EthHdrCompressionCfg | TLV | 9.11.4.28 | PARTIAL | FAIL | yes |
| 0x71 | ReceivedMBSCntr | TLV-E | 9.11.4.31 | PARTIAL | FAIL | no |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |

### 0xCC -- PDU Session Modification Complete

File: `message/pdu_session_modification_complete.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0x74 | PortMgmtInfoCntr | TLV-E | 9.11.4.27 | PASS | PASS | yes |

### 0xCD -- PDU Session Modification Command Reject

File: `message/pdu_session_modification_command_reject.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |

### 0xD1 -- PDU Session Release Request

File: `message/pdu_session_release_request.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x59 | Cause5GSM | TV | 9.11.4.2 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |

### 0xD2 -- PDU Session Release Reject

File: `message/pdu_session_release_reject.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |

### 0xD3 -- PDU Session Release Command

File: `message/pdu_session_release_command.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x37 | BackoffTimerValue | TLV | 9.11.2.5 | PASS | PASS | yes |
| 0x78 | EAPMsg | TLV-E | 9.11.2.2 | PASS | PASS | yes |
| 0x61 | CongestionReattemptIndicator5GSM | TLV | 9.11.4.21 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |
| 0xD0 | AccessType | TV | 9.11.2.1A | PASS | PASS | yes |
| 0x72 | SvcLvlAACntr | TLV-E | 9.11.2.10 | PARTIAL | FAIL | no |

### 0xD4 -- PDU Session Release Complete

File: `message/pdu_session_release_complete.go`; test file present: True

| IEI | IE (Go field) | Framing | 24.501 | Decode | Encode | IE in test |
|---|---|---|---|---|---|---|
| 0x59 | Cause5GSM | TV | 9.11.4.2 | PASS | PASS | yes |
| 0x7B | ExtendedProtCfgOpts | TLV-E | 9.11.4.6 | PASS* | PASS | yes |


## 5. IEToDoList in the actual CA flow

The IEToDo paths are real but none of them is reached by the current OpenAMSM
messages:

- 0xC2 Accept is built from Authorized QoS Rules, Session-AMBR, PDU Address,
  S-NSSAI, Authorized QoS Flow Descriptions, ePCO, DNN and optional 5GSM Cause
  (`gsm-build.c:161-260`). All of those are `PASS`.
- 0xCB Command is built from Authorized QoS Rules / Flow Descriptions,
  Session-AMBR, 5GSM Cause, ePCO (`gsm-build.c:354-...`). All `PASS`.
- The IEToDo IEs in the tables are R17/R18 additions (IP/Ethernet header
  compression, MBS containers, service-level AA, PDU session pair ID, RSN,
  ATSSS, control-plane-only, serving PLMN rate control, received MBS). They are
  not generated by the audited OpenAMSM messages.

However, the failure mode matters: a message carrying one of those IEIs decodes
partially and returns `(message, error)`. A caller that discards the error and
treats the message as complete will silently lose IEs such as the authorized QoS
flow description. Any CA facade must treat any non-nil error as PARTIAL and
must not promote it to a successful decode.

## 6. Byte-level evidence

All runs were local; the Go probes import the pinned modules from their tagged
source, and pycrate (independent APER/NGAP ASN.1 implementation) is used only as
an APER oracle. The vectors were produced by the CA `protocol/nas5gsm` encoder
and by the OpenAMSM-facing tests.

| Vector | Result with free5gc/nas v1.3.1 |
| --- | --- |
| `2e0101c1ffff91a1` (0xC1, IPv4, SSC1) | decoded, no error; PSI=1 PTI=1 |
| `2e0ffec1ffff92a2` (0xC1, IPv6, SSC2) | decoded, no error |
| `2e0203c1ffff93a37b000d...` (0xC1 + ePCO) | **rejected**: `ExtendedProtCfgOpts.UnmarshalFromMs() bad length, ofs=11, cntLen=8, total Len=13` |
| `2e0101c9`, `2e0101c97b0003000100` (0xC9) | decoded, no error |
| `2e0501d1` (0xD1), `2e0507d4` (0xD4) | decoded, no error |
| `2e0101c1ffff91a1 66 03 112233` (0xC1 + IEI 0x66) | `(message, IE To Do List: "f")`, `IEToDoList=[102]` |
| `... 72 0003 112233` (0xC1 + IEI 0x72) | `(message, IE To Do List: "r")`, `IEToDoList=[114]` |
| `... 70 0003 112233` (0xC1 + IEI 0x70) | `(message, IE To Do List: "p")`, `IEToDoList=[112]` |
| `... 76 03 112233` (unknown IEI) | nil message, `unknown iei[118]` |
| `2e0101c1` (4 octets) | non-nil message plus recovered panic error; must be treated as a decode failure |
| `2e0101c1ffff91a1deadbeef` (trailing bytes) | rejected as `unknown iei[208]` - the parser treats trailing octets as an IEI |

### 6.1 The ePCO rejection is a CA-side validation gap

The ePCO contents in that vector are
`00 01 04 01 02 03 04 00 0d 03 08 08 08`. Parsing per TS 24.501
(a 1-octet configuration protocol followed by `protocolId(2) | length(1) | value`)
gives a final option that declares length 8 with only 3 octets left. Free5GC
correctly rejects it.

`ca-client-tools/protocol/nas5gsm/nas5gsm.go` `ValidateEPCO` starts the option
walk at offset 0 instead of offset 1, so it accepts this malformed vector. The
minimal fix is to require `len(b) >= 1` and start at `i := 1`; the impact is
limited to validation of an opaque ePCO blob (no current consumer re-encodes
it), but the current bytes are accepted under a false validation claim.

## 7. Gaps and limits

1. No stored real OpenAMSM N1 bytes exist in the snapshot. The tests build N1
   with `ogs_nas_5gs_plain_encode` at runtime (`tests/smf-oes/request-msg-test.c`
   is the only literal fixture and is deliberately invalid). The 0xC2 / 0xCB /
   0xD3 paths are therefore source-audited, not byte-verified.
2. The 0xC2 Selected SSC Mode / Selected PDU Session Type octet layout differs
   between the two implementations and needs a captured Accept vector.
3. IEToDo partial results are a fail-closed contract, not a supported decode.
4. Unknown IEIs abort the whole message; there is no unknown-IE retention or
   re-encode path. A future R19 message that adds an IEI will not decode until
   the decoder is extended.
5. The CA `ValidateEPCO` off-by-one must be fixed before the Go codec is
   replaced or the facade must not reuse it.

**NAS verdict: SUBSET SUPPORTED / IE-ToDo PARTIAL / 0xC2 INTEROP UNVERIFIED.**
The messages and IEs that the current OpenAMSM Create/Modify/Release flows
actually set are implemented and encode/decode correctly in the audited paths;
the Accept message still lacks a captured vector and the partial-parse contract
must be enforced by the facade.

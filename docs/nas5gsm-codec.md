# Scoped 5GSM codec (`protocol/nas5gsm`)

Status: Accepted for the scoped slices (establishment, release, modification)
Version: v0.3
Date: 2026-11-22
Tracking: [ca-client-tools#18](https://github.com/odango-chan/ca-client-tools/issues/18),
[ca-client-tools#25](https://github.com/odango-chan/ca-client-tools/issues/25)

## 1. Purpose and boundary

`github.com/odango-chan/ca-client-tools/protocol/nas5gsm` is an intentionally
small, protocol-only Go package. It exists so the Mock AMF in `env-5gc-sim` can
generate a real PDU Session Establishment Request from semantic scenario input
without copying a second NAS decoder into the simulator, and so Host ingress can
perform a bounded semantic projection of an original N1 SM part.

It is:

- a pure encoder/decoder for the supported 5GSM messages and their supported
  IEs;
- independently importable, with no dependency on `internal/*`, Provider
  Catalog, Artifact Store, Runtime, network endpoints, credentials or Host
  state;
- not an Agent-callable tool and not a mutation authorization.

It is not:

- a 5GMM registration/security stack;
- an N1/N2 SBI transport;
- a replacement for retaining the original N1 bytes at Host ingress.

Production ingress **must keep the exact original N1 bytes**. Decoding yields a
controlled projection only; it does not authorize re-encoding and substituting
the wire payload on the way to the Owner.

## 2. Supported slice

Wire layout follows TS 24.501 and the pinned OpenAMSM NAS sources
(`lib/nas/5gs/message.h`, `ies.c`, `encoder.c`, `decoder.c`):

| Element | Support |
| --- | --- |
| Extended protocol discriminator 5GSM (`0x2E`) | Encode + validate |
| PDU session identity (1..15) | Encode + validate |
| Procedure transaction identity (1..254) | Encode + validate |
| Message type `PDU Session Establishment Request` (`0xC1`) | Encode + validate |
| Integrity protection maximum data rate (2 octets) | Encoded as FULL (`0xFF`,`0xFF`); decode validates presence |
| PDU Session Type (TV, IEI `0x9{x}`) | IPv4, IPv6, IPv4v6, UNSTRUCTURED, ETHERNET |
| SSC Mode (TV, IEI `0xA{x}`) | 1, 2, 3 |
| ePCO (TLV-E, IEI `0x7B`) | Validated option list: `protocolId uint16`, `length uint8`, value |
| Message types Release Request/Command/Complete (`0xD1`/`0xD3`/`0xD4`) | Header-only encode; command header decode ignores trailing IEs |
| Message type Modification Request (`0xC9`) | Header + Requested QoS flow descriptions (`0x79`, TLV-E) |
| Message type Modification Command (`0xCB`) | Header + Authorized QoS flow descriptions (`0x79`, TLV-E) |
| Message type Modification Complete (`0xCC`) | Header-only encode/decode |
| QoS flow description | QFI 1..63, operations create/delete/modify, E bit, parameters `5QI`, `GFBR uplink/downlink`, `MFBR uplink/downlink` |
| Bitrate parameter | `unit` 1..25, 16-bit `value`; `QoSFlowBitrateFromKbps` rejects values the unit grammar cannot represent exactly |

Unsupported optional IEs, duplicate supported IEs, truncated/overflowing
lengths, invalid enum values and empty ePCO fail closed with a wrapped sentinel:
`ErrMalformed`, `ErrUnsupported` or `ErrInvalidField`.

A zero `PduSessionType`/`SSCMode` means "IE absent"; encoding omits it and
decoding leaves it zero, so `Encode(Decode(b))` preserves the supported
semantic projection.

The modification slice is deliberately narrow. **Requested QoS rules (`0x7A`)
are not implemented** and fail closed with `ErrUnsupported`: the pinned
OpenAMSM QoS-rules handler only reconfigures packet filters and does not carry
QoS bitrates, so a QoS-only UE request uses Requested QoS flow descriptions.
Every modification IE other than `0x79` also fails closed, and non-zero spare
bits, duplicate QFIs/parameters, unknown parameter identifiers, wrong
parameter lengths and out-of-range bitrate units are rejected. This keeps the
Mock AMF from fabricating an authorized result it did not decode.

## 3. API

```go
type EstablishmentRequest struct {
    PduSessionID   uint8
    PTI            uint8
    PduSessionType PduSessionType
    SSCMode        SSCMode
    EPCO           []byte
}

func EncodeEstablishmentRequest(EstablishmentRequest) ([]byte, error)
func DecodeEstablishmentRequest([]byte) (EstablishmentRequest, error)
func ValidateEPCO([]byte) error

type QoSFlowBitrate struct { Value uint16; Unit uint8 }
func QoSFlowBitrateFromKbps(uint64) (QoSFlowBitrate, error)
func (QoSFlowBitrate) BitsPerSecond() (uint64, error)

type QoSFlowParameter struct {
    Identifier QoSFlowParameterIdentifier
    FiveQI     uint8
    Bitrate    QoSFlowBitrate
}
type QoSFlowDescription struct {
    QFI        uint8
    Operation  QoSFlowOperation
    ReplaceAll bool
    Parameters []QoSFlowParameter
}

func EncodeModificationRequest(ModificationRequest) ([]byte, error)
func DecodeModificationRequest([]byte) (ModificationRequest, error)
func EncodeModificationComplete(ModificationComplete) ([]byte, error)
func DecodeModificationComplete([]byte) (ModificationComplete, error)
func DecodeModificationCommand([]byte) (ModificationCommand, error)
```

The package is consumed across repositories at
`github.com/odango-chan/ca-client-tools/protocol/nas5gsm`. It is a public
compatibility commitment for this scoped API: additive changes are permitted;
removing or changing existing fields/functions requires a coordinated consumer
migration.

## 4. Verification

Owner-local gates:

```powershell
go test ./protocol/nas5gsm/ -count=1
go vet ./...
```

Covered cases:

- exact golden bytes for distinct PSI/PTI, all supported PDU Session Types, SSC
  modes 1..3, with and without ePCO;
- encode→decode→encode round-trip with identical bytes;
- absent PDU Session Type and SSC Mode;
- truncated header/mandatory IE;
- wrong EPD and wrong message type;
- PSI 0/16, PTI 0/255, invalid type and SSC values;
- duplicate PDU Session Type/SSC/ePCO;
- unknown optional IEI;
- ePCO truncated header, length overflow, empty IE, malformed option framing;
- decoded ePCO does not alias the input buffer;
- exact modification-request and modification-command golden vectors,
  decoded by the local Go codec tests; these Go tests alone are not claimed as
  independent OpenAMSM evidence;
- a separate native C oracle runner at
  `tools/nas5gsm-oracle/run.sh`, which invokes the fixed OpenAMSM 2.8.0
  `ogs_nas_5gsm_decode` and `ogs_nas_parse_qos_flow_descriptions` on the same
  checked-in raw 0xC9/0xCB bytes, plus the header-only 0xCC vector. It verifies
  the imported source-tree SHA-256, all 19 env-openamsm build-patch blob
  identities, the source-plus-patch build marker, and byte identity of the
  staged NAS decoder/parser before compiling. The run writes a manifest with
  patch/library/harness hashes and the actual C output;
- **Oracle gate status:** `NOT_RUN` until a successful manifest is attached to
  Issue #25 / PR #26. The existence of the runner and passing Go CI do not
  constitute `NAS_5GSM_MODIFY_OWNER_ORACLE_PASS`;
- modification request/command fail-closed matrix (wrong EPD/type/PSI/PTI,
  missing/duplicate/truncated/overflowing IE, QFI 0, bad operation, delete with
  parameters, create without E, non-zero spare bits, duplicate/unknown
  parameters, wrong parameter lengths, invalid bitrate units);
- `QoSFlowBitrateFromKbps` rejects zero and non-exactly-representable values.

Cross-repo acceptance is the `env-5gc-sim` Mock AMF import and its dynamic N1
generation tests. That import proves the package is reachable without exporting
`internal/*`.

## 5. Deliberate non-goals

- No NAS ciphering/integrity protection.
- No full 5GSM state machine or PDU Session Establishment Accept encoding.
- No NGAP/N2 codec (the Mock AMF owns its narrow N2 Transfer codec).
- No private Owner binary binding; that remains Host/OpenAMSM-owned and is
  tracked under #18.

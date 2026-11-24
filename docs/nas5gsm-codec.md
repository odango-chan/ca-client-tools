# Legacy scoped 5GSM codec (`protocol/nas5gsm`)

Status: **LEGACY / MIGRATION SOURCE** — not the target protocol coverage contract
Version: v0.1
Date: 2026-11-19
Tracking: [ca-client-tools#18](https://github.com/odango-chan/ca-client-tools/issues/18)

## 1. Purpose and boundary

`github.com/odango-chan/ca-client-tools/protocol/nas5gsm` is an intentionally
small, protocol-only Go package. It exists so the Mock AMF in `env-5gc-sim` can
generate a real PDU Session Establishment Request from semantic scenario input
without copying a second NAS decoder into the simulator, and so Host ingress can
perform a bounded semantic projection of an original N1 SM part.

It is:

- a pure encoder/decoder for one 5GSM message and its supported IEs;
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

Unsupported optional IEs, duplicate supported IEs, truncated/overflowing
lengths, invalid enum values and empty ePCO fail closed with a wrapped sentinel:
`ErrMalformed`, `ErrUnsupported` or `ErrInvalidField`.

A zero `PduSessionType`/`SSCMode` means "IE absent"; encoding omits it and
decoding leaves it zero, so `Encode(Decode(b))` preserves the supported
semantic projection.

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
```

The package is currently consumed by `env-5gc-sim` at
`github.com/odango-chan/ca-client-tools/protocol/nas5gsm`. It is retained only
as a migration source until its consumers move to the generic full-standard
NAS API. This scoped API is not the long-term protocol contract and must not
constrain the shape or coverage of the replacement. Coordinate its removal
with all consumers after the migration gates pass.

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
- decoded ePCO does not alias the input buffer.

Cross-repo acceptance is the `env-5gc-sim` Mock AMF import and its dynamic N1
generation tests. That import proves the package is reachable without exporting
`internal/*`.

## 5. Deliberate non-goals

- No NAS ciphering/integrity protection.
- No full 5GSM state machine or PDU Session Establishment Accept encoding.
- No NGAP/N2 codec (the Mock AMF owns its narrow N2 Transfer codec).
- No private Owner binary binding; that remains Host/OpenAMSM-owned and is
  tracked under #18.

## 6. Free5GC-backed integration target

**Status: migration direction; the replacement has not yet been implemented or
qualified.** The scoped \`protocol/nas5gsm\` package above is a legacy migration
source. It must not define the long-term NAS protocol surface.

Use the pinned upstream pure-Go implementation
[\`github.com/free5gc/nas\` v1.3.1](https://github.com/free5gc/nas/tree/v1.3.1)
as the initial NAS candidate, paired with
[\`github.com/free5gc/ngap\` v1.2.0](https://github.com/free5gc/ngap/tree/v1.2.0)
for NGAP and standalone N2 Transfer. Both currently declare Go 1.26.2 and
Apache-2.0 licensing. Pin exact versions and source commits in \`go.mod\` /
\`go.sum\`; preserve upstream license notices.

The NAS boundary remains the **SMF-visible 5GSM N1 SM payload carried across
the AMF-SMF SBI boundary**. It does not include 5GMM registration, UE-AMF
security-header processing, integrity protection, ciphering / deciphering,
NAS security contexts or key lifecycle. A protected UE-AMF envelope is not a
valid v1 input. Preserve the original N1 bytes and SBI metadata independently
of any decoded projection.

Go consumers use the CA facade around the upstream \`message.ParseGSM\` and
message \`MarshalBinary\` APIs. Complete NGAP PDU and standalone N2 Transfer
handling use the upstream \`free5gc/ngap/message\` and \`free5gc/ngap/ie\`
packages. Standalone N2 Transfer requires an explicit root type selected from
trusted SBI context; do not infer it from the bytes.

The Python Host does not maintain another NAS / NGAP parser. It calls a
long-lived worker subcommand of the existing \`ca-client-tools\` executable.
The worker uses bounded, versioned stdio frames with UTF-8 JSON metadata and a
separate raw binary payload. It is not a new daemon, C ABI, native shared
library, Go plugin or Agent-callable Tool. Go consumers call the facade
directly in-process.

The pinned upstream versions are not qualified as full R19 implementations.
The initial audit found APER extension / fragmentation limitations, incomplete
unknown NGAP IE preservation and NAS \`IEToDoList\` paths. Each supported
message and IE needs separate Encode / Decode evidence; unsupported behavior
must be visible and fail closed. Do not fill upstream gaps with duplicated
handwritten parsers in Host and Mock AMF.

Keep this package until all Go and Python consumers have migrated and the
cross-repository gates pass. Then remove it rather than preserving a permanent
compatibility API.

See [architecture §10](architecture.md#10-free5gc-backed-ngapnas-codec-integration-2026-11-24)
and [the implementation Goal](free5gc-codec-integration-goal.md).

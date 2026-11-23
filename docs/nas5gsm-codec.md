# Scoped 5GSM codec (`protocol/nas5gsm`)

Status: Implemented scoped first slice; replacement is an accepted target (2026-11-23)
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

## 6. Upstream implementation migration (2026-11-23)

The implementation above is the **current** locally maintained slice, not the
preferred long-term ASN.1/NAS codec strategy. The approved future direction is
the pure-Go `github.com/free5gc/nas` and `github.com/free5gc/ngap` modules,
wrapped only where our exact 3GPP/N2 SM profiles require type narrowing,
validation or Host-private projection. Do not build another full custom NAS or
APER library. `free5gc/ngap` is a Go implementation of ASN.1 APER and does
not require the C `asn1c` runtime; the optional OAI C library is **not** the
selected Provider dependency.

The detailed owner map, Windows-native/`CGO_ENABLED=0` constraints, real-wire
compatibility matrix and cutover tests are in
[architecture §10](architecture.md#10-ngapnas-upstream-codec-decision-2026-11-23).
Do **not** delete `protocol/nas5gsm` until the upstream replacement passes
those gates and the `env-5gc-sim` import plus Host ingress consumers have
been migrated. Preserve original NAS/N2 payloads; a typed decode is a
projection, never authorization to rewrite the Owner-facing wire data.

Implementation status: **NOT_STARTED**. No live or cross-platform new-codec
qualification is claimed by this documentation change.

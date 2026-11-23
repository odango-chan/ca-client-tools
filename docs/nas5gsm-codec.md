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

## 6. Pluggable protocol module target

This package documents the old, deliberately narrow 5GSM implementation and
its tests. It is a migration source, not the target CA protocol capability
boundary.

The target is a generic, versioned Protocol Codec Runtime with independently
versioned NGAP and NAS codec modules. Full coverage of an entire 3GPP Release
is not a prerequisite. Each module must publish an itemized coverage
declaration: exact TS / Release baseline, supported messages and IEs, separate
encode/decode status, constraints, extension and unknown-field behavior,
tests, provenance and unsupported cases.

Release 19 (TS 38.413 v19.4.0 and TS 24.501 v19.8.0) is the current reference
baseline, not a claim that every R19 message or IE must already be implemented.
A module may support a clearly declared subset, and multiple Release profiles
may coexist.

The preferred cross-platform integration is a long-lived local codec worker
process with a stable, versioned RPC contract. Do not spawn a worker for every
message. The Runtime routes calls according to explicit peer / deployment
profile configuration, validates module manifests and ABI compatibility,
and drains an old worker before replacing it. A native `.so` / `.dll` /
`.dylib` adapter may be added where useful, but a Linux-only `.so` or Go
`plugin` must not be the sole integration mechanism because CA Host and
Provider must remain Windows-capable and `CGO_ENABLED=0`.

Do not assume the Release can be inferred from an individual NAS or NGAP
packet. Select it from trusted peer / deployment configuration or explicit
session context. Keep the original N1/N2 wire bytes and transport metadata.
NAS security-context and key lifecycle remain separate from the message codec.

See [architecture §10](architecture.md#10-pluggable-ngapnas-codec-modules-2026-11-23)
for module manifests, routing, lifecycle, ownership and qualification gates.

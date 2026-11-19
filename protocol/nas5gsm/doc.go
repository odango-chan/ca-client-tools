// Package nas5gsm implements the scoped 5GSM NAS codec used by the Mock AMF
// SBI test peer and by Host ingress validation.
//
// The first slice covers only the PDU Session Establishment Request
// (TS 24.501 §8.3.1, message type 0xC1) and the IEs needed by the
// OpenAMSM PDU session establishment path:
//
//   - PDU session identity (octet 2)
//   - Procedure transaction identity (octet 3)
//   - Integrity protection maximum data rate (mandatory, 2 octets)
//   - PDU Session Type (optional TV, IEI 0x9)
//   - SSC Mode (optional TV, IEI 0xA)
//   - Extended protocol configuration options (optional TLV-E, IEI 0x7B)
//
// Unknown optional IEs and malformed lengths fail closed. This package is a
// protocol codec only: it performs no I/O, holds no state, is not an Agent
// tool and does not authorize any NF mutation.
//
// A zero PduSessionType or SSCMode means "IE absent". Encoding an absent IE
// omits it; decoding an absent IE leaves the zero value, so Encode(Decode(b))
// preserves the supported semantic projection.
package nas5gsm

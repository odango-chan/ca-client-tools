package nas5gsm

import (
	"errors"
	"fmt"
)

// 5GSM wire constants from TS 24.501 and the pinned OpenAMSM NAS sources
// (lib/nas/5gs/message.h, lib/nas/5gs/ies.c).
const (
	// ExtendedProtocolDiscriminator5GSM is the 5GSM EPD (TS 24.501 §9.2).
	ExtendedProtocolDiscriminator5GSM = 0x2e

	// MessageTypePDUSessionEstablishmentRequest is 5GSM message type 0xC1.
	MessageTypePDUSessionEstablishmentRequest = 0xc1

	// IntegrityProtectionMaximumDataRateFull means no integrity-protected
	// rate limit (TS 24.501 §9.11.4.7).
	IntegrityProtectionMaximumDataRateFull = 0xff

	// IEIEPCO is the TLV-E IEI for Extended protocol configuration options.
	IEIEPCO = 0x7b

	// EPCOConfigurationProtocolPPP is the configuration protocol octet that
	// selects the options defined by TS 24.501 §10.5.6.3 (all zero bits: PPP).
	// It is the first octet of the IE content, not an option header.
	EPCOConfigurationProtocolPPP = 0x00

	// ieiPDUSessionType and ieiSSCMode are the high nibbles of the Type-1
	// TV IEs.
	ieiPDUSessionType = 0x90
	ieiSSCMode        = 0xa0
)

// PduSessionType is the 3-bit TS 24.501 PDU session type value.
type PduSessionType uint8

const (
	PduSessionTypeAbsent       PduSessionType = 0
	PduSessionTypeIPv4         PduSessionType = 1
	PduSessionTypeIPv6         PduSessionType = 2
	PduSessionTypeIPv4v6       PduSessionType = 3
	PduSessionTypeUnstructured PduSessionType = 4
	PduSessionTypeEthernet     PduSessionType = 5
)

// Valid reports whether the value is a defined PDU session type or the
// explicit "absent" zero value.
func (t PduSessionType) Valid() bool {
	return t >= PduSessionTypeAbsent && t <= PduSessionTypeEthernet
}

func (t PduSessionType) String() string {
	switch t {
	case PduSessionTypeAbsent:
		return "ABSENT"
	case PduSessionTypeIPv4:
		return "IPV4"
	case PduSessionTypeIPv6:
		return "IPV6"
	case PduSessionTypeIPv4v6:
		return "IPV4V6"
	case PduSessionTypeUnstructured:
		return "UNSTRUCTURED"
	case PduSessionTypeEthernet:
		return "ETHERNET"
	default:
		return fmt.Sprintf("PduSessionType(%d)", uint8(t))
	}
}

// SSCMode is the 3-bit TS 24.501 SSC mode value.
type SSCMode uint8

const (
	SSCModeAbsent SSCMode = 0
	SSCMode1      SSCMode = 1
	SSCMode2      SSCMode = 2
	SSCMode3      SSCMode = 3
)

// Valid reports whether the value is a defined SSC mode or the explicit
// "absent" zero value.
func (m SSCMode) Valid() bool {
	return m >= SSCModeAbsent && m <= SSCMode3
}

func (m SSCMode) String() string {
	switch m {
	case SSCModeAbsent:
		return "ABSENT"
	case SSCMode1:
		return "SSC_MODE_1"
	case SSCMode2:
		return "SSC_MODE_2"
	case SSCMode3:
		return "SSC_MODE_3"
	default:
		return fmt.Sprintf("SSCMode(%d)", uint8(m))
	}
}

// EstablishmentRequest is the semantic input for a PDU Session Establishment
// Request owned by the codec. EPCO holds the raw IE content exactly as it
// appears on the wire: one configuration-protocol octet followed by zero or more
// {protocol id uint16, length uint8, value} options (TS 24.501 §10.5.6.3). It is
// not an arbitrary opaque NAS blob and it is never rewritten.
type EstablishmentRequest struct {
	PduSessionID   uint8
	PTI            uint8
	PduSessionType PduSessionType
	SSCMode        SSCMode
	EPCO           []byte
}

// Sentinel errors returned by this package. Callers can use errors.Is.
var (
	ErrMalformed    = errors.New("nas5gsm: malformed message")
	ErrUnsupported  = errors.New("nas5gsm: unsupported protocol element")
	ErrInvalidField = errors.New("nas5gsm: invalid field")
)

func validateEstablishmentRequest(req EstablishmentRequest) error {
	if req.PduSessionID < 1 || req.PduSessionID > 15 {
		return fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, req.PduSessionID)
	}
	if req.PTI < 1 || req.PTI > 254 {
		return fmt.Errorf("%w: pti %d outside 1..254", ErrInvalidField, req.PTI)
	}
	if !req.PduSessionType.Valid() {
		return fmt.Errorf("%w: pduSessionType %d", ErrInvalidField, req.PduSessionType)
	}
	if !req.SSCMode.Valid() {
		return fmt.Errorf("%w: sscMode %d", ErrInvalidField, req.SSCMode)
	}
	if err := ValidateEPCO(req.EPCO); err != nil {
		return err
	}
	return nil
}

// ValidateEPCO checks that b is a well-formed Extended protocol configuration
// options container as defined by TS 24.501 §9.11.4.8 / §10.5.6.3:
//
//	EPCO contents = configuration protocol (1 octet) || option*
//	option        = protocol id (2 octets) || length (1 octet) || value (length)
//
// The first octet is the configuration protocol, so the option walk starts at
// offset 1. An empty slice means "no ePCO IE" and is accepted. A single octet
// (configuration protocol only, no options) is structurally valid. The
// configuration protocol value is not interpreted: an unknown value is not an
// error here, and nothing in the container is dropped or rewritten.
//
// The check is read-only: b is never modified, and the caller keeps
// responsibility for the original NAS bytes.
func ValidateEPCO(b []byte) error {
	if len(b) == 0 {
		// No ePCO IE at all.
		return nil
	}
	if len(b) > 0xffff {
		return fmt.Errorf("%w: ePCO length %d exceeds 65535", ErrInvalidField, len(b))
	}
	// b[0] is the configuration protocol octet, not an option header.
	for i := 1; i < len(b); {
		if len(b)-i < 3 {
			return fmt.Errorf("%w: truncated ePCO option header at offset %d (%d octet(s) left, need 3)", ErrMalformed, i, len(b)-i)
		}
		plen := int(b[i+2])
		if len(b)-i-3 < plen {
			return fmt.Errorf("%w: ePCO option length %d exceeds remaining %d at offset %d", ErrMalformed, plen, len(b)-i-3, i)
		}
		i += 3 + plen
	}
	return nil
}

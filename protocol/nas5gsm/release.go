package nas5gsm

import "fmt"

// 5GSM PDU session release message types from TS 24.501 and the pinned
// OpenAMSM NAS sources (lib/nas/5gs/message.h):
//
//	OGS_NAS_5GS_PDU_SESSION_RELEASE_REQUEST  = 209 (0xd1)
//	OGS_NAS_5GS_PDU_SESSION_RELEASE_COMMAND  = 211 (0xd3)
//	OGS_NAS_5GS_PDU_SESSION_RELEASE_COMPLETE = 212 (0xd4)
const (
	MessageTypePDUSessionReleaseRequest  = 0xd1
	MessageTypePDUSessionReleaseCommand  = 0xd3
	MessageTypePDUSessionReleaseComplete = 0xd4
)

// ReleaseRequest is the UE-side PDU Session Release Request. The mock peer uses
// the header-only form TS 24.501 permits; no optional IE is required for the
// release procedure.
type ReleaseRequest struct {
	PduSessionID uint8
	PTI          uint8
}

// ReleaseComplete is the UE-side PDU Session Release Complete (header only).
type ReleaseComplete struct {
	PduSessionID uint8
	PTI          uint8
}

// ReleaseCommand is the decoded SMF-side PDU Session Release Command header.
type ReleaseCommand struct {
	PduSessionID uint8
	PTI          uint8
}

func validateReleaseHeader(pduSessionID, pti uint8) error {
	if pduSessionID < 1 || pduSessionID > 15 {
		return fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, pduSessionID)
	}
	if pti < 1 || pti > 254 {
		return fmt.Errorf("%w: pti %d outside 1..254", ErrInvalidField, pti)
	}
	return nil
}

func encodeReleaseHeader(pduSessionID, pti, messageType uint8) ([]byte, error) {
	if err := validateReleaseHeader(pduSessionID, pti); err != nil {
		return nil, err
	}
	return []byte{
		ExtendedProtocolDiscriminator5GSM,
		pduSessionID,
		pti,
		messageType,
	}, nil
}

// EncodeReleaseRequest encodes a 5GSM PDU Session Release Request header.
func EncodeReleaseRequest(req ReleaseRequest) ([]byte, error) {
	return encodeReleaseHeader(req.PduSessionID, req.PTI, MessageTypePDUSessionReleaseRequest)
}

// EncodeReleaseComplete encodes a 5GSM PDU Session Release Complete header.
func EncodeReleaseComplete(req ReleaseComplete) ([]byte, error) {
	return encodeReleaseHeader(req.PduSessionID, req.PTI, MessageTypePDUSessionReleaseComplete)
}

// DecodeReleaseCommand validates and decodes a 5GSM PDU Session Release
// Command header. Trailing optional IEs are allowed and not interpreted.
func DecodeReleaseCommand(b []byte) (ReleaseCommand, error) {
	if len(b) < 4 {
		return ReleaseCommand{}, fmt.Errorf("%w: release command length %d < 4", ErrMalformed, len(b))
	}
	if b[0] != ExtendedProtocolDiscriminator5GSM {
		return ReleaseCommand{}, fmt.Errorf("%w: EPD 0x%02x", ErrMalformed, b[0])
	}
	if b[3] != MessageTypePDUSessionReleaseCommand {
		return ReleaseCommand{}, fmt.Errorf("%w: message type 0x%02x is not release command", ErrUnsupported, b[3])
	}
	psi, pti := b[1], b[2]
	if psi < 1 || psi > 15 {
		return ReleaseCommand{}, fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, psi)
	}
	if pti > 254 {
		return ReleaseCommand{}, fmt.Errorf("%w: pti %d outside 0..254", ErrInvalidField, pti)
	}
	return ReleaseCommand{PduSessionID: psi, PTI: pti}, nil
}

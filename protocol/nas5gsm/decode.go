package nas5gsm

import "fmt"

// DecodeEstablishmentRequest decodes and validates a PDU Session Establishment
// Request. It fails closed on truncation, invalid mandatory fields, duplicate
// supported IEs and any unsupported optional IE.
func DecodeEstablishmentRequest(msg []byte) (EstablishmentRequest, error) {
	var req EstablishmentRequest

	if len(msg) < 6 {
		return req, fmt.Errorf("%w: message length %d shorter than 5GSM header plus mandatory IE", ErrMalformed, len(msg))
	}
	if msg[0] != ExtendedProtocolDiscriminator5GSM {
		return req, fmt.Errorf("%w: extended protocol discriminator 0x%02x is not 5GSM", ErrMalformed, msg[0])
	}
	if msg[3] != MessageTypePDUSessionEstablishmentRequest {
		return req, fmt.Errorf("%w: message type 0x%02x is not PDU Session Establishment Request", ErrUnsupported, msg[3])
	}

	req.PduSessionID = msg[1]
	req.PTI = msg[2]
	if req.PduSessionID < 1 || req.PduSessionID > 15 {
		return EstablishmentRequest{}, fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, req.PduSessionID)
	}
	if req.PTI < 1 || req.PTI > 254 {
		return EstablishmentRequest{}, fmt.Errorf("%w: pti %d outside 1..254", ErrInvalidField, req.PTI)
	}

	// msg[4], msg[5] are the mandatory integrity protection maximum data
	// rate. It is not part of this scoped semantic projection, but the two
	// octets must exist (checked above).

	var (
		haveType bool
		haveSSC  bool
		haveEPCO bool
	)
	for i := 6; i < len(msg); {
		iei := msg[i]
		switch {
		case iei&0xf0 == ieiPDUSessionType:
			if haveType {
				return EstablishmentRequest{}, fmt.Errorf("%w: duplicate PDU Session Type IE", ErrMalformed)
			}
			haveType = true
			req.PduSessionType = PduSessionType(iei & 0x07)
			if !req.PduSessionType.Valid() || req.PduSessionType == PduSessionTypeAbsent {
				return EstablishmentRequest{}, fmt.Errorf("%w: pduSessionType %d", ErrInvalidField, req.PduSessionType)
			}
			i++

		case iei&0xf0 == ieiSSCMode:
			if haveSSC {
				return EstablishmentRequest{}, fmt.Errorf("%w: duplicate SSC Mode IE", ErrMalformed)
			}
			haveSSC = true
			req.SSCMode = SSCMode(iei & 0x07)
			if !req.SSCMode.Valid() || req.SSCMode == SSCModeAbsent {
				return EstablishmentRequest{}, fmt.Errorf("%w: sscMode %d", ErrInvalidField, req.SSCMode)
			}
			i++

		case iei == IEIEPCO:
			if haveEPCO {
				return EstablishmentRequest{}, fmt.Errorf("%w: duplicate ePCO IE", ErrMalformed)
			}
			haveEPCO = true
			if len(msg)-i < 3 {
				return EstablishmentRequest{}, fmt.Errorf("%w: truncated ePCO header", ErrMalformed)
			}
			length := int(msg[i+1])<<8 | int(msg[i+2])
			start := i + 3
			end := start + length
			if end > len(msg) {
				return EstablishmentRequest{}, fmt.Errorf("%w: ePCO length %d exceeds remaining %d", ErrMalformed, length, len(msg)-start)
			}
			if length == 0 {
				return EstablishmentRequest{}, fmt.Errorf("%w: empty ePCO IE", ErrMalformed)
			}
			req.EPCO = append([]byte(nil), msg[start:end]...)
			if err := ValidateEPCO(req.EPCO); err != nil {
				return EstablishmentRequest{}, err
			}
			i = end

		default:
			return EstablishmentRequest{}, fmt.Errorf("%w: optional IEI 0x%02x at offset %d", ErrUnsupported, iei, i)
		}
	}

	return req, nil
}

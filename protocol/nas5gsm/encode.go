package nas5gsm

import "fmt"

// EncodeEstablishmentRequest encodes a PDU Session Establishment Request using
// the pinned TS 24.501 / OpenAMSM wire layout. The result is freshly allocated
// for this request; no global or reference byte array is reused.
func EncodeEstablishmentRequest(req EstablishmentRequest) ([]byte, error) {
	if err := validateEstablishmentRequest(req); err != nil {
		return nil, err
	}

	out := make([]byte, 0, 8+len(req.EPCO))
	out = append(out,
		ExtendedProtocolDiscriminator5GSM,
		req.PduSessionID,
		req.PTI,
		MessageTypePDUSessionEstablishmentRequest,
		// Integrity protection maximum data rate: UL then DL, full rate.
		IntegrityProtectionMaximumDataRateFull,
		IntegrityProtectionMaximumDataRateFull,
	)
	if req.PduSessionType != PduSessionTypeAbsent {
		out = append(out, ieiPDUSessionType|uint8(req.PduSessionType))
	}
	if req.SSCMode != SSCModeAbsent {
		out = append(out, ieiSSCMode|uint8(req.SSCMode))
	}
	if len(req.EPCO) > 0 {
		if len(req.EPCO) > 0xffff {
			return nil, fmt.Errorf("%w: ePCO length %d exceeds 65535", ErrInvalidField, len(req.EPCO))
		}
		out = append(out, IEIEPCO, byte(len(req.EPCO)>>8), byte(len(req.EPCO)))
		out = append(out, req.EPCO...)
	}
	return out, nil
}

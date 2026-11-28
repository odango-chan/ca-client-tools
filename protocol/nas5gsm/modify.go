package nas5gsm

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// 5GSM PDU session modification message types from TS 24.501 and the pinned
// OpenAMSM NAS sources (lib/nas/5gs/message.h):
//
//	OGS_NAS_5GS_PDU_SESSION_MODIFICATION_REQUEST  = 201 (0xc9)
//	OGS_NAS_5GS_PDU_SESSION_MODIFICATION_REJECT   = 202 (0xca)
//	OGS_NAS_5GS_PDU_SESSION_MODIFICATION_COMMAND  = 203 (0xcb)
//	OGS_NAS_5GS_PDU_SESSION_MODIFICATION_COMPLETE = 204 (0xcc)
const (
	MessageTypePDUSessionModificationRequest  = 0xc9
	MessageTypePDUSessionModificationReject   = 0xca
	MessageTypePDUSessionModificationCommand  = 0xcb
	MessageTypePDUSessionModificationComplete = 0xcc
)

// IEIQoSFlowDescriptions is the TLV-E IEI used by both the Requested QoS flow
// descriptions (modification request) and the Authorized QoS flow
// descriptions (modification command).
const IEIQoSFlowDescriptions = 0x79

// QoSFlowOperation is the TS 24.501 §9.11.4.12 QoS flow description operation
// code.
type QoSFlowOperation uint8

const (
	// QoSFlowOperationCreate creates a new QoS flow description.
	QoSFlowOperationCreate QoSFlowOperation = 1
	// QoSFlowOperationDelete deletes an existing QoS flow description. It
	// carries no parameter list.
	QoSFlowOperationDelete QoSFlowOperation = 2
	// QoSFlowOperationModify modifies an existing QoS flow description.
	QoSFlowOperationModify QoSFlowOperation = 3
)

func (o QoSFlowOperation) Valid() bool {
	return o == QoSFlowOperationCreate || o == QoSFlowOperationDelete || o == QoSFlowOperationModify
}

func (o QoSFlowOperation) String() string {
	switch o {
	case QoSFlowOperationCreate:
		return "CREATE"
	case QoSFlowOperationDelete:
		return "DELETE"
	case QoSFlowOperationModify:
		return "MODIFY"
	default:
		return fmt.Sprintf("QoSFlowOperation(%d)", uint8(o))
	}
}

// QoSFlowParameterIdentifier is the TS 24.501 §9.11.4.12 parameter identifier.
// Only the identifiers consumed by the pinned OpenAMSM parser are supported;
// every other identifier fails closed instead of being forwarded blind.
type QoSFlowParameterIdentifier uint8

const (
	QoSFlowParameterFiveQI       QoSFlowParameterIdentifier = 1
	QoSFlowParameterGFBRUplink   QoSFlowParameterIdentifier = 2
	QoSFlowParameterGFBRDownlink QoSFlowParameterIdentifier = 3
	QoSFlowParameterMFBRUplink   QoSFlowParameterIdentifier = 4
	QoSFlowParameterMFBRDownlink QoSFlowParameterIdentifier = 5
)

// Bitrate units from the pinned OpenAMSM lib/nas/common/types.h. OpenAMSM
// treats unit 0 as invalid, so the first supported unit is 1 (= 1 Kbps).
const (
	bitrateUnit1K   = 1
	bitrateUnit256P = 25
)

// QoSFlowBitrate is one TS 24.501 §9.11.4.12 bitrate parameter value: a
// 2-octet value multiplied by the unit.
type QoSFlowBitrate struct {
	Value uint16
	Unit  uint8
}

// QoSFlowBitrateFromKbps builds a bitrate parameter from a value in Kbps. It
// picks the smallest supported unit that can represent the value exactly, so
// the encoding is canonical and the original value round-trips.
func QoSFlowBitrateFromKbps(kbps uint64) (QoSFlowBitrate, error) {
	if kbps == 0 {
		return QoSFlowBitrate{}, fmt.Errorf("%w: bitrate must be greater than zero", ErrInvalidField)
	}
	unit := uint64(bitrateUnit1K)
	value := kbps
	for value > 0xffff {
		value = (value + 3) / 4
		unit++
	}
	if unit > bitrateUnit256P {
		return QoSFlowBitrate{}, fmt.Errorf("%w: bitrate %d Kbps exceeds the supported range", ErrInvalidField, kbps)
	}
	// Reject values that the chosen unit cannot represent exactly instead of
	// silently rounding the request.
	if value*(1<<(2*(unit-1))) != kbps {
		return QoSFlowBitrate{}, fmt.Errorf("%w: bitrate %d Kbps is not exactly representable", ErrInvalidField, kbps)
	}
	return QoSFlowBitrate{Value: uint16(value), Unit: uint8(unit)}, nil
}

// BitsPerSecond converts the parameter back to bits per second using the
// pinned OpenAMSM unit semantics. It returns an error when the exact result
// cannot be represented by uint64.
func (b QoSFlowBitrate) BitsPerSecond() (uint64, error) {
	if err := b.validate(); err != nil {
		return 0, err
	}
	scale := uint64(1000) << (2 * (uint64(b.Unit) - 1))
	hi, lo := bits.Mul64(uint64(b.Value), scale)
	if hi != 0 {
		return 0, fmt.Errorf("nas5gsm: bitrate %d/%d overflows uint64 bits per second", b.Value, b.Unit)
	}
	return lo, nil
}

func (b QoSFlowBitrate) validate() error {
	if b.Unit < bitrateUnit1K || b.Unit > bitrateUnit256P {
		return fmt.Errorf("%w: bitrate unit %d outside %d..%d", ErrInvalidField, b.Unit, bitrateUnit1K, bitrateUnit256P)
	}
	return nil
}

// QoSFlowParameter is one parameter of a QoS flow description. Exactly the
// field matching Identifier is used.
type QoSFlowParameter struct {
	Identifier QoSFlowParameterIdentifier
	FiveQI     uint8
	Bitrate    QoSFlowBitrate
}

// MaxQoSFlowParameters mirrors OGS_NAS_MAX_NUM_OF_QOS_FLOW_PARAMETER.
const MaxQoSFlowParameters = 8

// QoSFlowDescription is a TS 24.501 §9.11.4.12 QoS flow description. For the
// modify operation EBit selects replacement (true) or extension (false) of the
// previously provided parameters.
type QoSFlowDescription struct {
	QFI        uint8
	Operation  QoSFlowOperation
	ReplaceAll bool
	Parameters []QoSFlowParameter
}

// ModificationRequest is the UE-side PDU Session Modification Request (0xc9)
// limited to the requested QoS flow descriptions subset that the pinned
// OpenAMSM decoder/handler consumes. Requested QoS rules and every other IE
// are rejected as unsupported.
type ModificationRequest struct {
	PduSessionID        uint8
	PTI                 uint8
	QoSFlowDescriptions []QoSFlowDescription
}

// ModificationComplete is the UE-side PDU Session Modification Complete
// (0xcc) header. The scoped codec emits and accepts the header-only form.
type ModificationComplete struct {
	PduSessionID uint8
	PTI          uint8
}

// ModificationCommand is the SMF-side PDU Session Modification Command (0xcb)
// projection used to correlate the procedure. Only the header and the
// Authorized QoS flow descriptions IE are interpreted; any other IE fails
// closed so the peer never fabricates a correlation it did not observe.
type ModificationCommand struct {
	PduSessionID                  uint8
	PTI                           uint8
	AuthorizedQoSFlowDescriptions []QoSFlowDescription
}

func validateQoSFlowDescription(d QoSFlowDescription) error {
	if d.QFI < 1 || d.QFI > 63 {
		return fmt.Errorf("%w: qfi %d outside 1..63", ErrInvalidField, d.QFI)
	}
	if !d.Operation.Valid() {
		return fmt.Errorf("%w: qos flow operation %d", ErrInvalidField, d.Operation)
	}
	if len(d.Parameters) > MaxQoSFlowParameters {
		return fmt.Errorf("%w: %d qos flow parameters exceed %d", ErrInvalidField, len(d.Parameters), MaxQoSFlowParameters)
	}
	switch d.Operation {
	case QoSFlowOperationDelete:
		if len(d.Parameters) != 0 || d.ReplaceAll {
			return fmt.Errorf("%w: delete qos flow description must not carry parameters", ErrInvalidField)
		}
	case QoSFlowOperationCreate:
		if len(d.Parameters) == 0 || !d.ReplaceAll {
			return fmt.Errorf("%w: create qos flow description requires E=1 and a parameter list", ErrInvalidField)
		}
	}
	seen := make(map[QoSFlowParameterIdentifier]bool, len(d.Parameters))
	for _, p := range d.Parameters {
		if seen[p.Identifier] {
			return fmt.Errorf("%w: duplicate qos flow parameter identifier %d", ErrMalformed, p.Identifier)
		}
		seen[p.Identifier] = true
		switch p.Identifier {
		case QoSFlowParameterFiveQI:
			if p.FiveQI < 1 || p.FiveQI > 255 {
				return fmt.Errorf("%w: 5qi %d", ErrInvalidField, p.FiveQI)
			}
		case QoSFlowParameterGFBRUplink, QoSFlowParameterGFBRDownlink,
			QoSFlowParameterMFBRUplink, QoSFlowParameterMFBRDownlink:
			if err := p.Bitrate.validate(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: qos flow parameter identifier %d", ErrUnsupported, p.Identifier)
		}
	}
	return nil
}

func appendQoSFlowDescriptions(out []byte, descriptions []QoSFlowDescription) ([]byte, error) {
	value := make([]byte, 0, 8*len(descriptions))
	seenQFI := make(map[uint8]bool, len(descriptions))
	for _, d := range descriptions {
		if err := validateQoSFlowDescription(d); err != nil {
			return nil, err
		}
		if seenQFI[d.QFI] {
			return nil, fmt.Errorf("%w: duplicate qfi %d", ErrMalformed, d.QFI)
		}
		seenQFI[d.QFI] = true

		eBit := uint8(0)
		if d.ReplaceAll {
			eBit = 1
		}
		value = append(value, d.QFI&0x3f, (uint8(d.Operation)&0x07)<<5, eBit<<6|uint8(len(d.Parameters))&0x3f)
		for _, p := range d.Parameters {
			value = append(value, uint8(p.Identifier))
			switch p.Identifier {
			case QoSFlowParameterFiveQI:
				value = append(value, 1, p.FiveQI)
			case QoSFlowParameterGFBRUplink, QoSFlowParameterGFBRDownlink,
				QoSFlowParameterMFBRUplink, QoSFlowParameterMFBRDownlink:
				var encoded [2]byte
				binary.BigEndian.PutUint16(encoded[:], p.Bitrate.Value)
				value = append(value, 3, p.Bitrate.Unit, encoded[0], encoded[1])
			default:
				return nil, fmt.Errorf("%w: qos flow parameter identifier %d", ErrUnsupported, p.Identifier)
			}
		}
	}
	if len(descriptions) == 0 {
		return out, nil
	}
	if len(value) > 0xffff {
		return nil, fmt.Errorf("%w: qos flow descriptions length %d exceeds 65535", ErrInvalidField, len(value))
	}
	out = append(out, IEIQoSFlowDescriptions, byte(len(value)>>8), byte(len(value)))
	return append(out, value...), nil
}

func parseQoSFlowDescriptions(value []byte) ([]QoSFlowDescription, error) {
	var descriptions []QoSFlowDescription
	seenQFI := make(map[uint8]bool)
	for i := 0; i < len(value); {
		if len(value)-i < 3 {
			return nil, fmt.Errorf("%w: truncated qos flow description header at offset %d", ErrMalformed, i)
		}
		octet1, octet2, octet3 := value[i], value[i+1], value[i+2]
		if octet1&0xc0 != 0 || octet2&0x1f != 0 || octet3&0x80 != 0 {
			return nil, fmt.Errorf("%w: non-zero QoS flow description spare bits at offset %d", ErrMalformed, i)
		}
		d := QoSFlowDescription{
			QFI:        octet1 & 0x3f,
			Operation:  QoSFlowOperation((octet2 >> 5) & 0x07),
			ReplaceAll: octet3&0x40 != 0,
		}
		numParams := int(octet3 & 0x3f)
		if numParams > MaxQoSFlowParameters {
			return nil, fmt.Errorf("%w: %d qos flow parameters exceed %d", ErrInvalidField, numParams, MaxQoSFlowParameters)
		}
		i += 3
		for p := 0; p < numParams; p++ {
			if len(value)-i < 2 {
				return nil, fmt.Errorf("%w: truncated qos flow parameter header at offset %d", ErrMalformed, i)
			}
			id := QoSFlowParameterIdentifier(value[i])
			plen := int(value[i+1])
			i += 2
			if len(value)-i < plen {
				return nil, fmt.Errorf("%w: qos flow parameter %d length %d exceeds remaining %d", ErrMalformed, id, plen, len(value)-i)
			}
			param := QoSFlowParameter{Identifier: id}
			switch id {
			case QoSFlowParameterFiveQI:
				if plen != 1 {
					return nil, fmt.Errorf("%w: 5qi parameter length %d", ErrMalformed, plen)
				}
				param.FiveQI = value[i]
			case QoSFlowParameterGFBRUplink, QoSFlowParameterGFBRDownlink,
				QoSFlowParameterMFBRUplink, QoSFlowParameterMFBRDownlink:
				if plen != 3 {
					return nil, fmt.Errorf("%w: bitrate parameter length %d", ErrMalformed, plen)
				}
				param.Bitrate = QoSFlowBitrate{Unit: value[i], Value: binary.BigEndian.Uint16(value[i+1 : i+3])}
			default:
				return nil, fmt.Errorf("%w: qos flow parameter identifier %d", ErrUnsupported, id)
			}
			i += plen
			d.Parameters = append(d.Parameters, param)
		}
		if err := validateQoSFlowDescription(d); err != nil {
			return nil, err
		}
		if seenQFI[d.QFI] {
			return nil, fmt.Errorf("%w: duplicate qfi %d", ErrMalformed, d.QFI)
		}
		seenQFI[d.QFI] = true
		descriptions = append(descriptions, d)
	}
	if len(descriptions) == 0 {
		return nil, fmt.Errorf("%w: empty qos flow descriptions IE", ErrMalformed)
	}
	return descriptions, nil
}

func validateModificationHeader(pduSessionID, pti uint8) error {
	if pduSessionID < 1 || pduSessionID > 15 {
		return fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, pduSessionID)
	}
	if pti < 1 || pti > 254 {
		return fmt.Errorf("%w: pti %d outside 1..254", ErrInvalidField, pti)
	}
	return nil
}

// EncodeModificationRequest encodes a PDU Session Modification Request carrying
// one Requested QoS flow descriptions IE.
func EncodeModificationRequest(req ModificationRequest) ([]byte, error) {
	if err := validateModificationHeader(req.PduSessionID, req.PTI); err != nil {
		return nil, err
	}
	if len(req.QoSFlowDescriptions) == 0 {
		return nil, fmt.Errorf("%w: modification request requires at least one qos flow description", ErrInvalidField)
	}
	out := []byte{
		ExtendedProtocolDiscriminator5GSM,
		req.PduSessionID,
		req.PTI,
		MessageTypePDUSessionModificationRequest,
	}
	return appendQoSFlowDescriptions(out, req.QoSFlowDescriptions)
}

// DecodeModificationRequest decodes and validates a PDU Session Modification
// Request limited to the Requested QoS flow descriptions IE.
func DecodeModificationRequest(msg []byte) (ModificationRequest, error) {
	var req ModificationRequest
	if len(msg) < 4 {
		return req, fmt.Errorf("%w: message length %d shorter than 5GSM header", ErrMalformed, len(msg))
	}
	if msg[0] != ExtendedProtocolDiscriminator5GSM {
		return req, fmt.Errorf("%w: extended protocol discriminator 0x%02x is not 5GSM", ErrMalformed, msg[0])
	}
	if msg[3] != MessageTypePDUSessionModificationRequest {
		return req, fmt.Errorf("%w: message type 0x%02x is not PDU Session Modification Request", ErrUnsupported, msg[3])
	}
	req.PduSessionID, req.PTI = msg[1], msg[2]
	if err := validateModificationHeader(req.PduSessionID, req.PTI); err != nil {
		return ModificationRequest{}, err
	}
	var haveQoSFlowDescriptions bool
	for i := 4; i < len(msg); {
		iei := msg[i]
		if iei != IEIQoSFlowDescriptions {
			return ModificationRequest{}, fmt.Errorf("%w: optional IEI 0x%02x at offset %d", ErrUnsupported, iei, i)
		}
		if haveQoSFlowDescriptions {
			return ModificationRequest{}, fmt.Errorf("%w: duplicate qos flow descriptions IE", ErrMalformed)
		}
		haveQoSFlowDescriptions = true
		if len(msg)-i < 3 {
			return ModificationRequest{}, fmt.Errorf("%w: truncated qos flow descriptions header", ErrMalformed)
		}
		length := int(msg[i+1])<<8 | int(msg[i+2])
		start := i + 3
		end := start + length
		if end > len(msg) {
			return ModificationRequest{}, fmt.Errorf("%w: qos flow descriptions length %d exceeds remaining %d", ErrMalformed, length, len(msg)-start)
		}
		descriptions, err := parseQoSFlowDescriptions(msg[start:end])
		if err != nil {
			return ModificationRequest{}, err
		}
		req.QoSFlowDescriptions = descriptions
		i = end
	}
	if !haveQoSFlowDescriptions {
		return ModificationRequest{}, fmt.Errorf("%w: missing requested qos flow descriptions", ErrInvalidField)
	}
	return req, nil
}

// EncodeModificationComplete encodes the header-only PDU Session Modification
// Complete.
func EncodeModificationComplete(req ModificationComplete) ([]byte, error) {
	if err := validateModificationHeader(req.PduSessionID, req.PTI); err != nil {
		return nil, err
	}
	return []byte{
		ExtendedProtocolDiscriminator5GSM,
		req.PduSessionID,
		req.PTI,
		MessageTypePDUSessionModificationComplete,
	}, nil
}

// DecodeModificationComplete validates the header-only PDU Session
// Modification Complete.
func DecodeModificationComplete(msg []byte) (ModificationComplete, error) {
	var req ModificationComplete
	if len(msg) != 4 {
		return req, fmt.Errorf("%w: modification complete length %d is not the supported header-only form", ErrUnsupported, len(msg))
	}
	if msg[0] != ExtendedProtocolDiscriminator5GSM {
		return req, fmt.Errorf("%w: extended protocol discriminator 0x%02x is not 5GSM", ErrMalformed, msg[0])
	}
	if msg[3] != MessageTypePDUSessionModificationComplete {
		return req, fmt.Errorf("%w: message type 0x%02x is not PDU Session Modification Complete", ErrUnsupported, msg[3])
	}
	req.PduSessionID, req.PTI = msg[1], msg[2]
	if err := validateModificationHeader(req.PduSessionID, req.PTI); err != nil {
		return ModificationComplete{}, err
	}
	return req, nil
}

// DecodeModificationCommand decodes the header and the Authorized QoS flow
// descriptions IE of a PDU Session Modification Command. Any other IE fails
// closed.
func DecodeModificationCommand(msg []byte) (ModificationCommand, error) {
	var cmd ModificationCommand
	if len(msg) < 4 {
		return cmd, fmt.Errorf("%w: message length %d shorter than 5GSM header", ErrMalformed, len(msg))
	}
	if msg[0] != ExtendedProtocolDiscriminator5GSM {
		return cmd, fmt.Errorf("%w: extended protocol discriminator 0x%02x is not 5GSM", ErrMalformed, msg[0])
	}
	if msg[3] != MessageTypePDUSessionModificationCommand {
		return cmd, fmt.Errorf("%w: message type 0x%02x is not PDU Session Modification Command", ErrUnsupported, msg[3])
	}
	cmd.PduSessionID, cmd.PTI = msg[1], msg[2]
	if cmd.PduSessionID < 1 || cmd.PduSessionID > 15 {
		return ModificationCommand{}, fmt.Errorf("%w: pduSessionId %d outside 1..15", ErrInvalidField, cmd.PduSessionID)
	}
	if cmd.PTI > 254 {
		return ModificationCommand{}, fmt.Errorf("%w: pti %d outside 0..254", ErrInvalidField, cmd.PTI)
	}
	var haveQoSFlowDescriptions bool
	for i := 4; i < len(msg); {
		iei := msg[i]
		if iei != IEIQoSFlowDescriptions {
			return ModificationCommand{}, fmt.Errorf("%w: optional IEI 0x%02x at offset %d", ErrUnsupported, iei, i)
		}
		if haveQoSFlowDescriptions {
			return ModificationCommand{}, fmt.Errorf("%w: duplicate authorized qos flow descriptions IE", ErrMalformed)
		}
		haveQoSFlowDescriptions = true
		if len(msg)-i < 3 {
			return ModificationCommand{}, fmt.Errorf("%w: truncated authorized qos flow descriptions header", ErrMalformed)
		}
		length := int(msg[i+1])<<8 | int(msg[i+2])
		start := i + 3
		end := start + length
		if end > len(msg) {
			return ModificationCommand{}, fmt.Errorf("%w: authorized qos flow descriptions length %d exceeds remaining %d", ErrMalformed, length, len(msg)-start)
		}
		descriptions, err := parseQoSFlowDescriptions(msg[start:end])
		if err != nil {
			return ModificationCommand{}, err
		}
		cmd.AuthorizedQoSFlowDescriptions = descriptions
		i = end
	}
	return cmd, nil
}

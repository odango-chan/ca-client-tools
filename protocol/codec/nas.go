package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"

	"github.com/free5gc/nas/message"
)

const (
	nasEPD5GSM   = 0x2e
	nasHeaderLen = 4
	nasPSIMin    = 1
	nasPSIMax    = 15
	nasPTIMax    = 254
)

type nasIEMeta struct {
	IEI     uint8
	Field   string
	Type    string
	Framing string
	Size    int
	Partial bool
}

type nasMessageMeta struct {
	StructName string
	TypeName   string
	Mandatory  []nasIEMeta
	Optional   []nasIEMeta
}

type nasScanIssue struct {
	status Status
	diag   Diagnostic
}

// DecodeNAS decodes one AMF-SMF N1 SM 5GSM message. The input buffer is never
// mutated or retained; Raw is a private copy. A successful structural decode is
// not automatically re-encode eligible: the facade re-encodes the typed value
// solely as a byte-identity check and reports the result in
// ReencodeVerified/ReencodeEligible.
func DecodeNAS(input []byte) DecodeResult {
	raw := append([]byte(nil), input...)
	result := DecodeResult{
		Protocol:     ProtocolNAS,
		Status:       StatusInvalid,
		Raw:          raw,
		RawLength:    len(raw),
		RawPreserved: true,
		TotalBits:    len(raw) * 8,
	}
	result.Diagnostics = append(result.Diagnostics, diagnostic(DiagRawPreserved, "raw NAS bytes retained unchanged"))
	if len(raw) < nasHeaderLen {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASLength, "NAS message is shorter than the 4-octet 5GSM header"))
		return result
	}
	if raw[0] != nasEPD5GSM {
		result.Status = StatusUnsupportedMessage
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASHeaderInvalid,
			fmt.Sprintf("extended protocol discriminator 0x%02x is not 5GSM (0x2e)", raw[0])))
		return result
	}
	psi := raw[1]
	pti := raw[2]
	if psi < nasPSIMin || psi > nasPSIMax || pti > nasPTIMax {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASHeaderInvalid,
			fmt.Sprintf("invalid PSI %d or PTI %d", psi, pti)))
		return result
	}
	meta, ok := nasMessages[raw[3]]
	if !ok {
		result.Status = StatusUnsupportedMessage
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASUnsupportedMessage,
			fmt.Sprintf("upstream ParseGSM has no 5GSM decoder for message type 0x%02x", raw[3])))
		return result
	}
	result.MessageType = meta.StructName
	result.TypeName = meta.TypeName

	scan := scanNAS(raw, meta)
	if scan != nil {
		result.Status = scan.status
		result.Diagnostics = append(result.Diagnostics, scan.diag)
		return result
	}

	msg, err := message.ParseGSM(append([]byte(nil), raw...))
	if err != nil {
		var nasErr *message.Error
		if errors.As(err, &nasErr) && len(nasErr.IEToDoList) > 0 {
			result.Status = StatusPartial
			result.Complete = false
			result.IEToDo = append([]uint8(nil), nasErr.IEToDoList...)
			result.Decoded = msg
			result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASIEToDo,
				fmt.Sprintf("upstream typed IEToDoList reports unimplemented IEIs %v", nasErr.IEToDoList)))
			return result
		}
		result.Status = StatusInvalid
		result.Decoded = msg
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASLength, "upstream 5GSM decoder rejected the message"))
		return result
	}
	if msg == nil {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASLength, "upstream 5GSM decoder returned no message"))
		return result
	}

	result.Status = StatusSuccess
	result.Complete = true
	result.Decoded = msg
	if out, encErr := msg.MarshalBinary(); encErr != nil {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASRoundTripMismatch,
			"the decoded value cannot be re-encoded by the upstream encoder"))
		return result
	} else if !bytes.Equal(out, raw) {
		result.ReencodeEligible = false
		if hasField(msg, "ExtendedProtCfgOpts") {
			result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASEpcoLossy,
				"ePCO structure parsed but upstream re-encode is lossy; typed projection is decode-only"))
		} else {
			result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASRoundTripMismatch,
				"upstream re-encode is not byte-identical; typed projection is decode-only"))
		}
		return result
	}

	result.ReencodeVerified = true
	result.ReencodeEligible = true
	result.Qualified = true
	return result
}

// EncodeNAS encodes a typed upstream 5GSM message. The returned slice is newly
// allocated. The caller remains responsible for retaining the original bytes;
// this function never writes into an existing buffer.
func EncodeNAS(msg message.GSMMessage) (EncodeResult, error) {
	result := EncodeResult{Protocol: ProtocolNAS, Status: StatusInvalid}
	if msg == nil {
		return result, codecError("CODEC_INVALID_ARGUMENT", "nil NAS message")
	}
	result.TypeName = qualifiedGoType(msg, "nas.message")
	if nonNilField(msg, "ExtendedProtCfgOpts") {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASEpcoLossy,
			"ePCO is present; Free5GC re-encoding is lossy, so no re-encoded NAS bytes are emitted"))
		return result, nil
	}
	if out, err := msg.MarshalBinary(); err != nil {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNASRoundTripMismatch, err.Error()))
		return result, nil
	} else {
		result.Status = StatusSuccess
		result.Binary = append([]byte(nil), out...)
		result.Length = len(result.Binary)
	}
	return result, nil
}

// scanNAS performs the facade's structural IEI walk. It is not a decoder: it
// only checks the message's framing for duplicate, unknown and truncated IEs
// that the upstream decoder either silently skips or reports without a typed
// error.
func scanNAS(raw []byte, meta nasMessageMeta) *nasScanIssue {
	offset := nasHeaderLen
	for _, field := range meta.Mandatory {
		var next int
		var ok bool
		next, ok = skipNASMandatory(raw, offset, field)
		if !ok {
			return &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNASLength,
				fmt.Sprintf("mandatory IE %s is truncated", field.Field))}
		}
		offset = next
	}

	optional := make(map[uint8]nasIEMeta, len(meta.Optional))
	for _, field := range meta.Optional {
		optional[field.IEI] = field
	}
	seen := make(map[uint8]bool)
	for offset < len(raw) {
		start := offset
		ieiByte := raw[offset]
		offset++
		iei := ieiByte
		if ieiByte >= 0x80 {
			iei = ieiByte & 0xf0
		}
		field, ok := optional[iei]
		if !ok {
			return &nasScanIssue{status: StatusUnsupportedIE, diag: diagnosticIEI(DiagNASUnknownIEI,
				"unknown 5GSM IEI; upstream would abort or silently skip", iei, start)}
		}
		if seen[iei] {
			return &nasScanIssue{status: StatusDuplicateIE, diag: diagnosticIEI(DiagNASDuplicateIEI,
				"duplicate 5GSM IEI is not permitted; upstream may silently ignore it", iei, start)}
		}
		seen[iei] = true

		switch field.Framing {
		case "TV":
			// The value is carried in the low bits of the IEI octet.
		case "TLV":
			length, ok := readNASLength(raw, &offset, 1)
			if !ok {
				return &nasScanIssue{status: StatusInvalid, diag: diagnosticIEI(DiagNASLength,
					"truncated TLV length", iei, start)}
			}
			offset += length
		case "TLV-E":
			length, ok := readNASLength(raw, &offset, 2)
			if !ok {
				return &nasScanIssue{status: StatusInvalid, diag: diagnosticIEI(DiagNASLength,
					"truncated TLV-E length", iei, start)}
			}
			offset += length
		default:
			return &nasScanIssue{status: StatusInvalid, diag: diagnosticIEI(DiagNASLength,
				fmt.Sprintf("unsupported structural framing %q in generated metadata", field.Framing), iei, start)}
		}
		if offset > len(raw) {
			return &nasScanIssue{status: StatusInvalid, diag: diagnosticIEI(DiagNASLength,
				"IE length overruns the message", iei, start)}
		}
	}
	if offset != len(raw) {
		return &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNASLength,
			"structural IEI walk did not consume the message exactly")}
	}
	return nil
}

func skipNASMandatory(raw []byte, offset int, field nasIEMeta) (int, bool) {
	switch field.Framing {
	case "V":
		size := field.Size
		if size == 0 {
			size = 1
		}
		if offset+size > len(raw) {
			return offset, false
		}
		return offset + size, true
	case "LV":
		length, ok := readNASLength(raw, &offset, 1)
		if !ok || offset+length > len(raw) {
			return offset, false
		}
		return offset + length, true
	case "LV-E":
		length, ok := readNASLength(raw, &offset, 2)
		if !ok || offset+length > len(raw) {
			return offset, false
		}
		return offset + length, true
	case "TV":
		if offset >= len(raw) {
			return offset, false
		}
		return offset + 1, true
	default:
		return offset, false
	}
}

func readNASLength(raw []byte, offset *int, octets int) (int, bool) {
	if *offset+octets > len(raw) {
		return 0, false
	}
	if octets == 1 {
		length := int(raw[*offset])
		*offset = *offset + 1
		return length, true
	}
	length := int(binary.BigEndian.Uint16(raw[*offset : *offset+2]))
	*offset += 2
	return length, true
}

func hasField(value any, fieldName string) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}
	return rv.FieldByName(fieldName).IsValid()
}

func nonNilField(value any, fieldName string) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return false
	}
	switch field.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		return !field.IsNil()
	default:
		return true
	}
}

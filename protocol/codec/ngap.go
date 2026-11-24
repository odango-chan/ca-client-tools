package codec

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/free5gc/ngap/ie"
	"github.com/free5gc/ngap/message"
)

type ngapMessageMeta struct {
	Name          string
	TypeName      string
	ProcedureCode int64
	MessageType   int64
	Criticality   int64
}

// DecodeNGAPPDU decodes a complete NGAP PDU (the same root the SCTP/N2 APER
// payload uses). The typed projection is produced by the pinned Free5GC
// message package. The facade re-encodes it only to compare bytes; a mismatch
// is reported as NOT_QUALIFIED and the typed projection must not be used to
// replace the original payload.
func DecodeNGAPPDU(input []byte) DecodeResult {
	raw := append([]byte(nil), input...)
	result := DecodeResult{
		Protocol:      ProtocolNGAP,
		Status:        StatusInvalid,
		Raw:           raw,
		RawLength:     len(raw),
		RawPreserved:  true,
		DataPreserved: true,
		TotalBits:     len(raw) * 8,
	}
	result.Diagnostics = append(result.Diagnostics, diagnostic(DiagRawPreserved, "raw NGAP bytes retained unchanged"))
	if len(raw) == 0 {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPDecode, "empty NGAP PDU"))
		return result
	}

	msg, err := message.Parse(append([]byte(nil), raw...))
	if err != nil {
		status, diag := classifyNGAPError(err)
		result.Status = status
		result.Diagnostics = append(result.Diagnostics, diag)
		return result
	}
	if msg == nil {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPDecode, "upstream NGAP parser returned no message"))
		return result
	}
	result.Decoded = msg
	result.TypeName = qualifiedGoType(msg, "ngap.message")
	result.MessageType = shortGoType(msg)
	// The upstream parser exposes no per-IE completeness signal for a full PDU,
	// and the unpatched APER layer silently discards nested extension additions.
	// The facade therefore never claims Complete for a full NGAP PDU.
	result.Complete = false
	result.StructuralParse = true
	result.ProjectionValid = true

	out, encErr := msg.MarshalBinary()
	if encErr != nil {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPEncodeUnsupported,
			"typed NGAP message cannot be re-encoded by the upstream encoder"))
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPExtensionRisk,
			"unpatched Free5GC APER cannot preserve extension additions; the projection is not re-encode eligible"))
		return result
	}
	if !bytes.Equal(out, raw) {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPRoundTripMismatch,
			fmt.Sprintf("NGAP re-encode differs from the input (%d vs %d octets); typed view is projection-only", len(out), len(raw))))
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPExtensionRisk,
			"unpatched Free5GC APER silently ignores extensible SEQUENCE additions; byte identity is the only re-encode evidence"))
		return result
	}

	// Byte identity is evidence about this input only. No full NGAP PDU has an
	// independent qualification record in this phase, so the status is SUCCESS
	// while qualified stays false.
	result.Status = StatusSuccess
	result.ReencodeVerified = true
	result.ReencodeEligible = true
	result.Qualified = ngapPDUQualified(result.MessageType)
	if !result.Qualified {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPNotQualified,
			"no independent NGAP PDU qualification evidence; decode and byte round-trip succeeded, qualification does not"))
	}
	return result
}

// EncodeNGAPPDU encodes a typed upstream NGAP message into a new binary.
func EncodeNGAPPDU(msg message.Message) (EncodeResult, error) {
	result := EncodeResult{Protocol: ProtocolNGAP, Status: StatusInvalid}
	if msg == nil {
		return result, codecError("CODEC_INVALID_ARGUMENT", "nil NGAP message")
	}
	result.TypeName = qualifiedGoType(msg, "ngap.message")
	out, err := msg.MarshalBinary()
	if err != nil {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPEncodeUnsupported, err.Error()))
		return result, nil
	}
	result.Status = StatusSuccess
	result.Binary = append([]byte(nil), out...)
	result.Length = len(result.Binary)
	return result, nil
}

func classifyNGAPError(err error) (Status, Diagnostic) {
	var abstract *ie.AbstractSyntaxErr
	if errors.As(err, &abstract) {
		switch c := abstract.ErrorCase().(type) {
		case *ie.AbstractSyntaxErrNotComprehendedIE:
			if c.ReportIe != nil && (c.ReportIe.IsMsgTypeIe() || c.ReportIe.IsProcedureCodeIe()) {
				return StatusUnsupportedMessage, diagnostic(DiagNGAPUnsupportedMessage,
					"upstream NGAP implementation does not recognize the message type or procedure code")
			}
			return StatusUnsupportedIE, diagnostic(DiagNGAPUnsupportedIE,
				"upstream NGAP implementation does not comprehend an IE in this message")
		case *ie.AbstractSyntaxErrIEWrongOrderOrTooManyOccur:
			return StatusDuplicateIE, diagnostic(DiagNGAPDuplicateIE,
				"an NGAP IE occurred in the wrong order or too many times")
		case *ie.AbstractSyntaxErrMissingIE:
			return StatusInvalid, diagnostic(DiagNGAPDecode,
				"an NGAP mandatory IE is missing")
		case *ie.AbstractSyntaxErrIEErrPresent:
			return StatusInvalid, diagnostic(DiagNGAPDecode,
				"an NGAP conditional IE is erroneously present")
		default:
			return StatusInvalid, diagnostic(DiagNGAPDecode,
				"NGAP abstract syntax error")
		}
	}
	var transfer *ie.TransferSyntaxErr
	if errors.As(err, &transfer) {
		return StatusInvalid, diagnostic(DiagNGAPDecode,
			"NGAP transfer syntax error (malformed or truncated APER)")
	}
	// The standalone transfer container's unknown-IE path uses an untyped
	// pkg/errors value. Use it only as a last-resort classification; the
	// caller still sees the raw bytes and no re-encode eligibility.
	if strings.Contains(err.Error(), "unknown reference field value") {
		return StatusUnsupportedIE, diagnostic(DiagNGAPUnsupportedIE,
			"standalone N2 transfer container has an unknown protocol IE ID")
	}
	return StatusInvalid, diagnostic(DiagNGAPDecode, "NGAP decode failed")
}

func qualifiedGoType(value any, expectedPrefix string) string {
	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return ""
	}
	switch t.PkgPath() {
	case "github.com/free5gc/ngap/message":
		return "ngap.message." + t.Name()
	case "github.com/free5gc/ngap/ie":
		return "ngap.ie." + t.Name()
	case "github.com/free5gc/nas/message":
		return "nas.message." + t.Name()
	case "github.com/free5gc/nas/ie":
		return "nas.ie." + t.Name()
	default:
		return expectedPrefix + "." + t.Name()
	}
}

func shortGoType(value any) string {
	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return ""
	}
	return t.Name()
}

// Package codec provides the CA unified codec facade over the pinned Free5GC
// NAS and NGAP Go modules. It is a protocol-only package: it performs no
// network I/O, owns no business state, and never substitutes a re-encoded
// payload for the raw bytes supplied by a caller.
package codec

import "fmt"

// Status is the declared outcome of a decode or encode request. The values are
// deliberately distinct; callers must not collapse them into one "error" class.
type Status string

const (
	// StatusSuccess means the declared scope was decoded and, for decodes, the
	// exact-input round-trip check passed (ReencodeVerified).
	StatusSuccess Status = "SUCCESS"
	// StatusPartial means the message was recognized but at least one IE was
	// reported through the upstream typed IEToDoList and was not parsed.
	StatusPartial Status = "PARTIAL"
	// StatusUnsupportedMessage means the upstream implementation has no
	// decoder/encoder for the message type, procedure or transfer root.
	StatusUnsupportedMessage Status = "UNSUPPORTED_MESSAGE"
	// StatusUnsupportedIE means a well-formed IE or protocol IE ID is outside
	// the upstream implementation's supported set.
	StatusUnsupportedIE Status = "UNSUPPORTED_IE"
	// StatusDuplicateIE means an IE occurred more often than the message
	// permits. Free5GC may silently skip such an IE, so the facade checks it.
	StatusDuplicateIE Status = "DUPLICATE_IE"
	// StatusInvalid means malformed, truncated, wrong-order or non-decodable
	// input, or an input whose bytes cannot be consumed strictly.
	StatusInvalid Status = "INVALID"
	// StatusNotQualified means the typed projection was produced but the facade
	// cannot prove lossless or complete handling (for example an APER
	// extension-addition risk or a byte round-trip mismatch). The raw bytes are
	// retained and the re-encoded form must not replace them.
	StatusNotQualified Status = "NOT_QUALIFIED"
)

// Protocol identifies the codec root that was used.
type Protocol string

const (
	ProtocolNAS  Protocol = "NAS"
	ProtocolNGAP Protocol = "NGAP"
	ProtocolN2SM Protocol = "N2_SM"
	ProtocolJSON Protocol = "JSON"
	ProtocolNone Protocol = ""
)

// Diagnostic code strings are stable machine-readable identifiers. They are
// additive: callers should ignore unknown codes.
const (
	DiagNASHeaderInvalid       = "NAS_HEADER_INVALID"
	DiagNASUnsupportedMessage  = "NAS_UNSUPPORTED_MESSAGE"
	DiagNASUnknownIEI          = "NAS_UNKNOWN_IEI"
	DiagNASDuplicateIEI        = "NAS_DUPLICATE_IEI"
	DiagNASIEToDo              = "NAS_IE_TODO"
	DiagNASLength              = "NAS_LENGTH_INVALID"
	DiagNASEpcoLossy           = "NAS_EPCO_REENCODE_LOSSY"
	DiagNASRoundTripMismatch   = "NAS_ROUND_TRIP_MISMATCH"
	DiagNGAPUnsupportedMessage = "NGAP_UNSUPPORTED_MESSAGE"
	DiagNGAPUnsupportedIE      = "NGAP_UNSUPPORTED_IE"
	DiagNGAPDuplicateIE        = "NGAP_DUPLICATE_IE"
	DiagNGAPDecode             = "NGAP_DECODE_INVALID"
	DiagNGAPTrailing           = "NGAP_TRAILING_BITS"
	DiagNGAPExtensionRisk      = "NGAP_APER_EXTENSION_UNQUALIFIED"
	DiagNGAPRoundTripMismatch  = "NGAP_ROUND_TRIP_MISMATCH"
	DiagNGAPEncodeUnsupported  = "NGAP_ENCODE_UNSUPPORTED"
	DiagN2RootUnsupported      = "N2_ROOT_UNSUPPORTED"
	DiagN2RootInvalid          = "N2_ROOT_INVALID"
	DiagNASNotQualified        = "NAS_NOT_QUALIFIED"
	DiagN2NotQualified         = "N2_NOT_QUALIFIED"
	DiagNGAPNotQualified       = "NGAP_NOT_QUALIFIED"
	DiagJSONProjection         = "JSON_PROJECTION"
	DiagRawPreserved           = "RAW_BYTES_PRESERVED"
)

// Diagnostic is a structured, non-sensitive explanation attached to a result.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	IEI     *uint8 `json:"iei,omitempty"`
	IEID    *int64 `json:"ieId,omitempty"`
	Offset  *int   `json:"offset,omitempty"`
}

func diagnostic(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message}
}

func diagnosticIEI(code, message string, iei uint8, offset int) Diagnostic {
	return Diagnostic{Code: code, Message: message, IEI: &iei, Offset: &offset}
}

// DecodeResult carries the authoritative raw bytes together with the typed
// projection. Raw is always a private copy; the typed projection is a view and
// never a replacement for Raw.
type DecodeResult struct {
	Protocol         Protocol     `json:"protocol"`
	Status           Status       `json:"status"`
	MessageType      string       `json:"messageType,omitempty"`
	TypeName         string       `json:"typeName,omitempty"`
	Root             string       `json:"root,omitempty"`
	Raw              []byte       `json:"-"`
	RawLength        int          `json:"rawLength"`
	RawPreserved     bool         `json:"rawPreserved"`
	DataPreserved    bool         `json:"dataPreserved"`
	StructuralParse  bool         `json:"structuralParse"`
	ProjectionValid  bool         `json:"projectionValid"`
	Complete         bool         `json:"complete"`
	Qualified        bool         `json:"qualified"`
	ReencodeEligible bool         `json:"reencodeEligible"`
	ReencodeVerified bool         `json:"reencodeVerified"`
	IEToDo           []uint8      `json:"ieToDo,omitempty"`
	ConsumedBits     int          `json:"consumedBits,omitempty"`
	TotalBits        int          `json:"totalBits,omitempty"`
	Diagnostics      []Diagnostic `json:"diagnostics,omitempty"`
	Decoded          any          `json:"-"`
}

// EncodeResult is the result of encoding a typed message. Binary is always a
// newly allocated buffer; it never aliases a caller's input.
type EncodeResult struct {
	Protocol    Protocol     `json:"protocol"`
	Status      Status       `json:"status"`
	TypeName    string       `json:"typeName,omitempty"`
	Root        string       `json:"root,omitempty"`
	Binary      []byte       `json:"-"`
	Length      int          `json:"length"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// CodecError is returned for caller-side or implementation failures that are
// not protocol decode outcomes (for example an unknown N2 root name). Protocol
// outcomes are represented by Result.Status.
type CodecError struct {
	Code    string
	Message string
}

func (e *CodecError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func codecError(code, format string, args ...any) error {
	return &CodecError{Code: code, Message: fmt.Sprintf(format, args...)}
}

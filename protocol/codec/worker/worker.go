// Package worker implements the CA codec long-lived stdin/stdout process
// protocol. It is a transport adapter only: every protocol decision is made by
// protocol/codec. Frames are length-bounded and carry JSON and raw binary in
// separate segments; binary is never Base64 encoded.
package worker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	nasmessage "github.com/free5gc/nas/message"
	ngapmessage "github.com/free5gc/ngap/message"
	"github.com/odango-chan/ca-client-tools/protocol/codec"
)

const (
	// ProtocolVersion is the frame/IPC schema version.
	ProtocolVersion = 1
	// DefaultMaxFrameBytes bounds one request or response, including JSON and
	// binary. Hosts may pass a smaller value to Serve.
	DefaultMaxFrameBytes = 64 << 20
	// MaxJSONBytes bounds only the JSON header/body portion.
	MaxJSONBytes = 4 << 20
)

var (
	ErrFrameTooLarge = errors.New("codec worker frame exceeds the configured bound")
	ErrFrameVersion  = errors.New("codec worker frame version is unsupported")
)

type request struct {
	RequestID  string          `json:"requestId"`
	Operation  string          `json:"operation"`
	Kind       string          `json:"kind,omitempty"`
	Root       string          `json:"root,omitempty"`
	Type       string          `json:"type,omitempty"`
	Message    json.RawMessage `json:"message,omitempty"`
	Options    map[string]any  `json:"options,omitempty"`
	RequestRaw json.RawMessage `json:"-"`
}

type response struct {
	RequestID        string                    `json:"requestId"`
	Operation        string                    `json:"operation"`
	Status           codec.Status              `json:"status"`
	Protocol         codec.Protocol            `json:"protocol,omitempty"`
	MessageType      string                    `json:"messageType,omitempty"`
	TypeName         string                    `json:"typeName,omitempty"`
	Root             string                    `json:"root,omitempty"`
	RawLength        int                       `json:"rawLength"`
	RawPreserved     bool                      `json:"rawPreserved"`
	Complete         bool                      `json:"complete"`
	Qualified        bool                      `json:"qualified"`
	ReencodeEligible bool                      `json:"reencodeEligible"`
	ReencodeVerified bool                      `json:"reencodeVerified"`
	IEToDo           []int                     `json:"ieToDo,omitempty"`
	ConsumedBits     int                       `json:"consumedBits,omitempty"`
	TotalBits        int                       `json:"totalBits,omitempty"`
	Length           int                       `json:"length,omitempty"`
	Message          json.RawMessage           `json:"message,omitempty"`
	Diagnostics      []codec.Diagnostic        `json:"diagnostics,omitempty"`
	Capabilities     *codec.CapabilityManifest `json:"capabilities,omitempty"`
	Error            *responseError            `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Serve runs the long-lived worker until EOF, context cancellation, or an
// unrecoverable framing error. It never opens a socket or writes protocol data
// to stderr.
func Serve(ctx context.Context, in io.Reader, out io.Writer, stderr io.Writer, maxFrame int) error {
	if maxFrame <= 0 {
		maxFrame = DefaultMaxFrameBytes
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		header, payload, err := readFrame(in, maxFrame)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var req request
		if err := json.Unmarshal(header, &req); err != nil {
			resp := response{
				Status: codec.StatusInvalid,
				Error:  &responseError{Code: "REQUEST_JSON_INVALID", Message: "request JSON could not be decoded"},
			}
			if err := writeFrame(out, resp, nil, maxFrame); err != nil {
				return err
			}
			continue
		}
		req.RequestRaw = append([]byte(nil), header...)
		resp, binaryPayload := handle(req, payload)
		if err := writeFrame(out, resp, binaryPayload, maxFrame); err != nil {
			return err
		}
	}
}

func handle(req request, payload []byte) (response, []byte) {
	resp := response{RequestID: req.RequestID, Operation: req.Operation}
	switch req.Operation {
	case "ping":
		resp.Status = codec.StatusSuccess
		return resp, nil
	case "capabilities":
		caps := codec.Capabilities()
		resp.Status = codec.StatusSuccess
		resp.Capabilities = &caps
		return resp, nil
	case "decode":
		return handleDecode(req, payload)
	case "encode":
		return handleEncode(req)
	default:
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "OPERATION_UNSUPPORTED", Message: "operation must be decode, encode, capabilities or ping"}
		return resp, nil
	}
}

func handleDecode(req request, payload []byte) (response, []byte) {
	resp := response{RequestID: req.RequestID, Operation: req.Operation}
	var result codec.DecodeResult
	switch req.Kind {
	case "nas":
		result = codec.DecodeNAS(payload)
	case "ngap":
		result = codec.DecodeNGAPPDU(payload)
	case "n2":
		result = codec.DecodeN2Transfer(req.Root, payload)
	default:
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "KIND_UNSUPPORTED", Message: "kind must be nas, ngap or n2"}
		return resp, nil
	}
	fillDecodeResponse(&resp, result)
	if result.Decoded != nil {
		projected, err := codec.ProjectToJSON(result.Decoded)
		if err != nil {
			resp.Status = codec.StatusNotQualified
			resp.Diagnostics = append(resp.Diagnostics, codec.Diagnostic{
				Code:    codec.DiagJSONProjection,
				Message: "typed value exists but the canonical JSON projection is unsupported",
			})
		} else {
			resp.Message = projected
		}
	}
	return resp, nil
}

func fillDecodeResponse(resp *response, result codec.DecodeResult) {
	resp.Status = result.Status
	resp.Protocol = result.Protocol
	resp.MessageType = result.MessageType
	resp.TypeName = result.TypeName
	resp.Root = result.Root
	resp.RawLength = result.RawLength
	resp.RawPreserved = result.RawPreserved
	resp.Complete = result.Complete
	resp.Qualified = result.Qualified
	resp.ReencodeEligible = result.ReencodeEligible
	resp.ReencodeVerified = result.ReencodeVerified
	resp.IEToDo = make([]int, len(result.IEToDo))
	for i, value := range result.IEToDo {
		resp.IEToDo[i] = int(value)
	}
	resp.ConsumedBits = result.ConsumedBits
	resp.TotalBits = result.TotalBits
	resp.Diagnostics = result.Diagnostics
}

func handleEncode(req request) (response, []byte) {
	resp := response{RequestID: req.RequestID, Operation: req.Operation}
	if len(req.Message) == 0 {
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "MESSAGE_REQUIRED", Message: "encode requires a canonical message projection"}
		return resp, nil
	}
	typeName := req.Type
	if req.Kind == "n2" {
		if req.Root == "" {
			resp.Status = codec.StatusInvalid
			resp.Error = &responseError{Code: "ROOT_REQUIRED", Message: "N2 encode requires an explicit root"}
			return resp, nil
		}
		if meta, ok := codec.N2RootTypeName(req.Root); ok {
			typeName = meta
		}
	}
	if typeName == "" {
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "TYPE_REQUIRED", Message: "encode requires a Free5GC type name"}
		return resp, nil
	}
	if !codec.JSONEncodeSupported(typeName) {
		resp.Status = codec.StatusNotQualified
		resp.TypeName = typeName
		resp.Error = &responseError{Code: "JSON_ENCODE_NOT_QUALIFIED",
			Message: "this type is decode-projection-only; no canonical JSON encode round-trip test exists"}
		return resp, nil
	}
	typed, err := codec.ProjectFromJSON(typeName, req.Message)
	if err != nil {
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "MESSAGE_PROJECTION_INVALID", Message: err.Error()}
		return resp, nil
	}
	var encoded codec.EncodeResult
	switch req.Kind {
	case "nas":
		msg, ok := typed.(nasmessage.GSMMessage)
		if !ok {
			resp.Status = codec.StatusInvalid
			resp.Error = &responseError{Code: "TYPE_MISMATCH", Message: "type is not a 5GSM message"}
			return resp, nil
		}
		encoded, err = codec.EncodeNAS(msg)
	case "ngap":
		msg, ok := typed.(ngapmessage.Message)
		if !ok {
			resp.Status = codec.StatusInvalid
			resp.Error = &responseError{Code: "TYPE_MISMATCH", Message: "type is not an NGAP PDU message"}
			return resp, nil
		}
		encoded, err = codec.EncodeNGAPPDU(msg)
	case "n2":
		encoded, err = codec.EncodeN2Transfer(req.Root, typed)
	default:
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "KIND_UNSUPPORTED", Message: "kind must be nas, ngap or n2"}
		return resp, nil
	}
	if err != nil {
		resp.Status = codec.StatusInvalid
		resp.Error = &responseError{Code: "ENCODE_FAILED", Message: err.Error()}
		return resp, nil
	}
	resp.Status = encoded.Status
	resp.Protocol = encoded.Protocol
	resp.TypeName = encoded.TypeName
	resp.Root = encoded.Root
	resp.Length = encoded.Length
	resp.Diagnostics = encoded.Diagnostics
	if encoded.Binary == nil {
		return resp, nil
	}
	return resp, encoded.Binary
}

func readFrame(r io.Reader, maxFrame int) ([]byte, []byte, error) {
	var lengthBytes [4]byte
	if _, err := io.ReadFull(r, lengthBytes[:]); err != nil {
		return nil, nil, err
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length < 5 || int(length) > maxFrame {
		return nil, nil, ErrFrameTooLarge
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, nil, err
	}
	if body[0] != ProtocolVersion {
		return nil, nil, ErrFrameVersion
	}
	jsonLen := binary.BigEndian.Uint32(body[1:5])
	if int(jsonLen) > len(body)-5 || int(jsonLen) > MaxJSONBytes {
		return nil, nil, fmt.Errorf("codec worker JSON length %d is invalid", jsonLen)
	}
	header := append([]byte(nil), body[5:5+jsonLen]...)
	payload := append([]byte(nil), body[5+jsonLen:]...)
	return header, payload, nil
}

func writeFrame(w io.Writer, resp response, payload []byte, maxFrame int) error {
	header, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if len(header) > MaxJSONBytes {
		return fmt.Errorf("codec worker response JSON exceeds %d bytes", MaxJSONBytes)
	}
	total := 1 + 4 + len(header) + len(payload)
	if total > maxFrame {
		return ErrFrameTooLarge
	}
	frame := make([]byte, 4+total)
	binary.BigEndian.PutUint32(frame[0:4], uint32(total))
	frame[4] = ProtocolVersion
	binary.BigEndian.PutUint32(frame[5:9], uint32(len(header)))
	copy(frame[9:], header)
	copy(frame[9+len(header):], payload)
	_, err = w.Write(frame)
	return err
}

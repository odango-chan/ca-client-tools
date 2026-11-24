package worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"github.com/odango-chan/ca-client-tools/protocol/codec"
)

func requestFrame(t *testing.T, req map[string]any, binaryPayload []byte) []byte {
	t.Helper()
	header, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	total := 1 + 4 + len(header) + len(binaryPayload)
	frame := make([]byte, 4+total)
	binary.BigEndian.PutUint32(frame[0:4], uint32(total))
	frame[4] = ProtocolVersion
	binary.BigEndian.PutUint32(frame[5:9], uint32(len(header)))
	copy(frame[9:], header)
	copy(frame[9+len(header):], binaryPayload)
	return frame
}

func serveFrames(t *testing.T, frames []byte) []response {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(context.Background(), bytes.NewReader(frames), &out, io.Discard, DefaultMaxFrameBytes); err != nil {
		t.Fatal(err)
	}
	var responses []response
	for out.Len() > 0 {
		header, _, err := readFrame(&out, DefaultMaxFrameBytes)
		if err != nil {
			t.Fatal(err)
		}
		var resp response
		if err := json.Unmarshal(header, &resp); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestWorkerMatchesGoFacade(t *testing.T) {
	raw, err := codecDecodeHex("2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001")
	if err != nil {
		t.Fatal(err)
	}
	native := codec.DecodeNAS(raw)
	nativeJSON, err := codec.ProjectToJSON(native.Decoded)
	if err != nil {
		t.Fatal(err)
	}
	frames := requestFrame(t, map[string]any{"requestId": "same", "operation": "decode", "kind": "nas"}, raw)
	var out bytes.Buffer
	if err := Serve(context.Background(), bytes.NewReader(frames), &out, io.Discard, DefaultMaxFrameBytes); err != nil {
		t.Fatal(err)
	}
	header, _, err := readFrame(&out, DefaultMaxFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	var resp response
	if err := json.Unmarshal(header, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != native.Status || resp.MessageType != native.MessageType || resp.Qualified != native.Qualified {
		t.Fatalf("worker/native mismatch: worker=%+v native=%+v", resp, native)
	}
	if !bytes.Equal(resp.Message, nativeJSON) {
		t.Fatalf("projection mismatch:\nworker=%s\nnative=%s", resp.Message, nativeJSON)
	}
}

func TestWorkerEndToEnd(t *testing.T) {
	nasHex := "2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001"
	raw, err := codecDecodeHex(nasHex)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := codec.ProjectToJSON(mustDecodeNAS(t, raw).Decoded)
	if err != nil {
		t.Fatal(err)
	}
	var message any
	if err := json.Unmarshal(projected, &message); err != nil {
		t.Fatal(err)
	}

	var frames []byte
	frames = append(frames, requestFrame(t, map[string]any{"requestId": "1", "operation": "ping"}, nil)...)
	frames = append(frames, requestFrame(t, map[string]any{"requestId": "2", "operation": "capabilities"}, nil)...)
	frames = append(frames, requestFrame(t, map[string]any{
		"requestId": "3", "operation": "decode", "kind": "nas",
	}, raw)...)
	frames = append(frames, requestFrame(t, map[string]any{
		"requestId": "4", "operation": "encode", "kind": "nas",
		"type": "nas.message.PDUSessEstAccept", "message": message,
	}, nil)...)
	frames = append(frames, requestFrame(t, map[string]any{
		"requestId": "5", "operation": "encode", "kind": "nas",
		"type": "nas.message.PDUSessModComplete", "message": message,
	}, nil)...)

	responses := serveFrames(t, frames)
	if len(responses) != 5 {
		t.Fatalf("responses=%d", len(responses))
	}
	if responses[0].Status != codec.StatusSuccess || responses[1].Capabilities == nil {
		t.Fatalf("ping/capabilities: %+v %+v", responses[0], responses[1])
	}
	if responses[1].Capabilities.SchemaVersion != "ca-codec-capabilities/v1" {
		t.Fatalf("capability schema: %+v", responses[1].Capabilities)
	}
	if responses[2].Status != codec.StatusSuccess || len(responses[2].Message) == 0 {
		t.Fatalf("decode: %+v", responses[2])
	}
	if responses[3].Status != codec.StatusSuccess || responses[3].Length != len(raw) {
		t.Fatalf("encode: %+v", responses[3])
	}
	if responses[4].Status != codec.StatusNotQualified || responses[4].Error == nil {
		t.Fatalf("unqualified JSON encode was not gated: %+v", responses[4])
	}
}

func TestWorkerN2FramesCarryRawBinary(t *testing.T) {
	root := "PDUSessionResourceSetupRequestTransfer"
	raw, err := codecDecodeHex("000003008b000a01f00a0a0001000007d100860001000088000700010000091c00")
	if err != nil {
		t.Fatal(err)
	}
	frames := requestFrame(t, map[string]any{
		"requestId": "n2-1", "operation": "decode", "kind": "n2", "root": root,
	}, raw)
	var out bytes.Buffer
	if err := Serve(context.Background(), bytes.NewReader(frames), &out, io.Discard, DefaultMaxFrameBytes); err != nil {
		t.Fatal(err)
	}
	header, payload, err := readFrame(&out, DefaultMaxFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 0 {
		t.Fatal("decode response unexpectedly carried binary")
	}
	var resp response
	if err := json.Unmarshal(header, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != codec.StatusSuccess || resp.Root != root || !resp.ReencodeEligible {
		t.Fatalf("N2 decode: %+v", resp)
	}
}

func TestWorkerRejectsOversizedFrame(t *testing.T) {
	var frame [4]byte
	binary.BigEndian.PutUint32(frame[:], uint32(DefaultMaxFrameBytes+1))
	err := Serve(context.Background(), bytes.NewReader(frame[:]), io.Discard, io.Discard, DefaultMaxFrameBytes)
	if err != ErrFrameTooLarge {
		t.Fatalf("error=%v want ErrFrameTooLarge", err)
	}
}

func mustDecodeNAS(t *testing.T, raw []byte) codec.DecodeResult {
	t.Helper()
	result := codec.DecodeNAS(raw)
	if result.Status != codec.StatusSuccess {
		t.Fatalf("decode: %+v", result)
	}
	return result
}

func codecDecodeHex(value string) ([]byte, error) {
	out := make([]byte, len(value)/2)
	for i := range out {
		high, err := nibble(value[i*2])
		if err != nil {
			return nil, err
		}
		low, err := nibble(value[i*2+1])
		if err != nil {
			return nil, err
		}
		out[i] = high<<4 | low
	}
	return out, nil
}

func nibble(value byte) (byte, error) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', nil
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, nil
	}
	return 0, io.ErrUnexpectedEOF
}

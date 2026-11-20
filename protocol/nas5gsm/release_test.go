package nas5gsm

import (
	"bytes"
	"errors"
	"testing"
)

func TestReleaseMessageTypesMatchPinnedOpenAMSM(t *testing.T) {
	// lib/nas/5gs/message.h on env-openamsm@594dec78.
	for _, tc := range []struct {
		got  uint8
		want int
	}{
		{MessageTypePDUSessionReleaseRequest, 209},
		{MessageTypePDUSessionReleaseCommand, 211},
		{MessageTypePDUSessionReleaseComplete, 212},
	} {
		if int(tc.got) != tc.want {
			t.Fatalf("message type %d, want %d", tc.got, tc.want)
		}
	}
}

func TestEncodeReleaseRequest(t *testing.T) {
	got, err := EncodeReleaseRequest(ReleaseRequest{PduSessionID: 6, PTI: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "2e0601d1")
	if !bytes.Equal(got, want) {
		t.Fatalf("request = %x, want %x", got, want)
	}
}

func TestEncodeReleaseComplete(t *testing.T) {
	got, err := EncodeReleaseComplete(ReleaseComplete{PduSessionID: 6, PTI: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "2e0601d4")
	if !bytes.Equal(got, want) {
		t.Fatalf("complete = %x, want %x", got, want)
	}
}

func TestDecodeReleaseCommand(t *testing.T) {
	// Cause IE (0x58) follows the header in the real SMF command; the decoder
	// must read the header and ignore the IE bytes.
	raw := mustHex(t, "2e0601d3580100")
	got, err := DecodeReleaseCommand(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.PduSessionID != 6 || got.PTI != 1 {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestReleaseHeaderValidation(t *testing.T) {
	cases := []struct {
		name string
		req  ReleaseRequest
	}{
		{"psi zero", ReleaseRequest{PduSessionID: 0, PTI: 1}},
		{"psi too high", ReleaseRequest{PduSessionID: 16, PTI: 1}},
		{"pti zero", ReleaseRequest{PduSessionID: 1, PTI: 0}},
		{"pti too high", ReleaseRequest{PduSessionID: 1, PTI: 255}},
	}
	for _, tc := range cases {
		if _, err := EncodeReleaseRequest(tc.req); !errors.Is(err, ErrInvalidField) {
			t.Fatalf("%s: error = %v, want ErrInvalidField", tc.name, err)
		}
	}
}

func TestDecodeReleaseCommandRejects(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		want error
	}{
		{"short", "2e0601", ErrMalformed},
		{"bad epd", "2d0601d3", ErrMalformed},
		{"wrong type", "2e0601d4", ErrUnsupported},
		{"psi zero", "2e0001d3", ErrInvalidField},
		{"psi too high", "2e1001d3", ErrInvalidField},
		{"pti too high", "2e06ffd3", ErrInvalidField},
	}
	for _, tc := range cases {
		if _, err := DecodeReleaseCommand(mustHex(t, tc.hex)); !errors.Is(err, tc.want) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

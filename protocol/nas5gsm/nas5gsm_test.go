package nas5gsm

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad test hex %q: %v", s, err)
	}
	return b
}

func TestEncodeEstablishmentRequestGoldenVectors(t *testing.T) {
	epco := []byte{0x00, 0x01, 0x04, 0x01, 0x02, 0x03, 0x04, 0x00, 0x0d, 0x03, 0x08, 0x08, 0x08}
	cases := []struct {
		name string
		req  EstablishmentRequest
		want string
	}{
		{
			name: "ipv4 ssc1",
			req:  EstablishmentRequest{PduSessionID: 1, PTI: 1, PduSessionType: PduSessionTypeIPv4, SSCMode: SSCMode1},
			want: "2e0101c1ffff91a1",
		},
		{
			name: "ipv6 ssc2 high psi and pti",
			req:  EstablishmentRequest{PduSessionID: 15, PTI: 254, PduSessionType: PduSessionTypeIPv6, SSCMode: SSCMode2},
			want: "2e0ffec1ffff92a2",
		},
		{
			name: "ipv4v6 ssc3 epco",
			req:  EstablishmentRequest{PduSessionID: 2, PTI: 3, PduSessionType: PduSessionTypeIPv4v6, SSCMode: SSCMode3, EPCO: epco},
			want: "2e0203c1ffff93a37b000d0001040102030400" + "0d03080808",
		},
		{
			name: "ethernet ssc absent epco",
			req:  EstablishmentRequest{PduSessionID: 9, PTI: 7, PduSessionType: PduSessionTypeEthernet, EPCO: epco},
			want: "2e0907c1ffff957b000d0001040102030400" + "0d03080808",
		},
		{
			name: "type and ssc absent",
			req:  EstablishmentRequest{PduSessionID: 3, PTI: 2},
			want: "2e0302c1ffff",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EncodeEstablishmentRequest(tc.req)
			if err != nil {
				t.Fatalf("EncodeEstablishmentRequest: %v", err)
			}
			want := mustHex(t, tc.want)
			if !bytes.Equal(got, want) {
				t.Fatalf("encoded bytes = %x, want %x", got, want)
			}

			decoded, err := DecodeEstablishmentRequest(got)
			if err != nil {
				t.Fatalf("DecodeEstablishmentRequest: %v", err)
			}
			if decoded.PduSessionID != tc.req.PduSessionID ||
				decoded.PTI != tc.req.PTI ||
				decoded.PduSessionType != tc.req.PduSessionType ||
				decoded.SSCMode != tc.req.SSCMode ||
				!bytes.Equal(decoded.EPCO, tc.req.EPCO) {
				t.Fatalf("decode round-trip mismatch: got %+v want %+v", decoded, tc.req)
			}

			roundTripped, err := EncodeEstablishmentRequest(decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if !bytes.Equal(roundTripped, want) {
				t.Fatalf("re-encoded bytes = %x, want %x", roundTripped, want)
			}
		})
	}
}

func TestDecodeRejectsMalformedAndUnsupported(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		want error
	}{
		{"empty", "", ErrMalformed},
		{"header only", "2e0101c1", ErrMalformed},
		{"missing dl rate", "2e0101c1ff", ErrMalformed},
		{"wrong epd", "7e0101c1ffff", ErrMalformed},
		{"wrong message type", "2e0101c2ffff", ErrUnsupported},
		{"psi zero", "2e0001c1ffff", ErrInvalidField},
		{"pti zero", "2e0100c1ffff", ErrInvalidField},
		{"pti 255", "2e01ffc1ffff", ErrInvalidField},
		{"invalid pdu session type", "2e0101c1ffff97a1", ErrInvalidField},
		{"invalid ssc", "2e0101c1ffff91a4", ErrInvalidField},
		{"duplicate pdu session type", "2e0101c1ffff9192", ErrMalformed},
		{"duplicate ssc", "2e0101c1ffff91a1a2", ErrMalformed},
		{"unknown optional iei", "2e0101c1ffff91a1c3", ErrUnsupported},
		{"truncated epco header", "2e0101c1ffff7b00", ErrMalformed},
		{"epco length overflow", "2e0101c1ffff7b000900010401", ErrMalformed},
		{"empty epco", "2e0101c1ffff7b0000", ErrMalformed},
		{"epco option truncated", "2e0101c1ffff7b00030001", ErrMalformed},
		{"duplicate epco", "2e0101c1ffff7b00030001017b0003000101", ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeEstablishmentRequest(mustHex(t, tc.hex))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

func TestEncodeRejectsInvalidFields(t *testing.T) {
	cases := []EstablishmentRequest{
		{PduSessionID: 0, PTI: 1},
		{PduSessionID: 16, PTI: 1},
		{PduSessionID: 1, PTI: 0},
		{PduSessionID: 1, PTI: 255},
		{PduSessionID: 1, PTI: 1, PduSessionType: 6},
		{PduSessionID: 1, PTI: 1, SSCMode: 4},
		{PduSessionID: 1, PTI: 1, EPCO: []byte{0x00}},
	}
	for _, req := range cases {
		if _, err := EncodeEstablishmentRequest(req); err == nil {
			t.Fatalf("expected error for %+v, got nil", req)
		}
	}
}

func TestDecodeCopiesEPCO(t *testing.T) {
	msg := mustHex(t, "2e0101c1ffff91a17b0003000100")
	req, err := DecodeEstablishmentRequest(msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	msg[len(msg)-1] = 0xff
	if req.EPCO[2] != 0x00 {
		t.Fatalf("decoded EPCO aliases input: %x", req.EPCO)
	}
}

func TestValidateEPCO(t *testing.T) {
	valid := [][]byte{
		nil,
		{0x00, 0x01, 0x00},
		{0x00, 0x01, 0x01, 0xff},
		{0xff, 0x00, 0x01, 0x01},
	}
	for _, b := range valid {
		if err := ValidateEPCO(b); err != nil {
			t.Fatalf("ValidateEPCO(%x) = %v", b, err)
		}
	}
	invalid := [][]byte{
		{0x00},
		{0x00, 0x01},
		{0x00, 0x01, 0x02, 0xff},
	}
	for _, b := range invalid {
		if err := ValidateEPCO(b); err == nil {
			t.Fatalf("ValidateEPCO(%x) expected error", b)
		}
	}
}

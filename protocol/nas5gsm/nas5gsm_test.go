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

// epcoContent is a legal TS 24.501 ePCO container: configuration protocol 0x00
// followed by three options (protocol id 0x0104 length 1, protocol id 0x0304
// length 0, protocol id 0x0d03 length 3).
const epcoContent = "00010401020304000d0303080808"

// auditVectorEPCO is the ePCO content of the 2026-11-24 audit vector. It is
// malformed: the option at offset 8 declares length 8 with only 2 octets left.
// It is only legal if the walk starts at offset 0, which is the bug this test
// file pins. Free5GC rejects the same bytes.
const auditVectorEPCO = "00010401020304000d03080808"

func TestEncodeEstablishmentRequestGoldenVectors(t *testing.T) {
	epco := mustHex(t, epcoContent)
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
			want: "2e0203c1ffff93a37b000e" + epcoContent,
		},
		{
			name: "ethernet ssc absent epco",
			req:  EstablishmentRequest{PduSessionID: 9, PTI: 7, PduSessionType: PduSessionTypeEthernet, EPCO: epco},
			want: "2e0907c1ffff957b000e" + epcoContent,
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
		// 2026-11-24 audit vector: the ePCO declares an option length that
		// overruns the container. The pre-fix ValidateEPCO walked from offset 0
		// and accepted it; the first octet is the configuration protocol.
		{"audit epco vector pst ipv4v6", "2e0203c1ffff93a37b000d" + auditVectorEPCO, ErrMalformed},
		{"audit epco vector pst ipv6", "2e0203c1ffff92a37b000d" + auditVectorEPCO, ErrMalformed},
		{"audit epco vector only", "2e0101c1ffff7b000d" + auditVectorEPCO, ErrMalformed},
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
		{PduSessionID: 1, PTI: 1, EPCO: []byte{0x00, 0x01, 0x0a, 0x05, 0x01}},
	}
	for _, req := range cases {
		if _, err := EncodeEstablishmentRequest(req); err == nil {
			t.Fatalf("expected error for %+v, got nil", req)
		}
	}
}

func TestDecodeCopiesEPCO(t *testing.T) {
	// Configuration protocol 0x00, then protocol id 0x000a with length 1 and
	// value 0xaa.
	msg := mustHex(t, "2e0101c1ffff91a17b000500000a01aa")
	req, err := DecodeEstablishmentRequest(msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(req.EPCO) != 5 {
		t.Fatalf("decoded EPCO = %x, want 5 octets", req.EPCO)
	}
	msg[len(msg)-1] = 0xff
	if req.EPCO[4] != 0xaa {
		t.Fatalf("decoded EPCO aliases input: %x", req.EPCO)
	}
}

func TestValidateEPCO(t *testing.T) {
	valid := []struct {
		name string
		b    []byte
	}{
		{"absent", nil},
		{"empty slice", []byte{}},
		{"configuration protocol only", []byte{0x00}},
		{"unknown configuration protocol only", []byte{0xff}},
		{"one zero length option", []byte{0x00, 0x01, 0x0a, 0x00}},
		{"one option with value", []byte{0x00, 0x01, 0x0a, 0x01, 0xff}},
		{"audit-shaped but well formed", mustHex(t, epcoContent)},
		{"unknown configuration protocol with option", []byte{0xff, 0x00, 0x01, 0x01, 0xaa}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEPCO(tc.b); err != nil {
				t.Fatalf("ValidateEPCO(%x) = %v, want nil", tc.b, err)
			}
		})
	}

	invalid := []struct {
		name string
		b    []byte
	}{
		// The pre-fix walk started at offset 0, so a bare 2-octet option header
		// used to look like a complete option. It is a truncated option header
		// after the configuration protocol octet.
		{"truncated header after config protocol", []byte{0x00, 0x01}},
		{"truncated header, two octets left", []byte{0x00, 0x01, 0x0a}},
		{"option length exceeds remaining", []byte{0x00, 0x01, 0x0a, 0x05, 0x01}},
		{"option length exceeds remaining by one", []byte{0x00, 0x01, 0x0a, 0x04, 0x01, 0x02, 0x03}},
		{"audit vector epco", mustHex(t, auditVectorEPCO)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEPCO(tc.b)
			if err == nil {
				t.Fatalf("ValidateEPCO(%x) = nil, want error", tc.b)
			}
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("ValidateEPCO(%x) = %v, want errors.Is ErrMalformed", tc.b, err)
			}
		})
	}
}

// TestValidateEPCORejectsAuditVector pins the exact 2026-11-24 audit vector:
// the pre-fix walk from offset 0 accepted it, Free5GC rejects it.
func TestValidateEPCORejectsAuditVector(t *testing.T) {
	const msg = "2e0203c1ffff93a37b000d" + auditVectorEPCO
	if err := ValidateEPCO(mustHex(t, auditVectorEPCO)); err == nil {
		t.Fatal("ValidateEPCO accepted the audit ePCO vector")
	}
	if _, err := EncodeEstablishmentRequest(EstablishmentRequest{
		PduSessionID:   2,
		PTI:            3,
		PduSessionType: PduSessionTypeIPv4v6,
		SSCMode:        SSCMode3,
		EPCO:           mustHex(t, auditVectorEPCO),
	}); err == nil {
		t.Fatal("EncodeEstablishmentRequest accepted the audit ePCO vector")
	}
	if _, err := DecodeEstablishmentRequest(mustHex(t, msg)); err == nil {
		t.Fatal("DecodeEstablishmentRequest accepted the audit ePCO vector")
	}
}

// TestEPCOIsPreservedVerbatim checks that validation is read-only and that the
// raw ePCO bytes are carried through encode/decode without rewriting.
func TestEPCOIsPreservedVerbatim(t *testing.T) {
	epco := mustHex(t, epcoContent)
	original := append([]byte(nil), epco...)

	encoded, err := EncodeEstablishmentRequest(EstablishmentRequest{
		PduSessionID:   2,
		PTI:            3,
		PduSessionType: PduSessionTypeIPv4v6,
		SSCMode:        SSCMode3,
		EPCO:           epco,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(epco, original) {
		t.Fatalf("EncodeEstablishmentRequest mutated the caller's ePCO: %x", epco)
	}
	if !bytes.HasSuffix(encoded, epco) {
		t.Fatalf("encoded message does not carry the ePCO verbatim: %x", encoded)
	}

	decoded, err := DecodeEstablishmentRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(decoded.EPCO, original) {
		t.Fatalf("decoded ePCO = %x, want %x", decoded.EPCO, original)
	}
	reencoded, err := EncodeEstablishmentRequest(decoded)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(reencoded, encoded) {
		t.Fatalf("re-encoded message = %x, want %x", reencoded, encoded)
	}
}

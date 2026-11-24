package codec

import (
	"bytes"
	"encoding/hex"
	"testing"

	nasmessage "github.com/free5gc/nas/message"
	"github.com/free5gc/ngap/aper"
	"github.com/free5gc/ngap/ie"
	"github.com/free5gc/ngap/message"
)

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDecodeNASOpenAMSMVectors(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		typ  string
	}{
		{"establishment-accept", "2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001", "PDUSessEstAccept"},
		{"modification-command", "2e0101cb7a000901000631310101ff01790006012041010109", "PDUSessModCmd"},
		{"release-command", "2e0101d324", "PDUSessRelCmd"},
		{"establishment-reject", "2e0101c324", "PDUSessEstRej"},
		{"establishment-request", "2e0101c1ffff91a1", "PDUSessEstReq"},
		{"release-request", "2e0501d1", "PDUSessRelReq"},
		{"release-complete", "2e0507d4", "PDUSessRelComplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustHex(t, tc.hex)
			original := append([]byte(nil), raw...)
			result := DecodeNAS(raw)
			if result.Status != StatusSuccess {
				t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
			}
			if result.MessageType != tc.typ {
				t.Fatalf("message type=%s want %s", result.MessageType, tc.typ)
			}
			if !result.Complete || !result.Qualified || !result.ReencodeVerified || !result.ReencodeEligible {
				t.Fatalf("unexpected qualification flags: %+v", result)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("decoder mutated the input buffer")
			}
			if !bytes.Equal(result.Raw, original) {
				t.Fatal("result raw is not the original byte sequence")
			}
			if out, err := result.Decoded.(interface{ MarshalBinary() ([]byte, error) }).MarshalBinary(); err != nil || !bytes.Equal(out, original) {
				t.Fatalf("re-encode mismatch: %x err=%v", out, err)
			}
			raw[0] = 0
			if result.Raw[0] == 0 {
				t.Fatal("result raw aliases the caller input")
			}
		})
	}
}

func TestDecodeNASPartialUsesTypedIEToDo(t *testing.T) {
	result := DecodeNAS(mustHex(t, "2e0101c1ffff91a16603112233"))
	if result.Status != StatusPartial {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
	if result.Complete || result.Qualified || result.ReencodeEligible {
		t.Fatalf("partial decode must not be complete/re-encode eligible: %+v", result)
	}
	if len(result.IEToDo) != 1 || result.IEToDo[0] != 0x66 {
		t.Fatalf("IEToDo=%v want [0x66]", result.IEToDo)
	}
}

func TestDecodeNASErrorClasses(t *testing.T) {
	cases := []struct {
		name   string
		hex    string
		status Status
		code   string
	}{
		{"unsupported-message", "2e0101ee", StatusUnsupportedMessage, DiagNASUnsupportedMessage},
		{"wrong-epd", "7e0101c1ffff", StatusUnsupportedMessage, DiagNASHeaderInvalid},
		{"unknown-iei", "2e0101c1ffff91a17603112233", StatusUnsupportedIE, DiagNASUnknownIEI},
		{"duplicate-pdu-type", "2e0101c1ffff91a191", StatusDuplicateIE, DiagNASDuplicateIEI},
		{"truncated", "2e0101c1", StatusInvalid, DiagNASLength},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := DecodeNAS(mustHex(t, tc.hex))
			if result.Status != tc.status {
				t.Fatalf("status=%s want=%s diagnostics=%+v", result.Status, tc.status, result.Diagnostics)
			}
			found := false
			for _, diag := range result.Diagnostics {
				if diag.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("diagnostic %s missing: %+v", tc.code, result.Diagnostics)
			}
		})
	}
}

func TestDecodeNASEPCOIsDecodeProjectionOnly(t *testing.T) {
	raw := mustHex(t, "2e0203c1ffff93a37b000e00010401020304000d0303080808")
	result := DecodeNAS(raw)
	if result.Status != StatusSuccess {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
	if result.ReencodeEligible || result.ReencodeVerified || result.Qualified {
		t.Fatalf("lossy ePCO must not be re-encode eligible: %+v", result)
	}
	if !result.StructuralParse || !result.ProjectionValid || !result.DataPreserved || !result.RawPreserved {
		t.Fatalf("ePCO structural/projection/data flags are wrong: %+v", result)
	}
	found := false
	for _, diag := range result.Diagnostics {
		if diag.Code == DiagNASEpcoLossy {
			found = true
		}
	}
	if !found {
		t.Fatalf("ePCO diagnostic missing: %+v", result.Diagnostics)
	}
}

func TestN2OpenAMSMRoundTrips(t *testing.T) {
	cases := []struct {
		root string
		hex  string
	}{
		{"PDUSessionResourceSetupRequestTransfer", "000003008b000a01f00a0a0001000007d100860001000088000700010000091c00"},
		{"PDUSessionResourceModifyRequestTransfer", "000001008700170100a000011c0040030d4020030d40200186a0200186a0"},
		{"PDUSessionResourceSetupResponseTransfer", "0003e00a0a0008000003e90001"},
		{"PDUSessionResourceSetupResponseTransfer", "0003e0c0a80a010102030404010080"},
		{"PDUSessionResourceSetupResponseTransfer", "000fe020010db8000000000000000000000001000003eb0001"},
		{"PDUSessionResourceSetupResponseTransfer", "0003e00a141e28123456780801014090"},
		{"PDUSessionResourceSetupUnsuccessfulTransfer", "0800"},
		{"PDUSessionResourceModifyResponseTransfer", "100004"},
		{"PDUSessionResourceReleaseCommandTransfer", "10"},
		{"PDUSessionResourceReleaseResponseTransfer", "00"},
		{"PDUSessionResourceSetupResponseTransfer", "000fe020010db80000000000000000000000021020304004070fc0"},
	}
	for _, tc := range cases {
		t.Run(tc.root+"/"+tc.hex, func(t *testing.T) {
			raw := mustHex(t, tc.hex)
			original := append([]byte(nil), raw...)
			result := DecodeN2Transfer(tc.root, raw)
			if result.Status != StatusSuccess {
				t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
			}
			if !result.Complete || !result.Qualified || !result.ReencodeEligible || !result.ReencodeVerified {
				t.Fatalf("unexpected flags: %+v", result)
			}
			if !bytes.Equal(raw, original) || !bytes.Equal(result.Raw, original) {
				t.Fatal("N2 decoder did not preserve the original input")
			}
			enc, err := EncodeN2Transfer(tc.root, result.Decoded)
			if err != nil || enc.Status != StatusSuccess || !bytes.Equal(enc.Binary, original) {
				t.Fatalf("re-encode: %+v err=%v", enc, err)
			}
		})
	}
}

func TestN2ExtensionAndTrailingAreNotQualified(t *testing.T) {
	base := "00000400820008080f4240200f4240008b000a01f0ac101f3d263f259100860001000088000700010000091d00"
	cases := []struct {
		name   string
		hex    string
		status Status
	}{
		{"ext1-empty", "80" + base[2:] + "00", StatusNotQualified},
		{"ext1-one-addition", "80" + base[2:] + "0202aabb", StatusNotQualified},
		{"ext1-truncated", "80" + base[2:], StatusNotQualified},
		{"ext0-trailing", base + "ffff", StatusInvalid},
		{"release-ext1", "9000", StatusNotQualified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := DecodeN2Transfer("PDUSessionResourceSetupRequestTransfer", mustHex(t, tc.hex))
			if tc.name == "release-ext1" {
				result = DecodeN2Transfer("PDUSessionResourceReleaseCommandTransfer", mustHex(t, tc.hex))
			}
			if result.Status != tc.status {
				t.Fatalf("status=%s want=%s diagnostics=%+v", result.Status, tc.status, result.Diagnostics)
			}
			if result.ReencodeEligible || result.Qualified {
				t.Fatalf("unsafe decode marked qualified: %+v", result)
			}
		})
	}
}

func TestN2UnknownRoot(t *testing.T) {
	result := DecodeN2Transfer("NoSuchTransfer", []byte{0})
	if result.Status != StatusUnsupportedMessage {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestN2UnknownProtocolIEIsClassified(t *testing.T) {
	// Root: extensible sequence with an empty preamble, one ProtocolIE.
	// IE ID 999, criticality reject, open type of zero length.
	raw := mustHex(t, "0000030003e700000a01f00a0a0001000007d1")
	result := DecodeN2Transfer("PDUSessionResourceSetupRequestTransfer", raw)
	if result.Status != StatusUnsupportedIE {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
}

func TestNGAPPDURoundTrip(t *testing.T) {
	msg := &message.NGReset{
		Cause: &ie.Cause{Choice: &ie.CauseNas{Value: ie.CauseNasPresentNormalRelease}},
		ResetType: &ie.ResetType{
			Choice: &ie.ResetAll{},
		},
	}
	raw, err := msg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	result := DecodeNGAPPDU(raw)
	if result.Status != StatusSuccess {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
	// Byte-identical re-encode is not qualification: no NGAP PDU has independent
	// protocol evidence in this phase.
	if result.Qualified || result.Complete || !result.ReencodeVerified || result.MessageType != "NGReset" {
		t.Fatalf("unexpected result: %+v", result)
	}
	found := false
	for _, diag := range result.Diagnostics {
		if diag.Code == DiagNGAPNotQualified {
			found = true
		}
	}
	if !found {
		t.Fatalf("NGAP_NOT_QUALIFIED diagnostic missing: %+v", result.Diagnostics)
	}
	enc, err := EncodeNGAPPDU(result.Decoded.(message.Message))
	if err != nil || !bytes.Equal(enc.Binary, raw) {
		t.Fatalf("encode mismatch: %x err=%v", enc.Binary, err)
	}
	if out := DecodeNGAPPDU(append(raw, 0xff)); out.Status == StatusSuccess {
		t.Fatalf("trailing octet was accepted as qualified: %+v", out)
	}
}

func TestEncodeNASRejectsEPCO(t *testing.T) {
	result := DecodeNAS(mustHex(t, "2e0203c1ffff93a37b000e00010401020304000d0303080808"))
	if result.Status != StatusSuccess {
		t.Fatal(result.Status)
	}
	encoded, err := EncodeNAS(result.Decoded.(nasmessage.GSMMessage))
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Status != StatusNotQualified || len(encoded.Binary) != 0 {
		t.Fatalf("ePCO encode was not refused: %+v", encoded)
	}
}

func TestDecodeNGAPUnsupportedProcedure(t *testing.T) {
	result := DecodeNGAPPDU(mustHex(t, "00ff0000"))
	if result.Status != StatusUnsupportedMessage {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
}

func TestN2RootListIsExplicitAndSorted(t *testing.T) {
	roots := N2Roots()
	if len(roots) != 41 {
		t.Fatalf("roots=%d want 41", len(roots))
	}
	for i := 1; i < len(roots); i++ {
		if roots[i-1] >= roots[i] {
			t.Fatalf("roots not sorted: %q >= %q", roots[i-1], roots[i])
		}
	}
	if _, ok := n2Roots["PDUSessionResourceSetupRequestTransfer"]; !ok {
		t.Fatal("setup request root missing")
	}
}

var _ = aper.NewPerBitData

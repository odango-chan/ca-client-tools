package codec

import (
	"bytes"
	"testing"

	nasmessage "github.com/free5gc/nas/message"
	"github.com/free5gc/ngap/aper"
	"github.com/free5gc/ngap/ie"
	"github.com/free5gc/ngap/message"
)

func TestJSONProjectionN2QualifiedRoots(t *testing.T) {
	cases := []struct {
		root string
		hex  string
	}{
		{"PDUSessionResourceSetupRequestTransfer", "000003008b000a01f00a0a0001000007d100860001000088000700010000091c00"},
		{"PDUSessionResourceModifyRequestTransfer", "000001008700170100a000011c0040030d4020030d40200186a0200186a0"},
		{"PDUSessionResourceSetupResponseTransfer", "0003e00a0a0008000003e90001"},
		{"PDUSessionResourceSetupResponseTransfer", "000fe020010db8000000000000000000000001000003eb0001"},
		{"PDUSessionResourceSetupResponseTransfer", "000fe020010db80000000000000000000000021020304004070fc0"},
		{"PDUSessionResourceSetupUnsuccessfulTransfer", "0800"},
		{"PDUSessionResourceModifyResponseTransfer", "100004"},
		{"PDUSessionResourceReleaseCommandTransfer", "10"},
		{"PDUSessionResourceReleaseResponseTransfer", "00"},
	}
	for _, tc := range cases {
		t.Run(tc.root+"/"+tc.hex, func(t *testing.T) {
			raw := mustHex(t, tc.hex)
			decoded := DecodeN2Transfer(tc.root, raw)
			if decoded.Status != StatusSuccess {
				t.Fatalf("decode: %+v", decoded)
			}
			projected, err := ProjectToJSON(decoded.Decoded)
			if err != nil {
				t.Fatal(err)
			}
			typed, err := ProjectFromJSON("ngap.ie."+tc.root, projected)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := EncodeN2Transfer(tc.root, typed)
			if err != nil {
				t.Fatal(err)
			}
			if encoded.Status != StatusSuccess || !bytes.Equal(encoded.Binary, raw) {
				t.Fatalf("JSON N2 round-trip mismatch:\n in=%x\nout=%x\nstatus=%s", raw, encoded.Binary, encoded.Status)
			}
		})
	}
}

func TestJSONProjectionNASQualifiedTypes(t *testing.T) {
	cases := []struct {
		typeName string
		hex      string
	}{
		{"nas.message.PDUSessEstAccept", "2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001"},
		{"nas.message.PDUSessEstReq", "2e0101c1ffff91a1"},
		{"nas.message.PDUSessEstRej", "2e0101c324"},
		{"nas.message.PDUSessModReq", "2e0101c9"},
		{"nas.message.PDUSessRelReq", "2e0501d1"},
		{"nas.message.PDUSessRelComplete", "2e0507d4"},
	}
	for _, tc := range cases {
		t.Run(tc.typeName, func(t *testing.T) {
			raw := mustHex(t, tc.hex)
			decoded := DecodeNAS(raw)
			if decoded.Status != StatusSuccess {
				t.Fatalf("decode: %+v", decoded)
			}
			projected, err := ProjectToJSON(decoded.Decoded)
			if err != nil {
				t.Fatal(err)
			}
			typed, err := ProjectFromJSON(tc.typeName, projected)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := EncodeNAS(typed.(nasmessage.GSMMessage))
			if err != nil {
				t.Fatal(err)
			}
			if encoded.Status != StatusSuccess || !bytes.Equal(encoded.Binary, raw) {
				t.Fatalf("JSON NAS round-trip mismatch:\n in=%x\nout=%x\nstatus=%s", raw, encoded.Binary, encoded.Status)
			}
		})
	}
}

func TestJSONProjectionNAS(t *testing.T) {
	raw := mustHex(t, "2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001")
	result := DecodeNAS(raw)
	if result.Status != StatusSuccess {
		t.Fatal(result.Status)
	}
	projected, err := ProjectToJSON(result.Decoded)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := ProjectFromJSON("nas.message.PDUSessEstAccept", projected)
	if err != nil {
		t.Fatal(err)
	}
	out, err := typed.(interface{ MarshalBinary() ([]byte, error) }).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatalf("JSON round-trip mismatch:\n in=%x\nout=%x", raw, out)
	}
}

func TestJSONProjectionNGAPChoice(t *testing.T) {
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
	projected, err := ProjectToJSON(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(projected, []byte(`"_choice":"ngap.ie.CauseNas"`)) {
		t.Fatalf("choice type missing from projection: %s", projected)
	}
	typed, err := ProjectFromJSON("ngap.message.NGReset", projected)
	if err != nil {
		t.Fatal(err)
	}
	out, err := typed.(message.Message).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatalf("JSON NGAP round-trip mismatch:\n in=%x\nout=%x", raw, out)
	}
}

func TestJSONProjectionBitString(t *testing.T) {
	projected, err := ProjectToJSON(&ie.TransportLayerAddress{Value: aper.BitString{
		Bytes:     []byte{0xc0, 0xa8, 0x0a, 0x01},
		BitLength: 32,
	}})
	if err != nil {
		t.Fatal(err)
	}
	typed, err := ProjectFromJSON("ngap.ie.TransportLayerAddress", projected)
	if err != nil {
		t.Fatal(err)
	}
	value := typed.(*ie.TransportLayerAddress)
	if value.Value.BitLength != 32 || len(value.Value.Bytes) != 4 {
		t.Fatalf("bit string not preserved: %+v", value.Value)
	}
}

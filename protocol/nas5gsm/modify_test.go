package nas5gsm

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

func kbps(t *testing.T, v uint64) QoSFlowBitrate {
	t.Helper()
	b, err := QoSFlowBitrateFromKbps(v)
	if err != nil {
		t.Fatalf("QoSFlowBitrateFromKbps(%d): %v", v, err)
	}
	return b
}

func modificationRequestVector(t *testing.T) ModificationRequest {
	t.Helper()
	return ModificationRequest{
		PduSessionID: 6,
		PTI:          1,
		QoSFlowDescriptions: []QoSFlowDescription{{
			QFI:        1,
			Operation:  QoSFlowOperationModify,
			ReplaceAll: true,
			Parameters: []QoSFlowParameter{
				{Identifier: QoSFlowParameterGFBRUplink, Bitrate: kbps(t, 200)},
				{Identifier: QoSFlowParameterGFBRDownlink, Bitrate: kbps(t, 400)},
				{Identifier: QoSFlowParameterMFBRUplink, Bitrate: kbps(t, 500)},
				{Identifier: QoSFlowParameterMFBRDownlink, Bitrate: kbps(t, 1000)},
			},
		}},
	}
}

// TestModificationRequestGoldenVector pins the exact TS 24.501 wire bytes that
// the pinned OpenAMSM 5GSM decoder (lib/nas/5gs/decoder.c +
// ogs_nas_parse_qos_flow_descriptions) consumes: 0x79 TLV-E, one modify
// description for QFI 1 with E=1 and four bitrate parameters.
func TestModificationRequestGoldenVector(t *testing.T) {
	got, err := EncodeModificationRequest(modificationRequestVector(t))
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "2e0601c9790017"+
		"016044"+
		"02030100c8"+
		"0303010190"+
		"04030101f4"+
		"05030103e8")
	if !bytes.Equal(got, want) {
		t.Fatalf("request = %x, want %x", got, want)
	}

	decoded, err := DecodeModificationRequest(got)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PduSessionID != 6 || decoded.PTI != 1 {
		t.Fatalf("header = %+v", decoded)
	}
	if len(decoded.QoSFlowDescriptions) != 1 {
		t.Fatalf("descriptions = %+v", decoded.QoSFlowDescriptions)
	}
	d := decoded.QoSFlowDescriptions[0]
	if d.QFI != 1 || d.Operation != QoSFlowOperationModify || !d.ReplaceAll {
		t.Fatalf("description = %+v", d)
	}
	if len(d.Parameters) != 4 {
		t.Fatalf("parameters = %+v", d.Parameters)
	}
	wantBitrates := map[QoSFlowParameterIdentifier]uint64{
		QoSFlowParameterGFBRUplink:   200 * 1000,
		QoSFlowParameterGFBRDownlink: 400 * 1000,
		QoSFlowParameterMFBRUplink:   500 * 1000,
		QoSFlowParameterMFBRDownlink: 1000 * 1000,
	}
	for _, p := range d.Parameters {
		gotBPS, err := p.Bitrate.BitsPerSecond()
		if err != nil {
			t.Fatalf("parameter %d bitrate conversion: %v", p.Identifier, err)
		}
		if want, ok := wantBitrates[p.Identifier]; !ok || gotBPS != want {
			t.Fatalf("parameter %d = %d bps, want %d", p.Identifier, gotBPS, want)
		}
	}
}

func TestModificationCommandGoldenVector(t *testing.T) {
	// The real SMF answers a QoS-only modify with the Authorized QoS flow
	// descriptions IE carrying 5QI 9 and the authorized bitrates.
	raw := mustHex(t, "2e0601cb79001a"+
		"016045"+
		"010109"+
		"02030100c8"+
		"0303010190"+
		"04030101f4"+
		"05030103e8")
	cmd, err := DecodeModificationCommand(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.PduSessionID != 6 || cmd.PTI != 1 {
		t.Fatalf("header = %+v", cmd)
	}
	if len(cmd.AuthorizedQoSFlowDescriptions) != 1 {
		t.Fatalf("descriptions = %+v", cmd.AuthorizedQoSFlowDescriptions)
	}
	d := cmd.AuthorizedQoSFlowDescriptions[0]
	if d.QFI != 1 || d.Operation != QoSFlowOperationModify || len(d.Parameters) != 5 {
		t.Fatalf("description = %+v", d)
	}
	if d.Parameters[0].Identifier != QoSFlowParameterFiveQI || d.Parameters[0].FiveQI != 9 {
		t.Fatalf("5qi parameter = %+v", d.Parameters[0])
	}
}

func TestOpenAMSMOracleFixturesMatchGoGoldenVectors(t *testing.T) {
	cases := []struct {
		name string
		want []byte
	}{
		{
			name: "modify-request",
			want: mustHex(t, "2e0601c9790017"+
				"016044"+
				"02030100c8"+
				"0303010190"+
				"04030101f4"+
				"05030103e8"),
		},
		{
			name: "modify-command",
			want: mustHex(t, "2e0601cb79001a"+
				"016045"+
				"010109"+
				"02030100c8"+
				"0303010190"+
				"04030101f4"+
				"05030103e8"),
		},
		{
			name: "modify-complete",
			want: mustHex(t, "2e0601cc"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "../../testdata/nas5gsm/openamsm-oracle/" + tc.name + ".hex"
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := hex.DecodeString(string(bytes.TrimSpace(payload)))
			if err != nil {
				t.Fatalf("decode fixture %s: %v", path, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("fixture %s = %x, want Go golden vector %x", path, got, tc.want)
			}
		})
	}
}

func TestModificationCompleteRoundTrip(t *testing.T) {
	got, err := EncodeModificationComplete(ModificationComplete{PduSessionID: 6, PTI: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, mustHex(t, "2e0601cc")) {
		t.Fatalf("complete = %x", got)
	}
	decoded, err := DecodeModificationComplete(got)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != (ModificationComplete{PduSessionID: 6, PTI: 1}) {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestModificationRequestRoundTripsEverySupportedOperation(t *testing.T) {
	cases := []QoSFlowDescription{
		{QFI: 3, Operation: QoSFlowOperationCreate, ReplaceAll: true, Parameters: []QoSFlowParameter{
			{Identifier: QoSFlowParameterFiveQI, FiveQI: 7},
			{Identifier: QoSFlowParameterMFBRDownlink, Bitrate: kbps(t, 64)},
		}},
		{QFI: 4, Operation: QoSFlowOperationDelete},
		{QFI: 5, Operation: QoSFlowOperationModify, ReplaceAll: false, Parameters: []QoSFlowParameter{
			{Identifier: QoSFlowParameterFiveQI, FiveQI: 9},
		}},
	}
	for _, d := range cases {
		raw, err := EncodeModificationRequest(ModificationRequest{PduSessionID: 2, PTI: 9, QoSFlowDescriptions: []QoSFlowDescription{d}})
		if err != nil {
			t.Fatalf("%+v: %v", d, err)
		}
		decoded, err := DecodeModificationRequest(raw)
		if err != nil {
			t.Fatalf("%+v: %v", d, err)
		}
		if len(decoded.QoSFlowDescriptions) != 1 || decoded.QoSFlowDescriptions[0].QFI != d.QFI ||
			decoded.QoSFlowDescriptions[0].Operation != d.Operation || decoded.QoSFlowDescriptions[0].ReplaceAll != d.ReplaceAll {
			t.Fatalf("round trip %+v -> %+v", d, decoded.QoSFlowDescriptions)
		}
	}
}

func TestQoSFlowBitrateFromKbpsRejectsNonExactAndZero(t *testing.T) {
	if _, err := QoSFlowBitrateFromKbps(0); !errors.Is(err, ErrInvalidField) {
		t.Fatalf("zero bitrate error = %v", err)
	}
	// 70001 Kbps cannot be represented exactly by the unit/value grammar.
	if _, err := QoSFlowBitrateFromKbps(70001); !errors.Is(err, ErrInvalidField) {
		t.Fatalf("inexact bitrate error = %v", err)
	}
	b, err := QoSFlowBitrateFromKbps(70000)
	if err != nil {
		t.Fatal(err)
	}
	bps, err := b.BitsPerSecond()
	if err != nil {
		t.Fatal(err)
	}
	if b.Unit != 2 || b.Value != 17500 || bps != 70000*1000 {
		t.Fatalf("bitrate = %+v (%d bps)", b, bps)
	}
	if _, err := (QoSFlowBitrate{Value: 100, Unit: 25}).BitsPerSecond(); err == nil {
		t.Fatal("expected uint64 overflow to be reported for unit 25")
	}
	if _, err := (QoSFlowBitrate{Value: 1, Unit: 0}).BitsPerSecond(); !errors.Is(err, ErrInvalidField) {
		t.Fatalf("invalid unit error = %v, want ErrInvalidField", err)
	}
}

func TestDecodeModificationRequestRejects(t *testing.T) {
	valid := mustHex(t, "2e0601c979000801604102030100c8")
	cases := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short header", mustHex(t, "2e0601"), ErrMalformed},
		{"bad epd", mustHex(t, "2d0601c9790000"), ErrMalformed},
		{"wrong type command", mustHex(t, "2e0601cb790000"), ErrUnsupported},
		{"psi zero", mustHex(t, "2e0001c9790000"), ErrInvalidField},
		{"psi too high", mustHex(t, "2e1001c9790000"), ErrInvalidField},
		{"pti zero", mustHex(t, "2e0600c9790000"), ErrInvalidField},
		{"pti too high", mustHex(t, "2e06ffc9790000"), ErrInvalidField},
		{"missing ie", mustHex(t, "2e0601c9"), ErrInvalidField},
		{"duplicate ie", mustHex(t, "2e0601c979000801604402030100c879000801604402030100c8"), ErrMalformed},
		{"unsupported ie", mustHex(t, "2e0601c97b0000"), ErrUnsupported},
		{"truncated ie", mustHex(t, "2e0601c97900"), ErrMalformed},
		{"ie length overflow", mustHex(t, "2e0601c9790100"), ErrMalformed},
		{"empty ie", mustHex(t, "2e0601c9790000"), ErrMalformed},
		{"truncated description", mustHex(t, "2e0601c97900010160"), ErrMalformed},
		{"qfi zero", mustHex(t, "2e0601c979000800604102030100c8"), ErrInvalidField},
		{"bad operation", mustHex(t, "2e0601c9790003010000"), ErrInvalidField},
		{"delete with params", mustHex(t, "2e0601c9790006014001010109"), ErrInvalidField},
		{"create without e", mustHex(t, "2e0601c9790006012001010109"), ErrInvalidField},
		{"non-zero spare", mustHex(t, "2e0601c979000841604102030100c8"), ErrMalformed},
		{"duplicate param", mustHex(t, "2e0601c979000d01604202030100c802030100c8"), ErrMalformed},
		{"unknown param", mustHex(t, "2e0601c979000801604106030100c8"), ErrUnsupported},
		{"5qi wrong length", mustHex(t, "2e0601c979000701604101020900"), ErrMalformed},
		{"bitrate wrong length", mustHex(t, "2e0601c979000701604102020100"), ErrMalformed},
		{"bitrate unit zero", mustHex(t, "2e0601c979000801604102030000c8"), ErrInvalidField},
		{"bitrate unit too high", mustHex(t, "2e0601c979000801604102031a00c8"), ErrInvalidField},
	}
	for _, tc := range cases {
		_, err := DecodeModificationRequest(tc.raw)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := DecodeModificationRequest(valid); err != nil {
		t.Fatalf("valid vector: %v", err)
	}
}

func TestDecodeModificationCommandRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short", mustHex(t, "2e0601"), ErrMalformed},
		{"bad epd", mustHex(t, "2d0601cb"), ErrMalformed},
		{"wrong type", mustHex(t, "2e0601cc"), ErrUnsupported},
		{"psi zero", mustHex(t, "2e0001cb"), ErrInvalidField},
		{"pti too high", mustHex(t, "2e06ffcb"), ErrInvalidField},
		{"unsupported ie", mustHex(t, "2e0601cb590100"), ErrUnsupported},
		{"truncated ie", mustHex(t, "2e0601cb7900"), ErrMalformed},
		{"empty ie", mustHex(t, "2e0601cb790000"), ErrMalformed},
	}
	for _, tc := range cases {
		if _, err := DecodeModificationCommand(tc.raw); !errors.Is(err, tc.want) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestDecodeModificationCompleteRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short", mustHex(t, "2e0601"), ErrUnsupported},
		{"extra ie", mustHex(t, "2e0601cc7b0000"), ErrUnsupported},
		{"bad epd", mustHex(t, "2d0601cc"), ErrMalformed},
		{"wrong type", mustHex(t, "2e0601cb"), ErrUnsupported},
		{"psi zero", mustHex(t, "2e0001cc"), ErrInvalidField},
		{"pti zero", mustHex(t, "2e0600cc"), ErrInvalidField},
	}
	for _, tc := range cases {
		if _, err := DecodeModificationComplete(tc.raw); !errors.Is(err, tc.want) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

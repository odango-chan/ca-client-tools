package codec

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"testing"

	"github.com/free5gc/ngap/ie"
)

func TestN2RealBusinessVectors(t *testing.T) {
	t.Run("ipv6-two-qfi-setup-response", func(t *testing.T) {
		raw := mustHex(t, "000fe020010db80000000000000000000000021020304004070fc0")
		result := DecodeN2Transfer("PDUSessionResourceSetupResponseTransfer", raw)
		if result.Status != StatusSuccess {
			t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
		}
		transfer := result.Decoded.(*ie.PDUSessionResourceSetupResponseTransfer)
		if transfer == nil || transfer.DLQosFlowPerTNLInformation == nil {
			t.Fatal("typed projection is incomplete")
		}
		info := transfer.DLQosFlowPerTNLInformation
		tunnel, ok := info.UPTransportLayerInformation.Choice.(*ie.GTPTunnel)
		if !ok || tunnel.TransportLayerAddress == nil || tunnel.GTPTEID == nil {
			t.Fatal("GTP tunnel is missing")
		}
		if got := net.IP(tunnel.TransportLayerAddress.Value.Bytes).String(); got != "2001:db8::2" {
			t.Fatalf("address=%s", got)
		}
		if got := []byte(tunnel.GTPTEID.Value); !bytes.Equal(got, []byte{0x10, 0x20, 0x30, 0x40}) {
			t.Fatalf("TEID=%x", got)
		}
		if info.AssociatedQosFlowList == nil {
			t.Fatal("associated QoS flow list is missing")
		}
		var qfis []int64
		for _, item := range info.AssociatedQosFlowList.List {
			qfis = append(qfis, item.QosFlowIdentifier.Value)
		}
		if len(qfis) != 2 || qfis[0] != 7 || qfis[1] != 63 {
			t.Fatalf("QFIs=%v", qfis)
		}
		projected, err := ProjectToJSON(result.Decoded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(projected, []byte(`"QosFlowIdentifier"`)) {
			t.Fatalf("QoS flow projection missing: %s", projected)
		}
		encoded, err := EncodeN2Transfer("PDUSessionResourceSetupResponseTransfer", result.Decoded)
		if err != nil || !bytes.Equal(encoded.Binary, raw) {
			t.Fatalf("re-encode mismatch: %x err=%v", encoded.Binary, err)
		}
	})

	t.Run("two-qos-flow-setup-request", func(t *testing.T) {
		raw := mustHex(t, "00000400820008080f4240200f4240008b000a01f0ac101f3d263f259100860001000088000d04010000091d00200000091d00")
		result := DecodeN2Transfer("PDUSessionResourceSetupRequestTransfer", raw)
		if result.Status != StatusSuccess {
			t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
		}
		projected, err := ProjectToJSON(result.Decoded)
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(projected, &message); err != nil {
			t.Fatal(err)
		}
		transfer := result.Decoded.(*ie.PDUSessionResourceSetupRequestTransfer)
		if transfer.ProtocolIEs == nil {
			t.Fatal("ProtocolIE container is missing")
		}
		var qfis []int64
		for _, field := range transfer.ProtocolIEs.List {
			if field.QosFlowSetupRequestList == nil {
				continue
			}
			for _, item := range field.QosFlowSetupRequestList.List {
				qfis = append(qfis, item.QosFlowIdentifier.Value)
			}
		}
		if len(qfis) != 2 || qfis[0] != 1 || qfis[1] != 2 {
			t.Fatalf("QFIs=%v", qfis)
		}
		encoded, err := EncodeN2Transfer("PDUSessionResourceSetupRequestTransfer", result.Decoded)
		if err != nil || !bytes.Equal(encoded.Binary, raw) {
			t.Fatalf("re-encode mismatch: %x err=%v", encoded.Binary, err)
		}
	})
}

func TestNASDuplicateEPCOIsRejected(t *testing.T) {
	// A legal ePCO followed by a duplicate ePCO. Upstream silently skips the
	// duplicate; the facade's structural walk must reject it.
	raw := mustHex(t, "2e0203c1ffff93a37b00030001ff7b00030001ff")
	result := DecodeNAS(raw)
	if result.Status != StatusDuplicateIE {
		t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
	}
}

func TestCapabilityManifestIsHonest(t *testing.T) {
	caps := Capabilities()
	if len(caps.NAS) != 20 {
		t.Fatalf("NAS capabilities=%d", len(caps.NAS))
	}
	if len(caps.NGAP) != 120 {
		t.Fatalf("NGAP capabilities=%d", len(caps.NGAP))
	}
	if len(caps.N2) != 41 {
		t.Fatalf("N2 capabilities=%d", len(caps.N2))
	}
	for _, entry := range caps.NAS {
		if entry.CAQualified && !entry.CATested {
			t.Fatalf("NAS %s is qualified but not tested", entry.Name)
		}
		if entry.JSONEncode && !entry.CATested {
			t.Fatalf("NAS %s advertises JSON encode without test coverage", entry.Name)
		}
	}
	for _, entry := range caps.N2 {
		if entry.CAQualified && !entry.CATested {
			t.Fatalf("N2 %s is qualified but not tested", entry.Name)
		}
	}
	for _, entry := range caps.NGAP {
		if entry.CAQualified {
			t.Fatalf("NGAP %s should not be globally qualified without conformance evidence", entry.Name)
		}
	}
}

func TestCapabilityManifestMatchesDocumentedFile(t *testing.T) {
	documented, err := os.ReadFile("../../docs/codec-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	generated, err := json.MarshalIndent(Capabilities(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	generated = append(generated, '\n')
	if !bytes.Equal(documented, generated) {
		t.Fatal("docs/codec-capabilities.json is stale; run go run ./protocol/codec/internal/capgen")
	}
}

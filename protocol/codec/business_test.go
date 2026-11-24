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
	nasTested, nasQualified := 0, 0
	for _, entry := range caps.NAS {
		if entry.CAQualified && !entry.CATested {
			t.Fatalf("NAS %s is qualified but not tested", entry.Name)
		}
		if entry.CAQualified && entry.EvidenceID == "" {
			t.Fatalf("NAS %s is qualified without an evidence ID", entry.Name)
		}
		if entry.CATested && entry.EvidenceID == "" {
			t.Fatalf("NAS %s is tested without an evidence ID", entry.Name)
		}
		if entry.JSONEncode && !entry.CATested {
			t.Fatalf("NAS %s advertises JSON encode without test coverage", entry.Name)
		}
		if entry.CATested {
			nasTested++
		}
		if entry.CAQualified {
			nasQualified++
		}
	}
	if nasTested != 8 || nasQualified != 8 {
		t.Fatalf("NAS tested/qualified = %d/%d want 8/8", nasTested, nasQualified)
	}
	for _, code := range []uint8{0xca, 0xcc, 0xcd, 0xd2} {
		entry := findNASCapability(t, caps, code)
		if entry.CATested || entry.CAQualified || entry.EvidenceID != "" {
			t.Fatalf("NAS 0x%02x must remain untested and unqualified: %+v", code, entry)
		}
		if !entry.UpstreamDecode || !entry.UpstreamEncode {
			t.Fatalf("NAS 0x%02x must keep the upstream capability declaration: %+v", code, entry)
		}
	}
	n2Tested, n2Qualified := 0, 0
	for _, entry := range caps.N2 {
		if entry.CAQualified && !entry.CATested {
			t.Fatalf("N2 %s is qualified but not tested", entry.Name)
		}
		if entry.CAQualified && entry.EvidenceID == "" {
			t.Fatalf("N2 %s is qualified without an evidence ID", entry.Name)
		}
		if entry.CATested {
			n2Tested++
		}
		if entry.CAQualified {
			n2Qualified++
		}
	}
	if n2Tested != 7 || n2Qualified != 7 {
		t.Fatalf("N2 tested/qualified = %d/%d want 7/7", n2Tested, n2Qualified)
	}
	ngapQualified := 0
	for _, entry := range caps.NGAP {
		if entry.CAQualified {
			ngapQualified++
			t.Fatalf("NGAP %s should not be globally qualified without conformance evidence", entry.Name)
		}
	}
	if ngapQualified != 0 {
		t.Fatalf("NGAP qualified=%d want 0", ngapQualified)
	}
}

func findNASCapability(t *testing.T, caps CapabilityManifest, code uint8) NASCapability {
	t.Helper()
	for _, entry := range caps.NAS {
		if entry.MessageType == code {
			return entry
		}
	}
	t.Fatalf("NAS capability 0x%02x not found", code)
	return NASCapability{}
}

// TestRuntimeQualificationMatchesManifest closes the review gap where a
// manifest entry could say qualified=false while the runtime reported
// qualified=true for a byte-identical round-trip.
func TestRuntimeQualificationMatchesManifest(t *testing.T) {
	caps := Capabilities()
	for _, entry := range caps.NAS {
		if got := nasQualifiedMessageType(entry.MessageType); got != entry.CAQualified {
			t.Fatalf("NAS 0x%02x runtime qualified=%t manifest=%t", entry.MessageType, got, entry.CAQualified)
		}
	}
	for _, entry := range caps.N2 {
		if got := n2QualifiedRoot(entry.Name); got != entry.CAQualified {
			t.Fatalf("N2 %s runtime qualified=%t manifest=%t", entry.Name, got, entry.CAQualified)
		}
	}
	for _, entry := range caps.NGAP {
		if ngapPDUQualified(entry.Name) {
			t.Fatalf("NGAP %s runtime qualified without evidence", entry.Name)
		}
	}
}

// TestNASUnqualifiedMessagesStillDecode proves the downgrade is bookkeeping
// only: the Free5GC decoder remains callable, a byte-identical round-trip is
// still reported, and only the qualification claim is withheld.
func TestNASUnqualifiedMessagesStillDecode(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		typ  string
	}{
		{"modification-reject", "2e0101ca24", "PDUSessModRej"},
		{"modification-complete", "2e0101cc", "PDUSessModComplete"},
		{"modification-command-reject", "2e0101cd24", "PDUSessModCmdRej"},
		{"release-reject", "2e0101d224", "PDUSessRelRej"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustHex(t, tc.hex)
			result := DecodeNAS(raw)
			if result.Status != StatusSuccess {
				t.Fatalf("status=%s diagnostics=%+v", result.Status, result.Diagnostics)
			}
			if result.MessageType != tc.typ {
				t.Fatalf("message type=%s want %s", result.MessageType, tc.typ)
			}
			if !result.StructuralParse || !result.ProjectionValid || !result.Complete {
				t.Fatalf("structural/projection flags regressed: %+v", result)
			}
			if !result.ReencodeVerified || !result.ReencodeEligible {
				t.Fatalf("byte round-trip evidence was lost: %+v", result)
			}
			if result.Qualified {
				t.Fatalf("message without independent evidence was qualified: %+v", result)
			}
			found := false
			for _, diag := range result.Diagnostics {
				if diag.Code == DiagNASNotQualified {
					found = true
				}
			}
			if !found {
				t.Fatalf("NAS_NOT_QUALIFIED diagnostic missing: %+v", result.Diagnostics)
			}
			original := append([]byte(nil), result.Raw...)
			if !bytes.Equal(original, raw) || !result.RawPreserved || !result.DataPreserved {
				t.Fatal("raw bytes were not preserved")
			}
		})
	}
}

// TestN2QualificationIsScopedToEvidence proves that root-level evidence is not
// a blanket qualification: only the evidenced Protocol IE IDs are in scope, and
// non-container roots must match an evidenced vector exactly.
func TestN2QualificationIsScopedToEvidence(t *testing.T) {
	setup := n2Roots["PDUSessionResourceSetupRequestTransfer"]
	setupRaw := mustHex(t, "000003008b000a01f00a0a0001000007d100860001000088000700010000091c00")
	if !n2InputWithinScope(setup, setupRaw, []int64{139, 134, 136}) {
		t.Fatal("evidenced setup request scope was rejected")
	}
	if n2InputWithinScope(setup, setupRaw, []int64{139, 197}) {
		t.Fatal("an IE ID outside the evidenced set was accepted as qualified")
	}
	release := n2Roots["PDUSessionResourceReleaseCommandTransfer"]
	if !n2InputWithinScope(release, mustHex(t, "10"), nil) {
		t.Fatal("evidenced release command vector was rejected")
	}
	if n2InputWithinScope(release, mustHex(t, "18"), nil) {
		t.Fatal("a non-evidenced release command vector was accepted as qualified")
	}
	handover := n2Roots["HandoverCommandTransfer"]
	if n2InputWithinScope(handover, mustHex(t, "00"), nil) {
		t.Fatal("a root without evidence was accepted as qualified")
	}
}

// TestEvidenceRegistryIsTraceable validates the evidence registry itself so a
// future entry cannot be added without a decodable independent vector.
func TestEvidenceRegistryIsTraceable(t *testing.T) {
	seen := map[string]bool{}
	for code, evidence := range nasEvidence {
		if evidence.ID == "" {
			t.Fatalf("NAS 0x%02x evidence has no ID", code)
		}
		if seen[evidence.ID] {
			t.Fatalf("duplicate evidence ID %s", evidence.ID)
		}
		seen[evidence.ID] = true
		if evidence.Source == "" {
			t.Fatalf("NAS 0x%02x evidence has no source", code)
		}
		if len(evidence.Vectors) == 0 {
			t.Fatalf("NAS 0x%02x evidence has no vector", code)
		}
		for _, vector := range evidence.Vectors {
			raw := mustHex(t, vector)
			result := DecodeNAS(raw)
			if result.Status != StatusSuccess {
				t.Fatalf("NAS 0x%02x evidence vector %s does not decode: %+v", code, vector, result)
			}
			if !result.ReencodeVerified {
				t.Fatalf("NAS 0x%02x evidence vector %s does not re-encode identically: %+v", code, vector, result)
			}
			if result.Qualified != evidence.Qualified {
				t.Fatalf("NAS 0x%02x evidence vector %s qualified=%t want %t", code, vector, result.Qualified, evidence.Qualified)
			}
		}
	}
	for name, evidence := range n2Evidence {
		if evidence.ID == "" || evidence.Source == "" || len(evidence.Vectors) == 0 {
			t.Fatalf("N2 %s evidence is incomplete: %+v", name, evidence)
		}
		if seen[evidence.ID] {
			t.Fatalf("duplicate evidence ID %s", evidence.ID)
		}
		seen[evidence.ID] = true
		meta := n2Roots[name]
		if meta.ContainerType != "" && len(evidence.ScopeIEIDs) == 0 {
			t.Fatalf("N2 container root %s has no evidenced IE scope", name)
		}
		for _, vector := range evidence.Vectors {
			raw := mustHex(t, vector)
			result := DecodeN2Transfer(name, raw)
			if result.Status != StatusSuccess {
				t.Fatalf("N2 %s evidence vector %s does not decode: %+v", name, vector, result)
			}
			if !result.ReencodeVerified || !result.Qualified {
				t.Fatalf("N2 %s evidence vector %s is not qualified: %+v", name, vector, result)
			}
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

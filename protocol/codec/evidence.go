package codec

import "encoding/hex"

// CAEvidence records the independent vector evidence behind one CA
// qualification claim. It is the single source for both the generated capability
// manifest and the runtime qualification gate.
//
// A record may exist for a message or root whose decode has been exercised but
// whose evidence does not yet justify qualification. In that case Qualified is
// false and the runtime must report qualified=false even when the current input
// happens to re-encode byte-identically.
type CAEvidence struct {
	// ID is a stable traceability identifier. Tests and documents refer to it.
	ID string
	// Source describes who produced the vector and why it is independent of the
	// Free5GC encoder (for example an OpenAMSM or asn1c encoder, or a CA
	// TS 24.501 encoder written independently of the pinned module).
	Source string
	// Vectors are the exact wire inputs used as evidence, lowercase hex.
	Vectors []string
	// Qualified reports whether the recorded vectors justify a CA qualification
	// claim for the declared scope.
	Qualified bool
	// ScopeIEIDs limits a container-root qualification to the Protocol IE IDs
	// actually exercised by Vectors. Empty for non-container roots, where the
	// scope is the exact evidenced vector bytes.
	ScopeIEIDs []int64
}

// Scope describes the input range in which the qualification claim holds.
func (e CAEvidence) Scope(container bool) string {
	if !e.Qualified {
		return "no CA qualification evidence"
	}
	if container {
		return "ext=0, every protocol IE ID is inside the evidenced IE set, strict bit consumption, byte-identical re-encode"
	}
	return "ext=0, input equals an evidenced vector, strict bit consumption, byte-identical re-encode"
}

func (e CAEvidence) decoded() [][]byte {
	out := make([][]byte, 0, len(e.Vectors))
	for _, vector := range e.Vectors {
		raw, err := hex.DecodeString(vector)
		if err != nil {
			// The registry is compile-time data; an invalid literal is a
			// programming error surfaced by the evidence test.
			continue
		}
		out = append(out, raw)
	}
	return out
}

// nasEvidence is the only NAS qualification source. A message type absent from
// this map is not CA tested and cannot become qualified at runtime. The four
// PDU Session reject/complete types 0xCA, 0xCC, 0xCD and 0xD2 deliberately have
// no record: no independent vector exists for them in this repository, so their
// decode remains available through the upstream Free5GC decoder but their
// capability entry stays tested=false / qualified=false.
var nasEvidence = map[uint8]CAEvidence{
	0xc1: {
		ID:        "NAS-C1-CA-ENC-001",
		Source:    "CA TS 24.501 encoder vector (independent of Free5GC), Phase A audit",
		Vectors:   []string{"2e0101c1ffff91a1"},
		Qualified: true,
	},
	0xc2: {
		ID:        "NAS-C2-OPENAMSM-ENC-001",
		Source:    "OpenAMSM ogs_nas_5gs_plain_encode vector (openamsm@594dec78)",
		Vectors:   []string{"2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001"},
		Qualified: true,
	},
	0xc3: {
		ID:        "NAS-C3-OPENAMSM-ENC-001",
		Source:    "OpenAMSM ogs_nas_5gs_plain_encode vector (openamsm@594dec78)",
		Vectors:   []string{"2e0101c324"},
		Qualified: true,
	},
	0xc9: {
		ID:        "NAS-C9-CA-ENC-001",
		Source:    "CA TS 24.501 encoder vector (independent of Free5GC), Phase A audit",
		Vectors:   []string{"2e0101c9"},
		Qualified: true,
	},
	0xcb: {
		ID:        "NAS-CB-OPENAMSM-ENC-001",
		Source:    "OpenAMSM ogs_nas_5gs_plain_encode vector (openamsm@594dec78)",
		Vectors:   []string{"2e0101cb7a000901000631310101ff01790006012041010109"},
		Qualified: true,
	},
	0xd1: {
		ID:        "NAS-D1-CA-ENC-001",
		Source:    "CA TS 24.501 encoder vector (independent of Free5GC), Phase A audit",
		Vectors:   []string{"2e0501d1"},
		Qualified: true,
	},
	0xd3: {
		ID:        "NAS-D3-OPENAMSM-ENC-001",
		Source:    "OpenAMSM ogs_nas_5gs_plain_encode vector (openamsm@594dec78)",
		Vectors:   []string{"2e0101d324"},
		Qualified: true,
	},
	0xd4: {
		ID:        "NAS-D4-CA-ENC-001",
		Source:    "CA TS 24.501 encoder vector (independent of Free5GC), Phase A audit",
		Vectors:   []string{"2e0507d4"},
		Qualified: true,
	},
}

// n2Evidence is the only standalone N2 transfer qualification source. Only the
// seven roots below have independent OpenAMSM/asn1c vectors. The other 34 roots
// remain callable through the Free5GC decoder but are not CA qualified.
var n2Evidence = map[string]CAEvidence{
	"PDUSessionResourceSetupRequestTransfer": {
		ID:     "N2-SETUP-REQ-OPENAMSM-001",
		Source: "OpenAMSM asn1c APER vectors plus a live D5 modify capture",
		Vectors: []string{
			"000003008b000a01f00a0a0001000007d100860001000088000700010000091c00",
			"00000400820008080f4240200f4240008b000a01f0ac101f3d263f259100860001000088000d04010000091d00200000091d00",
		},
		Qualified:  true,
		ScopeIEIDs: []int64{130, 134, 136, 139},
	},
	"PDUSessionResourceModifyRequestTransfer": {
		ID:         "N2-MODIFY-REQ-OPENAMSM-001",
		Source:     "live D5 SMF capture stored by the Mock AMF tests",
		Vectors:    []string{"000001008700170100a000011c0040030d4020030d40200186a0200186a0"},
		Qualified:  true,
		ScopeIEIDs: []int64{135},
	},
	"PDUSessionResourceSetupResponseTransfer": {
		ID:     "N2-SETUP-RSP-OPENAMSM-001",
		Source: "OpenAMSM asn1c APER vectors (IPv4, IPv6, multi-QFI) and env-5gc-sim#83 re-verification",
		Vectors: []string{
			"0003e00a0a0008000003e90001",
			"0003e0c0a80a010102030404010080",
			"000fe020010db8000000000000000000000001000003eb0001",
			"0003e00a141e28123456780801014090",
			"000fe020010db80000000000000000000000021020304004070fc0",
		},
		Qualified: true,
	},
	"PDUSessionResourceSetupUnsuccessfulTransfer": {
		ID:        "N2-SETUP-UNSUCCESSFUL-OPENAMSM-001",
		Source:    "OpenAMSM asn1c APER harness vector (pycrate cross-checked)",
		Vectors:   []string{"0800"},
		Qualified: true,
	},
	"PDUSessionResourceModifyResponseTransfer": {
		ID:        "N2-MODIFY-RSP-OPENAMSM-001",
		Source:    "OpenAMSM asn1c APER harness vector (pycrate cross-checked)",
		Vectors:   []string{"100004"},
		Qualified: true,
	},
	"PDUSessionResourceReleaseCommandTransfer": {
		ID:        "N2-RELEASE-CMD-OPENAMSM-001",
		Source:    "OpenAMSM asn1c APER harness vector (pycrate cross-checked)",
		Vectors:   []string{"10"},
		Qualified: true,
	},
	"PDUSessionResourceReleaseResponseTransfer": {
		ID:        "N2-RELEASE-RSP-OPENAMSM-001",
		Source:    "OpenAMSM asn1c APER vector",
		Vectors:   []string{"00"},
		Qualified: true,
	},
}

// nasQualifiedMessageType is the runtime NAS qualification gate. It answers only
// "does a message type have independent CA evidence"; the caller must still
// prove that the current input is complete, non-duplicate, non-lossy and
// byte-identical before reporting qualified=true.
func nasQualifiedMessageType(code uint8) bool {
	evidence, ok := nasEvidence[code]
	return ok && evidence.Qualified
}

// n2QualifiedRoot reports whether a root has independent evidence at all. The
// concrete input must additionally satisfy n2InputWithinScope.
func n2QualifiedRoot(root string) bool {
	evidence, ok := n2Evidence[shortN2Root(root)]
	return ok && evidence.Qualified
}

// n2InputWithinScope limits qualification to the evidenced input range. A
// container root is in scope only when every protocol IE ID present was
// exercised by an evidenced vector; non-container roots must equal an evidenced
// vector exactly. Byte-identical re-encode alone never widens this scope.
func n2InputWithinScope(meta n2RootMeta, raw []byte, presentIEIDs []int64) bool {
	evidence, ok := n2Evidence[shortN2Root(meta.TypeName)]
	if !ok || !evidence.Qualified {
		return false
	}
	if meta.ContainerType != "" {
		allowed := make(map[int64]bool, len(evidence.ScopeIEIDs))
		for _, id := range evidence.ScopeIEIDs {
			allowed[id] = true
		}
		for _, id := range presentIEIDs {
			if !allowed[id] {
				return false
			}
		}
		return true
	}
	for _, vector := range evidence.decoded() {
		if string(vector) == string(raw) {
			return true
		}
	}
	return false
}

// ngapPDUQualified is intentionally empty: no full NGAP PDU message has
// independent 3GPP protocol evidence in this phase. Byte-identical re-encode is
// recorded as reencodeVerified but never as qualified.
func ngapPDUQualified(string) bool { return false }

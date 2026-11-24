package codec

import (
	"sort"
	"strings"
)

// CapabilityManifest is the machine-readable coverage record. "Upstream
// implemented" is a source fact; "CA tested" and "CA qualified" are narrower
// claims backed by this repository's tests. An untested message is never
// promoted to qualified.
type CapabilityManifest struct {
	SchemaVersion string              `json:"schemaVersion"`
	Modules       []ModulePin         `json:"modules"`
	NAS           []NASCapability     `json:"nas5gsm"`
	NGAP          []MessageCapability `json:"ngapPdu"`
	N2            []N2Capability      `json:"n2Transfer"`
	Limitations   []string            `json:"limitations"`
}

// ModulePin records the exact upstream module versions. The cryptographic
// checksums are the go.sum entries for those versions.
type ModulePin struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type NASCapability struct {
	MessageType      uint8    `json:"messageType"`
	Name             string   `json:"name"`
	TypeName         string   `json:"typeName"`
	UpstreamDecode   bool     `json:"upstreamDecode"`
	UpstreamEncode   bool     `json:"upstreamEncode"`
	CATested         bool     `json:"caTested"`
	CAQualified      bool     `json:"caQualified"`
	JSONEncode       bool     `json:"jsonEncode"`
	UnsupportedIEIs  []uint8  `json:"unsupportedIeIs,omitempty"`
	UnsupportedIENam []string `json:"unsupportedIeNames,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type MessageCapability struct {
	Name           string `json:"name"`
	TypeName       string `json:"typeName"`
	ProcedureCode  int64  `json:"procedureCode"`
	MessageType    int64  `json:"messageType"`
	UpstreamDecode bool   `json:"upstreamDecode"`
	UpstreamEncode bool   `json:"upstreamEncode"`
	CATested       bool   `json:"caTested"`
	CAQualified    bool   `json:"caQualified"`
	JSONEncode     bool   `json:"jsonEncode"`
	Notes          string `json:"notes,omitempty"`
}

type N2Capability struct {
	Name           string  `json:"name"`
	TypeName       string  `json:"typeName"`
	ProtocolIEIDs  []int64 `json:"protocolIeIds,omitempty"`
	Extensible     bool    `json:"rootExtensible"`
	UpstreamDecode bool    `json:"upstreamDecode"`
	UpstreamEncode bool    `json:"upstreamEncode"`
	CATested       bool    `json:"caTested"`
	CAQualified    bool    `json:"caQualified"`
	JSONEncode     bool    `json:"jsonEncode"`
	Notes          string  `json:"notes,omitempty"`
}

// The tested/qualified sets are intentionally explicit. Add a message only
// after a vector test (protocol layer or real business layer) exists.
var nasTested = map[uint8]bool{
	0xc1: true, 0xc2: true, 0xc3: true, 0xc9: true,
	0xca: true, 0xcb: true, 0xcc: true, 0xcd: true,
	0xd1: true, 0xd2: true, 0xd3: true, 0xd4: true,
}

var nasQualified = map[uint8]bool{
	0xc1: true, 0xc2: true, 0xc3: true, 0xc9: true,
	0xca: true, 0xcb: true, 0xcc: true, 0xcd: true,
	0xd1: true, 0xd2: true, 0xd3: true, 0xd4: true,
}

// JSON encode is gated separately: only types with a dedicated projection
// round-trip test are advertised as re-encodable from JSON.
var nasJSONEncode = map[uint8]bool{
	0xc1: true, 0xc2: true, 0xc3: true, 0xc9: true,
	0xd1: true, 0xd4: true,
}

var ngapPDUTested = map[string]bool{
	"NGReset": true,
}

var ngapPDUJSONEncode = map[string]bool{
	"NGReset": true,
}

var n2Tested = map[string]bool{
	"PDUSessionResourceSetupRequestTransfer":      true,
	"PDUSessionResourceModifyRequestTransfer":     true,
	"PDUSessionResourceSetupResponseTransfer":     true,
	"PDUSessionResourceSetupUnsuccessfulTransfer": true,
	"PDUSessionResourceModifyResponseTransfer":    true,
	"PDUSessionResourceReleaseCommandTransfer":    true,
	"PDUSessionResourceReleaseResponseTransfer":   true,
}

var n2Qualified = map[string]bool{
	"PDUSessionResourceSetupRequestTransfer":      true,
	"PDUSessionResourceModifyRequestTransfer":     true,
	"PDUSessionResourceSetupResponseTransfer":     true,
	"PDUSessionResourceSetupUnsuccessfulTransfer": true,
	"PDUSessionResourceModifyResponseTransfer":    true,
	"PDUSessionResourceReleaseCommandTransfer":    true,
	"PDUSessionResourceReleaseResponseTransfer":   true,
}

var n2JSONEncode = map[string]bool{
	"PDUSessionResourceSetupRequestTransfer":      true,
	"PDUSessionResourceModifyRequestTransfer":     true,
	"PDUSessionResourceSetupResponseTransfer":     true,
	"PDUSessionResourceSetupUnsuccessfulTransfer": true,
	"PDUSessionResourceModifyResponseTransfer":    true,
	"PDUSessionResourceReleaseCommandTransfer":    true,
	"PDUSessionResourceReleaseResponseTransfer":   true,
}

// Capabilities returns a fresh, deterministic copy of the capability manifest.
func Capabilities() CapabilityManifest {
	manifest := CapabilityManifest{
		SchemaVersion: "ca-codec-capabilities/v1",
		Modules: []ModulePin{
			{Path: "github.com/free5gc/nas", Version: "v1.3.1"},
			{Path: "github.com/free5gc/ngap", Version: "v1.2.0"},
		},
		Limitations: []string{
			"Free5GC NGAP v1.2.0 APER silently discards extensible-SEQUENCE extension additions. The facade reports a root extension bit as NOT_QUALIFIED and uses byte-identical re-encode as the qualification gate; nested extension additions are not independently proven and must not be treated as fully decoded.",
			"NAS PARTIAL is recognized only through the concrete *message.Error IEToDoList type, never by matching an error string.",
			"NAS ePCO can decode successfully while re-encoding lossily. A SUCCESS decode therefore does not imply reencodeEligible; the ePCO projection is decode-projection-only.",
			"NAS duplicate IEs are detected by a facade structural IEI walk because the upstream decoder may silently skip them.",
			"Unknown NAS IEs and NGAP protocol IE IDs outside the upstream container switch fail closed; the original bytes remain authoritative.",
			"JSON Encode is advertised only for types with a dedicated canonical-projection round-trip test. Other types are decode-projection-only.",
			"Self-encoded round-trip tests prove facade consistency, not 3GPP conformance. The real OpenAMSM and asn1c vectors remain the interoperability evidence.",
		},
	}

	indexes := make([]int, 0, len(nasMessages))
	for code := range nasMessages {
		indexes = append(indexes, int(code))
	}
	sort.Ints(indexes)
	for _, rawCode := range indexes {
		code := uint8(rawCode)
		meta := nasMessages[code]
		entry := NASCapability{
			MessageType:    code,
			Name:           meta.StructName,
			TypeName:       meta.TypeName,
			UpstreamDecode: true,
			UpstreamEncode: true,
			CATested:       nasTested[code],
			CAQualified:    nasQualified[code],
			JSONEncode:     nasJSONEncode[code],
		}
		for _, field := range meta.Optional {
			if field.Partial {
				entry.UnsupportedIEIs = append(entry.UnsupportedIEIs, field.IEI)
				entry.UnsupportedIENam = append(entry.UnsupportedIENam, field.Field)
			}
		}
		if len(entry.UnsupportedIEIs) > 0 {
			entry.Notes = "Some optional IEs are recognized by Free5GC but only return IEToDo; a message carrying them is PARTIAL."
		}
		manifest.NAS = append(manifest.NAS, entry)
	}

	for _, msg := range ngapMessages {
		manifest.NGAP = append(manifest.NGAP, MessageCapability{
			Name:           msg.Name,
			TypeName:       msg.TypeName,
			ProcedureCode:  msg.ProcedureCode,
			MessageType:    msg.MessageType,
			UpstreamDecode: true,
			UpstreamEncode: true,
			CATested:       ngapPDUTested[msg.Name],
			CAQualified:    false,
			JSONEncode:     ngapPDUJSONEncode[msg.Name],
			Notes:          "Full-PDU vectors are not an independent 3GPP conformance suite; decode/encode availability is a source fact, not a qualification claim.",
		})
	}

	rootNames := make([]string, 0, len(n2Roots))
	for name := range n2Roots {
		rootNames = append(rootNames, name)
	}
	sort.Strings(rootNames)
	for _, name := range rootNames {
		meta := n2Roots[name]
		manifest.N2 = append(manifest.N2, N2Capability{
			Name:           name,
			TypeName:       meta.TypeName,
			ProtocolIEIDs:  append([]int64(nil), meta.IEIDs...),
			Extensible:     meta.Extensible,
			UpstreamDecode: true,
			UpstreamEncode: true,
			CATested:       n2Tested[name],
			CAQualified:    n2Qualified[name],
			JSONEncode:     n2JSONEncode[name],
		})
	}
	return manifest
}

// JSONEncodeSupported reports whether the JSON Encode gate is open for a
// qualified type name or N2 root. It never authorizes a protocol path that the
// facade has not tested.
func JSONEncodeSupported(typeName string) bool {
	shortRoot := strings.TrimPrefix(typeName, "ngap.ie.")
	if meta, ok := n2Roots[shortRoot]; ok {
		return n2JSONEncode[shortN2Root(meta.TypeName)]
	}
	for code, meta := range nasMessages {
		if meta.TypeName == typeName {
			return nasJSONEncode[code]
		}
	}
	for _, msg := range ngapMessages {
		if msg.TypeName == typeName {
			return ngapPDUJSONEncode[msg.Name]
		}
	}
	return false
}

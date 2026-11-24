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
	EvidenceID       string   `json:"evidenceId,omitempty"`
	EvidenceSource   string   `json:"evidenceSource,omitempty"`
	QualifiedScope   string   `json:"qualifiedScope,omitempty"`
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
	EvidenceID     string `json:"evidenceId,omitempty"`
	EvidenceSource string `json:"evidenceSource,omitempty"`
	QualifiedScope string `json:"qualifiedScope,omitempty"`
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
	EvidenceID     string  `json:"evidenceId,omitempty"`
	EvidenceSource string  `json:"evidenceSource,omitempty"`
	QualifiedScope string  `json:"qualifiedScope,omitempty"`
	Notes          string  `json:"notes,omitempty"`
}

// The tested/qualified sets below are derived from the single evidence registry
// in evidence.go. Adding a map literal here cannot promote a message: only an
// independent vector record can.

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
			"Free5GC NGAP v1.2.0 APER silently discards extensible-SEQUENCE extension additions. The facade reports a root extension bit as NOT_QUALIFIED and uses byte-identical re-encode only as reencodeVerified evidence; no full NGAP PDU is CA qualified in this phase and nested extension additions are not independently proven.",
			"NAS PARTIAL is recognized only through the concrete *message.Error IEToDoList type, never by matching an error string.",
			"NAS ePCO can decode successfully while re-encoding lossily. A SUCCESS decode therefore does not imply reencodeEligible; the ePCO projection is decode-projection-only.",
			"NAS duplicate IEs are detected by a facade structural IEI walk because the upstream decoder may silently skip them.",
			"Unknown NAS IEs and NGAP protocol IE IDs outside the upstream container switch fail closed; the original bytes remain authoritative.",
			"JSON Encode is advertised only for types with a dedicated canonical-projection round-trip test. Other types are decode-projection-only.",
			"reencodeVerified=true means only that the current input re-encoded to the same bytes. qualified=true additionally requires an independent vector record in the CA evidence registry and every input-scope condition for that message or transfer root.",
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
		evidence, hasEvidence := nasEvidence[code]
		entry := NASCapability{
			MessageType:    code,
			Name:           meta.StructName,
			TypeName:       meta.TypeName,
			UpstreamDecode: true,
			UpstreamEncode: true,
			CATested:       hasEvidence,
			CAQualified:    hasEvidence && evidence.Qualified,
			JSONEncode:     nasJSONEncode[code],
		}
		if hasEvidence {
			entry.EvidenceID = evidence.ID
			entry.EvidenceSource = evidence.Source
			entry.QualifiedScope = evidence.Scope(false)
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
		if !hasEvidence {
			if entry.Notes != "" {
				entry.Notes += " "
			}
			entry.Notes += "Decode/encode availability is a Free5GC source fact only; no independent CA vector exists, so this type is callable but never CA qualified."
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
			CAQualified:    ngapPDUQualified(msg.Name),
			JSONEncode:     ngapPDUJSONEncode[msg.Name],
			Notes:          "Full-PDU availability is a Free5GC source fact. No NGAP PDU has independent 3GPP protocol evidence in this phase; byte-identical re-encode is reencodeVerified, not CA qualification.",
		})
	}

	rootNames := make([]string, 0, len(n2Roots))
	for name := range n2Roots {
		rootNames = append(rootNames, name)
	}
	sort.Strings(rootNames)
	for _, name := range rootNames {
		meta := n2Roots[name]
		evidence, hasEvidence := n2Evidence[shortN2Root(meta.TypeName)]
		entry := N2Capability{
			Name:           name,
			TypeName:       meta.TypeName,
			ProtocolIEIDs:  append([]int64(nil), meta.IEIDs...),
			Extensible:     meta.Extensible,
			UpstreamDecode: true,
			UpstreamEncode: true,
			CATested:       hasEvidence,
			CAQualified:    hasEvidence && evidence.Qualified,
			JSONEncode:     n2JSONEncode[name],
		}
		if hasEvidence {
			entry.EvidenceID = evidence.ID
			entry.EvidenceSource = evidence.Source
			entry.QualifiedScope = evidence.Scope(meta.ContainerType != "")
		}
		if !hasEvidence {
			entry.Notes = "Decode/encode availability is a Free5GC source fact only; no independent CA vector exists for this root, so it is callable but never CA qualified."
		}
		manifest.N2 = append(manifest.N2, entry)
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

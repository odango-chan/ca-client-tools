package codec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/free5gc/ngap/aper"
	"github.com/free5gc/ngap/ie"
)

// n2RootMeta is generated from the pinned ngap/ie package. It records the
// roots the upstream module exposes and, for ProtocolIE-container roots, the
// exact IE IDs the upstream item decoder understands.
type n2RootMeta struct {
	TypeName      string
	ContainerType string
	IEIDs         []int64
	Extensible    bool
}

type n2Wire interface {
	Read(*aper.PerBitData) error
	Write(*aper.PerBitData) error
}

// N2Roots returns the sorted, short names of every standalone N2 transfer root
// exposed by the pinned Free5GC module.
func N2Roots() []string {
	roots := make([]string, 0, len(n2Roots))
	for name := range n2Roots {
		roots = append(roots, name)
	}
	sortStrings(roots)
	return roots
}

// N2RootTypeName resolves a short or qualified standalone root name to its
// qualified ngap.ie type name.
func N2RootTypeName(root string) (string, bool) {
	meta, ok := lookupN2Root(root)
	if !ok {
		return "", false
	}
	return meta.TypeName, true
}

// DecodeN2Transfer decodes one standalone N2 SM transfer. root must be an
// explicit root name; it is never inferred from the bytes. Both the short name
// (PDUSessionResourceSetupRequestTransfer) and the qualified name
// (ngap.ie.PDUSessionResourceSetupRequestTransfer) are accepted.
func DecodeN2Transfer(root string, input []byte) DecodeResult {
	raw := append([]byte(nil), input...)
	result := DecodeResult{
		Protocol:      ProtocolN2SM,
		Status:        StatusInvalid,
		Root:          root,
		Raw:           raw,
		RawLength:     len(raw),
		RawPreserved:  true,
		DataPreserved: true,
		TotalBits:     len(raw) * 8,
	}
	result.Diagnostics = append(result.Diagnostics, diagnostic(DiagRawPreserved, "raw N2 transfer bytes retained unchanged"))
	if root == "" {
		result.Status = StatusUnsupportedMessage
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagN2RootUnsupported,
			"standalone N2 transfer requires an explicit root type"))
		return result
	}
	meta, ok := lookupN2Root(root)
	if !ok {
		result.Status = StatusUnsupportedMessage
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagN2RootUnsupported,
			"root is not a transfer type in the pinned ngap/ie package"))
		return result
	}
	result.Root = shortN2Root(meta.TypeName)
	result.TypeName = meta.TypeName
	ctor, ok := upstreamTypes[meta.TypeName]
	if !ok {
		result.Status = StatusUnsupportedMessage
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagN2RootUnsupported,
			"root type is missing from the generated Free5GC type registry"))
		return result
	}
	wire, ok := ctor().(n2Wire)
	if !ok {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagN2RootInvalid,
			"root type does not implement the upstream N2 transfer wire interface"))
		return result
	}
	result.Decoded = wire
	rootExtensionBit := meta.Extensible && len(raw) > 0 && raw[0]&0x80 != 0
	if rootExtensionBit {
		result.Decoded = nil
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPExtensionRisk,
			"the standalone root's extensible-SEQUENCE bit is 1; unpatched Free5GC APER discards extension additions"))
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPRoundTripMismatch,
			"root extension bit is set; no typed projection or re-encode is offered"))
		return result
	}

	var presentIEIDs []int64
	if meta.ContainerType != "" {
		var issue *nasScanIssue
		presentIEIDs, issue = scanN2Container(raw, meta)
		if issue != nil {
			result.Status = issue.status
			result.Diagnostics = append(result.Diagnostics, issue.diag)
			return result
		}
	}

	pd := aper.NewPerBitData(append([]byte(nil), raw...))
	if err := wire.Read(pd); err != nil {
		status, diag := classifyNGAPError(err)
		result.Status = status
		result.Diagnostics = append(result.Diagnostics, diag)
		return result
	}
	result.StructuralParse = true
	result.ProjectionValid = true
	consumed := int(pd.ByteOffset())*8 + int(pd.BitOffset())
	result.ConsumedBits = consumed
	if consumed > len(raw)*8 {
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPTrailing,
			"decoder consumed more bits than the input contains"))
		return result
	}
	if remaining := len(raw)*8 - consumed; remaining >= 8 {
		result.Status = StatusInvalid
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPTrailing,
			fmt.Sprintf("%d trailing bits remain after the root value", remaining)))
		return result
	} else if remaining > 0 && !onlyZeroBits(raw, consumed, len(raw)*8) {
		result.Status = StatusInvalid
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPTrailing,
			"non-zero padding bits remain after the root value"))
		return result
	}

	// Re-encode only as a byte-identity gate. This is the facade's evidence
	// against the unpatched APER extension-addition silence; a mismatch is
	// never repaired or emitted.
	outPD := aper.NewPerBitData(nil)
	if err := wire.Write(outPD); err != nil {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPEncodeUnsupported,
			"typed N2 transfer cannot be re-encoded by the upstream encoder"))
		return result
	}
	out := outPD.Bytes()
	if !bytes.Equal(out, raw) {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPRoundTripMismatch,
			fmt.Sprintf("N2 re-encode differs from the input (%d vs %d octets); typed view is projection-only", len(out), len(raw))))
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPExtensionRisk,
			"unpatched Free5GC APER silently ignores extension additions at the root and nested SEQUENCEs"))
		return result
	}

	result.Status = StatusSuccess
	result.Complete = true
	result.ReencodeVerified = true
	result.ReencodeEligible = true
	// Byte identity never widens qualification. The root must have independent
	// evidence and this input must remain inside the evidenced IE / vector scope.
	if n2InputWithinScope(meta, raw, presentIEIDs) {
		result.Qualified = true
		return result
	}
	result.Qualified = false
	result.Diagnostics = append(result.Diagnostics, diagnostic(DiagN2NotQualified,
		"root and input are outside the independently evidenced CA scope; decode and byte round-trip succeeded, qualification does not"))
	return result
}

// EncodeN2Transfer encodes a typed N2 transfer. The root is explicit and is
// checked against the same generated root list used by decode.
func EncodeN2Transfer(root string, value any) (EncodeResult, error) {
	result := EncodeResult{Protocol: ProtocolN2SM, Status: StatusInvalid, Root: root}
	if root == "" {
		return result, codecError("N2_ROOT_REQUIRED", "standalone N2 transfer requires an explicit root type")
	}
	meta, ok := lookupN2Root(root)
	if !ok {
		return result, codecError("N2_ROOT_UNSUPPORTED", "unknown standalone N2 transfer root %q", root)
	}
	wire, ok := value.(n2Wire)
	if !ok || value == nil {
		return result, codecError("N2_VALUE_INVALID", "value does not implement the N2 transfer wire interface")
	}
	if got := qualifiedGoType(value, "ngap.ie"); got != meta.TypeName {
		return result, codecError("N2_ROOT_MISMATCH", "value type %s does not match root %s", got, meta.TypeName)
	}
	result.Root = shortN2Root(meta.TypeName)
	result.TypeName = meta.TypeName
	pd := aper.NewPerBitData(nil)
	if err := wire.Write(pd); err != nil {
		result.Status = StatusNotQualified
		result.Diagnostics = append(result.Diagnostics, diagnostic(DiagNGAPEncodeUnsupported, err.Error()))
		return result, nil
	}
	result.Status = StatusSuccess
	result.Binary = append([]byte(nil), pd.Bytes()...)
	result.Length = len(result.Binary)
	return result, nil
}

func lookupN2Root(root string) (n2RootMeta, bool) {
	if meta, ok := n2Roots[root]; ok {
		return meta, true
	}
	prefix := "ngap.ie."
	if strings.HasPrefix(root, prefix) {
		meta, ok := n2Roots[strings.TrimPrefix(root, prefix)]
		return meta, ok
	}
	return n2RootMeta{}, false
}

func shortN2Root(qualified string) string {
	return strings.TrimPrefix(qualified, "ngap.ie.")
}

func scanN2Container(raw []byte, meta n2RootMeta) ([]int64, *nasScanIssue) {
	// Reuse the small issue carrier; its status/diagnostic shape is protocol
	// independent. No NAS-specific decoding is performed here.
	allowed := make(map[int64]bool, len(meta.IEIDs))
	for _, id := range meta.IEIDs {
		allowed[id] = true
	}
	pd := aper.NewPerBitData(append([]byte(nil), raw...))
	flags := []bool{}
	if err := pd.ReadSequencePreambleBitMap(&flags, true); err != nil {
		return nil, &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNGAPDecode,
			"cannot read the N2 transfer root preamble")}
	}
	lb, ub := uint64(0), uint64(65535)
	count, err := pd.ReadSequenceOfPreambleBitMap(false, &lb, &ub)
	if err != nil {
		return nil, &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNGAPDecode,
			"cannot read the N2 ProtocolIE container count")}
	}
	seen := make(map[int64]bool, count)
	present := make([]int64, 0, count)
	for i := uint64(0); i < count; i++ {
		offset := int(pd.ByteOffset())
		id, err := ie.ReadProtocolIEID(pd)
		if err != nil {
			return nil, &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNGAPDecode,
				"cannot read an N2 protocol IE ID")}
		}
		if !allowed[id] {
			diag := diagnostic(DiagNGAPUnsupportedIE,
				fmt.Sprintf("N2 transfer root does not implement protocol IE ID %d", id))
			diag.IEID = &id
			diag.Offset = &offset
			return nil, &nasScanIssue{status: StatusUnsupportedIE, diag: diag}
		}
		if seen[id] {
			diag := diagnostic(DiagNGAPDuplicateIE,
				fmt.Sprintf("duplicate N2 protocol IE ID %d is not permitted", id))
			diag.IEID = &id
			diag.Offset = &offset
			return nil, &nasScanIssue{status: StatusDuplicateIE, diag: diag}
		}
		seen[id] = true
		present = append(present, id)
		crit := &ie.ProtocolIECriticality{}
		if err := crit.Read(pd); err != nil {
			return nil, &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNGAPDecode,
				"cannot read an N2 protocol IE criticality")}
		}
		if _, err := pd.ReadOpenType(); err != nil {
			return nil, &nasScanIssue{status: StatusInvalid, diag: diagnostic(DiagNGAPDecode,
				"cannot read an N2 protocol IE open type")}
		}
	}
	return present, nil
}

func onlyZeroBits(raw []byte, from, to int) bool {
	for bit := from; bit < to; bit++ {
		if raw[bit/8]&(1<<uint(7-bit%8)) != 0 {
			return false
		}
	}
	return true
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

var _ = errors.Is

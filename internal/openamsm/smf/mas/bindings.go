package mas

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type invokeBinding struct {
	step, mas, op string
	// Fields after '=' are wire names. Identity fields are checked separately.
	inputs, outputs string
}

var invokeBindings = map[string]invokeBinding{
	"SC.VALIDATE_SUBSCRIPTION":     {"s1", "SC", "VALIDATE_SUBSCRIPTION", "ue_identity psi dnn sNssai", "validated:bit sub_summary secondary_auth_required:bit?"},
	"SAA.SECONDARY_AUTH":           {"s2", "SAA", "SECONDARY_AUTH", "ue_identity required=required:0", "phase:SKIPPED"},
	"SC.CREATE_SM_CONTEXT":         {"s3", "SC", "CREATE_SM_CONTEXT", "ue_identity psi dnn sNssai subSummary=sub_summary", "session_keys lifecycle:life sm_context_ref:ref"},
	"SC.REGISTER_UECM":             {"s3r", "SC", "REGISTER_UECM", "ue_identity psi sessionKeys=session_keys", "uecm_registered:bit"},
	"PA.CREATE_SM_POLICY":          {"s3p", "PA", "CREATE_SM_POLICY", "ue_identity psi dnn sNssai pduSessionType=pdu_session_type:IPV4 sessionKeys=session_keys", "up_intent authorized_qos policy_assoc_id"},
	"UPS.DECIDE_ANCHOR":            {"s4", "UPS", "DECIDE_ANCHOR", "dnn sNssai", "up_anchor:bit"},
	"IAM.ALLOCATE_ADDRESS":         {"s5", "IAM", "ALLOCATE_ADDRESS", "dnn allocator=allocator:LOCAL_POOL upAnchorReady=up_anchor", "ue_addr:bit"},
	"PA.UPDATE_SM_POLICY":          {"s6u", "PA", "UPDATE_SM_POLICY", "ue_identity psi dnn sNssai pduSessionType=pdu_session_type:IPV4 sessionKeys=session_keys ueAddrAllocated=ue_addr policyAssociationId=policy_assoc_id", "up_intent authorized_qos policy_assoc_id"},
	"CA.CREATE_CHARGING":           {"s7", "CA", "CREATE_CHARGING", "ue_identity=supi psi ratingGroupRefs=rating_group_refs:default", "charging_ref:1"},
	"UPN.CREATE_UP_SESSION":        {"s8", "UPN", "CREATE_UP_SESSION", "upIntent=up_intent upAnchorReady=up_anchor ueAddrAllocated=ue_addr dnn sNssai pduSessionType=pdu_session_type:IPV4", "n4_seid:uint est_done:bit"},
	"SSM.SIGNAL_UE_AN":             {"s10", "SSM", "SIGNAL_UE_AN", "sessionKeys=session_keys ueAddrAllocated=ue_addr n4Seid=n4_seid", "an_tunnel an_tunnel_teid:uint an_tunnel_ip:ip ran_teid:uint?"},
	"UPN.MODIFY_UP_SESSION":        {"s11", "UPN", "MODIFY_UP_SESSION", "anTunnelTeid=an_tunnel_teid anTunnelIp=an_tunnel_ip n4Seid=n4_seid", "n3_synced:bit n4_seid:uint?"},
	"UPN.MODIFY_QOS_FLOWS":         {"s11", "UPN", "MODIFY_UP_SESSION", "anTunnelTeid=an_tunnel_teid anTunnelIp=an_tunnel_ip n4Seid=n4_seid qosFlowsDelta=qos_flows_delta:1 qfi=qfi fiveQi=five_qi gfbrUl=gfbr_ul gfbrDl=gfbr_dl mfbrUl=mfbr_ul mfbrDl=mfbr_dl", "qos_delta_state:qosstate n1n2_dispatched:bit? qfi:qfi five_qi:fiveqi? gfbr_ul:bitrate gfbr_dl:bitrate mfbr_ul:bitrate mfbr_dl:bitrate n4_seid:uint?"},
	"SE.EMIT_EVENTS":               {"s13", "SE", "EMIT_EVENTS", "event=ev:ESTABLISHED sessionKeys=session_keys", "last_event:1"},
	"SC.UPDATE_SM_CONTEXT":         {"m0", "SC", "UPDATE_SM_CONTEXT", "", "lifecycle:OPERATIONAL sm_context_ref:ref?"},
	"UPN.MODIFY_UP_SESSION_INTENT": {"m3", "UPN", "MODIFY_UP_SESSION", "upIntent=up_intent:1", "n4_seid"},
	"SSM.SIGNAL_UE_AN_MODIFY":      {"m4", "SSM", "SIGNAL_UE_AN", "", "an_tunnel an_tunnel_teid:uint an_tunnel_ip:ip ran_teid:uint?"},
	"UPN.DELETE_UP_SESSION":        {"r2", "UPN", "DELETE_UP_SESSION", "trigger", "released:1"},
	"SSM.SIGNAL_UE_AN_RELEASE":     {"r3", "SSM", "SIGNAL_UE_AN", "release=release:1 trigger", ""},
	"PA.DELETE_SM_POLICY":          {"r4", "PA", "DELETE_SM_POLICY", "release=release:1", ""},
	"SC.RELEASE_SM_CONTEXT":        {"r5", "SC", "RELEASE_SM_CONTEXT", "release=release:1", "lifecycle:RELEASING"},
	"SE.EMIT_RELEASE_EVENT":        {"r6", "SE", "EMIT_EVENTS", "ev=ev:RELEASED", ""},

	// Scheme C UE Modify/Release occurrences. The merged OpenAMSM Owner owns the
	// native transaction: SSM parses/builds/confirms the original N1 and UPN
	// applies or deletes the user plane. The OES-shaped m0/m3/m4/m5 and r2-r6
	// callables above stay addressable for the older Procedures but are not the
	// Scheme C execution order.
	"SSM.PARSE_N1_MODIFY":       {"c-parse", "SSM", "SIGNAL_UE_AN", "parse_n1=parse_n1:1 n1_sm", "qfi five_qi? gfbr_ul? gfbr_dl? mfbr_ul? mfbr_dl? requested_kind:qos_flow_descriptions"},
	"UPN.MODIFY_QOS_DELTA":      {"c-apply", "UPN", "MODIFY_UP_SESSION", "qos_flows_delta=qos_flows_delta:1 qfi=qfi five_qi=five_qi gfbr_ul=gfbr_ul gfbr_dl=gfbr_dl mfbr_ul=mfbr_ul mfbr_dl=mfbr_dl", "qos_delta_state:qosstate n1n2_dispatched:bit? qfi:qfi five_qi:fiveqi? gfbr_ul:bitrate gfbr_dl:bitrate mfbr_ul:bitrate mfbr_dl:bitrate n4_seid:uint?"},
	"SSM.PARSE_N1_RELEASE":      {"r-parse", "SSM", "SIGNAL_UE_AN", "parse_n1=parse_n1:1 n1_sm", "psi pti cause? release_intent:ue_requested"},
	"SSM.BUILD_RELEASE_COMMAND": {"r-build", "SSM", "SIGNAL_UE_AN", "release=release:1 mode=mode:build trigger=trigger release_txn", "n1_sm_hex:hex n2_sm_hex:hex n2_sm_info_type:PDU_RES_REL_CMD release_generation:uint release_txn release_phase:command_built"},
	"SSM.RELEASE_CONFIRM_N1":    {"r-confirm-n1", "SSM", "SIGNAL_UE_AN", "release_confirm=release_confirm:n1 release_txn release_generation", "release_ready:bit n1_released:bit n2_released:bit"},
	"SSM.RELEASE_CONFIRM_N2":    {"r-confirm-n2", "SSM", "SIGNAL_UE_AN", "release_confirm=release_confirm:n2 release_txn release_generation", "release_ready:bit n1_released:bit n2_released:bit"},
	"PA.DELETE_SM_POLICY_C":     {"r-pa", "PA", "DELETE_SM_POLICY", "release=release:1 trigger=trigger", "terminated:1"},
	"SE.EMIT_RELEASE_EVENT_C":   {"r-se", "SE", "EMIT_EVENTS", "ev=ev:RELEASED", "last_event:1"},
	"SC.RELEASE_SM_CONTEXT_C":   {"r-sc", "SC", "RELEASE_SM_CONTEXT", "release=release:1 finalize=finalize:1 trigger=trigger", "lifecycle:RELEASING user_plane_ready:0 sm_context_ref:ref?"},
}

// errInvokeInput is a typed input error so the Provider classifies a bad
// callable argument as BACKEND_ARGUMENTS_INVALID instead of a transport error.
var errInvokeInput = &InputError{Reason: "invalid OpenAMSM MAS operation inputs or scope"}

// The typed QoS-delta profile is the one callable whose decimal values are
// business quantities rather than opaque references. These are the exact
// canonical lexical forms from the published input Schema; the Owner repeats
// the checks authoritatively.
var qosDeltaPatterns = map[string]*regexp.Regexp{
	"qfi":     regexp.MustCompile(`^([1-9]|[1-5][0-9]|6[0-3])$`),
	"five_qi": regexp.MustCompile(`^([1-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])$`),
	"gfbr_ul": regexp.MustCompile(`^[1-9][0-9]{0,12}$`),
	"gfbr_dl": regexp.MustCompile(`^[1-9][0-9]{0,12}$`),
	"mfbr_ul": regexp.MustCompile(`^[1-9][0-9]{0,12}$`),
	"mfbr_dl": regexp.MustCompile(`^[1-9][0-9]{0,12}$`),
}

// readOnlyCallables are the Scheme C occurrences that only read the Owner's
// state (they decode the original N1). They are still fixed Provider callables
// but they are not mutations, so the Host/Runtime must not count them as Effects
// that changed the network.
var readOnlyCallables = map[string]bool{
	"SSM.PARSE_N1_MODIFY":  true,
	"SSM.PARSE_N1_RELEASE": true,
}

func (b invokeBinding) isQoSDelta() bool {
	// The Scheme C typed delta (`UPN.MODIFY_QOS_DELTA`, occurrence c-apply) and
	// the older OES-shaped `UPN.MODIFY_QOS_FLOWS` (s11) share one Owner contract
	// and therefore one Provider-side validation.
	return b.mas == "UPN" && b.op == "MODIFY_UP_SESSION" &&
		strings.Contains(b.inputs, "qos_flows_delta")
}

func (b invokeBinding) validateQoSDelta(wire map[string]string) bool {
	for name, pattern := range qosDeltaPatterns {
		value, exists := wire[name]
		if !exists || !pattern.MatchString(value) {
			return false
		}
	}
	parse := func(name string) (uint64, bool) {
		value, err := strconv.ParseUint(wire[name], 10, 64)
		return value, err == nil
	}
	gfbrUL, ok := parse("gfbr_ul")
	if !ok {
		return false
	}
	gfbrDL, ok := parse("gfbr_dl")
	if !ok {
		return false
	}
	mfbrUL, ok := parse("mfbr_ul")
	if !ok {
		return false
	}
	mfbrDL, ok := parse("mfbr_dl")
	if !ok {
		return false
	}
	if gfbrUL == 0 || gfbrDL == 0 || mfbrUL == 0 || mfbrDL == 0 {
		return false
	}
	return gfbrUL <= mfbrUL && gfbrDL <= mfbrDL
}

func (b invokeBinding) wireInputs(raw []byte, scope HostInvocation) (map[string]string, error) {
	decoded, err := decodeUniqueJSON(raw)
	args, ok := decoded.(map[string]any)
	if err != nil || !ok || args["ue_identity"] != scope.SUPI {
		return nil, errInvokeInput
	}
	delete(args, "ue_identity")
	if b.step != "s2" {
		if args["psi"] != json.Number(strconv.Itoa(scope.PSI)) {
			return nil, errInvokeInput
		}
		delete(args, "psi")
	}

	wire := make(map[string]string)
	for _, field := range strings.Fields(b.inputs) {
		name, target, hasTarget := strings.Cut(field, "=")
		if !hasTarget {
			target = name
		}
		target, constant, _ := strings.Cut(target, ":")
		switch name {
		case "ue_identity":
			wire[target] = scope.SUPI
		case "psi":
			wire[target] = strconv.Itoa(scope.PSI)
		case "sNssai":
			snssai, ok := args[name].(map[string]any)
			if !ok || !flattenSlice(snssai, wire) {
				return nil, errInvokeInput
			}
			delete(args, name)
		case "trigger":
			// The declared argument type is integer; CA-SL passes the numeric
			// trigger through the compiled Procedure input.
			number, ok := args[name].(json.Number)
			if !ok {
				return nil, errInvokeInput
			}
			trigger, err := strconv.Atoi(number.String())
			if err != nil || trigger < 1 || trigger > 7 {
				return nil, errInvokeInput
			}
			wire[target] = strconv.Itoa(trigger)
			delete(args, name)
		default:
			value, ok := args[name].(string)
			if !ok || (constant != "" && value != constant) {
				return nil, errInvokeInput
			}
			// The original 5GSM NAS payload is an opaque hex string; every other
			// scalar is a bounded reference.
			if name == "n1_sm" {
				if !wireHex(value, 1024) {
					return nil, errInvokeInput
				}
			} else if !wireScalar(value) {
				return nil, errInvokeInput
			}
			wire[target] = value
			delete(args, name)
		}
	}
	if len(args) != 0 {
		return nil, errInvokeInput
	}
	if b.isQoSDelta() && !b.validateQoSDelta(wire) {
		return nil, errInvokeInput
	}
	return wire, nil
}

func wireScalar(value string) bool {
	return value != "" && len(value) <= 255 && !strings.ContainsRune(value, 0)
}

// wireHex accepts an opaque, even-length hexadecimal payload up to max chars.
func wireHex(value string, max int) bool {
	if value == "" || len(value) > max || len(value)%2 != 0 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdefABCDEF", r)
	}) < 0
}

func flattenSlice(slice map[string]any, wire map[string]string) bool {
	sst, ok := slice["sst"].(json.Number)
	if !ok {
		return false
	}
	number, err := strconv.ParseUint(sst.String(), 10, 8)
	if err != nil {
		return false
	}
	wire["s_nssai"] = strconv.FormatUint(number, 10)
	if value, exists := slice["sd"]; exists {
		sd, ok := value.(string)
		if !ok || len(sd) != 6 || strings.IndexFunc(sd, func(r rune) bool {
			return !strings.ContainsRune("0123456789abcdefABCDEF", r)
		}) >= 0 {
			return false
		}
		wire["s_nssai_sd"] = strings.ToLower(sd)
		return len(slice) == 2
	}
	return len(slice) == 1
}

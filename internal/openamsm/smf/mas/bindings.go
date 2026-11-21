package mas

import (
	"encoding/json"
	"errors"
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
	"SSM.SIGNAL_UE_AN":             {"s10", "SSM", "SIGNAL_UE_AN", "sessionKeys=session_keys ueAddrAllocated=ue_addr n4Seid=n4_seid", "an_tunnel an_tunnel_teid:uint an_tunnel_ip:ip"},
	"UPN.MODIFY_UP_SESSION":        {"s11", "UPN", "MODIFY_UP_SESSION", "anTunnelTeid=an_tunnel_teid anTunnelIp=an_tunnel_ip n4Seid=n4_seid", "n3_synced:bit n4_seid:uint?"},
	"SE.EMIT_EVENTS":               {"s13", "SE", "EMIT_EVENTS", "event=ev:ESTABLISHED sessionKeys=session_keys", "last_event:1"},
	"SC.UPDATE_SM_CONTEXT":         {"m0", "SC", "UPDATE_SM_CONTEXT", "", "lifecycle:OPERATIONAL"},
	"UPN.MODIFY_UP_SESSION_INTENT": {"m3", "UPN", "MODIFY_UP_SESSION", "upIntent=up_intent:1", "n4_seid"},
	"SSM.SIGNAL_UE_AN_MODIFY":      {"m4", "SSM", "SIGNAL_UE_AN", "", "an_tunnel an_tunnel_teid:uint an_tunnel_ip:ip"},
	"UPN.DELETE_UP_SESSION":        {"r2", "UPN", "DELETE_UP_SESSION", "trigger", "released:1"},
	"SSM.SIGNAL_UE_AN_RELEASE":     {"r3", "SSM", "SIGNAL_UE_AN", "release=release:1 trigger", ""},
	"PA.DELETE_SM_POLICY":          {"r4", "PA", "DELETE_SM_POLICY", "release=release:1", ""},
	"SC.RELEASE_SM_CONTEXT":        {"r5", "SC", "RELEASE_SM_CONTEXT", "release=release:1", "lifecycle:RELEASING"},
	"SE.EMIT_RELEASE_EVENT":        {"r6", "SE", "EMIT_EVENTS", "ev=ev:RELEASED", ""},
}

var errInvokeInput = errors.New("invalid OpenAMSM MAS operation inputs or scope")

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
			if !ok || !wireScalar(value) || (constant != "" && value != constant) {
				return nil, errInvokeInput
			}
			wire[target] = value
			delete(args, name)
		}
	}
	if len(args) != 0 {
		return nil, errInvokeInput
	}
	return wire, nil
}

func wireScalar(value string) bool {
	return value != "" && len(value) <= 255 && !strings.ContainsRune(value, 0)
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

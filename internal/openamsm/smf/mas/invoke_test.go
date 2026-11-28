package mas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var prototypeScope = HostInvocation{
	Version: HostInvocationVersion, DeploymentID: "deployment", OwnerInstanceID: "instance",
	OriginalInvocationID: "original", SessionRef: "session", SUPI: "imsi-00101", PSI: 5,
}

const s1Args = `{"ue_identity":"imsi-00101","psi":5,"dnn":"internet","sNssai":{"sst":1,"sd":"ABCDEF"}}`

// schemeCCallables are the Scheme C UE Modify/Release occurrences the merged
// OpenAMSM Owner performs. They are acceptance additions with a fixed
// mas/op/stepId mapping and closed projections, but their Owner ToolCatalog
// schemas are not part of the v0.3.0 candidate source map yet, so they must be
// declared here explicitly instead of silently widening the reviewed table.
var schemeCCallables = map[string]bool{
	"SSM.PARSE_N1_MODIFY":       true,
	"UPN.MODIFY_QOS_DELTA":      true,
	"SSM.PARSE_N1_RELEASE":      true,
	"SSM.BUILD_RELEASE_COMMAND": true,
	"SSM.RELEASE_CONFIRM_N1":    true,
	"SSM.RELEASE_CONFIRM_N2":    true,
	"PA.DELETE_SM_POLICY_C":     true,
	"SC.RELEASE_SM_CONTEXT_C":   true,
	"SE.EMIT_RELEASE_EVENT_C":   true,
}

func TestSchemeCWiringIsClosed(t *testing.T) {
	scope := prototypeScope
	tests := []struct {
		callable string
		args     string
		wire     map[string]string
	}{
		{
			"SSM.PARSE_N1_MODIFY",
			`{"ue_identity":"imsi-00101","psi":5,"parse_n1":"1","n1_sm":"2e0501c9"}`,
			map[string]string{"parse_n1": "1", "n1_sm": "2e0501c9"},
		},
		{
			"UPN.MODIFY_QOS_DELTA",
			`{"ue_identity":"imsi-00101","psi":5,"qos_flows_delta":"1","qfi":"1","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
			map[string]string{"qos_flows_delta": "1", "qfi": "1", "five_qi": "1", "gfbr_ul": "100000",
				"gfbr_dl": "100000", "mfbr_ul": "200000", "mfbr_dl": "200000"},
		},
		{
			"SSM.PARSE_N1_RELEASE",
			`{"ue_identity":"imsi-00101","psi":5,"parse_n1":"1","n1_sm":"2e0501d1"}`,
			map[string]string{"parse_n1": "1", "n1_sm": "2e0501d1"},
		},
		{
			"SSM.BUILD_RELEASE_COMMAND",
			`{"ue_identity":"imsi-00101","psi":5,"release":"1","mode":"build","trigger":2,"release_txn":"txn-1","pti":"7"}`,
			map[string]string{"release": "1", "mode": "build", "trigger": "2", "release_txn": "txn-1", "pti": "7"},
		},
		{
			"SSM.RELEASE_CONFIRM_N1",
			`{"ue_identity":"imsi-00101","psi":5,"release_confirm":"n1","release_txn":"txn-1","release_generation":"1"}`,
			map[string]string{"release_confirm": "n1", "release_txn": "txn-1", "release_generation": "1"},
		},
		{
			"SSM.RELEASE_CONFIRM_N2",
			`{"ue_identity":"imsi-00101","psi":5,"release_confirm":"n2","release_txn":"txn-1","release_generation":"1"}`,
			map[string]string{"release_confirm": "n2", "release_txn": "txn-1", "release_generation": "1"},
		},
		{
			"PA.DELETE_SM_POLICY_C",
			`{"ue_identity":"imsi-00101","psi":5,"release":"1","trigger":2}`,
			map[string]string{"release": "1", "trigger": "2"},
		},
		{
			"SE.EMIT_RELEASE_EVENT_C",
			`{"ue_identity":"imsi-00101","psi":5,"ev":"RELEASED"}`,
			map[string]string{"ev": "RELEASED"},
		},
		{
			"SC.RELEASE_SM_CONTEXT_C",
			`{"ue_identity":"imsi-00101","psi":5,"release":"1","finalize":"1","trigger":2}`,
			map[string]string{"release": "1", "finalize": "1", "trigger": "2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.callable, func(t *testing.T) {
			if !schemeCCallables[tt.callable] {
				t.Fatal("test callable is not declared as a Scheme C occurrence")
			}
			binding := invokeBindings[tt.callable]
			wire, err := binding.wireInputs([]byte(tt.args), scope)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(wire, tt.wire) {
				t.Fatalf("wire projection drifted: %v", wire)
			}
			for name, value := range tt.wire {
				if name == "release_txn" && !wireScalar(value) {
					t.Fatal("release transaction is not a bounded scalar")
				}
			}
			// The Host-private scope fields are never forwarded as Owner inputs.
			if _, ok := wire["ue_identity"]; ok {
				t.Fatal("SUPI leaked into Owner inputs")
			}
			if _, ok := wire["psi"]; ok {
				t.Fatal("PSI leaked into Owner inputs")
			}
		})
	}
}

func TestInvokeCandidateCoverage(t *testing.T) {
	const root = "../../../../contracts/mas-owner-candidate/v0.3.0/"
	var source struct {
		Operations []struct {
			StepID, MAS, Op, PublicCallable, InputSchemaFile string
			ArgumentMapping                                  []struct {
				From      string
				To        json.RawMessage
				ScopeOnly bool
			}
		}
	}
	load := func(path string, target any) {
		t.Helper()
		raw, err := os.ReadFile(root + path)
		if err != nil && strings.HasPrefix(path, "schemas/") {
			raw, err = os.ReadFile("../../../../contracts/mas-owner-candidate/v0.2.0/schemas/" + strings.TrimPrefix(path, "schemas/"))
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	load("source-map.json", &source)
	reviewed := make(map[string]bool, len(source.Operations))
	for _, op := range source.Operations {
		reviewed[op.PublicCallable] = true
		binding, ok := invokeBindings[op.PublicCallable]
		if !ok {
			t.Fatalf("reviewed callable %q has no binding", op.PublicCallable)
		}
		if binding.step != op.StepID || binding.mas != op.MAS || binding.op != op.Op {
			t.Fatalf("binding %q drifted from the reviewed source map", op.PublicCallable)
		}
	}
	for callable := range invokeBindings {
		if reviewed[callable] || schemeCCallables[callable] {
			continue
		}
		t.Fatalf("binding %q is neither reviewed nor a declared Scheme C occurrence", callable)
	}
	for _, op := range source.Operations {
		t.Run(op.StepID, func(t *testing.T) {
			var schema struct {
				Properties map[string]struct {
					Type  string
					Const json.RawMessage
					Enum  []json.RawMessage
				}
			}
			load("schemas/"+op.InputSchemaFile, &schema)
			args := make(map[string]any)
			for name, field := range schema.Properties {
				switch name {
				case "ue_identity":
					args[name] = prototypeScope.SUPI
				case "psi":
					args[name] = prototypeScope.PSI
				case "sNssai":
					args[name] = map[string]any{"sst": json.Number("1"), "sd": "ABCDEF"}
				default:
					var value any
					literal := field.Const
					if len(literal) == 0 && len(field.Enum) > 0 {
						literal = field.Enum[0]
					}
					if len(literal) > 0 {
						decoder := json.NewDecoder(strings.NewReader(string(literal)))
						decoder.UseNumber()
						if err := decoder.Decode(&value); err != nil {
							t.Fatal(err)
						}
					} else {
						switch field.Type {
						case "integer", "number":
							value = json.Number("1")
						case "boolean":
							value = true
						case "object":
							value = map[string]any{}
						default:
							value = qosPrototypeValue(name)
						}
					}
					args[name] = value
				}
			}
			raw, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			occurrence, _, body, err := prepareInvoke(prototypeScope, op.PublicCallable, raw)
			if err != nil {
				t.Fatal(err)
			}
			var request invokeRequest
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if request.StepID != op.StepID || request.MAS != op.MAS || request.Op != op.Op ||
				request.SessionRef != prototypeScope.SessionRef || request.IdempotencyKey != occurrence.IdempotencyKey {
				t.Fatal("wire envelope drift")
			}
			expected := make(map[string]string)
			for _, mapping := range op.ArgumentMapping {
				if mapping.ScopeOnly {
					continue
				}
				if mapping.From == "sNssai" {
					expected["s_nssai"], expected["s_nssai_sd"] = "1", "abcdef"
					continue
				}
				var target string
				if err := json.Unmarshal(mapping.To, &target); err != nil {
					t.Fatal(err)
				}
				expected[target] = fmt.Sprint(args[mapping.From])
			}
			if !reflect.DeepEqual(request.Inputs, expected) {
				t.Fatalf("wire input mismatch: got %v want %v", request.Inputs, expected)
			}
		})
	}
}

// qosPrototypeValue supplies schema-valid canonical values for the typed QoS
// delta; every other string field keeps an opaque placeholder.
func qosPrototypeValue(name string) string {
	switch name {
	case "qfi":
		return "7"
	case "fiveQi":
		return "9"
	case "gfbrUl", "gfbrDl":
		return "100000"
	case "mfbrUl", "mfbrDl":
		return "200000"
	default:
		return "example"
	}
}

func TestQoSDeltaInputRejectsIllegalQuantities(t *testing.T) {
	valid := map[string]any{
		"ue_identity":   prototypeScope.SUPI,
		"psi":           prototypeScope.PSI,
		"anTunnelTeid":  "456",
		"anTunnelIp":    "10.45.0.2",
		"n4Seid":        "123",
		"qosFlowsDelta": "1",
		"qfi":           "7",
		"fiveQi":        "9",
		"gfbrUl":        "100000",
		"gfbrDl":        "100000",
		"mfbrUl":        "200000",
		"mfbrDl":        "200000",
	}
	encode := func(mutate func(map[string]any)) []byte {
		copy := make(map[string]any, len(valid))
		for key, value := range valid {
			copy[key] = value
		}
		mutate(copy)
		raw, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if _, _, _, err := prepareInvoke(prototypeScope, "UPN.MODIFY_QOS_FLOWS", encode(func(map[string]any) {})); err != nil {
		t.Fatalf("canonical QoS delta rejected: %v", err)
	}
	cases := map[string]func(map[string]any){
		"qfi zero":             func(args map[string]any) { args["qfi"] = "0" },
		"qfi too large":        func(args map[string]any) { args["qfi"] = "64" },
		"qfi leading zero":     func(args map[string]any) { args["qfi"] = "07" },
		"five_qi not numeric":  func(args map[string]any) { args["fiveQi"] = "abc" },
		"five_qi zero":         func(args map[string]any) { args["fiveQi"] = "0" },
		"five_qi leading zero": func(args map[string]any) { args["fiveQi"] = "09" },
		"bitrate zero":         func(args map[string]any) { args["gfbrUl"] = "0" },
		"bitrate leading zero": func(args map[string]any) { args["gfbrUl"] = "0100000" },
		"bitrate overflow":     func(args map[string]any) { args["mfbrDl"] = "10000000000000" },
		"gbr above mbr UL":     func(args map[string]any) { args["gfbrUl"] = "300000" },
		"gbr above mbr DL":     func(args map[string]any) { args["gfbrDl"] = "300000" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := prepareInvoke(prototypeScope, "UPN.MODIFY_QOS_FLOWS", encode(mutate)); err == nil {
				t.Fatal("illegal QoS delta accepted")
			}
		})
	}
}

type testJournal struct {
	mu       sync.Mutex
	claims   map[string]Occurrence
	decision Decision
}

func (j *testJournal) Claim(_ context.Context, o Occurrence) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, claim := range j.claims {
		if claim.SessionRef == o.SessionRef && (claim.SUPI != o.SUPI || claim.PSI != o.PSI) {
			return false, errors.New("session binding conflict")
		}
	}
	if existing, ok := j.claims[o.OriginalInvocationID]; ok {
		if existing != o {
			return false, errors.New("occurrence conflict")
		}
		return false, nil
	}
	j.claims[o.OriginalInvocationID] = o
	return true, nil
}
func (j *testJournal) RecordObservation(context.Context, Occurrence, Observation) error { return nil }
func (j *testJournal) ReadDecision(context.Context, Occurrence) (Decision, error) {
	return j.decision, nil
}

func TestInvokeOnceAndReconcile(t *testing.T) {
	var calls atomic.Int32
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != invokePath || r.Header.Get("X-OpenAMSM-Orch-Token") != "orch-secret" ||
			r.Header.Get("X-OpenAMSM-Query-Token") != "" {
			t.Error("incorrect private transport")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"mas":"SC","op":"VALIDATE_SUBSCRIPTION","stepId":"s1","status":"OK","outputs":{"validated":"1","sub_summary":"ok","secondary_auth_required":"0"}}`)
	}))
	defer server.Close()
	journal := &testJournal{claims: map[string]Occurrence{}}
	newAdapter := func() *InvokePrototype {
		adapter, err := NewInvokePrototype(server.URL, "orch-secret", time.Second, journal)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(adapter.CloseIdleConnections)
		return adapter
	}
	adapter := newAdapter()
	ctx := context.Background()
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			out, err := adapter.Invoke(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
			if err != nil || out.CommitState != Unknown || out.OriginalInvocationID != prototypeScope.OriginalInvocationID {
				t.Errorf("unexpected duplicate outcome: %v %v", out, err)
			}
		})
	}
	group.Wait()
	adapter = newAdapter()
	if out, err := adapter.Invoke(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args)); err != nil || out.CommitState != Unknown {
		t.Fatal("restart must retain the claim")
	}
	changed := strings.Replace(s1Args, "internet", "other", 1)
	if _, err := adapter.Invoke(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(changed)); err == nil {
		t.Fatal("accepted same occurrence with changed input")
	}
	other := prototypeScope
	other.SUPI, other.OriginalInvocationID = "imsi-00202", "different"
	if _, err := adapter.Invoke(ctx, other, "SC.VALIDATE_SUBSCRIPTION", []byte(strings.Replace(s1Args, prototypeScope.SUPI, other.SUPI, 1))); err == nil {
		t.Fatal("accepted cross-subscriber sessionRef")
	}
	original := journal.claims[prototypeScope.OriginalInvocationID]
	for _, state := range []CommitState{Deferred, Committed, Denied, NotCommitted} {
		journal.decision = Decision{original, state, "owner-proof"}
		out, err := adapter.Reconcile(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
		if err != nil || out.CommitState != state || out.ProofRef != "owner-proof" {
			t.Fatalf("wrong reconcile state: %v %v", out, err)
		}
	}
	journal.decision.Occurrence.OriginalInvocationID = "another-occurrence"
	out, _ := adapter.Reconcile(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
	if out.CommitState != Unknown || calls.Load() != 1 {
		t.Fatal("accepted foreign proof or redispatched")
	}
}

func TestInvokeRejectsInvalidInputs(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		strings.Replace(s1Args, "imsi-00101", "imsi-00202", 1),
		strings.Replace(s1Args, `"psi":5`, `"psi":6`, 1),
		strings.Replace(s1Args, `"psi":5`, `"psi":5,"psi":5`, 1),
		strings.Replace(s1Args, `"sst":1`, `"sst":1,"extra":0`, 1),
		strings.Replace(s1Args, `"dnn":"internet"`, `"dnn":{"value":"internet"}`, 1),
		strings.TrimSuffix(s1Args, "}") + `,"op":"COMPENSATE_RELEASE"}`,
		strings.TrimSuffix(s1Args, "}") + `,"sessionRef":"forged"}`,
	} {
		if _, _, _, err := prepareInvoke(prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(raw)); err == nil {
			t.Fatal("accepted invalid arguments")
		}
	}
	if _, _, _, err := prepareInvoke(prototypeScope, "SC.COMPENSATE_RELEASE", []byte(s1Args)); err == nil {
		t.Fatal("accepted compensation binding")
	}
}

func TestLifecycleInvokeInputValidation(t *testing.T) {
	valid := []struct {
		callable string
		args     string
	}{
		{"SC.UPDATE_SM_CONTEXT", `{"ue_identity":"imsi-00101","psi":5}`},
		{"UPN.MODIFY_UP_SESSION_INTENT", `{"ue_identity":"imsi-00101","psi":5,"upIntent":"1"}`},
		{"SSM.SIGNAL_UE_AN_MODIFY", `{"ue_identity":"imsi-00101","psi":5}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5,"trigger":1}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5,"trigger":7}`},
		{"SSM.SIGNAL_UE_AN_RELEASE", `{"ue_identity":"imsi-00101","psi":5,"release":"1","trigger":6}`},
		{"PA.DELETE_SM_POLICY", `{"ue_identity":"imsi-00101","psi":5,"release":"1"}`},
		{"SC.RELEASE_SM_CONTEXT", `{"ue_identity":"imsi-00101","psi":5,"release":"1"}`},
		{"SE.EMIT_RELEASE_EVENT", `{"ue_identity":"imsi-00101","psi":5,"ev":"RELEASED"}`},
	}
	for _, tt := range valid {
		t.Run("valid/"+tt.callable, func(t *testing.T) {
			if _, _, _, err := prepareInvoke(prototypeScope, tt.callable, []byte(tt.args)); err != nil {
				t.Fatalf("valid source profile rejected: %v", err)
			}
		})
	}

	invalid := []struct {
		callable string
		args     string
	}{
		{"UPN.MODIFY_UP_SESSION_INTENT", `{"ue_identity":"imsi-00101","psi":5,"upIntent":"0"}`},
		{"UPN.MODIFY_UP_SESSION_INTENT", `{"ue_identity":"imsi-00101","psi":5,"upIntent":"1","extra":"x"}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5,"trigger":0}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5,"trigger":8}`},
		{"UPN.DELETE_UP_SESSION", `{"ue_identity":"imsi-00101","psi":5,"trigger":"1"}`},
		{"SSM.SIGNAL_UE_AN_RELEASE", `{"ue_identity":"imsi-00101","psi":5,"release":"0","trigger":6}`},
		{"SSM.SIGNAL_UE_AN_RELEASE", `{"ue_identity":"imsi-00101","psi":5,"release":"1","trigger":8}`},
		{"PA.DELETE_SM_POLICY", `{"ue_identity":"imsi-00101","psi":5,"release":"0"}`},
		{"SC.RELEASE_SM_CONTEXT", `{"ue_identity":"imsi-00101","psi":5,"release":"1","extra":"x"}`},
		{"SE.EMIT_RELEASE_EVENT", `{"ue_identity":"imsi-00101","psi":5,"ev":"ESTABLISHED"}`},
	}
	for _, tt := range invalid {
		t.Run("invalid/"+tt.callable, func(t *testing.T) {
			if _, _, _, err := prepareInvoke(prototypeScope, tt.callable, []byte(tt.args)); err == nil {
				t.Fatal("invalid or out-of-contract lifecycle input accepted")
			}
		})
	}
}

func TestInvokeWireOutcomes(t *testing.T) {
	for _, tt := range []struct {
		status      int
		cause, code string
		state       CommitState
	}{
		{403, "ACL", "ACL_DENIED", Denied},
		{403, "ORCH_FAILED", "OWNER_ERROR", Unknown},
		{404, "SESSION_ASSOC_NOT_FOUND", "OWNER_ERROR", Unknown},
		{422, "E_COMPAT", "OWNER_ERROR", Unknown},
		{502, "E_SOUTHBOUND", "OWNER_ERROR", Unknown},
		{504, "E_TIMEOUT", "OWNER_TIMEOUT", Unknown},
		{200, "", "INVALID_RESPONSE", Unknown},
	} {
		t.Run(fmt.Sprintf("%d-%s", tt.status, tt.cause), func(t *testing.T) {
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(tt.status)
				fmt.Fprintf(w, `{"status":%d,"cause":%q,"detail":"orch-secret imsi-00101"}`, tt.status, tt.cause)
			}))
			defer server.Close()
			adapter, err := NewInvokePrototype(server.URL, "orch-secret", time.Second, &testJournal{claims: map[string]Occurrence{}})
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.CloseIdleConnections()
			out, err := adapter.Invoke(context.Background(), prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
			if err != nil || out.CommitState != tt.state || out.Observation.Code != tt.code ||
				strings.Contains(fmt.Sprint(out, err), "orch-secret") || strings.Contains(fmt.Sprint(out, err), "imsi-00101") {
				t.Fatalf("unexpected outcome: %v %v", out, err)
			}
		})
	}
}

func TestInvokeDeadlineAndCancellation(t *testing.T) {
	var count atomic.Int32
	started := make(chan struct{}, 1)
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	for _, timed := range []bool{true, false} {
		timeout := time.Second
		if timed {
			timeout = 150 * time.Millisecond
		}
		adapter, err := NewInvokePrototype(server.URL, "orch-secret", timeout, &testJournal{claims: map[string]Occurrence{}})
		if err != nil {
			t.Fatal(err)
		}
		defer adapter.CloseIdleConnections()
		ctx, cancel := context.WithCancel(context.Background())
		if !timed {
			go func() { <-started; cancel() }()
		}
		out, err := adapter.Invoke(ctx, prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
		cancel()
		code := "CANCELED"
		if timed {
			code = "TIMEOUT"
			<-started
		}
		if err != nil || out.CommitState != Unknown || out.Observation.Code != code {
			t.Fatalf("unexpected interrupted call: %v %v", out, err)
		}
		_, _ = adapter.Invoke(context.Background(), prototypeScope, "SC.VALIDATE_SUBSCRIPTION", []byte(s1Args))
	}
	if count.Load() != 2 {
		t.Fatal("cancellation or timeout redispatched")
	}
}

func TestInvokeOutputValidation(t *testing.T) {
	header := http.Header{"Content-Type": {"application/json"}}
	vectors := map[string]string{
		"SC.VALIDATE_SUBSCRIPTION":     `{"validated":"1","sub_summary":"ok","secondary_auth_required":"0"}`,
		"SAA.SECONDARY_AUTH":           `{"phase":"SKIPPED"}`,
		"SC.CREATE_SM_CONTEXT":         `{"session_keys":"sess","lifecycle":"RESERVED","sm_context_ref":"123"}`,
		"SC.REGISTER_UECM":             `{"uecm_registered":"1"}`,
		"PA.CREATE_SM_POLICY":          `{"up_intent":"1","authorized_qos":"1","policy_assoc_id":"bound"}`,
		"UPS.DECIDE_ANCHOR":            `{"up_anchor":"1"}`,
		"IAM.ALLOCATE_ADDRESS":         `{"ue_addr":"1"}`,
		"PA.UPDATE_SM_POLICY":          `{"up_intent":"1","authorized_qos":"1","policy_assoc_id":"bound"}`,
		"CA.CREATE_CHARGING":           `{"charging_ref":"1"}`,
		"UPN.CREATE_UP_SESSION":        `{"n4_seid":"1234","est_done":"1"}`,
		"SSM.SIGNAL_UE_AN":             `{"an_tunnel":"1","an_tunnel_teid":"42","an_tunnel_ip":"192.0.2.1","ran_teid":"42"}`,
		"UPN.MODIFY_UP_SESSION":        `{"n3_synced":"1","n4_seid":"1234"}`,
		"UPN.MODIFY_QOS_FLOWS":         `{"qos_delta_state":"APPLIED","n1n2_dispatched":"1","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000","n4_seid":"500"}`,
		"SE.EMIT_EVENTS":               `{"last_event":"1"}`,
		"SC.UPDATE_SM_CONTEXT":         `{"lifecycle":"OPERATIONAL","sm_context_ref":"123"}`,
		"UPN.MODIFY_UP_SESSION_INTENT": `{"n4_seid":"1234"}`,
		"SSM.SIGNAL_UE_AN_MODIFY":      `{"an_tunnel":"1","an_tunnel_teid":"42","an_tunnel_ip":"192.0.2.1"}`,
		"UPN.DELETE_UP_SESSION":        `{"released":"1"}`,
		"SSM.SIGNAL_UE_AN_RELEASE":     `{}`,
		"PA.DELETE_SM_POLICY":          `{}`,
		"SC.RELEASE_SM_CONTEXT":        `{"lifecycle":"RELEASING"}`,
		"SE.EMIT_RELEASE_EVENT":        `{}`,
		"SSM.PARSE_N1_MODIFY":          `{"qfi":"1","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000","requested_kind":"qos_flow_descriptions"}`,
		"UPN.MODIFY_QOS_DELTA":         `{"qos_delta_state":"APPLIED","n1n2_dispatched":"1","qfi":"1","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"SSM.PARSE_N1_RELEASE":         `{"psi":"5","pti":"1","cause":"26","release_intent":"ue_requested"}`,
		"SSM.BUILD_RELEASE_COMMAND":    `{"n1_sm_hex":"2e0501d3","n2_sm_hex":"00112233","n2_sm_info_type":"PDU_RES_REL_CMD","release_generation":"1","release_txn":"txn-1","release_phase":"command_built"}`,
		"SSM.RELEASE_CONFIRM_N1":       `{"release_ready":"0","n1_released":"1","n2_released":"0"}`,
		"SSM.RELEASE_CONFIRM_N2":       `{"release_ready":"1","n1_released":"1","n2_released":"1"}`,
		"PA.DELETE_SM_POLICY_C":        `{"terminated":"1"}`,
		"SC.RELEASE_SM_CONTEXT_C":      `{"lifecycle":"RELEASING","user_plane_ready":"0"}`,
		"SE.EMIT_RELEASE_EVENT_C":      `{"last_event":"1"}`,
	}
	for name, binding := range invokeBindings {
		t.Run(name, func(t *testing.T) {
			envelope := func(outputs string) []byte {
				return []byte(fmt.Sprintf(`{"status":"OK","stepId":%q,"mas":%q,"op":%q,"outputs":%s}`, binding.step, binding.mas, binding.op, outputs))
			}
			good := vectors[name]
			projected, ref, ok := binding.project(header, envelope(good))
			if !ok {
				t.Fatal("source output rejected")
			}
			if name == "SC.CREATE_SM_CONTEXT" && (projected["sm_context_ref"] != "" || ref != "123") {
				t.Fatal("SM context reference was not isolated")
			}
			bad := strings.TrimSuffix(good, "}") + `,"unexpected":0}`
			if good == `{}` {
				bad = `{"unexpected":0}`
			} else {
				if _, _, ok := binding.project(header, envelope(`{}`)); ok {
					t.Fatal("missing required output accepted")
				}
			}
			if _, _, ok := binding.project(header, envelope(bad)); ok {
				t.Fatal("unexpected output accepted")
			}
		})
	}
}

func TestQoSProjectionKeepsPublicTargetAndDispatchRules(t *testing.T) {
	header := http.Header{"Content-Type": {"application/json"}}
	binding := invokeBindings["UPN.MODIFY_QOS_FLOWS"]
	envelope := func(outputs string) []byte {
		return []byte(fmt.Sprintf(`{"status":"OK","stepId":%q,"mas":%q,"op":%q,"outputs":%s}`,
			binding.step, binding.mas, binding.op, outputs))
	}
	applied := `{"qos_delta_state":"APPLIED","n1n2_dispatched":"1","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000","n4_seid":"500"}`
	outputs, _, ok := binding.project(header, envelope(applied))
	if !ok {
		t.Fatal("valid APPLIED result rejected")
	}
	if outputs["n4_seid"] != "500" {
		t.Fatal("declared public n4_seid was dropped for the QoS callable")
	}
	if outputs["five_qi"] != "1" || outputs["gfbr_ul"] != "100000" {
		t.Fatal("declared target projection drift")
	}

	bad := map[string]string{
		"applied without dispatch": `{"qos_delta_state":"APPLIED","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"already with dispatch":    `{"qos_delta_state":"ALREADY","n1n2_dispatched":"1","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"unknown state":            `{"qos_delta_state":"UNKNOWN","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"qfi leading zero":         `{"qos_delta_state":"ALREADY","qfi":"02","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"bitrate leading zero":     `{"qos_delta_state":"ALREADY","qfi":"2","five_qi":"1","gfbr_ul":"0100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"five_qi out of range":     `{"qos_delta_state":"ALREADY","qfi":"2","five_qi":"256","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`,
		"undeclared extra output":  `{"qos_delta_state":"ALREADY","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000","session_ref":"x"}`,
		"missing target":           `{"qos_delta_state":"ALREADY","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000"}`,
	}
	for name, outputs := range bad {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := binding.project(header, envelope(outputs)); ok {
				t.Fatal("invalid QoS result accepted")
			}
		})
	}

	already := `{"qos_delta_state":"ALREADY","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`
	if _, _, ok := binding.project(header, envelope(already)); !ok {
		t.Fatal("valid ALREADY result rejected")
	}
	failed := `{"qos_delta_state":"PFCP_APPLIED_N1N2_FAILED","qfi":"2","five_qi":"1","gfbr_ul":"100000","gfbr_dl":"100000","mfbr_ul":"200000","mfbr_dl":"200000"}`
	if _, _, ok := binding.project(header, envelope(failed)); !ok {
		t.Fatal("declared N1/N2 failure observation rejected")
	}

	legacy := invokeBindings["UPN.MODIFY_UP_SESSION"]
	legacyEnvelope := []byte(fmt.Sprintf(`{"status":"OK","stepId":%q,"mas":%q,"op":%q,"outputs":{"n3_synced":"1","n4_seid":"1234"}}`,
		legacy.step, legacy.mas, legacy.op))
	if projected, _, ok := legacy.project(header, legacyEnvelope); !ok || projected["n4_seid"] != "" {
		t.Fatal("legacy s11 private n4_seid handling regressed")
	}
}

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

func TestInvokeCandidateCoverage(t *testing.T) {
	const root = "../../../../contracts/mas-owner-candidate/v0.3.0/"
	var source struct {
		Operations []struct {
			StepID, MAS, Op, PublicCallable, InputSchemaFile string
			ArgumentMapping []struct {
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
	if len(source.Operations) != len(invokeBindings) {
		t.Fatal("incomplete bindings")
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
							value = "example"
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
		fmt.Fprint(w, `{"mas":"SC","op":"VALIDATE_SUBSCRIPTION","stepId":"s1","status":"OK","outputs":{"sub_summary":"ok","secondary_auth_required":"0"}}`)
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
		"SC.VALIDATE_SUBSCRIPTION": `{"sub_summary":"ok","secondary_auth_required":"0"}`,
		"SAA.SECONDARY_AUTH":       `{"phase":"SKIPPED"}`,
		"SC.CREATE_SM_CONTEXT":     `{"session_keys":"sess","lifecycle":"RESERVED","sm_context_ref":"123"}`,
		"SC.REGISTER_UECM":         `{"uecm_registered":"1"}`,
		"PA.CREATE_SM_POLICY":      `{"up_intent":"1","authorized_qos":"1","policy_assoc_id":"bound"}`,
		"UPS.DECIDE_ANCHOR":        `{"up_anchor":"1"}`,
		"IAM.ALLOCATE_ADDRESS":     `{"ue_addr":"1"}`,
		"PA.UPDATE_SM_POLICY":      `{"up_intent":"1","authorized_qos":"1","policy_assoc_id":"bound"}`,
		"CA.CREATE_CHARGING":       `{"charging_ref":"1"}`,
		"UPN.CREATE_UP_SESSION":    `{"n4_seid":"1234","est_done":"1"}`,
		"SSM.SIGNAL_UE_AN":         `{"an_tunnel":"1","an_tunnel_teid":"42","an_tunnel_ip":"192.0.2.1"}`,
		"UPN.MODIFY_UP_SESSION":    `{"n3_synced":"1","n4_seid":"1234"}`,
		"SE.EMIT_EVENTS":                `{"last_event":"1"}`,
		"SC.UPDATE_SM_CONTEXT":          `{"lifecycle":"OPERATIONAL"}`,
		"UPN.MODIFY_UP_SESSION_INTENT":  `{"n4_seid":"1234"}`,
		"SSM.SIGNAL_UE_AN_MODIFY":       `{"an_tunnel":"1","an_tunnel_teid":"42","an_tunnel_ip":"192.0.2.1"}`,
		"UPN.DELETE_UP_SESSION":         `{"released":"1"}`,
		"SSM.SIGNAL_UE_AN_RELEASE":      `{}`,
		"PA.DELETE_SM_POLICY":           `{}`,
		"SC.RELEASE_SM_CONTEXT":         `{"lifecycle":"RELEASING"}`,
		"SE.EMIT_RELEASE_EVENT":         `{}`,
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
			for _, bad := range []string{`{}`, strings.TrimSuffix(good, "}") + `,"unexpected":0}`} {
				if _, _, ok := binding.project(header, envelope(bad)); ok {
					t.Fatal("unexpected output accepted")
				}
			}
		})
	}
}

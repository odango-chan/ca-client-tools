package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// newH2CServer mirrors the OpenAMSM SBI listener: cleartext HTTP/2 with prior
// knowledge. Metrics endpoints remain plain HTTP/1.1 and use httptest directly.
func newH2CServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	server.Start()
	return server
}

func testEnv(values map[string]string) lookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestHelpAndVersion(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantText string
	}{
		{name: "root help", args: nil, wantText: "Available Commands:"},
		{name: "leaf help", args: []string{"amf", "ue", "list", "--help"}, wantText: "--page-size"},
		{name: "version", args: []string{"--version"}, wantText: "ca-client-tools dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(), test.args, strings.NewReader(""),
				&stdout, &stderr, testEnv(nil),
			)
			if code != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), test.wantText) {
				t.Fatalf("stdout %q does not contain %q", stdout.String(), test.wantText)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
		})
	}
}

func TestVersionIncludesBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := Version, GitCommit, BuildDate
	defer func() {
		Version, GitCommit, BuildDate = oldVersion, oldCommit, oldBuildDate
	}()

	Version = "v1.0.0"
	GitCommit = "0123456789abcdef"
	BuildDate = "2026-11-18T08:00:00Z"

	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(), []string{"--version"}, strings.NewReader(""),
		&stdout, &stderr, testEnv(nil),
	)
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	want := "ca-client-tools v1.0.0 (commit 0123456789abcdef; built 2026-11-18T08:00:00Z)\n"
	if stdout.String() != want {
		t.Fatalf("version output = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestSMFContextQueryReadsJSONFromStdinAndWrapsSuccess(t *testing.T) {
	var receivedBody map[string]any
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nsmf-pa-context/v1/query" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-OpenAMSM-Query-Token") != "cli-test-token" {
			t.Error("trusted query token was not injected")
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"PA","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"bound":false}}`)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(),
		[]string{"smf", "context", "query", "--type", "policy"},
		strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
		&stdout,
		&stderr,
		testEnv(map[string]string{
			"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
			"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
		}),
	)
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if receivedBody["supi"] != "imsi-001010000000001" || receivedBody["pduSessionId"] != float64(5) {
		t.Fatalf("unexpected request body: %#v", receivedBody)
	}
	if strings.Contains(strings.Join([]string{"smf", "context", "query", "--type", "policy"}, " "), "imsi-") {
		t.Fatal("subscriber identity must not be passed in argv")
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode success envelope: %v", err)
	}
	if envelope.Data["status"] != "AVAILABLE" {
		t.Fatalf("unexpected data envelope: %#v", envelope.Data)
	}
	if envelope.Data["type"] != "policy" {
		t.Fatalf("unexpected public context type: %#v", envelope.Data["type"])
	}
	if _, leaked := envelope.Data["masId"]; leaked {
		t.Fatalf("internal OpenAMSM identifier leaked into public output: %#v", envelope.Data)
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("success output must end with a newline: %q", stdout.String())
	}
}

// TestSMFContextQueryRoutesAllEightTypesFromTheCLI verifies the complete public
// type tree end to end: known SUPI + PSI, no list lookup, exact private route.
func TestSMFContextQueryRoutesAllEightTypesFromTheCLI(t *testing.T) {
	cases := []struct {
		publicType string
		masID      string
		path       string
	}{
		{"session", "SC", "/nsmf-sc-context/v1/query"},
		{"policy", "PA", "/nsmf-pa-context/v1/query"},
		{"up-node", "UPN", "/nsmf-upn-context/v1/query"},
		{"signaling-session", "SSM", "/nsmf-ssm-context/v1/query"},
		{"ip-address", "IAM", "/nsmf-iam-context/v1/query"},
		{"up-selection", "UPS", "/nsmf-ups-context/v1/query"},
		{"charging", "CA", "/nsmf-ca-context/v1/query"},
		{"session-auth", "SAA", "/nsmf-saa-context/v1/query"},
	}
	for _, test := range cases {
		t.Run(test.publicType, func(t *testing.T) {
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != test.path {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"masId":%q,"status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"contextType":%q}}`,
					test.masID, test.publicType)
			}))
			defer server.Close()

			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(),
				[]string{"smf", "context", "query", "--type", test.publicType},
				strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
				&stdout, &stderr,
				testEnv(map[string]string{
					"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
					"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
				}),
			)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), `"type":"`+test.publicType+`"`) ||
				!strings.Contains(stdout.String(), `"status":"AVAILABLE"`) {
				t.Fatalf("unexpected output: %s", stdout.String())
			}
			if strings.Contains(stdout.String(), `"masId"`) {
				t.Fatalf("internal route identifier leaked: %s", stdout.String())
			}
		})
	}
}

func TestSMFContextQueryRejectsCrossUEAndCrossSessionResponses(t *testing.T) {
	cases := []struct {
		name     string
		response string
	}{
		{
			name:     "UE-B response for UE-A request",
			response: `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000002","pduSessionId":5},"context":{"lifecycle":"OPERATIONAL"}}`,
		},
		{
			name:     "different PDU Session on same UE",
			response: `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":6},"context":{"lifecycle":"OPERATIONAL"}}`,
		},
		{
			name:     "UNAVAILABLE with another UE identity",
			response: `{"masId":"SC","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000002","pduSessionId":5},"context":{}}`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, test.response)
			}))
			defer server.Close()

			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(),
				[]string{"smf", "context", "query", "--type", "session"},
				strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
				&stdout, &stderr,
				testEnv(map[string]string{
					"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
					"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
				}),
			)
			if code != exitInternal {
				t.Fatalf("exit=%d, want %d: stdout=%s stderr=%s", code, exitInternal, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("protocol mismatch wrote to stdout: %s", stdout.String())
			}
			var envelope errorEnvelope
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != "OPENAMSM_PROTOCOL_INVALID" {
				t.Fatalf("unexpected error: %#v", envelope.Error)
			}
			if strings.Contains(stderr.String(), "imsi-") ||
				strings.Contains(stderr.String(), "001010000000002") {
				t.Fatalf("identity leaked to stderr: %s", stderr.String())
			}
		})
	}
}

func TestSMFContextQueryIMSIAndIPInputs(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantBody    map[string]any
		responseKey string
		publicType  string
		masID       string
	}{
		{
			name:        "IMSI plus PSI",
			input:       `{"imsi":"001010000000001","pduSessionId":5}`,
			wantBody:    map[string]any{"imsi": "001010000000001", "pduSessionId": float64(5)},
			responseKey: `{"supi":"imsi-001010000000001","pduSessionId":5}`,
			publicType:  "session",
			masID:       "SC",
		},
		{
			name:        "IPv4 only",
			input:       `{"ueIpv4":"10.45.0.2"}`,
			wantBody:    map[string]any{"ueIpv4": "10.45.0.2"},
			responseKey: `{"ueIpv4":"10.45.0.2"}`,
			publicType:  "ip-address",
			masID:       "IAM",
		},
		{
			name:        "IPv6 only",
			input:       `{"ueIpv6":"2001:db8::1"}`,
			wantBody:    map[string]any{"ueIpv6": "2001:db8::1"},
			responseKey: `{"ueIpv6":"2001:db8::1"}`,
			publicType:  "ip-address",
			masID:       "IAM",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var received map[string]any
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"masId":%q,"status":"AVAILABLE","key":%s,"context":{}}`, test.masID, test.responseKey)
			}))
			defer server.Close()

			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(),
				[]string{"smf", "context", "query", "--type", test.publicType},
				strings.NewReader(test.input),
				&stdout, &stderr,
				testEnv(map[string]string{
					"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
					"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
				}),
			)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if len(received) != len(test.wantBody) {
				t.Fatalf("request body = %#v, want %#v", received, test.wantBody)
			}
			for field, value := range test.wantBody {
				if received[field] != value {
					t.Errorf("request %s = %#v, want %#v", field, received[field], value)
				}
			}
		})
	}
}

func TestSMFContextTransportFailureIsBoundedAndRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	origin := server.URL
	server.Close()

	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(),
		[]string{"smf", "context", "query", "--type", "session"},
		strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
		&stdout, &stderr,
		testEnv(map[string]string{
			"CA_OPENAMSM_SMF_SBI_ORIGIN": origin,
			"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
		}),
	)
	if code != exitRemote {
		t.Fatalf("exit=%d, want %d: stdout=%s stderr=%s", code, exitRemote, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("transport error wrote to stdout: %s", stdout.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != "OPENAMSM_TRANSPORT_ERROR" {
		t.Fatalf("unexpected error: %#v", envelope.Error)
	}
	if strings.Contains(stderr.String(), "imsi-") || strings.Contains(stderr.String(), "cli-test-token") {
		t.Fatalf("sensitive data leaked to stderr: %s", stderr.String())
	}
}

func TestSMFContextTypeMappingUsesOnlyPublicTypes(t *testing.T) {
	want := map[string]string{
		"session":           "SC",
		"policy":            "PA",
		"up-node":           "UPN",
		"signaling-session": "SSM",
		"ip-address":        "IAM",
		"up-selection":      "UPS",
		"charging":          "CA",
		"session-auth":      "SAA",
	}
	if len(openAMSMContextTypeID) != len(want) {
		t.Fatalf("mapping has %d entries, want %d", len(openAMSMContextTypeID), len(want))
	}
	for publicType, wireID := range want {
		query, err := readSMFContextQuery(
			strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
			publicType,
		)
		if err != nil {
			t.Fatalf("type %q: %v", publicType, err)
		}
		if query.MAS != wireID {
			t.Errorf("type %q mapped to %q, want %q", publicType, query.MAS, wireID)
		}
	}
	for _, internalID := range []string{"SC", "PA", "UPN", "SSM", "IAM", "UPS", "CA", "SAA"} {
		if _, err := readSMFContextQuery(strings.NewReader(`{}`), internalID); err == nil {
			t.Errorf("internal wire identifier %q was accepted as a public type", internalID)
		}
	}
}

func TestSMFContextUnavailableRemainsSuccessfulData(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"UPN","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(),
		[]string{"smf", "context", "query", "--type", "up-node"},
		strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
		&stdout,
		&stderr,
		testEnv(map[string]string{
			"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
			"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
		}),
	)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status":"UNAVAILABLE"`) {
		t.Fatalf("UNAVAILABLE was not preserved in data output: %s", stdout.String())
	}
}

func TestSMFContextInputRejectsInvalidJSONBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	valid := `{"supi":"imsi-001010000000001","pduSessionId":5}`
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: []byte("")},
		{name: "malformed", input: []byte(`{"supi":`)},
		{name: "invalid UTF-8", input: []byte{0xff, 0xfe}},
		{name: "duplicate key", input: []byte(`{"supi":"imsi-001010000000001","supi":"imsi-001010000000002","pduSessionId":5}`)},
		{name: "unknown field", input: []byte(`{"supi":"imsi-001010000000001","pduSessionId":5,"extra":true}`)},
		{name: "trailing JSON", input: []byte(valid + ` {"imsi":"001010000000001"}`)},
		{name: "root array", input: []byte(`[{"supi":"imsi-001010000000001","pduSessionId":5}]`)},
		{name: "wrong field type", input: []byte(`{"supi":5,"pduSessionId":5}`)},
		{name: "non-integer PSI", input: []byte(`{"supi":"imsi-001010000000001","pduSessionId":5.0}`)},
		{name: "oversize", input: bytes.Repeat([]byte(" "), maxContextInputBytes+1)},
		{name: "missing query key", input: []byte(`{}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(),
				[]string{"smf", "context", "query", "--type", "session"},
				bytes.NewReader(test.input),
				&stdout,
				&stderr,
				testEnv(map[string]string{
					"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
					"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
				}),
			)
			if code != exitInvalid {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid input wrote to stdout: %s", stdout.String())
			}
			var envelope errorEnvelope
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != "INVALID_ARGUMENT" {
				t.Fatalf("unexpected error: %#v", envelope.Error)
			}
			if strings.Contains(stderr.String(), "imsi-") {
				t.Fatalf("subscriber identity leaked to stderr: %s", stderr.String())
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid stdin reached the remote endpoint %d times", requests.Load())
	}
}

func TestSMFContext404RemainsCLIError(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"status":404,"cause":"SESSION_NOT_FOUND","detail":"private diagnostic"}`)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(),
		[]string{"smf", "context", "query", "--type", "session"},
		strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
		&stdout,
		&stderr,
		testEnv(map[string]string{
			"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
			"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
		}),
	)
	if code != exitRemote {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("remote error wrote to stdout: %s", stdout.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != "OPENAMSM_HTTP_ERROR" ||
		envelope.Error.HTTPStatus != http.StatusNotFound ||
		envelope.Error.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected error envelope: %#v", envelope.Error)
	}
	if strings.Contains(stderr.String(), "private diagnostic") ||
		strings.Contains(stderr.String(), "imsi-") {
		t.Fatalf("private data leaked to stderr: %s", stderr.String())
	}
}

func TestMetricsCommandsUseResourceActionTreeAndDataEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "10" {
			t.Errorf("unexpected pagination query: %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/ue-info":
		case "/gnb-info":
		case "/pdu-info":
		default:
			t.Errorf("unexpected metrics path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"state":"active"}],"pager":{"page":2,"page_size":10,"count":1,"prev":"/`+strings.TrimPrefix(r.URL.Path, "/")+`?page=1"}}`)
	}))
	defer server.Close()

	cases := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{
			name: "AMF UE",
			args: []string{"amf", "ue", "list", "--page", "2", "--page-size", "10"},
			env:  map[string]string{"CA_OPENAMSM_AMF_METRICS_ORIGIN": server.URL},
		},
		{
			name: "AMF gNB",
			args: []string{"amf", "gnb", "list", "--page", "2", "--page-size", "10"},
			env:  map[string]string{"CA_OPENAMSM_AMF_METRICS_ORIGIN": server.URL},
		},
		{
			name: "SMF PDU",
			args: []string{"smf", "pdu", "list", "--page", "2", "--page-size", "10"},
			env:  map[string]string{"CA_OPENAMSM_SMF_METRICS_ORIGIN": server.URL},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(), test.args, strings.NewReader(""),
				&stdout, &stderr, testEnv(test.env),
			)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), `"data"`) ||
				!strings.Contains(stdout.String(), `"state":"active"`) ||
				!strings.Contains(stdout.String(), `"page_size":10`) {
				t.Fatalf("unexpected data envelope: %s", stdout.String())
			}
		})
	}
}

func TestUnknownCommandsFlagsAndDuplicateSingletonFlagsAreJSONErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid command/flags must fail before network access")
	}))
	defer server.Close()

	cases := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "unknown command", args: []string{"shell", "-c", "anything"}},
		{name: "unknown flag", args: []string{"amf", "ue", "list", "--unknown"}, env: map[string]string{"CA_OPENAMSM_AMF_METRICS_ORIGIN": server.URL}},
		{name: "duplicate page flag", args: []string{"amf", "ue", "list", "--page", "1", "--page", "2"}, env: map[string]string{"CA_OPENAMSM_AMF_METRICS_ORIGIN": server.URL}},
		{name: "extra positional argument", args: []string{"smf", "pdu", "list", "unexpected"}, env: map[string]string{"CA_OPENAMSM_SMF_METRICS_ORIGIN": server.URL}},
		{name: "missing required type flag", args: []string{"smf", "context", "query"}, env: map[string]string{"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL, "CA_OPENAMSM_QUERY_TOKEN": "token"}},
		{name: "unknown context type", args: []string{"smf", "context", "query", "--type", "mas"}, env: map[string]string{"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL, "CA_OPENAMSM_QUERY_TOKEN": "token"}},
		{name: "unknown context type without configuration", args: []string{"smf", "context", "query", "--type", "mas"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithContext(
				context.Background(), test.args, strings.NewReader(""),
				&stdout, &stderr, testEnv(test.env),
			)
			if code != exitInvalid || stdout.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			var envelope errorEnvelope
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != "INVALID_ARGUMENT" {
				t.Fatalf("unexpected error: %#v", envelope.Error)
			}
		})
	}
}

func TestMissingConfigurationUsesStableErrorEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithContext(
		context.Background(),
		[]string{"amf", "ue", "list"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		testEnv(nil),
	)
	if code != exitConfiguration || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != "CONFIGURATION_MISSING" {
		t.Fatalf("unexpected error: %#v", envelope.Error)
	}
}

func TestCancellationDiscardsPartialOutputAndEmitsNoEnvelope(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runWithContext(
			ctx,
			[]string{"smf", "context", "query", "--type", "session"},
			strings.NewReader(`{"supi":"imsi-001010000000001","pduSessionId":5}`),
			&stdout,
			&stderr,
			testEnv(map[string]string{
				"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
				"CA_OPENAMSM_QUERY_TOKEN":    "cli-test-token",
			}),
		)
	}()
	<-started
	cancel()
	code := <-done
	close(release)
	if code != 130 {
		t.Fatalf("exit=%d, want signal-style cancellation status 130", code)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("cancellation emitted output: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

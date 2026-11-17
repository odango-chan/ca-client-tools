package mas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// newH2CServer mirrors the OpenAMSM SBI listener: cleartext HTTP/2 with prior
// knowledge. A plain HTTP/1.1 test server would not exercise the real protocol.
func newH2CServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	server.Start()
	return server
}

func newTestClient(t *testing.T, origin string) *Client {
	t.Helper()
	client, err := NewClient(Config{Origin: origin, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func intPtr(value int) *int { return &value }

func queryKeyFor(t *testing.T, query Query) queryKey {
	t.Helper()
	key, err := query.validate()
	if err != nil {
		t.Fatalf("validate query: %v", err)
	}
	return key
}

func decodeResult(t *testing.T, raw string, masID string, query Query) (Result, error) {
	t.Helper()
	decoded, err := decodeUniqueJSON([]byte(raw))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("fixture is not an object")
	}
	return parseResult(root, masID, queryKeyFor(t, query))
}

// TestEveryContextTypeUsesItsExactOpenAMSMRoute pins the eight request
// mappings. The route identifier is adapter-private but must be exact.
func TestEveryContextTypeUsesItsExactOpenAMSMRoute(t *testing.T) {
	cases := []struct {
		masID string
		path  string
	}{
		{"SC", "/nsmf-sc-context/v1/query"},
		{"PA", "/nsmf-pa-context/v1/query"},
		{"UPN", "/nsmf-upn-context/v1/query"},
		{"SSM", "/nsmf-ssm-context/v1/query"},
		{"IAM", "/nsmf-iam-context/v1/query"},
		{"UPS", "/nsmf-ups-context/v1/query"},
		{"CA", "/nsmf-ca-context/v1/query"},
		{"SAA", "/nsmf-saa-context/v1/query"},
	}
	for _, test := range cases {
		t.Run(test.masID, func(t *testing.T) {
			var requests atomic.Int32
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.ProtoMajor != 2 {
					t.Errorf("protocol = %s, want cleartext HTTP/2", r.Proto)
				}
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != test.path {
					t.Errorf("path = %s, want %s", r.URL.Path, test.path)
				}
				if got := r.Header.Get(queryTokenHeader); got != "test-secret" {
					t.Errorf("query token = %q", got)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q", got)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if body["supi"] != "imsi-001010000000001" || body["pduSessionId"] != float64(5) {
					t.Errorf("request body = %#v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"masId":%q,"status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"marker":%q}}`,
					test.masID, test.masID)
			}))
			defer server.Close()

			result, err := newTestClient(t, server.URL).Query(context.Background(), Query{
				MAS: test.masID, SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if requests.Load() != 1 {
				t.Fatalf("request count = %d, want 1", requests.Load())
			}
			if result.MASID != test.masID || result.Status != "AVAILABLE" {
				t.Fatalf("unexpected result: %#v", result)
			}
			if result.Context["marker"] != test.masID {
				t.Fatalf("context not preserved: %#v", result.Context)
			}
		})
	}
}

func TestKnownSUPIAndPSIQueryDirectlyWithoutListLookup(t *testing.T) {
	var requests atomic.Int32
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body) != 2 || body["supi"] != "imsi-001010000000001" || body["pduSessionId"] != float64(5) {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"lifecycle":"OPERATIONAL","dnn":"internet"}}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL).Query(context.Background(), Query{
		MAS: "sc", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requests.Load())
	}
	if result.Status != "AVAILABLE" || result.Context["lifecycle"] != "OPERATIONAL" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestKnownIMSIAndPSIQueryDirectly(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["imsi"] != "001010000000001" || body["pduSessionId"] != float64(5) {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL).Query(context.Background(), Query{
		MAS: "SC", IMSI: "001010000000001", PDUSessionID: intPtr(5),
	})
	if err != nil {
		t.Fatalf("IMSI lookup must accept the echoed SUPI: %v", err)
	}
	if result.Key["supi"] != "imsi-001010000000001" {
		t.Fatalf("unexpected key: %#v", result.Key)
	}
}

func TestQuerySupportsIPv4IPv6AndJointLookups(t *testing.T) {
	cases := []struct {
		name  string
		query Query
		key   string
	}{
		{
			name:  "IPv4 only",
			query: Query{MAS: "IAM", UEIPv4: "10.45.0.2"},
			key:   `{"ueIpv4":"10.45.0.2"}`,
		},
		{
			name:  "IPv6 only",
			query: Query{MAS: "IAM", UEIPv6: "2001:db8::1"},
			key:   `{"ueIpv6":"2001:db8::1"}`,
		},
		{
			name: "identity plus both IPs",
			query: Query{
				MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
				UEIPv4: "10.45.0.2", UEIPv6: "2001:db8::1",
			},
			key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"2001:db8::1"}`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var body map[string]any
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"masId":"%s","status":"AVAILABLE","key":%s,"context":{}}`, test.query.MAS, test.key)
			}))
			defer server.Close()

			if _, err := newTestClient(t, server.URL).Query(context.Background(), test.query); err != nil {
				t.Fatalf("query: %v", err)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(test.key), &want); err != nil {
				t.Fatal(err)
			}
			if len(body) != len(want) {
				t.Fatalf("request body = %#v, want %#v", body, want)
			}
			for field, value := range want {
				if body[field] != value {
					t.Errorf("request %s = %#v, want %#v", field, body[field], value)
				}
			}
		})
	}
}

func TestInvalidQueryKeysFailBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL)

	tests := []struct {
		name  string
		query Query
	}{
		{name: "missing key", query: Query{MAS: "SC"}},
		{name: "identity without PSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001"}},
		{name: "IMSI without PSI", query: Query{MAS: "SC", IMSI: "001010000000001"}},
		{name: "inconsistent SUPI and IMSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001", IMSI: "001010000000002", PDUSessionID: intPtr(5)}},
		{name: "PSI without identity", query: Query{MAS: "SC", PDUSessionID: intPtr(5), UEIPv4: "10.45.0.2"}},
		{name: "negative PSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(-1)}},
		{name: "PSI above 255", query: Query{MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(256)}},
		{name: "invalid SUPI", query: Query{MAS: "SC", SUPI: "imsi-abc", PDUSessionID: intPtr(5)}},
		{name: "SUPI without imsi prefix", query: Query{MAS: "SC", SUPI: "001010000000001", PDUSessionID: intPtr(5)}},
		{name: "IMSI longer than 15 digits", query: Query{MAS: "SC", IMSI: "1234567890123456", PDUSessionID: intPtr(5)}},
		{name: "IMSI with non-digit", query: Query{MAS: "SC", IMSI: "00101abc", PDUSessionID: intPtr(5)}},
		{name: "unknown MAS", query: Query{MAS: "XYZ", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5)}},
		{name: "invalid IPv4", query: Query{MAS: "IAM", UEIPv4: "not-an-ip"}},
		{name: "IPv6 in IPv4 field", query: Query{MAS: "IAM", UEIPv4: "2001:db8::1"}},
		{name: "invalid IPv6", query: Query{MAS: "IAM", UEIPv6: "not-an-ip"}},
		{name: "IPv4 in IPv6 field", query: Query{MAS: "IAM", UEIPv6: "10.45.0.2"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.Query(context.Background(), test.query)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v, want InputError", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid requests reached server: %d", requests.Load())
	}
}

// TestResponseIdentityMismatchIsAProtocolError covers the critical
// cross-UE / cross-session case: UE-A is requested but a well-formed UE-B
// response is returned. It must be rejected, never reported as AVAILABLE,
// UNAVAILABLE or a transport failure.
func TestResponseIdentityMismatchIsAProtocolError(t *testing.T) {
	request := Query{MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5)}
	cases := []struct {
		name string
		key  string
	}{
		{name: "other UE same PSI", key: `{"supi":"imsi-001010000000002","pduSessionId":5}`},
		{name: "other PSI same UE", key: `{"supi":"imsi-001010000000001","pduSessionId":6}`},
		{name: "missing SUPI", key: `{"pduSessionId":5}`},
		{name: "missing PSI", key: `{"supi":"imsi-001010000000001"}`},
		{name: "IMSI echoed instead of SUPI", key: `{"imsi":"001010000000001","pduSessionId":5}`},
		{name: "extra IP field", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2"}`},
		{name: "SUPI as number", key: `{"supi":1,"pduSessionId":5}`},
		{name: "PSI as string", key: `{"supi":"imsi-001010000000001","pduSessionId":"5"}`},
		{name: "PSI out of range", key: `{"supi":"imsi-001010000000001","pduSessionId":256}`},
		{name: "PSI not an integer", key: `{"supi":"imsi-001010000000001","pduSessionId":5.5}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeResult(t,
				`{"masId":"SC","status":"AVAILABLE","key":`+test.key+`,"context":{"lifecycle":"OPERATIONAL"}}`,
				"SC", request)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
			if strings.Contains(err.Error(), "001010000000002") ||
				strings.Contains(err.Error(), "imsi-") {
				t.Fatalf("identity leaked in error: %v", err)
			}
		})
	}
}

func TestUNAVAILABLEWithMismatchedIdentityIsAlsoRejected(t *testing.T) {
	request := Query{MAS: "UPN", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5)}
	_, err := decodeResult(t,
		`{"masId":"UPN","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000002","pduSessionId":5},"context":{}}`,
		"UPN", request)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error = %v, want ProtocolError", err)
	}
}

func TestJointIPLookupResponseBinding(t *testing.T) {
	request := Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
		UEIPv4: "10.45.0.2", UEIPv6: "2001:db8::1",
	}
	cases := []struct {
		name string
		key  string
		want bool
	}{
		{name: "matching key", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"2001:db8::1"}`, want: true},
		{name: "equivalent expanded IPv6", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"2001:0db8:0000:0000:0000:0000:0000:0001"}`, want: true},
		{name: "other IPv4", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.3","ueIpv6":"2001:db8::1"}`},
		{name: "other IPv6", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"2001:db8::2"}`},
		{name: "missing IPv4", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv6":"2001:db8::1"}`},
		{name: "IPv4 in IPv6 field", key: `{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"10.45.0.2"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeResult(t,
				`{"masId":"SC","status":"AVAILABLE","key":`+test.key+`,"context":{}}`,
				"SC", request)
			if test.want {
				if err != nil {
					t.Fatalf("query: %v", err)
				}
				return
			}
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
		})
	}
}

func TestIPOnlyResponseBinding(t *testing.T) {
	request := Query{MAS: "IAM", UEIPv4: "10.45.0.2"}
	cases := []struct {
		name string
		key  string
		ok   bool
	}{
		{name: "matching IPv4", key: `{"ueIpv4":"10.45.0.2"}`, ok: true},
		{name: "other IPv4", key: `{"ueIpv4":"10.45.0.9"}`},
		{name: "missing IPv4", key: `{"ueIpv6":"2001:db8::1"}`},
		{name: "extra SUPI", key: `{"ueIpv4":"10.45.0.2","supi":"imsi-001010000000001"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeResult(t,
				`{"masId":"IAM","status":"AVAILABLE","key":`+test.key+`,"context":{}}`,
				"IAM", request)
			if test.ok {
				if err != nil {
					t.Fatalf("query: %v", err)
				}
				return
			}
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
		})
	}
}

func TestUnavailableIsSuccessfulAndRequiresEmptyContext(t *testing.T) {
	request := Query{MAS: "UPN", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5)}
	result, err := decodeResult(t,
		`{"masId":"UPN","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`,
		"UPN", request)
	if err != nil {
		t.Fatalf("UNAVAILABLE is a valid query response: %v", err)
	}
	if result.Status != "UNAVAILABLE" || len(result.Context) != 0 {
		t.Fatalf("unexpected unavailable result: %#v", result)
	}

	_, err = decodeResult(t,
		`{"masId":"UPN","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"guessed":true}}`,
		"UPN", request)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("UNAVAILABLE with context: error = %v, want ProtocolError", err)
	}
}

func TestResponseEnvelopeValidation(t *testing.T) {
	request := Query{MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5)}
	cases := []struct {
		name string
		body string
	}{
		{name: "wrong MAS", body: `{"masId":"PA","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`},
		{name: "unknown status", body: `{"masId":"SC","status":"UNKNOWN","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`},
		{name: "empty key", body: `{"masId":"SC","status":"AVAILABLE","key":{},"context":{}}`},
		{name: "key is not an object", body: `{"masId":"SC","status":"AVAILABLE","key":"x","context":{}}`},
		{name: "context is not an object", body: `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":[]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeResult(t, test.body, "SC", request)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
		})
	}
}

func TestIMSIRequestRequiresEchoedSUPI(t *testing.T) {
	request := Query{MAS: "SC", IMSI: "001010000000001", PDUSessionID: intPtr(5)}
	_, err := decodeResult(t,
		`{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000002","pduSessionId":5},"context":{}}`,
		"SC", request)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error = %v, want ProtocolError", err)
	}
}

func TestRemoteProblemDetailsCauseIsPreserved(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"status":404,"cause":"SESSION_NOT_FOUND","detail":"private diagnostic text imsi-001010000000001"}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("error = %v, want RemoteError", err)
	}
	if remoteErr.HTTPStatus != http.StatusNotFound || remoteErr.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected remote error: %#v", remoteErr)
	}
	if strings.Contains(err.Error(), "private diagnostic") || strings.Contains(err.Error(), "imsi-") {
		t.Fatalf("remote error leaked private data: %v", err)
	}
}

func TestHTTPErrorIsNotUNAVAILABLE(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"masId":"SC","status":"UNAVAILABLE","key":{},"context":{}}`)
			}))
			defer server.Close()
			_, err := newTestClient(t, server.URL).Query(context.Background(), Query{
				MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
			})
			var remoteErr *RemoteError
			if !errors.As(err, &remoteErr) || remoteErr.HTTPStatus != status {
				t.Fatalf("error = %v, want RemoteError %d", err, status)
			}
		})
	}
}

func TestTimeoutIsReportedAsTransportFailure(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout was not bounded: %v", time.Since(start))
	}
	var remoteErr *RemoteError
	var protocolErr *ProtocolError
	if errors.As(err, &remoteErr) || errors.As(err, &protocolErr) {
		t.Fatalf("timeout must not be mapped to remote/protocol error: %v", err)
	}
	if strings.Contains(err.Error(), "imsi-") {
		t.Fatalf("identity leaked in timeout error: %v", err)
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var targetRequests atomic.Int32
	target := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()
	source := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/nsmf-sc-context/v1/query", http.StatusFound)
	}))
	defer source.Close()
	_, err := newTestClient(t, source.URL).Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.HTTPStatus != http.StatusFound {
		t.Fatalf("error = %v, want RemoteError 302", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target was called %d times", targetRequests.Load())
	}
}

func TestDuplicateJSONAndNonJSONSuccessAreRejected(t *testing.T) {
	for _, body := range []string{
		`{"masId":"SC","masId":"PA"}`,
		`{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}} trailing`,
	} {
		if _, err := decodeUniqueJSON([]byte(body)); err == nil {
			t.Fatalf("expected JSON rejection for %q", body)
		}
	}

	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()
	_, err := newTestClient(t, server.URL).Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error = %v, want ProtocolError", err)
	}
}

func TestResponseSizeLimitIsEnforced(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"pad":"`+strings.Repeat("x", 256)+`"}}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret", MaxResponseBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error = %v, want ProtocolError", err)
	}
}

func TestOriginConfigurationIsStrict(t *testing.T) {
	for _, origin := range []string{
		"", "not-a-url", "http://user:pass@example.test:7777",
		"http://example.test:7777/path", "http://example.test:7777?x=1",
		"http://example.test:7777#fragment", "ftp://example.test:7777",
	} {
		t.Run(origin, func(t *testing.T) {
			_, err := NewClient(Config{Origin: origin, QueryToken: "test-secret"})
			var configErr *ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error = %v, want ConfigurationError", err)
			}
		})
	}
}

func TestClientConfigurationIsStrict(t *testing.T) {
	cases := []Config{
		{Origin: "http://example.test:7777", QueryToken: ""},
		{Origin: "http://example.test:7777", QueryToken: " token"},
		{Origin: "http://example.test:7777", QueryToken: "tok\nen"},
		{Origin: "http://example.test:7777", QueryToken: "token", Timeout: -time.Second},
		{Origin: "http://example.test:7777", QueryToken: "token", Timeout: 31 * time.Second},
		{Origin: "http://example.test:7777", QueryToken: "token", MaxResponseBytes: -1},
		{Origin: "http://example.test:7777", QueryToken: "token", MaxResponseBytes: (4 << 20) + 1},
	}
	for index, cfg := range cases {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			_, err := NewClient(cfg)
			var configErr *ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error = %v, want ConfigurationError", err)
			}
		})
	}
}

func TestRemoteProblemDetailsCauseMustBeSafeMachineToken(t *testing.T) {
	for _, test := range []struct {
		cause string
		want  string
	}{
		{cause: "SESSION_NOT_FOUND", want: "SESSION_NOT_FOUND"},
		{cause: "private diagnostic with subscriber imsi-001", want: ""},
		{cause: "SESSION_NOT_FOUND\nsecret", want: ""},
		{cause: strings.Repeat("A", 65), want: ""},
	} {
		err := remoteError(http.StatusNotFound, []byte(`{"cause":`+strconv.Quote(test.cause)+`}`))
		var remote *RemoteError
		if !errors.As(err, &remote) {
			t.Fatalf("error = %v, want RemoteError", err)
		}
		if remote.Cause != test.want {
			t.Errorf("cause = %q, want %q", remote.Cause, test.want)
		}
	}
}

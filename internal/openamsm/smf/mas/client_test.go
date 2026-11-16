package mas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQueryBuildsExactRequestAndPreservesAvailableResult(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nsmf-sc-context/v1/query" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get(queryTokenHeader); got != "test-secret" {
			t.Errorf("query token = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["supi"] != "imsi-001010000000001" || body["pduSessionId"] != float64(5) {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"lifecycle":"OPERATIONAL","dnn":"internet"}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	result, err := client.Query(context.Background(), Query{
		MAS: "sc", SUPI: "imsi-001010000000001", PDUSessionID: &psi,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requestCount.Load())
	}
	if result.MASID != "SC" || result.Status != "AVAILABLE" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Context["lifecycle"] != "OPERATIONAL" {
		t.Fatalf("context not preserved: %#v", result.Context)
	}
}

func TestUnavailableIsSuccessfulAndRequiresEmptyContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"UPN","status":"UNAVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	result, err := client.Query(context.Background(), Query{
		MAS: "UPN", SUPI: "imsi-001010000000001", PDUSessionID: &psi,
	})
	if err != nil {
		t.Fatalf("UNAVAILABLE is a valid query response: %v", err)
	}
	if result.Status != "UNAVAILABLE" || len(result.Context) != 0 {
		t.Fatalf("unexpected unavailable result: %#v", result)
	}
}

func TestQuerySupportsIPv4Lookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["ueIpv4"] != "10.45.0.2" {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"IAM","status":"AVAILABLE","key":{"ueIpv4":"10.45.0.2"},"context":{"allocated":true,"ueAddr":"10.45.0.2","allocator":"pool"}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Query(context.Background(), Query{MAS: "IAM", UEIPv4: "10.45.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.MASID != "IAM" || result.Context["allocated"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestInvalidQueryKeysFailBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	psi256 := 256
	tests := []struct {
		name  string
		query Query
	}{
		{name: "missing key", query: Query{MAS: "SC"}},
		{name: "identity without PSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001"}},
		{name: "inconsistent SUPI and IMSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001", IMSI: "001010000000002", PDUSessionID: &psi}},
		{name: "PSI without identity", query: Query{MAS: "SC", PDUSessionID: &psi, UEIPv4: "10.45.0.2"}},
		{name: "invalid PSI", query: Query{MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: &psi256}},
		{name: "invalid SUPI", query: Query{MAS: "SC", SUPI: "imsi-abc", PDUSessionID: &psi}},
		{name: "unknown MAS", query: Query{MAS: "XYZ", SUPI: "imsi-001010000000001", PDUSessionID: &psi}},
		{name: "invalid IPv4", query: Query{MAS: "IAM", UEIPv4: "not-an-ip"}},
		{name: "invalid IPv6", query: Query{MAS: "IAM", UEIPv6: "not-an-ip"}},
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

func TestQueryAllowsConsistentIdentityAndBothIPKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nsmf-sc-context/v1/query" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["supi"] != "imsi-001010000000001" ||
			body["pduSessionId"] != float64(5) ||
			body["ueIpv4"] != "10.45.0.2" ||
			body["ueIpv6"] != "2001:db8::1" ||
			len(body) != 5 {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5,"ueIpv4":"10.45.0.2","ueIpv6":"2001:db8::1"},"context":{}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	result, err := client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", IMSI: "001010000000001",
		PDUSessionID: &psi, UEIPv4: "10.45.0.2", UEIPv6: "2001:db8::1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "AVAILABLE" {
		t.Fatalf("unexpected result: %#v", result)
	}
}


func TestQueryAcceptsOpenAMSMIMSIUpToFifteenDigits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["imsi"] != "12345" || body["pduSessionId"] != float64(5) || len(body) != 2 {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-12345","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	result, err := client.Query(context.Background(), Query{
		MAS: "SC", IMSI: "12345", PDUSessionID: &psi,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "AVAILABLE" || result.Key["supi"] != "imsi-12345" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestIMSIRejectsMoreThanFifteenDigits(t *testing.T) {
	psi := 5
	_, err := (Query{
		MAS: "SC", IMSI: "1234567890123456", PDUSessionID: &psi,
	}).requestBody()
	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
}

func TestRemoteProblemDetailsCauseIsPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"status":404,"cause":"SESSION_NOT_FOUND","detail":"private diagnostic text"}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	_, err = client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: &psi,
	})
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("error = %v, want RemoteError", err)
	}
	if remoteErr.HTTPStatus != http.StatusNotFound || remoteErr.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected remote error: %#v", remoteErr)
	}
	if strings.Contains(err.Error(), "private diagnostic text") {
		t.Fatal("remote error leaked ProblemDetails detail")
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/nsmf-sc-context/v1/query", http.StatusFound)
	}))
	defer source.Close()
	client, err := NewClient(Config{Origin: source.URL, QueryToken: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	psi := 5
	_, err = client.Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: &psi,
	})
	if err == nil {
		t.Fatal("expected redirect response to be rejected")
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target was called %d times", targetRequests.Load())
	}
}

func TestDuplicateJSONKeysAreRejected(t *testing.T) {
	_, err := decodeUniqueJSON([]byte(`{"masId":"SC","masId":"PA"}`))
	if err == nil {
		t.Fatal("expected duplicate JSON key rejection")
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

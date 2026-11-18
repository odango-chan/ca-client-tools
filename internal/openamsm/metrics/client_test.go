package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

func TestGetUEInfoPreservesSourceItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/ue-info" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("page") != "0" || r.URL.Query().Get("page_size") != "100" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"supi":"imsi-001010000000001","mm_state":"registered","cm_state":"connected"}],"pager":{"page":0,"page_size":100,"count":1}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{AMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetUEInfo(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Pager.Page != 0 || page.Pager.Count != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if !strings.Contains(string(page.Items[0]), `"mm_state":"registered"`) {
		t.Fatalf("source item was not preserved: %s", page.Items[0])
	}
}

func TestGetGNBInfoValidatesSequentialPagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/gnb-info" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "0" {
			items := strings.TrimSuffix(strings.Repeat(`{"gnb_id":100},`, 100), ",")
			_, _ = fmt.Fprintf(w, `{"items":[%s],"pager":{"page":0,"page_size":100,"count":100,"next":"/gnb-info?page=1"}}`, items)
			return
		}
		_, _ = fmt.Fprint(w, `{"items":[{"gnb_id":101}],"pager":{"page":1,"page_size":100,"count":1,"prev":"/gnb-info?page=0"}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{AMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.GetGNBInfo(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Pager.Next != "/gnb-info?page=1" || len(first.Items) != 100 {
		t.Fatalf("unexpected first page: %#v", first.Pager)
	}
	second, err := client.GetGNBInfo(context.Background(), 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if second.Pager.Prev != "/gnb-info?page=0" || len(second.Items) != 1 {
		t.Fatalf("unexpected second page: %#v", second.Pager)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestGetPDUInfoPreservesOpenAMSMShapeWithoutN4Pair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdu-info" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"supi":"imsi-001010000000001","pdu":[{"psi":5,"dnn":"internet","snssai":{"sst":1,"sd":"ffffff"},"qos_flows":[{"qfi":1,"5qi":9}],"n3":{"gnb":{"teid":76},"upf":{"teid":11426,"pdr_id":2}},"pdu_state":"active","ue_location_timestamp":1778223227627488}],"ue_activity":"active"}],"pager":{"page":0,"page_size":100,"count":1}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{SMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetPDUInfo(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || !strings.Contains(string(page.Items[0]), `"pdu_state":"active"`) {
		t.Fatalf("unexpected PDU page: %#v", page)
	}
	if strings.Contains(string(page.Items[0]), "smf_n4_seid") || strings.Contains(string(page.Items[0]), "upf_n4_seid") {
		t.Fatal("client fabricated an N4 SEID pair")
	}
}

func TestMetricsRejectsTruncatedAndDuplicateJSON(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":  `{"items":[],"pager":{"page":0,"page_size":100,"count":0,"truncated":true}}`,
		"duplicate":  `{"items":[],"items":[],"pager":{"page":0,"page_size":100,"count":0}}`,
		"wrong page": `{"items":[],"pager":{"page":1,"page_size":100,"count":0}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parsePage([]byte(body), "/ue-info", 0, 100)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want ProtocolError", err)
			}
		})
	}
}

func TestMetricsPageBoundsFailBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(Config{AMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ page, size int }{
		{-1, 100}, {0, 0}, {0, 101},
	} {
		_, err := client.GetUEInfo(context.Background(), tc.page, tc.size)
		var inputErr *InputError
		if !errors.As(err, &inputErr) {
			t.Fatalf("GetUEInfo(%d,%d) error = %v, want InputError", tc.page, tc.size, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid queries reached endpoint: %d", requests.Load())
	}
}

func TestMetricsRedirectIsNotFollowed(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/ue-info", http.StatusFound)
	}))
	defer source.Close()

	client, err := NewClient(Config{AMFOrigin: source.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetUEInfo(context.Background(), 0, 100)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.HTTPStatus != http.StatusFound {
		t.Fatalf("error = %v, want HTTP 302 RemoteError", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target was called %d times", targetRequests.Load())
	}
}

func TestMetricsOriginIsStrict(t *testing.T) {
	for _, origin := range []string{
		"", "not-a-url", "http://user:pass@example.test:9090",
		"http://example.test:9090/path", "http://example.test:9090?x=1",
		"ftp://example.test:9090",
	} {
		t.Run(origin, func(t *testing.T) {
			_, err := NewClient(Config{AMFOrigin: origin})
			var configErr *ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error = %v, want ConfigurationError", err)
			}
		})
	}
}

func sharedMetricsTransport(t *testing.T, server *httptest.Server, amfKey, smfKey string) *sbi.Client {
	t.Helper()
	services := map[string]sbi.ServiceConfig{}
	if amfKey != "" {
		services[amfKey] = sbi.ServiceConfig{Origin: server.URL, Profile: sbi.TransportMetricsHTTP1}
	}
	if smfKey != "" {
		services[smfKey] = sbi.ServiceConfig{Origin: server.URL, Profile: sbi.TransportMetricsHTTP1}
	}
	client, err := sbi.NewClient(sbi.Config{Services: services})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

// TestNewClientWithSBIUsesSharedTransport proves the Provider path: the typed
// metrics client works over the one shared transport instance instead of
// creating its own HTTP client.
func TestNewClientWithSBIUsesSharedTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ue-info" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[],"pager":{"page":0,"page_size":100,"count":0}}`)
	}))
	defer server.Close()
	transport := sharedMetricsTransport(t, server, AMFMetricsServiceKey, SMFMetricsServiceKey)
	client, err := NewClientWithSBI(transport, AMFMetricsServiceKey, SMFMetricsServiceKey, Config{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetUEInfo(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Pager.Page != 0 || page.Pager.Count != 0 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if profile, ok := transport.ServiceProfile(AMFMetricsServiceKey); !ok || profile != sbi.TransportMetricsHTTP1 {
		t.Fatalf("shared service profile = %q", profile)
	}
}

func TestNewClientWithSBIValidatesConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()
	h2cTransport, err := sbi.NewClient(sbi.Config{Services: map[string]sbi.ServiceConfig{
		"h2c-service": {Origin: "http://127.0.0.1:1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer h2cTransport.CloseIdleConnections()
	metricsTransport := sharedMetricsTransport(t, server, AMFMetricsServiceKey, "")

	cases := map[string]struct {
		transport *sbi.Client
		amf, smf  string
		cfg       Config
	}{
		"nil transport":      {nil, AMFMetricsServiceKey, "", Config{}},
		"no service keys":    {metricsTransport, "", "", Config{}},
		"unknown service":    {metricsTransport, "missing", "", Config{}},
		"wrong profile":      {h2cTransport, "h2c-service", "", Config{}},
		"invalid timeout":    {metricsTransport, AMFMetricsServiceKey, "", Config{Timeout: -time.Second}},
		"oversized response": {metricsTransport, AMFMetricsServiceKey, "", Config{MaxResponseBytes: maxAllowedResponse + 1}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewClientWithSBI(testCase.transport, testCase.amf, testCase.smf, testCase.cfg)
			var configErr *ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error = %v, want ConfigurationError", err)
			}
		})
	}
}

func TestMetricsResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[],"pager":{"page":0,"page_size":100,"count":0},"padding":"`+strings.Repeat("x", 256)+`"}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{AMFOrigin: server.URL, MaxResponseBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetUEInfo(context.Background(), 0, 100)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error = %v, want ProtocolError", err)
	}
}

func TestMetricsCancellationStopsRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(Config{AMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.GetUEInfo(ctx, 0, 100)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("request did not reach the metrics endpoint")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled metrics request returned no error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled metrics request did not stop")
	}
}

func TestMetricsTimeoutIsEnforced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(Config{AMFOrigin: server.URL, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := client.GetUEInfo(context.Background(), 0, 100); err == nil {
		t.Fatal("timed-out metrics request returned no error")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("metrics timeout was not applied: %v", elapsed)
	}
}

func TestMetricsReusesSharedConnection(t *testing.T) {
	var newConnections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[],"pager":{"page":0,"page_size":100,"count":0}}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client, err := NewClient(Config{AMFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for range 2 {
		if _, err := client.GetUEInfo(context.Background(), 0, 100); err != nil {
			t.Fatal(err)
		}
	}
	if got := newConnections.Load(); got != 1 {
		t.Fatalf("expected one reused metrics connection, got %d", got)
	}
}

func TestMetricsAllowsPageIndexesBeyondArbitraryLocalCap(t *testing.T) {
	const requestedPage = 1000
	page, err := parsePage([]byte(`{"items":[],"pager":{"page":1000,"page_size":10,"count":0,"prev":"/ue-info?page=999"}}`), "/ue-info", requestedPage, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Pager.Page != requestedPage || page.Pager.Prev != "/ue-info?page=999" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

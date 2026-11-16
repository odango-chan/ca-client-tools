package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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

func TestGetUPFSessionInfoPreservesExactN4AssociationAndRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/session-info" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("page") != "0" || r.URL.Query().Get("page_size") != "20" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"upf_n4_seid":"38105","smf_n4_f_seid":{"seid":"77123","ipv4":"10.45.0.1"},"ue_ip":{"ipv4":"10.45.0.2"},"dnn":"internet","pdrs":[{"id":1,"source_interface":"access","qfi":1,"far_id":1,"qer_id":1}],"fars":[{"id":1,"destination_interface":"core","outer_header_creation":{"gtpu_ipv4":true,"gtpu_ipv6":false,"teid":38105,"ipv4":"10.45.0.3"}}],"qers":[{"id":1,"qfi":1}],"urrs":[{"id":1,"usage":{"total_octets":"3","uplink_octets":"2","downlink_octets":"1"}}]}],"pager":{"page":0,"page_size":20,"count":1}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{UPFOrigin: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetUPFSessionInfo(context.Background(), 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("unexpected UPF session page: %#v", page)
	}
	item := string(page.Items[0])
	for _, field := range []string{`"upf_n4_seid":"38105"`, `"seid":"77123"`, `"pdrs"`, `"fars"`, `"qers"`, `"urrs"`} {
		if !strings.Contains(item, field) {
			t.Errorf("UPF source item lost %s: %s", field, item)
		}
	}
}

func TestGetUPFSessionInfoRequiresUPFOrigin(t *testing.T) {
	client, err := NewClient(Config{AMFOrigin: "http://amf.example:9090"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetUPFSessionInfo(context.Background(), 0, 20)
	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("error = %v, want ConfigurationError", err)
	}
}

func TestMetricsRejectsTruncatedAndDuplicateJSON(t *testing.T) {
	for name, body := range map[string]string{
		"truncated": `{"items":[],"pager":{"page":0,"page_size":100,"count":0,"truncated":true}}`,
		"duplicate": `{"items":[],"items":[],"pager":{"page":0,"page_size":100,"count":0}}`,
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

package mas

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// TestClientUsesSharedSBITransport proves the typed SMF MAS client no longer
// owns a private HTTP/2 implementation: every query goes through internal/sbi.
func TestClientUsesSharedSBITransport(t *testing.T) {
	server := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, ok := client.transport.(*sbiTransport); !ok {
		t.Fatalf("transport = %T, want *sbiTransport", client.transport)
	}
}

func TestClientRejectsHTTP1OnlyEndpoint(t *testing.T) {
	// The shared transport may see an HTTP/1.1 response from an endpoint that
	// does not speak h2c; it then rejects the downgrade instead of parsing a
	// non-HTTP/2 success response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).Query(context.Background(), Query{
		MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
	})
	if err == nil {
		t.Fatal("expected HTTP/1-only endpoint to be rejected")
	}
	var protocolErr *ProtocolError
	var remoteErr *RemoteError
	if errors.As(err, &protocolErr) || errors.As(err, &remoteErr) {
		t.Fatalf("protocol downgrade must be a transport failure, got %v", err)
	}
}

func TestClientReusesOneH2CConnection(t *testing.T) {
	var connections atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"SC","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{}}`)
	})
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	client := newTestClient(t, server.URL)
	for index := 0; index < 4; index++ {
		if _, err := client.Query(context.Background(), Query{
			MAS: "SC", SUPI: "imsi-001010000000001", PDUSessionID: intPtr(5),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("expected one reused h2c connection, got %d", got)
	}
}

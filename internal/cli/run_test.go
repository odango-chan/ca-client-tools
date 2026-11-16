package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunSMFMASQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nsmf-pa-context/v1/query" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-OpenAMSM-Query-Token") != "cli-test-token" {
			t.Errorf("query token not injected")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"masId":"PA","status":"AVAILABLE","key":{"supi":"imsi-001010000000001","pduSessionId":5},"context":{"bound":false}}`)
	}))
	defer server.Close()

	var stdout strings.Builder
	var stderr strings.Builder
	env := map[string]string{
		"CA_OPENAMSM_SMF_SBI_ORIGIN": server.URL,
		"CA_OPENAMSM_QUERY_TOKEN":     "cli-test-token",
	}
	code := runWithEnv([]string{
		"smf", "mas", "query",
		"--mas", "PA",
		"--supi", "imsi-001010000000001",
		"--pdu-session-id", "5",
	}, &stdout, &stderr, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status":"AVAILABLE"`) ||
		!strings.Contains(stdout.String(), `"bound":false`) {
		t.Fatalf("unexpected CLI output: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunRequiresTrustedConfiguration(t *testing.T) {
	var stdout strings.Builder
	code := runWithEnv([]string{"smf", "mas", "query", "--mas", "SC"},
		&stdout, &strings.Builder{}, func(string) (string, bool) { return "", false })
	if code != 3 || !strings.Contains(stdout.String(), "CONFIGURATION_MISSING") {
		t.Fatalf("exit=%d output=%s", code, stdout.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout strings.Builder
	code := runWithEnv([]string{"shell", "-c", "anything"},
		&stdout, &strings.Builder{}, func(string) (string, bool) { return "", false })
	if code != 2 || !strings.Contains(stdout.String(), "COMMAND_UNSUPPORTED") {
		t.Fatalf("exit=%d output=%s", code, stdout.String())
	}
}

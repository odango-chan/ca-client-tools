package serviceconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	return writeFile(t, t.TempDir(), "service-config.yaml", content)
}

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	return writeFile(t, t.TempDir(), "query-token", content)
}

func baseConfig(tokenFile string) string {
	return `version: 1
services:
  namf-comm:
    nfType: AMF
    origin: http://127.0.0.1:7777
    apiRoot: /namf-comm/v1
    protocol: h2c
    headerTokenProfile: openamsm-query-token
    discovery: none
    requestTimeout: 10s
limits:
  requestTimeout: 10s
  dialTimeout: 5s
  maxRequestBytes: 16777216
  maxResponseBytes: 16777216
  maxIdleConns: 64
  maxIdleConnsPerHost: 8
  maxConnsPerHost: 32
  idleConnTimeout: 90s
headerTokenProfiles:
  openamsm-query-token:
    header: x-openamsm-query-token
    tokenFile: ` + tokenFile + `
`
}

func TestLoadValidFileToken(t *testing.T) {
	tokenPath := writeTokenFile(t, "private-query-token\n")
	path := writeConfig(t, baseConfig(tokenPath))
	config, err := Load(path, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	service, ok := config.Services["namf-comm"]
	if !ok {
		t.Fatalf("missing service: %+v", config.Services)
	}
	if service.Origin != "http://127.0.0.1:7777" ||
		service.APIRoot != "/namf-comm/v1" ||
		service.HeaderTokenProfile != "openamsm-query-token" {
		t.Fatalf("unexpected service: %+v", service)
	}
	profile := config.HeaderTokenProfiles["openamsm-query-token"]
	if profile.Header != "x-openamsm-query-token" || profile.Token != "private-query-token" {
		t.Fatalf("unexpected token profile: %+v", profile)
	}
	if config.Limits.RequestTimeout != 10*time.Second ||
		config.Limits.DialTimeout != 5*time.Second ||
		config.Limits.MaxRequestBytes != 16777216 ||
		config.Limits.MaxResponseBytes != 16777216 ||
		config.Limits.MaxIdleConns != 64 ||
		config.Limits.MaxIdleConnsPerHost != 8 ||
		config.Limits.MaxConnsPerHost != 32 ||
		config.Limits.IdleConnTimeout != 90*time.Second {
		t.Fatalf("unexpected limits: %+v", config.Limits)
	}
}

func TestLoadValidEnvironmentToken(t *testing.T) {
	content := strings.Replace(baseConfig("/unused"), "    tokenFile: /unused", "    tokenEnv: CA_TEST_QUERY_TOKEN", 1)
	path := writeConfig(t, content)
	lookup := func(name string) (string, bool) {
		if name == "CA_TEST_QUERY_TOKEN" {
			return " env-token \n", true
		}
		return "", false
	}
	config, err := Load(path, lookup)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if token := config.HeaderTokenProfiles["openamsm-query-token"].Token; token != "env-token" {
		t.Fatalf("unexpected env token %q", token)
	}

	if _, err := Load(path, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("expected a missing environment variable to fail")
	}
	t.Setenv("CA_TEST_CONFIG_TOKEN", "from-os-env")
	osContent := strings.Replace(baseConfig("/unused"), "    tokenFile: /unused", "    tokenEnv: CA_TEST_CONFIG_TOKEN", 1)
	osPath := writeConfig(t, osContent)
	osConfig, err := Load(osPath, nil)
	if err != nil {
		t.Fatalf("Load with os.LookupEnv: %v", err)
	}
	if token := osConfig.HeaderTokenProfiles["openamsm-query-token"].Token; token != "from-os-env" {
		t.Fatalf("unexpected token %q", token)
	}
}

func TestLoadPerServiceTimeout(t *testing.T) {
	tokenPath := writeTokenFile(t, "token")
	path := writeConfig(t, baseConfig(tokenPath))
	document, err := LoadDocument(path, nil)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	if timeout := document.RequestTimeout["namf-comm"]; timeout != 10*time.Second {
		t.Fatalf("unexpected per-service timeout %v", timeout)
	}
	if nfType := document.NFType["namf-comm"]; nfType != "AMF" {
		t.Fatalf("unexpected nfType %q", nfType)
	}
	redacted, ok := document.Redacted()["services"].(map[string]any)["namf-comm"].(map[string]any)
	if !ok || redacted["requestTimeout"] != "10s" || redacted["nfType"] != "AMF" {
		t.Fatalf("unexpected redacted document: %#v", document.Redacted())
	}

	for _, timeout := range []string{"0s", "-1s", "3m"} {
		content := strings.Replace(baseConfig(tokenPath), "    requestTimeout: 10s", "    requestTimeout: "+timeout, 1)
		if _, err := Load(writeConfig(t, content), nil); err == nil {
			t.Fatalf("expected requestTimeout %q to be rejected", timeout)
		}
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"unknown top-level field", "version: 1\nbogus: true\nservices:\n  a:\n    origin: http://127.0.0.1:1\n"},
		{"unknown service field", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n    oauth: true\n"},
		{"unknown limit field", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\nlimits:\n  tlsSkipVerify: true\n"},
		{"unknown token profile field", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\nheaderTokenProfiles:\n  p:\n    header: x\n    token: literal-secret\n"},
		{"duplicate top-level key", "version: 1\nversion: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n"},
		{"duplicate service key", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n  a:\n    origin: http://127.0.0.1:2\n"},
		{"alias", "version: 1\nservices:\n  a: &svc\n    origin: http://127.0.0.1:1\n  b: *svc\n"},
		{"merge key", "version: 1\nservices:\n  a:\n    <<: {origin: http://127.0.0.1:1}\n"},
		{"multiple documents", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n---\nversion: 1\n"},
		{"unsupported version", "version: 2\nservices:\n  a:\n    origin: http://127.0.0.1:1\n"},
		{"missing version", "services:\n  a:\n    origin: http://127.0.0.1:1\n"},
		{"empty services", "version: 1\nservices: {}\n"},
		{"invalid yaml", "version: [1\n"},
		{"discovery fields", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n    discovery:\n      method: nrf\n"},
		{"invalid discovery scalar", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n    discovery: nrf\n"},
		{"https protocol", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n    protocol: https\n"},
		{"tls field", "version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:1\n    tls: true\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, testCase.content), nil); err == nil {
				t.Fatalf("expected configuration rejection for %s", testCase.name)
			}
		})
	}
}

func TestLoadRejectsInvalidOriginsAndReferences(t *testing.T) {
	cases := []string{
		"version: 1\nservices:\n  a:\n    origin: https://127.0.0.1:7777\n",
		"version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:7777/base/path\n",
		"version: 1\nservices:\n  a:\n    origin: http://user:pass@127.0.0.1:7777\n",
		"version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:7777?x=1\n",
		"version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:7777\n    headerTokenProfile: missing\n",
		"version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:7777\n    apiRoot: /../admin\n",
		"version: 1\nservices:\n  a:\n    origin: http://127.0.0.1:7777\nlimits:\n  maxRequestBytes: -1\n",
	}
	for _, content := range cases {
		if _, err := Load(writeConfig(t, content), nil); err == nil {
			t.Fatalf("expected rejection for configuration:\n%s", content)
		}
	}
}

func TestLoadTokenFileRules(t *testing.T) {
	tokenPath := writeTokenFile(t, "token")
	directory := filepath.Dir(tokenPath)

	cases := []struct {
		name    string
		content string
	}{
		{"missing token file", strings.Replace(baseConfig(filepath.Join(directory, "missing")), "", "", 1)},
		{"relative token file", baseConfig("relative/token")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, testCase.content), nil); err == nil {
				t.Fatal("expected token file rejection")
			}
		})
	}

	oversized := writeTokenFile(t, strings.Repeat("x", MaxTokenFileBytes+1))
	if _, err := Load(writeConfig(t, baseConfig(oversized)), nil); err == nil {
		t.Fatal("expected oversized token file rejection")
	}
	multiline := writeTokenFile(t, "first-line\nsecond-line")
	if _, err := Load(writeConfig(t, baseConfig(multiline)), nil); err == nil {
		t.Fatal("expected multiline token rejection")
	}
	if _, err := Load(writeConfig(t, baseConfig(writeTokenFile(t, " \n\t"))), nil); err == nil {
		t.Fatal("expected empty token rejection")
	}

	both := strings.Replace(baseConfig(tokenPath), "    tokenFile: "+tokenPath, "    tokenFile: "+tokenPath+"\n    tokenEnv: CA_TEST_TOKEN", 1)
	if _, err := Load(writeConfig(t, both), nil); err == nil {
		t.Fatal("expected mutually exclusive token sources rejection")
	}
	noSource := strings.Replace(baseConfig(tokenPath), "    tokenFile: "+tokenPath, "", 1)
	if _, err := Load(writeConfig(t, noSource), nil); err == nil {
		t.Fatal("expected missing token source rejection")
	}
}

func TestRedactedNeverContainsToken(t *testing.T) {
	const token = "super-secret-query-token-value"
	tokenPath := writeTokenFile(t, token)
	config, err := Load(writeConfig(t, baseConfig(tokenPath)), nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	redacted := Redacted(config)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("marshal redacted view: %v", err)
	}
	if strings.Contains(string(encoded), token) {
		t.Fatalf("redacted view leaked the token: %s", encoded)
	}
	if !strings.Contains(string(encoded), "redacted") {
		t.Fatalf("redacted view does not mark the token: %s", encoded)
	}
	document, err := LoadDocument(writeConfig(t, baseConfig(tokenPath)), nil)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}
	documentJSON, err := json.Marshal(document.Redacted())
	if err != nil {
		t.Fatalf("marshal redacted document: %v", err)
	}
	if strings.Contains(string(documentJSON), token) {
		t.Fatalf("redacted document leaked the token: %s", documentJSON)
	}
}

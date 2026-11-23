package interfacepkg

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// withEnv returns the environment with exactly one value for name.
func withEnv(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, prefix+value)
}

// TestProviderManifestMatchesBinary proves the published runtime binding agrees
// with the actual Go binary: the initialize identity, version and reported
// backend catalog digest must equal provider.yaml, and every published public
// binding must resolve in the catalog RPC.
func TestProviderManifestMatchesBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("provider process identity gate is skipped in -short mode")
	}
	root := repoRoot(t)
	version := "1.1.0"
	binary := filepath.Join(t.TempDir(), "ca-client-tools-test.exe")
	if os.PathSeparator != '\\' {
		binary = strings.TrimSuffix(binary, ".exe")
	}
	build := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-X github.com/odango-chan/ca-client-tools/internal/providerapp.ProviderVersion="+version,
		"-o", binary, "./cmd/ca-client-tools")
	build.Dir = root
	build.Env = withEnv(os.Environ(), "GOWORK", "off")
	build.Env = withEnv(build.Env, "CGO_ENABLED", "0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider: %v\n%s", err, output)
	}

	result, err := Generate(Options{
		CatalogPath:     filepath.Join(root, CatalogPath),
		PackageVersion:  version,
		ProviderVersion: version,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var manifest providerManifest
	if err := yaml.Unmarshal(result.ProviderManifest, &manifest); err != nil {
		t.Fatalf("decode provider.yaml: %v", err)
	}

	command := exec.Command(binary, manifest.Transport.Argv[1:]...)
	command.Dir = filepath.Dir(binary)
	command.Env = withEnv(os.Environ(), "CA_OPENAMSM_SMF_SBI_ORIGIN", "http://127.0.0.1:1")
	command.Env = withEnv(command.Env, "CA_OPENAMSM_QUERY_TOKEN", "interfacepkg-test-token")
	command.Env = withEnv(command.Env, "CA_OPENAMSM_AMF_METRICS_ORIGIN", "http://127.0.0.1:2")
	command.Env = withEnv(command.Env, "CA_OPENAMSM_SMF_METRICS_ORIGIN", "http://127.0.0.1:3")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start provider: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = command.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
		}
	}()

	reader := bufio.NewReader(stdout)
	request := func(id, method string, params map[string]any) map[string]any {
		t.Helper()
		frame := map[string]any{
			"protocolVersion": "ca-client-tools.provider/v2",
			"type":            "request",
			"requestId":       id,
			"method":          method,
			"params":          params,
		}
		encoded, err := json.Marshal(frame)
		if err != nil {
			t.Fatalf("encode %s: %v", method, err)
		}
		if _, err := stdin.Write(append(encoded, '\n')); err != nil {
			t.Fatalf("write %s: %v", method, err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read %s response: %v", method, err)
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("decode %s response: %v", method, err)
		}
		if response["ok"] != true {
			t.Fatalf("%s failed: %s", method, line)
		}
		result, ok := response["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s result is not an object: %s", method, line)
		}
		return result
	}

	initialized := request("init-1", "initialize", map[string]any{
		"hostInstanceId":     "host:interfacepkg-test",
		"providerInstanceId": "provider:interfacepkg-test",
		"limits": map[string]any{
			"maxFrameBytes":     8388608,
			"maxInFlight":       4,
			"maxCallDurationMs": 30000,
		},
	})
	if initialized["providerId"] != manifest.ProviderID {
		t.Fatalf("initialize providerId = %v, want %s", initialized["providerId"], manifest.ProviderID)
	}
	if initialized["providerVersion"] != manifest.ProviderVersion {
		t.Fatalf("initialize providerVersion = %v, want %s", initialized["providerVersion"], manifest.ProviderVersion)
	}
	if initialized["backendBindingCatalogDigest"] != manifest.BackendCatalogDigest {
		t.Fatalf("initialize backendBindingCatalogDigest = %v, want %s",
			initialized["backendBindingCatalogDigest"], manifest.BackendCatalogDigest)
	}

	operationIDs := make([]string, 0, len(manifest.Bindings))
	for _, binding := range manifest.Bindings {
		operationIDs = append(operationIDs, binding.BackendOperationID)
	}
	catalogued := request("catalog-1", "catalog", map[string]any{"backendOperationIds": operationIDs})
	supported, ok := catalogued["supported"].([]any)
	if !ok || len(supported) != len(operationIDs) {
		t.Fatalf("catalog supported = %v, want all %d published bindings", catalogued["supported"], len(operationIDs))
	}
	supportedIDs := make(map[string]bool, len(supported))
	for _, id := range supported {
		supportedIDs[id.(string)] = true
	}
	for _, binding := range manifest.Bindings {
		if !supportedIDs[binding.BackendOperationID] {
			t.Fatalf("published binding %q is not supported by the binary", binding.BackendOperationID)
		}
	}
}

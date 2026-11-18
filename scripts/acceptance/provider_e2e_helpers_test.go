//go:build acceptance

package acceptance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// protocolV2 is the sole Provider process wire identity. There is no v1
// compatibility path: a frame carrying any other identity is rejected with
// PROTOCOL_VERSION_UNSUPPORTED.
const protocolV2 = "ca-client-tools.provider/v2"

// fixtureServiceKey is the acceptance-only trusted service key used by the
// providerfixture build's genericSBI bindings (acceptance.generic.*). The
// production binary never contains it.
const fixtureServiceKey = "acceptance-fixture-sbi"

// typedMASServiceKey is the production catalog service key of the eight typed
// SMF context bindings.
const typedMASServiceKey = "openamsm-smf-context"

type frame struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Type            string         `json:"type"`
	RequestID       string         `json:"requestId"`
	Method          string         `json:"method"`
	Params          map[string]any `json:"params"`
}

type response struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Type            string         `json:"type"`
	RequestID       string         `json:"requestId"`
	OK              bool           `json:"ok"`
	Result          map[string]any `json:"result"`
	Error           *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Detail  map[string]any `json:"detail"`
	} `json:"error"`
}

// providerProcess is a real resident Provider child process driven over its
// NDJSON stdin/stdout protocol.
type providerProcess struct {
	t       *testing.T
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	stderr  bytes.Buffer
	mu      sync.Mutex
	pending map[string][]*response
	all     []*response
	done    chan struct{}
	readErr error
}

func startProvider(t *testing.T, exe string, env map[string]string) *providerProcess {
	t.Helper()
	process := &providerProcess{
		t:       t,
		pending: make(map[string][]*response),
		done:    make(chan struct{}),
	}
	process.cmd = exec.Command(exe, "provider", "serve")
	process.cmd.Env = providerEnvironment(env)
	stderrPipe, err := process.cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := process.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := process.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.cmd.Start(); err != nil {
		t.Fatalf("start provider: %v", err)
	}
	process.stdin = stdin
	process.scanner = bufio.NewScanner(stdout)
	process.scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)

	go func() { _, _ = io.Copy(&process.stderr, stderrPipe) }()
	go func() {
		defer close(process.done)
		for process.scanner.Scan() {
			line := process.scanner.Bytes()
			var decoded response
			// Preserve JSON numbers as json.Number so the generic acceptance
			// test can assert exact integer precision (for example
			// 9007199254740993 is not representable as a float64).
			decoder := json.NewDecoder(bytes.NewReader(line))
			decoder.UseNumber()
			if err := decoder.Decode(&decoded); err != nil {
				process.setReadErr(fmt.Errorf("stdout is not a protocol frame: %q: %w", string(line), err))
				return
			}
			if decoded.Type != "response" {
				process.setReadErr(fmt.Errorf("stdout frame type %q is not response", decoded.Type))
				return
			}
			process.mu.Lock()
			process.all = append(process.all, &decoded)
			process.pending[decoded.RequestID] = append(process.pending[decoded.RequestID], &decoded)
			process.mu.Unlock()
		}
		if err := process.scanner.Err(); err != nil {
			process.setReadErr(err)
		}
	}()
	t.Cleanup(func() {
		_ = process.stdin.Close()
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			_ = process.cmd.Process.Kill()
		}
		_ = process.cmd.Wait()
		if t.Failed() {
			if output := strings.TrimSpace(process.stderr.String()); output != "" {
				t.Logf("provider stderr: %s", output)
			}
		}
	})
	return process
}

func providerEnvironment(extra map[string]string) []string {
	var env []string
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(name, "CA_") {
			continue
		}
		env = append(env, entry)
	}
	for name, value := range extra {
		env = append(env, name+"="+value)
	}
	return env
}

func (p *providerProcess) setReadErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readErr == nil {
		p.readErr = err
	}
}

func (p *providerProcess) stdinWriteRaw(raw string) error {
	_, err := io.WriteString(p.stdin, raw)
	return err
}

func (p *providerProcess) stdoutText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var lines []string
	for _, response := range p.all {
		encoded, _ := json.Marshal(response)
		lines = append(lines, string(encoded))
	}
	return strings.Join(lines, "\n")
}

func (p *providerProcess) write(request frame) {
	p.t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		p.t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if _, err := p.stdin.Write(encoded); err != nil {
		p.t.Fatalf("write frame: %v", err)
	}
}

// roundTrip sends one request and waits for its terminal response.
func (p *providerProcess) roundTrip(request frame) *response {
	p.t.Helper()
	p.write(request)
	return p.wait(request.RequestID)
}

func (p *providerProcess) wait(requestID string) *response {
	p.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		p.mu.Lock()
		if responses := p.pending[requestID]; len(responses) > 0 {
			decoded := responses[0]
			p.pending[requestID] = responses[1:]
			p.mu.Unlock()
			return decoded
		}
		readErr := p.readErr
		p.mu.Unlock()
		if readErr != nil {
			p.t.Fatalf("provider stdout failure: %v", readErr)
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for response %q", requestID)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *providerProcess) stderrText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderr.String()
}

func (p *providerProcess) closeAndWait() int {
	_ = p.stdin.Close()
	<-p.done
	err := p.cmd.Wait()
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Skipf("%s is not set; see scripts/acceptance/README.md or run through scripts/acceptance/windows-provider-e2e.ps1 / wsl-provider-e2e.sh", name)
	}
	return value
}

func liveEnvironment(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"CA_OPENAMSM_SMF_SBI_ORIGIN":     requireEnv(t, "CA_ACCEPTANCE_SMF_SBI_ORIGIN"),
		"CA_OPENAMSM_QUERY_TOKEN":        requireEnv(t, "CA_ACCEPTANCE_QUERY_TOKEN"),
		"CA_OPENAMSM_AMF_METRICS_ORIGIN": requireEnv(t, "CA_ACCEPTANCE_AMF_METRICS_ORIGIN"),
		"CA_OPENAMSM_SMF_METRICS_ORIGIN": requireEnv(t, "CA_ACCEPTANCE_SMF_METRICS_ORIGIN"),
	}
}

func providerExe(t *testing.T) string {
	return requireEnv(t, "CA_ACCEPTANCE_PROVIDER_EXE")
}

func cliExe(t *testing.T) string {
	return requireEnv(t, "CA_ACCEPTANCE_CLI_EXE")
}

// fixtureProviderExe is the explicit providerfixture build of the Provider. It
// is the only binary that contains the acceptance.generic.* bindings; the
// production binary does not.
func fixtureProviderExe(t *testing.T) string {
	t.Helper()
	return requireEnv(t, "CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE")
}

// fixtureServiceConfig renders the trusted serviceconfig v1 document that
// points the acceptance-only genericSBI service key at the deterministic
// loopback h2c upstream. Version 1 with an explicit h2c service is the same
// schema the deployed Provider reads from CA_PROVIDER_SERVICE_CONFIG.
func fixtureServiceConfig(origin string) string {
	return fixtureServiceConfigWithTyped(origin, false)
}

// fixtureServiceConfigWithTyped optionally also configures the trusted typed
// OpenAMSM SMF context service key so the eleven production typed bindings are
// available for catalog-descriptor regression assertions without a live lab.
// The origin is still the loopback acceptance upstream; no typed call is made.
func fixtureServiceConfigWithTyped(origin string, includeTyped bool) string {
	document := "version: 1\nservices:\n"
	document += fmt.Sprintf("  %s:\n    origin: %s\n    protocol: h2c\n", fixtureServiceKey, origin)
	if includeTyped {
		document += fmt.Sprintf("  %s:\n    origin: %s\n    protocol: h2c\n", typedMASServiceKey, origin)
	}
	return document
}

// writeFixtureServiceConfig writes trusted service configuration to a private
// temporary file and returns its absolute path. The file is written by the
// test, not by product code, and never contains a secret.
func writeFixtureServiceConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider-service-config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture service configuration: %v", err)
	}
	return path
}

// startFixtureProvider starts the providerfixture Provider child process with
// the trusted service configuration pointing at the loopback upstream. Only
// the fixture service is configured, so the eleven production typed bindings
// report NOT_CONFIGURED and no live OpenAMSM deployment is required.
func startFixtureProvider(t *testing.T, upstreamOrigin string) *providerProcess {
	t.Helper()
	configPath := writeFixtureServiceConfig(t, fixtureServiceConfig(upstreamOrigin))
	return startProvider(t, fixtureProviderExe(t), map[string]string{
		"CA_PROVIDER_SERVICE_CONFIG": configPath,
	})
}

// startFixtureProviderAllServices additionally trusts the typed SMF context
// and metrics service keys against the same loopback origin so every typed
// binding is available for catalog-descriptor assertions. No typed remote call
// is performed; the service is only configured, never exercised.
func startFixtureProviderAllServices(t *testing.T, upstreamOrigin string) *providerProcess {
	t.Helper()
	configPath := writeFixtureServiceConfig(t, fixtureServiceConfigWithTyped(upstreamOrigin, true))
	return startProvider(t, fixtureProviderExe(t), map[string]string{
		"CA_PROVIDER_SERVICE_CONFIG":     configPath,
		"CA_OPENAMSM_AMF_METRICS_ORIGIN": upstreamOrigin,
		"CA_OPENAMSM_SMF_METRICS_ORIGIN": upstreamOrigin,
	})
}

func (p *providerProcess) responseCount(requestID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, decoded := range p.all {
		if decoded.RequestID == requestID {
			count++
		}
	}
	return count
}

func initializeFrame(protocolVersion, requestID string) frame {
	return frame{
		ProtocolVersion: protocolVersion,
		Type:            "request",
		RequestID:       requestID,
		Method:          "initialize",
		Params: map[string]any{
			"hostInstanceId":     "acceptance-host",
			"providerInstanceId": "ca-client-tools.sbi",
			"limits": map[string]any{
				"maxFrameBytes":     8388608,
				"maxInFlight":       8,
				"maxCallDurationMs": 5000,
			},
		},
	}
}

func invocationMetadata(requestID string) map[string]any {
	return map[string]any{
		"invocationId":         "inv-" + requestID,
		"producerInvocationId": "producer-" + requestID,
		"deadline":             time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339),
	}
}

func invokeFrame(protocolVersion, requestID, operationID string, arguments map[string]any) frame {
	return frame{
		ProtocolVersion: protocolVersion,
		Type:            "request",
		RequestID:       requestID,
		Method:          "invoke",
		Params: map[string]any{
			"backendOperationId": operationID,
			"arguments":          arguments,
			"invocation":         invocationMetadata(requestID),
		},
	}
}

func catalogFrame(protocolVersion, requestID string, ids []string) frame {
	return frame{
		ProtocolVersion: protocolVersion,
		Type:            "request",
		RequestID:       requestID,
		Method:          "catalog",
		Params:          map[string]any{"backendOperationIds": ids},
	}
}

func cancelFrame(protocolVersion, requestID, target string) frame {
	return frame{
		ProtocolVersion: protocolVersion,
		Type:            "request",
		RequestID:       requestID,
		Method:          "cancel",
		Params:          map[string]any{"targetRequestId": target},
	}
}

func expectOK(t *testing.T, decoded *response) map[string]any {
	t.Helper()
	if decoded == nil {
		t.Fatal("nil response")
	}
	if !decoded.OK {
		t.Fatalf("request %q failed: %#v", decoded.RequestID, decoded.Error)
	}
	if decoded.Result == nil {
		t.Fatalf("request %q has no result", decoded.RequestID)
	}
	return decoded.Result
}

func expectFault(t *testing.T, decoded *response, code string) {
	t.Helper()
	if decoded == nil {
		t.Fatal("nil response")
	}
	if decoded.OK || decoded.Error == nil {
		t.Fatalf("request %q: expected fault %s, got ok=%v result=%#v", decoded.RequestID, code, decoded.OK, decoded.Result)
	}
	if decoded.Error.Code != code {
		t.Fatalf("request %q: fault code = %q, want %q", decoded.RequestID, decoded.Error.Code, code)
	}
	if decoded.Error.Message == "" {
		t.Fatalf("request %q: fault message is empty", decoded.RequestID)
	}
}

func number(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func runCLI(t *testing.T, exe string, env map[string]string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	command := exec.Command(exe, args...)
	command.Env = providerEnvironment(env)
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctx
	err := command.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run CLI: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func parseData(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
		t.Fatalf("CLI stdout is not a data envelope: %v: %q", err, stdout)
	}
	return envelope.Data
}

func intString(value int64) string { return strconv.FormatInt(value, 10) }

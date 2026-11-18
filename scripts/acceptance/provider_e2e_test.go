//go:build acceptance

package acceptance

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

type readTool struct {
	backendOperationID string
	masID              string
	contextType        string
}

var smfContextTools = []readTool{
	{backendOperationID: "openamsm.smf.mas.sc.query", masID: "SC", contextType: "session"},
	{backendOperationID: "openamsm.smf.mas.pa.query", masID: "PA", contextType: "policy"},
	{backendOperationID: "openamsm.smf.mas.upn.query", masID: "UPN", contextType: "up-node"},
	{backendOperationID: "openamsm.smf.mas.ssm.query", masID: "SSM", contextType: "signaling-session"},
	{backendOperationID: "openamsm.smf.mas.iam.query", masID: "IAM", contextType: "ip-address"},
	{backendOperationID: "openamsm.smf.mas.ups.query", masID: "UPS", contextType: "up-selection"},
	{backendOperationID: "openamsm.smf.mas.ca.query", masID: "CA", contextType: "charging"},
	{backendOperationID: "openamsm.smf.mas.saa.query", masID: "SAA", contextType: "session-auth"},
}

var listToolIDs = []string{
	"openamsm.amf.ue-info.list",
	"openamsm.amf.gnb-info.list",
	"openamsm.smf.pdu-info.list",
}

func allV2IDs() []string {
	ids := make([]string, 0, len(smfContextTools)+len(listToolIDs))
	for _, tool := range smfContextTools {
		ids = append(ids, tool.backendOperationID)
	}
	ids = append(ids, listToolIDs...)
	return ids
}

func liveValues(t *testing.T) (string, int64) {
	t.Helper()
	supi := requireEnv(t, "CA_ACCEPTANCE_SUPI")
	psiText := requireEnv(t, "CA_ACCEPTANCE_PSI")
	psi, err := strconv.ParseInt(psiText, 10, 32)
	if err != nil {
		t.Fatalf("invalid CA_ACCEPTANCE_PSI %q", psiText)
	}
	return supi, psi
}

func TestProviderV2AllReadTools(t *testing.T) {
	env := liveEnvironment(t)
	supi, psi := liveValues(t)
	process := startProvider(t, providerExe(t), env)

	initialized := process.roundTrip(initializeFrame(protocolV2, "init-001"))
	result := expectOK(t, initialized)
	if result["providerId"] != "ca-client-tools.sbi" {
		t.Fatalf("providerId = %v", result["providerId"])
	}
	if result["catalogVersion"] != "2" {
		t.Fatalf("catalogVersion = %v", result["catalogVersion"])
	}
	if digest, _ := result["backendBindingCatalogDigest"].(string); !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("catalog digest = %v", result["backendBindingCatalogDigest"])
	}

	// Catalog: every P0 tool is supported and described without route/secret data.
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "catalog-001", allV2IDs())))
	supported, ok := catalogResult["supported"].([]any)
	if !ok || len(supported) != len(allV2IDs()) {
		t.Fatalf("supported = %#v, want %d entries", catalogResult["supported"], len(allV2IDs()))
	}
	for index, raw := range supported {
		if raw != allV2IDs()[index] {
			t.Fatalf("supported[%d] = %v, want %s", index, raw, allV2IDs()[index])
		}
	}
	unsupported, ok := catalogResult["unsupported"].([]any)
	if !ok || len(unsupported) != 0 {
		t.Fatalf("unsupported = %#v, want empty", catalogResult["unsupported"])
	}
	descriptors, ok := catalogResult["bindings"].([]any)
	if !ok || len(descriptors) != len(allV2IDs()) {
		t.Fatalf("bindings = %#v", catalogResult["bindings"])
	}
	descriptorText, _ := json.Marshal(descriptors)
	for _, forbidden := range []string{"http://", "https://", "serviceKey", `"method"`, `"path"`, "token"} {
		if strings.Contains(string(descriptorText), forbidden) {
			t.Fatalf("catalog descriptor leaked %q: %s", forbidden, descriptorText)
		}
	}

	// Invoke every tool through the resident Provider.
	masArguments := map[string]any{"supi": supi, "pduSessionId": psi}
	for _, tool := range smfContextTools {
		decoded := process.roundTrip(invokeFrame(protocolV2, "invoke-"+tool.masID, tool.backendOperationID, masArguments))
		invocation := expectOK(t, decoded)
		if status, _ := number(invocation["httpStatus"]); status != 200 {
			t.Fatalf("%s httpStatus = %v", tool.backendOperationID, invocation["httpStatus"])
		}
		body, ok := invocation["response"].(map[string]any)
		if !ok {
			t.Fatalf("%s response = %#v", tool.backendOperationID, invocation["response"])
		}
		if body["masId"] != tool.masID {
			t.Fatalf("%s masId = %v", tool.backendOperationID, body["masId"])
		}
		if state, _ := body["status"].(string); state != "AVAILABLE" && state != "UNAVAILABLE" {
			t.Fatalf("%s status = %v", tool.backendOperationID, body["status"])
		}
		key, ok := body["key"].(map[string]any)
		if !ok || key["supi"] != supi {
			t.Fatalf("%s key = %#v", tool.backendOperationID, body["key"])
		}
		if keyPSI, ok := number(key["pduSessionId"]); !ok || keyPSI != psi {
			t.Fatalf("%s key.pduSessionId = %v", tool.backendOperationID, key["pduSessionId"])
		}
		if body["status"] == "UNAVAILABLE" {
			if contextValue, ok := body["context"].(map[string]any); !ok || len(contextValue) != 0 {
				t.Fatalf("%s UNAVAILABLE context = %#v", tool.backendOperationID, body["context"])
			}
		}
	}

	for index, operationID := range listToolIDs {
		decoded := process.roundTrip(invokeFrame(protocolV2, "invoke-list-"+intString(int64(index)), operationID,
			map[string]any{"page": 0, "pageSize": 2}))
		invocation := expectOK(t, decoded)
		if status, _ := number(invocation["httpStatus"]); status != 200 {
			t.Fatalf("%s httpStatus = %v", operationID, invocation["httpStatus"])
		}
		page, ok := invocation["response"].(map[string]any)
		if !ok {
			t.Fatalf("%s response = %#v", operationID, invocation["response"])
		}
		items, ok := page["items"].([]any)
		if !ok {
			t.Fatalf("%s items = %#v", operationID, page["items"])
		}
		pager, ok := page["pager"].(map[string]any)
		if !ok {
			t.Fatalf("%s pager = %#v", operationID, page["pager"])
		}
		if pageNumber, ok := number(pager["page"]); !ok || pageNumber != 0 {
			t.Fatalf("%s pager.page = %v", operationID, pager["page"])
		}
		if pageSize, ok := number(pager["page_size"]); !ok || pageSize != 2 {
			t.Fatalf("%s pager.page_size = %v", operationID, pager["page_size"])
		}
		if count, ok := number(pager["count"]); !ok || count != int64(len(items)) {
			t.Fatalf("%s pager.count = %v, items = %d", operationID, pager["count"], len(items))
		}
		if truncated, exists := pager["truncated"]; exists && truncated == true {
			t.Fatalf("%s returned truncated=true", operationID)
		}
	}

	// Live membership: AMF UE, AMF gNB and SMF PDU projections contain the lab UE.
	ue := process.roundTrip(invokeFrame(protocolV2, "invoke-ue-membership", "openamsm.amf.ue-info.list",
		map[string]any{"page": 0, "pageSize": 100}))
	uePage := expectOK(t, ue)["response"].(map[string]any)
	if !pageContains(uePage, "supi", supi) {
		t.Fatalf("AMF UE list does not contain %s", supi)
	}
	pdu := process.roundTrip(invokeFrame(protocolV2, "invoke-pdu-membership", "openamsm.smf.pdu-info.list",
		map[string]any{"page": 0, "pageSize": 100}))
	pduPage := expectOK(t, pdu)["response"].(map[string]any)
	if !pageContains(pduPage, "supi", supi) {
		t.Fatalf("SMF PDU list does not contain %s", supi)
	}
	gnb := process.roundTrip(invokeFrame(protocolV2, "invoke-gnb-membership", "openamsm.amf.gnb-info.list",
		map[string]any{"page": 0, "pageSize": 100}))
	gnbPage := expectOK(t, gnb)["response"].(map[string]any)
	if items, _ := gnbPage["items"].([]any); len(items) == 0 {
		t.Fatal("AMF gNB list is empty on a live deployment with a connected UE")
	}

	if stderrText := strings.TrimSpace(process.stderrText()); strings.Contains(stderrText, "token") ||
		strings.Contains(stderrText, supi) {
		t.Fatalf("provider stderr leaked secrets: %q", stderrText)
	}
}

func pageContains(page map[string]any, key, value string) bool {
	items, _ := page["items"].([]any)
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if object[key] == value {
			return true
		}
		// SMF PDU list nests the PDU array; AMF UE list carries supi at top level.
		if nested, ok := object["pdu"].([]any); ok {
			for _, candidate := range nested {
				if inner, ok := candidate.(map[string]any); ok && inner[key] == value {
					return true
				}
			}
		}
	}
	return false
}

func TestProviderV2ProblemDetailsAndErrorBoundaries(t *testing.T) {
	env := liveEnvironment(t)
	supi, psi := liveValues(t)
	process := startProvider(t, providerExe(t), env)
	expectOK(t, process.roundTrip(initializeFrame(protocolV2, "init-001")))

	// Valid remote ProblemDetails is a successful protocol result.
	wrongPSI := (psi + 50) % 200
	if wrongPSI == psi {
		wrongPSI = (psi + 1) % 200
	}
	problem := process.roundTrip(invokeFrame(protocolV2, "invoke-problem", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": wrongPSI}))
	problemResult := expectOK(t, problem)
	if status, _ := number(problemResult["httpStatus"]); status != 404 {
		t.Fatalf("problem httpStatus = %v", problemResult["httpStatus"])
	}
	details, ok := problemResult["problem"].(map[string]any)
	if !ok {
		t.Fatalf("problem = %#v", problemResult["problem"])
	}
	if problemStatus, _ := number(details["status"]); problemStatus != 404 {
		t.Fatalf("problem.status = %v", details["status"])
	}
	if details["cause"] != "SESSION_NOT_FOUND" {
		t.Fatalf("problem.cause = %v", details["cause"])
	}

	// Invalid typed arguments fail before any remote request.
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-bad-args", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": 300})), "BACKEND_ARGUMENTS_INVALID")
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-unknown-arg", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": psi, "imsi": "001"})), "BACKEND_ARGUMENTS_INVALID")
	expectFault(t, process.roundTrip(invokeFrame(protocolV2, "invoke-unknown-op", "openamsm.unknown.op",
		map[string]any{})), "BACKEND_OPERATION_UNSUPPORTED")

	// A protocol error does not terminate the process.
	expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-after-errors", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": psi})))
}

func TestProviderV2CancellationDeadlineAndFraming(t *testing.T) {
	env := liveEnvironment(t)
	supi, psi := liveValues(t)
	process := startProvider(t, providerExe(t), env)
	expectOK(t, process.roundTrip(initializeFrame(protocolV2, "init-001")))

	// Expired absolute deadline is rejected without remote dispatch.
	expired := invokeFrame(protocolV2, "invoke-expired", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": psi})
	expired.Params["invocation"] = map[string]any{
		"invocationId":         "inv-expired",
		"producerInvocationId": "producer-expired",
		"deadline":             "2001-01-01T00:00:00Z",
	}
	expectFault(t, process.roundTrip(expired), "CALL_DEADLINE_EXCEEDED")

	// Cancel the latest long-deadline invoke and require exactly one terminal
	// response for the original request.
	longDeadline := invokeFrame(protocolV2, "invoke-cancel-target", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": psi})
	longDeadline.Params["invocation"] = map[string]any{
		"invocationId":         "inv-cancel-target",
		"producerInvocationId": "producer-cancel-target",
		"deadline":             time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339),
	}
	process.write(longDeadline)
	cancelResult := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-001", "invoke-cancel-target")))
	if disposition := cancelResult["disposition"]; disposition != "CANCEL_REQUESTED" && disposition != "ALREADY_COMPLETED" {
		t.Fatalf("cancel disposition = %v", disposition)
	}
	terminal := process.wait("invoke-cancel-target")
	if terminal.OK {
		// A completion/cancellation race may return the valid remote result.
		result := expectOK(t, terminal)
		if status, _ := number(result["httpStatus"]); status != 200 {
			t.Fatalf("cancel-race result = %#v", result)
		}
	} else {
		expectFault(t, terminal, "CALL_CANCELLED")
	}
	secondCancel := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-002", "invoke-cancel-target")))
	if secondCancel["disposition"] != "ALREADY_COMPLETED" {
		t.Fatalf("second cancel disposition = %v", secondCancel["disposition"])
	}
	notFound := expectOK(t, process.roundTrip(cancelFrame(protocolV2, "cancel-003", "no-such-request")))
	if notFound["disposition"] != "NOT_FOUND" {
		t.Fatalf("unknown cancel disposition = %v", notFound["disposition"])
	}

	// Malformed envelope and duplicate request IDs are rejected, then service continues.
	if err := process.stdinWriteRaw("{\"protocolVersion\":\"ca-client-tools.provider/v2\",\"type\":\"request\",\"requestId\":\"bad-1\",\"method\":\"catalog\",\"params\":{},\"unknown\":1}\n"); err != nil {
		t.Fatal(err)
	}
	malformed := process.wait("bad-1")
	expectFault(t, malformed, "PROTOCOL_FRAME_INVALID")

	duplicate := invokeFrame(protocolV2, "duplicate-id", "openamsm.amf.ue-info.list", map[string]any{"page": 0, "pageSize": 1})
	process.write(duplicate)
	if first := process.wait("duplicate-id"); !first.OK && first.Error.Code == "PROTOCOL_REQUEST_ID_INVALID" {
		t.Fatalf("first duplicate-id request was rejected unexpectedly: %#v", first.Error)
	}
	process.write(duplicate)
	second := process.wait("duplicate-id")
	expectFault(t, second, "PROTOCOL_REQUEST_ID_INVALID")

	expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-after-framing", "openamsm.amf.ue-info.list",
		map[string]any{"page": 0, "pageSize": 1})))
}

func TestProviderV2ConcurrentCalls(t *testing.T) {
	env := liveEnvironment(t)
	supi, psi := liveValues(t)
	process := startProvider(t, providerExe(t), env)
	expectOK(t, process.roundTrip(initializeFrame(protocolV2, "init-001")))

	requests := make([]frame, 0, 8)
	for index := 0; index < 4; index++ {
		requests = append(requests, invokeFrame(protocolV2, "concurrent-mas-"+intString(int64(index)),
			"openamsm.smf.mas.sc.query", map[string]any{"supi": supi, "pduSessionId": psi}))
	}
	for index := 0; index < 4; index++ {
		requests = append(requests, invokeFrame(protocolV2, "concurrent-list-"+intString(int64(index)),
			"openamsm.amf.ue-info.list", map[string]any{"page": 0, "pageSize": 1}))
	}
	for _, request := range requests {
		process.write(request)
	}
	for _, request := range requests {
		decoded := process.wait(request.RequestID)
		if !decoded.OK {
			t.Fatalf("%s failed: %#v", request.RequestID, decoded.Error)
		}
	}
}

// TestProviderLegacyProtocolRejected proves the single wire identity. A legacy
// "ca-client-tools.provider/v1" initialize frame is rejected with
// PROTOCOL_VERSION_UNSUPPORTED and does not initialize the process; the same
// process then initializes and serves normally with v2, and a legacy frame
// after initialization is rejected too. There is no version negotiation,
// fallback or compatibility path.
func TestProviderLegacyProtocolRejected(t *testing.T) {
	env := liveEnvironment(t)
	process := startProvider(t, providerExe(t), env)

	legacyInitialize := initializeFrame("ca-client-tools.provider/v1", "legacy-init-001")
	rejected := process.roundTrip(legacyInitialize)
	expectFault(t, rejected, "PROTOCOL_VERSION_UNSUPPORTED")
	if rejected.ProtocolVersion != protocolV2 {
		t.Fatalf("legacy rejection response protocolVersion = %q, want %q", rejected.ProtocolVersion, protocolV2)
	}

	// The rejected legacy handshake must not initialize the process.
	expectFault(t, process.roundTrip(catalogFrame(protocolV2, "pre-init-catalog", allV2IDs())), "PROVIDER_NOT_INITIALIZED")

	initialized := expectOK(t, process.roundTrip(initializeFrame(protocolV2, "init-001")))
	if initialized["catalogVersion"] != "2" {
		t.Fatalf("catalogVersion = %v", initialized["catalogVersion"])
	}
	if digest, _ := initialized["backendBindingCatalogDigest"].(string); !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("catalog digest = %v", initialized["backendBindingCatalogDigest"])
	}

	// A legacy frame after initialization is still rejected; the sole identity
	// is reasserted and the process continues serving v2 requests.
	expectFault(t, process.roundTrip(catalogFrame("ca-client-tools.provider/v1", "legacy-catalog-001", allV2IDs())),
		"PROTOCOL_VERSION_UNSUPPORTED")
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "catalog-001", allV2IDs())))
	supported, ok := catalogResult["supported"].([]any)
	if !ok || len(supported) != len(allV2IDs()) {
		t.Fatalf("supported = %#v, want %d entries", catalogResult["supported"], len(allV2IDs()))
	}
	for index, raw := range supported {
		if raw != allV2IDs()[index] {
			t.Fatalf("supported[%d] = %v, want %s", index, raw, allV2IDs()[index])
		}
	}
	if unsupported, _ := catalogResult["unsupported"].([]any); len(unsupported) != 0 {
		t.Fatalf("unsupported = %#v, want empty", catalogResult["unsupported"])
	}

	// The same process serves a real typed read through the sole identity.
	supi, psi := liveValues(t)
	invocation := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "invoke-after-legacy", "openamsm.smf.mas.sc.query",
		map[string]any{"supi": supi, "pduSessionId": psi})))
	if status, _ := number(invocation["httpStatus"]); status != 200 && status != 404 {
		t.Fatalf("typed invoke httpStatus = %v", invocation["httpStatus"])
	}
}

func TestProviderCleanEOFShutdown(t *testing.T) {
	env := liveEnvironment(t)
	process := startProvider(t, providerExe(t), env)
	expectOK(t, process.roundTrip(initializeFrame(protocolV2, "init-001")))
	code := process.closeAndWait()
	if code != 0 {
		t.Fatalf("provider exit code = %d", code)
	}
	if lines := strings.TrimSpace(process.stdoutText()); lines != "" {
		for _, line := range strings.Split(lines, "\n") {
			var decoded response
			if err := json.Unmarshal([]byte(line), &decoded); err != nil {
				t.Fatalf("non-protocol stdout after shutdown: %q", line)
			}
		}
	}
}

func TestCLIRegressionLive(t *testing.T) {
	env := liveEnvironment(t)
	supi, psi := liveValues(t)
	exe := cliExe(t)

	ue := expectCLIData(t, exe, env, "", "amf", "ue", "list", "--page", "0", "--page-size", "2")
	if items, _ := ue["items"].([]any); len(items) > 2 {
		t.Fatalf("AMF UE page size not honored: %d items", len(items))
	}
	if !pageContains(ue, "supi", supi) {
		t.Fatalf("CLI AMF UE list does not contain %s", supi)
	}
	gnb := expectCLIData(t, exe, env, "", "amf", "gnb", "list", "--page", "0", "--page-size", "2")
	if items, _ := gnb["items"].([]any); len(items) == 0 {
		t.Fatal("CLI AMF gNB list is empty")
	}
	pdu := expectCLIData(t, exe, env, "", "smf", "pdu", "list", "--page", "0", "--page-size", "2")
	if !pageContains(pdu, "supi", supi) {
		t.Fatalf("CLI SMF PDU list does not contain %s", supi)
	}

	input := `{"supi":"` + supi + `","pduSessionId":` + intString(psi) + `}`
	for _, tool := range smfContextTools {
		data := expectCLIData(t, exe, env, input, "smf", "context", "query", "--type", tool.contextType)
		if data["type"] != tool.contextType {
			t.Fatalf("CLI context type = %v, want %s", data["type"], tool.contextType)
		}
		if data["status"] != "AVAILABLE" && data["status"] != "UNAVAILABLE" {
			t.Fatalf("CLI context status = %v", data["status"])
		}
		key, _ := data["key"].(map[string]any)
		if key["supi"] != supi {
			t.Fatalf("CLI context key = %#v", data["key"])
		}
	}

	// Negative CLI matrix: exact HTTP status/cause and exit codes.
	wrongPSI := (psi + 50) % 200
	if wrongPSI == psi {
		wrongPSI = (psi + 1) % 200
	}
	_, stderr, code := runCLI(t, exe, env, `{"supi":"`+supi+`","pduSessionId":`+intString(wrongPSI)+`}`,
		"smf", "context", "query", "--type", "session")
	if code != 4 {
		t.Fatalf("wrong PSI exit = %d, stderr = %s", code, stderr)
	}
	var cliError struct {
		Error struct {
			Code       string `json:"code"`
			HTTPStatus int    `json:"httpStatus"`
			Cause      string `json:"cause"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &cliError); err != nil {
		t.Fatalf("CLI stderr is not an error envelope: %v: %q", err, stderr)
	}
	if cliError.Error.Code != "OPENAMSM_HTTP_ERROR" || cliError.Error.HTTPStatus != 404 || cliError.Error.Cause != "SESSION_NOT_FOUND" {
		t.Fatalf("CLI error envelope = %#v", cliError.Error)
	}

	_, _, code = runCLI(t, exe, env, `{"supi":"`+supi+`","pduSessionId":300}`, "smf", "context", "query", "--type", "session")
	if code != 2 {
		t.Fatalf("invalid argument exit = %d", code)
	}
}

func TestCLIHelpSurfaceUnchanged(t *testing.T) {
	stdout, _, code := runCLI(t, cliExe(t), map[string]string{}, "", "--help")
	if code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	if !strings.Contains(stdout, "Available Commands:") || !strings.Contains(stdout, "amf") || !strings.Contains(stdout, "smf") {
		t.Fatalf("CLI help changed: %q", stdout)
	}
	if strings.Contains(stdout, "provider") {
		t.Fatalf("provider machine interface must stay hidden from CLI v1 help: %q", stdout)
	}
}

func expectCLIData(t *testing.T, exe string, env map[string]string, stdin string, args ...string) map[string]any {
	t.Helper()
	stdout, stderr, code := runCLI(t, exe, env, stdin, args...)
	if code != 0 {
		t.Fatalf("CLI %v exit = %d, stderr = %s", args, code, stderr)
	}
	return parseData(t, stdout)
}

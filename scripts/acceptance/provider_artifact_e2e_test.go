//go:build acceptance

package acceptance

// P1 process-level artifact acceptance. Every test starts the real
// providerfixture Provider child process over its NDJSON stdin/stdout protocol
// and drives it against:
//   - a deterministic loopback h2c upstream for the three Appendix A
//     acceptance.artifact.* bindings (service key acceptance-artifact), and
//   - an in-process HTTP/1.1 test double of the pinned Host Artifact Store
//     channel (ca-host.artifact-store/v1).
//
// It never contacts the live OpenAMSM lab and no byte is written to a filesystem
// path chosen by the Provider.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	artifactBinaryUpID    = "acceptance.artifact.binary.up"
	artifactBinaryDownID  = "acceptance.artifact.binary.down"
	artifactRoundtripID   = "acceptance.artifact.multipart.roundtrip"
	artifactServiceKey    = "acceptance-artifact"
	artifactUpstreamBytes = "upstream-binary-response-bytes"
	artifactN2Bytes       = "n2-response-bytes"
	artifactN1Bytes       = "n1-response-bytes"
)

// hostChannelDouble is a deterministic test double of the pinned Host Artifact
// Store channel.
type hostChannelDouble struct {
	server *httptest.Server
	token  string

	mutex     sync.Mutex
	inputs    map[string]hostChannelInput
	uploads   []hostChannelUpload
	getCount  int
	postCount int

	getHook  func(writer http.ResponseWriter, request *http.Request, reference string) bool
	postHook func(writer http.ResponseWriter, request *http.Request, upload hostChannelUpload) bool
}

type hostChannelInput struct {
	mediaType string
	body      []byte
}

type hostChannelUpload struct {
	invocationID string
	slot         string
	contentType  string
	body         []byte
	digest       string
}

func newHostChannelDouble(t *testing.T) *hostChannelDouble {
	t.Helper()
	channel := &hostChannelDouble{token: "fixture-process-token", inputs: make(map[string]hostChannelInput)}
	channel.server = httptest.NewServer(http.HandlerFunc(channel.handle))
	t.Cleanup(channel.server.Close)
	return channel
}

func (c *hostChannelDouble) bootstrap() map[string]any {
	return map[string]any{
		"baseUrl":           c.server.URL,
		"processToken":      c.token,
		"requestTimeoutMs":  2000,
		"maxArtifactBytes":  1048576,
		"maxAggregateBytes": 4194304,
		"maxAttachments":    8,
	}
}

func (c *hostChannelDouble) stageInput(mediaType string, body []byte) (string, string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	tag := sha256.Sum256(append([]byte(mediaType), body...))
	reference := "art_" + base64.RawURLEncoding.EncodeToString(tag[:32])
	digest := sha256.Sum256(body)
	c.inputs[reference] = hostChannelInput{mediaType: mediaType, body: body}
	return reference, hex.EncodeToString(digest[:])
}

func (c *hostChannelDouble) snapshot() ([]hostChannelUpload, int, int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]hostChannelUpload(nil), c.uploads...), c.getCount, c.postCount
}

func (c *hostChannelDouble) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Authorization") != "Bearer "+c.token {
		writeHostChannelError(writer, http.StatusUnauthorized, "CHANNEL_UNAUTHORIZED")
		return
	}
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.EscapedPath(), "/v1/artifacts/"):
		reference := strings.TrimPrefix(request.URL.EscapedPath(), "/v1/artifacts/")
		c.mutex.Lock()
		c.getCount++
		input, found := c.inputs[reference]
		hook := c.getHook
		c.mutex.Unlock()
		if hook != nil && hook(writer, request, reference) {
			return
		}
		if !found {
			writeHostChannelError(writer, http.StatusNotFound, "ARTIFACT_NOT_FOUND")
			return
		}
		digest := sha256.Sum256(input.body)
		writer.Header().Set("Content-Type", input.mediaType)
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(input.body)))
		writer.Header().Set("X-CA-Artifact-Sha256", hex.EncodeToString(digest[:]))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(input.body)
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/v1/artifacts":
		body, _ := io.ReadAll(io.LimitReader(request.Body, 8<<20))
		upload := hostChannelUpload{
			invocationID: request.Header.Get("X-CA-Invocation"),
			slot:         request.Header.Get("X-CA-Slot"),
			contentType:  request.Header.Get("Content-Type"),
			body:         body,
			digest:       request.Header.Get("X-CA-Artifact-Sha256"),
		}
		c.mutex.Lock()
		c.postCount++
		hook := c.postHook
		if hook == nil {
			for _, existing := range c.uploads {
				if existing.invocationID == upload.invocationID && existing.slot == upload.slot {
					c.mutex.Unlock()
					writeHostChannelError(writer, http.StatusConflict, "ARTIFACT_SLOT_DUPLICATED")
					return
				}
			}
		}
		c.uploads = append(c.uploads, upload)
		c.mutex.Unlock()
		if hook != nil && hook(writer, request, upload) {
			return
		}
		sum := sha256.Sum256(body)
		tag := sha256.Sum256([]byte(fmt.Sprintf("staged-%d-%s", len(c.uploads), upload.slot)))
		reference := "art_" + base64.RawURLEncoding.EncodeToString(tag[:32])
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"artifactRef": reference,
			"size":        len(body),
			"sha256":      hex.EncodeToString(sum[:]),
		})
	default:
		writeHostChannelError(writer, http.StatusForbidden, "ARTIFACT_FORBIDDEN")
	}
}

func writeHostChannelError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": "fixture channel error"},
	})
}

// artifactAcceptanceUpstream is the deterministic h2c upstream for Appendix A.
type artifactAcceptanceUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	hits   int
	paths  []string
	roots  []map[string]any
}

func newArtifactAcceptanceUpstream(t *testing.T) *artifactAcceptanceUpstream {
	t.Helper()
	upstream := &artifactAcceptanceUpstream{}
	server := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(upstream.handle), &http2.Server{}))
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	upstream.server = server
	return upstream
}

func (u *artifactAcceptanceUpstream) handle(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(request.Body, 8<<20))
	u.mu.Lock()
	u.hits++
	u.paths = append(u.paths, request.URL.EscapedPath())
	u.mu.Unlock()
	switch {
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/acceptance/artifact/binary-up":
		sum := sha256.Sum256(body)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"size":      len(body),
			"sha256":    hex.EncodeToString(sum[:]),
			"mediaType": request.Header.Get("Content-Type"),
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == "/acceptance/artifact/binary-down":
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write([]byte(artifactUpstreamBytes))
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/acceptance/v1/echo":
		writer.Header().Set("Content-Type", "application/json")
		if len(body) == 0 {
			_, _ = io.WriteString(writer, "{}")
			return
		}
		_, _ = writer.Write(body)
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/acceptance/artifact/multipart-roundtrip":
		mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/related" || params["boundary"] == "" {
			writeAcceptanceProblem(writer, http.StatusBadRequest, "MULTIPART_INVALID")
			return
		}
		reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
		rootPart, err := reader.NextPart()
		if err != nil {
			writeAcceptanceProblem(writer, http.StatusBadRequest, "MULTIPART_INVALID")
			return
		}
		rootRaw, _ := io.ReadAll(rootPart)
		_ = rootPart.Close()
		var root map[string]any
		if err := json.Unmarshal(rootRaw, &root); err != nil {
			writeAcceptanceProblem(writer, http.StatusBadRequest, "ROOT_INVALID")
			return
		}
		u.mu.Lock()
		u.roots = append(u.roots, root)
		u.mu.Unlock()
		if !pointerString(root, "/messages/0/n1MessageContent/contentId") {
			writeAcceptanceProblem(writer, http.StatusBadRequest, "ROOT_REFERENCE_MISSING")
			return
		}
		writeAcceptanceMultipartResponse(writer)
	default:
		writeAcceptanceProblem(writer, http.StatusNotFound, "NOT_FOUND")
	}
}

func pointerString(root map[string]any, pointer string) bool {
	segments := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	var current any = root
	for _, segment := range segments {
		switch node := current.(type) {
		case map[string]any:
			current = node[segment]
		case []any:
			index := -1
			if _, err := fmt.Sscanf(segment, "%d", &index); err != nil || index < 0 || index >= len(node) {
				return false
			}
			current = node[index]
		default:
			return false
		}
	}
	text, ok := current.(string)
	return ok && text != ""
}

func writeAcceptanceMultipartResponse(writer http.ResponseWriter) {
	n2ID := "n2-response-content-id"
	n1ID := "n1-response-content-id"
	root := map[string]any{
		"resultId": "roundtrip-1",
		"messages": []any{
			map[string]any{"n2MessageContent": map[string]any{"contentId": n2ID}},
			map[string]any{"n1MessageContent": map[string]any{"contentId": n1ID}},
		},
	}
	rootRaw, _ := json.Marshal(root)
	var wire strings.Builder
	mwriter := multipart.NewWriter(&wire)
	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", "application/json")
	rootPart, _ := mwriter.CreatePart(rootHeader)
	_, _ = rootPart.Write(rootRaw)
	for _, part := range []struct{ id, mediaType, body string }{
		{n2ID, "application/vnd.3gpp.ngap", artifactN2Bytes},
		{n1ID, "application/vnd.3gpp.5gnas", artifactN1Bytes},
	} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", part.mediaType)
		header.Set("Content-ID", part.id)
		partWriter, _ := mwriter.CreatePart(header)
		_, _ = io.WriteString(partWriter, part.body)
	}
	_ = mwriter.Close()
	writer.Header().Set("Content-Type", mime.FormatMediaType("multipart/related", map[string]string{
		"boundary": mwriter.Boundary(),
		"type":     "application/json",
	}))
	_, _ = io.WriteString(writer, wire.String())
}

func writeAcceptanceProblem(writer http.ResponseWriter, status int, cause string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"status": status, "cause": cause})
}

func (u *artifactAcceptanceUpstream) snapshot() (int, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits, append([]string(nil), u.paths...)
}

func artifactServiceConfig(origin string) string {
	return fmt.Sprintf(`version: 1
services:
  %s:
    origin: %s
    protocol: h2c
  %s:
    origin: %s
    protocol: h2c
`, fixtureServiceKey, origin, artifactServiceKey, origin)
}

func startArtifactFixtureProvider(t *testing.T, upstreamOrigin string) *providerProcess {
	t.Helper()
	configPath := writeFixtureServiceConfig(t, artifactServiceConfig(upstreamOrigin))
	env := map[string]string{"CA_PROVIDER_SERVICE_CONFIG": configPath}
	return startProvider(t, fixtureProviderExe(t), env)
}

func artifactInitializeFrame(requestID string, maxCallDurationMs int64, bootstrap map[string]any) frame {
	request := fixtureInitializeFrame(requestID, maxCallDurationMs)
	if bootstrap != nil {
		request.Params["artifactChannel"] = bootstrap
	}
	return request
}

func TestProviderArtifactCatalogAndBinaryRoundtrip(t *testing.T) {
	upstream := newArtifactAcceptanceUpstream(t)
	channel := newHostChannelDouble(t)
	process := startArtifactFixtureProvider(t, upstream.server.URL)

	initialized := expectOK(t, process.roundTrip(artifactInitializeFrame("artifact-init", 30000, channel.bootstrap())))
	effective, ok := initialized["artifactChannel"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result artifactChannel = %#v", initialized["artifactChannel"])
	}
	if fmt.Sprint(effective["maxArtifactBytes"]) != "1048576" || fmt.Sprint(effective["maxAttachments"]) != "8" {
		t.Fatalf("effective artifact limits = %#v", effective)
	}
	if strings.Contains(process.stdoutText(), channel.token) {
		t.Fatalf("stdout frames leaked the process token")
	}

	// Catalog: the three Appendix A bindings are available and publish closed
	// artifactSlots; no origin, path, token or Content-ID is exposed.
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "artifact-catalog",
		[]string{artifactBinaryUpID, artifactBinaryDownID, artifactRoundtripID})))
	if supported := requestStringList(t, catalogResult["supported"]); !reflect.DeepEqual(supported,
		[]string{artifactBinaryUpID, artifactBinaryDownID, artifactRoundtripID}) {
		t.Fatalf("supported = %#v", supported)
	}
	descriptors, ok := catalogResult["bindings"].([]any)
	if !ok || len(descriptors) != 3 {
		t.Fatalf("bindings = %#v", catalogResult["bindings"])
	}
	expectedSlots := map[string]map[string]any{
		artifactBinaryUpID: {
			"inputs":  []any{map[string]any{"slot": "body", "pointer": "/body", "mediaType": "application/octet-stream", "required": true}},
			"outputs": []any{},
		},
		artifactBinaryDownID: {
			"inputs":  []any{},
			"outputs": []any{map[string]any{"slot": "body", "pointer": "", "mediaType": "application/octet-stream", "required": true}},
		},
		artifactRoundtripID: {
			"inputs": []any{
				map[string]any{"slot": "n1Message", "pointer": "/payloads/n1Message", "mediaType": "application/vnd.3gpp.5gnas", "required": true},
				map[string]any{"slot": "n2Message", "pointer": "/payloads/n2Message", "mediaType": "application/vnd.3gpp.ngap", "required": false},
			},
			"outputs": []any{
				map[string]any{"slot": "n2Response", "pointer": "/messages/0/n2MessageContent", "mediaType": "application/vnd.3gpp.ngap", "required": true},
				map[string]any{"slot": "n1Response", "pointer": "/messages/1/n1MessageContent", "mediaType": "application/vnd.3gpp.5gnas", "required": true},
			},
		},
	}
	for index, raw := range descriptors {
		descriptor := raw.(map[string]any)
		operationID := descriptor["backendOperationId"].(string)
		slots, ok := descriptor["artifactSlots"].(map[string]any)
		if !ok {
			t.Fatalf("descriptor %s has no artifactSlots", operationID)
		}
		if !reflect.DeepEqual(slots, expectedSlots[operationID]) {
			t.Fatalf("%s artifactSlots = %#v, want %#v", operationID, slots, expectedSlots[operationID])
		}
		text, _ := json.Marshal(descriptor["artifactSlots"])
		for _, forbidden := range []string{"origin", "http://", "/acceptance/", "contentId", channel.token} {
			if strings.Contains(string(text), forbidden) {
				t.Fatalf("descriptor %s artifactSlots leaked %q: %s", operationID, forbidden, text)
			}
		}
		_ = index
	}

	// Binary upload: reference -> channel GET -> upstream binary body.
	inputBody := []byte("process-binary-request")
	reference, inputDigest := channel.stageInput("application/octet-stream", inputBody)
	up := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "artifact-binary-up", artifactBinaryUpID, map[string]any{
		"body": map[string]any{"artifactRef": reference},
	})))
	if status, _ := number(up["httpStatus"]); status != 200 {
		t.Fatalf("binary up httpStatus = %v", up["httpStatus"])
	}
	upResponse, ok := up["response"].(map[string]any)
	if !ok || upResponse["sha256"] != inputDigest || fmt.Sprint(upResponse["size"]) != fmt.Sprint(len(inputBody)) {
		t.Fatalf("binary up response = %#v", up["response"])
	}

	// Binary download: upstream body -> channel POST -> reference result.
	down := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "artifact-binary-down", artifactBinaryDownID, map[string]any{})))
	downResponse, ok := down["response"].(map[string]any)
	if !ok || len(downResponse) != 1 {
		t.Fatalf("binary down response = %#v", down["response"])
	}
	downReference, ok := downResponse["artifactRef"].(string)
	if !ok || !strings.HasPrefix(downReference, "art_") || len(downReference) != 47 {
		t.Fatalf("binary down reference = %#v", downResponse["artifactRef"])
	}
	uploads, gets, posts := channel.snapshot()
	if gets != 1 || posts != 1 || len(uploads) != 1 {
		t.Fatalf("channel gets=%d posts=%d uploads=%d", gets, posts, len(uploads))
	}
	if uploads[0].slot != "body" || uploads[0].contentType != "application/octet-stream" ||
		string(uploads[0].body) != artifactUpstreamBytes || uploads[0].invocationID == "" {
		t.Fatalf("binary down upload = %+v", uploads[0])
	}
}

func TestProviderArtifactMultipartRoundtrip(t *testing.T) {
	upstream := newArtifactAcceptanceUpstream(t)
	channel := newHostChannelDouble(t)
	process := startArtifactFixtureProvider(t, upstream.server.URL)
	expectOK(t, process.roundTrip(artifactInitializeFrame("artifact-init", 30000, channel.bootstrap())))

	n1Reference, _ := channel.stageInput("application/vnd.3gpp.5gnas", []byte("n1-process-bytes"))
	n2Reference, _ := channel.stageInput("application/vnd.3gpp.ngap", []byte("n2-process-bytes"))
	result := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "artifact-roundtrip", artifactRoundtripID, map[string]any{
		"body": map[string]any{
			"requestId": "process-roundtrip-1",
			"messages": []any{
				map[string]any{"n1MessageContent": map[string]any{"contentId": ""}},
				map[string]any{"n2MessageContent": map[string]any{"contentId": ""}},
			},
		},
		"payloads": map[string]any{
			"n1Message": map[string]any{"artifactRef": n1Reference},
			"n2Message": map[string]any{"artifactRef": n2Reference},
		},
	})))
	if status, _ := number(result["httpStatus"]); status != 200 {
		t.Fatalf("roundtrip httpStatus = %v", result["httpStatus"])
	}
	response, ok := result["response"].(map[string]any)
	if !ok || response["resultId"] != "roundtrip-1" {
		t.Fatalf("roundtrip response = %#v", result["response"])
	}
	messages := response["messages"].([]any)
	n2Slot := messages[0].(map[string]any)["n2MessageContent"].(map[string]any)
	n1Slot := messages[1].(map[string]any)["n1MessageContent"].(map[string]any)
	if len(n2Slot) != 1 || len(n1Slot) != 1 {
		t.Fatalf("projection kept contentId metadata: n2=%#v n1=%#v", n2Slot, n1Slot)
	}
	if _, ok := n2Slot["artifactRef"]; !ok {
		t.Fatalf("n2 slot is not a reference: %#v", n2Slot)
	}
	if _, ok := n1Slot["artifactRef"]; !ok {
		t.Fatalf("n1 slot is not a reference: %#v", n1Slot)
	}

	uploads, _, _ := channel.snapshot()
	if len(uploads) != 2 {
		t.Fatalf("uploads = %d, want 2", len(uploads))
	}
	bySlot := map[string]hostChannelUpload{}
	for _, upload := range uploads {
		bySlot[upload.slot] = upload
	}
	if string(bySlot["n2Response"].body) != artifactN2Bytes || bySlot["n2Response"].contentType != "application/vnd.3gpp.ngap" {
		t.Fatalf("n2Response upload = %+v", bySlot["n2Response"])
	}
	if string(bySlot["n1Response"].body) != artifactN1Bytes || bySlot["n1Response"].contentType != "application/vnd.3gpp.5gnas" {
		t.Fatalf("n1Response upload = %+v", bySlot["n1Response"])
	}
	if hits, _ := upstream.snapshot(); hits != 1 {
		t.Fatalf("upstream hits = %d", hits)
	}
}

func TestProviderArtifactMultipartOptionalPartOmitted(t *testing.T) {
	upstream := newArtifactAcceptanceUpstream(t)
	channel := newHostChannelDouble(t)
	process := startArtifactFixtureProvider(t, upstream.server.URL)
	expectOK(t, process.roundTrip(artifactInitializeFrame("artifact-init-optional", 30000, channel.bootstrap())))

	n1Reference, _ := channel.stageInput("application/vnd.3gpp.5gnas", []byte("only-n1-process-bytes"))
	result := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "artifact-optional", artifactRoundtripID, map[string]any{
		"body": map[string]any{
			"requestId": "process-optional-1",
			"messages": []any{
				map[string]any{"n1MessageContent": map[string]any{"contentId": ""}},
				map[string]any{"n2MessageContent": map[string]any{"contentId": ""}},
			},
		},
		"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": n1Reference}},
	})))
	if status, _ := number(result["httpStatus"]); status != 200 {
		t.Fatalf("optional-part httpStatus = %v", result["httpStatus"])
	}
	response, ok := result["response"].(map[string]any)
	if !ok || response["resultId"] != "roundtrip-1" {
		t.Fatalf("optional-part response = %#v", result["response"])
	}
	messages := response["messages"].([]any)
	if _, ok := messages[0].(map[string]any)["n2MessageContent"].(map[string]any)["artifactRef"]; !ok {
		t.Fatalf("n2 output slot missing: %#v", messages[0])
	}
	if _, ok := messages[1].(map[string]any)["n1MessageContent"].(map[string]any)["artifactRef"]; !ok {
		t.Fatalf("n1 output slot missing: %#v", messages[1])
	}
	uploads, gets, _ := channel.snapshot()
	if gets != 1 {
		t.Fatalf("channel GETs = %d, want only the supplied n1Message reference", gets)
	}
	if len(uploads) != 2 {
		t.Fatalf("uploads = %d, want the two required response parts", len(uploads))
	}
	if hits, _ := upstream.snapshot(); hits != 1 {
		t.Fatalf("upstream hits = %d", hits)
	}
}

func TestProviderArtifactNotConfiguredAndNegativeMatrix(t *testing.T) {
	upstream := newArtifactAcceptanceUpstream(t)
	process := startArtifactFixtureProvider(t, upstream.server.URL)

	// Without artifactChannel: artifact-bearing bindings are NOT_CONFIGURED and
	// rejected before any remote or channel I/O.
	expectOK(t, process.roundTrip(fixtureInitializeFrame("artifact-init-none", 30000)))
	catalogResult := expectOK(t, process.roundTrip(catalogFrame(protocolV2, "artifact-catalog-none",
		[]string{artifactBinaryUpID, artifactBinaryDownID, artifactRoundtripID})))
	if supported := requestStringList(t, catalogResult["supported"]); len(supported) != 0 {
		t.Fatalf("supported without channel = %#v", supported)
	}
	unsupported := catalogResult["unsupported"].([]any)
	if len(unsupported) != 3 {
		t.Fatalf("unsupported = %#v", unsupported)
	}
	for _, raw := range unsupported {
		if raw.(map[string]any)["reason"] != "NOT_CONFIGURED" {
			t.Fatalf("unsupported entry = %#v, want NOT_CONFIGURED", raw)
		}
	}
	notConfigured := process.roundTrip(invokeFrame(protocolV2, "artifact-invoke-none", artifactBinaryUpID, map[string]any{}))
	expectFault(t, notConfigured, "BACKEND_OPERATION_UNSUPPORTED")
	if notConfigured.Error.Detail["reason"] != "NOT_CONFIGURED" || len(notConfigured.Error.Detail) != 1 {
		t.Fatalf("fault detail = %#v", notConfigured.Error.Detail)
	}
	if hits, _ := upstream.snapshot(); hits != 0 {
		t.Fatalf("unconfigured artifact invoke reached the upstream")
	}

	// Re-initialize is rejected; the process continues with a fresh process.
}

func TestProviderArtifactRejectedBeforeIO(t *testing.T) {
	upstream := newArtifactAcceptanceUpstream(t)
	channel := newHostChannelDouble(t)
	process := startArtifactFixtureProvider(t, upstream.server.URL)
	expectOK(t, process.roundTrip(artifactInitializeFrame("artifact-init", 30000, channel.bootstrap())))

	valid := "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	shapeCases := map[string]map[string]any{
		"missing body":          {},
		"plain path at slot":    {"body": "/var/lib/artifacts/input.bin"},
		"bad token":             {"body": map[string]any{"artifactRef": "art_short"}},
		"extra member":          {"body": map[string]any{"artifactRef": valid, "size": 1}},
		"undeclared second ref": {"body": map[string]any{"artifactRef": valid}, "payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}}},
	}
	requestIndex := 0
	for name, arguments := range shapeCases {
		t.Run(name, func(t *testing.T) {
			operation := artifactBinaryUpID
			if name == "undeclared second ref" {
				operation = artifactRoundtripID
			}
			requestIndex++
			decoded := process.roundTrip(invokeFrame(protocolV2, fmt.Sprintf("artifact-shape-%02d", requestIndex), operation, arguments))
			expectFault(t, decoded, "BACKEND_ARGUMENTS_INVALID")
		})
	}

	// Foreign/expired/missing references map to ARTIFACT_REFERENCE_INVALID with
	// zero upstream requests.
	reference, _ := channel.stageInput("application/octet-stream", []byte("bytes"))
	for status, want := range map[int]string{
		404: "ARTIFACT_REFERENCE_INVALID",
		410: "ARTIFACT_REFERENCE_INVALID",
		403: "ARTIFACT_REFERENCE_INVALID",
		413: "ARTIFACT_TRANSFER_FAILED",
		503: "ARTIFACT_TRANSFER_FAILED",
	} {
		channel.getHook = func(writer http.ResponseWriter, _ *http.Request, _ string) bool {
			writeHostChannelError(writer, status, "FIXTURE_ERROR")
			return true
		}
		decoded := process.roundTrip(invokeFrame(protocolV2, fmt.Sprintf("artifact-channel-%d", status), artifactBinaryUpID, map[string]any{
			"body": map[string]any{"artifactRef": reference},
		}))
		expectFault(t, decoded, want)
		if decoded.Error.Detail != nil {
			t.Fatalf("channel fault leaked detail: %#v", decoded.Error.Detail)
		}
	}

	// Output staging failure: no reference is emitted.
	channel.getHook = nil
	channel.postHook = func(writer http.ResponseWriter, _ *http.Request, _ hostChannelUpload) bool {
		writeHostChannelError(writer, http.StatusServiceUnavailable, "INVOCATION_CLOSED")
		return true
	}
	decoded := process.roundTrip(invokeFrame(protocolV2, "artifact-output-fail", artifactBinaryDownID, map[string]any{}))
	expectFault(t, decoded, "ARTIFACT_TRANSFER_FAILED")

	// The process still serves a normal JSON invoke afterwards.
	channel.postHook = nil
	echo := expectOK(t, process.roundTrip(invokeFrame(protocolV2, "artifact-after-fail", echoOperationID, echoArguments())))
	if status, _ := number(echo["httpStatus"]); status != 200 {
		t.Fatalf("echo after artifact faults httpStatus = %v", echo["httpStatus"])
	}
	if hits, _ := upstream.snapshot(); hits != 2 {
		t.Fatalf("upstream hits = %d, want the output-staging call plus the final echo", hits)
	}
}

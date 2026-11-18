//go:build providerfixture

package providerapp

import (
	"context"
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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/provider"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	artifactBinaryUpID          = "acceptance.artifact.binary.up"
	artifactBinaryDownID        = "acceptance.artifact.binary.down"
	artifactMultipartID         = "acceptance.artifact.multipart.roundtrip"
	artifactInputMediaNGAP      = "application/vnd.3gpp.ngap"
	artifactInputMediaNAS       = "application/vnd.3gpp.5gnas"
	artifactBinaryUpstreamBytes = "upstream-binary-response-bytes"
	artifactN2ResponseBytes     = "n2-response-bytes"
	artifactN1ResponseBytes     = "n1-response-bytes"
)

// fakeChannel is a deterministic in-process test double of the pinned Host
// Artifact Store channel. It implements the contract GET/POST endpoints and the
// stable error registry; no byte is ever written to a filesystem.
type fakeChannel struct {
	t      *testing.T
	server *httptest.Server
	token  string

	mutex     sync.Mutex
	inputs    map[string]fakeChannelInput
	uploads   []fakeChannelUpload
	getCount  int
	postCount int

	getHook  func(writer http.ResponseWriter, request *http.Request, reference string) bool
	postHook func(writer http.ResponseWriter, request *http.Request, upload fakeChannelUpload) bool
}

type fakeChannelInput struct {
	mediaType string
	body      []byte
}

type fakeChannelUpload struct {
	invocationID string
	slot         string
	contentType  string
	body         []byte
	digest       string
}

func newFakeChannel(t *testing.T) *fakeChannel {
	t.Helper()
	channel := &fakeChannel{t: t, token: "fixture-process-token", inputs: make(map[string]fakeChannelInput)}
	channel.server = httptest.NewServer(http.HandlerFunc(channel.handle))
	t.Cleanup(channel.server.Close)
	return channel
}

func (c *fakeChannel) config() provider.ArtifactChannelConfig {
	return provider.ArtifactChannelConfig{
		BaseURL:           c.server.URL,
		ProcessToken:      c.token,
		RequestTimeout:    2 * time.Second,
		MaxArtifactBytes:  1 << 20,
		MaxAggregateBytes: 4 << 20,
		MaxAttachments:    8,
	}
}

func (c *fakeChannel) stageInput(mediaType string, body []byte) (string, string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	sum := sha256.Sum256(append([]byte(mediaType), body...))
	reference := "art_" + base64.RawURLEncoding.EncodeToString(sum[:32])
	digest := sha256.Sum256(body)
	c.inputs[reference] = fakeChannelInput{mediaType: mediaType, body: body}
	return reference, hex.EncodeToString(digest[:])
}

func (c *fakeChannel) snapshot() ([]fakeChannelUpload, int, int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]fakeChannelUpload(nil), c.uploads...), c.getCount, c.postCount
}

func (c *fakeChannel) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Authorization") != "Bearer "+c.token {
		writeFakeChannelError(writer, http.StatusUnauthorized, "CHANNEL_UNAUTHORIZED")
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
			writeFakeChannelError(writer, http.StatusNotFound, "ARTIFACT_NOT_FOUND")
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
		upload := fakeChannelUpload{
			invocationID: request.Header.Get("X-CA-Invocation"),
			slot:         request.Header.Get("X-CA-Slot"),
			contentType:  request.Header.Get("Content-Type"),
			body:         body,
			digest:       request.Header.Get("X-CA-Artifact-Sha256"),
		}
		c.mutex.Lock()
		c.postCount++
		hook := c.postHook
		c.mutex.Unlock()
		if hook != nil && hook(writer, request, upload) {
			return
		}
		c.mutex.Lock()
		c.uploads = append(c.uploads, upload)
		index := len(c.uploads)
		c.mutex.Unlock()
		sum := sha256.Sum256(body)
		tag := sha256.Sum256([]byte(fmt.Sprintf("staged-%d-%s", index, upload.slot)))
		reference := "art_" + base64.RawURLEncoding.EncodeToString(tag[:32])
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"artifactRef": reference,
			"size":        len(body),
			"sha256":      hex.EncodeToString(sum[:]),
		})
	default:
		writeFakeChannelError(writer, http.StatusForbidden, "ARTIFACT_FORBIDDEN")
	}
}

func writeFakeChannelError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": "fixture channel error"},
	})
}

// artifactUpstream is the deterministic h2c upstream for the Appendix A
// acceptance bindings. It validates the declared framing and echoes sizes and
// SHA-256 values; it represents no real NF operation.
type artifactUpstream struct {
	server *httptest.Server
	hits   atomic.Int64

	mutex    sync.Mutex
	requests []artifactUpstreamRequest
}

type artifactUpstreamRequest struct {
	path        string
	contentType string
	body        []byte
	root        map[string]any
	parts       map[string]string
}

func newArtifactUpstream(t *testing.T) *artifactUpstream {
	t.Helper()
	upstream := &artifactUpstream{}
	handler := http.HandlerFunc(upstream.handle)
	server := httptest.NewUnstartedServer(h2c.NewHandler(handler, &http2.Server{}))
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	upstream.server = server
	return upstream
}

func (u *artifactUpstream) handle(writer http.ResponseWriter, request *http.Request) {
	u.hits.Add(1)
	body, _ := io.ReadAll(io.LimitReader(request.Body, 8<<20))
	observed := artifactUpstreamRequest{
		path:        request.URL.EscapedPath(),
		contentType: request.Header.Get("Content-Type"),
		body:        body,
	}
	u.mutex.Lock()
	u.requests = append(u.requests, observed)
	u.mutex.Unlock()

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
		_, _ = writer.Write([]byte(artifactBinaryUpstreamBytes))
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/acceptance/artifact/multipart-roundtrip":
		mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/related" || params["boundary"] == "" {
			writeProblem(writer, http.StatusBadRequest, "MULTIPART_INVALID")
			return
		}
		reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
		rootPart, err := reader.NextPart()
		if err != nil {
			writeProblem(writer, http.StatusBadRequest, "MULTIPART_INVALID")
			return
		}
		rootRaw, _ := io.ReadAll(rootPart)
		_ = rootPart.Close()
		var root map[string]any
		if err := json.Unmarshal(rootRaw, &root); err != nil {
			writeProblem(writer, http.StatusBadRequest, "ROOT_INVALID")
			return
		}
		u.mutex.Lock()
		u.requests[len(u.requests)-1].root = root
		u.mutex.Unlock()
		parts := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeProblem(writer, http.StatusBadRequest, "MULTIPART_INVALID")
				return
			}
			data, _ := io.ReadAll(part)
			parts[part.Header.Get("Content-ID")] = part.Header.Get("Content-Type")
			_ = data
			_ = part.Close()
		}
		u.mutex.Lock()
		u.requests[len(u.requests)-1].root = root
		u.requests[len(u.requests)-1].parts = parts
		u.mutex.Unlock()
		if !multipartRootHasContentID(root, "/messages/0/n1MessageContent/contentId") {
			writeProblem(writer, http.StatusBadRequest, "ROOT_REFERENCE_MISSING")
			return
		}
		writeMultipartRoundtripResponse(writer)
	default:
		writeProblem(writer, http.StatusNotFound, "NOT_FOUND")
	}
}

func multipartRootHasContentID(root map[string]any, pointer string) bool {
	segments := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	var current any = root
	for _, segment := range segments {
		switch node := current.(type) {
		case map[string]any:
			current = node[segment]
		case []any:
			index := -1
			_, _ = fmt.Sscanf(segment, "%d", &index)
			if index < 0 || index >= len(node) {
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

func writeMultipartRoundtripResponse(writer http.ResponseWriter) {
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
	for _, responsePart := range []struct {
		contentID   string
		contentType string
		body        string
	}{
		{n2ID, artifactInputMediaNGAP, artifactN2ResponseBytes},
		{n1ID, artifactInputMediaNAS, artifactN1ResponseBytes},
	} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", responsePart.contentType)
		header.Set("Content-ID", responsePart.contentID)
		part, _ := mwriter.CreatePart(header)
		_, _ = io.WriteString(part, responsePart.body)
	}
	_ = mwriter.Close()
	writer.Header().Set("Content-Type", mime.FormatMediaType("multipart/related", map[string]string{
		"boundary": mwriter.Boundary(),
		"type":     "application/json",
	}))
	_, _ = io.WriteString(writer, wire.String())
}

func writeProblem(writer http.ResponseWriter, status int, cause string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"status": status, "cause": cause})
}

func (u *artifactUpstream) snapshot() ([]artifactUpstreamRequest, int64) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	return append([]artifactUpstreamRequest(nil), u.requests...), u.hits.Load()
}

func artifactServiceConfig(t *testing.T, origin string) string {
	t.Helper()
	return writeServiceConfig(t, fmt.Sprintf(`version: 1
services:
  %s:
    origin: %s
    protocol: h2c
  %s:
    origin: %s
    protocol: h2c
`, catalog.AcceptanceFixtureServiceKey, origin, catalog.AcceptanceArtifactServiceKey, origin))
}

func artifactRuntimeWithChannel(t *testing.T, upstream string, channelConfig provider.ArtifactChannelConfig, configure bool) provider.Runtime {
	t.Helper()
	env := map[string]string{EnvServiceConfig: artifactServiceConfig(t, upstream)}
	var resolved provider.Runtime
	var err error
	if configure {
		resolved, err = newRuntime(lookup(env), channelConfig)
	} else {
		resolved, err = newRuntime(lookup(env), provider.ArtifactChannelConfig{})
	}
	if err != nil {
		t.Fatalf("newRuntime: %v", err)
	}
	t.Cleanup(resolved.Close)
	return resolved
}

func faultCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var fault *provider.Fault
	if !faultAs(err, &fault) {
		t.Fatalf("error = %v, want *provider.Fault", err)
	}
	return fault.Code
}

func faultAs(err error, target **provider.Fault) bool {
	for err != nil {
		if fault, ok := err.(*provider.Fault); ok {
			*target = fault
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = next.Unwrap()
	}
	return false
}

func TestRuntimeArtifactAvailabilityRequiresChannel(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, provider.ArtifactChannelConfig{}, false)

	for _, operationID := range []string{artifactBinaryUpID, artifactBinaryDownID, artifactMultipartID} {
		binding, ok := runtime.Binding(operationID)
		if !ok {
			t.Fatalf("%s is not in the fixture catalog", operationID)
		}
		if binding.Available {
			t.Fatalf("%s is available without an artifactChannel", operationID)
		}
		if binding.UnavailableReason != "NOT_CONFIGURED" {
			t.Fatalf("%s unavailable reason = %q, want NOT_CONFIGURED", operationID, binding.UnavailableReason)
		}
		if !binding.ArtifactSlots.Configured() {
			t.Fatalf("%s has no artifactSlots descriptor", operationID)
		}
		_, err := runtime.Invoke(context.Background(), operationID, "inv-availability", map[string]any{})
		if code := faultCode(t, err); code != provider.CodeBackendOperationUnsupported {
			t.Fatalf("%s fault = %s", operationID, code)
		}
	}
	if _, hits := upstream.snapshot(); hits != 0 {
		t.Fatalf("unavailable artifact operations reached the upstream %d times", hits)
	}
	if _, _, posts := channel.snapshot(); posts != 0 {
		t.Fatalf("unavailable artifact operations reached the channel")
	}
}

func TestRuntimeArtifactBinaryRoundtrip(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)

	// Binary upload: the input reference is resolved through the channel before
	// the upstream request, and the upstream sees the exact bytes.
	inputBody := []byte("binary-request-body")
	reference, inputDigest := channel.stageInput("application/octet-stream", inputBody)
	result, err := runtime.Invoke(context.Background(), artifactBinaryUpID, "inv-binary-up", map[string]any{
		"body": map[string]any{"artifactRef": reference},
	})
	if err != nil {
		t.Fatalf("binary up invoke: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("httpStatus = %d", result.HTTPStatus)
	}
	response, ok := result.Response.(map[string]any)
	if !ok {
		t.Fatalf("response = %#v", result.Response)
	}
	if response["mediaType"] != "application/octet-stream" || response["sha256"] != inputDigest {
		t.Fatalf("upstream response = %#v", response)
	}
	requests, _ := upstream.snapshot()
	if len(requests) != 1 || requests[0].path != "/acceptance/artifact/binary-up" ||
		requests[0].contentType != "application/octet-stream" || string(requests[0].body) != string(inputBody) {
		t.Fatalf("upstream request = %+v", requests[0])
	}

	// Binary download: the response body is staged into the Host channel and the
	// Provider result is exactly the reference object.
	down, err := runtime.Invoke(context.Background(), artifactBinaryDownID, "inv-binary-down", map[string]any{})
	if err != nil {
		t.Fatalf("binary down invoke: %v", err)
	}
	if down.HTTPStatus != http.StatusOK {
		t.Fatalf("down httpStatus = %d", down.HTTPStatus)
	}
	downResponse, ok := down.Response.(map[string]any)
	if !ok || len(downResponse) != 1 {
		t.Fatalf("binary down response = %#v", down.Response)
	}
	stagedReference, ok := downResponse["artifactRef"].(string)
	if !ok || !strings.HasPrefix(stagedReference, "art_") || len(stagedReference) != 47 {
		t.Fatalf("binary down reference = %#v", downResponse["artifactRef"])
	}
	uploads, _, _ := channel.snapshot()
	if len(uploads) != 1 || uploads[0].slot != "body" || uploads[0].contentType != "application/octet-stream" ||
		string(uploads[0].body) != artifactBinaryUpstreamBytes {
		t.Fatalf("binary down upload = %+v", uploads)
	}
	expectedDigest := sha256.Sum256([]byte(artifactBinaryUpstreamBytes))
	if uploads[0].digest != hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("binary down digest = %q", uploads[0].digest)
	}
}

func TestRuntimeArtifactMultipartRoundtrip(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)

	n1Body := []byte("n1-message-bytes")
	n2Body := []byte("n2-message-bytes")
	n1Reference, _ := channel.stageInput(artifactInputMediaNAS, n1Body)
	n2Reference, _ := channel.stageInput(artifactInputMediaNGAP, n2Body)
	arguments := map[string]any{
		"body": map[string]any{
			"requestId": "roundtrip-request-1",
			"messages": []any{
				map[string]any{"n1MessageContent": map[string]any{"contentId": ""}},
				map[string]any{"n2MessageContent": map[string]any{"contentId": ""}},
			},
		},
		"payloads": map[string]any{
			"n1Message": map[string]any{"artifactRef": n1Reference},
			"n2Message": map[string]any{"artifactRef": n2Reference},
		},
	}
	result, err := runtime.Invoke(context.Background(), artifactMultipartID, "inv-multipart", arguments)
	if err != nil {
		requests, _ := upstream.snapshot()
		t.Fatalf("multipart invoke: %v (cause=%+v upstream requests: %#v)", err, result.Problem, requests)
	}
	response, ok := result.Response.(map[string]any)
	if !ok {
		cause := ""
		if result.Problem != nil {
			cause = result.Problem.Cause
		}
		binding, _ := runtime.Binding(artifactMultipartID)
		requests, _ := upstream.snapshot()
		t.Fatalf("response = %#v (status=%d cause=%s slots=%#v requests=%#v)", result.Response, result.HTTPStatus, cause, binding.ArtifactSlots, requests)
	}
	if response["resultId"] != "roundtrip-1" {
		t.Fatalf("resultId not preserved: %#v", response)
	}
	messages, ok := response["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", response["messages"])
	}
	n2Slot := messages[0].(map[string]any)["n2MessageContent"].(map[string]any)
	n1Slot := messages[1].(map[string]any)["n1MessageContent"].(map[string]any)
	if _, ok := n2Slot["artifactRef"]; !ok || len(n2Slot) != 1 {
		t.Fatalf("n2 output slot = %#v", n2Slot)
	}
	if _, ok := n1Slot["artifactRef"]; !ok || len(n1Slot) != 1 {
		t.Fatalf("n1 output slot = %#v", n1Slot)
	}

	uploads, _, _ := channel.snapshot()
	if len(uploads) != 2 {
		t.Fatalf("uploads = %d, want 2", len(uploads))
	}
	bySlot := map[string]fakeChannelUpload{}
	for _, upload := range uploads {
		bySlot[upload.slot] = upload
	}
	if string(bySlot["n2Response"].body) != artifactN2ResponseBytes || bySlot["n2Response"].contentType != artifactInputMediaNGAP {
		t.Fatalf("n2Response upload = %+v", bySlot["n2Response"])
	}
	if string(bySlot["n1Response"].body) != artifactN1ResponseBytes || bySlot["n1Response"].contentType != artifactInputMediaNAS {
		t.Fatalf("n1Response upload = %+v", bySlot["n1Response"])
	}

	requests, _ := upstream.snapshot()
	if len(requests) != 1 {
		t.Fatalf("upstream requests = %d", len(requests))
	}
	observed := requests[0]
	if observed.path != "/acceptance/artifact/multipart-roundtrip" || !strings.HasPrefix(observed.contentType, "multipart/related") {
		t.Fatalf("upstream request = %+v", observed)
	}
	if len(observed.parts) != 2 {
		t.Fatalf("upstream multipart parts = %#v", observed.parts)
	}
	for _, wanted := range []string{artifactInputMediaNAS, artifactInputMediaNGAP} {
		found := false
		for _, mediaType := range observed.parts {
			if mediaType == wanted {
				found = true
			}
		}
		if !found {
			t.Fatalf("upstream multipart missing %s: %#v", wanted, observed.parts)
		}
	}
}

func TestRuntimeArtifactReferenceShapeRejectedBeforeIO(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)

	valid := "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	cases := map[string]map[string]any{
		"missing binary body":    {},
		"plain string at slot":   {"body": "/var/lib/artifacts/input.bin"},
		"bad token":              {"body": map[string]any{"artifactRef": "art_short"}},
		"extra member reference": {"body": map[string]any{"artifactRef": valid, "size": 4}},
		"nested reference":       {"body": map[string]any{"artifactRef": map[string]any{"artifactRef": valid}}},
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.Invoke(context.Background(), artifactBinaryUpID, "inv-shape", arguments)
			if code := faultCode(t, err); code != provider.CodeBackendArgumentsInvalid {
				t.Fatalf("fault = %s, want BACKEND_ARGUMENTS_INVALID", code)
			}
		})
	}

	// A reference at an undeclared location is also an argument fault.
	multipartArguments := map[string]any{
		"body": map[string]any{
			"requestId": "r1",
			"messages": []any{
				map[string]any{"n1MessageContent": map[string]any{"contentId": ""}},
			},
			"foreign": map[string]any{"artifactRef": valid},
		},
		"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": valid}},
	}
	if _, err := runtime.Invoke(context.Background(), artifactMultipartID, "inv-foreign-location", multipartArguments); faultCode(t, err) != provider.CodeBackendArgumentsInvalid {
		t.Fatalf("undeclared reference location was not rejected as an argument fault")
	}

	if _, hits := upstream.snapshot(); hits != 0 {
		t.Fatalf("shape-invalid invocations reached the upstream %d times", hits)
	}
	if _, gets, posts := channel.snapshot(); gets != 0 || posts != 0 {
		t.Fatalf("shape-invalid invocations used the channel: gets=%d posts=%d", gets, posts)
	}
}

func TestRuntimeArtifactChannelReferenceErrors(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)
	reference, _ := channel.stageInput("application/octet-stream", []byte("bytes"))

	for status, want := range map[int]string{
		401: provider.CodeArtifactReferenceInvalid,
		403: provider.CodeArtifactReferenceInvalid,
		404: provider.CodeArtifactReferenceInvalid,
		409: provider.CodeArtifactReferenceInvalid,
		410: provider.CodeArtifactReferenceInvalid,
		400: provider.CodeArtifactTransferFailed,
		413: provider.CodeArtifactTransferFailed,
		429: provider.CodeArtifactTransferFailed,
		503: provider.CodeArtifactTransferFailed,
	} {
		channel.getHook = func(writer http.ResponseWriter, _ *http.Request, _ string) bool {
			writeFakeChannelError(writer, status, "FIXTURE_ERROR")
			return true
		}
		_, err := runtime.Invoke(context.Background(), artifactBinaryUpID, "inv-channel", map[string]any{
			"body": map[string]any{"artifactRef": reference},
		})
		if code := faultCode(t, err); code != want {
			t.Fatalf("channel status %d fault = %s, want %s", status, code, want)
		}
	}
	if _, hits := upstream.snapshot(); hits != 0 {
		t.Fatalf("channel failures reached the upstream %d times", hits)
	}
}

func TestRuntimeArtifactTransferFailuresProduceNoResult(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)
	reference, _ := channel.stageInput("application/octet-stream", []byte("stored-bytes"))

	// Digest mismatch on the input stream.
	channel.getHook = func(writer http.ResponseWriter, _ *http.Request, _ string) bool {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", "4")
		writer.Header().Set("X-CA-Artifact-Sha256", strings.Repeat("0", 64))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("diff"))
		return true
	}
	if _, err := runtime.Invoke(context.Background(), artifactBinaryUpID, "inv-digest", map[string]any{
		"body": map[string]any{"artifactRef": reference},
	}); faultCode(t, err) != provider.CodeArtifactTransferFailed {
		t.Fatalf("digest mismatch fault = %v", err)
	}

	// Output staging failure after a successful upstream response: no reference
	// is emitted and no partial result is returned.
	channel.getHook = nil
	channel.postHook = func(writer http.ResponseWriter, _ *http.Request, _ fakeChannelUpload) bool {
		writeFakeChannelError(writer, http.StatusServiceUnavailable, "INVOCATION_CLOSED")
		return true
	}
	result, err := runtime.Invoke(context.Background(), artifactBinaryDownID, "inv-output-fail", map[string]any{})
	if err == nil {
		t.Fatalf("output staging failure returned a result: %#v", result)
	}
	if result.Response != nil || result.HTTPStatus != 0 {
		t.Fatalf("output staging failure leaked a partial result: %#v", result)
	}
	if code := faultCode(t, err); code != provider.CodeArtifactTransferFailed {
		t.Fatalf("output staging fault = %s", code)
	}
}

func TestRuntimeArtifactQuotaAndLimits(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	quotaConfig := channel.config()
	quotaConfig.MaxAttachments = 1
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, quotaConfig, true)

	n1Reference, _ := channel.stageInput(artifactInputMediaNAS, []byte("n1"))
	arguments := map[string]any{
		"body": map[string]any{
			"requestId": "r1",
			"messages": []any{
				map[string]any{"n1MessageContent": map[string]any{"contentId": ""}},
			},
		},
		"payloads": map[string]any{"n1Message": map[string]any{"artifactRef": n1Reference}},
	}
	if _, err := runtime.Invoke(context.Background(), artifactMultipartID, "inv-quota", arguments); faultCode(t, err) != provider.CodeArtifactTransferFailed {
		t.Fatalf("attachment quota fault = %v", err)
	}
}

func TestRuntimeArtifactCancellationAndDeadline(t *testing.T) {
	upstream := newArtifactUpstream(t)
	channel := newFakeChannel(t)
	runtime := artifactRuntimeWithChannel(t, upstream.server.URL, channel.config(), true)
	reference, _ := channel.stageInput("application/octet-stream", []byte("bytes"))
	channel.getHook = func(_ http.ResponseWriter, request *http.Request, _ string) bool {
		<-request.Context().Done()
		return true
	}
	arguments := map[string]any{"body": map[string]any{"artifactRef": reference}}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.Invoke(cancelled, artifactBinaryUpID, "inv-cancel", arguments); err == nil {
		t.Fatal("cancelled artifact invoke returned no error")
	}

	deadline, cancelDeadline := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelDeadline()
	if _, err := runtime.Invoke(deadline, artifactBinaryUpID, "inv-deadline", arguments); err == nil {
		t.Fatal("deadline artifact invoke returned no error")
	}
	if _, hits := upstream.snapshot(); hits != 0 {
		t.Fatalf("cancelled/deadline invocations reached the upstream %d times", hits)
	}
}

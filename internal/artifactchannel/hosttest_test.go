package artifactchannel

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeHost is a deterministic in-process test double of the pinned Host
// Artifact Store channel (`ca-host.artifact-store/v1` §4.2/§4.3/§4.4). It is
// test-only acceptance scaffolding, never shipped.
type fakeHost struct {
	server *httptest.Server
	token  string

	mutex   sync.Mutex
	inputs  map[string]fakeHostInput
	uploads []fakeHostUpload
	gets    []fakeHostRequest
	posts   []fakeHostRequest
	nextID  int

	getHook  func(writer http.ResponseWriter, request *http.Request, reference string) bool
	postHook func(writer http.ResponseWriter, request *http.Request, upload fakeHostUpload) bool
}

type fakeHostInput struct {
	mediaType string
	body      []byte
	// Overrides for negative channel tests. Empty/zero values keep the
	// contract-correct behaviour.
	contentType       string
	contentLength     string
	digest            string
	skipContentLength bool
	bodyOverride      []byte
}

type fakeHostUpload struct {
	invocationID  string
	slot          string
	contentType   string
	declaredSize  int64
	digest        string
	body          []byte
	authorization string
}

type fakeHostRequest struct {
	method        string
	path          string
	authorization string
	invocation    string
	slot          string
}

func newFakeHost(t *testing.T, token string) *fakeHost {
	t.Helper()
	host := &fakeHost{token: token, inputs: make(map[string]fakeHostInput)}
	host.server = httptest.NewServer(http.HandlerFunc(host.handle))
	t.Cleanup(host.server.Close)
	return host
}

func (h *fakeHost) config() Config {
	return Config{
		BaseURL:           h.server.URL,
		ProcessToken:      h.token,
		RequestTimeout:    2_000_000_000,
		MaxArtifactBytes:  1 << 20,
		MaxAggregateBytes: 4 << 20,
		MaxAttachments:    8,
	}
}

func (h *fakeHost) addInput(mediaType string, body []byte) (string, string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.nextID++
	sum := sha256.Sum256([]byte(fmt.Sprintf("input-%d", h.nextID)))
	reference := "art_" + base64.RawURLEncoding.EncodeToString(sum[:32])
	h.inputs[reference] = fakeHostInput{mediaType: mediaType, body: body}
	digest := sha256.Sum256(body)
	return reference, hex.EncodeToString(digest[:])
}

func (h *fakeHost) addRawInput(reference string, input fakeHostInput) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.inputs[reference] = input
}

func (h *fakeHost) snapshot() ([]fakeHostRequest, []fakeHostRequest, []fakeHostUpload) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	gets := append([]fakeHostRequest(nil), h.gets...)
	posts := append([]fakeHostRequest(nil), h.posts...)
	uploads := append([]fakeHostUpload(nil), h.uploads...)
	return gets, posts, uploads
}

func (h *fakeHost) handle(writer http.ResponseWriter, request *http.Request) {
	record := fakeHostRequest{
		method:        request.Method,
		path:          request.URL.EscapedPath(),
		authorization: request.Header.Get("Authorization"),
		invocation:    request.Header.Get("X-CA-Invocation"),
		slot:          request.Header.Get("X-CA-Slot"),
	}
	if record.authorization != "Bearer "+h.token {
		writeChannelError(writer, http.StatusUnauthorized, "CHANNEL_UNAUTHORIZED", "channel authorization failed")
		return
	}
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.EscapedPath(), "/v1/artifacts/"):
		reference := strings.TrimPrefix(request.URL.EscapedPath(), "/v1/artifacts/")
		h.mutex.Lock()
		h.gets = append(h.gets, record)
		input, ok := h.inputs[reference]
		hook := h.getHook
		h.mutex.Unlock()
		if hook != nil && hook(writer, request, reference) {
			return
		}
		if !ok {
			writeChannelError(writer, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "artifact is not resolvable")
			return
		}
		body := input.body
		if input.bodyOverride != nil {
			body = input.bodyOverride
		}
		contentType := input.contentType
		if contentType == "" {
			contentType = input.mediaType
		}
		digest := input.digest
		if digest == "" {
			sum := sha256.Sum256(input.body)
			digest = hex.EncodeToString(sum[:])
		}
		if !input.skipContentLength {
			length := input.contentLength
			if length == "" {
				length = fmt.Sprintf("%d", len(input.body))
			}
			writer.Header().Set("Content-Length", length)
		}
		writer.Header().Set("Content-Type", contentType)
		writer.Header().Set("X-CA-Artifact-Sha256", digest)
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	case request.Method == http.MethodPost && request.URL.EscapedPath() == "/v1/artifacts":
		raw, _ := io.ReadAll(io.LimitReader(request.Body, 8<<20))
		upload := fakeHostUpload{
			invocationID:  record.invocation,
			slot:          record.slot,
			contentType:   request.Header.Get("Content-Type"),
			declaredSize:  request.ContentLength,
			digest:        request.Header.Get("X-CA-Artifact-Sha256"),
			body:          raw,
			authorization: record.authorization,
		}
		h.mutex.Lock()
		h.posts = append(h.posts, record)
		hook := h.postHook
		if hook == nil && h.slotStaged(upload.invocationID, upload.slot) {
			h.mutex.Unlock()
			writeChannelError(writer, http.StatusConflict, "ARTIFACT_SLOT_DUPLICATED", "artifact slot was already uploaded")
			return
		}
		h.uploads = append(h.uploads, upload)
		h.mutex.Unlock()
		if hook != nil && hook(writer, request, upload) {
			return
		}
		if int64(len(raw)) != upload.declaredSize {
			writeChannelError(writer, http.StatusBadRequest, "ARTIFACT_TRANSFER_INCOMPLETE", "artifact upload was not complete")
			return
		}
		sum := sha256.Sum256(raw)
		digestText := hex.EncodeToString(sum[:])
		if upload.digest != "" && upload.digest != digestText {
			writeChannelError(writer, http.StatusBadRequest, "ARTIFACT_TRANSFER_INCOMPLETE", "artifact upload was not complete")
			return
		}
		tag := sha256.Sum256([]byte(fmt.Sprintf("staged-%d-%s", len(h.uploads), upload.slot)))
		reference := "art_" + base64.RawURLEncoding.EncodeToString(tag[:32])
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"artifactRef": reference,
			"size":        len(raw),
			"sha256":      digestText,
		})
	default:
		writeChannelError(writer, http.StatusForbidden, "ARTIFACT_FORBIDDEN", "artifact is not available to this invocation")
	}
}

func (h *fakeHost) slotStaged(invocationID, slot string) bool {
	for _, upload := range h.uploads {
		if upload.invocationID == invocationID && upload.slot == slot {
			return true
		}
	}
	return false
}

func writeChannelError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

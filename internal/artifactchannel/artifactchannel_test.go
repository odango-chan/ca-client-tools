package artifactchannel

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReferenceValue(t *testing.T) {
	valid := map[string]any{"artifactRef": "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	if token, ok := ReferenceValue(valid); !ok || !ValidReferenceToken(token) {
		t.Fatalf("valid reference rejected: %#v", valid)
	}
	for name, value := range map[string]any{
		"plain string": "/var/lib/artifacts/input.bin",
		"extra member": map[string]any{"artifactRef": "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "size": 4},
		"nested":       map[string]any{"artifactRef": map[string]any{"artifactRef": "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}},
		"short token":  map[string]any{"artifactRef": "art_short"},
		"empty object": map[string]any{},
		"nil":          nil,
	} {
		if _, ok := ReferenceValue(value); ok {
			t.Fatalf("%s reference was accepted: %#v", name, value)
		}
	}
}

func TestConfigValidation(t *testing.T) {
	valid := Config{
		BaseURL:           "http://127.0.0.1:53211",
		ProcessToken:      "fixture-process-token-not-a-secret",
		RequestTimeout:    3 * time.Second,
		MaxArtifactBytes:  1 << 20,
		MaxAggregateBytes: 4 << 20,
		MaxAttachments:    8,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]func(*Config){
		"non-loopback host":   func(c *Config) { c.BaseURL = "http://0.0.0.0:53211" },
		"path":                func(c *Config) { c.BaseURL = "http://127.0.0.1:53211/host" },
		"https":               func(c *Config) { c.BaseURL = "https://127.0.0.1:53211" },
		"port":                func(c *Config) { c.BaseURL = "http://127.0.0.1:70000" },
		"empty token":         func(c *Config) { c.ProcessToken = "" },
		"control token":       func(c *Config) { c.ProcessToken = "bad\ntoken" },
		"zero timeout":        func(c *Config) { c.RequestTimeout = 0 },
		"zero artifact bound": func(c *Config) { c.MaxArtifactBytes = 0 },
		"zero attachments":    func(c *Config) { c.MaxAttachments = 0 },
	}
	for name, mutate := range cases {
		candidate := valid
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("%s configuration was accepted", name)
		}
	}
}

func TestOpenResolvesInputAndSendsContractHeaders(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	body := []byte("n1-bytes-exactly")
	reference, digest := host.addInput("application/vnd.3gpp.5gnas", body)
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()

	input, err := channel.Open(context.Background(), reference, "inv-1", "n1Message")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if input.MediaType != "application/vnd.3gpp.5gnas" || input.Size != int64(len(body)) || input.SHA256 != digest {
		t.Fatalf("unexpected input metadata: %+v", input)
	}
	got, err := io.ReadAll(input.Body)
	if err != nil {
		t.Fatalf("read input: %v", err)
	}
	if err := input.Body.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("input bytes = %q", got)
	}
	gets, _, _ := host.snapshot()
	if len(gets) != 1 {
		t.Fatalf("GET requests = %d", len(gets))
	}
	expected := fakeHostRequest{
		method: http.MethodGet, path: "/v1/artifacts/" + reference,
		authorization: "Bearer fixture-token", invocation: "inv-1", slot: "n1Message",
	}
	if gets[0] != expected {
		t.Fatalf("GET request = %+v, want %+v", gets[0], expected)
	}
}

func TestOpenMapsChannelErrors(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	cases := []struct {
		status int
		want   string
	}{
		{401, FaultReferenceInvalid},
		{403, FaultReferenceInvalid},
		{404, FaultReferenceInvalid},
		{409, FaultReferenceInvalid},
		{410, FaultReferenceInvalid},
		{400, FaultTransferFailed},
		{413, FaultTransferFailed},
		{429, FaultTransferFailed},
		{503, FaultTransferFailed},
		{500, FaultTransferFailed},
	}
	for _, testCase := range cases {
		host.getHook = func(writer http.ResponseWriter, _ *http.Request, _ string) bool {
			writeChannelError(writer, testCase.status, "ARTIFACT_FIXTURE_ERROR", "artifact error")
			return true
		}
		_, err := channel.Open(context.Background(), "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "inv-1", "body")
		code, ok := FaultCode(err)
		if !ok || code != testCase.want {
			t.Fatalf("status %d fault = %v, want %s", testCase.status, err, testCase.want)
		}
	}
}

func TestOpenRejectsMalformedControlHeaders(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	body := []byte("bytes")
	cases := map[string]fakeHostInput{
		"missing content type": {mediaType: "", body: body, contentType: " "},
		"bad content length":   {mediaType: "application/octet-stream", body: body, contentLength: "999"},
		"missing digest":       {mediaType: "application/octet-stream", body: body, digest: " "},
		"oversized body":       {mediaType: "application/octet-stream", body: append(body, []byte(strings.Repeat("x", 1<<20))...)},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			reference := "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			host.addRawInput(reference, input)
			opened, err := channel.Open(context.Background(), reference, "inv-1", "body")
			if err != nil {
				if code, ok := FaultCode(err); !ok || code != FaultTransferFailed {
					t.Fatalf("fault = %v, want ARTIFACT_TRANSFER_FAILED", err)
				}
				return
			}
			_, err = io.ReadAll(opened.Body)
			_ = opened.Body.Close()
			if code, ok := FaultCode(err); !ok || code != FaultTransferFailed {
				t.Fatalf("stream fault = %v, want ARTIFACT_TRANSFER_FAILED", err)
			}
		})
	}
}

func TestOpenVerifiesStreamedDigestAndSize(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	body := []byte("correct-bytes")
	reference, _ := host.addInput("application/octet-stream", body)
	host.addRawInput(reference, fakeHostInput{
		mediaType:    "application/octet-stream",
		body:         body,
		bodyOverride: []byte("wrong-bytes!!"),
	})
	input, err := channel.Open(context.Background(), reference, "inv-1", "body")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, readErr := io.ReadAll(input.Body); readErr == nil {
		t.Fatal("digest mismatch was not detected while streaming")
	} else if code, ok := FaultCode(readErr); !ok || code != FaultTransferFailed {
		t.Fatalf("stream fault = %v, want ARTIFACT_TRANSFER_FAILED", readErr)
	}
	_ = input.Body.Close()
}

func TestOpenHonoursCallerContext(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = channel.Open(cancelled, "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "inv-1", "body")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Open error = %v", err)
	}

	host.getHook = func(_ http.ResponseWriter, request *http.Request, _ string) bool {
		<-request.Context().Done()
		return true
	}
	deadline, cancelDeadline := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelDeadline()
	_, err = channel.Open(deadline, "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "inv-1", "body")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline Open error = %v", err)
	}
}

func TestSinkStagesOutputArtifact(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	sink := channel.NewSink("inv-1", NewQuota(4<<20, 8))
	body := []byte("n2-output-bytes")
	attachment, err := sink.StoreSlot(context.Background(), "n2Response", "content-id-1", "application/vnd.3gpp.ngap", strings.NewReader(string(body)), 1<<20)
	if err != nil {
		t.Fatalf("StoreSlot: %v", err)
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	if attachment.Size != int64(len(body)) || attachment.SHA256 != digest || !ValidReferenceToken(attachment.RelativePath) {
		t.Fatalf("unexpected attachment metadata: %+v", attachment)
	}
	refs := sink.Refs()
	if refs["n2Response"] != attachment.RelativePath {
		t.Fatalf("refs = %#v", refs)
	}
	_, posts, uploads := host.snapshot()
	if len(posts) != 1 || len(uploads) != 1 {
		t.Fatalf("posts=%d uploads=%d", len(posts), len(uploads))
	}
	post := posts[0]
	if post.method != http.MethodPost || post.path != "/v1/artifacts" || post.authorization != "Bearer fixture-token" ||
		post.invocation != "inv-1" || post.slot != "n2Response" {
		t.Fatalf("POST request = %+v", post)
	}
	upload := uploads[0]
	if upload.contentType != "application/vnd.3gpp.ngap" || upload.declaredSize != int64(len(body)) ||
		upload.digest != digest || string(upload.body) != string(body) {
		t.Fatalf("upload = %+v", upload)
	}
}

func TestSinkMapsChannelErrorsAndPublishesNothing(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	cases := []struct {
		status int
		want   string
	}{
		{401, FaultReferenceInvalid},
		{409, FaultReferenceInvalid},
		{400, FaultTransferFailed},
		{413, FaultTransferFailed},
		{429, FaultTransferFailed},
		{503, FaultTransferFailed},
	}
	for _, testCase := range cases {
		host.postHook = func(writer http.ResponseWriter, _ *http.Request, _ fakeHostUpload) bool {
			writeChannelError(writer, testCase.status, "ARTIFACT_FIXTURE_ERROR", "artifact error")
			return true
		}
		sink := channel.NewSink("inv-errors", NewQuota(4<<20, 8))
		_, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/vnd.3gpp.ngap", strings.NewReader("bytes"), 1<<20)
		code, ok := FaultCode(err)
		if !ok || code != testCase.want {
			t.Fatalf("status %d fault = %v, want %s", testCase.status, err, testCase.want)
		}
		if len(sink.Refs()) != 0 {
			t.Fatalf("status %d published a reference: %#v", testCase.status, sink.Refs())
		}
	}
}

func TestSinkEnforcesLocalBoundsQuotaAndDuplicates(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()

	// Per-artifact local bound.
	sink := channel.NewSink("inv-bounds", NewQuota(4<<20, 8))
	if _, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader(strings.Repeat("x", 1024)), 8); err == nil {
		t.Fatal("oversized artifact was accepted")
	}

	// Aggregate byte quota.
	quota := NewQuota(4, 8)
	sink = channel.NewSink("inv-quota", quota)
	if _, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader("12345"), 1<<20); err == nil {
		t.Fatal("aggregate quota was exceeded without an error")
	}

	// Attachment count quota.
	sink = channel.NewSink("inv-count", NewQuota(1<<20, 1))
	if _, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader("a"), 1<<20); err != nil {
		t.Fatalf("first staged artifact: %v", err)
	}
	if _, err := sink.StoreSlot(context.Background(), "n1Response", "", "application/octet-stream", strings.NewReader("b"), 1<<20); err == nil {
		t.Fatal("attachment count quota was exceeded without an error")
	}

	// Duplicate slot fails closed and records one reference only.
	sink = channel.NewSink("inv-duplicate", NewQuota(4<<20, 8))
	if _, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader("a"), 1<<20); err != nil {
		t.Fatalf("first staged artifact: %v", err)
	}
	if _, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader("b"), 1<<20); err == nil {
		t.Fatal("duplicate slot was accepted")
	}
	if len(sink.Refs()) != 1 {
		t.Fatalf("refs = %#v", sink.Refs())
	}
}

func TestSinkRejectsMalformed201Result(t *testing.T) {
	host := newFakeHost(t, "fixture-token")
	channel, err := New(host.config())
	if err != nil {
		t.Fatal(err)
	}
	defer channel.CloseIdleConnections()
	cases := map[string]map[string]any{
		"bad ref":  {"artifactRef": "art_short", "size": 5, "sha256": strings.Repeat("a", 64)},
		"bad size": {"artifactRef": "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "size": 999, "sha256": strings.Repeat("a", 64)},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			host.postHook = func(writer http.ResponseWriter, _ *http.Request, _ fakeHostUpload) bool {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(writer).Encode(body)
				return true
			}
			sink := channel.NewSink("inv-201", NewQuota(4<<20, 8))
			_, err := sink.StoreSlot(context.Background(), "n2Response", "", "application/octet-stream", strings.NewReader("bytes"), 1<<20)
			if code, ok := FaultCode(err); !ok || code != FaultTransferFailed {
				t.Fatalf("fault = %v, want ARTIFACT_TRANSFER_FAILED", err)
			}
			if len(sink.Refs()) != 0 {
				t.Fatal("malformed staging result published a reference")
			}
		})
	}
}

func TestDigestVectors(t *testing.T) {
	// The shared digest vectors are pinned conformance input; verify the
	// standard-library digest computation we rely on.
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "host-artifact-store", "v1", "digest-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Vectors []struct {
			Name   string `json:"name"`
			Size   int    `json:"size"`
			SHA256 string `json:"sha256"`
			Base64 string `json:"base64"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Vectors) == 0 {
		t.Fatal("digest vectors are missing")
	}
	for _, vector := range document.Vectors {
		body, err := base64.StdEncoding.DecodeString(vector.Base64)
		if err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		if len(body) != vector.Size {
			t.Fatalf("%s: size = %d, want %d", vector.Name, len(body), vector.Size)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != vector.SHA256 {
			t.Fatalf("%s: digest mismatch", vector.Name)
		}
	}
}

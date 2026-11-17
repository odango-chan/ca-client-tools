package payload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

type memoryResolver map[string][]byte

func (r memoryResolver) Open(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	body, ok := r[name]
	if !ok {
		return nil, 0, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
}

type memorySink struct {
	bodies map[string][]byte
}

func (s *memorySink) Store(ctx context.Context, contentID, contentType string, source io.Reader, maximum int64) (Attachment, error) {
	body, err := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: source}, maximum+1))
	if err != nil {
		return Attachment{}, err
	}
	if int64(len(body)) > maximum {
		return Attachment{}, ErrPayloadTooLarge
	}
	hash := sha256.Sum256(body)
	if s.bodies == nil {
		s.bodies = map[string][]byte{}
	}
	s.bodies[contentID] = body
	return Attachment{
		ContentID: contentID, RelativePath: "memory", ContentType: contentType,
		Size: int64(len(body)), SHA256: hex.EncodeToString(hash[:]),
	}, nil
}

func TestBuildJSONAndBinary(t *testing.T) {
	limits := Limits{MaxJSONBytes: 64, MaxBinaryBytes: 64, MaxTotalBytes: 128, MaxParts: 4}
	jsonBody, err := BuildJSON([]byte(`{"ueContextId":"ue-1"}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	if jsonBody.ContentType != "application/json" || jsonBody.ContentLength != int64(len(`{"ueContextId":"ue-1"}`)) {
		t.Fatalf("unexpected JSON metadata: %+v", jsonBody)
	}
	got, err := io.ReadAll(jsonBody.Body)
	if err != nil || string(got) != `{"ueContextId":"ue-1"}` {
		t.Fatalf("unexpected JSON body %q, %v", got, err)
	}
	jsonBody.Body.Close()

	resolver := memoryResolver{"request/n1.bin": {0x00, 0x01, 0xff}}
	binaryBody, err := BuildBinary(context.Background(), resolver, Source{RelativePath: "request/n1.bin", Size: 3}, "application/vnd.3gpp.5gnas", limits)
	if err != nil {
		t.Fatal(err)
	}
	defer binaryBody.Body.Close()
	if binaryBody.ContentLength != -1 || binaryBody.ContentType != "application/vnd.3gpp.5gnas" {
		t.Fatalf("unexpected binary metadata: %+v", binaryBody)
	}
	got, err = io.ReadAll(binaryBody.Body)
	if err != nil || !bytes.Equal(got, []byte{0, 1, 0xff}) {
		t.Fatalf("unexpected binary body %v, %v", got, err)
	}
}

func TestBuildBinaryRejectsSizeMismatch(t *testing.T) {
	resolver := memoryResolver{"request/n1.bin": []byte("longer")}
	_, err := BuildBinary(context.Background(), resolver, Source{RelativePath: "request/n1.bin", Size: 3}, "application/octet-stream", Limits{})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected staged-size mismatch rejection, got %v", err)
	}
}

func TestBuildMultipartRelatedStreamsMultipleTypedParts(t *testing.T) {
	root := []byte(`{"n1SmMsg":{"contentId":""},"n2SmInfo":{"contentId":""}}`)
	resolver := memoryResolver{
		"request/n1.bin": {0x01, 0x02},
		"request/n2.bin": {0x03, 0x04, 0x05},
	}
	spec := MultipartSpec{Parts: []PartBinding{
		{Name: "n1Payload", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1SmMsg/contentId", Required: true},
		{Name: "n2Payload", ContentType: "application/vnd.3gpp.ngap", ContentIDPointer: "/n2SmInfo/contentId", Required: true},
	}}
	encoded, err := BuildMultipartRelated(context.Background(), resolver, root, spec, map[string]Source{
		"n1Payload": {RelativePath: "request/n1.bin", Size: 2},
		"n2Payload": {RelativePath: "request/n2.bin", Size: 3},
	}, Limits{MaxJSONBytes: 1024, MaxBinaryBytes: 1024, MaxTotalBytes: 4096, MaxParts: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer encoded.Body.Close()
	mediaType, params, err := mime.ParseMediaType(encoded.ContentType)
	if err != nil || mediaType != "multipart/related" || params["type"] != "application/json" || params["boundary"] == "" {
		t.Fatalf("unexpected multipart Content-Type %q: %v", encoded.ContentType, err)
	}
	reader := multipart.NewReader(encoded.Body, params["boundary"])
	rootPart, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := io.ReadAll(rootPart)
	rootPart.Close()
	if err != nil {
		t.Fatal(err)
	}
	var rootObject map[string]map[string]string
	if err := json.Unmarshal(rootBody, &rootObject); err != nil {
		t.Fatal(err)
	}
	n1ID := rootObject["n1SmMsg"]["contentId"]
	n2ID := rootObject["n2SmInfo"]["contentId"]
	if n1ID == "" || n2ID == "" || n1ID == n2ID {
		t.Fatalf("invalid generated Content-IDs: %q %q", n1ID, n2ID)
	}
	want := map[string][]byte{n1ID: {0x01, 0x02}, n2ID: {0x03, 0x04, 0x05}}
	wantTypes := map[string]string{n1ID: "application/vnd.3gpp.5gnas", n2ID: "application/vnd.3gpp.ngap"}
	for i := 0; i < 2; i++ {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		rawID := part.Header.Get("Content-ID")
		id, err := normalizeContentID(rawID)
		if err != nil {
			t.Fatal(err)
		}
		if rawID != id {
			t.Fatalf("writer must place the same Content-ID value in the MIME header and JSON root: %q != %q", rawID, id)
		}
		if part.Header.Get("Content-Type") != wantTypes[id] {
			t.Fatalf("unexpected part content type for %s: %s", id, part.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(part)
		part.Close()
		if err != nil || !bytes.Equal(body, want[id]) {
			t.Fatalf("unexpected part %s body %v, %v", id, body, err)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing multipart parts: %v", want)
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("expected exactly three parts, got %v", err)
	}
}

func TestBuildMultipartRejectsUnboundOrMissingParts(t *testing.T) {
	resolver := memoryResolver{"request/n1.bin": []byte("n1")}
	spec := MultipartSpec{Parts: []PartBinding{{Name: "n1", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1/contentId", Required: true}}}
	_, err := BuildMultipartRelated(context.Background(), resolver, []byte(`{"n1":{}}`), spec, nil, Limits{})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected missing required part, got %v", err)
	}
	_, err = BuildMultipartRelated(context.Background(), resolver, []byte(`{"n1":{"contentId":""}}`), spec, map[string]Source{
		"n1":         {RelativePath: "request/n1.bin", Size: 2},
		"unexpected": {RelativePath: "request/n1.bin", Size: 2},
	}, Limits{})
	if !errors.Is(err, ErrUnknownPayloadPart) {
		t.Fatalf("expected unbound part rejection, got %v", err)
	}
}

func TestDecodeMultipartRelatedStoresReferencedAttachments(t *testing.T) {
	var wire bytes.Buffer
	writer := multipart.NewWriter(&wire)
	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(rootHeader)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, `{"n1SmMsg":{"contentId":"n1-id"},"n4Info":{"contentId":"n4-id"}}`)
	for _, item := range []struct{ id, media, body string }{
		{"n1-id", "application/vnd.3gpp.5gnas", "n1-bytes"},
		{"n4-id", "application/vnd.3gpp.pfcp", "n4-bytes"},
	} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", item.media)
		header.Set("Content-ID", item.id)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(part, item.body)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	contentType := mime.FormatMediaType("multipart/related", map[string]string{"boundary": writer.Boundary(), "type": "application/json"})
	sink := &memorySink{}
	parsed, err := DecodeResponse(context.Background(), contentType, bytes.NewReader(wire.Bytes()), ResponseSpec{
		Kind: KindMultipartRelated,
		Multipart: MultipartSpec{Parts: []PartBinding{
			{Name: "n1Payload", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1SmMsg/contentId", Required: true},
			{Name: "n4Payload", ContentType: "application/vnd.3gpp.pfcp", ContentIDPointer: "/n4Info/contentId", Required: true},
		}},
	}, sink, Limits{MaxJSONBytes: 1024, MaxBinaryBytes: 1024, MaxTotalBytes: 4096, MaxParts: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Attachments) != 2 || string(sink.bodies["n1-id"]) != "n1-bytes" || string(sink.bodies["n4-id"]) != "n4-bytes" {
		t.Fatalf("unexpected attachments: %+v, bodies=%v", parsed.Attachments, sink.bodies)
	}
	if len(parsed.JSON) == 0 {
		t.Fatal("JSON root was not returned")
	}
}

func TestDecodeMultipartRejectsUnknownDuplicateAndMissingParts(t *testing.T) {
	makeBody := func(ids []string) (string, []byte) {
		var wire bytes.Buffer
		writer := multipart.NewWriter(&wire)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", "application/json")
		root, _ := writer.CreatePart(header)
		_, _ = io.WriteString(root, `{"n1":{"contentId":"expected"}}`)
		for _, id := range ids {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Type", "application/vnd.3gpp.5gnas")
			header.Set("Content-ID", id)
			part, _ := writer.CreatePart(header)
			_, _ = io.WriteString(part, "bytes")
		}
		_ = writer.Close()
		ct := mime.FormatMediaType("multipart/related", map[string]string{"boundary": writer.Boundary(), "type": "application/json"})
		return ct, wire.Bytes()
	}
	spec := ResponseSpec{Kind: KindMultipartRelated, Multipart: MultipartSpec{Parts: []PartBinding{{Name: "n1", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1/contentId", Required: true}}}}
	for _, test := range []struct {
		name string
		ids  []string
	}{
		{"unknown", []string{"other"}},
		{"duplicate", []string{"expected", "expected"}},
		{"missing", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			ct, body := makeBody(test.ids)
			_, err := DecodeResponse(context.Background(), ct, bytes.NewReader(body), spec, &memorySink{}, Limits{})
			if err == nil {
				t.Fatal("expected malformed multipart to be rejected")
			}
		})
	}
}

func TestDecodeResponseEnforcesLimitsAndMediaType(t *testing.T) {
	limits := Limits{MaxJSONBytes: 4, MaxBinaryBytes: 4, MaxTotalBytes: 8, MaxParts: 4}
	_, err := DecodeResponse(context.Background(), "application/json", strings.NewReader(`{"x":1}`), ResponseSpec{Kind: KindJSON}, nil, limits)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected JSON limit error, got %v", err)
	}
	_, err = DecodeResponse(context.Background(), "application/problem+json", strings.NewReader(`{}`), ResponseSpec{Kind: KindJSON}, nil, limits)
	if err == nil {
		t.Fatal("expected unexpected JSON media type to be rejected")
	}
}

func TestDirectoryResolverRejectsTraversalAndSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "request"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "request", "part.bin"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	reader, size, err := resolver.Open(context.Background(), "request/part.bin")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || size != 7 || string(body) != "payload" {
		t.Fatalf("unexpected resolved file: %q %d %v", body, size, err)
	}
	if _, _, err := resolver.Open(context.Background(), "../outside"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	t.Run("symlink", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.bin")
		if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "request", "link.bin")); err != nil {
			if isSymlinkPrivilegeError(err) {
				// Windows without SeCreateSymbolicLinkPrivilege (e.g. an
				// unelevated account) cannot create the fixture; only this
				// sub-case is skipped. Traversal and regular-file checks above
				// and the Linux/WSL symlink rejection still run.
				t.Skipf("cannot create symlink fixture without privilege: %v", err)
			}
			t.Fatal(err)
		}
		if _, _, err := resolver.Open(context.Background(), "request/link.bin"); err == nil {
			t.Fatal("expected symlink rejection")
		}
	})
}

// isSymlinkPrivilegeError reports whether symlink creation failed because the
// process lacks the Windows SeCreateSymbolicLinkPrivilege (or permission).
// Symlink creation failures on other platforms are never treated as skips.
func isSymlinkPrivilegeError(err error) bool {
	if runtime.GOOS != "windows" || err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		// ERROR_PRIVILEGE_NOT_HELD (1314): the required privilege is not held.
		return errno == syscall.Errno(1314)
	}
	return false
}

func TestDirectorySinkWritesGeneratedNamesAndDigest(t *testing.T) {
	root := t.TempDir()
	sink, err := NewDirectorySink(root, Limits{MaxJSONBytes: 32, MaxBinaryBytes: 32, MaxTotalBytes: 64, MaxParts: 4})
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := sink.Store(context.Background(), "n1-id", "application/vnd.3gpp.5gnas", strings.NewReader("payload"), 16)
	if err != nil {
		t.Fatal(err)
	}
	if attachment.RelativePath != "part-0001.bin" || attachment.ContentID != "n1-id" || attachment.Size != 7 {
		t.Fatalf("unexpected attachment: %+v", attachment)
	}
	want := sha256.Sum256([]byte("payload"))
	if attachment.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("unexpected digest: %s", attachment.SHA256)
	}
	if _, err := os.Stat(filepath.Join(root, attachment.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Store(context.Background(), "too-large", "application/octet-stream", strings.NewReader(strings.Repeat("x", 20)), 8); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected sink size limit, got %v", err)
	}
}

func TestBuildMultipartHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := memoryResolver{"request/n1.bin": []byte("n1")}
	_, err := BuildMultipartRelated(ctx, resolver, []byte(`{"n1":{"contentId":""}}`), MultipartSpec{Parts: []PartBinding{{Name: "n1", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1/contentId", Required: true}}}, map[string]Source{"n1": {RelativePath: "request/n1.bin", Size: 2}}, Limits{})
	if err == nil {
		t.Fatal("expected cancelled payload construction to fail")
	}
}

func TestBuildJSONRejectsNestedDuplicateKeys(t *testing.T) {
	for _, body := range []string{
		`{"ueContextId":"ue-1","ueContextId":"ue-2"}`,
		`{"a":{"b":1,"b":2}}`,
		`{"a":[{"c":1,"c":2}]}`,
	} {
		if _, err := BuildJSON([]byte(body), Limits{}); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected nested duplicate-key rejection for %s, got %v", body, err)
		}
	}
	if _, err := BuildJSON([]byte(`{"a":{"b":1,"c":[{"d":2}]}}`), Limits{}); err != nil {
		t.Fatalf("unique keys must still be accepted, got %v", err)
	}
}

func TestDecodeResponseJSONRejectsNestedDuplicateKeys(t *testing.T) {
	for _, body := range []string{
		`{"a":{"b":1,"b":2}}`,
		`{"a":[{"c":1,"c":2}]}`,
	} {
		if _, err := DecodeResponse(context.Background(), "application/json", strings.NewReader(body), ResponseSpec{Kind: KindJSON}, nil, Limits{}); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected nested duplicate-key rejection for %s, got %v", body, err)
		}
	}
	if _, err := DecodeResponse(context.Background(), "application/json", strings.NewReader(`{"a":{"b":1}}`), ResponseSpec{Kind: KindJSON}, nil, Limits{}); err != nil {
		t.Fatalf("unique keys must still be accepted, got %v", err)
	}
}

func TestMultipartRootRejectsNestedDuplicateKeys(t *testing.T) {
	root := []byte(`{"n1":{"contentId":"one","contentId":"two"}}`)
	makeSpec := func() MultipartSpec {
		return MultipartSpec{Parts: []PartBinding{{
			Name: "n1", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1/contentId", Required: true,
		}}}
	}
	t.Run("build", func(t *testing.T) {
		_, err := BuildMultipartRelated(context.Background(), memoryResolver{}, root, makeSpec(), nil, Limits{})
		if !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected duplicate multipart root keys to be rejected, got %v", err)
		}
	})
	t.Run("decode", func(t *testing.T) {
		var wire bytes.Buffer
		writer := multipart.NewWriter(&wire)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", "application/json")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(root); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		contentType := mime.FormatMediaType("multipart/related", map[string]string{"boundary": writer.Boundary(), "type": "application/json"})
		_, err = DecodeResponse(context.Background(), contentType, bytes.NewReader(wire.Bytes()), ResponseSpec{
			Kind: KindMultipartRelated, Multipart: makeSpec(),
		}, &memorySink{}, Limits{})
		if !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected duplicate multipart root keys to be rejected, got %v", err)
		}
	})
}

func TestComposeLimitsKeepsPayloadsWithinTransportBound(t *testing.T) {
	// Payload defaults must never exceed the shared transport default and must
	// leave the multipart framing reserve free inside the aggregate bound.
	defaults, err := normalizeLimits(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.MaxJSONBytes > DefaultMaxTransportBytes ||
		defaults.MaxBinaryBytes > DefaultMaxTransportBytes ||
		defaults.MaxTotalBytes+MultipartFramingReserve > DefaultMaxTransportBytes {
		t.Fatalf("payload defaults exceed the transport default: %+v", defaults)
	}

	const transportBound int64 = 1 << 20
	composed, err := ComposeLimits(Limits{
		MaxJSONBytes: 1 << 30, MaxBinaryBytes: 1 << 30, MaxTotalBytes: 1 << 30, MaxParts: 8,
	}, transportBound, transportBound)
	if err != nil {
		t.Fatal(err)
	}
	if composed.MaxJSONBytes != transportBound-MultipartFramingReserve ||
		composed.MaxBinaryBytes != transportBound-MultipartFramingReserve ||
		composed.MaxTotalBytes != transportBound-MultipartFramingReserve {
		t.Fatalf("limits did not compose down to the transport bound: %+v", composed)
	}

	// A JSON body accepted by the engine stays within the transport request bound.
	exactJSON := []byte(`{"k":"` + strings.Repeat("x", int(composed.MaxJSONBytes)-8) + `"}`)
	encoded, err := BuildJSON(exactJSON, composed)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.ContentLength > transportBound {
		t.Fatalf("accepted JSON body %d exceeds the transport bound %d", encoded.ContentLength, transportBound)
	}
	body, err := io.ReadAll(encoded.Body)
	encoded.Body.Close()
	if err != nil || int64(len(body)) != encoded.ContentLength {
		t.Fatalf("unexpected accepted JSON body length %d, %v", len(body), err)
	}
	// One extra byte is rejected by the engine before the transport is invoked.
	if _, err := BuildJSON(append(append([]byte(nil), exactJSON...), ' '), composed); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected oversized JSON to be rejected before transport, got %v", err)
	}

	// A binary body accepted by the engine also stays within the transport bound.
	accepted := int64(composed.MaxTotalBytes)
	resolver := memoryResolver{"request/part.bin": make([]byte, accepted)}
	binaryBody, err := BuildBinary(context.Background(), resolver, Source{RelativePath: "request/part.bin", Size: accepted}, "application/octet-stream", composed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, binaryBody.Body); err != nil {
		t.Fatal(err)
	}
	binaryBody.Body.Close()
	oversized := memoryResolver{"request/part.bin": make([]byte, accepted+1)}
	if _, err := BuildBinary(context.Background(), oversized, Source{RelativePath: "request/part.bin", Size: accepted + 1}, "application/octet-stream", composed); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected oversized binary to be rejected before transport, got %v", err)
	}

	// Multipart framing stays inside the documented reserve: the encoded wire
	// body (root JSON plus parts plus MIME framing) fits the transport bound.
	root := []byte(`{"n1":{"contentId":""}}`)
	const generatedContentIDLength = 32
	partSize := composed.MaxTotalBytes - int64(len(root)) - generatedContentIDLength
	if partSize <= 0 {
		t.Fatalf("composed limits leave no room for multipart parts: %+v", composed)
	}
	spec := MultipartSpec{Parts: []PartBinding{{
		Name: "n1", ContentType: "application/vnd.3gpp.5gnas", ContentIDPointer: "/n1/contentId", Required: true,
	}}}
	multipartBody, err := BuildMultipartRelated(context.Background(),
		memoryResolver{"request/n1.bin": make([]byte, partSize)},
		root, spec, map[string]Source{"n1": {RelativePath: "request/n1.bin", Size: partSize}}, composed)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := io.ReadAll(multipartBody.Body)
	multipartBody.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(wire)) > transportBound {
		t.Fatalf("multipart wire body %d exceeds the transport bound %d", len(wire), transportBound)
	}
	if int64(len(wire)) <= partSize {
		t.Fatalf("multipart wire body %d does not contain the binary part of %d bytes", len(wire), partSize)
	}
}

func TestDecodeResponseKindNoneRequiresEmptyBody(t *testing.T) {
	if _, err := DecodeResponse(context.Background(), "application/json", strings.NewReader(""), ResponseSpec{Kind: KindNone}, nil, Limits{}); err != nil {
		t.Fatalf("empty body must be accepted for KindNone, got %v", err)
	}
	for _, body := range []string{"x", " ", `{}`, strings.Repeat("x", 4096)} {
		if _, err := DecodeResponse(context.Background(), "application/json", strings.NewReader(body), ResponseSpec{Kind: KindNone}, nil, Limits{}); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("expected unexpected %d-byte KindNone body to be rejected, got %v", len(body), err)
		}
	}
}

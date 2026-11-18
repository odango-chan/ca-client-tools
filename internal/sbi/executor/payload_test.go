package executor

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
	"net/http"
	"net/textproto"
	"strconv"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

func TestExecuteBinaryRequestBody(t *testing.T) {
	payloadBytes := []byte{0x01, 0x02, 0x03, 0xfe}
	type observed struct {
		contentType string
		body        []byte
	}
	observations := make(chan observed, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		observations <- observed{
			contentType: request.Header.Get("Content-Type"),
			body:        body,
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	client := newTestClient(t, server, nil)
	resolver := newFakeResolver(map[string][]byte{"n1": payloadBytes})
	executor := newTestExecutor(t, client, resolver, payload.Limits{})

	operation := Operation{
		OperationID:  "Test_SendN1Message",
		Effect:       EffectMutation,
		ServiceKey:   testServiceKey,
		Method:       http.MethodPost,
		PathTemplate: "/n1-messages",
		Body: &BodySpec{
			Kind:      PayloadBinary,
			MediaType: "application/vnd.3gpp.5gnas",
		},
		Idempotency: &Idempotency{Mode: "NONE"},
	}
	result, err := executor.Execute(context.Background(), operation, map[string]any{"body": "n1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusNoContent {
		t.Fatalf("unexpected status %d", result.HTTPStatus)
	}
	got := <-observations
	if got.contentType != "application/vnd.3gpp.5gnas" {
		t.Fatalf("unexpected content type %q", got.contentType)
	}
	if !bytes.Equal(got.body, payloadBytes) {
		t.Fatalf("unexpected binary body: %x", got.body)
	}
	if resolver.opens.Load() != 1 {
		t.Fatalf("expected exactly one resolver open, got %d", resolver.opens.Load())
	}

	// An unknown Host payload name fails before any network request.
	_, err = executor.Execute(context.Background(), operation, map[string]any{"body": "missing"})
	var inputError *InputError
	if !errors.As(err, &inputError) {
		t.Fatalf("expected *InputError for unknown payload, got %T: %v", err, err)
	}

	// A binary operation with no resolver is a configuration failure.
	noResolver := newTestExecutor(t, client, nil, payload.Limits{})
	_, err = noResolver.Execute(context.Background(), operation, map[string]any{"body": "n1"})
	var configurationError *ConfigurationError
	if !errors.As(err, &configurationError) {
		t.Fatalf("expected *ConfigurationError for missing resolver, got %T: %v", err, err)
	}
}

func TestExecuteSingleBinaryResponse(t *testing.T) {
	responseBytes := []byte{0xde, 0xad, 0xbe, 0xef}
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/vnd.3gpp.ngap")
		_, _ = writer.Write(responseBytes)
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})
	operation := Operation{
		OperationID:        "Test_GetBinary",
		Effect:             EffectRead,
		ServiceKey:         testServiceKey,
		Method:             http.MethodGet,
		PathTemplate:       "/n2-messages/{id}",
		PathParameters:     []Parameter{{Name: "id", Type: "string", Required: true}},
		ResponseMediaTypes: []string{"application/vnd.3gpp.ngap"},
	}
	result, err := executor.Execute(context.Background(), operation, map[string]any{"id": "m1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.MediaType != "application/vnd.3gpp.ngap" {
		t.Fatalf("unexpected media type %q", result.MediaType)
	}
	if !bytes.Equal(result.Body, responseBytes) {
		t.Fatalf("unexpected binary response: %x", result.Body)
	}
	if result.JSON != nil || len(result.Attachments) != 0 {
		t.Fatalf("unexpected decoded response fields: %+v", result)
	}
}

func TestExecuteMultipartRequest(t *testing.T) {
	n1Bytes := []byte{0x01, 0x02, 0x03}
	n2Bytes := []byte{0xaa, 0xbb}
	type receivedPart struct {
		contentID   string
		contentType string
		data        []byte
	}
	type received struct {
		mediaType string
		rootType  string
		root      map[string]any
		parts     []receivedPart
	}
	observations := make(chan received, 1)
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil {
			http.Error(writer, "bad content type", http.StatusBadRequest)
			return
		}
		reader := multipart.NewReader(request.Body, params["boundary"])
		var observation received
		observation.mediaType = mediaType
		observation.rootType = params["type"]
		for index := 0; ; index++ {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(writer, "bad multipart", http.StatusBadRequest)
				return
			}
			data, _ := io.ReadAll(part)
			if index == 0 {
				if part.Header.Get("Content-Type") != "application/json" {
					http.Error(writer, "root is not JSON", http.StatusBadRequest)
					return
				}
				if err := json.Unmarshal(data, &observation.root); err != nil {
					http.Error(writer, "bad root JSON", http.StatusBadRequest)
					return
				}
				continue
			}
			observation.parts = append(observation.parts, receivedPart{
				contentID:   part.Header.Get("Content-ID"),
				contentType: part.Header.Get("Content-Type"),
				data:        data,
			})
		}
		observations <- observation
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"cause":"OK"}`)
	}))
	client := newTestClient(t, server, nil)
	resolver := newFakeResolver(map[string][]byte{"n1": n1Bytes, "n2": n2Bytes})
	executor := newTestExecutor(t, client, resolver, payload.Limits{})

	operation := Operation{
		OperationID:  "Test_N1N2MessageTransfer",
		Effect:       EffectMutation,
		ServiceKey:   testServiceKey,
		Method:       http.MethodPost,
		PathTemplate: "/n1-n2-messages",
		Body: &BodySpec{
			Kind:          PayloadMultipart,
			MediaType:     "multipart/related",
			MultipartRoot: true,
			MaxParts:      4,
			Schema: mustSchema(t, `{
				"type":"object",
				"required":["n1MessageContent","n2MessageContent"],
				"properties":{
					"n1MessageContent":{"type":"string"},
					"n2MessageContent":{"type":"string"}
				}
			}`),
			Parts: []MultipartPart{
				{Name: "n1Payload", ContentType: "application/vnd.3gpp.5gnas", JSONPointerField: "/n1MessageContent", Required: true},
				{Name: "n2Payload", ContentType: "application/vnd.3gpp.ngap", JSONPointerField: "/n2MessageContent", Required: true},
			},
		},
		ResponseMediaTypes: []string{"application/json"},
		ResponseSchema:     mustSchema(t, `{"type":"object","required":["cause"],"properties":{"cause":{"type":"string"}}}`),
		Idempotency:        &Idempotency{Mode: "NONE"},
	}
	result, err := executor.Execute(context.Background(), operation, map[string]any{
		"body": map[string]any{"n1MessageContent": "", "n2MessageContent": ""},
		"payloads": map[string]any{
			"n1Payload": "n1",
			"n2Payload": "n2",
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected status %d", result.HTTPStatus)
	}
	got := <-observations
	if got.mediaType != "multipart/related" || got.rootType != "application/json" {
		t.Fatalf("unexpected multipart content type: %s type=%s", got.mediaType, got.rootType)
	}
	if len(got.parts) != 2 {
		t.Fatalf("expected two binary parts, got %d", len(got.parts))
	}
	if got.parts[0].contentType != "application/vnd.3gpp.5gnas" ||
		got.parts[1].contentType != "application/vnd.3gpp.ngap" {
		t.Fatalf("unexpected part content types: %+v", got.parts)
	}
	if got.parts[0].contentID == "" || got.parts[1].contentID == "" ||
		got.parts[0].contentID == got.parts[1].contentID {
		t.Fatalf("expected unique generated Content-IDs, got %+v", got.parts)
	}
	if got.root["n1MessageContent"] != got.parts[0].contentID ||
		got.root["n2MessageContent"] != got.parts[1].contentID {
		t.Fatalf("root JSON does not reference the part Content-IDs: root=%v parts=%+v", got.root, got.parts)
	}
	if !bytes.Equal(got.parts[0].data, n1Bytes) || !bytes.Equal(got.parts[1].data, n2Bytes) {
		t.Fatalf("unexpected part data: %x %x", got.parts[0].data, got.parts[1].data)
	}
}

func TestExecuteMultipartResponseAttachments(t *testing.T) {
	binaryData := []byte{0xaa, 0xbb, 0xcc}
	server := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		var buffer bytes.Buffer
		multipartWriter := multipart.NewWriter(&buffer)
		rootHeader := make(textproto.MIMEHeader)
		rootHeader.Set("Content-Type", "application/json")
		rootPart, err := multipartWriter.CreatePart(rootHeader)
		if err != nil {
			http.Error(writer, "cannot build response", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(rootPart, `{"n2MessageContent":"cid-1"}`)
		binaryHeader := make(textproto.MIMEHeader)
		binaryHeader.Set("Content-Type", "application/vnd.3gpp.ngap")
		binaryHeader.Set("Content-ID", "cid-1")
		binaryHeader.Set("Content-Length", strconv.Itoa(len(binaryData)))
		binaryPart, err := multipartWriter.CreatePart(binaryHeader)
		if err != nil {
			http.Error(writer, "cannot build response", http.StatusInternalServerError)
			return
		}
		_, _ = binaryPart.Write(binaryData)
		_ = multipartWriter.Close()
		writer.Header().Set("Content-Type", mime.FormatMediaType("multipart/related", map[string]string{
			"boundary": multipartWriter.Boundary(),
			"type":     "application/json",
		}))
		_, _ = writer.Write(buffer.Bytes())
	}))
	client := newTestClient(t, server, nil)
	executor := newTestExecutor(t, client, nil, payload.Limits{})
	operation := Operation{
		OperationID:        "Test_N1N2MessageNotification",
		Effect:             EffectRead,
		ServiceKey:         testServiceKey,
		Method:             http.MethodPost,
		PathTemplate:       "/n1-n2-notifications",
		ResponseMediaTypes: []string{"multipart/related"},
		ResponseSchema: mustSchema(t, `{
			"type":"object",
			"required":["n2MessageContent"],
			"properties":{"n2MessageContent":{"type":"string"}}
		}`),
		ResponseParts: []MultipartPart{
			{Name: "n2Payload", ContentType: "application/vnd.3gpp.ngap", JSONPointerField: "/n2MessageContent", Required: true},
		},
	}
	result, err := executor.Execute(context.Background(), operation, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.MediaType != "multipart/related" {
		t.Fatalf("unexpected media type %q", result.MediaType)
	}
	if len(result.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(result.Attachments))
	}
	attachment := result.Attachments[0]
	digest := sha256.Sum256(binaryData)
	if attachment.ContentID != "cid-1" ||
		attachment.ContentType != "application/vnd.3gpp.ngap" ||
		attachment.Size != int64(len(binaryData)) ||
		attachment.SHA256 != hex.EncodeToString(digest[:]) ||
		!bytes.Equal(attachment.Data, binaryData) {
		t.Fatalf("unexpected attachment: %+v", attachment)
	}
	root, ok := result.JSON.(map[string]any)
	if !ok || root["n2MessageContent"] != "cid-1" {
		t.Fatalf("unexpected multipart root JSON: %#v", result.JSON)
	}

	// A declared part that the remote does not send is a protocol violation.
	missingServer := newH2CTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		var buffer bytes.Buffer
		multipartWriter := multipart.NewWriter(&buffer)
		rootHeader := make(textproto.MIMEHeader)
		rootHeader.Set("Content-Type", "application/json")
		rootPart, _ := multipartWriter.CreatePart(rootHeader)
		_, _ = io.WriteString(rootPart, `{"n2MessageContent":"cid-1"}`)
		_ = multipartWriter.Close()
		writer.Header().Set("Content-Type", mime.FormatMediaType("multipart/related", map[string]string{
			"boundary": multipartWriter.Boundary(),
			"type":     "application/json",
		}))
		_, _ = writer.Write(buffer.Bytes())
	}))
	missingExecutor := newTestExecutor(t, newTestClient(t, missingServer, nil), nil, payload.Limits{})
	_, err = missingExecutor.Execute(context.Background(), operation, nil)
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("expected *ProtocolError for missing response part, got %T: %v", err, err)
	}
}

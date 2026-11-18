package payload

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

// slotRecordingSink implements the optional SlotAttachmentSink extension.
type slotRecordingSink struct {
	slots        []string
	contentIDs   []string
	contentTypes []string
	bodies       [][]byte
}

func (s *slotRecordingSink) Store(context.Context, string, string, io.Reader, int64) (Attachment, error) {
	return Attachment{}, ErrInvalidPayload
}

func (s *slotRecordingSink) StoreSlot(_ context.Context, slot, contentID, contentType string, source io.Reader, maximum int64) (Attachment, error) {
	body, err := io.ReadAll(io.LimitReader(source, maximum))
	if err != nil {
		return Attachment{}, err
	}
	s.slots = append(s.slots, slot)
	s.contentIDs = append(s.contentIDs, contentID)
	s.contentTypes = append(s.contentTypes, contentType)
	s.bodies = append(s.bodies, body)
	return Attachment{ContentID: contentID, RelativePath: "staged", ContentType: contentType, Size: int64(len(body))}, nil
}

func TestSlotAttachmentSinkReceivesDeclaredSlots(t *testing.T) {
	sink := &slotRecordingSink{}

	// Whole binary response body -> literal slot "body".
	if _, err := DecodeResponse(context.Background(), "application/octet-stream", strings.NewReader("binary"), ResponseSpec{Kind: KindBinary}, sink, Limits{}); err != nil {
		t.Fatalf("decode binary: %v", err)
	}
	if len(sink.slots) != 1 || sink.slots[0] != "body" {
		t.Fatalf("binary response slots = %#v", sink.slots)
	}

	// Declared multipart response part -> the catalog part name.
	var wire bytes.Buffer
	writer := multipart.NewWriter(&wire)
	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", "application/json")
	rootPart, _ := writer.CreatePart(rootHeader)
	_, _ = io.WriteString(rootPart, `{"messages":[{"n2MessageContent":{"contentId":"part-id"}}]}`)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Type", "application/vnd.3gpp.ngap")
	partHeader.Set("Content-ID", "part-id")
	part, _ := writer.CreatePart(partHeader)
	_, _ = io.WriteString(part, "ngap-bytes")
	_ = writer.Close()
	contentType := mime.FormatMediaType("multipart/related", map[string]string{"boundary": writer.Boundary(), "type": "application/json"})
	parsed, err := DecodeResponse(context.Background(), contentType, bytes.NewReader(wire.Bytes()), ResponseSpec{
		Kind: KindMultipartRelated,
		Multipart: MultipartSpec{Parts: []PartBinding{{
			Name:             "n2Response",
			ContentType:      "application/vnd.3gpp.ngap",
			ContentIDPointer: "/messages/0/n2MessageContent/contentId",
			Required:         true,
		}}},
	}, sink, Limits{})
	if err != nil {
		t.Fatalf("decode multipart: %v", err)
	}
	if len(sink.slots) != 2 || sink.slots[1] != "n2Response" || string(sink.bodies[1]) != "ngap-bytes" {
		t.Fatalf("multipart slot sink = slots %#v bodies %#v", sink.slots, sink.bodies)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].ContentID != "part-id" {
		t.Fatalf("parsed attachments = %#v", parsed.Attachments)
	}
}

func TestBuildMultipartOverwritesUntrustedContentIDPlaceholder(t *testing.T) {
	// The trusted invocation root may carry a placeholder Content-ID; the
	// Provider always generates the outbound Content-ID and overwrites it.
	root := []byte(`{"messages":[{"n1MessageContent":{"contentId":"runtime-generated"}}]}`)
	spec := MultipartSpec{Parts: []PartBinding{{
		Name: "n1Message", ContentType: "application/vnd.3gpp.5gnas",
		ContentIDPointer: "/messages/0/n1MessageContent/contentId", Required: true,
	}}}
	resolver := memoryResolver{"n1Message": []byte("bytes")}
	encoded, err := BuildMultipartRelated(context.Background(), resolver, root, spec, map[string]Source{
		"n1Message": {RelativePath: "n1Message", Size: 5},
	}, Limits{})
	if err != nil {
		t.Fatalf("build multipart: %v", err)
	}
	wire, err := io.ReadAll(encoded.Body)
	_ = encoded.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "runtime-generated") {
		t.Fatal("caller-supplied Content-ID placeholder leaked onto the wire")
	}
	mediaType, params, err := mime.ParseMediaType(encoded.ContentType)
	if err != nil || mediaType != "multipart/related" {
		t.Fatalf("content type = %q", encoded.ContentType)
	}
	reader := multipart.NewReader(bytes.NewReader(wire), params["boundary"])
	rootPart, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	rootJSON, _ := io.ReadAll(rootPart)
	if !bytes.Contains(rootJSON, []byte(`"contentId":"`)) || bytes.Contains(rootJSON, []byte("runtime-generated")) {
		t.Fatalf("root JSON was not rewritten: %s", rootJSON)
	}
}

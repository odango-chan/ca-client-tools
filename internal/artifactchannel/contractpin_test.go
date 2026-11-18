package artifactchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Pinned shared conformance fixtures for the Host Artifact Store contract
// (`ca-host.artifact-store/v1`). The bytes are copied verbatim from ca-agent
// commit 2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae (documentation-only update of
// the accepted 3a0d1404 contract; fixture bytes and MANIFEST are unchanged).
const (
	pinnedManifestSHA256 = "59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4"
	pinnedFixtureDir     = "../../contracts/host-artifact-store/v1"
)

var (
	fixtureSlotPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	fixtureMediaTypePattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]+/[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]+$`)
)

type fixtureEnvelope struct {
	FixtureVersion string          `json:"fixtureVersion"`
	Case           string          `json:"case"`
	Value          json.RawMessage `json:"value"`
	Request        json.RawMessage `json:"request"`
	Response       json.RawMessage `json:"response"`
	Expect         struct {
		Valid         bool   `json:"valid"`
		Reason        string `json:"reason"`
		ProviderFault string `json:"providerFault"`
		Published     *bool  `json:"published"`
	} `json:"expect"`
}

var fixtureFiles *[]string

func readFixture(t *testing.T, name string) fixtureEnvelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(pinnedFixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var envelope fixtureEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	if envelope.FixtureVersion != "ca-host.artifact-store/v1" {
		t.Fatalf("fixture %s version = %q", name, envelope.FixtureVersion)
	}
	return envelope
}

func fixtureNames(t *testing.T, prefix string) []string {
	t.Helper()
	if fixtureFiles == nil {
		entries, err := os.ReadDir(pinnedFixtureDir)
		if err != nil {
			t.Fatalf("read fixture directory: %v", err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
		fixtureFiles = &names
	}
	var matched []string
	for _, name := range *fixtureFiles {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") {
			matched = append(matched, name)
		}
	}
	if len(matched) == 0 {
		t.Fatalf("no fixtures with prefix %q", prefix)
	}
	return matched
}

// TestPinnedManifest proves the copied contract bytes match the published
// SHA-256 manifest and that the manifest file itself matches the pinned digest.
func TestPinnedManifest(t *testing.T) {
	manifestPath := filepath.Join(pinnedFixtureDir, "MANIFEST.sha256")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != pinnedManifestSHA256 {
		t.Fatalf("MANIFEST.sha256 digest = %s, want %s", got, pinnedManifestSHA256)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	previous := ""
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 {
			t.Fatalf("malformed manifest line %q", line)
		}
		name := parts[1]
		if name <= previous {
			t.Fatalf("manifest is not sorted: %q after %q", name, previous)
		}
		previous = name
		body, err := os.ReadFile(filepath.Join(pinnedFixtureDir, name))
		if err != nil {
			t.Fatalf("manifest names a missing fixture %s: %v", name, err)
		}
		digest := sha256.Sum256(body)
		if got := hex.EncodeToString(digest[:]); got != parts[0] {
			t.Fatalf("fixture %s digest = %s, want %s", name, got, parts[0])
		}
	}
}

func TestPinnedReferenceFixtures(t *testing.T) {
	for _, name := range fixtureNames(t, "reference-") {
		envelope := readFixture(t, name)
		var value any
		if err := json.Unmarshal(envelope.Value, &value); err != nil {
			t.Fatalf("%s value: %v", name, err)
		}
		token, valid := ReferenceValue(value)
		if valid != envelope.Expect.Valid {
			t.Fatalf("%s: validity = %v, want %v (reason %s)", name, valid, envelope.Expect.Valid, envelope.Expect.Reason)
		}
		if valid && !ValidReferenceToken(token) {
			t.Fatalf("%s: token rejected by grammar", name)
		}
	}
}

func TestPinnedChannelConfigFixtures(t *testing.T) {
	for _, name := range fixtureNames(t, "channel-config-") {
		envelope := readFixture(t, name)
		var value map[string]any
		if err := json.Unmarshal(envelope.Value, &value); err != nil {
			t.Fatalf("%s value: %v", name, err)
		}
		config := Config{
			BaseURL:           stringValue(value["baseUrl"]),
			ProcessToken:      stringValue(value["processToken"]),
			RequestTimeout:    3000 * 1e6,
			MaxArtifactBytes:  int64Value(value["maxArtifactBytes"]),
			MaxAggregateBytes: int64Value(value["maxAggregateBytes"]),
			MaxAttachments:    int(int64Value(value["maxAttachments"])),
		}
		valid := config.Validate() == nil
		if valid != envelope.Expect.Valid {
			t.Fatalf("%s: validity = %v, want %v (reason %s)", name, valid, envelope.Expect.Valid, envelope.Expect.Reason)
		}
	}
}

func TestPinnedChannelGetInputFixtures(t *testing.T) {
	for _, name := range fixtureNames(t, "channel-get-input-") {
		envelope := readFixture(t, name)
		var response struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(envelope.Response, &response); err != nil {
			t.Fatalf("%s response: %v", name, err)
		}
		host := newFakeHost(t, "fixture-token")
		channel, err := New(host.config())
		if err != nil {
			t.Fatal(err)
		}
		if envelope.Expect.Valid {
			if response.Status != http.StatusOK {
				t.Fatalf("%s: valid fixture status = %d", name, response.Status)
			}
			body := []byte("0123456789abcdef0123456789abcdef")
			reference, _ := host.addInput("application/vnd.3gpp.5gnas", body)
			input, err := channel.Open(context.Background(), reference, "inv-fixture", "n1Message")
			if err != nil {
				t.Fatalf("%s: Open: %v", name, err)
			}
			if input.Size != int64(len(body)) {
				t.Fatalf("%s: size = %d", name, input.Size)
			}
			_ = input.Body.Close()
		} else {
			host.getHook = func(writer http.ResponseWriter, _ *http.Request, _ string) bool {
				writeChannelError(writer, response.Status, "ARTIFACT_FIXTURE_ERROR", "artifact error")
				return true
			}
			_, err := channel.Open(context.Background(), "art_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "inv-fixture", "n1Message")
			code, ok := FaultCode(err)
			if !ok || code != envelope.Expect.ProviderFault {
				t.Fatalf("%s: fault = %v, want %s", name, err, envelope.Expect.ProviderFault)
			}
		}
		channel.CloseIdleConnections()
	}
}

func TestPinnedChannelPostOutputFixtures(t *testing.T) {
	for _, name := range fixtureNames(t, "channel-post-output-") {
		envelope := readFixture(t, name)
		var response struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(envelope.Response, &response); err != nil {
			t.Fatalf("%s response: %v", name, err)
		}
		host := newFakeHost(t, "fixture-token")
		channel, err := New(host.config())
		if err != nil {
			t.Fatal(err)
		}
		sink := channel.NewSink("inv-fixture", NewQuota(4<<20, 8))
		if envelope.Expect.Valid {
			if response.Status != http.StatusCreated {
				t.Fatalf("%s: valid fixture status = %d", name, response.Status)
			}
			attachment, err := sink.StoreSlot(context.Background(), "n2Message", "", "application/vnd.3gpp.ngap", strings.NewReader("output-bytes"), 1<<20)
			if err != nil {
				t.Fatalf("%s: StoreSlot: %v", name, err)
			}
			if !ValidReferenceToken(attachment.RelativePath) || len(sink.Refs()) != 1 {
				t.Fatalf("%s: staged reference invalid", name)
			}
		} else {
			host.postHook = func(writer http.ResponseWriter, _ *http.Request, _ fakeHostUpload) bool {
				writeChannelError(writer, response.Status, "ARTIFACT_FIXTURE_ERROR", "artifact error")
				return true
			}
			_, err := sink.StoreSlot(context.Background(), "n2Message", "", "application/vnd.3gpp.ngap", strings.NewReader("output-bytes"), 1<<20)
			code, ok := FaultCode(err)
			if !ok || code != envelope.Expect.ProviderFault {
				t.Fatalf("%s: fault = %v, want %s", name, err, envelope.Expect.ProviderFault)
			}
			if len(sink.Refs()) != 0 {
				t.Fatalf("%s: failed staging published a reference", name)
			}
		}
		channel.CloseIdleConnections()
	}
}

func TestPinnedSlotFixtures(t *testing.T) {
	for _, name := range fixtureNames(t, "slots-") {
		envelope := readFixture(t, name)
		var value struct {
			Inputs []struct {
				Slot      string `json:"slot"`
				Pointer   string `json:"pointer"`
				MediaType string `json:"mediaType"`
				Required  bool   `json:"required"`
			} `json:"inputs"`
			Outputs []struct {
				Slot      string `json:"slot"`
				Pointer   string `json:"pointer"`
				MediaType string `json:"mediaType"`
				Required  bool   `json:"required"`
			} `json:"outputs"`
		}
		if err := json.Unmarshal(envelope.Value, &value); err != nil {
			t.Fatalf("%s value: %v", name, err)
		}
		if !envelope.Expect.Valid {
			continue
		}
		slots := append([]struct {
			Slot      string `json:"slot"`
			Pointer   string `json:"pointer"`
			MediaType string `json:"mediaType"`
			Required  bool   `json:"required"`
		}{}, value.Inputs...)
		slots = append(slots, value.Outputs...)
		if len(slots) == 0 {
			t.Fatalf("%s declares no slots", name)
		}
		for _, slot := range slots {
			if !fixtureSlotPattern.MatchString(slot.Slot) {
				t.Fatalf("%s: invalid slot %q", name, slot.Slot)
			}
			if !fixtureMediaTypePattern.MatchString(slot.MediaType) {
				t.Fatalf("%s: invalid media type %q", name, slot.MediaType)
			}
		}
	}
}

func TestPinnedArgumentsAndResultFixtures(t *testing.T) {
	arguments := readFixture(t, "arguments-refs-multipart.json")
	var argumentValue struct {
		Body     map[string]any            `json:"body"`
		Payloads map[string]map[string]any `json:"payloads"`
	}
	if err := json.Unmarshal(arguments.Value, &argumentValue); err != nil {
		t.Fatalf("arguments fixture: %v", err)
	}
	if len(argumentValue.Payloads) == 0 {
		t.Fatal("arguments fixture has no payload references")
	}
	for name, reference := range argumentValue.Payloads {
		if !fixtureSlotPattern.MatchString(name) {
			t.Fatalf("payload part %q is invalid", name)
		}
		if _, ok := ReferenceValue(reference); !ok {
			t.Fatalf("payload %q is not a valid reference", name)
		}
	}

	foreign := readFixture(t, "result-refs-foreign.json")
	var foreignValue struct {
		HTTPStatus int            `json:"httpStatus"`
		Response   map[string]any `json:"response"`
	}
	if err := json.Unmarshal(foreign.Value, &foreignValue); err != nil {
		t.Fatalf("result fixture: %v", err)
	}
	if foreign.Expect.Valid {
		t.Fatal("foreign result fixture must be invalid")
	}
	// The fixture carries a published reference and a reference outside every
	// declared output slot; a Provider must never emit the latter.
	declared := map[string]struct{}{"/response/n2MessageContent": {}}
	foundDeclared, foundForeign := false, false
	var walk func(pointer string, node any)
	walk = func(pointer string, node any) {
		object, ok := node.(map[string]any)
		if !ok {
			return
		}
		if _, isReference := object["artifactRef"]; isReference {
			if _, ok := declared[pointer]; ok {
				foundDeclared = true
			} else {
				foundForeign = true
			}
			return
		}
		for key, child := range object {
			walk(pointer+"/"+key, child)
		}
	}
	for key, child := range map[string]any{"response": any(foreignValue.Response)} {
		walk("/"+key, child)
	}
	if !foundDeclared || !foundForeign {
		t.Fatalf("foreign fixture classification: declared=%v foreign=%v", foundDeclared, foundForeign)
	}

	multipart := readFixture(t, "result-refs-multipart.json")
	var multipartValue struct {
		HTTPStatus int            `json:"httpStatus"`
		Response   map[string]any `json:"response"`
	}
	if err := json.Unmarshal(multipart.Value, &multipartValue); err != nil {
		t.Fatalf("result fixture: %v", err)
	}
	if !multipart.Expect.Valid || multipartValue.HTTPStatus != 200 {
		t.Fatal("accepted multipart result fixture changed")
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func int64Value(value any) int64 {
	number, ok := value.(float64)
	if !ok {
		return 0
	}
	return int64(number)
}

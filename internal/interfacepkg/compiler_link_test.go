package interfacepkg

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Compiler link gate exercises the real ca-skill-compiler v2.1.0 binary.
// It is skipped when CA_SKILLC is unset so ordinary `go test ./...` stays
// hermetic; the release workflow sets it and treats the gate as mandatory.
const (
	linkSkillName = "inspect-smf-session"
	linkSkillDir  = "examples/interface-package-skill"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not locate the repository root")
		}
		directory = parent
	}
}

func compilerPath(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("CA_SKILLC")
	if binary == "" {
		t.Skip("CA_SKILLC is not set: skipping the real Compiler link gate")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("CA_SKILLC does not name an executable: %v", err)
	}
	return binary
}

func generatedPackage(t *testing.T) *Result {
	t.Helper()
	result, err := Generate(Options{
		CatalogPath:     filepath.Join(repoRoot(t), CatalogPath),
		PackageVersion:  "1.1.0",
		ProviderVersion: "1.1.0",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return result
}

func skillFiles(t *testing.T) []requestFile {
	t.Helper()
	root := filepath.Join(repoRoot(t), linkSkillDir, "skills", linkSkillName)
	files := make([]requestFile, 0, 4)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(filepath.Dir(filepath.Dir(root)), path)
		if err != nil {
			return err
		}
		files = append(files, requestFile{Path: filepath.ToSlash(relative), Bytes: raw})
		return nil
	})
	if err != nil {
		t.Fatalf("read example skill: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("example skill is empty")
	}
	return files
}

type requestFile struct {
	Path  string `json:"path"`
	Bytes []byte `json:"bytes"`
}

type requestKey struct {
	PackageID      string `json:"packageId"`
	Version        string `json:"version"`
	SnapshotDigest string `json:"snapshotDigest"`
}

type requestSnapshot struct {
	Key   requestKey    `json:"key"`
	Files []requestFile `json:"files"`
}

type requestLockRoot struct {
	Alias string     `json:"alias"`
	Key   requestKey `json:"key"`
}

type requestLockPackage struct {
	Key          requestKey        `json:"key"`
	Dependencies []requestLockEdge `json:"dependencies"`
}

type requestLockEdge struct {
	Alias string     `json:"alias"`
	Key   requestKey `json:"key"`
}

type requestLock struct {
	APIVersion string               `json:"apiVersion"`
	Roots      []requestLockRoot    `json:"roots"`
	Packages   []requestLockPackage `json:"packages"`
}

type buildRequest struct {
	APIVersion     string            `json:"apiVersion"`
	LanguageTarget string            `json:"languageTarget"`
	Profile        string            `json:"profile"`
	Files          []requestFile     `json:"files"`
	Targets        []string          `json:"targets"`
	Lock           requestLock       `json:"lock"`
	Snapshots      []requestSnapshot `json:"snapshots"`
}

func buildRequestFor(result *Result, files []GeneratedFile) buildRequest {
	key := requestKey{
		PackageID:      result.PackageID,
		Version:        result.PackageVersion,
		SnapshotDigest: SnapshotDigest(files),
	}
	snapshotFiles := make([]requestFile, 0, len(files))
	for _, file := range files {
		snapshotFiles = append(snapshotFiles, requestFile{Path: file.Path, Bytes: file.Bytes})
	}
	return buildRequest{
		APIVersion:     "connectionagent.io/compiler-development-build-request/v1alpha1",
		LanguageTarget: LanguageTarget,
		Profile:        "DEVELOPMENT",
		Files:          nil,
		Targets:        []string{linkSkillName},
		Lock: requestLock{
			APIVersion: "connectionagent.io/interface-dependency-lock/v1alpha1",
			Roots:      []requestLockRoot{{Alias: "ca", Key: key}},
			Packages:   []requestLockPackage{{Key: key, Dependencies: []requestLockEdge{}}},
		},
		Snapshots: []requestSnapshot{{Key: key, Files: snapshotFiles}},
	}
}

type buildResult struct {
	Success             bool   `json:"success"`
	SourceStatus        string `json:"sourceStatus"`
	InterfaceLinkStatus string `json:"interfaceLinkStatus"`
	ArtifactStatus      string `json:"artifactStatus"`
	ExecutionStatus     string `json:"executionStatus"`
}

// runCompiler writes and executes one Development build, returning the parsed
// result and the combined diagnostics text.
func runCompiler(t *testing.T, binary string, request buildRequest) (buildResult, string) {
	t.Helper()
	dir := t.TempDir()
	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatalf("encode build request: %v", err)
	}
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, encoded, 0o644); err != nil {
		t.Fatalf("write build request: %v", err)
	}
	out := filepath.Join(dir, "out")
	command := exec.Command(binary, "build-development", "--request", requestPath, "--out", out, "--json")
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	diagnostics := stdout.String() + stderr.String()
	var parsed buildResult
	if trimmed := strings.TrimSpace(stdout.String()); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			t.Fatalf("decode compiler result: %v\n%s", err, diagnostics)
		}
	}
	if runErr != nil {
		return parsed, diagnostics
	}
	if parsed.Success {
		if _, err := os.Stat(filepath.Join(out, "skills", linkSkillName, "compiled", "procedure.json")); err != nil {
			t.Fatalf("compiler reported success without the compiled procedure: %v", err)
		}
	}
	return parsed, diagnostics
}

// TestCompilerLinksGeneratedInterfacePackage is the G2 acceptance gate: the
// real Compiler v2.1.0 must link a CA-SL v1alpha5 Skill against the exact
// generated Interface Package, its Callable and its Capability.
func TestCompilerLinksGeneratedInterfacePackage(t *testing.T) {
	binary := compilerPath(t)
	result := generatedPackage(t)
	request := buildRequestFor(result, result.Files)
	request.Files = skillFiles(t)

	parsed, diagnostics := runCompiler(t, binary, request)
	if !parsed.Success {
		t.Fatalf("Compiler did not link the generated Interface Package:\n%s", diagnostics)
	}
	if parsed.SourceStatus != "VALIDATED" || parsed.InterfaceLinkStatus != "LINKED" ||
		parsed.ArtifactStatus != "BUILT" || parsed.ExecutionStatus != "NOT_ADMITTED" {
		t.Fatalf("unexpected compiler phase statuses: %+v", parsed)
	}
}

// TestCompilerRejectsBrokenSchemaReference proves the same gate fails closed
// when the Interface Package references a Schema that is not exported. All
// digests are recomputed so the failure is the broken $ref, not a digest
// mismatch.
func TestCompilerRejectsBrokenSchemaReference(t *testing.T) {
	binary := compilerPath(t)
	result := generatedPackage(t)
	files := make([]GeneratedFile, len(result.Files))
	copy(files, result.Files)

	broken := "urn:wtlab:ca-client-tools:openamsm:missing-result:v1"
	patched := false
	for index, file := range files {
		if file.Path != ToolCatalogName {
			continue
		}
		updated := strings.ReplaceAll(string(file.Bytes),
			"urn:wtlab:ca-client-tools:openamsm:mas-context-result:v1", broken)
		if updated == string(file.Bytes) {
			t.Fatal("tool catalog does not reference the expected result Schema")
		}
		files[index].Bytes = []byte(updated)
		patched = true
	}
	if !patched {
		t.Fatal("tool catalog missing from the generated package")
	}
	resealPackage(t, files)

	request := buildRequestFor(result, files)
	request.Files = skillFiles(t)
	// buildRequestFor recomputes the digest from the patched files, so the
	// snapshot itself is consistent and only the $ref is unresolvable.
	parsed, diagnostics := runCompiler(t, binary, request)
	if parsed.Success {
		t.Fatalf("Compiler accepted an Interface Package with an unresolvable $ref:\n%s", diagnostics)
	}
	if !strings.Contains(diagnostics, "error") {
		t.Fatalf("expected compiler diagnostics, got:\n%s", diagnostics)
	}
}

// resealPackage rewrites package.json so its file digests cover patched bytes.
func resealPackage(t *testing.T, files []GeneratedFile) {
	t.Helper()
	manifestIndex := -1
	var manifest packageManifest
	for index, file := range files {
		if file.Path != PackageManifestName {
			continue
		}
		if err := json.Unmarshal(file.Bytes, &manifest); err != nil {
			t.Fatalf("decode package.json: %v", err)
		}
		manifestIndex = index
	}
	if manifestIndex < 0 {
		t.Fatal("package.json missing from the generated package")
	}
	manifest.Files = make([]packageFile, 0, len(files)-1)
	for _, file := range files {
		if file.Path == PackageManifestName {
			continue
		}
		manifest.Files = append(manifest.Files, packageFile{
			Path:   file.Path,
			SHA256: "sha256:" + sha256Hex(file.Bytes),
		})
	}
	encoded, err := encodeJSON(manifest)
	if err != nil {
		t.Fatalf("encode package.json: %v", err)
	}
	files[manifestIndex].Bytes = encoded
}

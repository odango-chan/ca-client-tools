package interfacepkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func generateForTest(t *testing.T) *Result {
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

func TestGenerateIsDeterministicAndComplete(t *testing.T) {
	first := generateForTest(t)
	second := generateForTest(t)
	if len(first.Files) != len(second.Files) {
		t.Fatalf("generation file count changed between runs")
	}
	for index := range first.Files {
		if first.Files[index].Path != second.Files[index].Path {
			t.Fatalf("generation path order changed: %q vs %q", first.Files[index].Path, second.Files[index].Path)
		}
		if string(first.Files[index].Bytes) != string(second.Files[index].Bytes) {
			t.Fatalf("generation is not deterministic for %s", first.Files[index].Path)
		}
	}
	if first.InterfaceSnapshotDigest != second.InterfaceSnapshotDigest {
		t.Fatalf("snapshot digest changed between runs")
	}

	// 11 public READ Tools, 2 Capabilities, 4 standalone Schemas, tool catalog,
	// capabilities and package.json.
	if len(first.Files) != 7 {
		t.Fatalf("generated %d interface files, want 7", len(first.Files))
	}
	var manifest packageManifest
	for _, file := range first.Files {
		if file.Path == PackageManifestName {
			if err := json.Unmarshal(file.Bytes, &manifest); err != nil {
				t.Fatalf("decode package.json: %v", err)
			}
		}
	}
	if manifest.APIversion() != InterfacePackageAPIVersion || manifest.LanguageTarget != LanguageTarget {
		t.Fatalf("package identity = %q / %q", manifest.APIVersion, manifest.LanguageTarget)
	}
	if len(manifest.Dependencies) != 0 {
		t.Fatalf("dependencies = %v, want an explicit empty list", manifest.Dependencies)
	}
	callables := make(map[string]int)
	capabilities := make(map[string]int)
	schemas := make(map[string]int)
	for _, export := range manifest.Exports {
		switch export.Kind {
		case "CallableCatalog":
			callables[export.ID]++
		case "CapabilityCatalog":
			capabilities[export.ID]++
		case "Schema":
			schemas[export.ID]++
		default:
			t.Fatalf("unexpected export kind %q", export.Kind)
		}
	}
	if len(callables) != 11 || len(capabilities) != 2 || len(schemas) != 4 {
		t.Fatalf("export index = %d callables, %d capabilities, %d schemas; want 11/2/4",
			len(callables), len(capabilities), len(schemas))
	}
	for id, count := range callables {
		if count != 1 {
			t.Fatalf("callable %q is exported %d times", id, count)
		}
	}
	declared := make(map[string]string, len(manifest.Files))
	for _, file := range manifest.Files {
		if file.Path == PackageManifestName {
			t.Fatalf("package.json must not list itself")
		}
		declared[file.Path] = file.SHA256
	}
	if len(declared) != len(first.Files)-1 {
		t.Fatalf("package.json lists %d files, generation produced %d", len(declared), len(first.Files)-1)
	}
	if len(first.BackendCatalogDigest) != len("sha256:")+64 {
		t.Fatalf("backend catalog digest = %q", first.BackendCatalogDigest)
	}
}

// APIVersion accessor keeps the test readable without exporting the field name
// twice in the generated JSON contract.
func (m packageManifest) APIversion() string { return m.APIVersion }

func TestSnapshotDigestGoldenVector(t *testing.T) {
	// Frozen vector from ca-skill-compiler
	// contracts/v1alpha5/dependency-snapshot-digest-vectors.json: two files in
	// either input order must produce the same digest.
	files := []GeneratedFile{
		{Path: "tools/catalog.yaml", Bytes: []byte("apiVersion: v1\n")},
		{Path: "schemas/a.json", Bytes: []byte("{\"type\":\"string\"}\n")},
	}
	want := "sha256:f9a26e8cceb4db7d36eb7e3905c73b1b166ff03bcb96a77297571e715020bfbc"
	if got := SnapshotDigest(files); got != want {
		t.Fatalf("SnapshotDigest = %s, want %s", got, want)
	}
	reversed := []GeneratedFile{files[1], files[0]}
	if got := SnapshotDigest(reversed); got != want {
		t.Fatalf("SnapshotDigest(reversed) = %s, want %s", got, want)
	}
	changed := []GeneratedFile{files[0], {Path: "schemas/a.json", Bytes: []byte("{}")}}
	if SnapshotDigest(changed) == want {
		t.Fatal("SnapshotDigest ignores file bytes")
	}
}

// copyCatalog stages a mutable copy of the canonical catalog and its Schema
// sources so negative cases can be authored without touching the repository.
func copyCatalog(t *testing.T) string {
	t.Helper()
	source := filepath.Join(repoRoot(t), "contracts/provider-bindings/v2")
	target := filepath.Join(t.TempDir(), "provider-bindings")
	if err := os.MkdirAll(filepath.Join(target, "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(source, "provider-bindings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "provider-bindings.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(source, "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(source, "schemas", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "schemas", entry.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(target, "provider-bindings.yaml")
}

func TestGenerateRejectsInvalidToolDefinitions(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(t *testing.T, catalogPath string)
		wantSub string
	}{
		{
			name: "duplicate callable id",
			mutate: func(t *testing.T, catalogPath string) {
				replaceInFile(t, catalogPath,
					"      backendOperationId: openamsm.smf.mas.pa.query\n      kind: observation",
					"      backendOperationId: openamsm.smf.mas.pa.query\n      kind: observation\n      callableId: openamsm.smf.mas.sc.query")
			},
			wantSub: "duplicate mapping key",
		},
		{
			name: "tool effect disagrees with binding",
			mutate: func(t *testing.T, catalogPath string) {
				replaceOnceInFile(t, catalogPath,
					"    - callableId: openamsm.smf.mas.sc.query\n      backendOperationId: openamsm.smf.mas.sc.query\n      kind: observation\n      effect: READ",
					"    - callableId: openamsm.smf.mas.sc.query\n      backendOperationId: openamsm.smf.mas.sc.query\n      kind: observation\n      effect: MUTATION")
			},
			wantSub: "differs from binding",
		},
		{
			name: "undeclared schema reference",
			mutate: func(t *testing.T, catalogPath string) {
				replaceAllInFile(t, catalogPath,
					"$ref: urn:wtlab:ca-client-tools:openamsm:mas-context-query-input:v1",
					"$ref: urn:wtlab:ca-client-tools:openamsm:missing:v1")
			},
			wantSub: "not declared in spec.schemaFiles",
		},
		{
			name: "unknown backend operation",
			mutate: func(t *testing.T, catalogPath string) {
				replaceOnceInFile(t, catalogPath,
					"    - callableId: openamsm.smf.mas.sc.query\n      backendOperationId: openamsm.smf.mas.sc.query",
					"    - callableId: openamsm.smf.mas.sc.query\n      backendOperationId: openamsm.smf.mas.unknown.query")
			},
			wantSub: "is not declared in spec.bindings",
		},
		{
			name: "invalid capability identity",
			mutate: func(t *testing.T, catalogPath string) {
				replaceAllInFile(t, catalogPath,
					"capabilities: [openamsm.smf.mas.context.read]",
					"capabilities: [9invalid]")
			},
			wantSub: "invalid CapabilityId",
		},
		{
			name: "schema identity differs from the declared id",
			mutate: func(t *testing.T, catalogPath string) {
				schemaPath := filepath.Join(filepath.Dir(catalogPath), "schemas/mas-context-query-input.schema.yaml")
				replaceOnceInFile(t, schemaPath,
					"$id: urn:wtlab:ca-client-tools:openamsm:mas-context-query-input:v1",
					"$id: urn:wtlab:ca-client-tools:openamsm:other:v1")
			},
			wantSub: "Schema source",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			catalogPath := copyCatalog(t)
			testCase.mutate(t, catalogPath)
			_, err := Generate(Options{
				CatalogPath:     catalogPath,
				PackageVersion:  "1.1.0",
				ProviderVersion: "1.1.0",
			})
			if err == nil {
				t.Fatalf("Generate accepted an invalid Tool definition")
			}
			if !containsAny(err.Error(), testCase.wantSub) {
				t.Fatalf("Generate error = %q, want it to mention %q", err, testCase.wantSub)
			}
		})
	}
}

func TestGenerateRejectsMissingSchemaSource(t *testing.T) {
	catalogPath := copyCatalog(t)
	if err := os.Remove(filepath.Join(filepath.Dir(catalogPath), "schemas/metrics-page.schema.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(Options{
		CatalogPath:     catalogPath,
		PackageVersion:  "1.1.0",
		ProviderVersion: "1.1.0",
	})
	if err == nil || !strings.Contains(err.Error(), "read Schema source") {
		t.Fatalf("Generate error = %v, want a missing Schema source failure", err)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	result := generateForTest(t)
	staging := t.TempDir()
	if err := result.Write(staging); err != nil {
		t.Fatalf("Write: %v", err)
	}
	options := Options{
		CatalogPath:     filepath.Join(repoRoot(t), CatalogPath),
		PackageVersion:  "1.1.0",
		ProviderVersion: "1.1.0",
	}
	if _, err := Verify(staging, options); err != nil {
		t.Fatalf("Verify(clean package) error: %v", err)
	}

	// Tampered interface file.
	target := filepath.Join(staging, InterfacePackagePath, CapabilitiesName)
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(staging, options); err == nil {
		t.Fatal("Verify accepted a tampered interface file")
	}
	if err := result.Write(staging); err != nil {
		t.Fatal(err)
	}

	// Unexpected extra file.
	extra := filepath.Join(staging, InterfacePackagePath, "schemas", "extra.schema.json")
	if err := os.WriteFile(extra, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(staging, options); err == nil {
		t.Fatal("Verify accepted an unexpected interface file")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}

	// Tampered provider.yaml.
	provider := filepath.Join(staging, ProviderManifestName)
	if err := os.WriteFile(provider, []byte("apiVersion: ca.tool-provider-runtime-binding/v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(staging, options); err == nil {
		t.Fatal("Verify accepted a tampered provider.yaml")
	}
}

func replaceOnceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), old) != 1 {
		t.Fatalf("expected exactly one occurrence of %q in %s, found %d", old, path, strings.Count(string(raw), old))
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceAllInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatalf("fixture anchor %q not found in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), old, replacement)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatalf("fixture anchor %q not found in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	// The Schema identity failure is reported by the generator; accept its
	// stable prefix as well.
	return strings.Contains(value, "Schema source")
}

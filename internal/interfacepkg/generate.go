// Package interfacepkg deterministically generates the Compiler Interface
// Package and the Provider runtime manifest from the single canonical Tool
// definition source: contracts/provider-bindings/v2/provider-bindings.yaml and
// its standalone Schema source files.
//
// Nothing in this package authors public Tool data. Every callable identity,
// Schema reference, Effect, Owner outcome, Capability membership and private
// backend binding is read from the catalog; the generator only formats the two
// frozen views (Compiler Interface Package and Host runtime binding).
package interfacepkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/runtimebindings"
)

// Frozen interface identities. The generated package must use exactly these;
// changing them requires a new Compiler contract, not a generator tweak.
const (
	InterfacePackageAPIVersion = "connectionagent.io/interface-package/v1alpha1"
	LanguageTarget             = "connectionagent.io/v1alpha5"
	ToolCatalogAPIVersion      = "connectionagent.io/toolcatalog/v1alpha4"
	ToolCatalogKind            = "ToolCatalog"
	RuntimeBindingAPIVersion   = "ca.tool-provider-runtime-binding/v1"
	ProviderProtocol           = "ca-client-tools.provider/v2"
	InterfacePackagePath       = "interface"
	ProviderManifestName       = "provider.yaml"
	PackageManifestName        = "package.json"
	ToolCatalogName            = "tool-catalog.yaml"
	CapabilitiesName           = "capabilities.json"
	snapshotDigestDomain       = "CA-SKILL-DEPENDENCY-SNAPSHOT\x00v1\x00"
)

// CatalogPath is the repository-relative canonical Tool definition.
const CatalogPath = "contracts/provider-bindings/v2/provider-bindings.yaml"

// Options are the exact inputs of one deterministic generation.
type Options struct {
	// CatalogPath is the canonical catalog file path.
	CatalogPath string
	// PackageVersion is the Interface Package version (release tag without v).
	PackageVersion string
	// ProviderVersion is the Provider binary identity reported by initialize.
	ProviderVersion string
	// OwnerDir is the pinned OpenAMSM Owner publication directory. When empty
	// the repository default is used.
	OwnerDir string
}

// Result is the generated package plus the identities the release must publish.
type Result struct {
	PackageID               string
	PackageVersion          string
	ProviderVersion         string
	BackendCatalogDigest    string
	InterfaceSnapshotDigest string
	Files                   []GeneratedFile
	ProviderManifest        []byte
	RuntimeBindingsManifest []byte
	RuntimeBindingsDigest   string
	RuntimeBindingsFiles    []GeneratedFile
}

// GeneratedFile is one deterministic interface file (path is package-relative).
type GeneratedFile struct {
	Path  string
	Bytes []byte
}

// Generate produces the Interface Package and provider.yaml in memory.
func Generate(options Options) (*Result, error) {
	if options.CatalogPath == "" {
		return nil, fmt.Errorf("interfacepkg: catalog path is required")
	}
	if !validVersion(options.PackageVersion) {
		return nil, fmt.Errorf("interfacepkg: package version %q must be exact SemVer", options.PackageVersion)
	}
	if !validVersion(options.ProviderVersion) {
		return nil, fmt.Errorf("interfacepkg: provider version %q must be exact SemVer", options.ProviderVersion)
	}
	raw, err := os.ReadFile(options.CatalogPath)
	if err != nil {
		return nil, fmt.Errorf("interfacepkg: read catalog: %w", err)
	}
	loaded, err := catalog.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("interfacepkg: %w", err)
	}
	if loaded.Interface == nil {
		return nil, fmt.Errorf("interfacepkg: catalog does not declare a public interface identity")
	}
	if len(loaded.Tools) == 0 {
		return nil, fmt.Errorf("interfacepkg: catalog does not declare any public Tool")
	}
	contractDir := filepath.Dir(options.CatalogPath)

	schemaFiles, schemaByID, err := loadSchemaSources(contractDir, loaded)
	if err != nil {
		return nil, err
	}
	toolCatalog, err := renderToolCatalog(loaded)
	if err != nil {
		return nil, err
	}
	capabilities, err := renderCapabilities(loaded)
	if err != nil {
		return nil, err
	}

	files := make([]GeneratedFile, 0, len(schemaFiles)+2)
	files = append(files, GeneratedFile{Path: ToolCatalogName, Bytes: toolCatalog})
	files = append(files, GeneratedFile{Path: CapabilitiesName, Bytes: capabilities})
	for _, schema := range schemaFiles {
		files = append(files, GeneratedFile{Path: schema.Path, Bytes: schema.Bytes})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	exports := make([]packageExport, 0, len(loaded.Tools)+len(loaded.SchemaFiles)+1)
	for _, tool := range loaded.Tools {
		exports = append(exports, packageExport{Kind: "CallableCatalog", ID: tool.CallableID, Path: ToolCatalogName})
	}
	capabilityIDs := capabilityIDs(loaded)
	for _, capability := range capabilityIDs {
		exports = append(exports, packageExport{Kind: "CapabilityCatalog", ID: capability, Path: CapabilitiesName})
	}
	schemaIDs := make([]string, 0, len(schemaByID))
	for id := range schemaByID {
		schemaIDs = append(schemaIDs, id)
	}
	sort.Strings(schemaIDs)
	for _, id := range schemaIDs {
		exports = append(exports, packageExport{Kind: "Schema", ID: id, Path: schemaByID[id].Path})
	}

	digests := make([]packageFile, 0, len(files))
	for _, file := range files {
		digests = append(digests, packageFile{Path: file.Path, SHA256: "sha256:" + sha256Hex(file.Bytes)})
	}
	manifest := packageManifest{
		APIVersion:     InterfacePackageAPIVersion,
		PackageID:      loaded.Interface.PackageID,
		Version:        options.PackageVersion,
		LanguageTarget: LanguageTarget,
		Publisher:      loaded.Interface.Publisher,
		Dependencies:   []packageDependency{},
		Exports:        exports,
		Files:          digests,
	}
	manifestBytes, err := encodeJSON(manifest)
	if err != nil {
		return nil, err
	}
	all := append([]GeneratedFile{{Path: PackageManifestName, Bytes: manifestBytes}}, files...)
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })
	snapshotDigest := SnapshotDigest(all)

	// The Host compares the manifest digest with the running Provider. The
	// Provider reports the base artifact digest extended by the fixed typed MAS
	// closure, exactly as newRuntime computes it at startup.
	if err := catalog.InstallTypedMASBindings(loaded); err != nil {
		return nil, fmt.Errorf("interfacepkg: typed MAS closure: %w", err)
	}
	bindings := make([]runtimeBinding, 0, len(loaded.Tools))
	for _, tool := range loaded.Tools {
		bindings = append(bindings, runtimeBinding{
			PublicCallableID:   tool.CallableID,
			BackendOperationID: tool.BackendOperationID,
		})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].PublicCallableID < bindings[j].PublicCallableID })
	ownerDir := options.OwnerDir
	if ownerDir == "" {
		// The canonical catalog lives at <root>/contracts/provider-bindings/v2,
		// so the repository root is two levels above its directory. This keeps
		// the default independent of the process working directory.
		ownerDir = filepath.Join(filepath.Dir(options.CatalogPath), "..", "..", "..", filepath.FromSlash(runtimebindings.OwnerDirectory))
	}
	runtimePackage, err := runtimebindings.Generate(runtimebindings.Options{
		OwnerDir:        ownerDir,
		ProviderVersion: options.ProviderVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("interfacepkg: %w", err)
	}
	provider := providerManifest{
		APIVersion:              RuntimeBindingAPIVersion,
		ProviderID:              "ca-client-tools.sbi",
		ProviderVersion:         options.ProviderVersion,
		Protocol:                ProviderProtocol,
		BackendCatalogDigest:    loaded.Digest,
		Transport:               transportBinding{Type: "stdio", Argv: []string{"./ca-client-tools", "provider", "serve"}},
		InterfacePackagePath:    InterfacePackagePath,
		InterfaceSnapshotDigest: snapshotDigest,
		RuntimeBindingsPath:     runtimebindings.SidecarName,
		RuntimeBindingsDigest:   sha256Digest(runtimePackage.Sidecar),
		Bindings:                bindings,
	}
	providerBytes, err := encodeYAML(provider)
	if err != nil {
		return nil, err
	}
	runtimeFiles := make([]GeneratedFile, 0, len(runtimePackage.Files))
	for _, file := range runtimePackage.Files {
		runtimeFiles = append(runtimeFiles, GeneratedFile{Path: file.Path, Bytes: file.Bytes})
	}
	return &Result{
		PackageID:               loaded.Interface.PackageID,
		PackageVersion:          options.PackageVersion,
		ProviderVersion:         options.ProviderVersion,
		BackendCatalogDigest:    loaded.Digest,
		InterfaceSnapshotDigest: snapshotDigest,
		Files:                   all,
		ProviderManifest:        providerBytes,
		RuntimeBindingsManifest: runtimePackage.Sidecar,
		RuntimeBindingsDigest:   sha256Digest(runtimePackage.Sidecar),
		RuntimeBindingsFiles:    runtimeFiles,
	}, nil
}

// Write stages one generated package: <out>/interface/* and <out>/provider.yaml.
func (result *Result) Write(out string) error {
	interfaceDir := filepath.Join(out, InterfacePackagePath)
	if err := os.MkdirAll(interfaceDir, 0o755); err != nil {
		return err
	}
	for _, file := range result.Files {
		target := filepath.Join(interfaceDir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Bytes, 0o644); err != nil {
			return err
		}
	}
	for _, file := range result.RuntimeBindingsFiles {
		target := filepath.Join(out, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Bytes, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(out, ProviderManifestName), result.ProviderManifest, 0o644)
}

// Verify checks an already staged package against a fresh generation from the
// same canonical catalog. Any difference is a hard failure.
func Verify(out string, options Options) (*Result, error) {
	expected, err := Generate(options)
	if err != nil {
		return nil, err
	}
	interfaceDir := filepath.Join(out, InterfacePackagePath)
	for _, file := range expected.Files {
		actual, err := os.ReadFile(filepath.Join(interfaceDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, fmt.Errorf("interfacepkg: staged package is missing %s: %w", file.Path, err)
		}
		if !bytes.Equal(actual, file.Bytes) {
			return nil, fmt.Errorf("interfacepkg: staged %s differs from the canonical generation", file.Path)
		}
	}
	entries, err := os.ReadDir(interfaceDir)
	if err != nil {
		return nil, err
	}
	expectedPaths := make(map[string]bool, len(expected.Files))
	for _, file := range expected.Files {
		expectedPaths[file.Path] = true
	}
	actualPaths := make([]string, 0, len(entries))
	var walk func(string, string) error
	walk = func(root, relative string) error {
		children, err := os.ReadDir(filepath.Join(root, relative))
		if err != nil {
			return err
		}
		for _, child := range children {
			childRelative := path.Join(relative, child.Name())
			if child.IsDir() {
				if err := walk(root, childRelative); err != nil {
					return err
				}
				continue
			}
			actualPaths = append(actualPaths, childRelative)
		}
		return nil
	}
	if err := walk(interfaceDir, ""); err != nil {
		return nil, err
	}
	for _, actual := range actualPaths {
		if !expectedPaths[actual] {
			return nil, fmt.Errorf("interfacepkg: staged package contains an unexpected file %s", actual)
		}
	}
	provider, err := os.ReadFile(filepath.Join(out, ProviderManifestName))
	if err != nil {
		return nil, fmt.Errorf("interfacepkg: staged package is missing %s: %w", ProviderManifestName, err)
	}
	if !bytes.Equal(provider, expected.ProviderManifest) {
		return nil, fmt.Errorf("interfacepkg: staged %s differs from the canonical generation", ProviderManifestName)
	}
	for _, file := range expected.RuntimeBindingsFiles {
		actual, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, fmt.Errorf("interfacepkg: staged runtime file is missing %s: %w", file.Path, err)
		}
		if !bytes.Equal(actual, file.Bytes) {
			return nil, fmt.Errorf("interfacepkg: staged runtime file %s differs from the canonical generation", file.Path)
		}
	}
	return expected, nil
}

// SnapshotDigest computes the frozen CA-SKILL-DEPENDENCY-SNAPSHOT v1 digest.
// Paths are sorted by their raw bytes; every file, including package.json, is
// covered. Input order is not significant.
func SnapshotDigest(files []GeneratedFile) string {
	ordered := append([]GeneratedFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	hash := sha256.New()
	_, _ = hash.Write([]byte(snapshotDigestDomain))
	var u32 [4]byte
	var u64 [8]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(ordered)))
	_, _ = hash.Write(u32[:])
	for _, file := range ordered {
		pathBytes := []byte(file.Path)
		binary.BigEndian.PutUint32(u32[:], uint32(len(pathBytes)))
		_, _ = hash.Write(u32[:])
		_, _ = hash.Write(pathBytes)
		binary.BigEndian.PutUint64(u64[:], uint64(len(file.Bytes)))
		_, _ = hash.Write(u64[:])
		_, _ = hash.Write(file.Bytes)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

type packageManifest struct {
	APIVersion     string              `json:"apiVersion"`
	PackageID      string              `json:"packageId"`
	Version        string              `json:"version"`
	LanguageTarget string              `json:"languageTarget"`
	Publisher      string              `json:"publisher"`
	Dependencies   []packageDependency `json:"dependencies"`
	Exports        []packageExport     `json:"exports"`
	Files          []packageFile       `json:"files"`
}

type packageDependency struct {
	Alias     string `json:"alias"`
	PackageID string `json:"packageId"`
	Version   string `json:"version"`
}

type packageExport struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

type packageFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type providerManifest struct {
	APIVersion              string           `yaml:"apiVersion"`
	ProviderID              string           `yaml:"providerId"`
	ProviderVersion         string           `yaml:"providerVersion"`
	Protocol                string           `yaml:"protocol"`
	BackendCatalogDigest    string           `yaml:"backendCatalogDigest"`
	Transport               transportBinding `yaml:"transport"`
	InterfacePackagePath    string           `yaml:"interfacePackagePath"`
	InterfaceSnapshotDigest string           `yaml:"interfaceSnapshotDigest"`
	RuntimeBindingsPath     string           `yaml:"runtimeBindingsPath"`
	RuntimeBindingsDigest   string           `yaml:"runtimeBindingsDigest"`
	Bindings                []runtimeBinding `yaml:"bindings"`
}

type transportBinding struct {
	Type string   `yaml:"type"`
	Argv []string `yaml:"argv"`
}

type runtimeBinding struct {
	PublicCallableID   string `yaml:"publicCallableId"`
	BackendOperationID string `yaml:"backendOperationId"`
}

func renderToolCatalog(loaded *catalog.Catalog) ([]byte, error) {
	document := toolCatalog{
		APIVersion: ToolCatalogAPIVersion,
		Kind:       ToolCatalogKind,
	}
	document.Metadata.Name = loaded.Interface.Name
	document.Metadata.Owner = loaded.Interface.Owner
	document.Spec.Contracts = make([]toolContract, 0, len(loaded.Tools))
	for _, tool := range loaded.Tools {
		contract := toolContract{
			Kind:          tool.Kind,
			ID:            tool.CallableID,
			Effect:        string(tool.Effect),
			OwnerOutcomes: append([]string(nil), tool.OwnerOutcomes...),
		}
		if tool.InputSchemaID != "" {
			contract.InputSchema = &schemaRef{Ref: tool.InputSchemaID}
		}
		if tool.QueryContextSchemaID != "" {
			contract.QueryContextSchema = &schemaRef{Ref: tool.QueryContextSchemaID}
		}
		contract.OutputSchema = &schemaRef{Ref: tool.OutputSchemaID}
		if tool.ReceiptSchemaID != "" {
			contract.ReceiptSchema = &schemaRef{Ref: tool.ReceiptSchemaID}
		}
		document.Spec.Contracts = append(document.Spec.Contracts, contract)
	}
	return encodeYAML(document)
}

type toolCatalog struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name  string `yaml:"name"`
		Owner string `yaml:"owner"`
	} `yaml:"metadata"`
	Spec struct {
		Contracts []toolContract `yaml:"contracts"`
	} `yaml:"spec"`
}

type toolContract struct {
	Kind               string     `yaml:"kind"`
	ID                 string     `yaml:"id"`
	Effect             string     `yaml:"effect"`
	OwnerOutcomes      []string   `yaml:"ownerOutcomes"`
	InputSchema        *schemaRef `yaml:"inputSchema,omitempty"`
	QueryContextSchema *schemaRef `yaml:"queryContextSchema,omitempty"`
	OutputSchema       *schemaRef `yaml:"outputSchema,omitempty"`
	ReceiptSchema      *schemaRef `yaml:"receiptSchema,omitempty"`
}

type schemaRef struct {
	Ref string `yaml:"$ref"`
}

func renderCapabilities(loaded *catalog.Catalog) ([]byte, error) {
	ids := capabilityIDs(loaded)
	document := capabilityDocument{Capabilities: make([]capability, 0, len(ids))}
	for _, id := range ids {
		members := make([]capabilityMember, 0)
		for _, tool := range loaded.Tools {
			for _, capabilityID := range tool.Capabilities {
				if capabilityID == id {
					members = append(members, capabilityMember{CallableID: tool.CallableID})
				}
			}
		}
		sort.Slice(members, func(i, j int) bool { return members[i].CallableID < members[j].CallableID })
		document.Capabilities = append(document.Capabilities, capability{ID: id, Members: members})
	}
	return encodeJSON(document)
}

type capabilityDocument struct {
	Capabilities []capability `json:"capabilities"`
}

type capability struct {
	ID      string             `json:"id"`
	Members []capabilityMember `json:"members"`
}

type capabilityMember struct {
	CallableID string `json:"callableId"`
}

func capabilityIDs(loaded *catalog.Catalog) []string {
	seen := make(map[string]bool)
	ids := make([]string, 0)
	for _, tool := range loaded.Tools {
		for _, id := range tool.Capabilities {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// loadSchemaSources reads every declared standalone Schema source, converts it
// to canonical JSON and proves its $id matches the declared identity.
func loadSchemaSources(contractDir string, loaded *catalog.Catalog) ([]GeneratedFile, map[string]GeneratedFile, error) {
	byID := make(map[string]GeneratedFile, len(loaded.SchemaFiles))
	files := make([]GeneratedFile, 0, len(loaded.SchemaFiles))
	for _, source := range loaded.SchemaFiles {
		raw, err := os.ReadFile(filepath.Join(contractDir, filepath.FromSlash(source.Path)))
		if err != nil {
			return nil, nil, fmt.Errorf("interfacepkg: read Schema source %s: %w", source.Path, err)
		}
		var document any
		if err := yaml.Unmarshal(raw, &document); err != nil {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s is not valid YAML: %w", source.Path, err)
		}
		encoded, err := encodeJSON(document)
		if err != nil {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s: %w", source.Path, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil || decoded == nil {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s is not a JSON object", source.Path)
		}
		if decoded["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s must declare the Draft 2020-12 $schema", source.Path)
		}
		if decoded["$id"] != source.ID {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s $id does not match the declared identity %s", source.Path, source.ID)
		}
		outPath := strings.TrimSuffix(source.Path, path.Ext(source.Path)) + ".json"
		if !strings.HasPrefix(outPath, "schemas/") || !strings.HasSuffix(outPath, ".schema.json") {
			return nil, nil, fmt.Errorf("interfacepkg: Schema source %s must map to schemas/*.schema.json", source.Path)
		}
		file := GeneratedFile{Path: outPath, Bytes: encoded}
		if _, duplicate := byID[source.ID]; duplicate {
			return nil, nil, fmt.Errorf("interfacepkg: duplicate Schema identity %s", source.ID)
		}
		byID[source.ID] = file
		files = append(files, file)
	}
	for _, tool := range loaded.Tools {
		for _, id := range []string{tool.InputSchemaID, tool.QueryContextSchemaID, tool.OutputSchemaID, tool.ReceiptSchemaID} {
			if id == "" {
				continue
			}
			if _, exists := byID[id]; !exists {
				return nil, nil, fmt.Errorf("interfacepkg: Tool %s references an undeclared Schema %s", tool.CallableID, id)
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, byID, nil
}

// encodeJSON is the single deterministic JSON encoding: sorted object keys,
// two-space indentation and exactly one trailing newline.
func encodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func encodeYAML(value any) ([]byte, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) {
		encoded = append(encoded, '\n')
	}
	return encoded, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256Digest(data []byte) string {
	return "sha256:" + sha256Hex(data)
}

// validVersion accepts only exact SemVer without build metadata or ranges.
func validVersion(value string) bool {
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "*^~<>=|, +") {
		return false
	}
	core := value
	if index := strings.IndexByte(core, '-'); index >= 0 {
		if index == 0 || index == len(core)-1 {
			return false
		}
		core = core[:index]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return false
		}
		for index := 0; index < len(part); index++ {
			if part[index] < '0' || part[index] > '9' {
				return false
			}
		}
	}
	return true
}

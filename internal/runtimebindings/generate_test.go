package runtimebindings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	yaml "gopkg.in/yaml.v3"
)

func TestGeneratePinnedOwnerClosure(t *testing.T) {
	generated, err := Generate(Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if generated.Bindings != 25 {
		t.Fatalf("runtime bindings = %d, want 25 (23 business + 2 evidence)", generated.Bindings)
	}
	if generated.MutationBindings != 21 || generated.ReadBindings != 4 {
		t.Fatalf("effects = %d mutation / %d read, want 21 / 4", generated.MutationBindings, generated.ReadBindings)
	}

	loaded, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog load: %v", err)
	}
	if err := catalog.InstallTypedMASBindings(loaded); err != nil {
		t.Fatalf("install typed closure: %v", err)
	}

	document := decodeSidecar(t, generated.Sidecar)
	if document.APIVersion != SidecarAPIVersion {
		t.Fatalf("apiVersion = %q", document.APIVersion)
	}
	if document.OwnerContract.StableID != "urn:openamsm:mas:tool-contracts" ||
		document.OwnerContract.Version != "v1alpha4" {
		t.Fatalf("Owner contract identity = %+v", document.OwnerContract)
	}
	if document.OwnerContract.Operations != 25 {
		t.Fatalf("Owner operation count = %d", document.OwnerContract.Operations)
	}

	seen := map[string]bool{}
	reconcileBindingSeen := false
	for _, binding := range document.Bindings {
		for _, legacy := range []string{
			"SC.UPDATE_SM_CONTEXT",
			"UPN.MODIFY_UP_SESSION_INTENT",
			"SSM.SIGNAL_UE_AN_MODIFY",
			"SSM.SIGNAL_UE_AN_RELEASE",
			"PA.DELETE_SM_POLICY",
			"SC.RELEASE_SM_CONTEXT",
			"SE.EMIT_RELEASE_EVENT",
			"UPN.MODIFY_QOS_FLOWS",
		} {
			if binding.RuntimeOperationID == legacy {
				t.Fatalf("legacy callable %s entered the runtime binding surface", legacy)
			}
		}
		if seen[binding.RuntimeOperationID] {
			t.Fatalf("duplicate runtime operation %s", binding.RuntimeOperationID)
		}
		seen[binding.RuntimeOperationID] = true
		if binding.OwnerCallableID != binding.RuntimeOperationID {
			t.Fatalf("%s owner link differs", binding.RuntimeOperationID)
		}
		if binding.CapabilityID == "" {
			t.Fatalf("%s has no Owner capability", binding.RuntimeOperationID)
		}
		if !strings.HasPrefix(binding.InputSchema.Ref, SchemaDirectory+"/") ||
			!strings.HasPrefix(binding.OutputSchema.Ref, SchemaDirectory+"/") {
			t.Fatalf("%s Schema refs must be package-local exports: %q %q", binding.RuntimeOperationID, binding.InputSchema.Ref, binding.OutputSchema.Ref)
		}

		entry, ok := loaded.Find(binding.BackendOperationID)
		if !ok {
			t.Fatalf("%s backend %s is not in the Provider catalog", binding.RuntimeOperationID, binding.BackendOperationID)
		}
		if string(entry.Effect) != binding.Effect {
			t.Fatalf("%s catalog effect %s differs from binding %s", binding.RuntimeOperationID, entry.Effect, binding.Effect)
		}

		switch binding.Effect {
		case "READ":
			if binding.CommitProof != nil {
				t.Fatalf("%s READ binding declares a commit proof", binding.RuntimeOperationID)
			}
			if binding.Observation != nil {
				if binding.Observation.Outcomes["WIRE_OK"] != "OUTPUTS" {
					t.Fatalf("%s READ observation must project outputs", binding.RuntimeOperationID)
				}
			}
		case "MUTATION":
			if binding.CommitProof == nil || binding.CommitProof.Mode != "OWNER_RECEIPT" {
				t.Fatalf("%s MUTATION lacks an Owner receipt contract", binding.RuntimeOperationID)
			}
			if binding.CommitProof.Receipt.SchemaVersion == "" {
				t.Fatalf("%s receipt declares no Owner schema identity", binding.RuntimeOperationID)
			}
			if binding.CommitProof.Reconcile == nil ||
				binding.CommitProof.Reconcile.BindingID != "mas-reconcile" {
				t.Fatalf("%s has no exact-original reconcile binding", binding.RuntimeOperationID)
			}
			if binding.Observation == nil || binding.Observation.EnvelopePath != "response.observation" {
				t.Fatalf("%s has no Provider observation contract", binding.RuntimeOperationID)
			}
			if _, ok := binding.Observation.Outcomes["WIRE_OK"]; !ok {
				t.Fatalf("%s observation contract has no success code", binding.RuntimeOperationID)
			}
			if binding.HostInvocation == nil || len(binding.HostInvocation.Fields) == 0 {
				t.Fatalf("%s has no Host invocation scope", binding.RuntimeOperationID)
			}
		}
		if binding.RuntimeOperationID == "MAS.RECONCILE" {
			reconcileBindingSeen = true
			if binding.BackendOperationID != "openamsm.smf.mas.reconcile" {
				t.Fatalf("reconcile backend = %s", binding.BackendOperationID)
			}
		}
	}
	if !reconcileBindingSeen {
		t.Fatal("MAS.RECONCILE is missing from the runtime binding surface")
	}

	// Every declared Schema export exists, is a JSON object Schema and is
	// covered by exactly one file digest.
	files := map[string][]byte{}
	for _, file := range generated.Files {
		files[file.Path] = file.Bytes
	}
	if _, ok := files[SidecarName]; !ok {
		t.Fatal("sidecar file missing")
	}
	for _, digest := range document.Files {
		payload, ok := files[digest.Path]
		if !ok {
			t.Fatalf("digest names an unknown file %s", digest.Path)
		}
		if sha256Hex(payload) != digest.SHA256 {
			t.Fatalf("file digest differs for %s", digest.Path)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatalf("%s is not JSON: %v", digest.Path, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("%s is not an object Schema", digest.Path)
		}
		if containsExternalRef(schema) {
			t.Fatalf("%s still contains a non-local $ref", digest.Path)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	first, err := Generate(Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Sidecar) != string(second.Sidecar) || len(first.Files) != len(second.Files) {
		t.Fatal("generation is not deterministic")
	}
	for index := range first.Files {
		if first.Files[index].Path != second.Files[index].Path ||
			string(first.Files[index].Bytes) != string(second.Files[index].Bytes) {
			t.Fatalf("file %d differs between runs", index)
		}
	}
}

func TestGenerateRejectsTamperedOwnerArtifact(t *testing.T) {
	directory := t.TempDir()
	copyTree(t, filepath.Join("..", "..", OwnerDirectory), directory)
	target := filepath.Join(directory, "schemas", "sc-create_sm_context.input.schema.json")
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(payload, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{OwnerDir: directory, ProviderVersion: "1.2.0"}); err == nil ||
		!strings.Contains(err.Error(), "digest differs") {
		t.Fatalf("tampered Owner artifact was accepted: %v", err)
	}
}

func TestVerifyDetectsSidecarTampering(t *testing.T) {
	out := t.TempDir()
	generated, err := Generate(Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range generated.Files {
		target := filepath.Join(out, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, file.Bytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Verify(out, Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	target := filepath.Join(out, SidecarName)
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(payload, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(out, Options{OwnerDir: filepath.Join("..", "..", OwnerDirectory), ProviderVersion: "1.2.0"}); err == nil {
		t.Fatal("tampered sidecar passed verification")
	}
}

func decodeSidecar(t *testing.T, payload []byte) sidecarDocument {
	t.Helper()
	var document sidecarDocument
	decoder := json.NewDecoder(strings.NewReader(yamlToJSON(t, payload)))
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode sidecar: %v", err)
	}
	return document
}

func yamlToJSON(t *testing.T, payload []byte) string {
	t.Helper()
	var value any
	if err := yaml.Unmarshal(payload, &value); err != nil {
		t.Fatalf("decode sidecar YAML: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("normalize sidecar: %v", err)
	}
	return string(encoded)
}

func containsExternalRef(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok && reference != "" && !strings.HasPrefix(reference, "#") {
			return true
		}
		for _, child := range typed {
			if containsExternalRef(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsExternalRef(child) {
				return true
			}
		}
	}
	return false
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		payload, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, payload, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
}

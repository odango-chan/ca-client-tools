package runtimebindings

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
)

// TestProviderOwnerFieldClosure reports any divergence between the reviewed Go
// callable fields and the pinned Owner input/output Schemas. It is a release
// diagnostic: a divergence is a contract finding, not something the generator
// may paper over.
func TestProviderOwnerFieldClosure(t *testing.T) {
	owner, err := loadOwner(filepath.Join("..", "..", OwnerDirectory))
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, operation := range owner.Manifest.Operations {
		if operation.Table == "evidence" {
			continue
		}
		bind, ok := mas.LookupTypedBinding(operation.Callable)
		if !ok {
			continue
		}
		contract := owner.Contracts[operation.Callable]
		input := ownerSchemaProperties(t, owner, contract.InputSchema.Ref)
		output := ownerSchemaProperties(t, owner, contract.OutputSchema.Ref)
		private := map[string]bool{}
		for _, argument := range privateArgumentsFor(operation.Callable) {
			private[argument.Name] = true
		}
		for _, field := range bind.Fields {
			if private[field.Name] {
				continue
			}
			if _, ok := input[field.Name]; !ok {
				problems = append(problems, operation.Callable+" input Schema lacks Provider field "+field.Name)
			}
		}
		for name := range input {
			found := false
			for _, field := range bind.Fields {
				if field.Name == name {
					found = true
					break
				}
			}
			if !found && !private[name] {
				problems = append(problems, operation.Callable+" Owner input property "+name+" is not a Provider field")
			}
		}
		_ = output
	}
	sort.Strings(problems)
	for _, problem := range problems {
		t.Error(problem)
	}
	if len(problems) > 0 {
		t.Fatalf("%d Provider/Owner field closure differences", len(problems))
	}
}

func ownerSchemaProperties(t *testing.T, owner *ownerPackage, reference string) map[string]any {
	t.Helper()
	document, ok := owner.SchemaDocuments[reference]
	if !ok {
		t.Fatalf("%s does not resolve", reference)
	}
	object, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object", reference)
	}
	properties, _ := object["properties"].(map[string]any)
	return properties
}

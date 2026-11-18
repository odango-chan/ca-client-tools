//go:build providerfixture

package providerapp

import "github.com/odango-chan/ca-client-tools/internal/catalog"

// installFixtureBindings adds the acceptance-only trusted bindings. It exists
// only in the explicit providerfixture build and is never part of a deployed
// Provider binary.
func installFixtureBindings(cat *catalog.Catalog) error {
	return catalog.InstallAcceptanceBindings(cat)
}

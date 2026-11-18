//go:build !providerfixture

package providerapp

import "github.com/odango-chan/ca-client-tools/internal/catalog"

// installFixtureBindings is a no-op in every production build. The
// providerfixture build tag installs acceptance-only trusted bindings and never
// ships in a deployed Provider binary.
func installFixtureBindings(*catalog.Catalog) error { return nil }

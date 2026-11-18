// Package providerbindings embeds the single trusted Provider Backend Binding
// Catalog artifact. The YAML file under v2/ is the only authoritative copy; the
// catalog loader verifies its exact bytes and SHA-256 digest.
//
// P0 removed the legacy v1 artifact entirely: there is no version dispatch and
// no compatibility artifact.
package providerbindings

import _ "embed"

// Catalog is the sole ca-client-tools.provider-bindings/v2 artifact.
//
//go:embed v2/provider-bindings.yaml
var Catalog []byte

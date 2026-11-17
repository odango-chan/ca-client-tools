// Package providerbindings embeds the trusted Provider Backend Binding Catalog
// artifacts. The YAML files under v1/ and v2/ are the only authoritative
// copies; the catalog loader verifies their exact bytes and SHA-256 digest.
//
// The v1 catalog is frozen. The v2 catalog is additive: it keeps every v1
// private backend operation ID and adds the source-verified OpenAMSM list
// operations plus the generic SBI binding shape.
package providerbindings

import _ "embed"

// V1 is the frozen ca-client-tools.provider-bindings/v1 artifact.
//
//go:embed v1/provider-bindings.yaml
var V1 []byte

// V2 is the additive ca-client-tools.provider-bindings/v2 artifact.
//
//go:embed v2/provider-bindings.yaml
var V2 []byte

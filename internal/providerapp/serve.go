package providerapp

import (
	"context"
	"io"

	"github.com/odango-chan/ca-client-tools/internal/provider"
)

// Serve runs the resident Provider process on the given streams. stdout
// carries protocol frames only; stderr carries safe diagnostics only.
func Serve(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, lookup LookupEnv) error {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	return provider.Serve(ctx, provider.Options{
		In:  stdin,
		Out: stdout,
		Err: stderr,
		Info: provider.ProviderInfo{
			ProviderID:      provider.ProviderID,
			ProviderVersion: ProviderVersion,
			BuildDigest:     BuildDigest(),
		},
		NewRuntime: func(protocolVersion string) (provider.Runtime, error) {
			return newRuntime(protocolVersion, lookup)
		},
	})
}

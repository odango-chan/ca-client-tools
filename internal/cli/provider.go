package cli

import (
	"github.com/odango-chan/ca-client-tools/internal/providerapp"
	"github.com/spf13/cobra"
)

// newProviderCommand exposes the resident Provider machine interface. It is
// hidden from the CLI v1 help surface because it is a separate subprocess
// protocol owned by the Host adapter, not a human/Agent CLI command. Existing
// CLI v1 commands and their JSON stdin/stdout/stderr contract are unchanged.
func newProviderCommand(deps dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:    "provider",
		Short:  "Resident Tool Provider process interface",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   showHelp,
	}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Run the resident Provider over the NDJSON process protocol",
		Long: "Run one long-lived Provider process. Protocol frames are read from stdin and " +
			"written to stdout; diagnostics go to stderr only. Trusted deployment configuration " +
			"is read from the allowlisted CA_OPENAMSM_* environment variables.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := providerapp.Serve(cmd.Context(), deps.stdin, deps.stdout, deps.stderr, providerapp.LookupEnv(deps.getenv)); err != nil {
				return internalFailure()
			}
			return nil
		},
	}
	command.AddCommand(serve)
	return command
}

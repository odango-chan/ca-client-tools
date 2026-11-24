package cli

import (
	"github.com/odango-chan/ca-client-tools/protocol/codec/worker"
	"github.com/spf13/cobra"
)

// newCodecCommand exposes the long-lived codec worker. It is hidden from the
// CLI v1 help surface because it is a Host-owned subprocess interface, not a
// human or Agent command. The worker has no network listener and no Provider
// registration.
func newCodecCommand(deps dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:    "codec",
		Short:  "Protocol codec process interface",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   showHelp,
	}
	workerCommand := &cobra.Command{
		Use:   "worker",
		Short: "Run the long-lived framed codec worker over stdin/stdout",
		Long: "Run one long-lived codec worker. Requests are length-bounded frames containing " +
			"a JSON header and an optional raw binary segment. Diagnostics go to stderr only; " +
			"no network port is opened.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := worker.Serve(cmd.Context(), deps.stdin, deps.stdout, deps.stderr, worker.DefaultMaxFrameBytes); err != nil {
				return internalFailure()
			}
			return nil
		},
	}
	command.AddCommand(workerCommand)
	return command
}

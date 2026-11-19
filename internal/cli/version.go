package cli

import "strings"

// These variables are set by release builds through Go linker -X flags.
// Local builds deliberately identify themselves as development builds.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func versionDisplay() string {
	metadata := make([]string, 0, 2)
	if GitCommit != "" && GitCommit != "unknown" {
		metadata = append(metadata, "commit "+GitCommit)
	}
	if BuildDate != "" && BuildDate != "unknown" {
		metadata = append(metadata, "built "+BuildDate)
	}
	if len(metadata) == 0 {
		return Version
	}
	return Version + " (" + strings.Join(metadata, "; ") + ")"
}

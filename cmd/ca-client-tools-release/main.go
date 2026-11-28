// Command ca-client-tools-release generates the release Interface Package and
// the Provider runtime manifest from the single canonical Tool definition.
//
// It is a release tool, not part of the shipped CLI: the frozen CLI v1 command
// tree is unchanged. The release workflow builds the binaries, runs this
// generator exactly once and packages the identical interface tree into every
// platform archive and into the standalone Interface Package ZIP.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/odango-chan/ca-client-tools/internal/interfacepkg"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "generate":
		generate(os.Args[2:])
	case "verify":
		verify(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ca-client-tools-release generate|verify --out DIR --version VERSION --provider-version VERSION [--catalog PATH]")
}

func generate(arguments []string) {
	options, out := parse(arguments)
	result, err := interfacepkg.Generate(options)
	if err != nil {
		fail(err)
	}
	if err := result.Write(out); err != nil {
		fail(err)
	}
	report(result)
}

func verify(arguments []string) {
	options, out := parse(arguments)
	result, err := interfacepkg.Verify(out, options)
	if err != nil {
		fail(err)
	}
	report(result)
}

func parse(arguments []string) (interfacepkg.Options, string) {
	flags := flag.NewFlagSet("release", flag.ExitOnError)
	out := flags.String("out", "", "staging directory")
	version := flags.String("version", "", "Interface Package version (release tag without the leading v)")
	providerVersion := flags.String("provider-version", "", "Provider binary version reported by initialize")
	catalogPath := flags.String("catalog", interfacepkg.CatalogPath, "canonical Tool definition")
	if err := flags.Parse(arguments); err != nil {
		fail(err)
	}
	if *out == "" || *version == "" {
		usage()
		os.Exit(2)
	}
	if *providerVersion == "" {
		*providerVersion = *version
	}
	return interfacepkg.Options{
		CatalogPath:     *catalogPath,
		PackageVersion:  *version,
		ProviderVersion: *providerVersion,
	}, *out
}

func report(result *interfacepkg.Result) {
	payload := map[string]any{
		"packageId":               result.PackageID,
		"packageVersion":          result.PackageVersion,
		"providerVersion":         result.ProviderVersion,
		"backendCatalogDigest":    result.BackendCatalogDigest,
		"interfaceSnapshotDigest": result.InterfaceSnapshotDigest,
		"files":                   len(result.Files),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(encoded))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release:", err)
	os.Exit(1)
}

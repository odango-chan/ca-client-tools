package main

import (
	"os"

	"github.com/odango-chan/ca-client-tools/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

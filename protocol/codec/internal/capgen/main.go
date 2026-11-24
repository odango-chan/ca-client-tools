// Command capgen writes the checked-in CA codec capability manifest.
package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/odango-chan/ca-client-tools/protocol/codec"
)

func main() {
	out := flag.String("out", "docs/codec-capabilities.json", "output path")
	flag.Parse()
	data, err := json.MarshalIndent(codec.Capabilities(), "", "  ")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		panic(err)
	}
}

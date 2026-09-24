package main

import (
	"fmt"
	"os"

	"github.com/colony-2/colony2/server/internal/cortexcli"
	"github.com/colony-2/colony2/server/internal/webdist"
)

// Set by the release workflow through Go linker flags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := cortexcli.NewCommand(cortexcli.Options{Version: version, StaticFS: webdist.FS()}).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

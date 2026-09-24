package main

import (
	"fmt"
	"os"

	"github.com/colony-2/colony2/server/internal/cortexcli"
)

func main() {
	if err := cortexcli.NewCommand(cortexcli.Options{DefaultCORS: "http://localhost:3000,http://127.0.0.1:3000"}).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"os"

	"github.com/spf13/cobra"
	"vibethis/ono/cmd/ono"
)

var rootCmd = &cobra.Command{
	Use:   "ono",
	Short: "A CLI tool for orchestrating Temporal workflows with YAML",
}

func init() {
	rootCmd.AddCommand(ono.StartCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
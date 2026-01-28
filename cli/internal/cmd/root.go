package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/output"
)

// NewRootCmd wires the CLI command tree.
func NewRootCmd() *cobra.Command {
	var cfgPath string
	var flagCfg config.Config

	root := &cobra.Command{
		Use:           "colony2",
		Short:         "Colony2 CLI for the REST APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			fileCfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}
			merged := config.Merge(
				config.Default(),
				fileCfg,
				config.FromEnv(),
				flagCfg,
			)
			if err := merged.Validate(false); err != nil {
				return err
			}
			cl, err := client.New(merged)
			if err != nil {
				return err
			}
			pr := output.New(merged.Output, os.Stdout)
			cmd.SetContext(storeApp(cmd.Context(), &App{
				Config:  merged,
				Client:  cl,
				Printer: pr,
			}))
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "config file path")
	root.PersistentFlags().StringVar(&flagCfg.APIURL, "api-url", "", "API base URL (COLONY2_API_URL)")
	root.PersistentFlags().StringVar(&flagCfg.Token, "token", "", "Bearer token (COLONY2_TOKEN)")
	root.PersistentFlags().StringVar(&flagCfg.Project, "project", "", "Project ID (COLONY2_PROJECT)")
	root.PersistentFlags().StringVar(&flagCfg.Output, "output", "", "Output format: table|json (COLONY2_OUTPUT)")
	root.PersistentFlags().DurationVar(&flagCfg.Timeout, "timeout", 0, "Request timeout (COLONY2_TIMEOUT, e.g. 30s)")
	root.PersistentFlags().BoolVar(&flagCfg.Trace, "trace", false, "Trace HTTP requests (COLONY2_TRACE)")

	root.AddCommand(
		newTicketCmd(),
		newRecipeCmd(),
		newWorkflowCmd(),
		newInputCmd(),
		newCellCmd(),
		newProjectCmd(),
	)

	return root
}

func requireProject(project string) error {
	if project == "" {
		return fmt.Errorf("project is required; set --project or COLONY2_PROJECT")
	}
	return nil
}

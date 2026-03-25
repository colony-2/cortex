package cmd

import (
	"context"
	"os"

	"github.com/colony-2/colony2/server/c2j/internal/submitjob"
	"github.com/spf13/cobra"
)

func newSubmitCmd() *cobra.Command {
	opts := submitjob.Options{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a new recipe job through the SWF remote runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			return submitjob.Run(context.Background(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.TenantID, "tenant-id", "", "Tenant/project ID for the job")
	flags.StringVar(&opts.SWFURL, "swf-url", "", "Base URL for the SWF remote runtime")
	flags.StringVar(&opts.Recipe, "recipe", "", "Recipe name/reference to submit")
	flags.StringVar(&opts.RecipeFile, "recipe-file", "", "Path to a recipe YAML file to submit")
	flags.StringVar(&opts.RecipesDir, "recipes-dir", "", "Directory used to resolve --recipe locally (defaults to current directory when --recipe is used)")
	flags.StringVar(&opts.InputsJSON, "inputs-json", "", "Inline JSON object for recipe inputs")
	flags.StringVar(&opts.InputsFile, "inputs-file", "", "Path to a JSON or YAML file containing recipe inputs")
	flags.StringVar(&opts.RepoPath, "repo-path", "", "Base repository path recorded in job context (defaults to current directory)")
	flags.StringVar(&opts.GitRef, "git-ref", "", "Git ref/hash recorded in job context (defaults to current HEAD when available)")
	flags.StringVar(&opts.CellPath, "cell-path", "", "Cell path recorded in job context (defaults to .)")
	flags.StringVar(&opts.CellName, "cell-name", "", "Cell name recorded in job context (defaults from --cell-path)")
	flags.StringVar(&opts.ActorEmail, "actor-email", "", "Actor email recorded in job context")
	flags.StringVar(&opts.TicketID, "ticket-id", "", "Ticket ID recorded in job context")
	flags.BoolVar(&opts.JSONOutput, "json", false, "Emit the submitted job identity as JSON")

	return cmd
}

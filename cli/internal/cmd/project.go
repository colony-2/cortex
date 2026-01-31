package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/cli/internal/openapi"
)

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(
		newProjectListCmd(),
		newProjectCreateCmd(),
		newProjectDeleteCmd(),
		newProjectUpdateCmd(),
	)
	return cmd
}

func newProjectListCmd() *cobra.Command {
	var ids []string
	var names []string
	var nameContains string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			params := &openapi.GetApiProjectsParams{}
			if len(ids) > 0 {
				params.Ids = &ids
			}
			if len(names) > 0 {
				params.Names = &names
			}
			if nameContains != "" {
				params.NameContains = &nameContains
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.GetApiProjectsWithResponse(ctx, params)
			if err != nil {
				return err
			}
			payload, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			projects := *payload
			if app.Config.Output == "json" {
				return app.Printer.JSON(projects)
			}
			headers := []string{"ID", "Name", "Repo Path", "Updated", "Created"}
			rows := make([][]string, 0, len(projects))
			for _, p := range projects {
				rows = append(rows, []string{
					p.Id,
					p.Name,
					p.GitRepoPath,
					p.UpdatedAt.Format(time.RFC3339),
					p.CreatedAt.Format(time.RFC3339),
				})
			}
			return app.Printer.Table(headers, rows)
		},
	}

	cmd.Flags().StringSliceVar(&ids, "id", nil, "Filter by project id (repeatable)")
	cmd.Flags().StringSliceVar(&names, "name", nil, "Filter by project name (repeatable)")
	cmd.Flags().StringVar(&nameContains, "name-contains", "", "Substring match on project name")
	return cmd
}

func newProjectCreateCmd() *cobra.Command {
	var name string
	var gitRepoPath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if name == "" || gitRepoPath == "" {
				return fmt.Errorf("name and git-repo-path are required")
			}
			req := openapi.ProjectCreateRequest{
				Name:        name,
				GitRepoPath: gitRepoPath,
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PostApiProjectsWithResponse(ctx, req)
			if err != nil {
				return err
			}
			project, err := requirePayload(resp.JSON201, resp.HTTPResponse, resp.Body, 201)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(project)
			}
			return app.Printer.Text(project.Id)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&gitRepoPath, "git-repo-path", "", "Git repo path")
	return cmd
}

func newProjectDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <project-id>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.DeleteApiProjectsProjectIdWithResponse(ctx, args[0])
			if err != nil {
				return err
			}
			if err := expectStatus(resp.HTTPResponse, resp.Body, 200, 202, 204); err != nil {
				return err
			}
			return app.Printer.Text("deleted")
		},
	}
	return cmd
}

func newProjectUpdateCmd() *cobra.Command {
	var name string
	var gitRepoPath string
	var gitRepoBranch string
	var defaultTicketRecipe string
	var clearDefaultTicketRecipe bool

	cmd := &cobra.Command{
		Use:   "update <project-id>",
		Short: "Update a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}

			var req openapi.ProjectUpdateRequest
			if name != "" {
				req.Name = &name
			}
			if gitRepoPath != "" {
				req.GitRepoPath = &gitRepoPath
			}
			if gitRepoBranch != "" {
				req.GitRepoBranch = &gitRepoBranch
			}
			if defaultTicketRecipe != "" {
				req.DefaultTicketRecipe = &defaultTicketRecipe
			}
			if clearDefaultTicketRecipe {
				empty := ""
				req.DefaultTicketRecipe = &empty
			}
			if req == (openapi.ProjectUpdateRequest{}) {
				return fmt.Errorf("no update fields provided; use flags")
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PatchApiProjectsProjectIdWithResponse(ctx, args[0], req)
			if err != nil {
				return err
			}
			project, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(project)
			}
			return app.Printer.Text("updated")
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New project name")
	cmd.Flags().StringVar(&gitRepoPath, "git-repo-path", "", "New git repo path")
	cmd.Flags().StringVar(&gitRepoBranch, "git-repo-branch", "", "Git branch")
	cmd.Flags().StringVar(&defaultTicketRecipe, "default-ticket-recipe", "", "Default ticket recipe name")
	cmd.Flags().BoolVar(&clearDefaultTicketRecipe, "clear-default-ticket-recipe", false, "Clear default ticket recipe")
	return cmd
}

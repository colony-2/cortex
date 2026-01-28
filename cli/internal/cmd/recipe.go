package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func newRecipeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Manage recipes",
	}
	cmd.AddCommand(
		newRecipeListCmd(),
		newRecipeCreateCmd(),
		newRecipeUpdateCmd(),
	)
	return cmd
}

func newRecipeListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recipes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			params := &openapi.ListRecipesParams{}
			if status != "" {
				s := openapi.ListRecipesParamsStatus(status)
				params.Status = &s
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.ListRecipesWithResponse(ctx, app.Config.Project, params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			list := resp.JSON200.Recipes
			if app.Config.Output == "json" {
				return app.Printer.JSON(list)
			}
			headers := []string{"Name", "Latest Commit", "Latest At", "Published At"}
			rows := make([][]string, 0, len(list))
			for _, r := range list {
				rows = append(rows, []string{
					r.Name,
					r.LatestCommit,
					r.LatestCommitAt.Format(time.RFC3339),
					timePtr(r.PublishedAt),
				})
			}
			return app.Printer.Table(headers, rows)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (all|published|unpublished)")
	return cmd
}

func newRecipeCreateCmd() *cobra.Command {
	var file string
	var publish bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a recipe (payload via YAML/JSON)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			data, err := readData(file)
			if err != nil {
				return err
			}
			var req openapi.CreateRecipeRequest
			if err := unmarshalYAMLOrJSON(data, &req); err != nil {
				return fmt.Errorf("parse request: %w", err)
			}
			if publish {
				req.AutoPublish = boolPtr(true)
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.CreateRecipeWithResponse(ctx, app.Config.Project, req)
			if err != nil {
				return err
			}
			if resp.JSON201 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(resp.JSON201)
			}
			return app.Printer.Text("created")
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "CreateRecipeRequest payload file (YAML/JSON)")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish immediately after create")
	return cmd
}

func newRecipeUpdateCmd() *cobra.Command {
	var file string
	var publish bool
	cmd := &cobra.Command{
		Use:   "update <recipe-name>",
		Short: "Update a recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			data, err := readData(file)
			if err != nil {
				return err
			}
			var req openapi.UpdateRecipeRequest
			if err := unmarshalYAMLOrJSON(data, &req); err != nil {
				return fmt.Errorf("parse request: %w", err)
			}
			if publish {
				req.AutoPublish = boolPtr(true)
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.UpdateRecipeWithResponse(ctx, app.Config.Project, args[0], req)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(resp.JSON200)
			}
			return app.Printer.Text("updated")
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "UpdateRecipeRequest payload file (YAML/JSON)")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish immediately after update")
	return cmd
}

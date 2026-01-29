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
		newRecipeValidateCmd(),
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
			payload, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			list := payload.Recipes
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
	var publish bool
	var name string
	var contentFile string
	var contentLiteral string
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
			var req openapi.CreateRecipeRequest
			if name == "" {
				return fmt.Errorf("name is required")
			}
			req.Name = name
			if contentLiteral == "" && contentFile == "" {
				return fmt.Errorf("content is required (use --content or --content-file)")
			}
			if contentLiteral != "" {
				req.Content = contentLiteral
			}
			if contentFile != "" {
				data, err := readData(contentFile)
				if err != nil {
					return err
				}
				req.Content = string(data)
			}
			if req.Name == "" {
				return fmt.Errorf("name is required (missing in payload)")
			}
			if req.Content == "" {
				return fmt.Errorf("content is required (missing in payload)")
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
			payload, err := requirePayload(resp.JSON201, resp.HTTPResponse, resp.Body, 201)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(payload)
			}
			return app.Printer.Text("created")
		},
	}
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish immediately after create")
	cmd.Flags().StringVar(&name, "name", "", "Recipe name")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "Path to recipe content")
	cmd.Flags().StringVar(&contentLiteral, "content", "", "Inline recipe content (YAML)")
	return cmd
}

func newRecipeUpdateCmd() *cobra.Command {
	var publish bool
	var contentFile string
	var contentLiteral string
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
			var req openapi.UpdateRecipeRequest
			// Content is required for update.
			if contentLiteral == "" && contentFile == "" {
				return fmt.Errorf("content is required (use --content or --content-file)")
			}
			if contentLiteral != "" {
				req.Content = contentLiteral
			}
			if contentFile != "" {
				data, err := readData(contentFile)
				if err != nil {
					return err
				}
				req.Content = string(data)
			}
			if req.Content == "" {
				return fmt.Errorf("content is required (missing in payload)")
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
			payload, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(payload)
			}
			return app.Printer.Text("updated")
		},
	}
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish immediately after update")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "Path to recipe content")
	cmd.Flags().StringVar(&contentLiteral, "content", "", "Inline recipe content (YAML)")
	return cmd
}

func newRecipeValidateCmd() *cobra.Command {
	var name string
	var contentFile string
	var contentLiteral string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a recipe definition without saving",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("name is required")
			}
			if contentLiteral == "" && contentFile == "" {
				return fmt.Errorf("content is required (use --content or --content-file)")
			}
			req := openapi.ValidateRecipeRequest{Name: name}
			if contentLiteral != "" {
				req.Content = contentLiteral
			}
			if contentFile != "" {
				data, err := readData(contentFile)
				if err != nil {
					return err
				}
				req.Content = string(data)
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.ValidateRecipeWithResponse(ctx, app.Config.Project, req)
			if err != nil {
				return err
			}
			payload := resp.JSON200
			if payload == nil {
				payload = resp.JSON400
			}
			result, err := requirePayload(payload, resp.HTTPResponse, resp.Body, 200, 400)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(result)
			}
			if result.Valid {
				return app.Printer.Text("valid")
			}
			headers := []string{"Path", "Message"}
			rows := make([][]string, 0, len(result.Errors))
			for _, e := range result.Errors {
				path := ""
				if e.Path != nil {
					path = *e.Path
				}
				rows = append(rows, []string{path, e.Message})
			}
			return app.Printer.Table(headers, rows)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Recipe name")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "Path to recipe content")
	cmd.Flags().StringVar(&contentLiteral, "content", "", "Inline recipe content (YAML)")
	return cmd
}

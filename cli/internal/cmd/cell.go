package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func newCellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cell",
		Short: "Inspect cells",
	}
	cmd.AddCommand(
		newCellListCmd(),
		newCellCreateCmd(),
		newCellUpdateCmd(),
	)
	return cmd
}

func newCellListCmd() *cobra.Command {
	var names []string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cells",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			params := &openapi.GetApiProjectsProjectIdCellsParams{}
			if len(names) > 0 {
				params.Names = &names
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.GetApiProjectsProjectIdCellsWithResponse(ctx, app.Config.Project, params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			cells := *resp.JSON200
			if app.Config.Output == "json" {
				return app.Printer.JSON(cells)
			}
			headers := []string{"ID", "Name", "Deps", "Created", "Updated"}
			rows := make([][]string, 0, len(cells))
			for _, c := range cells {
				depCount := 0
				if c.Dependencies != nil {
					depCount = len(*c.Dependencies)
				}
				rows = append(rows, []string{
					c.Id,
					c.Name,
					fmt.Sprintf("%d", depCount),
					c.CreatedAt.Format(time.RFC3339),
					c.UpdatedAt.Format(time.RFC3339),
				})
			}
			return app.Printer.Table(headers, rows)
		},
	}
	cmd.Flags().StringSliceVar(&names, "name", nil, "Filter by cell name")
	return cmd
}

func newCellCreateCmd() *cobra.Command {
	var file string
	var name string
	var workingPath string
	var description string
	var populator string
	var populatorId string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cell",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			var req openapi.CellCreateRequest
			if file != "" {
				data, err := readData(file)
				if err != nil {
					return err
				}
				if err := unmarshalYAMLOrJSON(data, &req); err != nil {
					return fmt.Errorf("parse request: %w", err)
				}
			} else {
				if name == "" || workingPath == "" {
					return fmt.Errorf("name and working-path are required (or provide --file)")
				}
				req.Name = name
				req.WorkingPath = workingPath
				if description != "" {
					req.Description = &description
				}
				if populator != "" {
					req.Populator = &populator
				}
				if populatorId != "" {
					req.PopulatorId = &populatorId
				}
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PostApiProjectsProjectIdCellsWithResponse(ctx, app.Config.Project, req)
			if err != nil {
				return err
			}
			if resp.JSON201 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(resp.JSON201)
			}
			return app.Printer.Text(resp.JSON201.Id)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "CellCreateRequest payload (YAML/JSON)")
	cmd.Flags().StringVar(&name, "name", "", "Cell name")
	cmd.Flags().StringVar(&workingPath, "working-path", "", "Working path for the cell")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&populator, "populator", "", "Populator name")
	cmd.Flags().StringVar(&populatorId, "populator-id", "", "Populator ID")
	return cmd
}

func newCellUpdateCmd() *cobra.Command {
	var file string
	var name string
	var workingPath string
	var description string

	cmd := &cobra.Command{
		Use:   "update <cell-id>",
		Short: "Update a cell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			var req openapi.CellUpdateRequest
			if file != "" {
				data, err := readData(file)
				if err != nil {
					return err
				}
				if err := unmarshalYAMLOrJSON(data, &req); err != nil {
					return fmt.Errorf("parse request: %w", err)
				}
			}
			if name != "" {
				req.Name = &name
			}
			if workingPath != "" {
				req.WorkingPath = &workingPath
			}
			if description != "" {
				req.Description = &description
			}
			if req == (openapi.CellUpdateRequest{}) {
				return fmt.Errorf("no update fields provided; use flags or --file")
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PatchApiProjectsProjectIdCellsCellIdWithResponse(ctx, app.Config.Project, args[0], req)
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

	cmd.Flags().StringVarP(&file, "file", "f", "", "CellUpdateRequest payload (YAML/JSON)")
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&workingPath, "working-path", "", "New working path")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	return cmd
}

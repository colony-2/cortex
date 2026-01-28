package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func newTicketCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ticket",
		Short: "Manage tickets",
	}
	cmd.AddCommand(
		newTicketListCmd(),
		newTicketCreateCmd(),
	)
	return cmd
}

func newTicketListCmd() *cobra.Command {
	var states []string
	var stages []string
	var cells []string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tickets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()

			params := &openapi.GetApiProjectsProjectIdTicketsParams{}
			if len(states) > 0 {
				var ts []openapi.TicketState
				for _, s := range states {
					ts = append(ts, openapi.TicketState(s))
				}
				params.States = &ts
			}
			if len(stages) > 0 {
				params.Stages = &stages
			}
			if len(cells) > 0 {
				params.Cells = &cells
			}

			resp, err := app.Client.GetApiProjectsProjectIdTicketsWithResponse(ctx, app.Config.Project, params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			tickets := *resp.JSON200
			if app.Config.Output == "json" {
				return app.Printer.JSON(tickets)
			}
			headers := []string{"ID", "Title", "State", "Stage", "Cell", "Created", "Updated"}
			rows := make([][]string, 0, len(tickets))
			for _, t := range tickets {
				rows = append(rows, []string{
					t.Id,
					t.Title,
					string(t.State),
					t.Stage,
					t.CellName,
					t.CreatedAt.Format(time.RFC3339),
					t.UpdatedAt.Format(time.RFC3339),
				})
			}
			return app.Printer.Table(headers, rows)
		},
	}

	cmd.Flags().StringSliceVar(&states, "state", nil, "Filter by ticket state")
	cmd.Flags().StringSliceVar(&stages, "stage", nil, "Filter by stage")
	cmd.Flags().StringSliceVar(&cells, "cell", nil, "Filter by cell name")
	return cmd
}

func newTicketCreateCmd() *cobra.Command {
	var file string
	var title string
	var cell string
	var state string
	var stage string
	var description string
	var actorType string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()

			var req openapi.TicketCreateRequest
			if file != "" {
				data, err := readData(file)
				if err != nil {
					return err
				}
				if err := unmarshalYAMLOrJSON(data, &req); err != nil {
					return fmt.Errorf("parse request: %w", err)
				}
			} else {
				if title == "" || cell == "" || state == "" || stage == "" {
					return fmt.Errorf("title, cell, state, and stage are required (use flags or --file)")
				}
				req = openapi.TicketCreateRequest{
					Actor: openapi.Actor{Type: openapi.User},
					Cell:  cell,
					State: openapi.TicketState(state),
					Stage: stage,
					Title: title,
				}
				if description != "" {
					req.Description = &description
				}
			}

			// Apply actor overrides if provided.
			switch actorType {
			case "":
				// leave as-is
			case string(openapi.Agent), string(openapi.User):
				req.Actor.Type = openapi.ActorType(actorType)
			default:
				return fmt.Errorf("invalid --actor-type %q", actorType)
			}
			if req.Actor.Type == "" {
				req.Actor.Type = openapi.User
			}

			resp, err := app.Client.PostApiProjectsProjectIdTicketsWithResponse(ctx, app.Config.Project, req)
			if err != nil {
				return err
			}
			if resp.JSON201 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			ticket := resp.JSON201
			if app.Config.Output == "json" {
				return app.Printer.JSON(ticket)
			}
			return app.Printer.Text(ticket.Id)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Ticket create payload (YAML/JSON, use - for stdin)")
	cmd.Flags().StringVar(&title, "title", "", "Ticket title")
	cmd.Flags().StringVar(&cell, "cell", "", "Cell name")
	cmd.Flags().StringVar(&state, "state", "", "Ticket state")
	cmd.Flags().StringVar(&stage, "stage", "", "Ticket stage")
	cmd.Flags().StringVar(&description, "description", "", "Ticket description")
	cmd.Flags().StringVar(&actorType, "actor-type", "", "Actor type user|agent")
	return cmd
}

package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Inspect workflows",
	}
	cmd.AddCommand(newWorkflowListCmd())
	return cmd
}

func newWorkflowListCmd() *cobra.Command {
	var statuses []string
	var ticketID string
	var cellID string
	var sinceStr string
	var untilStr string
	var limit int
	var offset int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}

			var sincePtr, untilPtr *time.Time
			if sinceStr != "" {
				t, err := time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				sincePtr = &t
			}
			if untilStr != "" {
				t, err := time.Parse(time.RFC3339, untilStr)
				if err != nil {
					return fmt.Errorf("invalid --until: %w", err)
				}
				untilPtr = &t
			}

			params := &openapi.GetApiProjectsWorkflowsParams{
				Since:  sincePtr,
				Until:  untilPtr,
			}
			if len(statuses) > 0 {
				var s []openapi.WorkflowStatus
				for _, v := range statuses {
					s = append(s, openapi.WorkflowStatus(v))
				}
				params.Status = &s
			}
			if ticketID != "" {
				params.TicketId = &ticketID
			}
			if cellID != "" {
				params.CellId = &cellID
			}
			if limit > 0 {
				params.Limit = &limit
			}
			if offset > 0 {
				params.Offset = &offset
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.GetApiProjectsWorkflowsWithResponse(ctx, app.Config.Project, params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			workflows := *resp.JSON200
			if app.Config.Output == "json" {
				return app.Printer.JSON(workflows)
			}
			headers := []string{"Run ID", "Recipe", "Status", "Cell", "Ticket", "Created", "Start", "Close"}
			rows := make([][]string, 0, len(workflows))
			for _, w := range workflows {
				rows = append(rows, []string{
					w.RunId,
					w.RecipeName,
					string(w.Status),
					coalescePtr(w.CellName),
					coalescePtr(w.TicketTitle),
					w.CreatedAt.Format(time.RFC3339),
					timePtr(w.StartTime),
					timePtr(w.CloseTime),
				})
			}
			return app.Printer.Table(headers, rows)
		},
	}

	cmd.Flags().StringSliceVar(&statuses, "status", nil, "Filter by status")
	cmd.Flags().StringVar(&ticketID, "ticket-id", "", "Filter by ticket id")
	cmd.Flags().StringVar(&cellID, "cell-id", "", "Filter by cell id")
	cmd.Flags().StringVar(&sinceStr, "since", "", "Filter created after (RFC3339)")
	cmd.Flags().StringVar(&untilStr, "until", "", "Filter created before (RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit number of results")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset for pagination")
	return cmd
}

func coalescePtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func timePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

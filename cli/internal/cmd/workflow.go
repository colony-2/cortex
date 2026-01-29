package cmd

import (
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Inspect workflows",
	}
	cmd.AddCommand(
		newWorkflowListCmd(),
		newWorkflowRunCmd(),
	)
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
				Since: sincePtr,
				Until: untilPtr,
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
			payload, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			workflows := *payload
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

func newWorkflowRunCmd() *cobra.Command {
	var recipe string
	var cellID string
	var ticketID string
	var gitRef string
	var actorEmail string
	var idempotencyKey string
	var inputs []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a workflow run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if recipe == "" || cellID == "" {
				return fmt.Errorf("recipe and cell-id are required")
			}
			req := openapi.StartWorkflowRequest{
				RecipeName: recipe,
				CellId:     cellID,
			}
			if ticketID != "" {
				req.TicketId = &ticketID
			}
			if gitRef != "" {
				req.GitRef = &gitRef
			}
			if actorEmail != "" {
				req.ActorEmail = (*openapi_types.Email)(&actorEmail)
			}
			if idempotencyKey != "" {
				req.IdempotencyKey = &idempotencyKey
			}
			if len(inputs) > 0 {
				m, err := parseKeyValue(inputs)
				if err != nil {
					return err
				}
				req.Inputs = &m
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PostApiProjectsWorkflowsWithResponse(ctx, app.Config.Project, req)
			if err != nil {
				return err
			}
			workflow, err := requirePayload(resp.JSON201, resp.HTTPResponse, resp.Body, 201)
			if err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(workflow)
			}
			return app.Printer.Text(workflow.RunId)
		},
	}

	cmd.Flags().StringVar(&recipe, "recipe", "", "Recipe name (required)")
	cmd.Flags().StringVar(&cellID, "cell-id", "", "Cell ID (required)")
	cmd.Flags().StringVar(&ticketID, "ticket-id", "", "Ticket ID to associate")
	cmd.Flags().StringVar(&gitRef, "git-ref", "", "Git ref override")
	cmd.Flags().StringVar(&actorEmail, "actor-email", "", "Actor email")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency key")
	cmd.Flags().StringSliceVar(&inputs, "input", nil, "Input key=value (repeatable)")
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

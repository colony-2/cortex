package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/cli/internal/openapi"
)

func newInputCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "input-request",
		Short: "Handle workflow input requests",
	}
	cmd.AddCommand(
		newInputListCmd(),
		newInputRespondCmd(),
	)
	return cmd
}

func newInputListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending input requests",
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
			resp, err := app.Client.GetApiProjectsProjectIdUserInputsPendingWithResponse(ctx, app.Config.Project)
			if err != nil {
				return err
			}
			payload, err := requirePayload(resp.JSON200, resp.HTTPResponse, resp.Body, 200)
			if err != nil {
				return err
			}
			pending := *payload
			if app.Config.Output == "json" {
				return app.Printer.JSON(pending)
			}
			headers := []string{"Job ID"}
			rows := make([][]string, 0, len(pending))
			for _, p := range pending {
				rows = append(rows, []string{p.Id})
			}
			return app.Printer.Table(headers, rows)
		},
	}
	return cmd
}

func newInputRespondCmd() *cobra.Command {
	var fields []string
	var responseValue string

	cmd := &cobra.Command{
		Use:   "respond <job-id>",
		Short: "Respond to an input request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}

			var req openapi.FormResponse
			if len(fields) > 0 {
				fieldMap, err := parseKeyValue(fields)
				if err != nil {
					return err
				}
				if req.Fields == nil {
					req.Fields = map[string]interface{}{}
				}
				for k, v := range fieldMap {
					req.Fields[k] = v
				}
			}
			if responseValue != "" {
				var any interface{} = responseValue
				req.Response = &any
			}
			if req.Fields == nil && req.Response == nil {
				return fmt.Errorf("provide --field and/or --response data")
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PostApiProjectsProjectIdUserInputsJobIdRespondWithResponse(ctx, app.Config.Project, args[0], req)
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
			return app.Printer.Text("ok")
		},
	}

	cmd.Flags().StringSliceVar(&fields, "field", nil, "Set field value key=value (repeatable)")
	cmd.Flags().StringVar(&responseValue, "response", "", "Set single response value")
	return cmd
}

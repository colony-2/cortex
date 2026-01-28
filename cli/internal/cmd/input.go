package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/colony-2/colony2/cli/internal/client"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
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
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			pending := *resp.JSON200
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
	var file string
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
			if file != "" {
				data, err := readData(file)
				if err != nil {
					return err
				}
				if err := unmarshalYAMLOrJSON(data, &req); err != nil {
					return fmt.Errorf("parse request: %w", err)
				}
			}
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
				return fmt.Errorf("provide --file or --field/--response data")
			}

			ctx, cancel := client.Context(cmd.Context(), app.Config.Timeout)
			defer cancel()
			resp, err := app.Client.PostApiProjectsProjectIdUserInputsJobIdRespondWithResponse(ctx, app.Config.Project, args[0], req)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response status %d", resp.StatusCode())
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(resp.JSON200)
			}
			return app.Printer.Text("ok")
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "FormResponse payload (YAML/JSON)")
	cmd.Flags().StringSliceVar(&fields, "field", nil, "Set field value key=value (repeatable)")
	cmd.Flags().StringVar(&responseValue, "response", "", "Set single response value")
	return cmd
}

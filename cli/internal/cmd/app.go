package cmd

import (
	"context"
	"fmt"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/cli/internal/output"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

type appContext struct{}

// App holds shared dependencies for commands.
type App struct {
	Config  config.Config
	Client  openapi.ClientWithResponsesInterface
	Printer *output.Printer
}

func storeApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appContext{}, app)
}

func fetchApp(ctx context.Context) (*App, error) {
	val := ctx.Value(appContext{})
	if val == nil {
		return nil, fmt.Errorf("internal error: app context missing")
	}
	app, ok := val.(*App)
	if !ok || app == nil {
		return nil, fmt.Errorf("internal error: app context invalid")
	}
	return app, nil
}

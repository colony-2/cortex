package opssetup

import (
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/api/pkg/web"
	gitexport "github.com/colony-2/colony2/server/git/pkg/export"
	opsexport "github.com/colony-2/colony2/server/ops/pkg/export"
	"github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
	ticketop "github.com/colony-2/colony2/server/ticket/pkg/op"
)

// NewDependencyContainer exposes the recipe-core builder so callers can compose dependencies fluently.
func NewDependencyContainer() *ops.ServiceDepsBuilder {
	return ops.NewServiceDepsBuilder()
}

// RegisterOps registers all known ops into the registry and returns the list.
func RegisterOps() []ops.RegisterableOp {
	ops.Clear()
	impls := opsexport.GetAll()
	impls = append(impls, workerexport.GetAll()...)
	impls = append(impls, input.GetOp())
	impls = append(impls, input.GetAutoFillOp())
	impls = append(impls, recipe.GetOps()...)
	impls = append(impls, gitexport.GetAll()...)
	impls = append(impls, ticketop.GetOp())
	ops.Register(impls...)
	return impls
}

// SetupOps initializes management services for registered ops and returns routes + cleanup.
func SetupOps(deps ops.ServiceDependencies2) (routes []web.ExtensionRoute, cleanup []func(), err error) {
	cleanupFuncs := []func(){}
	var extensionRoutes []web.ExtensionRoute

	for _, op := range RegisterOps() {
		mgmt := op.GetManagementService()
		if mgmt == nil {
			continue
		}
		if initErr := mgmt.Initialize(deps); initErr != nil {
			return nil, nil, fmt.Errorf("failed to initialize management service: %w", initErr)
		}
		cleanupFuncs = append(cleanupFuncs, func() { mgmt.Close() })
		for _, route := range mgmt.GetRoutes() {
			path := route.Path
			if strings.HasPrefix(path, "/api") {
				path = strings.TrimPrefix(path, "/api")
			}
			extensionRoutes = append(extensionRoutes, web.ExtensionRoute{
				Method:  route.Method,
				Path:    path,
				Handler: route.Handler,
			})
		}
	}

	return extensionRoutes, cleanupFuncs, nil
}

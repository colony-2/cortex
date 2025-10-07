package opssetup

import (
	"fmt"
	"strings"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	gitexport "github.com/divisive-ai/vibethis/server/git/pkg/export"
	opsexport "github.com/divisive-ai/vibethis/server/ops/pkg/export"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	workerexport "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/export"
	ticketop "github.com/divisive-ai/vibethis/server/ticket/pkg/op"
)

// NewDependencyContainer exposes the recipe-core builder so callers can compose dependencies fluently.
func NewDependencyContainer() *ops.ServiceDepsBuilder {
	return ops.NewServiceDepsBuilder()
}

// RegisterOps registers all known ops into the registry and returns the list
func RegisterOps() []ops.RegisterableOp {
	impls := opsexport.GetAll()
	impls = append(impls, workerexport.GetAll()...)
	impls = append(impls, gitexport.GetAll()...)
	impls = append(impls, ticketop.GetOp())
	ops.Register(impls...)
	return impls
}

// SetupOps initializes management services for registered ops and returns routes + cleanup
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

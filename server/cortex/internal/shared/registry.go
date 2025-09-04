package shared

import (
	"fmt"
	"strings"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	export3 "github.com/divisive-ai/vibethis/server/git/pkg/export"
	"github.com/divisive-ai/vibethis/server/ops/pkg/export"
	ops2 "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	export2 "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/export"
)

// RegisterOps registers all ops with the ops registry
func RegisterOps() []ops2.RegisterableOp {
	opImpls := export.GetAll()
	opImpls = append(opImpls, export2.GetAll()...)
	opImpls = append(opImpls, export3.GetAll()...)
	ops2.Register(opImpls...)
	return opImpls
}

// SetupOps sets up all ops and returns the management service routes
func SetupOps(context ops2.ServiceDependencies) (routes []web.ExtensionRoute, cleanupFunctions []func(), err error) {
	cleanup := []func(){}
	opImpls := RegisterOps()
	// find any management services and initialize them
	var extensionRoutes []web.ExtensionRoute
	for _, op := range opImpls {
		mgmt := op.GetManagementService()
		if mgmt != nil {
			// Initialize with empty dependencies for now (no Temporal client)
			err := mgmt.Initialize(context)
			if err != nil {
				return []web.ExtensionRoute{}, nil, fmt.Errorf("failed to initialize management service: %w", err)
			}
			cleanup = append(cleanup, func() { mgmt.Close() })

			// Convert input service routes to extension routes
			// Strip /api prefix since routes are added to the api subrouter

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
	}

	return extensionRoutes, cleanup, nil
}

package shared

import (
	"fmt"
	"strings"

	ops2 "github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/colony2/server/api/pkg/serverdeps/opregistry"
	"github.com/colony-2/colony2/server/api/pkg/web"
)

// RegisterOps registers all ops with the ops registry
func RegisterOps() []ops2.RegisterableOp {
	return opregistry.Register(opregistry.ProfileServer)
}

// SetupOps sets up all ops and returns the management service routes
func SetupOps(context ops2.ServiceDependencies2) (routes []web.ExtensionRoute, cleanupFunctions []func(), err error) {
	cleanup := []func(){}
	opImpls := RegisterOps()
	// find any management services and initialize them
	var extensionRoutes []web.ExtensionRoute
	for _, op := range opImpls {
		mgmt := op.GetManagementService()
		if mgmt != nil {
			// Initialize management service with provided dependencies
			if err := mgmt.Initialize(context); err != nil {
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

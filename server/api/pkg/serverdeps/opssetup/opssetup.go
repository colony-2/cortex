package opssetup

import (
	"fmt"
	"strings"

	"github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/colony2/server/api/pkg/serverdeps/opregistry"
	"github.com/colony-2/colony2/server/api/pkg/web"
)

// NewDependencyContainer exposes the recipe-core builder so callers can compose dependencies fluently.
func NewDependencyContainer() *ops.ServiceDepsBuilder {
	return ops.NewServiceDepsBuilder()
}

// RegisterOps registers all known ops into the registry and returns the list.
func RegisterOps() []ops.RegisterableOp {
	return opregistry.Register(opregistry.ProfileServer)
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

package opssetup

import (
    "fmt"
    "strings"

    "github.com/divisive-ai/vibethis/server/api/pkg/web"
    gitexport "github.com/divisive-ai/vibethis/server/git/pkg/export"
    opsexport "github.com/divisive-ai/vibethis/server/ops/pkg/export"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
    workerexport "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/export"
)

// ServiceDeps is a simple map-backed implementation of ops.ServiceDependencies
type ServiceDeps struct {
    items map[string]interface{}
}

// NewServiceDeps creates a new dependency container
func NewServiceDeps() *ServiceDeps {
    return &ServiceDeps{items: make(map[string]interface{})}
}

// Set adds a dependency
func (d *ServiceDeps) Set(name string, v interface{}) { d.items[name] = v }

// Get implements ops.ServiceDependencies
func (d *ServiceDeps) Get(name string) (interface{}, error) {
    v, ok := d.items[name]
    if !ok {
        return nil, fmt.Errorf("service not found: %s", name)
    }
    // Explicitly treat nil values as missing to avoid ambiguous typed-nil assertions
    if v == nil {
        return nil, fmt.Errorf("service not found: %s", name)
    }
    return v, nil
}

// RegisterOps registers all known ops into the registry and returns the list
func RegisterOps() []ops.RegisterableOp {
    impls := opsexport.GetAll()
    impls = append(impls, workerexport.GetAll()...)
    impls = append(impls, gitexport.GetAll()...)
    ops.Register(impls...)
    return impls
}

// SetupOps initializes management services for registered ops and returns routes + cleanup
// shimDeps wraps the underlying ServiceDependencies to provide sensible fallbacks
// without masking real dependencies. In particular, if a temporal client has
// been provided by the caller, pass it through. Only fall back to a typed-nil
// temporal client when the dependency is truly missing.
type shimDeps struct{ base ops.ServiceDependencies }

func (s shimDeps) Get(name string) (interface{}, error) {
    // Pass-through; do not mask missing dependencies
    return s.base.Get(name)
}

func SetupOps(deps ops.ServiceDependencies) (routes []web.ExtensionRoute, cleanup []func(), err error) {
    wrapped := shimDeps{base: deps}
    cleanupFuncs := []func(){}
    var extensionRoutes []web.ExtensionRoute

    for _, op := range RegisterOps() {
        mgmt := op.GetManagementService()
        if mgmt == nil {
            continue
        }
        if initErr := mgmt.Initialize(wrapped); initErr != nil {
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

package opssetup

import (
	"fmt"
	"strings"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	gitexport "github.com/divisive-ai/vibethis/server/git/pkg/export"
	opsexport "github.com/divisive-ai/vibethis/server/ops/pkg/export"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	workerexport "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/export"
)

// serviceDeps is a concrete ServiceDependencies2 used by the API server.
type serviceDeps struct {
	workflowCtl workflowctl.WorkflowControl
	sse         ops.SSEManager
	namespace   string
}

// NewServiceDeps constructs a typed dependency bundle for management services.
func NewServiceDeps(sse ops.SSEManager, ctl workflowctl.WorkflowControl, namespace string) ops.ServiceDependencies2 {
	return &serviceDeps{workflowCtl: ctl, sse: sse, namespace: namespace}
}

func (d *serviceDeps) WorkflowControl() (workflowctl.WorkflowControl, bool) {
	if d == nil || d.workflowCtl == nil {
		return nil, false
	}
	return d.workflowCtl, true
}

func (d *serviceDeps) SSEManager() (ops.SSEManager, bool) {
	if d == nil || d.sse == nil {
		return nil, false
	}
	return d.sse, true
}

func (d *serviceDeps) TemporalNamespace() (string, bool) {
	if d == nil || d.namespace == "" {
		return "", false
	}
	return d.namespace, true
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

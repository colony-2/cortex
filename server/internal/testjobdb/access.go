// Package testjobdb supplies trusted fixture access for embedded and HTTP test
// runtimes. Production remote JobDB access remains deployment-managed.
package testjobdb

import (
	"context"
	"net/http"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

func Context(ctx context.Context) context.Context {
	return jobdb.WithCellAuthority(ctx, jobdb.CellAuthority{Principal: "cortex-tests", TenantID: "*", Permissions: map[string]bool{
		"cells.create": true, "cells.get": true, "cells.list": true,
		"cells.update": true, "cells.retire": true, "jobs.submit": true, "jobs.list": true, "jobs.acquire": true,
	}})
}
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r.WithContext(Context(r.Context()))) })
}

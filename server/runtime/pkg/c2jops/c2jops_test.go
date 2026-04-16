package c2jops

import (
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
)

func TestOpsIncludesGHAOps(t *testing.T) {
	names := opNames(Ops())
	require.Contains(t, names, "gha.run")
	require.Contains(t, names, "gha.runs")
}

func TestRegisterInstallsGHAOps(t *testing.T) {
	original := coreops.List()
	t.Cleanup(func() {
		coreops.Replace(original...)
	})

	Register()

	names := opNames(coreops.List())
	require.Contains(t, names, "gha.run")
	require.Contains(t, names, "gha.runs")
}

func opNames(ops []coreops.RegisterableOp) map[string]struct{} {
	names := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		names[op.GetName()] = struct{}{}
	}
	return names
}

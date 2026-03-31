package opregistry

import (
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
)

func TestOpsForProfileC2JIncludesGHAOps(t *testing.T) {
	names := opNames(OpsForProfile(ProfileC2J))
	require.Contains(t, names, "gha.run")
	require.Contains(t, names, "gha.runs")
}

func TestRegisterProfileC2JInstallsGHAOps(t *testing.T) {
	original := coreops.List()
	t.Cleanup(func() {
		coreops.Replace(original...)
	})

	Register(ProfileC2J)

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

package cli

import (
	"testing"

	// Test that all required imports for devserver are available
	_ "go.temporal.io/server/common/cluster"
	_ "go.temporal.io/server/common/config"
	_ "go.temporal.io/server/common/log"
	_ "go.temporal.io/server/common/log/tag"
	_ "go.temporal.io/server/common/metrics"
	_ "go.temporal.io/server/temporal"
	_ "go.uber.org/zap"
)

func TestImportsAvailable(t *testing.T) {
	// This test just ensures all imports compile
	t.Log("All required imports are available")
}

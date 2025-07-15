package rwshim

import (
	"context"
	"strings"
	"testing"
)

// TestProcessStartWithoutMonitor tests that StartProcess fails when monitor is not running
func TestProcessStartWithoutMonitor(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	// Don't start the monitor
	
	ctx := context.Background()
	_, err := monitor.StartProcess(ctx, "echo", []string{"test"})
	if err == nil {
		t.Error("StartProcess should fail when monitor is not running")
	}
}

// TestProcessWithCustomShimPath tests that custom shim path is properly set
func TestProcessWithCustomShimPath(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	
	// Use a fake shim path (process will fail but we're testing the option)
	proc, err := monitor.StartProcess(ctx, "echo", []string{"test"}, 
		WithShimPath("/fake/path/intercept.so"))
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Verify the shim path was set
	if proc.GetShimPath() != "/fake/path/intercept.so" {
		t.Errorf("Expected shim path /fake/path/intercept.so, got %s", proc.GetShimPath())
	}
}

// TestLDPreloadHandling tests that LD_PRELOAD environment variable is properly managed
func TestLDPreloadHandling(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	
	// Test with existing LD_PRELOAD
	existingPreload := "/existing/lib.so"
	proc, err := monitor.StartProcess(ctx, "sh", []string{"-c", "echo $LD_PRELOAD"},
		WithEnv([]string{"LD_PRELOAD=" + existingPreload}),
		WithShimPath("/test/intercept.so"))
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Check that LD_PRELOAD was properly combined
	found := false
	for _, env := range proc.GetCmd().Env {
		if strings.HasPrefix(env, "LD_PRELOAD=") {
			value := env[11:]
			// Should contain both the existing and new library
			if !strings.Contains(value, existingPreload) {
				t.Error("LD_PRELOAD should contain existing library")
			}
			if !strings.Contains(value, "/test/intercept.so") {
				t.Error("LD_PRELOAD should contain shim library")
			}
			found = true
			break
		}
	}
	
	if !found {
		t.Error("LD_PRELOAD not found in environment")
	}
}
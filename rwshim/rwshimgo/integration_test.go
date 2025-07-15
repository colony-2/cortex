// +build integration

package rwshimgo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationWithCShim(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Find the test_app binary
	testAppPath := findTestApp(t)
	
	// Find the intercept library
	shimPath := findInterceptLibrary(t)

	// Test 1: Normal execution (no shim)
	t.Run("Normal execution without shim", func(t *testing.T) {
		cmd := exec.Command(testAppPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Test app failed without shim: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(string(output), "Test application completed successfully!") {
			t.Errorf("Expected success message, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 2: Shimmed execution with ALLOW_ALL policy
	t.Run("Shimmed execution with ALLOW_ALL", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Test app failed with AllowAll: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(string(output), "Test application completed successfully!") {
			t.Errorf("Expected success message, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 3: Shimmed execution with DENY_ALL policy
	t.Run("Shimmed execution with DENY_ALL", func(t *testing.T) {
		monitor := NewMonitor(DenyAll)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		// Should fail
		if err == nil {
			t.Fatalf("Expected test app to fail with DenyAll, but it succeeded\nOutput: %s", output)
		}
		if !strings.Contains(string(output), "failed") || (!strings.Contains(string(output), "Permission denied") && !strings.Contains(string(output), "Operation not permitted")) {
			t.Logf("Expected permission denied error, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 4: Shimmed execution with DENY_WRITES policy
	t.Run("Shimmed execution with DENY_WRITES", func(t *testing.T) {
		monitor := NewMonitor(DenyWrites)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		// Should fail because it tries to write
		if err == nil {
			t.Fatalf("Expected test app to fail with DenyWrites, but it succeeded\nOutput: %s", output)
		}
		if !strings.Contains(string(output), "Write") && !strings.Contains(string(output), "failed") {
			t.Logf("Expected write failure, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 5: Shimmed execution with DENY_READS policy
	t.Run("Shimmed execution with DENY_READS", func(t *testing.T) {
		monitor := NewMonitor(DenyReads)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		// Create the test file first
		if err := os.WriteFile("test_output.txt", []byte("Test content for read test\n"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		// Should fail when trying to read
		if err == nil {
			t.Fatalf("Expected test app to fail with DenyReads, but it succeeded\nOutput: %s", output)
		}
		if !strings.Contains(string(output), "Read") && !strings.Contains(string(output), "failed") {
			t.Logf("Expected read failure, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 6: No monitor running (should allow by default)
	t.Run("No monitor running", func(t *testing.T) {
		// Make sure no monitor is running
		time.Sleep(100 * time.Millisecond)
		
		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Test app failed with no monitor: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(string(output), "Test application completed successfully!") {
			t.Errorf("Expected success message, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 7: Using StartProcess API
	t.Run("Using StartProcess API", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		proc, err := monitor.StartProcess(ctx, testAppPath, nil, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		// Capture output
		var stdout, stderr bytes.Buffer
		proc.cmd.Stdout = &stdout
		proc.cmd.Stderr = &stderr

		if err := proc.Start(); err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		if err := proc.Wait(); err != nil {
			t.Fatalf("Process failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
		}

		output := stdout.String() + stderr.String()
		if !strings.Contains(output, "Test application completed successfully!") {
			t.Errorf("Expected success message, got: %s", output)
		}
		os.Remove("test_output.txt")
	})

	// Test 8: Complex policy with logging
	t.Run("Complex policy with logging", func(t *testing.T) {
		var operations []string
		policy := func(req Request) Response {
			operations = append(operations, fmt.Sprintf("%s %s", req.Operation, req.Filename))
			// Allow stdout/stderr, deny file writes
			if req.Filename == "stdout" || req.Filename == "stderr" {
				return Response{Allow: true}
			}
			if req.Operation == OpWrite && strings.Contains(req.Filename, "test_output.txt") {
				return Response{Allow: false}
			}
			return Response{Allow: true}
		}

		monitor := NewMonitor(policy)
		if err := monitor.Start(); err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		cmd := exec.Command(testAppPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("LD_PRELOAD=%s", shimPath))
		output, err := cmd.CombinedOutput()
		
		// Should fail when trying to write to file
		if err == nil {
			t.Fatalf("Expected test app to fail with custom policy, but it succeeded\nOutput: %s", output)
		}

		// Check that we logged operations
		if len(operations) == 0 {
			t.Error("Expected to log some operations")
		}
		
		// Should have attempted to write to stdout or pipe (when output is redirected)
		found := false
		for _, op := range operations {
			if strings.Contains(op, "WRITE stdout") || strings.Contains(op, "WRITE pipe:") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to see stdout/pipe write operation, got: %v", operations)
		}

		os.Remove("test_output.txt")
	})
}

// findTestApp locates the test_app binary
func findTestApp(t *testing.T) string {
	searchPaths := []string{
		"./test_app",
		"../clib/test_app",
		"./clib/test_app",
	}

	// Also check relative to test binary location
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		searchPaths = append(searchPaths,
			filepath.Join(exeDir, "test_app"),
			filepath.Join(exeDir, "..", "clib", "test_app"),
			filepath.Join(exeDir, "..", "..", "clib", "test_app"),
		)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	t.Fatal("test_app binary not found. Please build it first with: cd ../clib && gcc -o test_app test_app.c")
	return ""
}

// findInterceptLibrary locates the intercept.so library
func findInterceptLibrary(t *testing.T) string {
	searchPaths := []string{
		"./intercept.so",
		"../clib/intercept.so",
		"./clib/intercept.so",
	}

	// Also check relative to test binary location
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		searchPaths = append(searchPaths,
			filepath.Join(exeDir, "intercept.so"),
			filepath.Join(exeDir, "..", "clib", "intercept.so"),
			filepath.Join(exeDir, "..", "..", "clib", "intercept.so"),
		)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	t.Fatal("intercept.so library not found. Please build it first with: cd ../clib && gcc -shared -fPIC -o intercept.so intercept.c -ldl")
	return ""
}
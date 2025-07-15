// +build integration

package rwshim

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
		proc.GetCmd().Stdout = &stdout
		proc.GetCmd().Stderr = &stderr

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

// TestProcessIntegration contains process-specific integration tests
func TestProcessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Find the intercept library
	shimPath := findInterceptLibrary(t)

	t.Run("ProcessCreation", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		
		// Test creating a simple process
		proc, err := monitor.StartProcess(ctx, "echo", []string{"test"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		// Check that process hasn't started yet
		if proc.Pid() != -1 {
			t.Error("Process should not have a PID before Start()")
		}
	})

	t.Run("ProcessExecution", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		// Capture output
		var stdout bytes.Buffer
		
		ctx := context.Background()
		proc, err := monitor.StartProcess(ctx, "echo", []string{"hello", "world"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		// Redirect stdout to our buffer
		r, w, _ := os.Pipe()
		proc.GetCmd().Stdout = w
		
		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		// Check PID
		if proc.Pid() <= 0 {
			t.Error("Started process should have a valid PID")
		}

		// Use a waitgroup to ensure output is read
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Read output
			buf := make([]byte, 1024)
			n, _ := r.Read(buf)
			stdout.Write(buf[:n])
			r.Close()
		}()

		err = proc.Wait()
		w.Close()
		wg.Wait() // Wait for output to be read

		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Check exit code
		if proc.ExitCode() != 0 {
			t.Errorf("Expected exit code 0, got %d", proc.ExitCode())
		}

		// Check output
		output := strings.TrimSpace(stdout.String())
		if output != "hello world" {
			t.Errorf("Expected 'hello world', got '%s'", output)
		}
	})

	t.Run("ProcessOptions", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		
		// Test with multiple options
		tempDir := t.TempDir()
		envVar := "TEST_VAR=test_value"
		
		proc, err := monitor.StartProcess(ctx, "sh", []string{"-c", "pwd && echo $TEST_VAR"},
			WithShimPath(shimPath),
			WithDir(tempDir),
			WithEnv([]string{envVar}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		// Capture output
		var stdout bytes.Buffer
		r, w, _ := os.Pipe()
		proc.GetCmd().Stdout = w

		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		err = proc.Wait()
		w.Close()

		// Read output
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		stdout.Write(buf[:n])
		r.Close()

		output := strings.TrimSpace(stdout.String())
		lines := strings.Split(output, "\n")
		
		// Check working directory (handle symlink resolution on macOS)
		if len(lines) > 0 {
			resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
			resolvedOutput, _ := filepath.EvalSymlinks(lines[0])
			if resolvedOutput != resolvedTempDir {
				t.Errorf("Expected working directory %s, got %s", resolvedTempDir, resolvedOutput)
			}
		}
		
		// Check environment variable
		if len(lines) > 1 && lines[1] != "test_value" {
			t.Errorf("Expected env var value 'test_value', got %s", lines[1])
		}
	})

	t.Run("ProcessWaitTimeout", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		
		// Create a long-running process
		proc, err := monitor.StartProcess(ctx, "sleep", []string{"5"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		// Wait with short timeout
		err = proc.WaitWithTimeout(100 * time.Millisecond)
		if err == nil {
			t.Error("WaitWithTimeout should timeout")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error, got: %v", err)
		}

		// Kill the process to clean up
		proc.Kill()
	})

	t.Run("ProcessKill", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		
		// Create a long-running process
		proc, err := monitor.StartProcess(ctx, "sleep", []string{"10"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		// Kill the process
		err = proc.Kill()
		if err != nil {
			t.Fatalf("Failed to kill process: %v", err)
		}

		// Wait should complete quickly
		done := make(chan error)
		go func() {
			done <- proc.Wait()
		}()

		select {
		case <-done:
			// Process terminated as expected
		case <-time.After(1 * time.Second):
			t.Error("Process did not terminate after Kill()")
		}
	})

	t.Run("ProcessDoubleStart", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		proc, err := monitor.StartProcess(ctx, "echo", []string{"test"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		// Try to start again
		err = proc.Start()
		if err == nil {
			t.Error("Starting an already started process should return an error")
		}

		proc.Wait()
	})

	t.Run("ProcessWithContext", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		// Create a context that we'll cancel
		ctx, cancel := context.WithCancel(context.Background())
		
		// Create a long-running process
		proc, err := monitor.StartProcess(ctx, "sleep", []string{"10"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		err = proc.Start()
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}

		// Cancel the context
		cancel()

		// Process should terminate
		done := make(chan error)
		go func() {
			done <- proc.Wait()
		}()

		select {
		case <-done:
			// Process terminated as expected
		case <-time.After(1 * time.Second):
			t.Error("Process did not terminate after context cancellation")
			proc.Kill()
		}
	})

	t.Run("ProcessNotStartedErrors", func(t *testing.T) {
		monitor := NewMonitor(AllowAll)
		err := monitor.Start()
		if err != nil {
			t.Fatalf("Failed to start monitor: %v", err)
		}
		defer monitor.Stop()

		ctx := context.Background()
		proc, err := monitor.StartProcess(ctx, "echo", []string{"test"}, WithShimPath(shimPath))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}

		// Test operations on non-started process
		err = proc.Wait()
		if err == nil || !strings.Contains(err.Error(), "not started") {
			t.Error("Wait on non-started process should return 'not started' error")
		}

		err = proc.WaitWithTimeout(100 * time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "not started") {
			t.Error("WaitWithTimeout on non-started process should return 'not started' error")
		}

		err = proc.Kill()
		if err == nil || !strings.Contains(err.Error(), "not started") {
			t.Error("Kill on non-started process should return 'not started' error")
		}

		if proc.Pid() != -1 {
			t.Error("Pid of non-started process should be -1")
		}

		if proc.ExitCode() != -1 {
			t.Error("ExitCode of non-started process should be -1")
		}
	})
}

// findTestApp locates the test_app binary
func findTestApp(t *testing.T) string {
	searchPaths := []string{
		"./test_app",
		"../clib/test_app",
		"./clib/test_app",
		"../../clib/test_app",
		"../../../clib/test_app",
	}

	// Also check relative to test binary location
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		searchPaths = append(searchPaths,
			filepath.Join(exeDir, "test_app"),
			filepath.Join(exeDir, "..", "clib", "test_app"),
			filepath.Join(exeDir, "..", "..", "clib", "test_app"),
			filepath.Join(exeDir, "..", "..", "..", "clib", "test_app"),
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
		"../../clib/intercept.so",
		"../../../clib/intercept.so",
	}

	// Also check relative to test binary location
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		searchPaths = append(searchPaths,
			filepath.Join(exeDir, "intercept.so"),
			filepath.Join(exeDir, "..", "clib", "intercept.so"),
			filepath.Join(exeDir, "..", "..", "clib", "intercept.so"),
			filepath.Join(exeDir, "..", "..", "..", "clib", "intercept.so"),
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
package rwshimgo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessCreation(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	
	// Test creating a simple process
	proc, err := monitor.StartProcess(ctx, "echo", []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Check that process hasn't started yet
	if proc.Pid() != -1 {
		t.Error("Process should not have a PID before Start()")
	}
}

func TestProcessStartWithoutMonitor(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	// Don't start the monitor
	
	ctx := context.Background()
	_, err := monitor.StartProcess(ctx, "echo", []string{"test"})
	if err == nil {
		t.Error("StartProcess should fail when monitor is not running")
	}
}

func TestProcessExecution(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Capture output
	var stdout bytes.Buffer
	
	ctx := context.Background()
	proc, err := monitor.StartProcess(ctx, "echo", []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Redirect stdout to our buffer
	r, w, _ := os.Pipe()
	proc.cmd.Stdout = w
	
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
}

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
	if proc.shimPath != "/fake/path/intercept.so" {
		t.Errorf("Expected shim path /fake/path/intercept.so, got %s", proc.shimPath)
	}
}

func TestProcessOptions(t *testing.T) {
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
		WithDir(tempDir),
		WithEnv([]string{envVar}))
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Capture output
	var stdout bytes.Buffer
	r, w, _ := os.Pipe()
	proc.cmd.Stdout = w

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
}

func TestProcessWaitTimeout(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	
	// Create a long-running process
	proc, err := monitor.StartProcess(ctx, "sleep", []string{"5"})
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
}

func TestProcessKill(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	
	// Create a long-running process
	proc, err := monitor.StartProcess(ctx, "sleep", []string{"10"})
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
}

func TestProcessDoubleStart(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	proc, err := monitor.StartProcess(ctx, "echo", []string{"test"})
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
}

func TestProcessWithContext(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create a long-running process
	proc, err := monitor.StartProcess(ctx, "sleep", []string{"10"})
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
}

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
	for _, env := range proc.cmd.Env {
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

func TestFindShimLibrary(t *testing.T) {
	// This test might fail if the shim library isn't built
	// We'll just test that the function doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("findShimLibrary panicked: %v", r)
		}
	}()

	path, err := findShimLibrary()
	if err == nil {
		// If found, it should be an absolute path
		if !filepath.IsAbs(path) {
			t.Errorf("Expected absolute path, got %s", path)
		}
		// Should end with .so or .dylib
		if !strings.HasSuffix(path, ".so") && !strings.HasSuffix(path, ".dylib") {
			t.Errorf("Expected .so or .dylib file, got %s", path)
		}
	}
	// It's okay if the library isn't found in test environment
}

func TestProcessNotStartedErrors(t *testing.T) {
	monitor := NewMonitor(AllowAll)
	err := monitor.Start()
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	ctx := context.Background()
	proc, err := monitor.StartProcess(ctx, "echo", []string{"test"})
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
}
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colony-2/colony2/rwshim/rwshimgo/pkg/rwshim"
)

func main() {
	// Example 1: Simple allow-all policy
	simpleExample()

	// Example 2: Custom policy with logging
	customPolicyExample()

	// Example 3: Using PolicyBuilder
	policyBuilderExample()

	// Example 4: Running multiple processes
	multiProcessExample()
}

func simpleExample() {
	fmt.Println("=== Example 1: Simple Allow-All Policy ===")

	// Create monitor with allow-all policy
	monitor := rwshim.NewMonitor(rwshim.AllowAll)

	// Start the monitor
	if err := monitor.Start(); err != nil {
		log.Fatal("Failed to start monitor:", err)
	}
	defer monitor.Stop()

	// Start a process
	ctx := context.Background()
	proc, err := monitor.StartProcess(ctx, "echo", []string{"Hello, World!"})
	if err != nil {
		log.Fatal("Failed to create process:", err)
	}

	// Start and wait for the process
	if err := proc.Start(); err != nil {
		log.Fatal("Failed to start process:", err)
	}

	if err := proc.Wait(); err != nil {
		log.Fatal("Process failed:", err)
	}

	fmt.Println("Process completed successfully")
}

func customPolicyExample() {
	fmt.Println("=== Example 2: Custom Policy with Logging ===")

	// Create a custom policy that logs all operations
	customPolicy := func(req rwshim.Request) rwshim.Response {
		fmt.Printf("Intercepted: %s fd=%d size=%d file=%s\n",
			req.Operation, req.FD, req.Size, req.Filename)

		// Deny writes to /etc
		if req.Operation == rwshim.OpWrite && len(req.Filename) >= 4 && req.Filename[:4] == "/etc" {
			fmt.Printf("  -> DENIED (write to /etc)\n")
			return rwshim.Response{Allow: false}
		}

		fmt.Printf("  -> ALLOWED\n")
		return rwshim.Response{Allow: true}
	}

	// Create and start monitor
	monitor := rwshim.NewMonitor(customPolicy)
	if err := monitor.Start(); err != nil {
		log.Fatal("Failed to start monitor:", err)
	}
	defer monitor.Stop()

	// Run a command that reads and writes
	ctx := context.Background()
	proc, err := monitor.StartProcess(ctx, "ls", []string{"-la", "/tmp"})
	if err != nil {
		log.Fatal("Failed to create process:", err)
	}

	if err := proc.Start(); err != nil {
		log.Fatal("Failed to start process:", err)
	}

	if err := proc.Wait(); err != nil {
		log.Fatal("Process failed:", err)
	}

	fmt.Println()
}

func policyBuilderExample() {
	fmt.Println("=== Example 3: Using PolicyBuilder ===")

	// Build a complex policy
	policy := rwshim.NewPolicyBuilder().
		// Allow all reads from standard streams
		AllowRead(rwshim.MatchStdStreams()).
		// Allow all writes to standard streams
		AllowWrite(rwshim.MatchStdStreams()).
		// Deny writes to .log files
		DenyWrite(rwshim.MatchFilenameSuffix(".log")).
		// Allow reads from /tmp
		AllowRead(rwshim.MatchFilenamePrefix("/tmp/")).
		// Deny large writes (>1MB)
		DenyWrite(rwshim.MatchLargeOperations(1024 * 1024)).
		// Default: allow everything else
		Default(true).
		Build()

	// Create and start monitor
	monitor := rwshim.NewMonitor(policy)
	if err := monitor.Start(); err != nil {
		log.Fatal("Failed to start monitor:", err)
	}
	defer monitor.Stop()

	// Test the policy
	ctx := context.Background()

	// This should work (writes to stdout)
	proc1, _ := monitor.StartProcess(ctx, "echo", []string{"This works"})
	proc1.Start()
	proc1.Wait()

	// This would be denied if it tried to write to a .log file
	proc2, _ := monitor.StartProcess(ctx, "ls", []string{"/tmp"})
	proc2.Start()
	proc2.Wait()

	fmt.Println()
}

func multiProcessExample() {
	fmt.Println("=== Example 4: Running Multiple Processes ===")

	// Create monitor with a policy that counts operations
	var readCount, writeCount int
	countingPolicy := func(req rwshim.Request) rwshim.Response {
		switch req.Operation {
		case rwshim.OpRead:
			readCount++
		case rwshim.OpWrite:
			writeCount++
		}
		return rwshim.Response{Allow: true}
	}

	monitor := rwshim.NewMonitor(countingPolicy)
	if err := monitor.Start(); err != nil {
		log.Fatal("Failed to start monitor:", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	// Start multiple processes
	fmt.Println("Starting multiple processes...")

	// Process 1: continuous output
	proc1, _ := monitor.StartProcess(ctx, "bash", []string{"-c", "while true; do echo 'Process 1'; sleep 1; done"})
	proc1.Start()

	// Process 2: file operations
	proc2, _ := monitor.StartProcess(ctx, "bash", []string{"-c", "for i in {1..5}; do echo \"Line $i\" > /tmp/test$i.txt; done"})
	proc2.Start()

	// Wait for a bit or until interrupted
	select {
	case <-time.After(3 * time.Second):
		fmt.Println("\nTimeout reached")
	case <-sigChan:
		fmt.Println("\nInterrupted")
	}

	// Cancel context to stop process 1
	cancel()

	// Wait for processes to finish
	proc1.Kill()
	proc2.Wait()

	// Print statistics
	fmt.Printf("\nOperation statistics:\n")
	fmt.Printf("  Total reads:  %d\n", readCount)
	fmt.Printf("  Total writes: %d\n", writeCount)

	// Clean shutdown
	monitor.Stop()
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
	"github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

// MultiStringFlag allows multiple -rw flags
type MultiStringFlag []string

func (m *MultiStringFlag) String() string {
	return fmt.Sprint(*m)
}

func (m *MultiStringFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func main() {
	var rwPaths MultiStringFlag
	var containerName string
	var verbose bool
	var noCache bool
	
	flag.Var(&rwPaths, "rw", "Read-write directory (can be specified multiple times)")
	flag.StringVar(&containerName, "name", "", "Container name")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&noCache, "no-cache", false, "Force rebuild")
	flag.Parse()
	
	if len(rwPaths) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one -rw path required\n")
		fmt.Fprintf(os.Stderr, "Usage: shai -rw <path1> [-rw <path2> ...] [flags]\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	
	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	// Create devcontainer manager (cmd can import from internal)
	manager, err := devcontainer.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating container manager: %v\n", err)
		os.Exit(1)
	}
	
	// Create shai runner with the manager
	runner, err := shai.New(shai.Config{
		WorkingDir:     workingDir,
		ReadWritePaths: rwPaths,
		ContainerName:  containerName,
		NoCache:        noCache,
	}, manager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer runner.Close()
	
	// Set up progress display
	setupProgressDisplay(runner, verbose)
	
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	
	// Start container
	container, err := runner.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
	
	// Clear line for clean shell prompt
	fmt.Println()
	
	// Attach interactive shell
	err = runner.AttachInteractive(ctx, container.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

func setupProgressDisplay(runner *shai.Runner, verbose bool) {
	var lastPhase shai.Phase
	var phaseStart time.Time
	
	runner.OnProgress(func(phase shai.Phase, message string) {
		// Track phase changes
		if phase != lastPhase {
			// Complete previous phase if it was a long-running one
			if lastPhase == shai.PhasePulling || 
			   lastPhase == shai.PhaseBuilding || 
			   lastPhase == shai.PhaseInstalling {
				elapsed := time.Since(phaseStart)
				if elapsed > 2*time.Second {
					fmt.Printf(" (%ds)\n", int(elapsed.Seconds()))
				} else {
					fmt.Println()
				}
			}
			
			lastPhase = phase
			phaseStart = time.Now()
		}
		
		// Display progress based on phase
		switch phase {
		case shai.PhaseValidating, shai.PhaseCreating, shai.PhaseStarting:
			// Quick operations - show with checkmark
			fmt.Printf("✓ %s\n", message)
			
		case shai.PhasePulling, shai.PhaseBuilding, shai.PhaseInstalling:
			// Long operations - show as ongoing
			// Use carriage return to update the same line
			fmt.Printf("\r⟳ %s...", message)
			
		default:
			if verbose {
				fmt.Printf("  %s\n", message)
			}
		}
	})
}

// Simple spinner characters for terminal animation
var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func showSpinner(message string, done chan bool) {
	i := 0
	for {
		select {
		case <-done:
			// Clear the spinner line
			fmt.Printf("\r%s\r", strings.Repeat(" ", len(message)+5))
			return
		default:
			fmt.Printf("\r%s %s", spinnerChars[i%len(spinnerChars)], message)
			time.Sleep(100 * time.Millisecond)
			i++
		}
	}
}
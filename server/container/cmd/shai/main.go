package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
    var ephemeral bool
    var hideProgress bool
    var printScript bool
	
	flag.Var(&rwPaths, "rw", "Read-write directory (can be specified multiple times)")
	flag.StringVar(&containerName, "name", "", "Container name")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&noCache, "no-cache", false, "Force rebuild")
    flag.BoolVar(&ephemeral, "ephemeral", true, "Run in ephemeral mode (container removed on exit)")
    flag.BoolVar(&hideProgress, "hide-progress", false, "Hide progress markers in ephemeral mode")
    flag.BoolVar(&printScript, "print-script", false, "Print the generated setup script in ephemeral mode")
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
	
    // Set up signal handling for graceful shutdown
    ctx, cancel := setupSignals(ephemeral)
    defer cancel()
	
	// Run in ephemeral mode if requested
    if ephemeral {
        runEphemeral(ctx, workingDir, rwPaths, noCache, hideProgress, verbose, printScript)
    } else {
        runPersistent(ctx, workingDir, rwPaths, containerName, noCache, verbose)
    }
}

func runEphemeral(ctx context.Context, workingDir string, rwPaths []string, noCache, hideProgress, verbose, printScript bool) {
	// Create ephemeral runner
    runner, err := shai.NewEphemeralRunner(shai.EphemeralConfig{
        WorkingDir:          workingDir,
        ReadWritePaths:      rwPaths,
        NoCache:             noCache,
        HideProgressMarkers: hideProgress,
        DebugScript:         printScript || verbose,
    })
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer runner.Close()
	
	// Set up progress display for ephemeral mode
	setupEphemeralProgressDisplay(runner, verbose)
	
	// Run the ephemeral container
	if err := runner.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

// setupSignals configures signal handling and returns a cancellable context.
// In ephemeral mode, SIGINT is ignored so Ctrl-C reaches the container shell.
func setupSignals(ephemeral bool) (context.Context, context.CancelFunc) {
    ctx, cancel := context.WithCancel(context.Background())
    sigCh := make(chan os.Signal, 1)
    if ephemeral {
        signal.Ignore(syscall.SIGINT)
        signal.Notify(sigCh, syscall.SIGTERM)
    } else {
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    }
    go func() {
        <-sigCh
        cancel()
    }()
    return ctx, cancel
}

func runPersistent(ctx context.Context, workingDir string, rwPaths []string, containerName string, noCache, verbose bool) {
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

func setupEphemeralProgressDisplay(runner *shai.EphemeralRunner, verbose bool) {
    // Simpler UI: accumulate completed items as checkmarks, show a single spinner line for the current item.
    var completed []string
    current := ""
    spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
    spinnerIdx := 0
    var spinTicker *time.Ticker
    renderSpinner := func() {
        if current == "" { return }
        fmt.Printf("\r\033[K%s %s", spinner[spinnerIdx%len(spinner)], current)
    }
    startSpinner := func() {
        if spinTicker != nil { return }
        spinTicker = time.NewTicker(120 * time.Millisecond)
        go func() {
            for range spinTicker.C {
                spinnerIdx = (spinnerIdx + 1) % len(spinner)
                renderSpinner()
            }
        }()
    }
    // print a completed item as a persistent checkmark line
    finishCurrent := func() {
        if current == "" { return }
        if spinTicker != nil { spinTicker.Stop(); spinTicker = nil }
        // Clear spinner line and print checkmark line
        fmt.Printf("\r\033[K✓ %s\n", current)
        completed = append(completed, current)
        current = ""
    }

    runner.OnProgress(func(update shai.ProgressUpdate) {
        switch update.Phase {
        case "INIT":
            if update.Status == "START" {
                current = "Initialize devcontainer setup"
                startSpinner()
                renderSpinner()
            }
        case "FEATURES":
            switch update.Status {
            case "START":
                if current == "Initialize devcontainer setup" {
                    finishCurrent()
                }
            case "PROGRESS":
                finishCurrent()
                current = update.Message
                startSpinner()
                renderSpinner()
            case "COMPLETE":
                finishCurrent()
            }
        case "POSTCREATE":
            if update.Status == "START" {
                current = "PostCreate"
                startSpinner()
                renderSpinner()
            } else if update.Status == "COMPLETE" {
                finishCurrent()
            }
        case "USERSWITCH":
            if update.Status == "START" {
                finishCurrent()
                // Completed items already printed above; do not print again
            }
        }
    })
}

// formatEphemeralProgress writes a clean, line-based progress message to w without using carriage returns.
func formatEphemeralProgress(w io.Writer, update shai.ProgressUpdate, verbose bool) {
	switch update.Status {
	case "START":
		switch update.Phase {
		case "FEATURES":
			fmt.Fprintf(w, "🔧 %s\n", update.Message)
		case "ONCREATE":
			fmt.Fprintf(w, "📦 %s\n", update.Message)
		case "UPDATECONTENT":
			fmt.Fprintf(w, "🔄 %s\n", update.Message)
		case "POSTCREATE":
			fmt.Fprintf(w, "🔨 %s\n", update.Message)
		case "POSTSTART":
			fmt.Fprintf(w, "🚀 %s\n", update.Message)
		case "POSTATTACH":
			fmt.Fprintf(w, "📎 %s\n", update.Message)
		case "USERSWITCH":
			fmt.Fprintf(w, "👤 %s\n", update.Message)
		case "INIT":
			fmt.Fprintf(w, "⚙️  %s\n", update.Message)
		default:
			fmt.Fprintf(w, "⚙️  %s\n", update.Message)
		}
	case "COMPLETE":
		fmt.Fprintf(w, "✅ %s\n", update.Message)
	case "ERROR":
		fmt.Fprintf(w, "❌ %s\n", update.Message)
	case "PROGRESS":
		if update.Phase == "FEATURES" {
			fmt.Fprintf(w, "  - %s\n", update.Message)
		} else if verbose {
			fmt.Fprintf(w, "  • %s\n", update.Message)
		}
	}
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

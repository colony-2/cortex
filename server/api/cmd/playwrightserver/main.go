package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	directtestsupport "github.com/colony-2/swf-go/pkg/swf/runtime/direct/testsupport"
	"github.com/spf13/pflag"
)

func main() {
	var port int
	var storagePath string
	var corsOrigins []string

	flags := pflag.NewFlagSet("playwrightserver", pflag.ExitOnError)
	flags.IntVarP(&port, "port", "p", 8080, "Port for the API test server")
	flags.StringVar(&storagePath, "storage", "", "Storage directory for the temporary server state")
	flags.StringSliceVar(&corsOrigins, "cors-origin", []string{"http://localhost:5173"}, "Allowed CORS origins")
	flags.Parse(os.Args[1:])

	if storagePath == "" {
		tmpDir, err := os.MkdirTemp("", "api-playwright-storage-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: create temp storage: %v\n", err)
			os.Exit(1)
		}
		storagePath = tmpDir
		defer os.RemoveAll(storagePath)
	} else {
		absPath, err := filepath.Abs(storagePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: resolve storage path: %v\n", err)
			os.Exit(1)
		}
		storagePath = absPath
	}

	dsn, stopPG, err := directtestsupport.StartEmbeddedPostgres()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: start embedded postgres: %v\n", err)
		os.Exit(1)
	}
	defer stopPG()

	args := []string{
		"run",
		"./cmd/testserver",
		"--new",
		"--initializedb",
		"--port", strconv.Itoa(port),
		"--storage", storagePath,
	}
	for _, origin := range corsOrigins {
		args = append(args, "--cors-origins", origin)
	}

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "NEON_C2_DEV_DSN="+dsn)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: start api test server: %v\n", err)
		os.Exit(1)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case sig := <-signals:
		if cmd.Process != nil {
			_ = cmd.Process.Signal(sig)
		}
		if err := <-waitCh; err != nil {
			exitWithErr(err)
		}
	case err := <-waitCh:
		if err != nil {
			exitWithErr(err)
		}
	}
}

func exitWithErr(err error) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

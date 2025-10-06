package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/divisive-ai/vibethis/server/nucleus/internal/client"
	"github.com/divisive-ai/vibethis/server/nucleus/internal/config"
	recipeworker "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	ErrClientCreate = fmt.Errorf("failed to create Temporal client")
	ErrWorkerCreate = fmt.Errorf("failed to create worker")
	ErrWorkerStart  = fmt.Errorf("failed to start worker")
)

var rootCmd = &cobra.Command{
	Use:   "recipe-watcher",
	Short: "Run recipe-worker with Temporal integration",
	Long: `Recipe Watcher is a CLI tool that provides a managed way to run the recipe-worker
package with configurable options. It monitors recipe files and dynamically executes
workflows through Temporal.`,
	RunE: run,
}

func init() {
	rootCmd.PersistentFlags().String("name", "", "Worker name/identifier (required)")
	rootCmd.PersistentFlags().StringP("temporal-server", "t", "localhost:7233", "Temporal server address")
	rootCmd.PersistentFlags().StringP("recipes-path", "r", "", "Path to recipes directory (required)")
	rootCmd.PersistentFlags().String("namespace", "default", "Temporal namespace")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging")

	rootCmd.MarkPersistentFlagRequired("name")
	rootCmd.MarkPersistentFlagRequired("recipes-path")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := parseConfig(cmd)
	if err != nil {
		return err
	}

	logger := setupLogger(cfg.Debug)
	defer logger.Sync()

	logger.Info("Starting recipe-watcher",
		zap.String("name", cfg.Name),
		zap.String("recipes_path", cfg.RecipesPath),
		zap.String("temporal_server", cfg.TemporalServer),
		zap.String("namespace", cfg.Namespace),
		zap.Bool("debug", cfg.Debug),
	)

	temporalClient, err := client.NewTemporalClient(cfg)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrClientCreate, err)
	}
	defer temporalClient.Close()

	worker, err := recipeworker.NewWorker(logger, cfg.RecipesPath, temporalClient, cfg.Namespace)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWorkerCreate, err)
	}

	if err := worker.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrWorkerStart, err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutdown signal received")

	if err := worker.Stop(); err != nil {
		logger.Error("Failed to stop worker gracefully", zap.Error(err))
		return err
	}

	logger.Info("Worker stopped successfully")
	return nil
}

func parseConfig(cmd *cobra.Command) (*config.Config, error) {
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, fmt.Errorf("failed to get name flag: %w", err)
	}

	temporalServer, err := cmd.Flags().GetString("temporal-server")
	if err != nil {
		return nil, fmt.Errorf("failed to get temporal-server flag: %w", err)
	}

	recipesPath, err := cmd.Flags().GetString("recipes-path")
	if err != nil {
		return nil, fmt.Errorf("failed to get recipes-path flag: %w", err)
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace flag: %w", err)
	}

	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return nil, fmt.Errorf("failed to get debug flag: %w", err)
	}

	cfg := &config.Config{
		Name:           name,
		TemporalServer: temporalServer,
		RecipesPath:    recipesPath,
		Namespace:      namespace,
		Debug:          debug,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

func setupLogger(debug bool) *zap.Logger {
	var logger *zap.Logger
	var err error

	if debug {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	return logger
}

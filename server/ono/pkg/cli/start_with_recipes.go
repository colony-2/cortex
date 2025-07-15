package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vibethis/embeddedtemporal"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"vibethis/ono/internal/recipe"
)

// StartWithRecipesCmd represents the start command with recipe support
var StartWithRecipesCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Temporal dev server with automatic recipe worker management",
	Long: `Starts a local Temporal development server with automatic recipe discovery
and worker management. 

This command combines the Temporal dev server with the recipe system, automatically
starting workers for all discovered recipes and managing their lifecycle.`,
	RunE: runStartWithRecipes,
}

var (
	recipeDir string
	enableRecipes bool
)

func init() {
	// Inherit flags from the original start command
	StartWithRecipesCmd.Flags().StringP("ip", "i", "127.0.0.1", "IP address to bind the frontend service to")
	StartWithRecipesCmd.Flags().IntP("port", "p", 7233, "Port for the frontend gRPC service")
	StartWithRecipesCmd.Flags().IntP("ui-port", "", 8080, "Port for the Web UI")
	StartWithRecipesCmd.Flags().StringP("namespace", "n", "default", "Default namespace to create")
	StartWithRecipesCmd.Flags().StringP("db-filename", "f", "", "SQLite database filename (default: temporary file)")
	StartWithRecipesCmd.Flags().StringP("log-level", "l", "info", "Log level (debug, info, warn, error)")
	StartWithRecipesCmd.Flags().StringSliceP("sqlite-pragma", "", []string{}, "SQLite pragma statements")
	
	// Recipe-specific flags
	StartWithRecipesCmd.Flags().StringVar(&recipeDir, "recipe-dir", "", "Recipe directory (default: ~/.ono/recipes)")
	StartWithRecipesCmd.Flags().BoolVar(&enableRecipes, "enable-recipes", true, "Enable automatic recipe discovery and worker management")
}

func runStartWithRecipes(cmd *cobra.Command, args []string) error {
	// Get flags
	ip, _ := cmd.Flags().GetString("ip")
	port, _ := cmd.Flags().GetInt("port")
	uiPort, _ := cmd.Flags().GetInt("ui-port")
	namespace, _ := cmd.Flags().GetString("namespace")
	dbPath, _ := cmd.Flags().GetString("db-filename")
	logLevel, _ := cmd.Flags().GetString("log-level")
	sqlitePragmas, _ := cmd.Flags().GetStringSlice("sqlite-pragma")

	// Convert pragma slice to map
	pragmaMap := make(map[string]string)
	for _, pragma := range sqlitePragmas {
		// Simple parsing - in production you'd want more robust parsing
		parts := strings.SplitN(pragma, "=", 2)
		if len(parts) == 2 {
			pragmaMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Create dev server options
	opts := embeddedtemporal.Options{
		FrontendIP:    ip,
		FrontendPort:  port,
		UIPort:        uiPort,
		Namespaces:    []string{namespace},
		DatabaseFile:  dbPath,
		LogLevel:      logLevel,
		SQLitePragmas: pragmaMap,
		EnableUI:      true,
	}

	// Ensure absolute path for database file
	if opts.DatabaseFile != "" {
		absPath, err := filepath.Abs(opts.DatabaseFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		opts.DatabaseFile = absPath
	}

	fmt.Printf("Starting Temporal development server with recipe support...\n")
	fmt.Printf("Frontend address: %s:%d\n", opts.FrontendIP, opts.FrontendPort)
	fmt.Printf("UI address: http://%s:%d\n", opts.FrontendIP, opts.UIPort)
	fmt.Printf("Namespace: %s\n", namespace)
	fmt.Printf("Database: %s\n", opts.DatabaseFile)

	// Create and start dev server
	devServer, err := embeddedtemporal.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create dev server: %w", err)
	}

	if err := devServer.Start(); err != nil {
		return fmt.Errorf("failed to start dev server: %w", err)
	}

	fmt.Println("\nTemporal dev server is running!")

	// Start recipe system if enabled
	var registry *recipe.Registry
	var workerManager *recipe.WorkerManager
	
	if enableRecipes {
		// Get recipe directory
		if recipeDir == "" {
			recipeDir = getRecipesDir()
		}
		
		fmt.Printf("\nStarting recipe system...\n")
		fmt.Printf("Recipe directory: %s\n", recipeDir)
		
		// Create logger for recipe system
		logger, _ := zap.NewProduction()
		defer logger.Sync()
		
		// Connect to Temporal
		c, err := client.Dial(client.Options{
			HostPort:  fmt.Sprintf("%s:%d", ip, port),
			Namespace: namespace,
		})
		if err != nil {
			devServer.Stop()
			return fmt.Errorf("failed to connect to Temporal: %w", err)
		}
		defer c.Close()
		
		// Create worker manager
		workerManager = recipe.NewWorkerManager(logger, c)
		
		// Create and start recipe registry
		registry, err = recipe.NewRegistry(logger, recipeDir, workerManager)
		if err != nil {
			devServer.Stop()
			return fmt.Errorf("failed to create recipe registry: %w", err)
		}
		
		if err := registry.Start(); err != nil {
			devServer.Stop()
			return fmt.Errorf("failed to start recipe registry: %w", err)
		}
		
		// Wait a moment for initial discovery
		time.Sleep(500 * time.Millisecond)
		
		// List discovered recipes
		recipes, _ := registry.ListRecipes(nil)
		if len(recipes) > 0 {
			fmt.Printf("\nDiscovered %d recipe(s):\n", len(recipes))
			for _, r := range recipes {
				fmt.Printf("  - %s (v%s): %s\n", r.Name, r.Version, r.WorkerStatus)
			}
		} else {
			fmt.Println("\nNo recipes discovered.")
			fmt.Printf("Place recipe YAML files in: %s\n", recipeDir)
		}
	}
	
	fmt.Println("\nPress Ctrl+C to stop")

	// Handle graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	sig := <-interrupt
	fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
	
	// Stop recipe system first
	if registry != nil {
		fmt.Println("Stopping recipe workers...")
		registry.Stop()
	}
	
	if workerManager != nil {
		workerManager.StopAll()
	}
	
	// Stop Temporal server
	if err := devServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	fmt.Println("Server stopped successfully")
	return nil
}
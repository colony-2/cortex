package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	rec "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

var (
	executeInputs        []string
	executeInputFile     string
	executeOutputFormat  string
	executeLogLevel      string
	executeLogFormat     string
	executeTimeout       string
	executeNoColor       bool
	executeStateDir      string
	executeCleanup       bool
	executeParallelLimit int
	executeDryRun        bool
)

// executeCmd executes a rec from the command line
var executeCmd = &cobra.Command{
	Use:   "execute <rec-file> [flags]",
	Short: "Execute a rec directly from the command line",
	Long: `Execute a rec YAML file directly, passing inputs as arguments and receiving outputs when complete.
This command is designed for CI/CD integration, testing, and standalone rec execution.`,
	Args: cobra.ExactArgs(1),
	RunE: runExecute,
}

func init() {
	executeCmd.Flags().StringArrayVarP(&executeInputs, "input", "i", nil, "Set a rec input value (JSON string or key=value for backwards compatibility)")
	executeCmd.Flags().StringVarP(&executeInputFile, "input-file", "f", "", "Load inputs from a JSON or YAML file")
	executeCmd.Flags().StringVarP(&executeOutputFormat, "output", "o", "json", "Output format for results (text, json, yaml)")
	executeCmd.Flags().StringVarP(&executeLogLevel, "log-level", "l", "info", "Set logging verbosity (debug, info, warn, error)")
	executeCmd.Flags().StringVar(&executeLogFormat, "log-format", "text", "Log output format (text, json)")
	executeCmd.Flags().StringVar(&executeTimeout, "timeout", "30m", "Maximum execution time (e.g., 30s, 5m, 1h)")
	executeCmd.Flags().BoolVar(&executeNoColor, "no-color", false, "Disable colored output for logs")
	executeCmd.Flags().StringVar(&executeStateDir, "state-dir", "", "Directory for temporary state files (default: system temp)")
	executeCmd.Flags().BoolVar(&executeCleanup, "cleanup", true, "Clean up state files after execution")
	executeCmd.Flags().IntVar(&executeParallelLimit, "parallel-limit", 10, "Maximum number of parallel operations")
	executeCmd.Flags().BoolVar(&executeDryRun, "dry-run", false, "Validate the recipe and inputs without executing")
}

// ExecutionResult represents the result of executing a rec
type ExecutionResult struct {
	Success        bool                   `json:"success" yaml:"success"`
	Outputs        map[string]interface{} `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Error          *ExecutionError        `json:"error,omitempty" yaml:"error,omitempty"`
	PartialOutputs map[string]interface{} `json:"partial_outputs,omitempty" yaml:"partial_outputs,omitempty"`
	ExecutionTime  string                 `json:"execution_time" yaml:"execution_time"`
	Recipe         string                 `json:"rec" yaml:"rec"`
	RunID          string                 `json:"run_id" yaml:"run_id"`
}

// ExecutionError represents an error during execution
type ExecutionError struct {
	Message string `json:"message" yaml:"message"`
	StepID  string `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Details string `json:"details,omitempty" yaml:"details,omitempty"`
}

// ExecutionState represents the state of an execution for interruption handling
type ExecutionState struct {
	RunID          string                 `json:"run_id"`
	RecipeFile     string                 `json:"recipe_file"`
	StartTime      time.Time              `json:"start_time"`
	LastUpdate     time.Time              `json:"last_update"`
	Status         string                 `json:"status"`
	CompletedSteps []string               `json:"completed_steps"`
	CurrentStep    string                 `json:"current_step,omitempty"`
	Outputs        map[string]interface{} `json:"outputs,omitempty"`
}

func runExecute(cmd *cobra.Command, args []string) error {
	recipeFile := args[0]
	startTime := time.Now()
	runID := fmt.Sprintf("run_%s", uuid.New().String()[:8])

	// Setup logger
	logger, err := setupLogger(executeLogLevel, executeLogFormat, executeNoColor)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer logger.Sync()

	// Log start
	logger.Info("Recipe started",
		zap.String("rec", filepath.Base(recipeFile)),
		zap.String("run_id", runID),
	)

	// Load recipe (new unified format only)
	recipeData, err := os.ReadFile(recipeFile)
	if err != nil {
		return outputError(err, "Failed to read rec file", recipeFile, runID, startTime)
	}

	var recipeDef rec.Recipe
	if err := yaml.Unmarshal(recipeData, &recipeDef); err != nil {
		if executeDryRun {
			return outputError(fmt.Errorf("validation error: %v", err), "Failed to parse rec YAML", recipeFile, runID, startTime)
		}
		return outputError(err, "Failed to parse rec YAML", recipeFile, runID, startTime)
	}

	// Parse inputs
	inputs, err := parseInputs(executeInputs, executeInputFile)
	if err != nil {
		return outputError(err, "Failed to parse inputs", recipeFile, runID, startTime)
	}

	// Apply default values for missing inputs using InputSchema
	inputSchema := recipeDef.GetMetdata().InputSchema
	if inputSchema != nil {
		for name, inputDef := range inputSchema {
			if _, exists := inputs[name]; !exists && inputDef.Default != nil {
				inputs[name] = inputDef.Default
			}
		}
	}

	// Log inputs
	logger.Info("Recipe inputs loaded",
		zap.Any("inputs", inputs),
	)
	// Emit a debug log when debug level is enabled
	logger.Debug("Debug mode enabled; inputs prepared", zap.Any("inputs", inputs))

	// Dry-run mode: validate and exit without executing
	if executeDryRun {
		// Basic structure validation: require a name for legacy recipes
		var raw map[string]interface{}
		if err := yaml.Unmarshal(recipeData, &raw); err == nil {
			if _, ok := raw["name"].(string); !ok || raw["name"].(string) == "" {
				return outputError(fmt.Errorf("validation error: missing required field 'name'"), "Recipe validation failed", recipeFile, runID, startTime)
			}
		}

		// Validate required inputs against InputSchema
		validator := shared.NewRecipeValidator()
		if err := validator.ValidateInputs(&recipeDef, inputs); err != nil {
			return outputError(fmt.Errorf("input validation error: %v", err), "Input validation failed", recipeFile, runID, startTime)
		}

		// Successful validation
		res := ExecutionResult{
			Success:       true,
			Outputs:       map[string]interface{}{},
			ExecutionTime: fmt.Sprintf("%.2fs", time.Since(startTime).Seconds()),
			Recipe:        recipeFile,
			RunID:         runID,
		}
		return outputResult(res, executeOutputFormat)
	}

	// Setup state directory
	stateDir := executeStateDir
	if stateDir == "" {
		stateDir = os.TempDir()
	}
	// Create a subdirectory for this run
	runStateDir := filepath.Join(stateDir, fmt.Sprintf("cortex-run-%s", runID))
	if err := os.MkdirAll(runStateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	stateDir = runStateDir
	if executeCleanup {
		defer os.RemoveAll(stateDir)
	}

	// Initialize execution state
	state := &ExecutionState{
		RunID:      runID,
		RecipeFile: recipeFile,
		StartTime:  startTime,
		LastUpdate: time.Now(),
		Status:     "running",
		Outputs:    make(map[string]interface{}),
	}

	// Save initial state
	if err := saveState(stateDir, state); err != nil {
		logger.Warn("Failed to save initial state", zap.Error(err))
	}

	// Parse timeout
	timeout, err := time.ParseDuration(executeTimeout)
	if err != nil {
		return outputError(err, "Invalid timeout format", recipeFile, runID, startTime)
	}

	// Setup execution context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Track signal count for force shutdown
	var signalCount int
	var signalMutex sync.Mutex

	go func() {
		for sig := range sigChan {
			signalMutex.Lock()
			signalCount++
			count := signalCount
			signalMutex.Unlock()

			if count == 1 {
				logger.Info("Shutdown requested, waiting for current operations to complete...")
				state.Status = "interrupted"
				saveState(stateDir, state)
				cancel()
			} else {
				logger.Warn("Force shutdown initiated")
				os.Exit(130)
			}

			_ = sig // Use sig to avoid unused variable warning
		}
	}()

	// Execute rec
	result, err := executeRecipe(ctx, &recipeDef, inputs, logger, state, stateDir)

	// Update final state
	state.Status = "completed"
	if err != nil {
		state.Status = "failed"
	}
	state.LastUpdate = time.Now()
	saveState(stateDir, state)

	// Check if context was cancelled
	if ctx.Err() == context.DeadlineExceeded {
		return outputError(fmt.Errorf("execution timeout exceeded"), "Timeout", recipeFile, runID, startTime)
	} else if ctx.Err() == context.Canceled {
		return outputError(fmt.Errorf("execution interrupted by user"), "Interrupted", recipeFile, runID, startTime)
	}

	if err != nil {
		return outputError(err, "Recipe execution failed", recipeFile, runID, startTime)
	}

	// Prepare final result
	result.Recipe = recipeFile
	result.RunID = runID
	result.ExecutionTime = fmt.Sprintf("%.2fs", time.Since(startTime).Seconds())

	logger.Info("Recipe completed",
		zap.String("total_duration", result.ExecutionTime),
		zap.String("status", "success"),
	)

	return outputResult(*result, executeOutputFormat)
}

func setupLogger(level, format string, noColor bool) (*zap.Logger, error) {
	// Parse log level
	var zapLevel zapcore.Level
	switch strings.ToLower(level) {
	case "debug":
		zapLevel = zap.DebugLevel
	case "info":
		zapLevel = zap.InfoLevel
	case "warn", "warning":
		zapLevel = zap.WarnLevel
	case "error":
		zapLevel = zap.ErrorLevel
	default:
		return nil, fmt.Errorf("invalid log level: %s", level)
	}

	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if !noColor && format == "text" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	// Create encoder
	var encoder zapcore.Encoder
	if format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Create logger
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stderr), zapLevel)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))

	return logger, nil
}

func parseInputs(inputFlags []string, inputFile string) (map[string]interface{}, error) {
	inputs := make(map[string]interface{})

	// Parse file inputs first
	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read input file: %w", err)
		}

		// Try JSON first
		if err := json.Unmarshal(data, &inputs); err != nil {
			// Try YAML
			if err := yaml.Unmarshal(data, &inputs); err != nil {
				return nil, fmt.Errorf("failed to parse input file as JSON or YAML: %w", err)
			}
		}
	}

	// Parse command-line inputs (override file inputs)
	for _, input := range inputFlags {
		// First try to parse as JSON object
		if strings.HasPrefix(strings.TrimSpace(input), "{") {
			var jsonInputs map[string]interface{}
			if err := json.Unmarshal([]byte(input), &jsonInputs); err != nil {
				return nil, fmt.Errorf("invalid JSON input: %s (%w)", input, err)
			}
			// Merge JSON inputs
			for k, v := range jsonInputs {
				inputs[k] = v
			}
		} else {
			// Fall back to key=value format for backwards compatibility
			parts := strings.SplitN(input, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid input format: %s (expected JSON object or key=value)", input)
			}

			key := parts[0]
			value := parts[1]

			// Try to parse value as JSON for complex types
			var parsedValue interface{}
			if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
				// If not JSON, treat as string
				parsedValue = value
			}

			// Handle nested keys (e.g., "data.source=database")
			if strings.Contains(key, ".") {
				setNestedValue(inputs, key, parsedValue)
			} else {
				inputs[key] = parsedValue
			}
		}
	}

	return inputs, nil
}

func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				// Can't set nested value if parent is not a map
				current[part] = make(map[string]interface{})
				current = current[part].(map[string]interface{})
			}
		}
	}
}

func validateRecipeStructure(recipe *rec.Recipe) error {
	return nil
}

func executeRecipe(ctx context.Context, recipe *rec.Recipe, inputs map[string]interface{}, logger *zap.Logger, state *ExecutionState, stateDir string) (*ExecutionResult, error) {
	// Use shared registry manager to get executor
	shared.RegisterOps()
	registry, err := ops.NewActivityRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to create activity registry: %w", err)
	}
	registry.SetDependencies(coreops.NewServiceDepsBuilder().Build())

	execute, err := executor.NewStandaloneExecutor(registry, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone executor: %w", err)
	}

	opts := executor.DefaultExecutionOptions()

	// Execute the rec
	outputs, err := execute.Execute(ctx, *recipe, inputs, opts)
	if err != nil {
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("execution timeout exceeded")
		}
		return nil, err
	}

	return &ExecutionResult{
		Success: true,
		Outputs: outputs,
	}, nil
}

func saveState(stateDir string, state *ExecutionState) error {
	state.LastUpdate = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	stateFile := filepath.Join(stateDir, "state.json")
	return os.WriteFile(stateFile, data, 0644)
}

func outputResult(result ExecutionResult, format string) error {
	var output []byte
	var err error

	switch format {
	case "json":
		// On success, emit only outputs; on error emit full structure
		if result.Success && result.Error == nil {
			if result.Outputs == nil {
				result.Outputs = map[string]interface{}{}
			}
			output, err = json.MarshalIndent(result.Outputs, "", "  ")
		} else {
			output, err = json.MarshalIndent(result, "", "  ")
		}
	case "yaml":
		// On success, emit only outputs; on error emit full structure
		if result.Success && result.Error == nil {
			if result.Outputs == nil {
				result.Outputs = map[string]interface{}{}
			}
			output, err = yaml.Marshal(result.Outputs)
		} else {
			output, err = yaml.Marshal(result)
		}
	case "text":
		// Human-readable formatting
		output = []byte(formatTextResult(result))
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	// Write to stdout
	fmt.Print(string(output))
	if format != "text" && len(output) > 0 {
		fmt.Println() // Add newline for json/yaml
	}
	return nil
}

// Legacy compatibility removed: recipes must conform to new unified schema (op/sequence/state)

func formatTextOutputs(outputs map[string]interface{}) string {
	var sb strings.Builder

	for key, value := range outputs {
		sb.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}

	return sb.String()
}

func formatTextResult(result ExecutionResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Recipe: %s\n", result.Recipe))
	if result.Success {
		sb.WriteString("Status: SUCCESS\n")
	} else {
		sb.WriteString("Status: FAILED\n")
	}
	sb.WriteString(fmt.Sprintf("Execution Time: %s\n", result.ExecutionTime))
	sb.WriteString(fmt.Sprintf("Run ID: %s\n", result.RunID))

	if len(result.Outputs) > 0 {
		sb.WriteString("\nOutputs:\n")
		for key, value := range result.Outputs {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	if result.Error != nil {
		sb.WriteString("\nError:\n")
		sb.WriteString(fmt.Sprintf("  Message: %s\n", result.Error.Message))
		if result.Error.StepID != "" {
			sb.WriteString(fmt.Sprintf("  Step: %s\n", result.Error.StepID))
		}
		if result.Error.Details != "" {
			sb.WriteString(fmt.Sprintf("  Details: %s\n", result.Error.Details))
		}
	}

	return sb.String()
}

func outputError(err error, message, recipeFile, runID string, startTime time.Time) error {
	result := ExecutionResult{
		Success: false,
		Error: &ExecutionError{
			Message: message,
			Details: err.Error(),
		},
		Recipe:        recipeFile,
		RunID:         runID,
		ExecutionTime: fmt.Sprintf("%.2fs", time.Since(startTime).Seconds()),
	}

	// Output the error result
	outputResult(result, executeOutputFormat)

	// Return appropriate exit code
	switch {
	case strings.Contains(err.Error(), "input"):
		os.Exit(4)
	case strings.Contains(err.Error(), "validation"):
		os.Exit(3)
	case strings.Contains(err.Error(), "timeout"):
		os.Exit(5)
	case strings.Contains(err.Error(), "interrupted"):
		os.Exit(130)
	default:
		os.Exit(1)
	}

	return err
}

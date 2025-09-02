package ops

import (
	"context"
	"fmt"
	"strings"

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// Test activity input/output types
type GenericInput struct {
	Message  string                 `json:"message,omitempty"`
	Command  string                 `json:"command,omitempty"`
	Text     string                 `json:"text,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Run      string                 `json:"run,omitempty"`
	Duration string                 `json:"duration,omitempty"`
	Data     interface{}            `json:"data,omitempty"`
	Items    []interface{}          `json:"items,omitempty"`
	Error    bool                   `json:"error,omitempty"`
	Extra    map[string]interface{} `json:"-" mapstructure:",remain"`
}

type GenericOutput struct {
	// String outputs
	Result      interface{}            `json:"result,omitempty"`
	Output      string                 `json:"output,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Stdout      string                 `json:"stdout,omitempty"`
	Report      string                 `json:"report,omitempty"`
	Response    string                 `json:"response,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Slept       string                 `json:"slept,omitempty"`
	Action      string                 `json:"action,omitempty"`
	Handler     string                 `json:"handler,omitempty"`
	Processor   string                 `json:"processor,omitempty"`
	Recipe      string                 `json:"recipe,omitempty"`
	
	// Boolean outputs
	Logged      bool                   `json:"logged,omitempty"`
	Valid       bool                   `json:"valid,omitempty"`
	
	// Numeric outputs
	Count       int                    `json:"count,omitempty"`
	Confidence  float64                `json:"confidence,omitempty"`
	
	// Complex outputs
	Model       interface{}            `json:"model,omitempty"`
	Data        interface{}            `json:"data,omitempty"`
}

func init() {
	// Register all test activities needed by the test suite
	registerTestActivities()
}

func registerTestActivities() {
	// Register command_execution activity
	commandExecution := recipeops.NewActivityMappedOp(
		recipeops.OpMetadata{
			Type: "command_execution",
			Name: "command_execution",
		},
		func(ctx context.Context, input GenericInput) (GenericOutput, error) {
			if input.Command == "" && input.Run == "" && input.Extra["run"] == nil {
				return GenericOutput{}, fmt.Errorf("command is required")
			}
			command := input.Command
			if command == "" {
				command = input.Run
			}
			if command == "" && input.Extra != nil {
				if cmd, ok := input.Extra["run"].(string); ok {
					command = cmd
				}
			}
			
			// For test purposes, handle echo commands specially
			if strings.HasPrefix(command, "echo '") && strings.HasSuffix(command, "'") {
				// Extract the message from echo command
				message := strings.TrimSuffix(strings.TrimPrefix(command, "echo '"), "'")
				stdout := message + "\n"
				return GenericOutput{
					Result: stdout,
					Stdout: stdout,
				}, nil
			}
			
			// Handle for loop echo commands
			if strings.Contains(command, "for i in $(seq") {
				// Parse the repeat count and message
				// Example: "for i in $(seq 1 3); do echo 'Hi'; done"
				var repeatCount int = 1
				var message string = ""
				
				// Extract repeat count
				if idx := strings.Index(command, "$(seq 1 "); idx != -1 {
					endIdx := strings.Index(command[idx+8:], ")")
					if endIdx != -1 {
						countStr := command[idx+8 : idx+8+endIdx]
						fmt.Sscanf(countStr, "%d", &repeatCount)
					}
				}
				
				// Extract message
				if idx := strings.Index(command, "echo '"); idx != -1 {
					endIdx := strings.Index(command[idx+6:], "'")
					if endIdx != -1 {
						message = command[idx+6 : idx+6+endIdx]
					}
				}
				
				// Build repeated output
				var result strings.Builder
				for i := 0; i < repeatCount; i++ {
					if i > 0 {
						result.WriteString(" ")
					}
					result.WriteString(message)
				}
				result.WriteString("\n")
				
				output := result.String()
				return GenericOutput{
					Result: output,
					Stdout: output,
				}, nil
			}
			
			// Handle date command
			if strings.Contains(command, "date") {
				return GenericOutput{
					Result: "timestamp",
					Stdout: "timestamp",
				}, nil
			}
			
			// Default simulation for other commands
			result := fmt.Sprintf("Executed: %s", command)
			return GenericOutput{
				Result: result,
				Stdout: result,
				Status: "success",
			}, nil
		},
	)
	recipeops.Register(commandExecution)

	// Register echo_activity
	echoActivity := recipeops.NewActivityMappedOp(
		recipeops.OpMetadata{
			Type: "echo_activity",
			Name: "echo_activity",
		},
		func(ctx context.Context, input GenericInput) (GenericOutput, error) {
			message := input.Message
			if message == "" {
				message = "Hello, World!"
			}
			return GenericOutput{
				Output: message + "\n",
			}, nil
		},
	)
	recipeops.Register(echoActivity)

	// Register all other typed test activities
	registerTypedTestActivities()
}
package ops

import (
	"context"

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
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
	Result    interface{} `json:"result,omitempty"`
	Output    string      `json:"output,omitempty"`
	Status    string      `json:"status,omitempty"`
	Stdout    string      `json:"stdout,omitempty"`
	Report    string      `json:"report,omitempty"`
	Response  string      `json:"response,omitempty"`
	Body      string      `json:"body,omitempty"`
	Slept     string      `json:"slept,omitempty"`
	Action    string      `json:"action,omitempty"`
	Handler   string      `json:"handler,omitempty"`
	Processor string      `json:"processor,omitempty"`
	Recipe    string      `json:"recipe,omitempty"`

	// Boolean outputs
	Logged bool `json:"logged,omitempty"`
	Valid  bool `json:"valid,omitempty"`

	// Numeric outputs
	Count      int     `json:"count,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`

	// Complex outputs
	Model interface{}   `json:"model,omitempty"`
	Data  interface{}   `json:"data,omitempty"`
	Items []interface{} `json:"items,omitempty"`
}

func init() {
	// Register all test activities needed by the test suite
	registerTestActivities()
}

func registerTestActivities() {

	recipeops.Register(commandop.GetOp())
	recipeops.Register(sleepop.GetOp())

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
				Output: message,
			}, nil
		},
	)
	recipeops.Register(echoActivity)

	// Register all other typed test activities
	registerTypedTestActivities()
}

package sleep

import (
	"context"
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/ops/pkg/types"
)

// SleepConfig defines the configuration for sleep activities - ALL fields MUST have json tags
type SleepConfig struct {
	// Optional: default duration if not specified in input
	DefaultDuration string `json:"default_duration"`
}

// SleepInput defines the input for sleep activities - ALL fields MUST have json tags
type SleepInput struct {
	Duration string `json:"duration"` // Required: duration to sleep (e.g., "5s", "1m", "500ms")
}

// SleepOutput defines the output from sleep activities - ALL fields MUST have json tags
type SleepOutput struct {
	StartTime      time.Time `json:"start_time"`      // When the sleep started
	EndTime        time.Time `json:"end_time"`        // When the sleep ended
	ActualDuration string    `json:"actual_duration"` // Actual duration slept
	Completed      bool      `json:"completed"`       // Whether sleep completed normally
	Interrupted    bool      `json:"interrupted"`     // Whether sleep was interrupted
	ErrorMessage   string    `json:"error_message"`   // Error message if interrupted
}

// SleepActivityWrapper implements the RegisterableActivity interface
type SleepActivityWrapper struct{}

// Ensure we implement the interface
var _ types.RegisterableActivity[SleepConfig, SleepInput, SleepOutput] = (*SleepActivityWrapper)(nil)

// NewSleepActivity creates a new sleep activity that implements RegisterableActivity
func NewSleepActivity() types.RegisterableActivity[SleepConfig, SleepInput, SleepOutput] {
	return &SleepActivityWrapper{}
}

// GetMetadata returns activity metadata for registration
func (a *SleepActivityWrapper) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "sleep",
		Name:           "Sleep/Delay",
		Description:    "Pauses execution for a specified duration",
		Version:        "1.0.0",
		DefaultTimeout: 24 * time.Hour, // Long timeout to support long sleeps
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    1, // Don't retry sleeps
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			NonRetryableErrorTypes: []string{
				"InvalidDurationError",
				"CancelledError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *SleepActivityWrapper) Execute(ctx context.Context, config SleepConfig, input SleepInput) (SleepOutput, error) {
	// Determine duration to use
	durationStr := input.Duration
	if durationStr == "" && config.DefaultDuration != "" {
		durationStr = config.DefaultDuration
	}
	if durationStr == "" {
		return SleepOutput{}, fmt.Errorf("duration is required")
	}

	// Parse the duration
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return SleepOutput{}, fmt.Errorf("invalid duration '%s': %w", durationStr, err)
	}

	// Record start time
	startTime := time.Now()

	// Create a timer
	timer := time.NewTimer(duration)
	defer timer.Stop()

	// Wait for either the timer or context cancellation
	select {
	case <-timer.C:
		// Sleep completed normally
		endTime := time.Now()
		actualDuration := endTime.Sub(startTime)
		
		return SleepOutput{
			StartTime:      startTime,
			EndTime:        endTime,
			ActualDuration: actualDuration.String(),
			Completed:      true,
			Interrupted:    false,
		}, nil
		
	case <-ctx.Done():
		// Context cancelled (timeout or interruption)
		endTime := time.Now()
		actualDuration := endTime.Sub(startTime)
		
		output := SleepOutput{
			StartTime:      startTime,
			EndTime:        endTime,
			ActualDuration: actualDuration.String(),
			Completed:      false,
			Interrupted:    true,
			ErrorMessage:   ctx.Err().Error(),
		}
		
		// Return the output with the error
		// The error will cause the activity to fail, but the output still contains useful info
		return output, ctx.Err()
	}
}
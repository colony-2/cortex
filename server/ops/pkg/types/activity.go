// Package types defines interfaces for activities that can be consumed by recipe-worker
package types

import (
	"context"
	"time"
)

// RegisterableActivity defines the contract for activities that can be consumed
// by external systems like recipe-worker via YAML definitions
type RegisterableActivity[TConfig any, TInput any, TOutput any] interface {
	// GetMetadata returns activity metadata for registration
	GetMetadata() ActivityMetadata

	// Execute runs the activity with provided configuration and inputs
	Execute(ctx context.Context, config TConfig, inputs TInput) (TOutput, error)
}

// ActivityMetadata describes the activity for registration and documentation
type ActivityMetadata struct {
	Type           string         // Unique identifier for the activity type
	Name           string         // Human-readable name
	Description    string         // Detailed description
	Version        string         // Semantic version
	DefaultTimeout time.Duration  // Default execution timeout
	RetryPolicy    *RetryPolicy   // Default retry configuration
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaximumAttempts        int32
	InitialInterval        time.Duration
	BackoffCoefficient     float64
	MaximumInterval        time.Duration
	NonRetryableErrorTypes []string
}
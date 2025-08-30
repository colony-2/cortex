package compiler

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"go.temporal.io/sdk/temporal"
)

// ToTemporalRetryPolicy converts our RetryPolicy to Temporal's RetryPolicy
func ToTemporalRetryPolicy(r *recipe.RetryPolicy) *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:        r.InitialInterval.ToDuration(),
		BackoffCoefficient:     r.BackoffCoefficient,
		MaximumInterval:        r.MaximumInterval.ToDuration(),
		MaximumAttempts:        r.MaximumAttempts,
		NonRetryableErrorTypes: r.NonRetryableErrorTypes,
	}
}

// FromTemporalRetryPolicy converts Temporal's RetryPolicy to our RetryPolicy
func FromTemporalRetryPolicy(temporalPolicy *temporal.RetryPolicy) *recipe.RetryPolicy {
	if temporalPolicy == nil {
		return nil
	}

	return &recipe.RetryPolicy{
		InitialInterval:        recipe.Duration(temporalPolicy.InitialInterval),
		BackoffCoefficient:     temporalPolicy.BackoffCoefficient,
		MaximumInterval:        recipe.Duration(temporalPolicy.MaximumInterval),
		MaximumAttempts:        temporalPolicy.MaximumAttempts,
		NonRetryableErrorTypes: temporalPolicy.NonRetryableErrorTypes,
	}
}

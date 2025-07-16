package recipehistory

import (
	"context"
	"fmt"

	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	recipecore "github.com/vibethis/server/recipe-core"
)

// Client provides recipe-centric view of execution history
type Client struct {
	temporal    client.Client
	transformer *Transformer
	logger      *zap.Logger
}

// NewClient creates a new history client
func NewClient(temporal client.Client, getRecipe GetRecipeFunc, logger *zap.Logger) *Client {
	return &Client{
		temporal:    temporal,
		transformer: NewTransformer(getRecipe),
		logger:      logger,
	}
}

// ListJobs lists job executions for a recipe
func (c *Client) ListJobs(ctx context.Context, recipeName string, filter *JobFilter) ([]*recipecore.Job, error) {
	// Build query for listing workflows
	query := fmt.Sprintf(`TaskQueue = "%s-%s"`, "ono-recipes", recipeName)

	// Add status filter if specified
	if filter != nil && filter.Status != "" {
		switch filter.Status {
		case "running":
			query += ` AND ExecutionStatus = "Running"`
		case "completed":
			query += ` AND ExecutionStatus = "Completed"`
		case "failed":
			query += ` AND ExecutionStatus = "Failed"`
		case "all":
			// No additional filter
		default:
			return nil, fmt.Errorf("invalid status filter: %s", filter.Status)
		}
	}

	// Set default limit
	limit := int32(20)
	if filter != nil && filter.Limit > 0 {
		limit = int32(filter.Limit)
	}

	// List workflow executions
	listRequest := &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: limit,
		Query:    query,
	}

	resp, err := c.temporal.WorkflowService().ListWorkflowExecutions(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow executions: %w", err)
	}

	// Convert workflow executions to jobs
	jobs, err := c.transformer.WorkflowExecutionsToJobs(resp.Executions, recipeName)
	if err != nil {
		return nil, fmt.Errorf("failed to transform workflow executions: %w", err)
	}

	return jobs, nil
}

// GetJob retrieves detailed information about a specific job
func (c *Client) GetJob(ctx context.Context, recipeName, jobID string, includeActivities bool) (*recipecore.Job, error) {
	// Get workflow execution details
	desc, err := c.temporal.DescribeWorkflowExecution(ctx, jobID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to describe workflow execution: %w", err)
	}

	// Transform workflow description to job
	job, err := c.transformer.DescribeWorkflowToJob(desc, recipeName)
	if err != nil {
		return nil, fmt.Errorf("failed to transform workflow description: %w", err)
	}

	// Get activity executions if requested and workflow is completed
	if includeActivities && len(desc.PendingActivities) == 0 {
		activities, err := c.getActivityExecutions(ctx, jobID, recipeName)
		if err != nil {
			c.logger.Warn("Failed to get activity executions", 
				zap.String("jobID", jobID),
				zap.Error(err))
		} else {
			job.Activities = activities
		}
	}

	return job, nil
}

// getActivityExecutions retrieves activity execution history for a workflow
func (c *Client) getActivityExecutions(ctx context.Context, workflowID, recipeName string) ([]*recipecore.ActivityExecution, error) {
	// Get workflow history
	iter := c.temporal.GetWorkflowHistory(ctx, workflowID, "", false, 0)
	var events []*history.HistoryEvent
	
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to iterate workflow history: %w", err)
		}
		events = append(events, event)
	}

	if len(events) == 0 {
		return nil, nil
	}

	// Transform history to activity executions
	hist := &history.History{
		Events: events,
	}
	
	activities, err := c.transformer.HistoryToActivityExecutions(hist, recipeName)
	if err != nil {
		return nil, fmt.Errorf("failed to transform activity history: %w", err)
	}

	return activities, nil
}

// JobFilter contains filter options for listing jobs
type JobFilter struct {
	Status string
	Limit  int
}
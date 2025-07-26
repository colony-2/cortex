package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// RecipeFormatter handles formatting of recipe-related output
type RecipeFormatter struct {
	useColor bool
}

// NewRecipeFormatter creates a new recipe formatter
func NewRecipeFormatter(useColor bool) *RecipeFormatter {
	return &RecipeFormatter{
		useColor: useColor,
	}
}

// FormatRecipeList formats a list of recipes for display
func (f *RecipeFormatter) FormatRecipeList(recipes []*recipe.Recipe) string {
	if len(recipes) == 0 {
		return "No recipes found."
	}

	var sb strings.Builder
	
	// Header
	sb.WriteString(f.formatHeader("RECIPES"))
	sb.WriteString("\n\n")
	
	// Table header
	headers := []string{"NAME", "VERSION", "STATUS", "LAST MODIFIED", "DESCRIPTION"}
	widths := []int{20, 10, 12, 20, 40}
	
	sb.WriteString(f.formatTableHeader(headers, widths))
	sb.WriteString("\n")
	sb.WriteString(f.formatSeparator(widths))
	sb.WriteString("\n")
	
	// Recipe rows
	for _, r := range recipes {
		row := []string{
			r.Name,
			r.Version,
			f.formatWorkerStatus(r.WorkerStatus),
			f.formatTime(r.LastModified),
			truncate(r.Description, 40),
		}
		sb.WriteString(f.formatTableRow(row, widths))
		sb.WriteString("\n")
	}
	
	// Summary
	sb.WriteString(fmt.Sprintf("\nTotal: %d recipe(s)\n", len(recipes)))
	
	return sb.String()
}

// FormatRecipeDetail formats detailed recipe information
func (f *RecipeFormatter) FormatRecipeDetail(r *recipe.Recipe) string {
	if r == nil {
		return "Recipe not found"
	}
	
	var sb strings.Builder
	
	// Header
	sb.WriteString(f.formatHeader(fmt.Sprintf("RECIPE: %s", r.Name)))
	sb.WriteString("\n\n")
	
	// Basic info
	sb.WriteString(f.formatField("Name", r.Name))
	sb.WriteString(f.formatField("Version", r.Version))
	sb.WriteString(f.formatField("Description", r.Description))
	sb.WriteString(f.formatField("Status", f.formatWorkerStatus(r.WorkerStatus)))
	sb.WriteString(f.formatField("Path", r.BasePath))
	sb.WriteString(f.formatField("Last Modified", f.formatTime(r.LastModified)))
	sb.WriteString(f.formatField("Hash", r.Hash[:12]+"..."))
	
	// Workflow info
	if r.Workflow != nil {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("WORKFLOW"))
		sb.WriteString("\n")
		sb.WriteString(f.formatField("  Name", r.Workflow.Name))
		sb.WriteString(f.formatField("  Description", r.Workflow.Description))
		sb.WriteString(f.formatField("  Type", r.Workflow.Workflow.Type))
		sb.WriteString(f.formatField("  Steps", fmt.Sprintf("%d", len(r.Workflow.Workflow.Steps))))
	}
	
	// Activities
	if len(r.Activities) > 0 {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("ACTIVITIES"))
		sb.WriteString("\n")
		for _, a := range r.Activities {
			sb.WriteString(fmt.Sprintf("  • %s", f.colorize(a.Name, color.FgCyan)))
			if a.Description != "" {
				sb.WriteString(fmt.Sprintf(" - %s", a.Description))
			}
			sb.WriteString("\n")
		}
	}
	
	// Recent jobs
	sb.WriteString("\n")
	sb.WriteString(f.formatSubHeader("RECENT JOBS"))
	sb.WriteString("\n")
	sb.WriteString("  Use 'ono recipe history " + r.Name + "' to view execution history\n")
	
	return sb.String()
}

// FormatJobList formats a list of jobs for display
func (f *RecipeFormatter) FormatJobList(jobs []*recipe.Job, recipeName string) string {
	if len(jobs) == 0 {
		return fmt.Sprintf("No jobs found for recipe '%s'.", recipeName)
	}

	var sb strings.Builder
	
	// Header
	sb.WriteString(f.formatHeader(fmt.Sprintf("JOBS FOR RECIPE: %s", recipeName)))
	sb.WriteString("\n\n")
	
	// Table header
	headers := []string{"JOB ID", "STATUS", "START TIME", "DURATION", "ERROR"}
	widths := []int{30, 12, 20, 12, 40}
	
	sb.WriteString(f.formatTableHeader(headers, widths))
	sb.WriteString("\n")
	sb.WriteString(f.formatSeparator(widths))
	sb.WriteString("\n")
	
	// Job rows
	for _, j := range jobs {
		duration := "-"
		if j.Duration != nil {
			duration = formatDuration(*j.Duration)
		}
		
		errorMsg := ""
		if j.Error != "" {
			errorMsg = truncate(j.Error, 40)
		}
		
		row := []string{
			truncate(j.ID, 30),
			f.formatJobStatus(j.Status),
			f.formatTime(j.StartTime),
			duration,
			errorMsg,
		}
		sb.WriteString(f.formatTableRow(row, widths))
		sb.WriteString("\n")
	}
	
	// Summary
	sb.WriteString(fmt.Sprintf("\nTotal: %d job(s)\n", len(jobs)))
	
	return sb.String()
}

// FormatJobDetail formats detailed job information
func (f *RecipeFormatter) FormatJobDetail(job *recipe.Job) string {
	if job == nil {
		return "Job not found"
	}
	
	var sb strings.Builder
	
	// Header
	sb.WriteString(f.formatHeader(fmt.Sprintf("JOB: %s", job.ID)))
	sb.WriteString("\n\n")
	
	// Basic info
	sb.WriteString(f.formatField("Job ID", job.ID))
	sb.WriteString(f.formatField("Recipe", job.RecipeName))
	sb.WriteString(f.formatField("Status", f.formatJobStatus(job.Status)))
	sb.WriteString(f.formatField("Started", f.formatTime(job.StartTime)))
	
	if job.Duration != nil {
		sb.WriteString(f.formatField("Duration", formatDuration(*job.Duration)))
	}
	
	if job.UpdateTime.After(job.StartTime) {
		sb.WriteString(f.formatField("Last Update", f.formatTime(job.UpdateTime)))
	}
	
	if job.Error != "" {
		sb.WriteString("\n")
		sb.WriteString(f.formatField("Error", f.colorize(job.Error, color.FgRed)))
	}
	
	// Inputs
	if len(job.Inputs) > 0 {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("INPUTS"))
		sb.WriteString("\n")
		for k, v := range job.Inputs {
			sb.WriteString(f.formatField("  "+k, fmt.Sprintf("%v", v)))
		}
	}
	
	// Outputs
	if len(job.Outputs) > 0 {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("OUTPUTS"))
		sb.WriteString("\n")
		for k, v := range job.Outputs {
			sb.WriteString(f.formatField("  "+k, fmt.Sprintf("%v", v)))
		}
	}
	
	// Activities
	if len(job.Activities) > 0 {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("ACTIVITIES"))
		sb.WriteString("\n")
		
		for i, a := range job.Activities {
			sb.WriteString(fmt.Sprintf("  %d. %s", i+1, f.colorize(a.Name, color.FgCyan)))
			sb.WriteString(fmt.Sprintf(" [%s]", f.formatActivityStatus(a.Status)))
			
			if a.Duration != nil {
				sb.WriteString(fmt.Sprintf(" (%s)", formatDuration(*a.Duration)))
			}
			
			if a.Error != "" {
				sb.WriteString(fmt.Sprintf("\n     Error: %s", f.colorize(a.Error, color.FgRed)))
			}
			
			sb.WriteString("\n")
		}
	}
	
	// Execution info (for debugging)
	if job.ExecutionInfo != nil {
		sb.WriteString("\n")
		sb.WriteString(f.formatSubHeader("EXECUTION INFO"))
		sb.WriteString("\n")
		sb.WriteString(f.formatField("  Workflow ID", job.ExecutionInfo.WorkflowID))
		sb.WriteString(f.formatField("  Run ID", job.ExecutionInfo.RunID))
	}
	
	return sb.String()
}

// Helper methods

func (f *RecipeFormatter) formatHeader(text string) string {
	if f.useColor {
		return color.New(color.FgWhite, color.Bold).Sprint(text)
	}
	return text
}

func (f *RecipeFormatter) formatSubHeader(text string) string {
	if f.useColor {
		return color.New(color.FgYellow, color.Bold).Sprint(text)
	}
	return text
}

func (f *RecipeFormatter) formatField(name, value string) string {
	if f.useColor {
		nameColor := color.New(color.FgWhite)
		return fmt.Sprintf("%s: %s\n", nameColor.Sprint(name), value)
	}
	return fmt.Sprintf("%s: %s\n", name, value)
}

func (f *RecipeFormatter) formatWorkerStatus(status recipe.WorkerStatus) string {
	statusStr := string(status)
	if !f.useColor {
		return statusStr
	}
	
	switch status {
	case recipe.WorkerStatusRunning:
		return color.GreenString(statusStr)
	case recipe.WorkerStatusStopped:
		return color.YellowString(statusStr)
	case recipe.WorkerStatusFailed:
		return color.RedString(statusStr)
	default:
		return statusStr
	}
}

func (f *RecipeFormatter) formatJobStatus(status recipe.JobStatus) string {
	statusStr := string(status)
	if !f.useColor {
		return statusStr
	}
	
	switch status {
	case recipe.JobStatusRunning:
		return color.CyanString(statusStr)
	case recipe.JobStatusCompleted:
		return color.GreenString(statusStr)
	case recipe.JobStatusFailed:
		return color.RedString(statusStr)
	case recipe.JobStatusCanceled:
		return color.YellowString(statusStr)
	default:
		return statusStr
	}
}

func (f *RecipeFormatter) formatActivityStatus(status string) string {
	if !f.useColor {
		return status
	}
	
	switch status {
	case "running":
		return color.CyanString(status)
	case "completed":
		return color.GreenString(status)
	case "failed":
		return color.RedString(status)
	case "scheduled":
		return color.YellowString(status)
	default:
		return status
	}
}

func (f *RecipeFormatter) formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

func (f *RecipeFormatter) colorize(text string, attrs ...color.Attribute) string {
	if !f.useColor {
		return text
	}
	return color.New(attrs...).Sprint(text)
}

func (f *RecipeFormatter) formatTableHeader(headers []string, widths []int) string {
	var parts []string
	for i, h := range headers {
		parts = append(parts, f.pad(h, widths[i]))
	}
	return strings.Join(parts, " ")
}

func (f *RecipeFormatter) formatTableRow(values []string, widths []int) string {
	var parts []string
	for i, v := range values {
		parts = append(parts, f.pad(v, widths[i]))
	}
	return strings.Join(parts, " ")
}

func (f *RecipeFormatter) formatSeparator(widths []int) string {
	var parts []string
	for _, w := range widths {
		parts = append(parts, strings.Repeat("-", w))
	}
	return strings.Join(parts, " ")
}

func (f *RecipeFormatter) pad(s string, width int) string {
	if len(s) > width {
		return s[:width-3] + "..."
	}
	return fmt.Sprintf("%-*s", width, s)
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
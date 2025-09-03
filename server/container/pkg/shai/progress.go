package shai

import (
	"fmt"
	"time"
)

// Phase represents a stage in container creation
type Phase string

const (
	PhaseValidating  Phase = "validating"
	PhasePulling     Phase = "pulling"
	PhaseBuilding    Phase = "building"
	PhaseInstalling  Phase = "installing"
	PhaseCreating    Phase = "creating"
	PhaseStarting    Phase = "starting"
)

// ProgressUpdate represents a progress update for ephemeral containers
type ProgressUpdate struct {
	Phase   string // FEATURES, ONCREATE, UPDATECONTENT, POSTCREATE, POSTSTART, POSTATTACH, USERSWITCH
	Status  string // START, COMPLETE, ERROR, PROGRESS
	Message string // Human-readable message
}

// ProgressCallback reports progress during operations
type ProgressCallback func(phase Phase, message string)

// ProgressReporter manages progress callbacks
type ProgressReporter struct {
	callback ProgressCallback
}

// NewProgressReporter creates a progress reporter
func NewProgressReporter() *ProgressReporter {
	return &ProgressReporter{}
}

// SetCallback sets the progress callback
func (p *ProgressReporter) SetCallback(cb ProgressCallback) {
	p.callback = cb
}

// Report sends a progress update
func (p *ProgressReporter) Report(phase Phase, message string) {
	if p.callback != nil {
		p.callback(phase, message)
	}
}

// ReportWithDuration reports progress with elapsed time
func (p *ProgressReporter) ReportWithDuration(phase Phase, message string, start time.Time) {
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		message = fmt.Sprintf("%s (%ds)", message, int(elapsed.Seconds()))
	}
	p.Report(phase, message)
}

// StartPhase begins tracking a phase and returns a function to complete it
func (p *ProgressReporter) StartPhase(phase Phase, message string) func(string) {
	start := time.Now()
	p.Report(phase, message)
	
	return func(completionMessage string) {
		if completionMessage == "" {
			completionMessage = message
		}
		p.ReportWithDuration(phase, completionMessage, start)
	}
}

// WithProgress wraps an operation with progress reporting
func (p *ProgressReporter) WithProgress(phase Phase, message string, fn func() error) error {
	start := time.Now()
	p.Report(phase, message)
	
	err := fn()
	
	if err != nil {
		p.Report(phase, fmt.Sprintf("%s: failed", message))
		return err
	}
	
	p.ReportWithDuration(phase, fmt.Sprintf("%s: done", message), start)
	return nil
}
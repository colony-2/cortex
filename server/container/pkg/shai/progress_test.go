package shai

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProgressReporter_SetCallback(t *testing.T) {
	reporter := NewProgressReporter()
	
	called := false
	var capturedPhase Phase
	var capturedMessage string
	
	reporter.SetCallback(func(phase Phase, message string) {
		called = true
		capturedPhase = phase
		capturedMessage = message
	})
	
	reporter.Report(PhaseValidating, "test message")
	
	if !called {
		t.Error("callback was not called")
	}
	if capturedPhase != PhaseValidating {
		t.Errorf("expected phase %v, got %v", PhaseValidating, capturedPhase)
	}
	if capturedMessage != "test message" {
		t.Errorf("expected message 'test message', got %q", capturedMessage)
	}
}

func TestProgressReporter_Report(t *testing.T) {
	tests := []struct {
		name     string
		phase    Phase
		message  string
		hasCallback bool
	}{
		{
			name:        "with callback",
			phase:       PhasePulling,
			message:     "pulling image",
			hasCallback: true,
		},
		{
			name:        "without callback",
			phase:       PhaseBuilding,
			message:     "building container",
			hasCallback: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := NewProgressReporter()
			
			called := false
			if tt.hasCallback {
				reporter.SetCallback(func(phase Phase, message string) {
					called = true
					if phase != tt.phase {
						t.Errorf("expected phase %v, got %v", tt.phase, phase)
					}
					if message != tt.message {
						t.Errorf("expected message %q, got %q", tt.message, message)
					}
				})
			}
			
			reporter.Report(tt.phase, tt.message)
			
			if tt.hasCallback && !called {
				t.Error("callback should have been called")
			}
			if !tt.hasCallback && called {
				t.Error("callback should not have been called")
			}
		})
	}
}

func TestProgressReporter_ReportWithDuration(t *testing.T) {
	reporter := NewProgressReporter()
	
	var capturedMessage string
	reporter.SetCallback(func(phase Phase, message string) {
		capturedMessage = message
	})
	
	// Test with short duration (< 2 seconds)
	start := time.Now()
	reporter.ReportWithDuration(PhaseCreating, "creating container", start)
	if capturedMessage != "creating container" {
		t.Errorf("expected message without duration, got %q", capturedMessage)
	}
	
	// Test with long duration (> 2 seconds)
	start = time.Now().Add(-3 * time.Second)
	reporter.ReportWithDuration(PhaseInstalling, "installing features", start)
	if !strings.Contains(capturedMessage, "(3s)") {
		t.Errorf("expected message with duration, got %q", capturedMessage)
	}
}

func TestProgressReporter_StartPhase(t *testing.T) {
	reporter := NewProgressReporter()
	
	var messages []string
	reporter.SetCallback(func(phase Phase, message string) {
		messages = append(messages, message)
	})
	
	// Start a phase
	complete := reporter.StartPhase(PhaseStarting, "starting container")
	
	// Should have received initial message
	if len(messages) != 1 || messages[0] != "starting container" {
		t.Errorf("expected initial message 'starting container', got %v", messages)
	}
	
	// Complete with custom message
	time.Sleep(10 * time.Millisecond) // Small delay to test duration
	complete("container started")
	
	// Should have received completion message
	if len(messages) != 2 || messages[1] != "container started" {
		t.Errorf("expected completion message 'container started', got %v", messages)
	}
}

func TestProgressReporter_StartPhase_DefaultCompletion(t *testing.T) {
	reporter := NewProgressReporter()
	
	var messages []string
	reporter.SetCallback(func(phase Phase, message string) {
		messages = append(messages, message)
	})
	
	// Start a phase
	complete := reporter.StartPhase(PhaseValidating, "validating config")
	
	// Complete with empty message (should use original)
	complete("")
	
	if len(messages) != 2 || messages[1] != "validating config" {
		t.Errorf("expected default completion message, got %v", messages)
	}
}

func TestProgressReporter_WithProgress(t *testing.T) {
	tests := []struct {
		name          string
		operationErr  error
		expectSuccess bool
	}{
		{
			name:          "successful operation",
			operationErr:  nil,
			expectSuccess: true,
		},
		{
			name:          "failed operation",
			operationErr:  errors.New("operation failed"),
			expectSuccess: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := NewProgressReporter()
			
			var messages []string
			reporter.SetCallback(func(phase Phase, message string) {
				messages = append(messages, message)
			})
			
			err := reporter.WithProgress(PhasePulling, "pulling image", func() error {
				return tt.operationErr
			})
			
			// Check error propagation
			if tt.expectSuccess && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if !tt.expectSuccess && err == nil {
				t.Error("expected error, got nil")
			}
			
			// Check messages
			if len(messages) < 2 {
				t.Fatalf("expected at least 2 messages, got %d", len(messages))
			}
			
			// First message should be the start message
			if messages[0] != "pulling image" {
				t.Errorf("expected start message 'pulling image', got %q", messages[0])
			}
			
			// Last message should indicate success or failure
			lastMessage := messages[len(messages)-1]
			if tt.expectSuccess {
				if !strings.Contains(lastMessage, "done") {
					t.Errorf("expected success message to contain 'done', got %q", lastMessage)
				}
			} else {
				if !strings.Contains(lastMessage, "failed") {
					t.Errorf("expected failure message to contain 'failed', got %q", lastMessage)
				}
			}
		})
	}
}

func TestProgressReporter_NoCallback(t *testing.T) {
	reporter := NewProgressReporter()
	
	// These should not panic when no callback is set
	reporter.Report(PhaseValidating, "test")
	reporter.ReportWithDuration(PhaseCreating, "test", time.Now())
	complete := reporter.StartPhase(PhaseStarting, "test")
	complete("done")
	err := reporter.WithProgress(PhaseBuilding, "test", func() error {
		return nil
	})
	
	if err != nil {
		t.Errorf("WithProgress should not return error from operation: %v", err)
	}
}
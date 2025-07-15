// Package rwshimgo provides the compatibility wrapper for backwards compatibility.
// All functionality is now in the pkg/rwshim package.
package rwshimgo

import "github.com/divisive-ai/vibethis/rwshim/rwshimgo/pkg/rwshim"

// Re-export all public types and functions for backwards compatibility

// Operation represents a read or write operation
type Operation = rwshim.Operation

const (
	// OpRead represents a read operation
	OpRead = rwshim.OpRead
	// OpWrite represents a write operation
	OpWrite = rwshim.OpWrite
)

// Request represents an intercepted I/O operation
type Request = rwshim.Request

// Response represents the decision for an I/O operation
type Response = rwshim.Response

// PolicyFunc is the callback function type for deciding whether to allow operations
type PolicyFunc = rwshim.PolicyFunc

// Monitor manages the Unix domain socket server and intercepts I/O operations
type Monitor = rwshim.Monitor

// Process represents a monitored process
type Process = rwshim.Process

// ProcessOption configures process execution
type ProcessOption = rwshim.ProcessOption

// PolicyBuilder helps create complex policies
type PolicyBuilder = rwshim.PolicyBuilder

// PolicyRule represents a single policy rule
type PolicyRule = rwshim.PolicyRule

const (
	// DefaultSocketPath is the default Unix domain socket path
	DefaultSocketPath = rwshim.DefaultSocketPath
)

// NewMonitor creates a new Monitor with the given policy function
var NewMonitor = rwshim.NewMonitor

// NewMonitorWithPath creates a new Monitor with a custom socket path
var NewMonitorWithPath = rwshim.NewMonitorWithPath

// WithShimPath sets a custom path to the intercept.so library
var WithShimPath = rwshim.WithShimPath

// WithEnv adds environment variables to the process
var WithEnv = rwshim.WithEnv

// WithDir sets the working directory for the process
var WithDir = rwshim.WithDir

// WithStdin sets the process stdin
var WithStdin = rwshim.WithStdin

// WithStdout sets the process stdout
var WithStdout = rwshim.WithStdout

// WithStderr sets the process stderr
var WithStderr = rwshim.WithStderr

// Common policy implementations
var AllowAll = rwshim.AllowAll
var DenyAll = rwshim.DenyAll
var DenyWrites = rwshim.DenyWrites
var DenyReads = rwshim.DenyReads

// NewPolicyBuilder creates a new policy builder
var NewPolicyBuilder = rwshim.NewPolicyBuilder

// Common matcher functions
var MatchFilename = rwshim.MatchFilename
var MatchFilenamePrefix = rwshim.MatchFilenamePrefix
var MatchFilenameSuffix = rwshim.MatchFilenameSuffix
var MatchFD = rwshim.MatchFD
var MatchStdStreams = rwshim.MatchStdStreams
var MatchLargeOperations = rwshim.MatchLargeOperations
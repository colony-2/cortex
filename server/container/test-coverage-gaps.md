# Test Coverage Gaps

## Runner Initialization (`pkg/shai/runner.go`)
- **New() function**: Test runner creation with valid manager, working directory validation, devcontainer.json detection in both standard and alternate locations
- **Mount builder integration**: Verify mount builder is correctly initialized and stored, handles invalid paths, and propagates mount builder errors
- **Configuration defaults**: Test working directory auto-detection when not specified, error handling for getcwd failures, and config validation

## Container Lifecycle (`pkg/shai/runner.go`)
- **Start() method**: Test successful container creation and startup flow with mock manager, verify progress reporting at each phase
- **Container creation failures**: Test error propagation when manager.Create fails, cleanup behavior on failure, and appropriate error messages
- **Container start failures**: Test error handling when manager.Start fails after successful creation, verify partial state cleanup
- **GetInfo failures**: Test behavior when container info retrieval fails after successful start, ensure container is left in consistent state

## Interactive Attachment (`pkg/shai/runner.go`)
- **AttachInteractive with support**: Test successful attachment when manager implements AttachInteractive interface, verify containerID is passed correctly
- **AttachInteractive without support**: Test error returned when manager doesn't implement AttachInteractive, verify graceful degradation with clear error message
- **Attachment failures**: Test error propagation when AttachInteractive fails, verify terminal state is properly restored on failure

## Resource Cleanup (`pkg/shai/runner.go`)
- **Close() with closeable manager**: Test cleanup when manager implements Close interface, verify Close is called exactly once
- **Close() without closeable manager**: Test no panic when manager doesn't implement Close, verify method returns nil
- **Progress callback cleanup**: Test OnProgress callback is properly set and can be called multiple times without issues

## Terminal Attachment (`internal/devcontainer/terminal.go`)
- **Non-terminal environment**: Test error handling when not running in a terminal, verify appropriate error message for non-TTY context
- **Terminal state restoration**: Test oldState is properly saved and restored on cleanup, verify cleanup on panic/unexpected exit
- **Container attach failures**: Test error handling when Docker attach fails, verify terminal state restored even on attach failure
- **I/O streaming errors**: Test behavior when stdin/stdout copy fails during streaming, verify graceful shutdown on I/O errors

## Terminal Resize (`internal/devcontainer/terminal.go`)
- **Resize signal handling**: Test SIGWINCH signal triggers resize, verify resize dimensions are correctly calculated and sent
- **Resize with nil client**: Test no panic when client is nil during resize, verify graceful handling of resize errors
- **Context cancellation**: Test resize goroutine exits cleanly on context cancel, verify no goroutine leaks after shutdown

## Mount Conflict Edge Cases (`pkg/shai/mounts.go`)
- **Complex path overlaps**: Test detection of nested mount conflicts beyond simple parent-child, verify symlink handling in mount paths
- **Mount string generation**: Test BuildMountStrings with various mount configurations, verify correct read-only and read-write flags
- **Special characters in paths**: Test paths with spaces, unicode, and special characters are properly escaped in mount strings

## Progress Reporting Edge Cases (`pkg/shai/progress.go`)
- **Concurrent progress updates**: Test thread-safety of progress reporter with concurrent Report calls from multiple goroutines
- **Callback replacement**: Test SetCallback can safely replace existing callback while operations are in progress
- **Duration calculation accuracy**: Test ReportWithDuration correctly calculates elapsed time across system clock changes

## CLI Integration (`cmd/shai/main.go`)
- **Flag parsing errors**: Test handling of invalid flag combinations, missing required flags, and malformed flag values
- **Manager creation failure**: Test graceful exit when devcontainer.NewManager fails, verify error message clarity for user
- **Signal handling**: Test SIGINT/SIGTERM properly triggers cleanup, verify container removal on forced termination
- **Progress display formatting**: Test spinner animation and progress messages display correctly, verify verbose mode shows additional details
package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
)

// handleErr handles the structured expansion of error attributes
func handleErr(groups []string, a slog.Attr) slog.Attr {
	if err, ok := a.Value.Any().(error); ok {
		// Get PC (Program Counter) for the stack trace
		pcs := make([]uintptr, 10)
		n := runtime.Callers(4, pcs) // Skip internal slog/runtime frames
		frames := runtime.CallersFrames(pcs[:n])

		var stack []string
		for {
			frame, more := frames.Next()
			stack = append(stack, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
			if !more {
				break
			}
		}

		// Return a Group so the JSON output is structured
		return slog.Group(a.Key,
			slog.String("msg", err.Error()),
			slog.String("type", fmt.Sprintf("%T", err)),
			slog.Any("stacktrace", stack),
		)
	}
	return a
}

func setupLogger() {
	// 1. Configure Handler Options
	opts := &slog.HandlerOptions{
		Level:       slog.LevelDebug, // Enable verbose Debug output
		AddSource:   true,            // Adds the file:line of the log call itself
		ReplaceAttr: handleErr,       // Intercept attributes to expand errors
	}

	// 2. Initialize JSON Logger
	// JSON is required for readable "verbose" structured data
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)
}

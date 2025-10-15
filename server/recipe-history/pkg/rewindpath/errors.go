package rewindpath

import "errors"

var (
	// ErrInvalidRequest indicates the caller supplied an incomplete rewind request.
	ErrInvalidRequest = errors.New("rewindpath: invalid request")
	// ErrUnavailable indicates the builder cannot reach the underlying story provider.
	ErrUnavailable = errors.New("rewindpath: story provider unavailable")
	// ErrStoryUnavailable indicates the requested story could not be loaded.
	ErrStoryUnavailable = errors.New("rewindpath: story unavailable")
	// ErrInvocationNotFound is returned when the target invocation cannot be resolved in a story.
	ErrInvocationNotFound = errors.New("rewindpath: invocation not found")
	// ErrMissingResumeEvent indicates the story is missing the resume marker required for reset.
	ErrMissingResumeEvent = errors.New("rewindpath: missing resume event id")
)

package template

import (
	"strings"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

type invocationTracker struct {
	segments []string
	counters map[string]int
}

func newInvocationTracker() *invocationTracker {
	return &invocationTracker{
		segments: make([]string, 0, 8),
		counters: make(map[string]int),
	}
}

func (t *invocationTracker) child(segment string) *invocationTracker {
	if segment == "" {
		return t
	}
	segments := append([]string(nil), t.segments...)
	segments = append(segments, segment)
	return &invocationTracker{
		segments: segments,
		counters: t.counters,
	}
}

func (t *invocationTracker) currentPath() string {
	if len(t.segments) == 0 {
		return ""
	}
	return strings.Join(t.segments, "/")
}

func (t *invocationTracker) nextInvocation() coreops.Invocation {
	path := t.currentPath()
	seq := t.counters[path]
	t.counters[path] = seq + 1

	inv := coreops.Invocation{
		NodePath:  path,
		InvokeSeq: seq,
	}
	return inv
}

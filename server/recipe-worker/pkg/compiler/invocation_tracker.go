package compiler

import (
	"strings"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

type invocationTracker struct {
	recipeID string
	segments []string
	counters map[string]int
}

func newInvocationTracker(meta recipe.RecipeMetadata) *invocationTracker {
	recipeID := meta.ID
	if recipeID == "" {
		recipeID = meta.Desc
	}
	if recipeID == "" {
		recipeID = "recipe"
	}

	return &invocationTracker{
		recipeID: recipeID,
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
		recipeID: t.recipeID,
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

func (t *invocationTracker) nextInvocation(boxID, activityID string) coreops.Invocation {
	path := t.currentPath()
	seq := t.counters[path]
	t.counters[path] = seq + 1

	inv := coreops.Invocation{
		RecipeID:   t.recipeID,
		NodePath:   path,
		InvokeSeq:  seq,
		BoxID:      boxID,
		ActivityID: activityID,
	}
	if inv.ID == "" {
		inv.ID = inv.Hash()
	}
	return inv
}

func segmentForMetadata(meta recipe.NodeMetadata, fallback string) string {
	if meta.ID != "" {
		return meta.ID
	}
	if fallback != "" {
		return fallback
	}
	return "node"
}

func extractFirstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

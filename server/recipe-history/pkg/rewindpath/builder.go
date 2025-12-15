package rewindpath

import (
	"context"
	"fmt"

	runmetadata "github.com/colony-2/colony2/server/recipe-core/pkg/runmetadata"
	"github.com/colony-2/colony2/server/recipe-history/pkg/storybuilder"
)

// StoryProvider fetches recipe execution stories keyed by workflow identifiers.
type StoryProvider interface {
	BuildStory(ctx context.Context, recipeName, workflowID, runID string) (*storybuilder.Story, error)
}

// Builder constructs resume payload segments for recipe rewinds using execution stories.
type Builder struct {
	provider StoryProvider
	cache    map[string]*storybuilder.Story
}

// New creates a new execution-path builder backed by the supplied story provider.
func New(provider StoryProvider) *Builder {
	return &Builder{
		provider: provider,
		cache:    make(map[string]*storybuilder.Story),
	}
}

// Request describes the leaf invocation we want to rewind to.
type Request struct {
	LeafRecipeName       string
	LeafWorkflowID       string
	LeafRunID            string
	TargetInvocationHash string
}

// Result contains the ordered execution path segments (outermost -> innermost).
type Result struct {
	Segments []runmetadata.Segment
}

// Build constructs the execution path required to rewind the target invocation.
func (b *Builder) Build(ctx context.Context, req Request) (*Result, error) {
	if req.TargetInvocationHash == "" {
		return nil, fmt.Errorf("%w: target invocation hash missing", ErrInvalidRequest)
	}
	if req.LeafWorkflowID == "" {
		return nil, fmt.Errorf("%w: leaf workflow id missing", ErrInvalidRequest)
	}
	if req.LeafRunID == "" {
		return nil, fmt.Errorf("%w: leaf run id missing", ErrInvalidRequest)
	}

	leafStory, err := b.getStory(ctx, req.LeafRecipeName, req.LeafWorkflowID, req.LeafRunID)
	if err != nil {
		return nil, err
	}

	leafRun, err := findRunForInvocation(leafStory, req.TargetInvocationHash, "")
	if err != nil {
		return nil, err
	}

	segments := make([]runmetadata.Segment, 0, 4)
	currentStory := leafStory
	currentRun := leafRun
	currentInvocation := req.TargetInvocationHash

	for {
		segment, err := buildSegment(currentStory, currentRun, currentInvocation)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)

		parent := currentStory.Metadata.Parent
		if parent == nil {
			break
		}

		parentStory, err := b.getStory(ctx, "", parent.WorkflowID, parent.RunID)
		if err != nil {
			return nil, err
		}

		parentRun, err := findRunForInvocation(parentStory, parent.InvocationHash, currentStory.Metadata.RunID)
		if err != nil {
			return nil, err
		}

		currentStory = parentStory
		currentRun = parentRun
		currentInvocation = parent.InvocationHash
	}

	reverseSegments(segments)
	return &Result{Segments: segments}, nil
}

func (b *Builder) getStory(ctx context.Context, recipeName, workflowID, runID string) (*storybuilder.Story, error) {
	key := cacheKey(workflowID, runID)
	if cached, ok := b.cache[key]; ok {
		return cached, nil
	}
	if b.provider == nil {
		return nil, fmt.Errorf("%w: story provider missing", ErrUnavailable)
	}
	story, err := b.provider.BuildStory(ctx, recipeName, workflowID, runID)
	if err != nil {
		return nil, err
	}
	if story == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrStoryUnavailable, workflowID, runID)
	}
	b.cache[key] = story
	return story, nil
}

func buildSegment(story *storybuilder.Story, run *storybuilder.NodeRun, invocationHash string) (runmetadata.Segment, error) {
	if story == nil || run == nil {
		return runmetadata.Segment{}, ErrInvocationNotFound
	}
	if run.ResumeEventID == 0 {
		return runmetadata.Segment{}, fmt.Errorf("%w: %s", ErrMissingResumeEvent, invocationHash)
	}

	workflowID := run.ChildWorkflowID
	runID := run.ChildRunID
	if workflowID == "" {
		workflowID = story.Metadata.WorkflowID
	}
	if runID == "" {
		runID = story.Metadata.RunID
	}
	if workflowID == "" || runID == "" {
		return runmetadata.Segment{}, fmt.Errorf("%w: invocation %s has blank workflow identifiers", ErrInvocationNotFound, invocationHash)
	}

	segment := runmetadata.Segment{
		InvocationHash: invocationHash,
		WorkflowID:     workflowID,
		RunID:          runID,
		EventID:        run.ResumeEventID,
	}
	if run.RecipeSetIndex != nil {
		idx := *run.RecipeSetIndex
		segment.RecipeSetIndex = &idx
	}
	if run.WaitForChild != nil {
		wait := *run.WaitForChild
		segment.WaitForChild = &wait
	}
	return segment, nil
}

func findRunForInvocation(story *storybuilder.Story, invocationHash, childRunID string) (*storybuilder.NodeRun, error) {
	if story == nil {
		return nil, ErrInvocationNotFound
	}
	if invocationHash == "" {
		return nil, ErrInvocationNotFound
	}

	if run := story.Indexes.ByInvocationHash[invocationHash]; run != nil {
		if childRunID == "" || childRunID == run.ChildRunID || run.ChildRunID == "" {
			return run, nil
		}
	}

	if childRunID != "" {
		if run := story.Indexes.ByChildRunID[childRunID]; run != nil {
			if run.InvocationHash == invocationHash || run.InvocationID == invocationHash {
				return run, nil
			}
		}
	}

	if run := story.Indexes.ByInvocationID[invocationHash]; run != nil {
		if childRunID == "" || childRunID == run.ChildRunID || run.ChildRunID == "" {
			return run, nil
		}
	}

	for _, node := range story.Nodes {
		if found := searchNodeRuns(node, invocationHash, childRunID); found != nil {
			return found, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrInvocationNotFound, invocationHash)
}

func searchNodeRuns(node *storybuilder.StoryNode, invocationHash, childRunID string) *storybuilder.NodeRun {
	if node == nil {
		return nil
	}
	for _, run := range node.Runs {
		if run.InvocationHash == invocationHash || run.InvocationID == invocationHash {
			if childRunID == "" || childRunID == run.ChildRunID || run.ChildRunID == "" {
				return run
			}
		}
	}
	for _, child := range node.Children {
		if found := searchNodeRuns(child, invocationHash, childRunID); found != nil {
			return found
		}
	}
	return nil
}

func reverseSegments(segments []runmetadata.Segment) {
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}
}

func cacheKey(workflowID, runID string) string {
	return workflowID + "@" + runID
}

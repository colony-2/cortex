package rewindpath

import (
	"context"
	"fmt"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-history/pkg/storybuilder"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build(t *testing.T) {
	rootRun := newBool(true)
	midAsync := newBool(false)
	recipeIndex := 1

	leafStory := &storybuilder.Story{
		Metadata: storybuilder.StoryMetadata{
			RecipeName: "leaf",
			WorkflowID: "leaf-wf",
			RunID:      "leaf-run",
			Parent: &storybuilder.StoryParentLink{
				WorkflowID:     "mid-wf",
				RunID:          "mid-run",
				InvocationHash: "mid.set",
				RecipeSetIndex: &recipeIndex,
			},
		},
		Nodes: []*storybuilder.StoryNode{{
			Path: "leaf/target",
			Runs: []*storybuilder.NodeRun{{
				InvocationID:   "leaf.target",
				InvocationHash: "leaf.target",
				ResumeEventID:  42,
				Status:         "completed",
			}},
		}},
	}
	leafStory.Indexes = storybuilder.StoryIndexes{
		ByInvocationID:   map[string]*storybuilder.NodeRun{"leaf.target": leafStory.Nodes[0].Runs[0]},
		ByInvocationHash: map[string]*storybuilder.NodeRun{"leaf.target": leafStory.Nodes[0].Runs[0]},
		ByChildRunID:     map[string]*storybuilder.NodeRun{},
	}

	midRun := &storybuilder.NodeRun{
		InvocationID:    "mid.set",
		InvocationHash:  "mid.set",
		ResumeEventID:   80,
		ChildWorkflowID: "leaf-wf",
		ChildRunID:      "leaf-run",
		RecipeSetIndex:  &recipeIndex,
		WaitForChild:    midAsync,
	}
	midStory := &storybuilder.Story{
		Metadata: storybuilder.StoryMetadata{
			RecipeName: "mid",
			WorkflowID: "mid-wf",
			RunID:      "mid-run",
			Parent: &storybuilder.StoryParentLink{
				WorkflowID:     "root-wf",
				RunID:          "root-run",
				InvocationHash: "root.recipe",
			},
		},
		Nodes: []*storybuilder.StoryNode{{
			Path: "mid/set",
			Runs: []*storybuilder.NodeRun{midRun},
		}},
	}
	midStory.Indexes = storybuilder.StoryIndexes{
		ByInvocationID: map[string]*storybuilder.NodeRun{
			"mid.set": midRun,
		},
		ByInvocationHash: map[string]*storybuilder.NodeRun{
			"mid.set": midRun,
		},
		ByChildRunID: map[string]*storybuilder.NodeRun{
			"leaf-run": midRun,
		},
	}

	rootRunNode := &storybuilder.NodeRun{
		InvocationID:    "root.recipe",
		InvocationHash:  "root.recipe",
		ResumeEventID:   100,
		ChildWorkflowID: "mid-wf",
		ChildRunID:      "mid-run",
		WaitForChild:    rootRun,
	}
	rootStory := &storybuilder.Story{
		Metadata: storybuilder.StoryMetadata{
			RecipeName: "root",
			WorkflowID: "root-wf",
			RunID:      "root-run",
		},
		Nodes: []*storybuilder.StoryNode{{
			Path: "root/recipe",
			Runs: []*storybuilder.NodeRun{rootRunNode},
		}},
	}
	rootStory.Indexes = storybuilder.StoryIndexes{
		ByInvocationID: map[string]*storybuilder.NodeRun{
			"root.recipe": rootRunNode,
		},
		ByInvocationHash: map[string]*storybuilder.NodeRun{
			"root.recipe": rootRunNode,
		},
		ByChildRunID: map[string]*storybuilder.NodeRun{
			"mid-run": rootRunNode,
		},
	}

	provider := &fakeProvider{
		stories: map[string]*storybuilder.Story{
			cacheKey("leaf-wf", "leaf-run"): leafStory,
			cacheKey("mid-wf", "mid-run"):   midStory,
			cacheKey("root-wf", "root-run"): rootStory,
		},
	}

	builder := New(provider)
	result, err := builder.Build(context.Background(), Request{
		LeafRecipeName:       "leaf",
		LeafWorkflowID:       "leaf-wf",
		LeafRunID:            "leaf-run",
		TargetInvocationHash: "leaf.target",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Segments, 3)

	rootSeg := result.Segments[0]
	require.Equal(t, "root.recipe", rootSeg.InvocationHash)
	require.Equal(t, "mid-wf", rootSeg.WorkflowID)
	require.Equal(t, "mid-run", rootSeg.RunID)
	require.EqualValues(t, 100, rootSeg.EventID)
	require.NotNil(t, rootSeg.WaitForChild)
	require.True(t, *rootSeg.WaitForChild)

	midSeg := result.Segments[1]
	require.Equal(t, "mid.set", midSeg.InvocationHash)
	require.Equal(t, "leaf-wf", midSeg.WorkflowID)
	require.Equal(t, "leaf-run", midSeg.RunID)
	require.EqualValues(t, 80, midSeg.EventID)
	require.NotNil(t, midSeg.RecipeSetIndex)
	require.Equal(t, recipeIndex, *midSeg.RecipeSetIndex)
	require.NotNil(t, midSeg.WaitForChild)
	require.False(t, *midSeg.WaitForChild)

	leafSeg := result.Segments[2]
	require.Equal(t, "leaf.target", leafSeg.InvocationHash)
	require.Equal(t, "leaf-wf", leafSeg.WorkflowID)
	require.Equal(t, "leaf-run", leafSeg.RunID)
	require.EqualValues(t, 42, leafSeg.EventID)
	require.Nil(t, leafSeg.RecipeSetIndex)
	require.Nil(t, leafSeg.WaitForChild)
}

func newBool(v bool) *bool {
	b := v
	return &b
}

type fakeProvider struct {
	stories map[string]*storybuilder.Story
}

func (f *fakeProvider) BuildStory(ctx context.Context, recipeName, workflowID, runID string) (*storybuilder.Story, error) {
	if f == nil {
		return nil, ErrUnavailable
	}
	if story, ok := f.stories[cacheKey(workflowID, runID)]; ok {
		return story, nil
	}
	return nil, fmt.Errorf("story not found: %s/%s", workflowID, runID)
}

package template

import (
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMode_ArtifactLookupReturnsPlaceholderKey(t *testing.T) {
	opts := DefaultResolutionOptions()
	opts.Mode = ModeValidate
	opts.ClampSliceIndex = true

	root, err := NewRecipeResolutionContext(&contextual.GitCommitContext{}, map[string]interface{}{}, contextual.JobContext{}, opts)
	require.NoError(t, err)

	seqCtx := newSequenceCtx(t, root, "seq", map[string]interface{}{})
	seqCtx.TemplateData.Sequence["write"] = StepOutput{
		Outputs:   map[string]interface{}{},
		Artifacts: map[string]swf.Artifact{},
	}

	val, err := seqCtx.resolveTemplate(`${{ sequence.write.artifacts["foo.txt"] }}`)
	require.NoError(t, err)

	key, ok := val.(swf.ArtifactKey)
	require.True(t, ok, "expected swf.ArtifactKey, got %T", val)
	assert.Equal(t, validationArtifactPlaceholderJobID, key.JobId)
	assert.Equal(t, validationArtifactPlaceholderTaskOrdinal, key.TaskOrdinal)
	assert.Equal(t, "foo.txt", key.Name)
	assert.Equal(t, int64(-1), key.SizeBytes)
}

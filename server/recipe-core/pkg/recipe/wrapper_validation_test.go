package recipe

import (
	"fmt"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/task"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

// normalIn/Out used to verify existing behavior remains intact
type normalIn struct {
	N int `yaml:"n" json:"n"`
}
type normalOut struct {
	M int `yaml:"m" json:"m"`
}

// wrapperIn validates via UnmarshalYAML that a required key exists
type wrapperIn struct {
	Data map[string]interface{} `yaml:",inline"`
}

func (w *wrapperIn) UnmarshalYAML(node *yamlv3.Node) error {
	var m map[string]interface{}
	if err := node.Decode(&m); err != nil {
		return err
	}
	w.Data = m
	if _, ok := m["must"]; !ok {
		return fmt.Errorf("missing required key 'must'")
	}
	return nil
}

func TestOpInputs_Normal_Decode_Succeeds(t *testing.T) {
	ops.Clear()
	op := ops.NewActivityMappedOpV2[normalIn, normalOut](
		ops.OpMetadata{Type: "normal"},
		func(_ ops.Invocation, _ task.Context, in normalIn) (normalOut, error) {
			return normalOut{M: in.N}, nil
		},
	)
	ops.Register(op)

	var n Node
	// Correct type for n should decode without error
	require.NoError(t, yamlv3.Unmarshal([]byte("op: normal\ninputs: {n: 42}"), &n))
}

func TestOpInputs_Normal_InvalidType_ProducesError(t *testing.T) {
	ops.Clear()
	op := ops.NewActivityMappedOpV2[normalIn, normalOut](
		ops.OpMetadata{Type: "normal"},
		func(_ ops.Invocation, _ task.Context, in normalIn) (normalOut, error) {
			return normalOut{M: in.N}, nil
		},
	)
	ops.Register(op)

	var n Node
	// Provide string for int field to trigger YAML type error during input decoding
	err := yamlv3.Unmarshal([]byte("op: normal\ninputs: {n: \"oops\"}"), &n)
	require.Error(t, err)
}

func TestOpInputs_Wrapper_UnmarshalYAML_Validation_Invoked(t *testing.T) {
	ops.Clear()
	op := ops.NewActivityMappedOpV2[wrapperIn, normalOut](
		ops.OpMetadata{Type: "wrapped"},
		func(_ ops.Invocation, _ task.Context, in wrapperIn) (normalOut, error) {
			return normalOut{M: 0}, nil
		},
	)
	ops.Register(op)

	// Missing required key should surface wrapper's validation error
	var n Node
	err := yamlv3.Unmarshal([]byte("op: wrapped\ninputs: {x: 1}"), &n)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required key 'must'")

	// When key is present, it should decode successfully
	var n2 Node
	require.NoError(t, yamlv3.Unmarshal([]byte("op: wrapped\ninputs: {must: yes}"), &n2))
}

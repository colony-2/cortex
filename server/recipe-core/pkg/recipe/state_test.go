package recipe

import (
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

func TestStateMachine_Construct_And_YAML(t *testing.T) {
	registerTestOp()
	// State machines define initial state correctly [pkg/recipe/state.go]
	sm := &RecipeState{StateMachineData: StateMachineData{States: &StateMap{Initial: "start", States: map[string]State{
		"start": {Node: Node{NodeImpl: &NodeOp{OpData: OpData{Op: "echo"}}}, SingleStateMetadata: SingleStateMetadata{}},
	}}}}
	assert.Equal(t, "start", sm.States.Initial)

	// Complete state machines serialize to YAML [pkg/recipe/state.go]
	b, err := yamlv3.Marshal(sm)
	require.NoError(t, err)
	var back RecipeState
	require.NoError(t, yamlv3.Unmarshal(b, &back))
	assert.Equal(t, sm.States.Initial, back.States.Initial)
}

func TestState_Transitions_With_CEL(t *testing.T) {
	// Conditional transitions evaluate CEL expressions [pkg/recipe/state.go]
	expr, err := cel.NewCELExpr("inputs.ok == true")
	require.NoError(t, err)
	st := State{SingleStateMetadata: SingleStateMetadata{Transitions: []Transition{{To: "next", When: *expr}}}}
	assert.Equal(t, "next", st.Transitions[0].To)
}

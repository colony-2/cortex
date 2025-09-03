package recipe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidOp(t *testing.T) {
	_, err := LoadRecipeFromString([]byte(r2))
	require.EqualErrorf(t, err, "unknown op: [my_invalid_op] at [2:1]", "invalid op")
}

const r2 = `
id: invalid_op_test
version: 1.0.0
op: my_invalid_op
`

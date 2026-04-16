package main

import (
	"github.com/colony-2/colony2/server/llm/pkg/llm"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

func main() {
	ops.CommandMain(llm.GetEnhancedOp())
}

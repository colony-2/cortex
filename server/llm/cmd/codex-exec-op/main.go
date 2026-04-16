package main

import (
	"github.com/colony-2/colony2/server/llm/pkg/codex"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

func main() {
	ops.CommandMain(codex.GetOp())
}

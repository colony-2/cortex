package main

import (
	"github.com/colony-2/colony2/server/gha/pkg/gha"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

func main() {
	ops.CommandMain(gha.GetRunsOp())
}

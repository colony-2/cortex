package test_fixtures_test

import (
	"testing"

	recipechild "github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	testfixtures "github.com/colony-2/colony2/server/recipe-worker/test-fixtures"
)

func TestRecipeChildFixtures(t *testing.T) {
	ops.Register(recipechild.GetOps()...)
	testfixtures.RunTestOnAllRecipes("recipes/*.test.yaml", t)
}

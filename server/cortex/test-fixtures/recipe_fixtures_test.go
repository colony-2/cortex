package test_fixtures_test

import (
	"testing"

	serverdepsops "github.com/colony-2/colony2/server/api/pkg/serverdeps/opssetup"
	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	recipechild "github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	testfixtures "github.com/colony-2/colony2/server/recipe-worker/test-fixtures"
)

// Runs the recipe fixture harness described in the hot-add fixtures spec.
// Add fixture yaml files under test-fixtures/recipes/ and they will be picked up by the glob.
func TestRecipeFixtures(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)
	db := pg.DB

	serverdepsops.RegisterOps()
	ops.Register(recipechild.GetOps()...)
	testfixtures.RunTestOnAllRecipesWithDeps(ops.NewServiceDepsBuilder().WithDatabase(db).Build(), "recipes/*.test.yaml", t)
}

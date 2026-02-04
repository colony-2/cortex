package serverdeps

import (
	"context"
	"errors"
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	workerworkflow "github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubSSEManager struct{}

func (s *stubSSEManager) Broadcast(coreops.SSEEvent)             {}
func (s *stubSSEManager) Subscribe(string) <-chan coreops.SSEEvent { return make(chan coreops.SSEEvent) }
func (s *stubSSEManager) Unsubscribe(string)                    {}

var _ coreops.SSEManager = (*stubSSEManager)(nil)

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestBuildServiceDependencies_ReusesProvidedValues(t *testing.T) {
	eng := toy.NewToyEngine([]swf.WorkSet{})

	db := openTestDB(t, "primary")
	fallbackDB := openTestDB(t, "fallback")
	sse := &stubSSEManager{}

	base := coreops.NewServiceDepsBuilder().
		WithDatabase(db).
		WithSSEManager(sse).
		Build()

	deps := buildServiceDependencies(EngineConfig{
		Context:      context.Background(),
		PostgresDB:   fallbackDB,
		Dependencies: base,
	}, eng)

	require.Same(t, db, deps.Database())
	require.Same(t, sse, deps.SSEManager())

	ctl := deps.WorkflowControl()
	require.NotNil(t, ctl)

	wfCtl, ok := ctl.(*workerworkflow.SWFWorkflowControl)
	require.True(t, ok)
	require.Same(t, eng, wfCtl.Engine)
	require.NotNil(t, wfCtl.Registry)
}

func TestBuildServiceDependencies_WiresWorkflowControlEngineAndRegistry(t *testing.T) {
	eng := toy.NewToyEngine([]swf.WorkSet{})

	wantErr := errors.New("registry called")
	registry := workerworkflow.RecipeProjectProvider(func(string, string) (*recipecore.Recipe, error) {
		return nil, wantErr
	})

	placeholder := &workerworkflow.SWFWorkflowControl{
		Registry: registry,
	}

	base := coreops.NewServiceDepsBuilder().
		WithWorkflowControl(placeholder).
		Build()

	deps := buildServiceDependencies(EngineConfig{
		PostgresDB:   openTestDB(t, "fallback"),
		Dependencies: base,
	}, eng)

	wfCtl, ok := deps.WorkflowControl().(*workerworkflow.SWFWorkflowControl)
	require.True(t, ok)
	require.Same(t, eng, wfCtl.Engine)

	_, err := wfCtl.Registry("p", "r")
	require.ErrorIs(t, err, wantErr)

	// Ensure we build a new dependency container and don't mutate the original placeholder.
	require.Nil(t, placeholder.Engine)
}

func TestBuildServiceDependencies_UsesFallbackDBWhenMissing(t *testing.T) {
	eng := toy.NewToyEngine([]swf.WorkSet{})

	fallbackDB := openTestDB(t, "fallback")
	base := coreops.NewServiceDepsBuilder().Build()

	deps := buildServiceDependencies(EngineConfig{
		PostgresDB:   fallbackDB,
		Dependencies: base,
	}, eng)

	require.Same(t, fallbackDB, deps.Database())
}

func TestBuildServiceDependencies_DefaultRegistryReturnsError(t *testing.T) {
	eng := toy.NewToyEngine([]swf.WorkSet{})

	deps := buildServiceDependencies(EngineConfig{
		PostgresDB:   openTestDB(t, "fallback"),
		Dependencies: coreops.NewServiceDepsBuilder().Build(),
	}, eng)

	ctl := deps.WorkflowControl()
	require.NotNil(t, ctl)

	_, err := ctl.StartJob(context.Background(), workflowctl.StartJob{TenantId: "p", RecipeName: "r"})
	require.Error(t, err)
}

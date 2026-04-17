package serverdeps

import (
	"context"
	"errors"
	"testing"

	coreops "github.com/colony-2/c2j/pkg/ops"
	recipecore "github.com/colony-2/c2j/pkg/recipe"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	toyruntime "github.com/colony-2/swf-go/pkg/swf/runtime/toy"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubSSEManager struct{}

func (s *stubSSEManager) Broadcast(coreops.SSEEvent) {}
func (s *stubSSEManager) Subscribe(string) <-chan coreops.SSEEvent {
	return make(chan coreops.SSEEvent)
}
func (s *stubSSEManager) Unsubscribe(string) {}

var _ coreops.SSEManager = (*stubSSEManager)(nil)

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func newToyEngine(t *testing.T) swf.SWFEngine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eng, err := swf.NewEngineBuilder().
		WithRuntime(toyruntime.New()).
		BuildEngine()
	require.NoError(t, err)
	go eng.Run(ctx)
	return eng
}

func TestBuildServiceDependencies_ReusesProvidedValues(t *testing.T) {
	eng := newToyEngine(t)

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
	eng := newToyEngine(t)

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
	eng := newToyEngine(t)

	fallbackDB := openTestDB(t, "fallback")
	base := coreops.NewServiceDepsBuilder().Build()

	deps := buildServiceDependencies(EngineConfig{
		PostgresDB:   fallbackDB,
		Dependencies: base,
	}, eng)

	require.Same(t, fallbackDB, deps.Database())
}

func TestBuildServiceDependencies_DefaultRegistryReturnsError(t *testing.T) {
	eng := newToyEngine(t)

	deps := buildServiceDependencies(EngineConfig{
		PostgresDB:   openTestDB(t, "fallback"),
		Dependencies: coreops.NewServiceDepsBuilder().Build(),
	}, eng)

	ctl := deps.WorkflowControl()
	require.NotNil(t, ctl)

	_, err := ctl.StartJob(context.Background(), workflowctl.StartJob{TenantId: "p", RecipeName: "r"})
	require.Error(t, err)
}

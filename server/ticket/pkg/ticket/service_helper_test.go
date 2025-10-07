package ticket_test

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/ticket/internal/testutil"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
)

func TestNewServiceFromDB(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	svc, err := ticket.NewServiceFromDB(pg.DB)
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestNewServiceFromDBNil(t *testing.T) {
	svc, err := ticket.NewServiceFromDB(nil)
	require.Error(t, err)
	require.Nil(t, svc)
}

package ticket_test

import (
	"testing"

	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
)

func TestNewServiceFromDB(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
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

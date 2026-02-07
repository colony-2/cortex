package store

import (
	"context"
	"testing"

	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
)

func TestStore_MigrateAndTx(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	s, err := NewWithOptions(pg.DB, Options{Migrate: true})
	if err != nil {
		t.Fatalf("NewWithOptions failed: %v", err)
	}

	ctx := context.Background()
	err = s.WithTx(ctx, func(ctx context.Context, s Store) error {
		// Ensure transaction is usable.
		return s.DB().WithContext(ctx).Exec("SELECT 1").Error
	})
	if err != nil {
		t.Fatalf("WithTx failed: %v", err)
	}
}

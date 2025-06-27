// Package storage provides storage implementations for vibethis.
package storage

import (
	"vibethis/core/pkg/core"
	"vibethis/storage/internal/bolt"
	"vibethis/storage/internal/memory"
)

// Config defines configuration options for storage backends.
type Config struct {
	// DatabasePath is the path to the database file (for BoltDB).
	DatabasePath string
	
	// ReadOnly indicates if the storage should be opened in read-only mode.
	ReadOnly bool
}

// NewBoltStorage creates a new BoltDB-backed storage implementation.
func NewBoltStorage(config Config) (core.Storage, error) {
	return bolt.New(config.DatabasePath, config.ReadOnly)
}

// NewMemoryStorage creates a new in-memory storage implementation.
// This is primarily useful for testing.
func NewMemoryStorage() core.Storage {
	return memory.New()
}
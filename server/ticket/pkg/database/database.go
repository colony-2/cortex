package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Config defines options for opening the ticket database.
type Config struct {
	// DSN overrides the environment variable when provided.
	DSN string
	// FallbackPath points to a SQLite database file when DSN is empty. When
	// left blank, the database runs purely in-memory.
	FallbackPath string
}

// Open returns a gorm.DB configured for ticket storage along with a cleanup
// function that releases the underlying connection pool.
func Open(cfg Config) (*gorm.DB, func() error, error) {
	if cfg.DSN == "" {
		cfg.DSN = os.Getenv("TICKET_DATABASE_DSN")
	}
	if cfg.DSN != "" {
		db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
		if err != nil {
			return nil, nil, fmt.Errorf("ticketdb: open postgres: %w", err)
		}
		return db, func() error { return closeSQL(db) }, nil
	}

	path := strings.TrimSpace(cfg.FallbackPath)
	if path == "" {
		path = "file::memory:?cache=shared"
	} else {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("ticketdb: create sqlite dir: %w", err)
		}
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("ticketdb: open sqlite: %w", err)
	}
	return db, func() error { return closeSQL(db) }, nil
}

func closeSQL(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

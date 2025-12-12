package database

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Config defines options for opening the project database.
type Config struct {
	// DSN overrides the environment variable when provided.
	DSN string
}

// Open returns a gorm.DB configured for project storage along with a cleanup
// function that releases the underlying connection pool.
func Open(cfg Config) (*gorm.DB, func() error, error) {
	if cfg.DSN == "" {
		cfg.DSN = os.Getenv("PROJECT_DATABASE_DSN")
	}
	if cfg.DSN == "" {
		return nil, nil, fmt.Errorf("projectdb: DSN is required")
	}
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("projectdb: open postgres: %w", err)
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

package model

import (
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"gorm.io/plugin/optimisticlock"
)

// ID is a unique identifier for a published recipe entry.
type ID string

// PublishedRecipe represents a recipe that has been published at a specific git commit.
// This is stored in the database as a lightweight index mapping recipe names to git commits.
type PublishedRecipe struct {
	ID        ID                     `gorm:"type:varchar(27);primaryKey"`
	Version   optimisticlock.Version `gorm:"column:version"`
	ProjectID project.ID             `gorm:"column:project_id;type:varchar(27);uniqueIndex:idx_project_name;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	Name      string                 `gorm:"column:name;not null;uniqueIndex:idx_project_name"`
	GitPath   string                 `gorm:"column:git_path;not null"`

	// Published commit information
	CommitHash string `gorm:"column:commit_hash;type:varchar(40);not null"`

	// Publishing metadata
	PublishedAt time.Time `gorm:"column:published_at;not null"`
	PublishedBy *string   `gorm:"column:published_by"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName specifies the table name for GORM.
func (p *PublishedRecipe) TableName() string {
	return "public.published_recipes"
}

// ShortIDGenerator generates unique short IDs (like KSUID).
type ShortIDGenerator interface {
	NewID() (string, error)
}

// SearchFilter controls published recipe listing.
type SearchFilter struct {
	IDs        []ID
	ProjectIDs []project.ID
	Names      []string       // Exact name matches
	NamePrefix string         // Hierarchical prefix filter
	Limit      int            // Pagination limit
	Offset     int            // Pagination offset
}

package model

import (
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"gorm.io/gorm"
)

// RecipeBlob is the content-addressable storage (CAS) table for recipe bytes.
// Digest is sha256(content) as raw 32 bytes.
type RecipeBlob struct {
	Digest          []byte `gorm:"column:digest;type:bytea;primaryKey"`
	Algo            string `gorm:"column:algo;type:text;not null;default:'sha256'"`
	Content         []byte `gorm:"column:content;type:bytea;not null"`
	ContentEncoding string `gorm:"column:content_encoding;type:text;not null;default:'identity'"`
	SizeBytes       int    `gorm:"column:size_bytes;not null"`
	CreatedAt       time.Time
}

func (RecipeBlob) TableName() string {
	return "public.recipe_blobs"
}

// RecipeRow is the recipe identity table.
type RecipeRow struct {
	ID        string     `gorm:"column:id;type:varchar(27);primaryKey"`
	ProjectID project.ID `gorm:"column:project_id;type:varchar(27);not null;uniqueIndex:idx_recipe_project_name,priority:1"`
	Name      string     `gorm:"column:name;type:text;not null;uniqueIndex:idx_recipe_project_name,priority:2"`

	LatestSavedEventID *string `gorm:"column:latest_saved_event_id;type:varchar(27)"`
	LatestSavedOrdinal *int64  `gorm:"column:latest_saved_ordinal"`

	// PublishedEventID points at the last publish-affecting event (publish or unpublish).
	PublishedEventID *string `gorm:"column:published_event_id;type:varchar(27)"`

	DeletedAt *time.Time `gorm:"column:deleted_at"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RecipeRow) TableName() string {
	return "public.recipes"
}

// RecipeEvent is the append-only event log for a recipe.
type RecipeEvent struct {
	ID        string     `gorm:"column:id;type:varchar(27);primaryKey"`
	ProjectID project.ID `gorm:"column:project_id;type:varchar(27);not null;index:idx_recipe_events_project_recipe_time,priority:1"`
	RecipeID  string     `gorm:"column:recipe_id;type:varchar(27);not null;index:idx_recipe_events_project_recipe_time,priority:2;index:idx_recipe_events_recipe_saved_ordinal,priority:1"`

	Digest []byte `gorm:"column:digest;type:bytea"`

	// SavedOrdinal is assigned only for save events.
	SavedOrdinal *int64 `gorm:"column:saved_ordinal;index:idx_recipe_events_recipe_saved_ordinal,priority:2"`
	// TargetSavedOrdinal is set for publish-only events to point to a previously saved version.
	TargetSavedOrdinal *int64 `gorm:"column:target_saved_ordinal"`

	Published bool `gorm:"column:published;not null"`

	EventAt time.Time `gorm:"column:event_at;not null;index:idx_recipe_events_project_recipe_time,priority:3"`
	Actor   *string   `gorm:"column:actor;type:text"`
	Message *string   `gorm:"column:message;type:text"`

	CreatedAt time.Time
}

func (RecipeEvent) TableName() string {
	return "public.recipe_events"
}

// NormalizeTimes ensures all timestamps are UTC.
func NormalizeTimes(t *time.Time) {
	if t == nil {
		return
	}
	*t = t.UTC()
}

func NormalizeModelTimes(db *gorm.DB) {
	_ = db
}

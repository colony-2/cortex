package model

import (
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"gorm.io/plugin/optimisticlock"
)

type ID string

// Cell represents a stored cell.
type Cell struct {
	ID            ID                     `gorm:"type:char(27);primaryKey"`
	Version       optimisticlock.Version `gorm:"column:version"`
	ProjectID     project.ID             `gorm:"column:project_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	Name          string                 `gorm:"column:name;not null"`
	Description   string                 `gorm:"column:description;default:''"`
	WorkingPath   string                 `gorm:"column:working_path;not null"`
	Populator     string                 `gorm:"column:populator;default:''"`
	PopulatorID   string                 `gorm:"column:populator_id;default:'';index"`
	GitRepoName   *string                `gorm:"column:git_repo_name"`
	GitBranch     *string                `gorm:"column:git_branch"`
	DefaultRecipe *string                `gorm:"column:default_recipe"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time `gorm:"column:deleted_at;index"`
}

// Dependency represents a cell-to-cell dependency edge.
type Dependency struct {
	ID         uint       `gorm:"primaryKey;autoIncrement"`
	ProjectID  project.ID `gorm:"column:project_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	FromCellID ID         `gorm:"column:from_cell_id;type:char(27);index;not null;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ToCellID   ID         `gorm:"column:to_cell_id;type:char(27);index;not null;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt  time.Time
}

func (d *Dependency) TableName() string {
	return "public.cell_dependencies"
}

type ShortIDGenerator interface {
	NewID() (string, error)
}

// SearchFilter controls cell listing.
type SearchFilter struct {
	IDs            []ID
	ProjectIDs     []project.ID
	Names          []string
	NameContains   string
	PathPrefix     string
	IncludeDeleted bool
	DependsOn      []ID // return cells that depend on any provided ID
	Populator      string
	PopulatorIDs   []string
}

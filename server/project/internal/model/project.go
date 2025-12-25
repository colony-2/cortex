package model

import (
	"time"

	"gorm.io/plugin/optimisticlock"
)

type ID string

type Project struct {
	ID                  ID                     `gorm:"type:char(27);primaryKey"`
	Version             optimisticlock.Version `gorm:"column:version"`
	Name                string                 `gorm:"uniqueIndex"`
	GitRepoPath         string                 `gorm:"column:git_repo_path"`
	DefaultTicketRecipe *string                `gorm:"column:default_ticket_recipe"`
	GitRepoBranch       *string                `gorm:"column:git_repo_branch"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ShortIDGenerator interface {
	NewID() (string, error)
}

type SearchFilter struct {
	IDs          []ID
	Names        []string
	NameContains string
}

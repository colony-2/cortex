package model

import (
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/project/pkg/project"
)

type SearchFilter struct {
	StageAny      []Stage
	StageNotIn    []Stage
	States        []State
	Actors        []ActorType
	Cells         []core.CellName
	Projects      []project.ID
	At            *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

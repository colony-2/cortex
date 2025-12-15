package model

import (
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
)

type SearchFilter struct {
	StageAny      []Stage
	StageNotIn    []Stage
	States        []State
	Actors        []ActorType
	Cells         []core.CellName
	CellIDs       []cell.ID
	Projects      []project.ID
	At            *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

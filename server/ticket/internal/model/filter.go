package model

import (
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
)

type SearchFilter struct {
	StageAny      []Stage
	StageNotIn    []Stage
	States        []State
	Actors        []ActorType
	Cells         []core.CellName
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

package service

import (
	"strings"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/model"
)

const fallbackTicketRecipe = "internal://new_ticket"

func defaultRecipeName(cellRecord *cell.Cell, projectRecord *project.Project) string {
	if cellRecord != nil && cellRecord.DefaultRecipe != nil && strings.TrimSpace(*cellRecord.DefaultRecipe) != "" {
		return strings.TrimSpace(*cellRecord.DefaultRecipe)
	}
	if projectRecord != nil && projectRecord.DefaultTicketRecipe != nil && strings.TrimSpace(*projectRecord.DefaultTicketRecipe) != "" {
		return strings.TrimSpace(*projectRecord.DefaultTicketRecipe)
	}
	return fallbackTicketRecipe
}

func actorEmail(actor model.Actor) string {
	if actor.Type == model.ActorTypeUser && actor.User != nil {
		return string(actor.User.Email)
	}
	return ""
}

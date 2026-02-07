package model

import (
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
)

// PublishedRecipe is a view of the currently published version for a recipe name.
// It is returned from PublishRecipe and can be derived from recipe_events + recipes pointers.
type PublishedRecipe struct {
	ProjectID project.ID
	Name      string

	// CommitHash is a stable reference string for the published saved version (e.g. "v12").
	// Field name preserved for API compatibility; this is no longer a git commit hash.
	CommitHash string

	PublishedAt time.Time
	PublishedBy *string
}

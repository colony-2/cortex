package service

import (
	"context"
	"fmt"

	"github.com/colony-2/c2j/pkg/recipe"
	"github.com/colony-2/colony2/server/recipes/internal/model"
)

// ValidateRecipe validates recipe content without creating or publishing.
func (s *service) ValidateRecipe(ctx context.Context, input model.ValidateInput) (*model.ValidationResult, error) {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	if err := validateRecipeName(input.Name); err != nil {
		return nil, err
	}
	return s.validateRecipe(ctx, input)
}

func (s *service) validateRecipe(ctx context.Context, input model.ValidateInput) (*model.ValidationResult, error) {
	result := &model.ValidationResult{Valid: true}

	rec, err := recipe.LoadRecipeFromString(input.Content)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, model.ValidationError{
			Code:    "schema",
			Message: err.Error(),
		})
		return result, &model.ValidationFailedError{Result: result}
	}

	if rec.GetMetadata().ID != input.Name {
		result.Valid = false
		result.Errors = append(result.Errors, model.ValidationError{
			Code:    "recipe_id_mismatch",
			Message: fmt.Sprintf("recipe ID %q does not match name %q", rec.GetMetadata().ID, input.Name),
			Path:    "id",
		})
	}

	if s.celValidator == nil {
		return nil, model.ErrValidationUnavailable
	}

	celErrors, err := s.celValidator.ValidateCEL(ctx, input.ProjectID, *rec)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrValidationUnavailable, err)
	}
	if len(celErrors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, celErrors...)
	}

	if !result.Valid {
		return result, &model.ValidationFailedError{Result: result}
	}

	return result, nil
}

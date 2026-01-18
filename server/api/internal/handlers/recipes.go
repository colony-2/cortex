package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// handleListRecipes implements GET /projects/{projectId}/recipes
func (h *Handlers) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])

	// Parse status filter
	statusParam := r.URL.Query().Get("status")
	var publishStatus recipesvc.PublishStatus
	switch statusParam {
	case "published":
		publishStatus = recipesvc.PublishStatusPublished
	case "unpublished":
		publishStatus = recipesvc.PublishStatusUnpublished
	default:
		publishStatus = recipesvc.PublishStatusAll
	}

	// List recipes
	iter, err := h.recipeSvc.ListRecipes(r.Context(), recipesvc.RecipeFilter{
		ProjectIDs:    []project.ID{projectID},
		PublishStatus: publishStatus,
	})
	if err != nil {
		writeRecipeError(w, err)
		return
	}
	defer iter.Close(r.Context())

	// Collect results
	recipes := []openapi.RecipeInfo{}
	for {
		info, err := iter.Next(r.Context())
		if err != nil {
			if errors.Is(err, recipesvc.ErrIteratorDone) {
				break
			}
			writeRecipeError(w, err)
			return
		}

		recipeInfo := openapi.RecipeInfo{
			Name:           info.Name,
			LatestCommit:   info.LatestCommit,
			LatestCommitAt: info.LatestCommitAt,
		}
		if info.PublishedCommit != nil {
			recipeInfo.PublishedCommit = info.PublishedCommit
			recipeInfo.PublishedAt = info.PublishedAt
			recipeInfo.PublishedBy = info.PublishedBy
		}

		recipes = append(recipes, recipeInfo)
	}

	writeJSON(w, http.StatusOK, openapi.RecipeListResponse{
		Recipes: recipes,
	})
}

// handleCreateRecipe implements POST /projects/{projectId}/recipes
func (h *Handlers) handleCreateRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])

	var req openapi.CreateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.Name == "" || req.Content == "" {
		writeError(w, fmt.Errorf("name and content are required"), http.StatusBadRequest)
		return
	}

	// Create recipe
	autoPublish := false
	if req.AutoPublish != nil {
		autoPublish = *req.AutoPublish
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	version, err := h.recipeSvc.CreateRecipe(r.Context(), recipesvc.CreateInput{
		ProjectID:   projectID,
		Name:        req.Name,
		Content:     []byte(req.Content),
		Description: description,
		AutoPublish: autoPublish,
	})
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, openapi.RecipeVersion{
		CommitHash:  version.CommitHash,
		ShortHash:   version.ShortHash,
		CreatedAt:   version.CreatedAt,
		Author:      version.Author,
		Message:     version.Message,
		IsPublished: version.IsPublished,
	})
}

// handleValidateRecipe implements POST /projects/{projectId}/recipes/validate
func (h *Handlers) handleValidateRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])

	var req openapi.ValidateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Content == "" {
		writeError(w, fmt.Errorf("name and content are required"), http.StatusBadRequest)
		return
	}

	result, err := h.recipeSvc.ValidateRecipe(r.Context(), recipesvc.ValidateInput{
		ProjectID: projectID,
		Name:      req.Name,
		Content:   []byte(req.Content),
	})
	if err != nil {
		if vErr, ok := err.(*recipesvc.ValidationFailedError); ok {
			writeJSON(w, http.StatusBadRequest, openapi.RecipeValidationResponse{
				Valid:  false,
				Errors: toOpenapiValidationErrors(vErr.Result.Errors),
			})
			return
		}
		writeRecipeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, openapi.RecipeValidationResponse{
		Valid:  result.Valid,
		Errors: toOpenapiValidationErrors(result.Errors),
	})
}

// handleGetRecipe implements GET /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleGetRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]
	ref := r.URL.Query().Get("ref")

	recipe, err := h.recipeSvc.GetRecipe(r.Context(), projectID, recipeName, ref)
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	// Use raw YAML content directly to preserve original formatting
	contentMap := make(map[string]interface{})
	rawYAML := ""

	if len(recipe.Content) > 0 {
		// Use raw bytes directly - preserves original formatting and field order
		rawYAML = string(recipe.Content)
		// Parse YAML to map for structured content (ignore errors - invalid recipes can be saved)
		_ = yaml.Unmarshal(recipe.Content, &contentMap)
	}

	// Convert to response format
	response := openapi.RecipeWithContent{
		Name:        recipe.Name,
		CommitHash:  recipe.CommitHash,
		IsPublished: recipe.IsPublished,
		Content:     contentMap,
		RawYaml:     rawYAML,
	}
	if recipe.PublishedAt != nil {
		response.PublishedAt = recipe.PublishedAt
	}
	if recipe.PublishedBy != nil {
		response.PublishedBy = recipe.PublishedBy
	}

	writeJSON(w, http.StatusOK, response)
}

// handleUpdateRecipe implements PUT /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]

	var req openapi.UpdateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		writeError(w, fmt.Errorf("content is required"), http.StatusBadRequest)
		return
	}

	expectedCommit := ""
	if req.ExpectedCommit != nil {
		expectedCommit = *req.ExpectedCommit
	}

	message := "Update recipe"
	if req.Message != nil {
		message = *req.Message
	}

	autoPublish := false
	if req.AutoPublish != nil {
		autoPublish = *req.AutoPublish
	}

	version, err := h.recipeSvc.UpdateRecipe(r.Context(), recipesvc.UpdateInput{
		ProjectID:      projectID,
		Name:           recipeName,
		Content:        []byte(req.Content),
		Message:        message,
		AutoPublish:    autoPublish,
		ExpectedCommit: expectedCommit,
	})
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, openapi.RecipeVersion{
		CommitHash:  version.CommitHash,
		ShortHash:   version.ShortHash,
		CreatedAt:   version.CreatedAt,
		Author:      version.Author,
		Message:     version.Message,
		IsPublished: version.IsPublished,
	})
}

// handleDeleteRecipe implements DELETE /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]

	err := h.recipeSvc.DeleteRecipe(r.Context(), projectID, recipeName)
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handlePublishRecipe implements POST /projects/{projectId}/recipes/{recipeName}/publish
func (h *Handlers) handlePublishRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]

	var req openapi.PublishRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Empty body is OK
		req = openapi.PublishRecipeRequest{}
	}

	commitHash := ""
	if req.CommitHash != nil {
		commitHash = *req.CommitHash
	}

	var publishedBy *string
	if req.PublishedBy != nil {
		publishedBy = req.PublishedBy
	}

	var expectedCommit *string
	if req.ExpectedCommit != nil {
		expectedCommit = req.ExpectedCommit
	}

	published, err := h.recipeSvc.PublishRecipe(r.Context(), recipesvc.PublishInput{
		ProjectID:      projectID,
		Name:           recipeName,
		CommitHash:     commitHash,
		PublishedBy:    publishedBy,
		ExpectedCommit: expectedCommit,
	})
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	response := openapi.PublishedRecipe{
		Name:            published.Name,
		PublishedCommit: published.CommitHash,
		PublishedAt:     published.PublishedAt,
	}
	if published.PublishedBy != nil {
		response.PublishedBy = published.PublishedBy
	}

	writeJSON(w, http.StatusOK, response)
}

// handleUnpublishRecipe implements POST /projects/{projectId}/recipes/{recipeName}/unpublish
func (h *Handlers) handleUnpublishRecipe(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]

	err := h.recipeSvc.UnpublishRecipe(r.Context(), recipesvc.UnpublishInput{
		ProjectID: projectID,
		Name:      recipeName,
	})
	if err != nil {
		writeRecipeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetRecipeHistory implements GET /projects/{projectId}/recipes/{recipeName}/history
func (h *Handlers) handleGetRecipeHistory(w http.ResponseWriter, r *http.Request) {
	projectID := project.ID(mux.Vars(r)["projectId"])
	recipeName := mux.Vars(r)["recipeName"]

	iter, err := h.recipeSvc.GetRecipeHistory(r.Context(), projectID, recipeName)
	if err != nil {
		writeRecipeError(w, err)
		return
	}
	defer iter.Close(r.Context())

	versions := []openapi.RecipeVersion{}
	for {
		version, err := iter.Next(r.Context())
		if err != nil {
			if errors.Is(err, recipesvc.ErrIteratorDone) {
				break
			}
			writeRecipeError(w, err)
			return
		}

		versions = append(versions, openapi.RecipeVersion{
			CommitHash:  version.CommitHash,
			ShortHash:   version.ShortHash,
			CreatedAt:   version.CreatedAt,
			Author:      version.Author,
			Message:     version.Message,
			IsPublished: version.IsPublished,
		})
	}

	writeJSON(w, http.StatusOK, openapi.RecipeHistoryResponse{
		Versions: versions,
	})
}

// writeRecipeError handles recipe service errors with appropriate HTTP status codes
func writeRecipeError(w http.ResponseWriter, err error) {
	var vErr *recipesvc.ValidationFailedError
	if errors.As(err, &vErr) {
		if vErr != nil && vErr.Result != nil {
			writeJSON(w, http.StatusBadRequest, openapi.RecipeValidationResponse{
				Valid:  false,
				Errors: toOpenapiValidationErrors(vErr.Result.Errors),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, openapi.RecipeValidationResponse{Valid: false})
		return
	}

	switch {
	case errors.Is(err, recipesvc.ErrNotFound):
		writeError(w, err, http.StatusNotFound)
	case errors.Is(err, recipesvc.ErrNotPublished):
		writeError(w, fmt.Errorf("recipe not published"), http.StatusConflict)
	case errors.Is(err, recipesvc.ErrVersionConflict):
		writeError(w, fmt.Errorf("version conflict"), http.StatusConflict)
	case errors.Is(err, recipesvc.ErrInvalidContent):
		writeError(w, err, http.StatusBadRequest)
	case errors.Is(err, recipesvc.ErrAlreadyExists):
		writeError(w, err, http.StatusConflict)
	case errors.Is(err, recipesvc.ErrValidationUnavailable):
		writeError(w, err, http.StatusServiceUnavailable)
	default:
		writeError(w, err, http.StatusInternalServerError)
	}
}

func toOpenapiValidationErrors(errs []recipesvc.ValidationError) []openapi.ValidationError {
	out := make([]openapi.ValidationError, 0, len(errs))
	for _, err := range errs {
		path := stringPtrIfNotEmpty(err.Path)
		expr := stringPtrIfNotEmpty(err.Expression)
		out = append(out, openapi.ValidationError{
			Code:       err.Code,
			Message:    err.Message,
			Path:       path,
			Expression: expr,
		})
	}
	return out
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

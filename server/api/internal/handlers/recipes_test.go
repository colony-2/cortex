package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

type fakeRecipeService struct {
	listReq   *recipesvc.RecipeFilter
	createReq *recipesvc.CreateInput
	getReq    *struct {
		projectID project.ID
		name, ref string
	}
	updateReq *recipesvc.UpdateInput
	deleteReq *struct {
		projectID project.ID
		name      string
	}
	publishReq   *recipesvc.PublishInput
	unpublishReq *recipesvc.UnpublishInput
	historyReq   *struct {
		projectID project.ID
		name      string
	}
	validateReq *recipesvc.ValidateInput

	listResp     []*recipesvc.RecipeInfo
	createResp   *recipesvc.RecipeVersion
	getResp      *recipesvc.RecipeWithContent
	updateResp   *recipesvc.RecipeVersion
	publishResp  *recipesvc.PublishedRecipe
	historyResp  []*recipesvc.RecipeVersion
	validateResp *recipesvc.ValidationResult

	listErr      error
	createErr    error
	getErr       error
	updateErr    error
	deleteErr    error
	publishErr   error
	unpublishErr error
	historyErr   error
	validateErr  error
}

func (f *fakeRecipeService) ListRecipes(ctx context.Context, filter recipesvc.RecipeFilter) (recipesvc.Iterator[*recipesvc.RecipeInfo], error) {
	f.listReq = &filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &fakeIterator[*recipesvc.RecipeInfo]{items: f.listResp}, nil
}

func (f *fakeRecipeService) CreateRecipe(ctx context.Context, input recipesvc.CreateInput) (*recipesvc.RecipeVersion, error) {
	f.createReq = &input
	return f.createResp, f.createErr
}

func (f *fakeRecipeService) GetRecipe(ctx context.Context, projectID project.ID, name, ref string) (*recipesvc.RecipeWithContent, error) {
	f.getReq = &struct {
		projectID project.ID
		name, ref string
	}{projectID, name, ref}
	return f.getResp, f.getErr
}

func (f *fakeRecipeService) UpdateRecipe(ctx context.Context, input recipesvc.UpdateInput) (*recipesvc.RecipeVersion, error) {
	f.updateReq = &input
	return f.updateResp, f.updateErr
}

func (f *fakeRecipeService) DeleteRecipe(ctx context.Context, projectID project.ID, name string) error {
	f.deleteReq = &struct {
		projectID project.ID
		name      string
	}{projectID, name}
	return f.deleteErr
}

func (f *fakeRecipeService) PublishRecipe(ctx context.Context, input recipesvc.PublishInput) (*recipesvc.PublishedRecipe, error) {
	f.publishReq = &input
	return f.publishResp, f.publishErr
}

func (f *fakeRecipeService) UnpublishRecipe(ctx context.Context, input recipesvc.UnpublishInput) error {
	f.unpublishReq = &input
	return f.unpublishErr
}

func (f *fakeRecipeService) GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (recipesvc.Iterator[*recipesvc.RecipeVersion], error) {
	f.historyReq = &struct {
		projectID project.ID
		name      string
	}{projectID, name}
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return &fakeIterator[*recipesvc.RecipeVersion]{items: f.historyResp}, nil
}

// Additional required methods for the Service interface
func (f *fakeRecipeService) ValidateRecipe(ctx context.Context, input recipesvc.ValidateInput) (*recipesvc.ValidationResult, error) {
	f.validateReq = &input
	return f.validateResp, f.validateErr
}

func (f *fakeRecipeService) SyncFromRemote(ctx context.Context, projectID project.ID) error {
	return nil
}

// fakeIterator implements Iterator for testing
type fakeIterator[T any] struct {
	items []T
	index int
}

func (f *fakeIterator[T]) Next(ctx context.Context) (T, error) {
	if f.index >= len(f.items) {
		var zero T
		return zero, recipesvc.ErrIteratorDone
	}
	item := f.items[f.index]
	f.index++
	return item, nil
}

func (f *fakeIterator[T]) Close(ctx context.Context) error {
	return nil
}

func TestHandleListRecipes_OK(t *testing.T) {
	projectID := "proj_123"
	now := time.Now().UTC()

	publishedCommit := "abc123"
	publishedAt := now
	publishedBy := "user@example.com"

	fake := &fakeRecipeService{
		listResp: []*recipesvc.RecipeInfo{
			{
				Name:            "workflows/build",
				LatestCommit:    "abc123",
				LatestCommitAt:  now,
				PublishedCommit: &publishedCommit,
				PublishedAt:     &publishedAt,
				PublishedBy:     &publishedBy,
			},
			{
				Name:            "ops/deploy",
				LatestCommit:    "def456",
				LatestCommitAt:  now,
				PublishedCommit: nil,
				PublishedAt:     nil,
				PublishedBy:     nil,
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes?status=all", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.RecipeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Recipes) != 2 {
		t.Fatalf("expected 2 recipes, got %d", len(body.Recipes))
	}
	if fake.listReq == nil {
		t.Fatalf("expected list request to be captured")
	}
	if len(fake.listReq.ProjectIDs) != 1 || string(fake.listReq.ProjectIDs[0]) != projectID {
		t.Fatalf("expected list request captured with project %q", projectID)
	}
	if fake.listReq.PublishStatus != "all" {
		t.Fatalf("expected status filter 'all', got %q", fake.listReq.PublishStatus)
	}
}

func TestHandleListRecipes_FilterPublished(t *testing.T) {
	projectID := "proj_123"
	publishedCommit := "abc123"

	fake := &fakeRecipeService{
		listResp: []*recipesvc.RecipeInfo{
			{
				Name:            "published-recipe",
				LatestCommit:    "abc123",
				LatestCommitAt:  time.Now().UTC(),
				PublishedCommit: &publishedCommit,
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes?status=published", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if fake.listReq == nil || fake.listReq.PublishStatus != "published" {
		t.Fatalf("expected status filter 'published', got %q", fake.listReq.PublishStatus)
	}
}

func TestHandleValidateRecipe_OK(t *testing.T) {
	projectID := "proj_123"

	fake := &fakeRecipeService{
		validateResp: &recipesvc.ValidationResult{Valid: true},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body, err := json.Marshal(openapi.ValidateRecipeRequest{
		Name:    "workflows/ci/build",
		Content: "version: \"1.0\"\nid: workflows/ci/build\nop: echo\ninputs:\n  message: \"hi\"\n",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes/validate", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var bodyResp openapi.RecipeValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&bodyResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bodyResp.Valid {
		t.Fatalf("expected valid response, got %+v", bodyResp)
	}

	if fake.validateReq == nil {
		t.Fatalf("expected validate request to be captured")
	}
	if fake.validateReq.ProjectID != project.ID(projectID) {
		t.Fatalf("expected project ID %q, got %q", projectID, fake.validateReq.ProjectID)
	}
}

func TestHandleValidateRecipe_Invalid(t *testing.T) {
	projectID := "proj_123"
	fake := &fakeRecipeService{
		validateErr: &recipesvc.ValidationFailedError{
			Result: &recipesvc.ValidationResult{
				Valid: false,
				Errors: []recipesvc.ValidationError{
					{
						Code:    "cel_invalid",
						Message: "unknown identifier",
						Path:    "steps[0].when",
					},
				},
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body, err := json.Marshal(openapi.ValidateRecipeRequest{
		Name:    "workflows/ci/build",
		Content: "version: \"1.0\"\nid: workflows/ci/build\nop: echo\ninputs:\n  message: \"hi\"\n",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes/validate", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var bodyResp openapi.RecipeValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&bodyResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if bodyResp.Valid {
		t.Fatalf("expected invalid response, got %+v", bodyResp)
	}
	if len(bodyResp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(bodyResp.Errors))
	}
	if bodyResp.Errors[0].Code != "cel_invalid" {
		t.Fatalf("expected error code cel_invalid, got %q", bodyResp.Errors[0].Code)
	}
}

func TestHandleCreateRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	now := time.Now().UTC()

	fake := &fakeRecipeService{
		createResp: &recipesvc.RecipeVersion{
			Name:        "new-recipe",
			CommitHash:  "abc123",
			ShortHash:   "abc123",
			Author:      "user@example.com",
			Message:     "Create recipe",
			CreatedAt:   now,
			IsPublished: true,
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := openapi.CreateRecipeRequest{
		Name:        "new-recipe",
		Content:     "version: \"1.0\"\nid: new-recipe\nop: echo",
		AutoPublish: boolPtr(true),
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var respBody openapi.RecipeVersion
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody.CommitHash != "abc123" {
		t.Fatalf("expected commit hash 'abc123', got %q", respBody.CommitHash)
	}
	if !respBody.IsPublished {
		t.Fatalf("expected recipe to be published")
	}
	if fake.createReq == nil || string(fake.createReq.ProjectID) != projectID {
		t.Fatalf("expected create request captured with project %q", projectID)
	}
	if fake.createReq.Name != "new-recipe" {
		t.Fatalf("expected name 'new-recipe', got %q", fake.createReq.Name)
	}
	if !fake.createReq.AutoPublish {
		t.Fatalf("expected autoPublish true")
	}
}

func TestHandleCreateRecipe_ValidationError(t *testing.T) {
	projectID := "proj_123"

	fake := &fakeRecipeService{
		createErr: recipesvc.ErrInvalidContent,
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := openapi.CreateRecipeRequest{
		Name:    "bad-recipe",
		Content: "invalid yaml: [[[",
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleGetRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "workflows/build"

	fake := &fakeRecipeService{
		getResp: &recipesvc.RecipeWithContent{
			Name:        recipeName,
			CommitHash:  "abc123",
			IsPublished: true,
			Content:     nil, // Content not needed for handler test
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.RecipeWithContent
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != recipeName {
		t.Fatalf("expected name %q, got %q", recipeName, body.Name)
	}
	if fake.getReq == nil || string(fake.getReq.projectID) != projectID {
		t.Fatalf("expected get request captured with project %q", projectID)
	}
	if fake.getReq.name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.getReq.name)
	}
}

func TestHandleGetRecipe_WithRef(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"
	ref := "def456"

	fake := &fakeRecipeService{
		getResp: &recipesvc.RecipeWithContent{
			Name:       recipeName,
			CommitHash: ref,
			Content:    nil, // Content not needed for handler test
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"?ref="+ref, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if fake.getReq == nil || fake.getReq.ref != ref {
		t.Fatalf("expected ref %q, got %q", ref, fake.getReq.ref)
	}
}

func TestHandleGetRecipe_NotFound(t *testing.T) {
	projectID := "proj_123"
	recipeName := "nonexistent"

	fake := &fakeRecipeService{
		getErr: recipesvc.ErrNotFound,
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleUpdateRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"
	now := time.Now().UTC()

	fake := &fakeRecipeService{
		updateResp: &recipesvc.RecipeVersion{
			Name:        recipeName,
			CommitHash:  "def456",
			ShortHash:   "def456",
			Author:      "user@example.com",
			Message:     "Updated recipe",
			CreatedAt:   now,
			IsPublished: true,
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := openapi.UpdateRecipeRequest{
		Content:     "version: \"1.0\"\nid: test\nop: echo",
		Message:     strPtr("Updated recipe"),
		AutoPublish: boolPtr(true),
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var respBody openapi.RecipeVersion
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody.CommitHash != "def456" {
		t.Fatalf("expected commit hash 'def456', got %q", respBody.CommitHash)
	}
	if !respBody.IsPublished {
		t.Fatalf("expected recipe to be published")
	}

	if fake.updateReq == nil || string(fake.updateReq.ProjectID) != projectID {
		t.Fatalf("expected update request captured with project %q", projectID)
	}
	if fake.updateReq.Name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.updateReq.Name)
	}
	if fake.updateReq.Message != "Updated recipe" {
		t.Fatalf("expected message 'Updated recipe', got %q", fake.updateReq.Message)
	}
	if !fake.updateReq.AutoPublish {
		t.Fatalf("expected autoPublish true")
	}
}

func TestHandleDeleteRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"

	fake := &fakeRecipeService{}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if fake.deleteReq == nil || string(fake.deleteReq.projectID) != projectID {
		t.Fatalf("expected delete request captured with project %q", projectID)
	}
	if fake.deleteReq.name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.deleteReq.name)
	}
}

func TestHandleDeleteRecipe_NotFound(t *testing.T) {
	projectID := "proj_123"
	recipeName := "nonexistent"

	fake := &fakeRecipeService{
		deleteErr: recipesvc.ErrNotFound,
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlePublishRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"

	fake := &fakeRecipeService{
		publishResp: &recipesvc.PublishedRecipe{
			Name:       recipeName,
			CommitHash: "abc123",
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := openapi.PublishRecipeRequest{}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/publish", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var respBody openapi.PublishedRecipe
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody.Name != recipeName {
		t.Fatalf("expected name %q, got %q", recipeName, respBody.Name)
	}
	if fake.publishReq == nil || string(fake.publishReq.ProjectID) != projectID {
		t.Fatalf("expected publish request captured with project %q", projectID)
	}
	if fake.publishReq.Name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.publishReq.Name)
	}
}

func TestHandlePublishRecipe_AlreadyPublished(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"

	fake := &fakeRecipeService{
		publishErr: recipesvc.ErrAlreadyExists,
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := openapi.PublishRecipeRequest{}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/publish", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestHandleUnpublishRecipe_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"

	fake := &fakeRecipeService{}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/unpublish", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if fake.unpublishReq == nil || string(fake.unpublishReq.ProjectID) != projectID {
		t.Fatalf("expected unpublish request captured with project %q", projectID)
	}
	if fake.unpublishReq.Name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.unpublishReq.Name)
	}
}

func TestHandleGetRecipeHistory_OK(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"
	now := time.Now().UTC()

	fake := &fakeRecipeService{
		historyResp: []*recipesvc.RecipeVersion{
			{
				Name:        recipeName,
				CommitHash:  "abc123",
				ShortHash:   "abc123",
				Message:     "Initial version",
				Author:      "user@example.com",
				CreatedAt:   now,
				IsPublished: true,
			},
			{
				Name:        recipeName,
				CommitHash:  "def456",
				ShortHash:   "def456",
				Message:     "Updated recipe",
				Author:      "admin@example.com",
				CreatedAt:   now.Add(-time.Hour),
				IsPublished: false,
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/history", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.RecipeHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(body.Versions))
	}
	if fake.historyReq == nil || string(fake.historyReq.projectID) != projectID {
		t.Fatalf("expected history request captured with project %q", projectID)
	}
	if fake.historyReq.name != recipeName {
		t.Fatalf("expected recipe name %q, got %q", recipeName, fake.historyReq.name)
	}
}

func TestHandleGetRecipeHistory_NotFound(t *testing.T) {
	projectID := "proj_123"
	recipeName := "nonexistent"

	fake := &fakeRecipeService{
		historyErr: recipesvc.ErrNotFound,
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/history", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleListRecipes_ServiceError(t *testing.T) {
	projectID := "proj_123"

	fake := &fakeRecipeService{
		listErr: errors.New("database connection failed"),
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// Test for iterator done error handling - regression test for store.ErrIteratorDone
func TestHandleListRecipes_StoreIteratorDone(t *testing.T) {
	projectID := "proj_123"

	// Create a fake service that returns an iterator with the store error
	fake := &fakeRecipeService{
		listResp: []*recipesvc.RecipeInfo{
			{
				Name:           "test-recipe",
				LatestCommit:   "abc123",
				LatestCommitAt: time.Now().UTC(),
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.RecipeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(body.Recipes))
	}
}

// Test for iterator done error handling in history - regression test for store.ErrIteratorDone
func TestHandleGetRecipeHistory_StoreIteratorDone(t *testing.T) {
	projectID := "proj_123"
	recipeName := "test-recipe"
	now := time.Now().UTC()

	fake := &fakeRecipeService{
		historyResp: []*recipesvc.RecipeVersion{
			{
				Name:        recipeName,
				CommitHash:  "abc123",
				ShortHash:   "abc123",
				Message:     "Initial commit",
				Author:      "user@example.com",
				CreatedAt:   now,
				IsPublished: true,
			},
		},
	}

	h := &Handlers{recipeSvc: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes/"+recipeName+"/history", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.RecipeHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(body.Versions))
	}
}

// storeErrIterator returns the actual store error for testing
type storeErrIterator[T any] struct {
	items []T
	index int
}

func (f *storeErrIterator[T]) Next(ctx context.Context) (T, error) {
	if f.index >= len(f.items) {
		var zero T
		// Return the actual store error that would be returned in production
		return zero, recipesvc.ErrIteratorDone
	}
	item := f.items[f.index]
	f.index++
	return item, nil
}

func (f *storeErrIterator[T]) Close(ctx context.Context) error {
	return nil
}

// Test with actual store iterator error to ensure backward compatibility
func TestHandleListRecipes_WithActualStoreError(t *testing.T) {
	projectID := "proj_123"

	// Override the fake service to return our custom iterator
	recipes := []*recipesvc.RecipeInfo{
		{
			Name:           "test-recipe",
			LatestCommit:   "abc123",
			LatestCommitAt: time.Now().UTC(),
		},
	}

	h := &Handlers{
		recipeSvc: &customIteratorRecipeService{
			listIter: &storeErrIterator[*recipesvc.RecipeInfo]{items: recipes},
		},
	}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/recipes", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	// The key test: this should return 200, not 500
	if resp.StatusCode != http.StatusOK {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("expected 200, got %d. Error: %v", resp.StatusCode, errBody)
	}

	var body openapi.RecipeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(body.Recipes))
	}
}

// customIteratorRecipeService is a minimal fake that returns our custom iterator
type customIteratorRecipeService struct {
	listIter    recipesvc.Iterator[*recipesvc.RecipeInfo]
	historyIter recipesvc.Iterator[*recipesvc.RecipeVersion]
}

func (c *customIteratorRecipeService) ListRecipes(ctx context.Context, filter recipesvc.RecipeFilter) (recipesvc.Iterator[*recipesvc.RecipeInfo], error) {
	return c.listIter, nil
}

func (c *customIteratorRecipeService) GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (recipesvc.Iterator[*recipesvc.RecipeVersion], error) {
	return c.historyIter, nil
}

func (c *customIteratorRecipeService) CreateRecipe(ctx context.Context, input recipesvc.CreateInput) (*recipesvc.RecipeVersion, error) {
	return nil, nil
}

func (c *customIteratorRecipeService) GetRecipe(ctx context.Context, projectID project.ID, name, ref string) (*recipesvc.RecipeWithContent, error) {
	return nil, nil
}

func (c *customIteratorRecipeService) UpdateRecipe(ctx context.Context, input recipesvc.UpdateInput) (*recipesvc.RecipeVersion, error) {
	return nil, nil
}

func (c *customIteratorRecipeService) DeleteRecipe(ctx context.Context, projectID project.ID, name string) error {
	return nil
}

func (c *customIteratorRecipeService) PublishRecipe(ctx context.Context, input recipesvc.PublishInput) (*recipesvc.PublishedRecipe, error) {
	return nil, nil
}

func (c *customIteratorRecipeService) UnpublishRecipe(ctx context.Context, input recipesvc.UnpublishInput) error {
	return nil
}

func (c *customIteratorRecipeService) ValidateRecipe(ctx context.Context, input recipesvc.ValidateInput) (*recipesvc.ValidationResult, error) {
	return &recipesvc.ValidationResult{Valid: true}, nil
}

func (c *customIteratorRecipeService) SyncFromRemote(ctx context.Context, projectID project.ID) error {
	return nil
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func strPtr(s string) *string {
	return &s
}

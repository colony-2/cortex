package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecipeIntegration_FullWorkflow(t *testing.T) {
	// SQLite in-memory DB for testing
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Setup project service
	projectStore, err := project.NewStore(db)
	if err != nil {
		t.Fatalf("project store: %v", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		t.Fatalf("project service: %v", err)
	}

	// Create a test project
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-proj",
		GitRepoPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projID := string(proj.ID)

	// Setup recipe service (using fake for this test)
	recipeSvc := &fakeRecipeService{
		listResp: []*recipesvc.RecipeInfo{},
	}

	// Setup handlers and server
	h := &Handlers{recipeSvc: recipeSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Test 1: List recipes (should be empty)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes?status=all", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("list recipes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var listResp openapi.RecipeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Recipes) != 0 {
		t.Fatalf("expected 0 recipes, got %d", len(listResp.Recipes))
	}

	// Test 2: Create new recipe
	now := time.Now().UTC()
	recipeSvc.createResp = &recipesvc.RecipeVersion{
		Name:        "workflows/build",
		CommitHash:  "abc123",
		ShortHash:   "abc123",
		Message:     "Create recipe",
		Author:      "user@example.com",
		CreatedAt:   now,
		IsPublished: true,
	}

	createReqBody := openapi.CreateRecipeRequest{
		Name:        "workflows/build",
		Content:     "version: \"1.0\"\nid: build\nop: echo\ninputs:\n  message: \"Building...\"",
		AutoPublish: boolPtr(true),
	}
	body, _ := json.Marshal(createReqBody)
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/projects/"+projID+"/recipes", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp2.StatusCode)
	}

	var createResp openapi.RecipeVersion
	if err := json.NewDecoder(resp2.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if createResp.CommitHash != "abc123" {
		t.Fatalf("expected commit hash abc123, got %s", createResp.CommitHash)
	}
	if !createResp.IsPublished {
		t.Fatalf("expected recipe to be published")
	}

	// Verify the create request was captured with correct parameters
	if recipeSvc.createReq == nil {
		t.Fatalf("expected create request to be captured")
	}
	if recipeSvc.createReq.Name != "workflows/build" {
		t.Fatalf("expected name workflows/build, got %s", recipeSvc.createReq.Name)
	}
	if !recipeSvc.createReq.AutoPublish {
		t.Fatalf("expected autoPublish true")
	}

	// Test 3: Get recipe
	recipeSvc.getResp = &recipesvc.RecipeWithContent{
		Name:        "workflows/build",
		CommitHash:  "abc123",
		IsPublished: true,
		PublishedAt: &now,
		PublishedBy: strPtr("user@example.com"),
		Content:     nil, // Content not needed for handler test
	}

	req3, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp3, err := srv.Client().Do(req3)
	if err != nil {
		t.Fatalf("get recipe: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}

	var getResp openapi.RecipeWithContent
	if err := json.NewDecoder(resp3.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if getResp.Name != "workflows/build" {
		t.Fatalf("expected name workflows/build, got %s", getResp.Name)
	}
	if !getResp.IsPublished {
		t.Fatalf("expected recipe to be published")
	}

	// Test 4: Update recipe
	recipeSvc.updateResp = &recipesvc.RecipeVersion{
		Name:        "workflows/build",
		CommitHash:  "def456",
		ShortHash:   "def456",
		Message:     "Updated recipe",
		Author:      "user@example.com",
		CreatedAt:   now.Add(time.Minute),
		IsPublished: true,
	}

	updateReqBody := openapi.UpdateRecipeRequest{
		Content:     "version: \"1.0\"\nid: build\nop: echo\ninputs:\n  message: \"Updated build\"",
		Message:     strPtr("Updated recipe"),
		AutoPublish: boolPtr(true),
	}
	body4, _ := json.Marshal(updateReqBody)
	req4, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build", bytes.NewReader(body4))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req4.Header.Set("Content-Type", "application/json")

	resp4, err := srv.Client().Do(req4)
	if err != nil {
		t.Fatalf("update recipe: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp4.StatusCode)
	}

	var updateResp openapi.RecipeVersion
	if err := json.NewDecoder(resp4.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updateResp.CommitHash != "def456" {
		t.Fatalf("expected commit hash def456, got %s", updateResp.CommitHash)
	}

	// Test 5: Get recipe history
	recipeSvc.historyResp = []*recipesvc.RecipeVersion{
		{
			Name:        "workflows/build",
			CommitHash:  "def456",
			ShortHash:   "def456",
			Message:     "Updated recipe",
			Author:      "user@example.com",
			CreatedAt:   now.Add(time.Minute),
			IsPublished: true,
		},
		{
			Name:        "workflows/build",
			CommitHash:  "abc123",
			ShortHash:   "abc123",
			Message:     "Create recipe",
			Author:      "user@example.com",
			CreatedAt:   now,
			IsPublished: false,
		},
	}

	req5, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build/history", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp5, err := srv.Client().Do(req5)
	if err != nil {
		t.Fatalf("get recipe history: %v", err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp5.StatusCode)
	}

	var historyResp openapi.RecipeHistoryResponse
	if err := json.NewDecoder(resp5.Body).Decode(&historyResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(historyResp.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(historyResp.Versions))
	}

	// Test 6: Unpublish recipe
	recipeSvc.unpublishErr = nil

	req6, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build/unpublish", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp6, err := srv.Client().Do(req6)
	if err != nil {
		t.Fatalf("unpublish recipe: %v", err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp6.StatusCode)
	}

	// Test 7: Publish recipe again
	recipeSvc.publishResp = &recipesvc.PublishedRecipe{
		Name:       "workflows/build",
		CommitHash: "def456",
	}

	publishReqBody := openapi.PublishRecipeRequest{}
	body7, _ := json.Marshal(publishReqBody)
	req7, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build/publish", bytes.NewReader(body7))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req7.Header.Set("Content-Type", "application/json")

	resp7, err := srv.Client().Do(req7)
	if err != nil {
		t.Fatalf("publish recipe: %v", err)
	}
	defer resp7.Body.Close()
	if resp7.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp7.StatusCode)
	}

	var publishResp openapi.PublishedRecipe
	if err := json.NewDecoder(resp7.Body).Decode(&publishResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if publishResp.Name != "workflows/build" {
		t.Fatalf("expected name workflows/build, got %s", publishResp.Name)
	}

	// Test 8: Delete recipe
	recipeSvc.deleteErr = nil

	req8, err := http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL+"/api/projects/"+projID+"/recipes/workflows/build", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp8, err := srv.Client().Do(req8)
	if err != nil {
		t.Fatalf("delete recipe: %v", err)
	}
	defer resp8.Body.Close()
	if resp8.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp8.StatusCode)
	}

	// Verify the delete request was captured
	if recipeSvc.deleteReq == nil {
		t.Fatalf("expected delete request to be captured")
	}
	if recipeSvc.deleteReq.name != "workflows/build" {
		t.Fatalf("expected recipe name workflows/build, got %s", recipeSvc.deleteReq.name)
	}
}

func TestRecipeIntegration_ErrorHandling(t *testing.T) {
	// SQLite in-memory DB for testing
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Setup project service
	projectStore, err := project.NewStore(db)
	if err != nil {
		t.Fatalf("project store: %v", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		t.Fatalf("project service: %v", err)
	}

	// Create a test project
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-proj",
		GitRepoPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projID := string(proj.ID)

	// Test 1: Create recipe with validation error
	recipeSvc := &fakeRecipeService{
		createErr: recipesvc.ErrInvalidContent,
	}

	h := &Handlers{recipeSvc: recipeSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	createReqBody := openapi.CreateRecipeRequest{
		Name:    "bad-recipe",
		Content: "invalid yaml: [[[",
	}
	body, _ := json.Marshal(createReqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/projects/"+projID+"/recipes", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	// Test 2: Get non-existent recipe
	recipeSvc.getErr = recipesvc.ErrNotFound

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes/nonexistent", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatalf("get recipe: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp2.StatusCode)
	}

	// Test 3: Update non-existent recipe
	recipeSvc.updateErr = recipesvc.ErrNotFound

	updateReqBody := openapi.UpdateRecipeRequest{
		Content: "version: \"1.0\"",
	}
	body3, _ := json.Marshal(updateReqBody)
	req3, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/api/projects/"+projID+"/recipes/nonexistent", bytes.NewReader(body3))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req3.Header.Set("Content-Type", "application/json")

	resp3, err := srv.Client().Do(req3)
	if err != nil {
		t.Fatalf("update recipe: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp3.StatusCode)
	}

	// Test 4: Delete non-existent recipe
	recipeSvc.deleteErr = recipesvc.ErrNotFound

	req4, err := http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL+"/api/projects/"+projID+"/recipes/nonexistent", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp4, err := srv.Client().Do(req4)
	if err != nil {
		t.Fatalf("delete recipe: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp4.StatusCode)
	}
}

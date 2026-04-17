package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/c2j/pkg/contextual"
	"github.com/colony-2/c2j/pkg/template/funcregistry"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

func TestHandleRecipeTestCaseValidateInlineRecipe(t *testing.T) {
	h := &Handlers{recipeSvc: &fakeRecipeService{}}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: hi\n    type: short_answer\n",
		},
		"case": map[string]interface{}{
			"id":   "c1",
			"type": "recipe_case",
		},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/validate", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if valid, _ := out["valid"].(bool); !valid {
		t.Fatalf("expected valid=true, got %#v", out)
	}
	if hash, _ := out["case_hash"].(string); hash == "" {
		t.Fatalf("expected case hash")
	}
}

func TestHandleRecipeTestCaseExecute_IsolatedBlocksUnmockedTicketManage(t *testing.T) {
	h := &Handlers{recipeSvc: &fakeRecipeService{}}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: test_unmocked_op\ninputs:\n  foo: bar\n",
		},
		"case": map[string]interface{}{
			"id":   "c2",
			"type": "recipe_case",
		},
		"execution": map[string]interface{}{"mode": "isolated"},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status, _ := out["status"].(string); status != "failed" {
		t.Fatalf("expected failed status, got %q", status)
	}
	if cat, _ := out["failure_category"].(string); cat != "policy_blocked" {
		t.Fatalf("expected policy_blocked, got %q", cat)
	}
}

func TestHandleRecipeTestCaseExecute_MockedInputAndArtifactLimit(t *testing.T) {
	h := &Handlers{recipeSvc: &fakeRecipeService{}}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: hi\n    type: short_answer\n",
		},
		"case": map[string]interface{}{
			"id":   "c3",
			"type": "recipe_case",
			"mocks": map[string]interface{}{
				"ops": []map[string]interface{}{
					{
						"match": map[string]interface{}{"op": "input"},
						"behavior": map[string]interface{}{
							"mode":      "return",
							"outputs":   map[string]interface{}{"response": "ok"},
							"artifacts": map[string]interface{}{"report.txt": "1234567890"},
						},
					},
				},
			},
			"assertions": []map[string]interface{}{
				{"type": "output_equals", "path": "response", "value": "ok"},
				{"type": "artifact_exists", "path": "report.txt"},
			},
		},
		"execution": map[string]interface{}{"mode": "isolated", "artifact_mode": "inline", "artifact_max_bytes": 4},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status, _ := out["status"].(string); status != "passed" {
		t.Fatalf("expected passed, got %q", status)
	}
	assertions, _ := out["assertions"].([]interface{})
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %#v", assertions)
	}
	arts, _ := out["artifacts"].(map[string]interface{})
	artRaw, ok := arts["report.txt"]
	if !ok {
		t.Fatalf("expected inline artifact")
	}
	art, _ := artRaw.(map[string]interface{})
	if truncated, _ := art["truncated"].(bool); !truncated {
		t.Fatalf("expected truncated artifact")
	}
	contentB64, _ := art["content_base64"].(string)
	decoded, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != "1234" {
		t.Fatalf("expected truncated content 1234, got %q", string(decoded))
	}
}

func TestHandleRecipeTestCaseValidate_ServerRef(t *testing.T) {
	fakeSvc := &fakeRecipeService{
		getResp: &recipesvc.RecipeWithContent{
			Name: "r1", CommitHash: "v1", Content: []byte("version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: hi\n    type: short_answer\n"),
		},
	}
	h := &Handlers{recipeSvc: fakeSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{"mode": "server_ref", "name": "r1", "version": "v1"},
		"case":          map[string]interface{}{"id": "c4", "type": "recipe_case"},
	}
	raw, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/validate", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if fakeSvc.getReq == nil || fakeSvc.getReq.name != "r1" || fakeSvc.getReq.ref != "v1" {
		t.Fatalf("expected server_ref lookup, got %#v", fakeSvc.getReq)
	}
}

func TestHandleRecipeTestCaseExecute_OpCaseScopeEnforced(t *testing.T) {
	h := &Handlers{recipeSvc: &fakeRecipeService{}}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: hi\n    type: short_answer\n",
		},
		"case": map[string]interface{}{
			"id":   "c5",
			"type": "op_case",
			"target": map[string]interface{}{
				"node_path": "non.existent.path",
			},
			"mocks": map[string]interface{}{
				"ops": []map[string]interface{}{
					{
						"match": map[string]interface{}{"op": "input"},
						"behavior": map[string]interface{}{
							"mode":    "return",
							"outputs": map[string]interface{}{"response": "ok"},
						},
					},
				},
			},
		},
		"execution": map[string]interface{}{"mode": "isolated"},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status, _ := out["status"].(string); status != "failed" {
		t.Fatalf("expected failed status, got %q", status)
	}
	if cat, _ := out["failure_category"].(string); cat != "policy_blocked" {
		t.Fatalf("expected policy_blocked, got %q", cat)
	}
}

func TestHandleRecipeTestCaseExecute_PassthroughDependencyRequirement(t *testing.T) {
	h := &Handlers{recipeSvc: &fakeRecipeService{}}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: hi\n    type: short_answer\n",
		},
		"case": map[string]interface{}{
			"id":   "c6",
			"type": "recipe_case",
			"mocks": map[string]interface{}{
				"ops": []map[string]interface{}{
					{
						"match": map[string]interface{}{"op": "input"},
						"behavior": map[string]interface{}{
							"mode": "passthrough",
						},
					},
				},
			},
			"options": map[string]interface{}{
				"policy": map[string]interface{}{
					"required_dependencies": []string{"database"},
				},
			},
		},
		"execution": map[string]interface{}{"mode": "isolated"},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/p1/recipe-tests/cases/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errorsRaw, _ := out["errors"].([]interface{})
	if len(errorsRaw) == 0 {
		t.Fatalf("expected validation errors for missing dependency")
	}
}

func TestHandleRecipeTestCaseExecute_CellsTemplate_UsesInjectedProviderAndProjectID(t *testing.T) {
	const projectID = "proj-cells-template"

	builder := funcregistry.NewBuilder().WithDefaults()
	funcregistry.AddZeroFuncWithContext(builder, "cells", func(_ context.Context, taskCtx contextual.TaskExecutionContext) ([]funcregistry.CELCell, error) {
		if taskCtx.Workflow.ProjectId != projectID {
			return nil, fmt.Errorf("cells: expected project_id %q, got %q", projectID, taskCtx.Workflow.ProjectId)
		}
		return []funcregistry.CELCell{
			{Name: "alpha", ID: "1", Path: "/cells/alpha", Description: "alpha cell"},
		}, nil
	})

	h := &Handlers{
		recipeSvc:                    &fakeRecipeService{},
		recipeTestCELOptionsProvider: builder,
	}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"target_recipe": map[string]interface{}{
			"mode":    "inline_recipe",
			"format":  "yaml",
			"content": "version: '1.0'\nid: test\nop: input\ninputs:\n  form:\n    question: \"{{ cells | to_json }}\"\n    type: short_answer\n",
		},
		"case": map[string]interface{}{
			"id":   "c7",
			"type": "recipe_case",
			"mocks": map[string]interface{}{
				"ops": []map[string]interface{}{
					{
						"match": map[string]interface{}{"op": "input"},
						"behavior": map[string]interface{}{
							"mode":    "return",
							"outputs": map[string]interface{}{"response": "ok"},
						},
					},
				},
			},
		},
		"execution": map[string]interface{}{"mode": "isolated"},
	}
	raw, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/recipe-tests/cases/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status, _ := out["status"].(string); status != "passed" {
		t.Fatalf("expected passed, got %q; response=%#v", status, out)
	}
}

package handlers

import (
	"net/http"

	"github.com/colony-2/colony2/server/api/internal/recipetesting"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/gorilla/mux"
)

func (h *Handlers) handleRecipeTestCaseValidate(w http.ResponseWriter, r *http.Request) {
	req, issues := recipetesting.DecodeRequest(r.Body)
	if len(issues) > 0 {
		writeJSON(w, http.StatusBadRequest, recipetesting.ValidateResponse{Valid: false, Errors: issues})
		return
	}
	service := recipetesting.NewService(h.recipeSvc, h.recipeTestDeps)
	prepared := service.Prepare(r.Context(), project.ID(mux.Vars(r)["projectId"]), req)
	status := http.StatusOK
	if len(prepared.Validation.Errors) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, prepared.Validation)
}

func (h *Handlers) handleRecipeTestCaseExecute(w http.ResponseWriter, r *http.Request) {
	req, issues := recipetesting.DecodeRequest(r.Body)
	if len(issues) > 0 {
		writeJSON(w, http.StatusBadRequest, recipetesting.ValidateResponse{Valid: false, Errors: issues})
		return
	}
	service := recipetesting.NewService(h.recipeSvc, h.recipeTestDeps)
	prepared := service.Prepare(r.Context(), project.ID(mux.Vars(r)["projectId"]), req)
	if len(prepared.Validation.Errors) > 0 {
		writeJSON(w, http.StatusBadRequest, prepared.Validation)
		return
	}
	writeJSON(w, http.StatusOK, service.Execute(r.Context(), req, prepared))
}

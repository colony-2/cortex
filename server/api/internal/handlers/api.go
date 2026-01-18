package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/gorilla/mux"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

type cellDependencyLister interface {
	ListDependencies(ctx context.Context, projectID project.ID, from cell.ID) ([]cell.ID, error)
}

type graphBuilderPopulator struct {
	name    string
	builder core.GraphBuilder
}

func (p *graphBuilderPopulator) Name() string { return p.name }

func (p *graphBuilderPopulator) Populate(ctx context.Context, _ project.ID) ([]cell.PopulatorCell, error) {
	if p.builder == nil {
		return nil, errors.New("graph populator: builder is nil")
	}
	g, err := p.builder.BuildGraph(ctx)
	if err != nil {
		return nil, err
	}
	cells := make([]cell.PopulatorCell, 0, len(g.Cells))
	for _, c := range g.Cells {
		cells = append(cells, cell.PopulatorCell{
			Name:         c.ID,
			Description:  "",
			WorkingPath:  c.Path,
			ExternalID:   c.ID,
			Dependencies: c.Dependencies,
		})
	}
	return cells, nil
}

// registerAPIRoutes wires the OpenAPI endpoints into the /api subrouter.
func registerAPIRoutes(api *mux.Router, h *Handlers) {
	api.HandleFunc("/projects", withHandlerLog("projects:list", h.handleListProjects)).Methods(http.MethodGet)
	api.HandleFunc("/projects", withHandlerLog("projects:create", h.handleCreateProject)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}", withHandlerLog("projects:get", h.handleGetProject)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}", withHandlerLog("projects:update", h.handleUpdateProject)).Methods(http.MethodPatch)
	api.HandleFunc("/projects/{projectId}", withHandlerLog("projects:delete", h.handleDeleteProject)).Methods(http.MethodDelete)

	api.HandleFunc("/projects/{projectId}/cells", withHandlerLog("cells:list", h.handleListCells)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/cells", withHandlerLog("cells:create", h.handleCreateCell)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/cells/{cellId}", withHandlerLog("cells:get", h.handleGetCell)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/cells/{cellId}", withHandlerLog("cells:update", h.handleUpdateCell)).Methods(http.MethodPatch)
	api.HandleFunc("/projects/{projectId}/cells/{cellId}", withHandlerLog("cells:delete", h.handleDeleteCell)).Methods(http.MethodDelete)
	api.HandleFunc("/projects/{projectId}/cells/{cellId}/dependencies", withHandlerLog("cells:dependencies", h.handleReplaceDependencies)).Methods(http.MethodPut)
	api.HandleFunc("/projects/{projectId}/cells/sync", withHandlerLog("cells:sync", h.handleSyncCells)).Methods(http.MethodPost)

	api.HandleFunc("/projects/{projectId}/graph", withHandlerLog("graph:get", h.handleGetGraph)).Methods(http.MethodGet)

	api.HandleFunc("/projects/{projectId}/tickets", withHandlerLog("tickets:list", h.handleListTickets)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/tickets", withHandlerLog("tickets:create", h.handleCreateTicket)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/tickets/stages", withHandlerLog("tickets:stages", h.handleListTicketStages)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/tickets/states", withHandlerLog("tickets:states", h.handleListTicketStates)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/tickets/{ticketId}", withHandlerLog("tickets:get", h.handleGetTicket)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/tickets/{ticketId}", withHandlerLog("tickets:update", h.handleUpdateTicket)).Methods(http.MethodPatch)
	api.HandleFunc("/projects/{projectId}/tickets/{ticketId}/at", withHandlerLog("tickets:getAt", h.handleGetTicketAt)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/tickets/{ticketId}/events", withHandlerLog("tickets:events", h.handleListTicketEvents)).Methods(http.MethodGet)

	// Recipe management endpoints - using {recipeName:.*} to allow slashes in recipe names
	api.HandleFunc("/projects/{projectId}/recipes", withHandlerLog("recipes:list", h.handleListRecipes)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/recipes", withHandlerLog("recipes:create", h.handleCreateRecipe)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/recipes/validate", withHandlerLog("recipes:validate", h.handleValidateRecipe)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}/publish", withHandlerLog("recipes:publish", h.handlePublishRecipe)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}/unpublish", withHandlerLog("recipes:unpublish", h.handleUnpublishRecipe)).Methods(http.MethodPost)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}/history", withHandlerLog("recipes:history", h.handleGetRecipeHistory)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}", withHandlerLog("recipes:get", h.handleGetRecipe)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}", withHandlerLog("recipes:update", h.handleUpdateRecipe)).Methods(http.MethodPut)
	api.HandleFunc("/projects/{projectId}/recipes/{recipeName:.*}", withHandlerLog("recipes:delete", h.handleDeleteRecipe)).Methods(http.MethodDelete)

	api.HandleFunc("/projects/{projectId}/workflows", withHandlerLog("workflows:list", h.handleListWorkflows)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/workflows/{workflowId}", withHandlerLog("workflows:get", h.handleGetWorkflow)).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/workflows/{workflowId}/chapters/{chapterNumber}/artifacts/{artifactName}", withHandlerLog("workflows:artifact:get", h.handleGetWorkflowArtifact)).Methods(http.MethodGet)
}

func (h *Handlers) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}
	var filter project.SearchFilter
	q := r.URL.Query()
	if ids := q["ids"]; len(ids) > 0 {
		filter.IDs = make([]project.ID, len(ids))
		for i, id := range ids {
			filter.IDs[i] = project.ID(id)
		}
	}
	if names := q["names"]; len(names) > 0 {
		filter.Names = names
	}
	if nameContains := q.Get("nameContains"); nameContains != "" {
		filter.NameContains = nameContains
	}
	it, err := h.projects.ListProjects(r.Context(), filter)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer it.Close(r.Context())

	result := make([]openapi.Project, 0, 10)
	for {
		p, err := it.Next(r.Context())
		if errors.Is(err, project.ErrIteratorDone) {
			break
		}
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		result = append(result, toOpenAPIProject(p))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}
	var body openapi.ProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	created, err := h.projects.CreateProject(r.Context(), project.CreateInput{
		Name:        body.Name,
		GitRepoPath: body.GitRepoPath,
	})
	if err != nil {
		writeDomainError(w, err, projectErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusCreated, toOpenAPIProject(created))
}

func (h *Handlers) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}
	id := mux.Vars(r)["projectId"]
	prj, err := h.projects.GetProject(r.Context(), project.ID(id))
	if err != nil {
		status := projectErrorStatus(err)
		writeDomainError(w, err, status)
		return
	}
	writeJSON(w, http.StatusOK, toOpenAPIProject(prj))
}

func (h *Handlers) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}
	id := mux.Vars(r)["projectId"]
	var body openapi.ProjectUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	updated, err := h.projects.UpdateProject(r.Context(), project.ID(id), project.UpdateInput{
		Name:                body.Name,
		GitRepoPath:         body.GitRepoPath,
		GitRepoBranch:       body.GitRepoBranch,
		DefaultTicketRecipe: body.DefaultTicketRecipe,
	})
	if err != nil {
		status := projectErrorStatus(err)
		writeDomainError(w, err, status)
		return
	}
	writeJSON(w, http.StatusOK, toOpenAPIProject(updated))
}

func (h *Handlers) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}
	id := mux.Vars(r)["projectId"]
	if err := h.projects.DeleteProject(r.Context(), project.ID(id)); err != nil {
		status := projectErrorStatus(err)
		writeDomainError(w, err, status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleListCells(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	filter := cell.SearchFilter{
		ProjectIDs: []project.ID{projectID},
	}
	q := r.URL.Query()
	if ids := q["ids"]; len(ids) > 0 {
		filter.IDs = make([]cell.ID, len(ids))
		for i, id := range ids {
			filter.IDs[i] = cell.ID(id)
		}
	}
	if names := q["names"]; len(names) > 0 {
		filter.Names = names
	}
	if nameContains := q.Get("nameContains"); nameContains != "" {
		filter.NameContains = nameContains
	}
	if pathPrefix := q.Get("pathPrefix"); pathPrefix != "" {
		filter.PathPrefix = pathPrefix
	}
	if includeDeleted := q.Get("includeDeleted"); includeDeleted != "" {
		if parsed, err := strconv.ParseBool(includeDeleted); err == nil {
			filter.IncludeDeleted = parsed
		}
	}
	if dependsOn := q["dependsOn"]; len(dependsOn) > 0 {
		filter.DependsOn = make([]cell.ID, len(dependsOn))
		for i, id := range dependsOn {
			filter.DependsOn[i] = cell.ID(id)
		}
	}
	if pop := q.Get("populator"); pop != "" {
		filter.Populator = pop
	}
	if popIDs := q["populatorIds"]; len(popIDs) > 0 {
		filter.PopulatorIDs = popIDs
	}

	it, err := h.cells.ListCells(r.Context(), filter)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer it.Close(r.Context())

	var result []openapi.Cell
	for {
		c, err := it.Next(r.Context())
		if errors.Is(err, cell.ErrIteratorDone) {
			break
		}
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		deps, _ := h.listCellDeps(r.Context(), c.ProjectID, c.ID)
		result = append(result, toOpenAPICell(c, deps))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) handleCreateCell(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	var body openapi.CellCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	input := cell.CreateInput{
		ProjectID:   projectID,
		Name:        body.Name,
		WorkingPath: body.WorkingPath,
	}
	if body.Description != nil {
		input.Description = *body.Description
	}
	if body.Populator != nil {
		input.Populator = strings.TrimSpace(*body.Populator)
	}
	if body.PopulatorId != nil {
		input.PopulatorID = strings.TrimSpace(*body.PopulatorId)
	}

	created, err := h.cells.CreateCell(r.Context(), input)
	if err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}
	deps, _ := h.listCellDeps(r.Context(), projectID, created.ID)
	writeJSON(w, http.StatusCreated, toOpenAPICell(created, deps))
}

func (h *Handlers) handleGetCell(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	cellID := cell.ID(vars["cellId"])
	c, err := h.cells.GetCell(r.Context(), cellID)
	if err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}
	deps, _ := h.listCellDeps(r.Context(), c.ProjectID, c.ID)
	writeJSON(w, http.StatusOK, toOpenAPICell(c, deps))
}

func (h *Handlers) handleUpdateCell(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	cellID := cell.ID(vars["cellId"])
	var body openapi.CellUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	updated, err := h.cells.UpdateCell(r.Context(), cellID, cell.UpdateInput{
		Name:        body.Name,
		Description: body.Description,
		WorkingPath: body.WorkingPath,
	})
	if err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}
	deps, _ := h.listCellDeps(r.Context(), updated.ProjectID, updated.ID)
	writeJSON(w, http.StatusOK, toOpenAPICell(updated, deps))
}

func (h *Handlers) handleDeleteCell(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	cellID := cell.ID(vars["cellId"])
	if err := h.cells.MarkDeleted(r.Context(), cellID); err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleReplaceDependencies(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	cellID := cell.ID(vars["cellId"])
	var body openapi.CellDependenciesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	depIDs := make([]cell.ID, 0, len(body.Dependencies))
	for _, id := range body.Dependencies {
		depIDs = append(depIDs, cell.ID(id))
	}
	if err := h.cells.ReplaceDependencies(r.Context(), cellID, depIDs); err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleSyncCells(w http.ResponseWriter, r *http.Request) {
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}
	if h.projects == nil {
		http.Error(w, "project service unavailable", http.StatusNotImplemented)
		return
	}

	var body openapi.CellSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])

	popName := "graph/moon"
	if body.Populator != nil {
		if trimmed := strings.TrimSpace(*body.Populator); trimmed != "" {
			popName = trimmed
		}
	}

	var pop cell.Populator
	switch popName {
	case "graph/moon":
		if h.graphFactory == nil {
			writeError(w, fmt.Errorf("graph factory not configured"), http.StatusInternalServerError)
			return
		}
		gb, err := h.graphFactory(r.Context(), string(projectID))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, project.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, fmt.Errorf("graph factory: %w", err), status)
			return
		}
		pop = &graphBuilderPopulator{name: popName, builder: gb}
	default:
		writeError(w, fmt.Errorf("unsupported populator: %s", popName), http.StatusBadRequest)
		return
	}

	opts := cell.SyncOptions{PruneMissing: false}
	if body.PruneMissing != nil {
		opts.PruneMissing = *body.PruneMissing
	}

	result, err := h.cells.SyncFromPopulator(r.Context(), projectID, pop, opts)
	if err != nil {
		writeDomainError(w, err, cellErrorStatus(err))
		return
	}

	var affected *[]string
	if len(result.AffectedIDs) > 0 {
		ids := make([]string, len(result.AffectedIDs))
		for i, id := range result.AffectedIDs {
			ids[i] = string(id)
		}
		affected = &ids
	}

	resp := openapi.CellSyncResult{
		Populator:           popName,
		Created:             int32(result.Created),
		Updated:             int32(result.Updated),
		Restored:            int32(result.Restored),
		Deleted:             int32(result.Deleted),
		Skipped:             int32(result.Skipped),
		DependenciesUpdated: int32(result.DependenciesUpdated),
		AffectedIds:         affected,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	if h.cells == nil {
		http.Error(w, "cell service unavailable", http.StatusNotImplemented)
		return
	}

	graph, err := h.buildGraphResponse(r.Context(), projectID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (h *Handlers) buildGraphResponse(ctx context.Context, projectID project.ID) (openapi.Graph, error) {
	it, err := h.cells.ListCells(ctx, cell.SearchFilter{ProjectIDs: []project.ID{projectID}})
	if err != nil {
		return openapi.Graph{}, err
	}
	defer it.Close(ctx)

	var (
		cellsOut []openapi.Cell
		edges    []openapi.Edge
	)
	for {
		c, err := it.Next(ctx)
		if errors.Is(err, cell.ErrIteratorDone) {
			break
		}
		if err != nil {
			return openapi.Graph{}, err
		}
		deps, err := h.listCellDeps(ctx, c.ProjectID, c.ID)
		if err != nil {
			return openapi.Graph{}, err
		}
		cellsOut = append(cellsOut, toOpenAPICell(c, deps))
		edges = append(edges, depsToEdges(c.ID, deps)...)
	}

	return openapi.Graph{
		Cells: cellsOut,
		Edges: edges,
	}, nil
}

func depsToEdges(from cell.ID, deps []cell.ID) []openapi.Edge {
	edges := make([]openapi.Edge, 0, len(deps))
	for _, dep := range deps {
		edges = append(edges, openapi.Edge{
			Id:     fmt.Sprintf("%s-%s", from, dep),
			Source: string(from),
			Target: string(dep),
		})
	}
	return edges
}

func (h *Handlers) handleListTickets(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	filter := ticket.SearchFilter{
		Projects: []project.ID{projectID},
	}
	q := r.URL.Query()
	parseTime := func(key string) *time.Time {
		raw := q.Get(key)
		if raw == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return &t
		}
		return nil
	}
	if stages := q["stages"]; len(stages) > 0 {
		for _, s := range stages {
			filter.StageAny = append(filter.StageAny, ticket.Stage(s))
		}
	}
	if stageNot := q["stageNotIn"]; len(stageNot) > 0 {
		for _, s := range stageNot {
			filter.StageNotIn = append(filter.StageNotIn, ticket.Stage(s))
		}
	}
	if states := q["states"]; len(states) > 0 {
		for _, s := range states {
			filter.States = append(filter.States, ticket.State(s))
		}
	}
	if actors := q["actors"]; len(actors) > 0 {
		for _, a := range actors {
			filter.Actors = append(filter.Actors, ticket.ActorType(a))
		}
	}
	if cells := q["cells"]; len(cells) > 0 {
		for _, c := range cells {
			filter.Cells = append(filter.Cells, core.CellName(c))
		}
	}
	filter.UpdatedAfter = parseTime("updatedAfter")
	filter.UpdatedBefore = parseTime("updatedBefore")
	filter.CreatedAfter = parseTime("createdAfter")
	filter.CreatedBefore = parseTime("createdBefore")

	it, err := h.tickets.SearchTickets(r.Context(), filter)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer it.Close(r.Context())

	var result []openapi.Ticket
	for {
		tk, err := it.Next(r.Context())
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		result = append(result, toOpenAPITicket(tk))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	var body openapi.TicketCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	input := ticket.CreateInput{
		Cell:        core.CellName(body.Cell),
		ProjectID:   projectID,
		Title:       body.Title,
		Description: stringValue(body.Description),
		Stage:       ticket.Stage(body.Stage),
		State:       ticket.State(body.State),
		Actor:       toTicketActor(body.Actor),
	}
	created, err := h.tickets.CreateTicket(r.Context(), input)
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusCreated, toOpenAPITicket(created))
}

func (h *Handlers) handleListTicketStages(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	filter := ticket.SearchFilter{
		Projects: []project.ID{projectID},
	}
	q := r.URL.Query()
	if stages := q["stages"]; len(stages) > 0 {
		for _, s := range stages {
			filter.StageAny = append(filter.StageAny, ticket.Stage(s))
		}
	}
	if stageNot := q["stageNotIn"]; len(stageNot) > 0 {
		for _, s := range stageNot {
			filter.StageNotIn = append(filter.StageNotIn, ticket.Stage(s))
		}
	}
	if states := q["states"]; len(states) > 0 {
		for _, s := range states {
			filter.States = append(filter.States, ticket.State(s))
		}
	}
	if actors := q["actors"]; len(actors) > 0 {
		for _, a := range actors {
			filter.Actors = append(filter.Actors, ticket.ActorType(a))
		}
	}
	if cells := q["cells"]; len(cells) > 0 {
		for _, c := range cells {
			filter.Cells = append(filter.Cells, core.CellName(c))
		}
	}

	it, err := h.tickets.SearchStages(r.Context(), filter)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	defer it.Close(r.Context())

	var result []string
	for {
		stage, err := it.Next(r.Context())
		if errors.Is(err, ticket.ErrIteratorDone) {
			break
		}
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		result = append(result, string(stage))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) handleListTicketStates(w http.ResponseWriter, r *http.Request) {
	states := ticket.BuiltinStates()
	result := make([]string, len(states))
	for i, s := range states {
		result[i] = string(s)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	ticketID := ticket.ID(vars["ticketId"])
	tk, err := h.tickets.GetTicketAt(r.Context(), ticketID, time.Now().UTC())
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}
	if tk.ProjectID != projectID {
		writeError(w, errors.New("ticket does not belong to project"), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toOpenAPITicket(tk))
}

func (h *Handlers) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	ticketID := ticket.ID(vars["ticketId"])
	var body openapi.TicketUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	input := ticket.UpdateInput{
		CompletedAt: body.CompletedAt,
		Description: body.Description,
	}
	if body.Stage != nil {
		stage := ticket.Stage(*body.Stage)
		input.Stage = &stage
	}
	if body.State != nil {
		state := ticket.State(*body.State)
		input.State = &state
	}
	if body.ExpectedVersion != nil {
		input.ExpectedVersion = optimisticlock.Version{
			Int64: *body.ExpectedVersion,
			Valid: true,
		}
	}
	if body.Actor != nil {
		actor := toTicketActorPatch(*body.Actor)
		input.Actor = &actor
	}
	updated, err := h.tickets.UpdateTicket(r.Context(), ticketID, input)
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, toOpenAPITicket(updated))
}

func (h *Handlers) handleGetTicketAt(w http.ResponseWriter, r *http.Request) {
	if h.tickets == nil {
		http.Error(w, "ticket service unavailable", http.StatusNotImplemented)
		return
	}
	vars := mux.Vars(r)
	projectID := project.ID(vars["projectId"])
	ticketID := ticket.ID(vars["ticketId"])
	rawAt := r.URL.Query().Get("at")
	if rawAt == "" {
		http.Error(w, "at is required", http.StatusBadRequest)
		return
	}
	at, err := time.Parse(time.RFC3339, rawAt)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	tk, err := h.tickets.GetTicketAt(r.Context(), ticketID, at)
	if err != nil {
		writeDomainError(w, err, ticketErrorStatus(err))
		return
	}
	if tk.ProjectID != projectID {
		writeError(w, errors.New("ticket does not belong to project"), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toOpenAPITicket(tk))
}

// Old recipe handlers removed - now using recipe service handlers in recipes.go

func (h *Handlers) listCellDeps(ctx context.Context, projectID project.ID, cellID cell.ID) ([]cell.ID, error) {
	if h.cellDeps != nil {
		return h.cellDeps.ListDependencies(ctx, projectID, cellID)
	}
	return nil, nil
}

func toOpenAPIProject(p *project.Project) openapi.Project {
	return openapi.Project{
		Id:                  string(p.ID),
		Name:                p.Name,
		GitRepoPath:         p.GitRepoPath,
		GitRepoBranch:       p.GitRepoBranch,
		DefaultTicketRecipe: p.DefaultTicketRecipe,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
		Version:             p.Version.Int64,
	}
}

func toOpenAPICell(c *cell.Cell, deps []cell.ID) openapi.Cell {
	depStrs := make([]string, len(deps))
	for i, d := range deps {
		depStrs[i] = string(d)
	}
	return openapi.Cell{
		Id:           string(c.ID),
		Name:         c.Name,
		Path:         c.WorkingPath,
		Type:         c.Populator,
		Dependencies: depStrs,
	}
}

func toOpenAPIGraph(g *core.Graph) openapi.Graph {
	cells := make([]openapi.Cell, len(g.Cells))
	for i, c := range g.Cells {
		cells[i] = openapi.Cell{
			Id:           c.ID,
			Name:         c.Name,
			Path:         c.Path,
			Type:         c.Type,
			Dependencies: c.Dependencies,
		}
	}
	edges := make([]openapi.Edge, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = openapi.Edge{
			Id:     e.ID,
			Source: e.Source,
			Target: e.Target,
		}
	}
	return openapi.Graph{
		Cells: cells,
		Edges: edges,
	}
}

func toOpenAPITicket(tk *ticket.Ticket) openapi.Ticket {
	return openapi.Ticket{
		CellId:      string(tk.CellID),
		CellName:    string(tk.CellName),
		CompletedAt: tk.CompletedAt,
		CreatedAt:   tk.CreatedAt,
		Creator:     toOpenAPITicketActor(tk.Creator),
		Description: stringPtr(tk.Description),
		Id:          string(tk.ID),
		LastResetAt: tk.LastResetAt,
		LastResetId: toOptionalString(tk.LastResetID),
		ProjectId:   string(tk.ProjectID),
		Stage:       string(tk.Stage),
		State:       openapi.TicketState(tk.State),
		Title:       tk.Title,
		UpdatedAt:   tk.UpdatedAt,
		ValidFrom:   tk.ValidFrom,
		ValidUntil:  tk.ValidUntil,
		Version:     tk.Version.Int64,
	}
}

func toOpenAPITicketActor(actor ticket.Actor) openapi.Actor {
	switch actor.Type {
	case ticket.ActorTypeUser:
		email := ""
		if actor.User != nil {
			email = string(actor.User.Email)
		}
		return openapi.Actor{
			Type: openapi.User,
			User: &openapi.ActorUser{Email: openapi_types.Email(email)},
		}
	case ticket.ActorTypeAgent:
		agent := actor.Agent
		if agent == nil {
			return openapi.Actor{Type: openapi.Agent}
		}
		return openapi.Actor{
			Type: openapi.Agent,
			Agent: &openapi.ActorAgent{
				Cell:           agent.CellName,
				WorkflowName:   agent.WorkflowName,
				ExecutionId:    agent.ExecutionID,
				InvocationHash: agent.InvocationHash,
			},
		}
	default:
		return openapi.Actor{Type: openapi.User}
	}
}

func toTicketActor(actor openapi.Actor) ticket.Actor {
	switch actor.Type {
	case openapi.User:
		if actor.User != nil {
			return ticket.NewUserActor(string(actor.User.Email))
		}
	case openapi.Agent:
		if actor.Agent != nil {
			return ticket.NewAgentActor(actor.Agent.Cell, actor.Agent.WorkflowName, actor.Agent.ExecutionId, actor.Agent.InvocationHash)
		}
	}
	return ticket.NewUserActor("")
}

func toTicketActorPatch(actor openapi.ActorPatch) ticket.ActorPatch {
	var patch ticket.ActorPatch
	if actor.Type != nil {
		patch.Type = ticket.ActorType(*actor.Type)
	}
	if actor.User != nil {
		patch.User = &ticket.ActorUser{Email: ticket.EmailAddress(actor.User.Email)}
	}
	if actor.Agent != nil {
		patch.Agent = &ticket.ActorAgent{
			CellName:       actor.Agent.Cell,
			WorkflowName:   actor.Agent.WorkflowName,
			ExecutionID:    actor.Agent.ExecutionId,
			InvocationHash: actor.Agent.InvocationHash,
		}
	}
	return patch
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error, status int) {
	// Log stack trace for 500 errors
	if status == http.StatusInternalServerError {
		stack := debug.Stack()
		log.Printf("Internal Server Error: %v\nStack trace:\n%s", err, stack)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func writeDomainError(w http.ResponseWriter, err error, status int) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeError(w, err, status)
}

func projectErrorStatus(err error) int {
	switch {
	case errors.Is(err, project.ErrEmptyName), errors.Is(err, project.ErrEmptyGitRepo):
		return http.StatusBadRequest
	case errors.Is(err, project.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, project.ErrVersionConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func cellErrorStatus(err error) int {
	switch {
	case errors.Is(err, cell.ErrEmptyName), errors.Is(err, cell.ErrEmptyWorkingPath), errors.Is(err, cell.ErrInvalidProject), errors.Is(err, cell.ErrInvalidDependency):
		return http.StatusBadRequest
	case errors.Is(err, cell.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, cell.ErrVersionConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func ticketErrorStatus(err error) int {
	switch {
	case errors.Is(err, ticket.ErrInvalidProject), errors.Is(err, ticket.ErrInvalidCell), errors.Is(err, ticket.ErrInvalidState), errors.Is(err, ticket.ErrInvalidActor), errors.Is(err, ticket.ErrEmptyTitle), errors.Is(err, ticket.ErrEmptyStage):
		return http.StatusBadRequest
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, ticket.ErrVersionConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func stringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func stringPtr(val string) *string {
	if val == "" {
		return nil
	}
	return &val
}

func toOptionalString(id *ticket.TicketResetID) *string {
	if id == nil {
		return nil
	}
	str := string(*id)
	return &str
}

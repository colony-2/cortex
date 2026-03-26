package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/ticket/internal/idgen"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	eventstore "github.com/colony-2/colony2/server/ticket/internal/store/events"
	store "github.com/colony-2/colony2/server/ticket/internal/store/tickets"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/go-playground/validator/v10"
	"github.com/imdario/mergo"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

var (
	ErrInvalidState           = errors.New("ticket: invalid state")
	ErrInvalidActor           = errors.New("ticket: invalid actor")
	ErrEmptyTitle             = errors.New("ticket: title is required")
	ErrEmptyStage             = errors.New("ticket: stage is required")
	ErrInvalidProject         = errors.New("ticket: invalid project")
	ErrInvalidCell            = errors.New("ticket: invalid cell")
	ErrIDGeneration           = errors.New("ticket: id generation failed")
	ErrVersionConflict        = errors.New("ticket: version conflict")
	ErrInvalidEventKind       = errors.New("ticket: invalid event kind")
	ErrInvalidEventPayload    = errors.New("ticket: invalid event payload")
	ErrEventNotFound          = errors.New("ticket: event not found")
	ErrResetNoEvents          = errors.New("ticket: no events to reset")
	ErrUpdateNoFields         = errors.New("ticket: update requires at least one field")
	ErrInvalidDependency      = errors.New("ticket: invalid dependency")
	ErrDependencyNoPrimaryJob = errors.New("ticket: dependency ticket missing primary job")
	ErrAutostartUnavailable   = errors.New("ticket: autostart unavailable")
)

type Clock interface {
	Now() time.Time
}

type Service interface {
	CreateTicket(ctx context.Context, input CreateInput) (*model.Ticket, string, error)
	UpdateTicket(ctx context.Context, id model.ID, patch UpdateInput) (*model.Ticket, error)
	ApplyActions(ctx context.Context, actions []model.Action, fallbackActor model.Actor) ([]model.ActionResult, error)
	SearchTickets(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error)
	SearchStages(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error)
	GetStates(ctx context.Context) ([]model.State, error)
	GetTicketAt(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error)
	AppendWorkflowEvent(ctx context.Context, id model.ID, input WorkflowEventInput) (*model.TicketEvent, error)
	AppendMarkdownEvent(ctx context.Context, id model.ID, input MarkdownEventInput) (*model.TicketEvent, error)
	AppendChangeSetEvent(ctx context.Context, id model.ID, input ChangeSetEventInput) (*model.TicketEvent, error)
	AppendTicketEvent(ctx context.Context, id model.ID, input TicketEventInput) (*model.TicketEvent, error)
	ListEvents(ctx context.Context, id model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error)
	ResetTicket(ctx context.Context, id model.ID, input TicketResetInput) (*model.TicketReset, error)
}

// RecipeProjectProvider is a function that retrieves a recipe by project ID and recipe reference.
type RecipeProjectProvider func(projectId string, recipeRef string) (*recipe.Recipe, error)

type ServiceConfig struct {
	Store      store.Store
	EventStore eventstore.Store
	Projects   project.Service
	Cells      cell.Service
	Clock      Clock
	IDGen      model.ShortIDGenerator
	EventIDGen model.ShortIDGenerator
	Engine     swf.SWFEngine
	Recipes    RecipeProjectProvider
}

type service struct {
	store      store.Store
	events     eventstore.Store
	projects   project.Service
	cells      cell.Service
	clock      Clock
	idGen      model.ShortIDGenerator
	eventIDGen model.ShortIDGenerator
	validate   *validator.Validate
	engine     swf.SWFEngine
	recipes    RecipeProjectProvider
}

func New(config ServiceConfig) (Service, error) {
	if config.Store == nil {
		return nil, errors.New("ticket service: store is required")
	}
	if config.EventStore == nil {
		return nil, errors.New("ticket service: event store is required")
	}
	if config.Projects == nil {
		return nil, errors.New("ticket service: projects service is required")
	}
	if config.Cells == nil {
		return nil, errors.New("ticket service: cells service is required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.IDGen == nil {
		config.IDGen = idgen.NewKSUIDGenerator()
	}
	if config.EventIDGen == nil {
		config.EventIDGen = idgen.NewKSUIDGenerator()
	}
	validate, err := newValidator()
	if err != nil {
		return nil, err
	}
	return &service{
		store:      config.Store,
		events:     config.EventStore,
		projects:   config.Projects,
		cells:      config.Cells,
		clock:      config.Clock,
		idGen:      config.IDGen,
		eventIDGen: config.EventIDGen,
		validate:   validate,
		engine:     config.Engine,
		recipes:    config.Recipes,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func (s *service) CreateTicket(ctx context.Context, input CreateInput) (*model.Ticket, string, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Stage = normalizeStage(input.Stage)
	input.Actor = sanitizeActorFields(input.Actor)
	normalizedDeps, err := normalizeDependencyIDs(input.DependsOnTicketIDs)
	if err != nil {
		return nil, "", err
	}
	input.DependsOnTicketIDs = normalizedDeps

	if err := s.validateCreateInput(input); err != nil {
		return nil, "", err
	}

	actor, err := s.normalizeActor(input.Actor)
	if err != nil {
		return nil, "", err
	}

	projectID := strings.TrimSpace(string(input.ProjectID))
	if projectID == "" {
		return nil, "", ErrInvalidProject
	}
	projectRecord, err := s.projects.GetProject(ctx, project.ID(projectID))
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return nil, "", ErrInvalidProject
		}
		return nil, "", err
	}

	cellRecord, err := s.resolveCell(ctx, project.ID(projectID), input.Cell)
	if err != nil {
		return nil, "", err
	}

	ticketID, err := s.idGen.NewID()
	if err != nil {
		return nil, "", errors.Join(ErrIDGeneration, err)
	}

	var primaryJobID string
	if s.engine != nil && s.recipes != nil {
		primaryJobID, err = s.idGen.NewID()
		if err != nil {
			return nil, "", errors.Join(ErrIDGeneration, err)
		}
	} else if len(input.DependsOnTicketIDs) > 0 {
		return nil, "", ErrAutostartUnavailable
	}

	now := s.clock.Now()
	ticket := &model.Ticket{
		ID:          model.ID(ticketID),
		CellID:      cellRecord.ID,
		CellName:    core.CellName(cellRecord.Name),
		ProjectID:   cellRecord.ProjectID,
		Title:       input.Title,
		Description: input.Description,
		Stage:       input.Stage,
		State:       input.State,
		Creator:     actor,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if primaryJobID != "" {
		ticket.PrimaryJobID = &primaryJobID
	}
	if len(input.DependsOnTicketIDs) > 0 {
		deps := make([]string, 0, len(input.DependsOnTicketIDs))
		for _, id := range input.DependsOnTicketIDs {
			deps = append(deps, string(id))
		}
		ticket.DependsOnTicketIDs = pq.StringArray(deps)
	}
	if ticket.Stage == model.CompletedStage {
		ticket.CompletedAt = ptrTime(now)
	}
	ticket.ValidFrom = now
	ticket.ValidUntil = infinity()

	var created *model.Ticket
	var jobID string
	err = s.store.WithTx(ctx, func(ctx context.Context, st store.Store) error {
		if err := st.Create(ctx, ticket); err != nil {
			return err
		}

		if s.engine != nil && s.recipes != nil {
			prereqs, err := s.resolveTicketPrerequisites(ctx, st, ticket.ProjectID, ticket.ID, input.DependsOnTicketIDs)
			if err != nil {
				return err
			}
			jobKey, err := s.startTicketRecipe(ctx, st, ticket, projectRecord, cellRecord, primaryJobID, prereqs)
			if err != nil {
				return err
			}
			jobID = jobKey
		}

		created = ticket
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	if err := s.attachLastReset(ctx, created); err != nil {
		return nil, "", err
	}

	return created, jobID, nil
}

func normalizeDependencyIDs(ids []model.ID) ([]model.ID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[model.ID]struct{}, len(ids))
	out := make([]model.ID, 0, len(ids))
	for _, id := range ids {
		trimmed := model.ID(strings.TrimSpace(string(id)))
		if trimmed == "" {
			return nil, ErrInvalidDependency
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out, nil
}

func (s *service) UpdateTicket(ctx context.Context, id model.ID, patch UpdateInput) (*model.Ticket, error) {
	normalized := patch
	normalized.Stage = normalizeStagePtr(patch.Stage)
	if patch.Actor != nil {
		sanitized := sanitizeActorPatch(*patch.Actor)
		normalized.Actor = &sanitized
	}
	if patch.Description != nil {
		desc := strings.TrimSpace(*patch.Description)
		normalized.Description = &desc
	}

	if err := s.validateUpdateInput(normalized); err != nil {
		return nil, err
	}

	var updated *model.Ticket
	err := s.store.WithTx(ctx, func(ctx context.Context, st store.Store) error {
		existing, err := st.Get(ctx, id)
		if err != nil {
			return err
		}
		if normalized.ExpectedVersion.Valid && existing.Version != normalized.ExpectedVersion {
			return ErrVersionConflict
		}
		if err := s.attachLastReset(ctx, existing); err != nil {
			return err
		}

		now := s.clock.Now()
		if !existing.ValidFrom.Before(now) {
			now = existing.ValidFrom.Add(time.Microsecond)
		}

		original := *existing
		closeSlice := *existing
		closeSlice.ValidUntil = now
		closeSlice.UpdatedAt = now
		if err := st.Update(ctx, &closeSlice, "ValidUntil", "UpdatedAt"); err != nil {
			if errors.Is(err, store.ErrOptimisticLock) {
				return ErrVersionConflict
			}
			return err
		}

		next := original
		next.ValidFrom = now
		next.ValidUntil = infinity()
		next.UpdatedAt = now
		next.Version = optimisticlock.Version{Int64: original.Version.Int64 + 1, Valid: true}

		var changes []model.TicketFieldChange

		if normalized.Stage != nil {
			if next.Stage != *normalized.Stage {
				changes = append(changes, model.TicketFieldChange{Field: model.TicketFieldName("stage"), From: string(next.Stage), To: string(*normalized.Stage)})
			}
			next.Stage = *normalized.Stage
		}

		if normalized.State != nil {
			if next.State != *normalized.State {
				changes = append(changes, model.TicketFieldChange{Field: model.TicketFieldName("state"), From: string(next.State), To: string(*normalized.State)})
			}
			next.State = *normalized.State
		}

		if normalized.Description != nil {
			desc := strings.TrimSpace(*normalized.Description)
			if next.Description != desc {
				changes = append(changes, model.TicketFieldChange{Field: model.TicketFieldName("description"), From: next.Description, To: desc})
			}
			next.Description = desc
		}

		if normalized.Actor != nil {
			actor, err := s.applyActorPatch(next.Creator, *normalized.Actor)
			if err != nil {
				return err
			}
			if actorSummary(next.Creator) != actorSummary(actor) {
				changes = append(changes, model.TicketFieldChange{Field: model.TicketFieldName("creator"), From: actorSummary(next.Creator), To: actorSummary(actor)})
			}
			next.Creator = actor
		}

		var completedExplicit bool
		if normalized.CompletedAt != nil {
			next.CompletedAt = normalized.CompletedAt
			completedExplicit = true
		}

		if next.Stage == model.CompletedStage && next.CompletedAt == nil {
			next.CompletedAt = ptrTime(now)
		}

		if original.Stage == model.CompletedStage && next.Stage != model.CompletedStage && !completedExplicit {
			next.CompletedAt = nil
		}

		if err := st.Create(ctx, &next); err != nil {
			return err
		}
		updated = &next

		if len(changes) > 0 {
			if _, err := s.appendTicketEventInTx(ctx, st, &original, id, next.Creator, changes, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.attachLastReset(ctx, updated); err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *service) SearchTickets(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error) {
	return s.store.Search(ctx, filter)
}

func (s *service) SearchStages(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error) {
	stages := filterStages(model.BuiltinStages(), filter)
	return store.NewSliceIterator(stages), nil
}

func (s *service) GetStates(ctx context.Context) ([]model.State, error) {
	_ = ctx
	return model.BuiltinStates(), nil
}

func (s *service) GetTicketAt(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error) {
	ticket, err := s.store.GetAt(ctx, id, at)
	if err != nil {
		return nil, err
	}
	if err := s.attachLastReset(ctx, ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}

func (s *service) validateCreateInput(input CreateInput) error {
	if err := s.validate.Struct(input); err != nil {
		return mapValidationError(err)
	}
	return nil
}

func (s *service) validateUpdateInput(input UpdateInput) error {
	if err := s.validate.Struct(input); err != nil {
		return mapValidationError(err)
	}
	return nil
}

func (s *service) normalizeActor(actor model.Actor) (model.Actor, error) {
	actor = sanitizeActorFields(actor)
	if err := s.validate.Struct(actor); err != nil {
		return model.Actor{}, ErrInvalidActor
	}
	switch actor.Type {
	case model.ActorTypeUser:
		actor.Agent = nil
	case model.ActorTypeAgent:
		actor.User = nil
	default:
		return model.Actor{}, ErrInvalidActor
	}
	return actor, nil
}

func (s *service) applyActorPatch(existing model.Actor, patch model.ActorPatch) (model.Actor, error) {
	merged := existing
	if patch.Type != "" {
		merged.Type = patch.Type
	}
	if patch.User != nil {
		if merged.User == nil {
			merged.User = &model.ActorUser{}
		}
		if err := mergo.Merge(merged.User, patch.User, mergo.WithOverride); err != nil {
			return model.Actor{}, err
		}
	}
	if patch.Agent != nil {
		if merged.Agent == nil {
			merged.Agent = &model.ActorAgent{}
		}
		if err := mergo.Merge(merged.Agent, patch.Agent, mergo.WithOverride); err != nil {
			return model.Actor{}, err
		}
	}
	merged = sanitizeActorFields(merged)
	actor, err := s.normalizeActor(merged)
	if err != nil {
		return model.Actor{}, err
	}
	return actor, nil
}

func infinity() time.Time {
	return time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
}

func ptrTime(t time.Time) *time.Time {
	tt := t
	return &tt
}

func actorSummary(actor model.Actor) string {
	switch actor.Type {
	case model.ActorTypeUser:
		if actor.User != nil {
			return string(actor.User.Email)
		}
	case model.ActorTypeAgent:
		if actor.Agent != nil {
			return strings.Join([]string{actor.Agent.CellName, actor.Agent.WorkflowName, actor.Agent.ExecutionID}, ":")
		}
	}
	return string(actor.Type)
}

func (s *service) attachLastReset(ctx context.Context, ticket *model.Ticket) error {
	if ticket == nil {
		return nil
	}
	if ticket.LastResetAt != nil {
		lastReset := ticket.LastResetAt.UTC()
		ticket.LastResetAt = &lastReset
	}
	return nil
}

func applyTicketResetMetadata(ticket *model.Ticket, reset *model.TicketReset) {
	if ticket == nil {
		return
	}
	if reset == nil {
		ticket.LastResetID = nil
		ticket.LastResetAt = nil
		return
	}
	idCopy := reset.ID
	ticket.LastResetID = &idCopy
	atCopy := reset.CreatedAt.UTC()
	ticket.LastResetAt = &atCopy
}

func (s *service) startTicketRecipe(ctx context.Context, st store.Store, ticket *model.Ticket, projectRecord *project.Project, cellRecord *cell.Cell, primaryJobID string, prereqs []swf.JobPrerequisite) (string, error) {
	if st == nil || ticket == nil || projectRecord == nil || cellRecord == nil {
		return "", errors.New("ticket: missing dependencies for recipe start")
	}

	recipeName := defaultRecipeName(cellRecord, projectRecord)

	repo := projectRecord.GitRepoPath
	if cellRecord.GitRepoName != nil && strings.TrimSpace(*cellRecord.GitRepoName) != "" {
		repo = strings.TrimSpace(*cellRecord.GitRepoName)
	}
	ref := "main"
	if projectRecord.GitRepoBranch != nil && strings.TrimSpace(*projectRecord.GitRepoBranch) != "" {
		ref = strings.TrimSpace(*projectRecord.GitRepoBranch)
	}
	if cellRecord.GitBranch != nil && strings.TrimSpace(*cellRecord.GitBranch) != "" {
		ref = strings.TrimSpace(*cellRecord.GitBranch)
	}

	//blobStore := filepath.Join(projectRecord.GitRepoPath, ".colony2", "blobstore")
	//filepath.Join(blobStore, "artifacts")
	//os.MkdirTemp("",'c2-recipe-work')
	startJob := workflowctl.StartJob{
		TenantId:   string(ticket.ProjectID),
		RecipeName: recipeName,
		Inputs:     map[string]interface{}{},
		JobContext: contextual.JobContext{
			Actor: contextual.ActorContext{
				TicketID:   string(ticket.ID),
				ActorEmail: actorEmail(ticket.Creator),
			},
			Ticket: contextual.TicketContext{
				ID:          string(ticket.ID),
				Title:       ticket.Title,
				Description: ticket.Description,
				Creator:     actorToCreatorContext(ticket.Creator),
				CreatedAt:   ticket.CreatedAt,
				UpdatedAt:   ticket.UpdatedAt,
			},
			Workflow: contextual.WorkflowContext{
				CellID:   string(cellRecord.ID),
				CellName: string(ticket.CellName),
				CellPath: cellRecord.WorkingPath,
			},
			Environment: contextual.EnvironmentContext{
				//WorktreePath: cellRecord.WorkingPath,
				//BlobStoreURI: "file://" + blobStore,
			},
			GitBase: contextual.GitBaseContext{
				BaseRepo: repo,
				BaseRef:  ref,
			},
		},
		GitRef: ref,
	}

	jobCtx := ctx
	if tx := st.DB(); tx != nil {
		jobCtx = swf.WithTx(ctx, tx)
	}

	jobKey, err := starter.StartRecipeJobWithOptions(jobCtx, startJob, s.engine, starter.StartRecipeJobOptions{
		JobID:         strings.TrimSpace(primaryJobID),
		Prerequisites: prereqs,
	})
	if err != nil {
		return "", err
	}

	workflowPayload := model.WorkflowEventPayload{
		Type:       model.WorkflowEventRunning,
		WorkflowID: model.WorkflowID(jobKey.JobId),
		RunID:      model.WorkflowRunID(jobKey.JobId),
	}
	_, err = s.appendEventInTx(ctx, st, ticket, ticket.ID, ticket.Creator, model.TicketEventKindWorkflow, model.TicketEventBody{Workflow: &workflowPayload}, s.clock.Now())
	if err != nil {
		return "", err
	}
	return jobKey.JobId, nil
}

func (s *service) resolveTicketPrerequisites(ctx context.Context, st store.Store, projectID project.ID, ticketID model.ID, dependsOn []model.ID) ([]swf.JobPrerequisite, error) {
	if len(dependsOn) == 0 {
		return nil, nil
	}
	prereqs := make([]swf.JobPrerequisite, 0, len(dependsOn))
	for _, depID := range dependsOn {
		if depID == "" {
			return nil, ErrInvalidDependency
		}
		if depID == ticketID {
			return nil, ErrInvalidDependency
		}
		dep, err := st.Get(ctx, depID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrInvalidDependency
			}
			return nil, err
		}
		if dep.ProjectID != projectID {
			return nil, ErrInvalidDependency
		}
		if dep.PrimaryJobID == nil || strings.TrimSpace(*dep.PrimaryJobID) == "" {
			return nil, ErrDependencyNoPrimaryJob
		}
		prereqs = append(prereqs, swf.JobPrerequisite{
			JobID:     strings.TrimSpace(*dep.PrimaryJobID),
			Condition: swf.JobPrereqSuccess,
		})
	}
	return prereqs, nil
}

func (s *service) resolveCell(ctx context.Context, projectID project.ID, name core.CellName) (*cell.Cell, error) {
	trimmed := strings.TrimSpace(string(name))
	if trimmed == "" {
		return nil, ErrInvalidCell
	}
	iter, err := s.cells.ListCells(ctx, cell.SearchFilter{
		ProjectIDs: []project.ID{projectID},
		Names:      []string{trimmed},
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close(ctx)
	c, err := iter.Next(ctx)
	if errors.Is(err, cell.ErrIteratorDone) {
		return nil, ErrInvalidCell
	}
	if err != nil {
		return nil, err
	}
	if c == nil || c.ProjectID != projectID {
		return nil, ErrInvalidCell
	}
	return c, nil
}

func newValidator() (*validator.Validate, error) {
	v := validator.New()
	if err := v.RegisterValidation("stage", stageValidator); err != nil {
		return nil, err
	}
	if err := v.RegisterValidation("state", stateValidator); err != nil {
		return nil, err
	}
	if err := v.RegisterValidation("version", versionValidator); err != nil {
		return nil, err
	}
	v.RegisterStructValidation(actorStructValidation, model.Actor{})
	return v, nil
}

func stageValidator(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}
		field = field.Elem()
	}
	if field.Kind() != reflect.String {
		return false
	}
	normalized := model.Stage(strings.ToLower(strings.TrimSpace(field.String())))
	return model.IsValidStage(normalized)
}

func stateValidator(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return true
		}
		field = field.Elem()
	}
	if field.Kind() != reflect.String {
		return false
	}
	return model.IsValidState(model.State(field.String()))
}

func versionValidator(fl validator.FieldLevel) bool {
	version, ok := fl.Field().Interface().(optimisticlock.Version)
	if !ok {
		if val, ok := fl.Field().Addr().Interface().(*optimisticlock.Version); ok && val != nil {
			version = *val
		} else {
			return false
		}
	}
	return version.Valid
}

func actorStructValidation(sl validator.StructLevel) {
	actor, ok := sl.Current().Interface().(model.Actor)
	if !ok {
		return
	}
	switch actor.Type {
	case model.ActorTypeUser:
		if actor.User == nil || strings.TrimSpace(string(actor.User.Email)) == "" {
			sl.ReportError(actor.User, "Actor", "Actor", "user", "")
		}
	case model.ActorTypeAgent:
		if actor.Agent == nil {
			sl.ReportError(actor.Agent, "Actor", "Actor", "agent", "")
			return
		}
		if strings.TrimSpace(actor.Agent.CellName) == "" ||
			strings.TrimSpace(actor.Agent.WorkflowName) == "" ||
			strings.TrimSpace(actor.Agent.ExecutionID) == "" ||
			strings.TrimSpace(actor.Agent.InvocationHash) == "" {
			sl.ReportError(actor.Agent, "Actor", "Actor", "agent", "")
		}
	default:
		sl.ReportError(actor.Type, "Actor", "Actor", "type", "")
	}
}

func mapValidationError(err error) error {
	if err == nil {
		return nil
	}
	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fieldErr := range ve {
			switch fieldErr.StructField() {
			case "Title":
				return ErrEmptyTitle
			case "Stage":
				return ErrEmptyStage
			case "State":
				return ErrInvalidState
			case "Actor":
				return ErrInvalidActor
			case "Kind":
				return ErrInvalidEventKind
			case "Payload":
				return ErrInvalidEventPayload
			case "ExpectedVersion":
				return ErrVersionConflict
			}
		}
	}
	return err
}

func normalizeStage(stage model.Stage) model.Stage {
	return model.Stage(strings.ToLower(strings.TrimSpace(string(stage))))
}

func normalizeStagePtr(stage *model.Stage) *model.Stage {
	if stage == nil {
		return nil
	}
	normalized := normalizeStage(*stage)
	return &normalized
}

func filterStages(stages []model.Stage, filter model.SearchFilter) []model.Stage {
	if len(stages) == 0 {
		return nil
	}
	include := map[model.Stage]struct{}{}
	if len(filter.StageAny) > 0 {
		for _, stage := range filter.StageAny {
			normalized := normalizeStage(stage)
			if model.IsValidStage(normalized) {
				include[normalized] = struct{}{}
			}
		}
	}
	exclude := map[model.Stage]struct{}{}
	if len(filter.StageNotIn) > 0 {
		for _, stage := range filter.StageNotIn {
			normalized := normalizeStage(stage)
			if model.IsValidStage(normalized) {
				exclude[normalized] = struct{}{}
			}
		}
	}

	filtered := make([]model.Stage, 0, len(stages))
	for _, stage := range stages {
		if len(include) > 0 {
			if _, ok := include[stage]; !ok {
				continue
			}
		}
		if _, ok := exclude[stage]; ok {
			continue
		}
		filtered = append(filtered, stage)
	}
	return filtered
}

func sanitizeActorFields(actor model.Actor) model.Actor {
	actor.Type = model.ActorType(strings.TrimSpace(string(actor.Type)))
	if actor.User != nil {
		email := strings.TrimSpace(string(actor.User.Email))
		actor.User = &model.ActorUser{Email: model.EmailAddress(email)}
	}
	if actor.Agent != nil {
		agent := *actor.Agent
		agent.CellName = strings.TrimSpace(agent.CellName)
		agent.WorkflowName = strings.TrimSpace(agent.WorkflowName)
		agent.ExecutionID = strings.TrimSpace(agent.ExecutionID)
		agent.InvocationHash = strings.TrimSpace(agent.InvocationHash)
		actor.Agent = &agent
	}
	return actor
}

func sanitizeActorPatch(patch model.ActorPatch) model.ActorPatch {
	patch.Type = model.ActorType(strings.TrimSpace(string(patch.Type)))
	if patch.User != nil {
		email := strings.TrimSpace(string(patch.User.Email))
		patch.User = &model.ActorUser{Email: model.EmailAddress(email)}
	}
	if patch.Agent != nil {
		agent := *patch.Agent
		agent.CellName = strings.TrimSpace(agent.CellName)
		agent.WorkflowName = strings.TrimSpace(agent.WorkflowName)
		agent.ExecutionID = strings.TrimSpace(agent.ExecutionID)
		agent.InvocationHash = strings.TrimSpace(agent.InvocationHash)
		patch.Agent = &agent
	}
	return patch
}

package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/idgen"
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	eventstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/events"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"github.com/go-playground/validator/v10"
	"github.com/imdario/mergo"
	"gorm.io/plugin/optimisticlock"
)

var (
	ErrInvalidState        = errors.New("ticket: invalid state")
	ErrInvalidActor        = errors.New("ticket: invalid actor")
	ErrEmptyTitle          = errors.New("ticket: title is required")
	ErrEmptyStage          = errors.New("ticket: stage is required")
	ErrIDGeneration        = errors.New("ticket: id generation failed")
	ErrVersionConflict     = errors.New("ticket: version conflict")
	ErrInvalidEventKind    = errors.New("ticket: invalid event kind")
	ErrInvalidEventPayload = errors.New("ticket: invalid event payload")
	ErrEventNotFound       = errors.New("ticket: event not found")
	ErrResetNoEvents       = errors.New("ticket: no events to reset")
)

type Clock interface {
	Now() time.Time
}

type Service interface {
	CreateTicket(ctx context.Context, input CreateInput) (*model.Ticket, error)
	UpdateTicket(ctx context.Context, id model.ID, patch UpdateInput) (*model.Ticket, error)
	SearchTickets(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error)
	SearchStages(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error)
	GetStates(ctx context.Context) ([]model.State, error)
	GetTicketAt(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error)
	AppendWorkflowEvent(ctx context.Context, id model.ID, input WorkflowEventInput) (*model.TicketEvent, error)
	AppendMarkdownEvent(ctx context.Context, id model.ID, input MarkdownEventInput) (*model.TicketEvent, error)
	AppendChangeSetEvent(ctx context.Context, id model.ID, input ChangeSetEventInput) (*model.TicketEvent, error)
	ListEvents(ctx context.Context, id model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error)
	ResetTicket(ctx context.Context, id model.ID, input TicketResetInput) (*model.TicketReset, error)
}

type ServiceConfig struct {
	Store      store.Store
	EventStore eventstore.Store
	Clock      Clock
	IDGen      model.ShortIDGenerator
	EventIDGen model.ShortIDGenerator
}

type service struct {
	store      store.Store
	events     eventstore.Store
	clock      Clock
	idGen      model.ShortIDGenerator
	eventIDGen model.ShortIDGenerator
	validate   *validator.Validate
}

func New(config ServiceConfig) (Service, error) {
	if config.Store == nil {
		return nil, errors.New("ticket service: store is required")
	}
	if config.EventStore == nil {
		return nil, errors.New("ticket service: event store is required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.IDGen == nil {
		config.IDGen = idgen.NewBase58Generator(idgen.DefaultIDLength)
	}
	if config.EventIDGen == nil {
		config.EventIDGen = config.IDGen
	}
	validate, err := newValidator()
	if err != nil {
		return nil, err
	}
	return &service{
		store:      config.Store,
		events:     config.EventStore,
		clock:      config.Clock,
		idGen:      config.IDGen,
		eventIDGen: config.EventIDGen,
		validate:   validate,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func (s *service) CreateTicket(ctx context.Context, input CreateInput) (*model.Ticket, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Stage = normalizeStage(input.Stage)
	input.Actor = sanitizeActorFields(input.Actor)

	if err := s.validateCreateInput(input); err != nil {
		return nil, err
	}

	actor, err := s.normalizeActor(input.Actor)
	if err != nil {
		return nil, err
	}

	ticketID, err := s.idGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}

	now := s.clock.Now()
	ticket := &model.Ticket{
		ID:          model.ID(ticketID),
		CellName:    input.Cell,
		Title:       input.Title,
		Description: input.Description,
		Stage:       input.Stage,
		State:       input.State,
		Creator:     actor,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if ticket.Stage == model.CompletedStage {
		ticket.CompletedAt = ptrTime(now)
	}
	ticket.ValidFrom = now
	ticket.ValidUntil = temporalInfinity()

	if err := s.store.Create(ctx, ticket); err != nil {
		return nil, err
	}

	return ticket, nil
}

func (s *service) UpdateTicket(ctx context.Context, id model.ID, patch UpdateInput) (*model.Ticket, error) {
	normalized := patch
	normalized.Stage = normalizeStagePtr(patch.Stage)
	if patch.Actor != nil {
		sanitized := sanitizeActorPatch(*patch.Actor)
		normalized.Actor = &sanitized
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
		if existing.Version != normalized.ExpectedVersion {
			return ErrVersionConflict
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
		next.ValidUntil = temporalInfinity()
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
			if _, err := s.appendTicketEvent(ctx, id, next.Creator, changes, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *service) SearchTickets(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Ticket], error) {
	return s.store.Search(ctx, filter)
}

func (s *service) SearchStages(ctx context.Context, filter model.SearchFilter) (store.Iterator[model.Stage], error) {
	return s.store.SearchStages(ctx, filter)
}

func (s *service) GetStates(ctx context.Context) ([]model.State, error) {
	_ = ctx
	return model.BuiltinStates(), nil
}

func (s *service) GetTicketAt(ctx context.Context, id model.ID, at time.Time) (*model.Ticket, error) {
	return s.store.GetAt(ctx, id, at)
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

func temporalInfinity() time.Time {
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
	return strings.TrimSpace(field.String()) != ""
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
	return model.Stage(strings.TrimSpace(string(stage)))
}

func normalizeStagePtr(stage *model.Stage) *model.Stage {
	if stage == nil {
		return nil
	}
	normalized := normalizeStage(*stage)
	return &normalized
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

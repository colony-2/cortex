package service

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/idgen"
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"github.com/go-playground/validator/v10"
	"github.com/imdario/mergo"
	"gorm.io/plugin/optimisticlock"
)

var (
	ErrInvalidState    = errors.New("ticket: invalid state")
	ErrInvalidActor    = errors.New("ticket: invalid actor")
	ErrEmptyTitle      = errors.New("ticket: title is required")
	ErrEmptyStage      = errors.New("ticket: stage is required")
	ErrIDGeneration    = errors.New("ticket: id generation failed")
	ErrVersionConflict = errors.New("ticket: version conflict")
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
}

type ServiceConfig struct {
	Store store.Store
	Clock Clock
	IDGen model.ShortIDGenerator
}

type service struct {
	store    store.Store
	clock    Clock
	idGen    model.ShortIDGenerator
	validate *validator.Validate
}

func New(config ServiceConfig) (Service, error) {
	if config.Store == nil {
		return nil, errors.New("ticket service: store is required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.IDGen == nil {
		config.IDGen = idgen.NewBase58Generator(idgen.DefaultIDLength)
	}
	validate, err := newValidator()
	if err != nil {
		return nil, err
	}
	return &service{
		store:    config.Store,
		clock:    config.Clock,
		idGen:    config.IDGen,
		validate: validate,
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
		fieldSet := map[string]struct{}{"UpdatedAt": {}}
		existing.UpdatedAt = now

		prevStage := existing.Stage
		stageChanged := false
		if normalized.Stage != nil {
			if existing.Stage != *normalized.Stage {
				stageChanged = true
			}
			existing.Stage = *normalized.Stage
			fieldSet["Stage"] = struct{}{}
		}

		if normalized.State != nil {
			existing.State = *normalized.State
			fieldSet["State"] = struct{}{}
		}

		if normalized.Actor != nil {
			actor, err := s.applyActorPatch(existing.Creator, *normalized.Actor)
			if err != nil {
				return err
			}
			existing.Creator = actor
			fieldSet["Creator"] = struct{}{}
		}

		var completedExplicit bool
		if normalized.CompletedAt != nil {
			existing.CompletedAt = normalized.CompletedAt
			fieldSet["CompletedAt"] = struct{}{}
			completedExplicit = true
		}

		if existing.Stage == model.CompletedStage && existing.CompletedAt == nil {
			existing.CompletedAt = ptrTime(now)
			fieldSet["CompletedAt"] = struct{}{}
		}

		if stageChanged && prevStage == model.CompletedStage && existing.Stage != model.CompletedStage && !completedExplicit {
			existing.CompletedAt = nil
			fieldSet["CompletedAt"] = struct{}{}
		}

		fields := slices.Collect(maps.Keys(fieldSet))
		if err := st.Update(ctx, existing, fields...); err != nil {
			if errors.Is(err, store.ErrOptimisticLock) {
				return ErrVersionConflict
			}
			return err
		}
		updated = existing
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

func ptrTime(t time.Time) *time.Time {
	tt := t
	return &tt
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

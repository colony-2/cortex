package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"gorm.io/gorm"
)

// Service provides recipe lifecycle operations.
type Service interface {
	CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error)
	UpdateRecipe(ctx context.Context, input model.UpdateInput) (*model.RecipeVersion, error)
	DeleteRecipe(ctx context.Context, projectID project.ID, name string) error

	PublishRecipe(ctx context.Context, input model.PublishInput) (*model.PublishedRecipe, error)
	UnpublishRecipe(ctx context.Context, input model.UnpublishInput) error

	GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*model.RecipeWithContent, error)
	ListRecipes(ctx context.Context, filter model.RecipeFilter) (store.Iterator[*model.RecipeInfo], error)
	GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (store.Iterator[*model.RecipeVersion], error)

	ValidateRecipe(ctx context.Context, input model.ValidateInput) (*model.ValidationResult, error)

	// Kept for API compatibility; no-op for Postgres-backed storage.
	SyncFromRemote(ctx context.Context, projectID project.ID) error
}

// ServiceConfig contains dependencies for the service.
type ServiceConfig struct {
	Store store.Store
	// GitRepo is ignored by the Postgres-backed implementation.
	// Field preserved for compatibility with older wiring.
	GitRepo      git.Repository
	Projects     project.Service
	IDGen        model.ShortIDGenerator
	Clock        model.Clock
	CELValidator CELValidator
}

type service struct {
	store        store.Store
	projects     project.Service
	idGen        model.ShortIDGenerator
	clock        model.Clock
	celValidator CELValidator
}

func New(cfg ServiceConfig) (Service, error) {
	if cfg.Store == nil {
		return nil, errors.New("recipe service: store is required")
	}
	if cfg.Projects == nil {
		return nil, errors.New("recipe service: project service is required")
	}
	if cfg.IDGen == nil {
		return nil, errors.New("recipe service: ID generator is required")
	}
	if cfg.CELValidator == nil {
		return nil, errors.New("recipe service: CEL validator is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = model.SystemClock{}
	}
	return &service{
		store:        cfg.Store,
		projects:     cfg.Projects,
		idGen:        cfg.IDGen,
		clock:        cfg.Clock,
		celValidator: cfg.CELValidator,
	}, nil
}

func (s *service) CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error) {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	if err := validateRecipeName(input.Name); err != nil {
		return nil, err
	}

	if input.AutoPublish {
		if err := s.preValidateRecipe(ctx, model.ValidateInput{
			ProjectID: input.ProjectID,
			Name:      input.Name,
			Content:   input.Content,
		}); err != nil {
			return nil, err
		}
	}

	digest := sha256DigestBytes(input.Content)
	now := s.clock.Now().UTC()

	var createdEvent model.RecipeEvent
	var createdOrdinal int64 = 1

	err := s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		tx := txStore.DB()

		// Ensure recipe doesn't exist.
		_, err := s.findRecipeRow(ctx, tx, input.ProjectID, input.Name)
		if err == nil {
			return model.ErrAlreadyExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := upsertBlob(ctx, tx, digest, input.Content); err != nil {
			return err
		}

		recipeID, err := s.idGen.NewID()
		if err != nil {
			return err
		}
		eventID, err := s.idGen.NewID()
		if err != nil {
			return err
		}

		msg := fmt.Sprintf("Create recipe: %s", input.Name)
		if strings.TrimSpace(input.Description) != "" {
			msg += "\n\n" + strings.TrimSpace(input.Description)
		}

		createdEvent = model.RecipeEvent{
			ID:                 eventID,
			ProjectID:          input.ProjectID,
			RecipeID:           recipeID,
			Digest:             digest,
			SavedOrdinal:       &createdOrdinal,
			TargetSavedOrdinal: nil,
			Published:          input.AutoPublish,
			EventAt:            now,
			Message:            &msg,
		}

		if err := tx.WithContext(ctx).Create(&model.RecipeRow{
			ID:                 recipeID,
			ProjectID:          input.ProjectID,
			Name:               input.Name,
			LatestSavedEventID: &eventID,
			LatestSavedOrdinal: &createdOrdinal,
			PublishedEventID:   nil,
			DeletedAt:          nil,
			CreatedAt:          now,
			UpdatedAt:          now,
		}).Error; err != nil {
			if isDuplicateKey(err) {
				return model.ErrAlreadyExists
			}
			return err
		}

		if err := tx.WithContext(ctx).Create(&createdEvent).Error; err != nil {
			return err
		}

		if input.AutoPublish {
			// Publish is represented by this same save event.
			if err := tx.WithContext(ctx).Model(&model.RecipeRow{}).
				Where("id = ?", recipeID).
				Update("published_event_id", eventID).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &model.RecipeVersion{
		Name:        input.Name,
		CommitHash:  formatSavedOrdinalRef(createdOrdinal),
		ShortHash:   formatSavedOrdinalRef(createdOrdinal),
		Author:      "",
		Message:     derefString(createdEvent.Message),
		CreatedAt:   createdEvent.EventAt,
		IsPublished: input.AutoPublish,
	}, nil
}

func (s *service) UpdateRecipe(ctx context.Context, input model.UpdateInput) (*model.RecipeVersion, error) {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	if err := validateRecipeName(input.Name); err != nil {
		return nil, err
	}

	digest := sha256DigestBytes(input.Content)
	now := s.clock.Now().UTC()

	var outVersion *model.RecipeVersion

	err := s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		tx := txStore.DB()

		row, err := s.lockRecipeRow(ctx, tx, input.ProjectID, input.Name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrNotFound
			}
			return err
		}

		// Concurrency check against latest saved.
		if strings.TrimSpace(input.ExpectedCommit) != "" {
			expected := strings.TrimSpace(input.ExpectedCommit)
			expectedOrdinal, ok := parseSavedOrdinalRef(expected)
			if !ok {
				return fmt.Errorf("%w: invalid expected commit %q", model.ErrVersionConflict, expected)
			}
			currentOrdinal := int64(0)
			if row.LatestSavedOrdinal != nil {
				currentOrdinal = *row.LatestSavedOrdinal
			}
			if expectedOrdinal != currentOrdinal {
				return fmt.Errorf("%w: expected %s, got %s",
					model.ErrVersionConflict, formatSavedOrdinalRef(expectedOrdinal), formatSavedOrdinalRef(currentOrdinal))
			}
		}

		latestEvent, err := s.getLatestSavedEvent(ctx, tx, row)
		if err != nil {
			return err
		}

		if bytes.Equal(latestEvent.Digest, digest) {
			// No-op save. Optional auto-publish.
			ordinal := int64(0)
			if latestEvent.SavedOrdinal != nil {
				ordinal = *latestEvent.SavedOrdinal
			}

			isPublished := false
			publishedOrdinal, _, err := s.currentPublishedOrdinal(ctx, tx, row)
			if err != nil {
				return err
			}
			if publishedOrdinal != nil && *publishedOrdinal == ordinal {
				isPublished = true
			}

			if input.AutoPublish && !isPublished {
				var override *string
				if strings.TrimSpace(input.Message) != "" {
					override = &input.Message
				}
				if err := s.publishSavedOrdinalTx(ctx, tx, row, ordinal, digest, now, nil, "Auto-publish recipe", override); err != nil {
					return err
				}
				isPublished = true
			}

			outVersion = &model.RecipeVersion{
				Name:        input.Name,
				CommitHash:  formatSavedOrdinalRef(ordinal),
				ShortHash:   formatSavedOrdinalRef(ordinal),
				Author:      derefString(latestEvent.Actor),
				Message:     derefString(latestEvent.Message),
				CreatedAt:   latestEvent.EventAt,
				IsPublished: isPublished,
			}
			return nil
		}

		if input.AutoPublish {
			if err := s.preValidateRecipe(ctx, model.ValidateInput{
				ProjectID: input.ProjectID,
				Name:      input.Name,
				Content:   input.Content,
			}); err != nil {
				return err
			}
		}

		if err := upsertBlob(ctx, tx, digest, input.Content); err != nil {
			return err
		}

		nextOrdinal := int64(1)
		if row.LatestSavedOrdinal != nil {
			nextOrdinal = *row.LatestSavedOrdinal + 1
		}

		eventID, err := s.idGen.NewID()
		if err != nil {
			return err
		}

		msg := strings.TrimSpace(input.Message)
		if msg == "" {
			msg = fmt.Sprintf("Update recipe: %s", input.Name)
		}

		saveEvent := model.RecipeEvent{
			ID:                 eventID,
			ProjectID:          input.ProjectID,
			RecipeID:           row.ID,
			Digest:             digest,
			SavedOrdinal:       &nextOrdinal,
			Published:          input.AutoPublish,
			EventAt:            now,
			Message:            &msg,
			TargetSavedOrdinal: nil,
		}

		if err := tx.WithContext(ctx).Create(&saveEvent).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"latest_saved_event_id": eventID,
			"latest_saved_ordinal":  nextOrdinal,
			"updated_at":            now,
		}
		if input.AutoPublish {
			updates["published_event_id"] = eventID
		}

		if err := tx.WithContext(ctx).Model(&model.RecipeRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}

		outVersion = &model.RecipeVersion{
			Name:        input.Name,
			CommitHash:  formatSavedOrdinalRef(nextOrdinal),
			ShortHash:   formatSavedOrdinalRef(nextOrdinal),
			Author:      derefString(saveEvent.Actor),
			Message:     derefString(saveEvent.Message),
			CreatedAt:   saveEvent.EventAt,
			IsPublished: input.AutoPublish,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return outVersion, nil
}

func (s *service) DeleteRecipe(ctx context.Context, projectID project.ID, name string) error {
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}
	if err := validateRecipeName(name); err != nil {
		return err
	}

	now := s.clock.Now().UTC()

	return s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		tx := txStore.DB()
		row, err := s.lockRecipeRow(ctx, tx, projectID, name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrNotFound
			}
			return err
		}

		// Insert an unpublish event so @asof behaves sensibly for timestamps after deletion.
		unpublishEventID, err := s.idGen.NewID()
		if err != nil {
			return err
		}
		unpublish := model.RecipeEvent{
			ID:        unpublishEventID,
			ProjectID: projectID,
			RecipeID:  row.ID,
			Digest:    nil,
			Published: false,
			EventAt:   now,
		}
		if err := tx.WithContext(ctx).Create(&unpublish).Error; err != nil {
			return err
		}

		if err := tx.WithContext(ctx).Model(&model.RecipeRow{}).
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"deleted_at":         now,
				"published_event_id": unpublishEventID,
				"updated_at":         now,
			}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (s *service) PublishRecipe(ctx context.Context, input model.PublishInput) (*model.PublishedRecipe, error) {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	if err := validateRecipeName(input.Name); err != nil {
		return nil, err
	}

	now := s.clock.Now().UTC()

	var out *model.PublishedRecipe

	err := s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		tx := txStore.DB()

		row, err := s.lockRecipeRow(ctx, tx, input.ProjectID, input.Name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrNotFound
			}
			return err
		}

		expected := input.ExpectedCommit
		if expected != nil {
			want := strings.TrimSpace(*expected)
			currentRef, err := s.currentPublishedRef(ctx, tx, row)
			if err != nil {
				return err
			}
			if want != "" && want != currentRef {
				return fmt.Errorf("%w: expected %s, current is %s", model.ErrVersionConflict, want, currentRef)
			}
		}

		savedEvent, savedOrdinal, err := s.resolveSavedVersionRefTx(ctx, tx, row, strings.TrimSpace(input.CommitHash))
		if err != nil {
			return err
		}

		if savedEvent == nil || savedOrdinal == 0 {
			return model.ErrNotFound
		}

		content, err := loadBlobContent(ctx, tx, savedEvent.Digest)
		if err != nil {
			return err
		}

		if _, err := s.validateRecipe(ctx, model.ValidateInput{
			ProjectID: input.ProjectID,
			Name:      input.Name,
			Content:   content,
		}); err != nil {
			return err
		}

		if err := s.publishSavedOrdinalTx(ctx, tx, row, savedOrdinal, savedEvent.Digest, now, input.PublishedBy, "Publish recipe", nil); err != nil {
			return err
		}

		out = &model.PublishedRecipe{
			ProjectID:   input.ProjectID,
			Name:        input.Name,
			CommitHash:  formatSavedOrdinalRef(savedOrdinal),
			PublishedAt: now,
			PublishedBy: input.PublishedBy,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) UnpublishRecipe(ctx context.Context, input model.UnpublishInput) error {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return err
	}
	if err := validateRecipeName(input.Name); err != nil {
		return err
	}

	now := s.clock.Now().UTC()

	return s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		tx := txStore.DB()
		row, err := s.lockRecipeRow(ctx, tx, input.ProjectID, input.Name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrNotFound
			}
			return err
		}

		// Optimistic check (best-effort, for API parity).
		if strings.TrimSpace(input.ExpectedCommit) != "" {
			currentRef, err := s.currentPublishedRef(ctx, tx, row)
			if err != nil {
				return err
			}
			if strings.TrimSpace(input.ExpectedCommit) != currentRef {
				return fmt.Errorf("%w: expected %s, current is %s", model.ErrVersionConflict, strings.TrimSpace(input.ExpectedCommit), currentRef)
			}
		}

		eventID, err := s.idGen.NewID()
		if err != nil {
			return err
		}
		unpublish := model.RecipeEvent{
			ID:        eventID,
			ProjectID: input.ProjectID,
			RecipeID:  row.ID,
			Digest:    nil,
			Published: false,
			EventAt:   now,
		}
		if err := tx.WithContext(ctx).Create(&unpublish).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&model.RecipeRow{}).
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"published_event_id": eventID,
				"updated_at":         now,
			}).Error
	})
}

func (s *service) GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*model.RecipeWithContent, error) {
	if err := s.ensureProject(ctx, projectID); err != nil {
		return nil, err
	}
	if err := validateRecipeName(name); err != nil {
		return nil, err
	}

	db := s.store.DB()

	row, err := s.findRecipeRow(ctx, db, projectID, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}

	ref = strings.TrimSpace(ref)

	if asof, ok := parseAsOfRef(ref); ok {
		return s.getRecipeAsOf(ctx, db, row, name, asof)
	}

	if ref == "" {
		// Current published if available, else latest saved.
		pubEvent, pubOrdinal, err := s.currentPublishedEvent(ctx, db, row)
		if err != nil {
			return nil, err
		}
		if pubEvent != nil && pubEvent.Published && pubOrdinal != nil {
			content, err := loadBlobContent(ctx, db, pubEvent.Digest)
			if err != nil {
				return nil, err
			}
			return &model.RecipeWithContent{
				Name:        name,
				CommitHash:  formatSavedOrdinalRef(*pubOrdinal),
				Content:     content,
				IsPublished: true,
				PublishedAt: ptrTime(pubEvent.EventAt),
				PublishedBy: pubEvent.Actor,
			}, nil
		}

		latest, err := s.getLatestSavedEvent(ctx, db, row)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, model.ErrNotFound
			}
			return nil, err
		}
		content, err := loadBlobContent(ctx, db, latest.Digest)
		if err != nil {
			return nil, err
		}
		ordinal := int64(0)
		if latest.SavedOrdinal != nil {
			ordinal = *latest.SavedOrdinal
		}
		return &model.RecipeWithContent{
			Name:        name,
			CommitHash:  formatSavedOrdinalRef(ordinal),
			Content:     content,
			IsPublished: false,
		}, nil
	}

	// Resolve a specific saved version.
	savedEvent, savedOrdinal, err := s.resolveSavedVersionRef(ctx, db, row, ref)
	if err != nil {
		return nil, err
	}
	if savedEvent == nil {
		return nil, model.ErrNotFound
	}
	content, err := loadBlobContent(ctx, db, savedEvent.Digest)
	if err != nil {
		return nil, err
	}

	isPublished := false
	pubOrdinal, pubEvent, err := s.currentPublishedOrdinal(ctx, db, row)
	if err != nil {
		return nil, err
	}
	if pubOrdinal != nil && *pubOrdinal == savedOrdinal {
		isPublished = true
	}

	var publishedAt *time.Time
	var publishedBy *string
	if isPublished && pubEvent != nil {
		publishedAt = ptrTime(pubEvent.EventAt)
		publishedBy = pubEvent.Actor
	}

	return &model.RecipeWithContent{
		Name:        name,
		CommitHash:  formatSavedOrdinalRef(savedOrdinal),
		Content:     content,
		IsPublished: isPublished,
		PublishedAt: publishedAt,
		PublishedBy: publishedBy,
	}, nil
}

func (s *service) SyncFromRemote(ctx context.Context, projectID project.ID) error {
	// Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}
	return nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrTime(t time.Time) *time.Time {
	tt := t.UTC()
	return &tt
}

func (s *service) preValidateRecipe(ctx context.Context, input model.ValidateInput) error {
	_, err := s.validateRecipe(ctx, input)
	return err
}

// publishSavedOrdinalTx inserts a publish-only event and updates the recipe's published pointer.
func (s *service) publishSavedOrdinalTx(
	ctx context.Context,
	tx *gorm.DB,
	row *model.RecipeRow,
	savedOrdinal int64,
	digest []byte,
	now time.Time,
	actor *string,
	defaultMessage string,
	overrideMessage *string,
) error {
	if row == nil {
		return fmt.Errorf("nil recipe row")
	}

	eventID, err := s.idGen.NewID()
	if err != nil {
		return err
	}

	msg := strings.TrimSpace(defaultMessage)
	if overrideMessage != nil && strings.TrimSpace(*overrideMessage) != "" {
		msg = strings.TrimSpace(*overrideMessage)
	}

	publishEvent := model.RecipeEvent{
		ID:                 eventID,
		ProjectID:          row.ProjectID,
		RecipeID:           row.ID,
		Digest:             digest,
		SavedOrdinal:       nil,
		TargetSavedOrdinal: &savedOrdinal,
		Published:          true,
		EventAt:            now,
		Actor:              actor,
		Message:            &msg,
	}

	if err := tx.WithContext(ctx).Create(&publishEvent).Error; err != nil {
		return err
	}

	return tx.WithContext(ctx).Model(&model.RecipeRow{}).
		Where("id = ?", row.ID).
		Updates(map[string]interface{}{
			"published_event_id": eventID,
			"updated_at":         now,
		}).Error
}

func (s *service) currentPublishedEvent(ctx context.Context, db *gorm.DB, row *model.RecipeRow) (*model.RecipeEvent, *int64, error) {
	if row == nil || row.PublishedEventID == nil {
		return nil, nil, nil
	}
	e, err := s.getEventByID(ctx, db, *row.PublishedEventID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	ord, ok := publishedSavedOrdinal(e)
	if !ok {
		return e, nil, nil
	}
	return e, &ord, nil
}

func (s *service) currentPublishedOrdinal(ctx context.Context, db *gorm.DB, row *model.RecipeRow) (*int64, *model.RecipeEvent, error) {
	e, ord, err := s.currentPublishedEvent(ctx, db, row)
	if err != nil {
		return nil, nil, err
	}
	return ord, e, nil
}

func (s *service) currentPublishedRef(ctx context.Context, db *gorm.DB, row *model.RecipeRow) (string, error) {
	ord, _, err := s.currentPublishedOrdinal(ctx, db, row)
	if err != nil {
		return "", err
	}
	if ord == nil {
		return "", nil
	}
	return formatSavedOrdinalRef(*ord), nil
}

func (s *service) resolveSavedVersionRef(ctx context.Context, db *gorm.DB, row *model.RecipeRow, ref string) (*model.RecipeEvent, int64, error) {
	return s.resolveSavedVersionRefTx(ctx, db, row, ref)
}

func (s *service) resolveSavedVersionRefTx(ctx context.Context, db *gorm.DB, row *model.RecipeRow, ref string) (*model.RecipeEvent, int64, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		latest, err := s.getLatestSavedEvent(ctx, db, row)
		if err != nil {
			return nil, 0, err
		}
		if latest.SavedOrdinal == nil {
			return nil, 0, model.ErrNotFound
		}
		return latest, *latest.SavedOrdinal, nil
	}

	if _, ok := parseAsOfRef(ref); ok {
		return nil, 0, fmt.Errorf("%w: invalid ref for publish", model.ErrInvalidContent)
	}

	if ord, ok := parseSavedOrdinalRef(ref); ok {
		e, err := s.getSavedEventByOrdinal(ctx, db, row.ID, ord)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, 0, model.ErrNotFound
			}
			return nil, 0, err
		}
		return e, ord, nil
	}

	if digest, ok := parseSHA256Ref(ref); ok {
		var e model.RecipeEvent
		err := db.WithContext(ctx).
			Where("recipe_id = ? AND digest = ? AND saved_ordinal IS NOT NULL", row.ID, digest).
			Order("saved_ordinal DESC").
			First(&e).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, 0, model.ErrNotFound
			}
			return nil, 0, err
		}
		if e.SavedOrdinal == nil {
			return nil, 0, model.ErrNotFound
		}
		return &e, *e.SavedOrdinal, nil
	}

	if id, ok := parseVerRef(ref); ok {
		e, err := s.getEventByID(ctx, db, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, 0, model.ErrNotFound
			}
			return nil, 0, err
		}
		if e.RecipeID != row.ID {
			return nil, 0, model.ErrNotFound
		}
		if e.SavedOrdinal == nil {
			return nil, 0, model.ErrNotFound
		}
		return e, *e.SavedOrdinal, nil
	}

	return nil, 0, model.ErrNotFound
}

func (s *service) getRecipeAsOf(ctx context.Context, db *gorm.DB, row *model.RecipeRow, name string, asof time.Time) (*model.RecipeWithContent, error) {
	var e model.RecipeEvent
	err := db.WithContext(ctx).
		Where("project_id = ? AND recipe_id = ? AND event_at <= ? AND (published = TRUE OR digest IS NULL)", row.ProjectID, row.ID, asof.UTC()).
		Order("event_at DESC, id DESC").
		First(&e).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrNotPublished
		}
		return nil, err
	}
	if !e.Published {
		return nil, model.ErrNotPublished
	}
	ord, ok := publishedSavedOrdinal(&e)
	if !ok {
		return nil, model.ErrNotPublished
	}
	content, err := loadBlobContent(ctx, db, e.Digest)
	if err != nil {
		return nil, err
	}
	return &model.RecipeWithContent{
		Name:        name,
		CommitHash:  formatSavedOrdinalRef(ord),
		Content:     content,
		IsPublished: true,
		PublishedAt: ptrTime(e.EventAt),
		PublishedBy: e.Actor,
	}, nil
}

var _ Service = (*service)(nil)

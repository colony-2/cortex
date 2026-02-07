package service

import (
	"context"
	"errors"
	"strings"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"gorm.io/gorm"
)

func (s *service) ListRecipes(ctx context.Context, filter model.RecipeFilter) (store.Iterator[*model.RecipeInfo], error) {
	// Validate project exists if filtering by single project.
	if len(filter.ProjectIDs) == 1 {
		if err := s.ensureProject(ctx, filter.ProjectIDs[0]); err != nil {
			return nil, err
		}
	}

	if len(filter.ProjectIDs) == 0 {
		return store.NewSliceIterator([]*model.RecipeInfo{}), nil
	}

	db := s.store.DB().WithContext(ctx)

	var rows []model.RecipeRow
	query := db.Where("deleted_at IS NULL").Where("project_id IN ?", filter.ProjectIDs)

	if len(filter.Names) > 0 {
		query = query.Where("name IN ?", filter.Names)
	}
	if strings.TrimSpace(filter.NamePrefix) != "" {
		query = query.Where("name LIKE ?", strings.TrimSpace(filter.NamePrefix)+"%")
	}

	if err := query.Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	// Batch-load referenced events (latest saved + published pointers).
	eventIDs := make([]string, 0, len(rows)*2)
	for i := range rows {
		if rows[i].LatestSavedEventID != nil {
			eventIDs = append(eventIDs, *rows[i].LatestSavedEventID)
		}
		if rows[i].PublishedEventID != nil {
			eventIDs = append(eventIDs, *rows[i].PublishedEventID)
		}
	}

	eventsByID := map[string]*model.RecipeEvent{}
	if len(eventIDs) > 0 {
		var events []model.RecipeEvent
		if err := db.Where("id IN ?", eventIDs).Find(&events).Error; err != nil {
			return nil, err
		}
		for i := range events {
			e := events[i]
			eventsByID[e.ID] = &e
		}
	}

	out := make([]*model.RecipeInfo, 0, len(rows))
	for i := range rows {
		row := rows[i]
		latestEvent := (*model.RecipeEvent)(nil)
		if row.LatestSavedEventID != nil {
			latestEvent = eventsByID[*row.LatestSavedEventID]
		}
		if latestEvent == nil || latestEvent.SavedOrdinal == nil {
			// Shouldn't happen for non-deleted rows, but be defensive.
			continue
		}

		info := &model.RecipeInfo{
			Name:           row.Name,
			LatestCommit:   formatSavedOrdinalRef(*latestEvent.SavedOrdinal),
			LatestCommitAt: latestEvent.EventAt,
		}

		pubEvent := (*model.RecipeEvent)(nil)
		if row.PublishedEventID != nil {
			pubEvent = eventsByID[*row.PublishedEventID]
		}
		if pubEvent != nil && pubEvent.Published {
			ord, ok := publishedSavedOrdinal(pubEvent)
			if ok {
				ref := formatSavedOrdinalRef(ord)
				info.PublishedCommit = &ref
				at := pubEvent.EventAt.UTC()
				info.PublishedAt = &at
				info.PublishedBy = pubEvent.Actor
			}
		}

		out = append(out, info)
	}

	// Apply publish-status filter (post-filter, since it depends on event pointer semantics).
	filtered := out[:0]
	for _, r := range out {
		switch filter.PublishStatus {
		case model.PublishStatusPublished:
			if r.PublishedCommit == nil {
				continue
			}
		case model.PublishStatusUnpublished:
			if r.PublishedCommit != nil {
				continue
			}
		case model.PublishStatusAll:
		}
		filtered = append(filtered, r)
	}

	return store.NewSliceIterator(filtered), nil
}

func (s *service) GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (store.Iterator[*model.RecipeVersion], error) {
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

	var events []model.RecipeEvent
	if err := db.WithContext(ctx).
		Where("recipe_id = ? AND saved_ordinal IS NOT NULL", row.ID).
		Order("saved_ordinal DESC").
		Find(&events).Error; err != nil {
		return nil, err
	}

	pubOrdinal, _, err := s.currentPublishedOrdinal(ctx, db, row)
	if err != nil {
		return nil, err
	}

	versions := make([]*model.RecipeVersion, 0, len(events))
	for i := range events {
		e := events[i]
		if e.SavedOrdinal == nil {
			continue
		}
		ord := *e.SavedOrdinal
		isPublished := pubOrdinal != nil && *pubOrdinal == ord

		versions = append(versions, &model.RecipeVersion{
			Name:        name,
			CommitHash:  formatSavedOrdinalRef(ord),
			ShortHash:   formatSavedOrdinalRef(ord),
			Author:      derefString(e.Actor),
			Message:     derefString(e.Message),
			CreatedAt:   e.EventAt,
			IsPublished: isPublished,
		})
	}

	return store.NewSliceIterator(versions), nil
}

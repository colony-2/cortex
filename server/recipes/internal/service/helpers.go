package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ensureProject validates that a project exists.
func (s *service) ensureProject(ctx context.Context, projectID project.ID) error {
	_, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return model.ErrInvalidProject
	}
	return nil
}

// validateRecipeName validates a recipe name format.
func validateRecipeName(name string) error {
	if name == "" {
		return model.ErrEmptyName
	}

	segments := strings.Split(name, "/")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("%w: empty path segment", model.ErrInvalidName)
		}
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '_' || r == '-') {
				return fmt.Errorf("%w: invalid character '%c' in segment '%s'",
					model.ErrInvalidName, r, segment)
			}
		}
	}
	return nil
}

func sha256DigestBytes(content []byte) []byte {
	sum := sha256.Sum256(content)
	return sum[:]
}

func digestToRef(digest []byte) string {
	return "sha256:" + hex.EncodeToString(digest)
}

func parseSHA256Ref(ref string) ([]byte, bool) {
	if !strings.HasPrefix(ref, "sha256:") {
		return nil, false
	}
	hexPart := strings.TrimPrefix(ref, "sha256:")
	b, err := hex.DecodeString(hexPart)
	if err != nil || len(b) != 32 {
		return nil, false
	}
	return b, true
}

func parseSavedOrdinalRef(ref string) (int64, bool) {
	if len(ref) < 2 || ref[0] != 'v' {
		return 0, false
	}
	n, err := strconv.ParseInt(ref[1:], 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func formatSavedOrdinalRef(ordinal int64) string {
	return fmt.Sprintf("v%d", ordinal)
}

func parseVerRef(ref string) (string, bool) {
	if strings.HasPrefix(ref, "ver:") {
		id := strings.TrimPrefix(ref, "ver:")
		if id == "" {
			return "", false
		}
		return id, true
	}
	return "", false
}

func parseAsOfRef(ref string) (time.Time, bool) {
	if !strings.HasPrefix(ref, "asof:") {
		return time.Time{}, false
	}
	ts := strings.TrimPrefix(ref, "asof:")
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (s *service) lockRecipeRow(ctx context.Context, tx *gorm.DB, projectID project.ID, name string) (*model.RecipeRow, error) {
	var row model.RecipeRow
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND name = ? AND deleted_at IS NULL", projectID, name).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *service) findRecipeRow(ctx context.Context, db *gorm.DB, projectID project.ID, name string) (*model.RecipeRow, error) {
	var row model.RecipeRow
	err := db.WithContext(ctx).
		Where("project_id = ? AND name = ? AND deleted_at IS NULL", projectID, name).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *service) getEventByID(ctx context.Context, db *gorm.DB, id string) (*model.RecipeEvent, error) {
	var e model.RecipeEvent
	err := db.WithContext(ctx).First(&e, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *service) getSavedEventByOrdinal(ctx context.Context, db *gorm.DB, recipeID string, ordinal int64) (*model.RecipeEvent, error) {
	var e model.RecipeEvent
	err := db.WithContext(ctx).
		Where("recipe_id = ? AND saved_ordinal = ?", recipeID, ordinal).
		First(&e).Error
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *service) getLatestSavedEvent(ctx context.Context, db *gorm.DB, row *model.RecipeRow) (*model.RecipeEvent, error) {
	if row == nil || row.LatestSavedEventID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.getEventByID(ctx, db, *row.LatestSavedEventID)
}

func publishedSavedOrdinal(e *model.RecipeEvent) (int64, bool) {
	if e == nil || !e.Published {
		return 0, false
	}
	if e.TargetSavedOrdinal != nil {
		return *e.TargetSavedOrdinal, true
	}
	if e.SavedOrdinal != nil {
		return *e.SavedOrdinal, true
	}
	return 0, false
}

func upsertBlob(ctx context.Context, db *gorm.DB, digest []byte, content []byte) error {
	if len(digest) != 32 {
		return fmt.Errorf("invalid digest length: %d", len(digest))
	}
	blob := model.RecipeBlob{
		Digest:    digest,
		Algo:      "sha256",
		Content:   content,
		SizeBytes: len(content),
	}
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "digest"}},
			DoNothing: true,
		}).
		Create(&blob).Error
}

func loadBlobContent(ctx context.Context, db *gorm.DB, digest []byte) ([]byte, error) {
	var blob model.RecipeBlob
	err := db.WithContext(ctx).First(&blob, "digest = ?", digest).Error
	if err != nil {
		return nil, err
	}
	return blob.Content, nil
}

func isDuplicateKey(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/model"
)

type AnnotationSetRepository struct{ db *gorm.DB }

func NewAnnotationSetRepository(db *gorm.DB) *AnnotationSetRepository {
	return &AnnotationSetRepository{db: db}
}

func (repository *AnnotationSetRepository) WithDB(db *gorm.DB) *AnnotationSetRepository {
	return &AnnotationSetRepository{db: db}
}

func (repository *AnnotationSetRepository) Create(annotation *model.AnnotationSet) error {
	if err := repository.db.Create(annotation).Error; err != nil {
		return fmt.Errorf("create annotation set: %w", err)
	}
	return nil
}

func (repository *AnnotationSetRepository) Get(id uint) (model.AnnotationSet, error) {
	var annotation model.AnnotationSet
	if err := repository.db.Preload("Dataset").Preload("Schema").Preload("Annotator").First(&annotation, id).Error; err != nil {
		return annotation, fmt.Errorf("get annotation set: %w", err)
	}
	return annotation, nil
}

func (repository *AnnotationSetRepository) ByIDs(ids []uint) ([]model.AnnotationSet, error) {
	var annotations []model.AnnotationSet
	if err := repository.db.Preload("Dataset").Preload("Schema").Preload("Annotator").
		Where("id IN ?", ids).Order("id ASC").Find(&annotations).Error; err != nil {
		return nil, fmt.Errorf("get annotation sets: %w", err)
	}
	return annotations, nil
}

func (repository *AnnotationSetRepository) List(page, pageSize int, datasetID uint, itemKey, state string) ([]model.AnnotationSet, int64, error) {
	query := repository.db.Model(&model.AnnotationSet{})
	if datasetID > 0 {
		query = query.Where("dataset_id = ?", datasetID)
	}
	if itemKey != "" {
		query = query.Where("item_key = ?", itemKey)
	}
	if state != "" {
		query = query.Where("annotation_state = ?", state)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count annotation sets: %w", err)
	}
	var annotations []model.AnnotationSet
	if err := query.Preload("Dataset").Preload("Schema").Preload("Annotator").
		Order("item_key ASC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&annotations).Error; err != nil {
		return nil, 0, fmt.Errorf("list annotation sets: %w", err)
	}
	return annotations, total, nil
}

// PinStatusPage filters result sets by their schema-version pin status, which
// powers the operations-console unresolved backlog.
func (repository *AnnotationSetRepository) PinStatusPage(page, pageSize int, pinStatus string) ([]model.AnnotationSet, int64, error) {
	query := repository.db.Model(&model.AnnotationSet{}).Where("schema_pin_status = ?", pinStatus)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count annotation sets by pin status: %w", err)
	}
	var annotations []model.AnnotationSet
	if err := query.Preload("Dataset").Preload("Schema").Preload("Annotator").
		Order("dataset_id ASC, item_key ASC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&annotations).Error; err != nil {
		return nil, 0, fmt.Errorf("list annotation sets by pin status: %w", err)
	}
	return annotations, total, nil
}

// LegacyGroups returns submitted legacy result sets grouped by the
// dataset/item reconciliation key. Backfill can only fill a group when every
// result set for that item points at the same schema version.
func (repository *AnnotationSetRepository) LegacyGroups() ([]LegacyPinGroup, error) {
	var groups []LegacyPinGroup
	err := repository.db.Model(&model.AnnotationSet{}).
		Select("dataset_id AS dataset_id, item_key AS item_key, COUNT(*) AS row_count, COUNT(DISTINCT schema_id) AS schema_count, MIN(schema_id) AS schema_id").
		Where("schema_pin_status = ? AND submitted_at IS NOT NULL", "legacy").
		Group("dataset_id, item_key").
		Order("dataset_id ASC, item_key ASC").
		Scan(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("load legacy annotation groups: %w", err)
	}
	return groups, nil
}

// LegacyUnresolved returns distinct dataset/item keys for legacy rows that
// cannot be backfilled automatically, after the current backfill pass has
// applied every unambiguous group.
func (repository *AnnotationSetRepository) LegacyUnresolved() ([]LegacyPinGroup, error) {
	var groups []LegacyPinGroup
	err := repository.db.Model(&model.AnnotationSet{}).
		Select("dataset_id AS dataset_id, item_key AS item_key, COUNT(*) AS row_count").
		Where("schema_pin_status = ?", "unresolved").
		Group("dataset_id, item_key").
		Order("dataset_id ASC, item_key ASC").
		Scan(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("load unresolved annotation groups: %w", err)
	}
	return groups, nil
}

// BackfillGroup marks every still-legacy row in a reconciliation group. A
// single ambiguous version isolates the whole item so scores never mix
// schema revisions silently.
func (repository *AnnotationSetRepository) BackfillGroup(datasetID uint, itemKey string, updates map[string]any) error {
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("dataset_id = ? AND item_key = ? AND schema_pin_status = ?", datasetID, itemKey, "legacy").
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("backfill annotation pin group: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}

// PinOnSubmit records the schema version and source checksum that were in
// effect when the result left the draft state. The conditional state predicate
// guarantees a retry after a failed publish never pins a non-submitted row.
func (repository *AnnotationSetRepository) PinOnSubmit(id uint, schemaCode string, schemaVersion int, checksum string) (time.Time, error) {
	var annotation model.AnnotationSet
	if err := repository.db.First(&annotation, id).Error; err != nil {
		return time.Time{}, fmt.Errorf("load submitted annotation for pin: %w", err)
	}
	if annotation.SubmittedAt == nil {
		return time.Time{}, ErrStateConflict
	}
	pinnedAt := *annotation.SubmittedAt
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("id = ? AND annotation_state = ? AND schema_pin_status <> ?", id, "submitted", "pinned").
		Updates(map[string]any{
			"pinned_schema_code":     schemaCode,
			"pinned_schema_version":  schemaVersion,
			"pinned_source_checksum": checksum,
			"pinned_at":              pinnedAt,
			"schema_pin_status":      "pinned",
			"backfill_note":          "",
		})
	if result.Error != nil {
		return time.Time{}, fmt.Errorf("pin annotation on submit: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return time.Time{}, ErrStateConflict
	}
	return pinnedAt, nil
}

// LegacyPinGroup is a dataset/item aggregation of unpinned result sets.
type LegacyPinGroup struct {
	DatasetID   uint   `gorm:"column:dataset_id"`
	ItemKey     string `gorm:"column:item_key"`
	RowCount    int    `gorm:"column:row_count"`
	SchemaCount int    `gorm:"column:schema_count"`
	SchemaID    uint   `gorm:"column:schema_id"`
}

func (repository *AnnotationSetRepository) UpdateDraft(id, annotatorID uint, expectedUpdatedAt time.Time, labelsJSON, qualityNote string) error {
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("id = ? AND annotator_id = ? AND annotation_state = ? AND updated_at = ?", id, annotatorID, "draft", expectedUpdatedAt).
		Updates(map[string]any{"labels_json": labelsJSON, "quality_note": qualityNote})
	if result.Error != nil {
		return fmt.Errorf("update annotation draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

func (repository *AnnotationSetRepository) Transition(id uint, from, to string) error {
	updates := map[string]any{"annotation_state": to}
	if to == "submitted" {
		updates["submitted_at"] = time.Now().UTC()
	}
	result := repository.db.Model(&model.AnnotationSet{}).Where("id = ? AND annotation_state = ?", id, from).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition annotation set: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

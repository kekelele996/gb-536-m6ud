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

// PinOnSubmit snapshots the schema version that was effective at submission and
// freezes the submitted source checksum. Submission is the binding point.
func (repository *AnnotationSetRepository) PinOnSubmit(id uint, schemaID *uint, schemaCode string, schemaVersion int, submittedChecksum string) error {
	now := time.Now().UTC()
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("id = ? AND annotation_state = ?", id, "draft").
		Updates(map[string]any{
			"annotation_state":          "submitted",
			"submitted_at":              now,
			"pinned_at":                 now,
			"pinned_schema_id":          schemaID,
			"schema_code":               schemaCode,
			"schema_version":            schemaVersion,
			"submitted_source_checksum": submittedChecksum,
			"schema_binding_state":      "current",
		})
	if result.Error != nil {
		return fmt.Errorf("pin schema on annotation submit: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// BackfillSchemaBinding fills the effective version snapshot on legacy rows.
func (repository *AnnotationSetRepository) BackfillSchemaBinding(id, schemaID uint, schemaCode string, schemaVersion int, bindingState string) error {
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("id = ? AND schema_version = 0", id).
		Updates(map[string]any{
			"schema_id":            schemaID,
			"schema_code":          schemaCode,
			"schema_version":       schemaVersion,
			"pinned_schema_id":     schemaID,
			"schema_binding_state": bindingState,
		})
	if result.Error != nil {
		return fmt.Errorf("backfill annotation schema binding: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

func (repository *AnnotationSetRepository) MarkUnresolvedBinding(id uint) error {
	result := repository.db.Model(&model.AnnotationSet{}).
		Where("id = ?", id).
		Updates(map[string]any{"schema_binding_state": "unresolved"})
	if result.Error != nil {
		return fmt.Errorf("mark unresolved annotation binding: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// LegacyRowsMissingVersion returns rows created before effective schema versions
// were recorded. Backfill keys these rows by dataset and item key.
func (repository *AnnotationSetRepository) LegacyRowsMissingVersion() ([]model.AnnotationSet, error) {
	var annotations []model.AnnotationSet
	if err := repository.db.Preload("Dataset").Preload("Schema").
		Where("schema_version = 0 OR schema_code = ''").
		Order("dataset_id ASC, item_key ASC, id ASC").Find(&annotations).Error; err != nil {
		return nil, fmt.Errorf("list legacy annotation rows: %w", err)
	}
	return annotations, nil
}

// UniqueSchemasForItemKey returns the distinct schema ids that annotated a given
// item, ordered for deterministic backfill.
func (repository *AnnotationSetRepository) UniqueSchemasForItemKey(datasetID uint, itemKey string) ([]model.AnnotationSet, error) {
	var annotations []model.AnnotationSet
	if err := repository.db.Where("dataset_id = ? AND item_key = ? AND schema_id > 0", datasetID, itemKey).
		Order("schema_id ASC").Find(&annotations).Error; err != nil {
		return nil, fmt.Errorf("resolve item schemas: %w", err)
	}
	return annotations, nil
}

// ByDatasetAndItemKey loads every result set for a dataset item during
// reconciliation between the schema center and the annotation console.
func (repository *AnnotationSetRepository) ByDatasetAndItemKey(datasetID uint, itemKey string) ([]model.AnnotationSet, error) {
	var annotations []model.AnnotationSet
	if err := repository.db.Preload("Dataset").Preload("Schema").Preload("Annotator").
		Where("dataset_id = ? AND item_key = ?", datasetID, itemKey).
		Order("id ASC").Find(&annotations).Error; err != nil {
		return nil, fmt.Errorf("list annotations by item: %w", err)
	}
	return annotations, nil
}

// ItemKeysForDataset returns distinct item keys registered on the console side.
func (repository *AnnotationSetRepository) ItemKeysForDataset(datasetID uint) ([]string, error) {
	var keys []string
	if err := repository.db.Model(&model.AnnotationSet{}).
		Where("dataset_id = ?", datasetID).Distinct().Pluck("item_key", &keys).Error; err != nil {
		return nil, fmt.Errorf("list item keys: %w", err)
	}
	return keys, nil
}

package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/model"
)

type AnnotationSchemaRepository struct{ db *gorm.DB }

func NewAnnotationSchemaRepository(db *gorm.DB) *AnnotationSchemaRepository {
	return &AnnotationSchemaRepository{db: db}
}

func (repository *AnnotationSchemaRepository) WithDB(db *gorm.DB) *AnnotationSchemaRepository {
	return &AnnotationSchemaRepository{db: db}
}

func (repository *AnnotationSchemaRepository) Create(schema *model.AnnotationSchema) error {
	if err := repository.db.Create(schema).Error; err != nil {
		return fmt.Errorf("create annotation schema: %w", err)
	}
	return nil
}

func (repository *AnnotationSchemaRepository) Get(id uint) (model.AnnotationSchema, error) {
	var schema model.AnnotationSchema
	if err := repository.db.Preload("Dataset").First(&schema, id).Error; err != nil {
		return schema, fmt.Errorf("get annotation schema: %w", err)
	}
	return schema, nil
}

func (repository *AnnotationSchemaRepository) List(page, pageSize int, datasetID uint, state, schemaCode string) ([]model.AnnotationSchema, int64, error) {
	query := repository.db.Model(&model.AnnotationSchema{})
	if datasetID > 0 {
		query = query.Where("dataset_id = ?", datasetID)
	}
	if state != "" {
		query = query.Where("schema_state = ?", state)
	}
	if schemaCode != "" {
		query = query.Where("schema_code = ?", schemaCode)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count annotation schemas: %w", err)
	}
	var schemas []model.AnnotationSchema
	if err := query.Preload("Dataset").Order("dataset_id ASC, schema_code ASC, version DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&schemas).Error; err != nil {
		return nil, 0, fmt.Errorf("list annotation schemas: %w", err)
	}
	return schemas, total, nil
}

func (repository *AnnotationSchemaRepository) PublishedForDataset(datasetID uint) ([]model.AnnotationSchema, error) {
	var schemas []model.AnnotationSchema
	if err := repository.db.Preload("Dataset").Where("dataset_id = ? AND schema_state = ?", datasetID, "published").
		Order("schema_code ASC, version DESC").Find(&schemas).Error; err != nil {
		return nil, fmt.Errorf("list published annotation schemas: %w", err)
	}
	return schemas, nil
}

// CurrentPublishedForCode returns the active published version of a schema code
// for a dataset, which is the "current schema" re-annotation must target.
func (repository *AnnotationSchemaRepository) CurrentPublishedForCode(datasetID uint, schemaCode string) (model.AnnotationSchema, error) {
	var schema model.AnnotationSchema
	if err := repository.db.Where("dataset_id = ? AND schema_code = ? AND schema_state = ?", datasetID, schemaCode, "published").
		Order("version DESC").First(&schema).Error; err != nil {
		return schema, fmt.Errorf("find current published schema: %w", err)
	}
	return schema, nil
}

// VersionByCode resolves a specific released schema version independently of its
// current state. Deprecated versions remain resolvable so pinned scores stay
// explainable after deprecation.
func (repository *AnnotationSchemaRepository) VersionByCode(datasetID uint, schemaCode string, version int) (model.AnnotationSchema, error) {
	var schema model.AnnotationSchema
	if err := repository.db.Where("dataset_id = ? AND schema_code = ? AND version = ?", datasetID, schemaCode, version).
		First(&schema).Error; err != nil {
		return schema, fmt.Errorf("find schema version by code: %w", err)
	}
	return schema, nil
}

// CountAnnotationsOnSchema counts operation-console result sets bound to the
// given schema version. The schema center reads this for impact reporting only;
// deprecation never deletes the rows.
func (repository *AnnotationSchemaRepository) CountAnnotationsOnSchema(schemaID uint) (int64, error) {
	var total int64
	if err := repository.db.Model(&model.AnnotationSet{}).
		Where("schema_id = ? OR pinned_schema_id = ?", schemaID, schemaID).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count annotations on schema: %w", err)
	}
	return total, nil
}

func (repository *AnnotationSchemaRepository) UpdateDraft(schema *model.AnnotationSchema, expectedUpdatedAt time.Time) error {
	result := repository.db.Model(&model.AnnotationSchema{}).
		Where("id = ? AND version = ? AND schema_state = ? AND updated_at = ?", schema.ID, schema.Version, "draft", expectedUpdatedAt).
		Updates(map[string]any{
			"label_definitions_json": schema.LabelDefinitionsJSON,
			"span_policy":            schema.SpanPolicy, "overlap_policy": schema.OverlapPolicy,
			"examples_json": schema.ExamplesJSON,
		})
	if result.Error != nil {
		return fmt.Errorf("update annotation schema: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (repository *AnnotationSchemaRepository) Transition(id uint, from, to string) error {
	updates := map[string]any{"schema_state": to}
	if to == "published" {
		updates["published_at"] = time.Now().UTC()
	}
	result := repository.db.Model(&model.AnnotationSchema{}).Where("id = ? AND schema_state = ?", id, from).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition annotation schema: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

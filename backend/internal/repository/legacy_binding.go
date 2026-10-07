package repository

import (
	"fmt"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/model"
)

type LegacyBindingRepository struct{ db *gorm.DB }

func NewLegacyBindingRepository(db *gorm.DB) *LegacyBindingRepository {
	return &LegacyBindingRepository{db: db}
}

func (repository *LegacyBindingRepository) WithDB(db *gorm.DB) *LegacyBindingRepository {
	return &LegacyBindingRepository{db: db}
}

func (repository *LegacyBindingRepository) Upsert(binding *model.LegacySchemaBinding) error {
	var existing model.LegacySchemaBinding
	err := repository.db.Where("annotation_set_id = ?", binding.AnnotationSetID).First(&existing).Error
	if err == nil {
		binding.ID = existing.ID
		binding.CreatedAt = existing.CreatedAt
		return repository.db.Model(&existing).Updates(map[string]any{
			"dataset_id": binding.DatasetID, "dataset_code": binding.DatasetCode,
			"item_key": binding.ItemKey, "annotator_id": binding.AnnotatorID,
			"reason": binding.Reason, "resolved": binding.Resolved,
		}).Error
	}
	if err := repository.db.Create(binding).Error; err != nil {
		return fmt.Errorf("record legacy schema binding: %w", err)
	}
	return nil
}

func (repository *LegacyBindingRepository) List(resolved *bool) ([]model.LegacySchemaBinding, error) {
	var bindings []model.LegacySchemaBinding
	query := repository.db.Model(&model.LegacySchemaBinding{})
	if resolved != nil {
		query = query.Where("resolved = ?", *resolved)
	}
	if err := query.Order("dataset_code ASC, item_key ASC, annotation_set_id ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list legacy schema bindings: %w", err)
	}
	return bindings, nil
}

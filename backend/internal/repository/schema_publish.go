package repository

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/model"
)

type SchemaPublishRepository struct{ db *gorm.DB }

func NewSchemaPublishRepository(db *gorm.DB) *SchemaPublishRepository {
	return &SchemaPublishRepository{db: db}
}

func (repository *SchemaPublishRepository) WithDB(db *gorm.DB) *SchemaPublishRepository {
	return &SchemaPublishRepository{db: db}
}

func (repository *SchemaPublishRepository) LatestForSchema(schemaID uint) (model.SchemaPublishAttempt, error) {
	var attempt model.SchemaPublishAttempt
	if err := repository.db.Where("schema_id = ?", schemaID).Order("attempt_no DESC").First(&attempt).Error; err != nil {
		return attempt, fmt.Errorf("get latest publish attempt: %w", err)
	}
	return attempt, nil
}

// RecordAttempt appends a schema-center-only release attempt. The attempt number
// is derived from the prior ledger rows so retries form an explicit sequence.
func (repository *SchemaPublishRepository) RecordAttempt(schemaID, actorID uint, status, failureReason string) (model.SchemaPublishAttempt, error) {
	var latest model.SchemaPublishAttempt
	findErr := repository.db.Where("schema_id = ?", schemaID).Order("attempt_no DESC").First(&latest).Error
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return model.SchemaPublishAttempt{}, fmt.Errorf("load publish ledger: %w", findErr)
	}
	attempt := model.SchemaPublishAttempt{
		SchemaID: schemaID, AttemptNo: latest.AttemptNo + 1,
		Status: status, FailureReason: failureReason, AttemptedBy: actorID,
	}
	if status == "succeeded" {
		publishedAt := time.Now().UTC()
		attempt.PublishedAt = &publishedAt
	}
	if err := repository.db.Create(&attempt).Error; err != nil {
		return attempt, fmt.Errorf("record publish attempt: %w", err)
	}
	return attempt, nil
}

func (repository *SchemaPublishRepository) ListBySchema(schemaID uint) ([]model.SchemaPublishAttempt, error) {
	var attempts []model.SchemaPublishAttempt
	if err := repository.db.Where("schema_id = ?", schemaID).Order("attempt_no DESC").Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("list publish attempts: %w", err)
	}
	return attempts, nil
}

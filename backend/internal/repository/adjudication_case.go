package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/model"
)

type AdjudicationCaseRepository struct{ db *gorm.DB }

func NewAdjudicationCaseRepository(db *gorm.DB) *AdjudicationCaseRepository {
	return &AdjudicationCaseRepository{db: db}
}

func (repository *AdjudicationCaseRepository) WithDB(db *gorm.DB) *AdjudicationCaseRepository {
	return &AdjudicationCaseRepository{db: db}
}

func (repository *AdjudicationCaseRepository) Create(adjudication *model.AdjudicationCase) error {
	if err := repository.db.Create(adjudication).Error; err != nil {
		return fmt.Errorf("create adjudication case: %w", err)
	}
	return nil
}

func (repository *AdjudicationCaseRepository) Get(id uint) (model.AdjudicationCase, error) {
	var adjudication model.AdjudicationCase
	if err := repository.db.Preload("Dataset").First(&adjudication, id).Error; err != nil {
		return adjudication, fmt.Errorf("get adjudication case: %w", err)
	}
	return adjudication, nil
}

func (repository *AdjudicationCaseRepository) List(page, pageSize int, datasetID uint, state, disagreementType, clusterKey string) ([]model.AdjudicationCase, int64, error) {
	query := repository.db.Model(&model.AdjudicationCase{})
	if datasetID > 0 {
		query = query.Where("dataset_id = ?", datasetID)
	}
	if state != "" {
		query = query.Where("case_state = ?", state)
	}
	if disagreementType != "" {
		query = query.Where("disagreement_type = ?", disagreementType)
	}
	if clusterKey != "" {
		query = query.Where("cluster_key = ?", clusterKey)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count adjudication cases: %w", err)
	}
	var cases []model.AdjudicationCase
	if err := query.Preload("Dataset").Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&cases).Error; err != nil {
		return nil, 0, fmt.Errorf("list adjudication cases: %w", err)
	}
	return cases, total, nil
}

func (repository *AdjudicationCaseRepository) FindByIdempotencyKey(key string) (model.AdjudicationCase, error) {
	var adjudication model.AdjudicationCase
	if err := repository.db.Preload("Dataset").Where("idempotency_key = ?", key).First(&adjudication).Error; err != nil {
		return adjudication, fmt.Errorf("find idempotent adjudication: %w", err)
	}
	return adjudication, nil
}

func (repository *AdjudicationCaseRepository) LatestByInput(inputHash, algorithmVersion string) (model.AdjudicationCase, error) {
	var adjudication model.AdjudicationCase
	if err := repository.db.Preload("Dataset").
		Where("input_hash = ? AND algorithm_version = ?", inputHash, algorithmVersion).
		Order("id DESC").First(&adjudication).Error; err != nil {
		return adjudication, fmt.Errorf("find computed adjudication: %w", err)
	}
	return adjudication, nil
}

func (repository *AdjudicationCaseRepository) Assign(id uint, actorID uint) error {
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state IN ?", id, []string{"open", "reopened"}).
		Updates(map[string]any{"case_state": "assigned", "adjudicator_id": actorID})
	if result.Error != nil {
		return fmt.Errorf("assign adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

func (repository *AdjudicationCaseRepository) Decide(id, actorID uint, labelsJSON, rationale, idempotencyKey string) error {
	now := time.Now().UTC()
	key := idempotencyKey
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state = ? AND adjudicator_id = ?", id, "assigned", actorID).
		Updates(map[string]any{
			"case_state": "adjudicated", "final_labels_json": labelsJSON,
			"rationale": rationale, "decided_at": now, "decision_idempotency_key": &key,
		})
	if result.Error != nil {
		return fmt.Errorf("decide adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

func (repository *AdjudicationCaseRepository) Review(id uint, from, to string, reviewerID uint, rationale string) error {
	updates := map[string]any{"case_state": to, "reviewed_by": reviewerID}
	if rationale != "" {
		updates["rationale"] = gorm.Expr("rationale || ?", "\nReview: "+rationale)
	}
	if to == "reopened" {
		updates["reopen_count"] = gorm.Expr("reopen_count + 1")
		updates["adjudicator_id"] = nil
	}
	result := repository.db.Model(&model.AdjudicationCase{}).Where("id = ? AND case_state = ?", id, from).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("review adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// Suspend places an unresolved case on hold. Suspension keeps the adjudication
// record and score intact; it is never a deletion.
func (repository *AdjudicationCaseRepository) Suspend(id uint, fromState, reason string) error {
	now := time.Now().UTC()
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state = ?", id, fromState).
		Updates(map[string]any{"case_state": "suspended", "suspended_reason": reason, "suspended_at": now})
	if result.Error != nil {
		return fmt.Errorf("suspend adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// Resume returns a suspended case to the open queue after the schema center and
// annotation console are reconciled.
func (repository *AdjudicationCaseRepository) Resume(id uint) error {
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state = ?", id, "suspended").
		Updates(map[string]any{"case_state": "open", "suspended_reason": "", "suspended_at": nil})
	if result.Error != nil {
		return fmt.Errorf("resume adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// ByDatasetAndItemKey returns cases for a dataset/item reconciliation key.
func (repository *AdjudicationCaseRepository) ByDatasetAndItemKey(datasetID uint, itemKey string) ([]model.AdjudicationCase, error) {
	var cases []model.AdjudicationCase
	if err := repository.db.Preload("Dataset").
		Where("dataset_id = ? AND item_key = ?", datasetID, itemKey).
		Order("id ASC").Find(&cases).Error; err != nil {
		return nil, fmt.Errorf("list cases by item: %w", err)
	}
	return cases, nil
}

// OrphanItemKeys returns case (dataset_code,item_key) pairs that do not resolve
// to any console result set. Reconciliation suspends these cases.
func (repository *AdjudicationCaseRepository) All() ([]model.AdjudicationCase, error) {
	var cases []model.AdjudicationCase
	if err := repository.db.Preload("Dataset").Order("id ASC").Find(&cases).Error; err != nil {
		return nil, fmt.Errorf("list all adjudication cases: %w", err)
	}
	return cases, nil
}

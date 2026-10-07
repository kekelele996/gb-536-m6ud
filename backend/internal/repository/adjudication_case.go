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

// OpenCases returns cases that reconciliation must inspect. Accepted cases
// are historical records and suspended cases are already on hold.
func (repository *AdjudicationCaseRepository) OpenCases(datasetID uint, itemKey string) ([]model.AdjudicationCase, error) {
	query := repository.db.Model(&model.AdjudicationCase{}).
		Where("case_state <> ? AND case_state <> ?", "accepted", "suspended")
	if datasetID > 0 {
		query = query.Where("dataset_id = ?", datasetID)
	}
	if itemKey != "" {
		query = query.Where("item_key = ?", itemKey)
	}
	var cases []model.AdjudicationCase
	if err := query.Preload("Dataset").Order("id ASC").Find(&cases).Error; err != nil {
		return nil, fmt.Errorf("list open adjudication cases: %w", err)
	}
	return cases, nil
}

// Suspend moves an open case onto reconciliation hold, remembering the state
// to restore once the two sides agree again. Deprecated-schema holds never
// auto-resume because deprecated data must be re-annotated, not unblocked.
func (repository *AdjudicationCaseRepository) Suspend(id uint, previous, reasonCode, reason string, at time.Time) error {
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state = ?", id, previous).
		Updates(map[string]any{
			"case_state":           "suspended",
			"state_before_suspend": previous,
			"suspend_reason_code":  reasonCode,
			"suspend_reason":       reason,
			"suspended_at":         at,
		})
	if result.Error != nil {
		return fmt.Errorf("suspend adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

// Resume releases a reconciliation hold and restores the remembered state.
func (repository *AdjudicationCaseRepository) Resume(id uint, previous string, at time.Time) error {
	result := repository.db.Model(&model.AdjudicationCase{}).
		Where("id = ? AND case_state = ? AND state_before_suspend = ?", id, "suspended", previous).
		Updates(map[string]any{
			"case_state":           previous,
			"state_before_suspend": "",
			"suspend_reason_code":  "",
			"suspend_reason":       "",
			"suspended_at":         nil,
		})
	if result.Error != nil {
		return fmt.Errorf("resume adjudication case: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStateConflict
	}
	return nil
}

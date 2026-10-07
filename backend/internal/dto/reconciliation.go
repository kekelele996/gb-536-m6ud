package dto

import "time"

type ReconcileRequest struct {
	DatasetID uint   `json:"dataset_id" validate:"required"`
	ItemKey   string `json:"item_key" validate:"required,min=2,max=120"`
}

type ReconcileFinding struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	AdjudicationID  uint   `json:"adjudication_id,omitempty"`
	AnnotationSetID uint   `json:"annotation_set_id,omitempty"`
	ExpectedDataset string `json:"expected_dataset_code,omitempty"`
	ExpectedItemKey string `json:"expected_item_key,omitempty"`
}

type ReconcileResponse struct {
	DatasetID      uint               `json:"dataset_id"`
	DatasetCode    string             `json:"dataset_code"`
	ItemKey        string             `json:"item_key"`
	AnnotationSets []ReconciledSet    `json:"annotations"`
	Cases          []ReconciledCase   `json:"cases"`
	Suspended      []uint             `json:"suspended_case_ids"`
	Findings       []ReconcileFinding `json:"findings"`
	Matched        bool               `json:"matched"`
	ReconciledAt   time.Time          `json:"reconciled_at"`
}

type ReconciledSet struct {
	ID            uint   `json:"id"`
	Annotator     string `json:"annotator"`
	SchemaCode    string `json:"schema_code"`
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"annotation_state"`
	BindingState  string `json:"schema_binding_state"`
}

type ReconciledCase struct {
	ID              uint    `json:"id"`
	CaseState       string  `json:"case_state"`
	SchemaCode      string  `json:"schema_code"`
	SchemaVersion   int     `json:"schema_version"`
	Score           float64 `json:"agreement_score"`
	SuspendedReason string  `json:"suspended_reason,omitempty"`
}

type ResumeRequest struct {
	Note string `json:"note" validate:"required,min=8,max=500"`
}

type LegacyBindingResponse struct {
	AnnotationSetID uint      `json:"annotation_set_id"`
	DatasetID       uint      `json:"dataset_id"`
	DatasetCode     string    `json:"dataset_code"`
	ItemKey         string    `json:"item_key"`
	AnnotatorID     uint      `json:"annotator_id"`
	Reason          string    `json:"reason"`
	Resolved        bool      `json:"resolved"`
	CreatedAt       time.Time `json:"created_at"`
}

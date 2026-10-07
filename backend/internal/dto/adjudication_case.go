package dto

import "time"

type AgreementDetails struct {
	Metric            string  `json:"metric"`
	Score             float64 `json:"score"`
	ObservedAgreement float64 `json:"observed_agreement"`
	ChanceAgreement   float64 `json:"chance_agreement"`
	SampleSize        int     `json:"sample_size"`
	CoderCount        int     `json:"coder_count"`
	MissingValueCount int     `json:"missing_value_count"`
	Applicability     string  `json:"applicability"`
}

type ConfusionCell struct {
	LeftLabel  string `json:"left_label"`
	RightLabel string `json:"right_label"`
	Count      int    `json:"count"`
}

type DiffEvidence struct {
	Type       string `json:"type"`
	UnitKey    string `json:"unit_key"`
	LeftLabel  string `json:"left_label,omitempty"`
	RightLabel string `json:"right_label,omitempty"`
	LeftStart  int    `json:"left_start,omitempty"`
	LeftEnd    int    `json:"left_end,omitempty"`
	RightStart int    `json:"right_start,omitempty"`
	RightEnd   int    `json:"right_end,omitempty"`
	Overlap    int    `json:"overlap,omitempty"`
	Evidence   string `json:"evidence"`
}

type ComputeAdjudicationRequest struct {
	DatasetID        uint   `json:"dataset_id" validate:"required"`
	ItemKey          string `json:"item_key" validate:"required,min=2,max=120"`
	AnnotationSetIDs []uint `json:"annotation_set_ids" validate:"required,min=2,dive,gt=0"`
	Metric           string `json:"metric" validate:"omitempty,oneof=auto cohen_kappa krippendorff_alpha"`
}

type AssignCaseRequest struct {
	Note string `json:"note" validate:"required,min=4,max=500"`
}

type AdjudicateCaseRequest struct {
	FinalLabels []AnnotationLabel `json:"final_labels" validate:"required,min=1,dive"`
	Rationale   string            `json:"rationale" validate:"required,min=12,max=1200"`
}

type CaseReviewRequest struct {
	Note string `json:"note" validate:"required,min=8,max=800"`
}

type AdjudicationCaseResponse struct {
	ID                     uint              `json:"id"`
	DatasetID              uint              `json:"dataset_id"`
	DatasetCode            string            `json:"dataset_code"`
	ItemKey                string            `json:"item_key"`
	AnnotationSetIDs       []uint            `json:"annotation_set_ids"`
	Agreement              AgreementDetails  `json:"agreement"`
	DisagreementType       string            `json:"disagreement_type"`
	ConfusionSnapshot      []ConfusionCell   `json:"confusion_snapshot"`
	EvidenceSnapshot       []DiffEvidence    `json:"evidence_snapshot"`
	ClusterKey             string            `json:"cluster_key"`
	CaseState              string            `json:"case_state"`
	FinalLabels            []AnnotationLabel `json:"final_labels"`
	Rationale              string            `json:"rationale"`
	AdjudicatorID          *uint             `json:"adjudicator_id"`
	ReviewedBy             *uint             `json:"reviewed_by"`
	DecidedAt              *time.Time        `json:"decided_at"`
	InputHash              string            `json:"input_hash"`
	AlgorithmVersion       string            `json:"algorithm_version"`
	IdempotencyKey         string            `json:"idempotency_key"`
	DecisionIdempotencyKey *string           `json:"decision_idempotency_key"`
	ReopenCount            int               `json:"reopen_count"`
	SuspendReasonCode      string            `json:"suspend_reason_code"`
	SuspendReason          string            `json:"suspend_reason"`
	StateBeforeSuspend     string            `json:"state_before_suspend"`
	SuspendedAt            *time.Time        `json:"suspended_at"`
	CreatedBy              uint              `json:"created_by"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	Reused                 bool              `json:"reused"`
}

type ReconcileRequest struct {
	DatasetID uint   `json:"dataset_id"`
	ItemKey   string `json:"item_key" validate:"omitempty,min=2,max=120"`
}

type ResumeCaseRequest struct {
	Note string `json:"note" validate:"required,min=8,max=500"`
}

type ReconciliationFinding struct {
	CaseID        uint   `json:"case_id"`
	DatasetID     uint   `json:"dataset_id"`
	DatasetCode   string `json:"dataset_code"`
	ItemKey       string `json:"item_key"`
	Action        string `json:"action"`
	ReasonCode    string `json:"reason_code"`
	Reason        string `json:"reason"`
	PreviousState string `json:"previous_state,omitempty"`
}

type ReconciliationRunResponse struct {
	ID             uint                    `json:"id"`
	Scope          string                  `json:"scope"`
	DatasetID      uint                    `json:"dataset_id"`
	ItemKey        string                  `json:"item_key"`
	CasesChecked   int                     `json:"cases_checked"`
	CasesSuspended int                     `json:"cases_suspended"`
	CasesResumed   int                     `json:"cases_resumed"`
	OpenMismatches int                     `json:"open_mismatches"`
	Findings       []ReconciliationFinding `json:"findings"`
	TriggeredBy    uint                    `json:"triggered_by"`
	CompletedAt    time.Time               `json:"completed_at"`
	CreatedAt      time.Time               `json:"created_at"`
}

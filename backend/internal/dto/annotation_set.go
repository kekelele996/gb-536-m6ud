package dto

import "time"

type AnnotationLabel struct {
	UnitKey string `json:"unit_key" validate:"required,min=1,max=120"`
	Label   string `json:"label" validate:"required,min=1,max=40"`
	Start   int    `json:"start,omitempty" validate:"gte=0"`
	End     int    `json:"end,omitempty" validate:"gte=0"`
}

func (label AnnotationLabel) IsSpan() bool { return label.End > label.Start }

type CreateAnnotationSetRequest struct {
	DatasetID      uint              `json:"dataset_id" validate:"required"`
	SchemaID       uint              `json:"schema_id" validate:"required"`
	ItemKey        string            `json:"item_key" validate:"required,min=2,max=120"`
	Labels         []AnnotationLabel `json:"labels" validate:"required,min=1,dive"`
	SourceChecksum string            `json:"source_checksum" validate:"required,len=64,hexadecimal"`
	QualityNote    string            `json:"quality_note" validate:"max=600"`
	// SupersedesID points at the prior compared result set when re-annotating an
	// item after its pinned schema version was deprecated. The prior set is kept
	// on record; SchemaID must reference the current published version.
	SupersedesID *uint `json:"supersedes_id"`
}

type UpdateAnnotationSetRequest struct {
	Labels      []AnnotationLabel `json:"labels" validate:"required,min=1,dive"`
	QualityNote string            `json:"quality_note" validate:"max=600"`
}

type AnnotationTransitionRequest struct {
	TargetState string `json:"target_state" validate:"required"`
	Reason      string `json:"reason" validate:"max=300"`
	// SubmittedSourceChecksum binds the source checksum effective at submit time.
	// Drafts record the working checksum; submission freezes this value.
	SubmittedSourceChecksum string `json:"submitted_source_checksum" validate:"omitempty,len=64,hexadecimal"`
}

type AnnotationSetResponse struct {
	ID                      uint              `json:"id"`
	DatasetID               uint              `json:"dataset_id"`
	DatasetCode             string            `json:"dataset_code"`
	SchemaID                uint              `json:"schema_id"`
	SchemaCode              string            `json:"schema_code"`
	SchemaVersion           int               `json:"schema_version"`
	EffectiveSchemaCode     string            `json:"effective_schema_code"`
	EffectiveSchemaVersion  int               `json:"effective_schema_version"`
	SchemaBindingState      string            `json:"schema_binding_state"`
	SchemaCurrent           bool              `json:"schema_current"`
	PinnedAt                *time.Time        `json:"pinned_at"`
	AnnotatorID             uint              `json:"annotator_id"`
	Annotator               string            `json:"annotator"`
	ItemKey                 string            `json:"item_key"`
	Labels                  []AnnotationLabel `json:"labels"`
	SourceChecksum          string            `json:"source_checksum"`
	SubmittedSourceChecksum string            `json:"submitted_source_checksum"`
	AnnotationState         string            `json:"annotation_state"`
	SubmittedAt             *time.Time        `json:"submitted_at"`
	SupersedesID            *uint             `json:"supersedes_id"`
	QualityNote             string            `json:"quality_note"`
	CreatedAt               time.Time         `json:"created_at"`
	UpdatedAt               time.Time         `json:"updated_at"`
}

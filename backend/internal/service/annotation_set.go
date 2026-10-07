package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

type AnnotationSetService struct {
	db         *gorm.DB
	repository *repository.AnnotationSetRepository
	datasets   *repository.CorpusDatasetRepository
	schemas    *repository.AnnotationSchemaRepository
	system     *SystemService
}

func NewAnnotationSetService(db *gorm.DB, repository *repository.AnnotationSetRepository, datasets *repository.CorpusDatasetRepository, schemas *repository.AnnotationSchemaRepository, system *SystemService) *AnnotationSetService {
	return &AnnotationSetService{db: db, repository: repository, datasets: datasets, schemas: schemas, system: system}
}

func (service *AnnotationSetService) Create(request dto.CreateAnnotationSetRequest, actor dto.Actor, requestID string) (dto.AnnotationSetResponse, error) {
	dataset, err := service.datasets.Get(request.DatasetID)
	if err != nil {
		return dto.AnnotationSetResponse{}, MapRepositoryError("corpus dataset", err)
	}
	if dataset.DatasetState != constants.DatasetFrozen {
		return dto.AnnotationSetResponse{}, Conflict("dataset_not_frozen", "annotations require a frozen dataset version", repository.ErrStateConflict)
	}
	schema, err := service.schemas.Get(request.SchemaID)
	if err != nil {
		return dto.AnnotationSetResponse{}, MapRepositoryError("annotation schema", err)
	}
	if schema.DatasetID != dataset.ID || schema.SchemaState != constants.SchemaPublished {
		return dto.AnnotationSetResponse{}, Unprocessable("incompatible_schema", "schema must be published for the selected dataset", nil)
	}
	labels, err := normalizeAndValidateLabels(request.Labels, schema)
	if err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	labelsJSON, _ := json.Marshal(labels)
	annotation := model.AnnotationSet{
		DatasetID: dataset.ID, SchemaID: schema.ID, AnnotatorID: actor.ID,
		ItemKey: strings.TrimSpace(request.ItemKey), LabelsJSON: string(labelsJSON),
		SourceChecksum: strings.ToLower(request.SourceChecksum), AnnotationState: constants.AnnotationDraft,
		SupersedesID: request.SupersedesID, QualityNote: strings.TrimSpace(request.QualityNote),
		SchemaPinStatus: constants.SchemaPinLegacy,
	}
	err = service.db.Transaction(func(tx *gorm.DB) error {
		annotations := service.repository.WithDB(tx)
		if request.SupersedesID != nil {
			previous, loadErr := annotations.Get(*request.SupersedesID)
			if loadErr != nil {
				return MapRepositoryError("superseded annotation set", loadErr)
			}
			if previous.AnnotatorID != actor.ID || previous.AnnotationState != constants.AnnotationCompared {
				return Conflict("invalid_supersedes", "only your compared annotation can be superseded", repository.ErrStateConflict)
			}
			if previous.DatasetID != dataset.ID || previous.ItemKey != annotation.ItemKey {
				return Unprocessable("invalid_supersedes", "superseded annotation must belong to the same dataset item", nil)
			}
		}
		if createErr := annotations.Create(&annotation); createErr != nil {
			if repository.IsUniqueViolation(createErr) {
				return Conflict("duplicate_annotation_revision", "the same annotation payload already exists for this annotator", createErr)
			}
			return Internal("could not create annotation set", createErr)
		}
		if request.SupersedesID != nil {
			if transitionErr := annotations.Transition(*request.SupersedesID, constants.AnnotationCompared, constants.AnnotationSuperseded); transitionErr != nil {
				return Conflict("state_conflict", "superseded annotation changed concurrently", transitionErr)
			}
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "annotation_set.created", "annotation_set", auditID(annotation.ID),
			map[string]any{"dataset_id": annotation.DatasetID, "schema_id": annotation.SchemaID},
			nil, annotationSummary(annotation, len(labels)))
	})
	if err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	return service.Get(annotation.ID)
}

func (service *AnnotationSetService) Get(id uint) (dto.AnnotationSetResponse, error) {
	annotation, err := service.repository.Get(id)
	if err != nil {
		return dto.AnnotationSetResponse{}, MapRepositoryError("annotation set", err)
	}
	return annotationResponse(annotation), nil
}

func (service *AnnotationSetService) List(page, pageSize int, datasetID uint, itemKey, state string) ([]dto.AnnotationSetResponse, dto.PageMeta, error) {
	annotations, total, err := service.repository.List(page, pageSize, datasetID, itemKey, state)
	if err != nil {
		return nil, dto.PageMeta{}, Internal("could not list annotation sets", err)
	}
	responses := make([]dto.AnnotationSetResponse, 0, len(annotations))
	for _, annotation := range annotations {
		responses = append(responses, annotationResponse(annotation))
	}
	return responses, PageMeta(page, pageSize, total), nil
}

func (service *AnnotationSetService) Update(id uint, request dto.UpdateAnnotationSetRequest, actor dto.Actor, requestID string) (dto.AnnotationSetResponse, error) {
	before, err := service.repository.Get(id)
	if err != nil {
		return dto.AnnotationSetResponse{}, MapRepositoryError("annotation set", err)
	}
	if before.AnnotatorID != actor.ID {
		return dto.AnnotationSetResponse{}, Forbidden("annotators can edit only their own draft")
	}
	labels, err := normalizeAndValidateLabels(request.Labels, before.Schema)
	if err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	labelsJSON, _ := json.Marshal(labels)
	var after model.AnnotationSet
	err = service.db.Transaction(func(tx *gorm.DB) error {
		annotations := service.repository.WithDB(tx)
		if updateErr := annotations.UpdateDraft(id, actor.ID, before.UpdatedAt, string(labelsJSON), strings.TrimSpace(request.QualityNote)); updateErr != nil {
			if errors.Is(updateErr, repository.ErrStateConflict) {
				return Conflict("state_conflict", "only an unchanged owned draft can be edited", updateErr)
			}
			return Internal("could not update annotation draft", updateErr)
		}
		var reloadErr error
		after, reloadErr = annotations.Get(id)
		if reloadErr != nil {
			return Internal("could not reload annotation set", reloadErr)
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "annotation_set.updated", "annotation_set", auditID(id),
			map[string]any{"label_count": len(labels)}, annotationSummary(before, labelCount(before.LabelsJSON)), annotationSummary(after, len(labels)))
	})
	if err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	return annotationResponse(after), nil
}

func (service *AnnotationSetService) Transition(id uint, request dto.AnnotationTransitionRequest, actor dto.Actor, requestID string) (dto.AnnotationSetResponse, error) {
	before, err := service.repository.Get(id)
	if err != nil {
		return dto.AnnotationSetResponse{}, MapRepositoryError("annotation set", err)
	}
	if !constants.CanTransitionAnnotation(before.AnnotationState, request.TargetState) {
		return dto.AnnotationSetResponse{}, Conflict("invalid_annotation_transition",
			"annotation transition is not allowed from "+before.AnnotationState+" to "+request.TargetState, repository.ErrStateConflict)
	}
	if err := authorizeAnnotationTransition(before, request.TargetState, actor); err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	submitting := request.TargetState == constants.AnnotationSubmitted
	if submitting {
		if before.Schema.SchemaState != constants.SchemaPublished {
			return dto.AnnotationSetResponse{}, Unprocessable("schema_not_current",
				"a result can only be submitted against a currently published schema version; re-annotate on the current schema", nil)
		}
		if submittedChecksum := strings.ToLower(strings.TrimSpace(request.SourceChecksum)); submittedChecksum != "" && submittedChecksum != before.SourceChecksum {
			return dto.AnnotationSetResponse{}, Conflict("source_checksum_mismatch",
				"the submitted source checksum does not match the stored result-set checksum", repository.ErrVersionConflict)
		}
	}
	err = service.db.Transaction(func(tx *gorm.DB) error {
		annotations := service.repository.WithDB(tx)
		if transitionErr := annotations.Transition(id, before.AnnotationState, request.TargetState); transitionErr != nil {
			return Conflict("state_conflict", "annotation state changed concurrently", transitionErr)
		}
		after := before
		after.AnnotationState = request.TargetState
		if submitting {
			if _, pinErr := annotations.PinOnSubmit(id, before.Schema.SchemaCode, before.Schema.Version, before.SourceChecksum); pinErr != nil {
				return Conflict("state_conflict", "submitted result could not pin its schema version", pinErr)
			}
			after.SchemaPinStatus = constants.SchemaPinPinned
			after.PinnedSchemaCode = before.Schema.SchemaCode
			after.PinnedSchemaVersion = before.Schema.Version
			after.PinnedSourceChecksum = before.SourceChecksum
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "annotation_set."+request.TargetState, "annotation_set", auditID(id),
			map[string]any{
				"reason_present":      strings.TrimSpace(request.Reason) != "",
				"schema_pin_status":   after.SchemaPinStatus,
				"pinned_schema_code":  after.PinnedSchemaCode,
				"pinned_schema_value": after.PinnedSchemaVersion,
			},
			annotationSummary(before, labelCount(before.LabelsJSON)), annotationSummary(after, labelCount(before.LabelsJSON)))
	})
	if err != nil {
		return dto.AnnotationSetResponse{}, err
	}
	return service.Get(id)
}

func authorizeAnnotationTransition(annotation model.AnnotationSet, target string, actor dto.Actor) error {
	ownerAction := target == constants.AnnotationSubmitted || target == constants.AnnotationDraft
	if ownerAction && annotation.AnnotatorID != actor.ID {
		return Forbidden("annotators can submit or resume only their own annotation")
	}
	if target == constants.AnnotationLocked || target == constants.AnnotationReturned || target == constants.AnnotationCompared || target == constants.AnnotationSuperseded {
		if actor.Role != constants.RoleDataManager && actor.Role != constants.RoleAdjudicator && actor.Role != constants.RoleAdmin {
			return Forbidden("this annotation transition requires data manager or adjudicator authority")
		}
	}
	return nil
}

func normalizeAndValidateLabels(labels []dto.AnnotationLabel, schema model.AnnotationSchema) ([]dto.AnnotationLabel, error) {
	definitions := []dto.LabelDefinition{}
	if err := json.Unmarshal([]byte(schema.LabelDefinitionsJSON), &definitions); err != nil {
		return nil, Internal("schema label definitions could not be decoded", err)
	}
	allowed := map[string]string{}
	for _, definition := range definitions {
		allowed[strings.ToUpper(definition.Code)] = definition.TaskType
	}
	normalized := make([]dto.AnnotationLabel, 0, len(labels))
	seenClassification := map[string]bool{}
	for _, label := range labels {
		label.UnitKey = strings.TrimSpace(label.UnitKey)
		label.Label = strings.ToUpper(strings.TrimSpace(label.Label))
		taskType, exists := allowed[label.Label]
		if !exists {
			return nil, Unprocessable("unknown_label", "annotation uses a label not defined by the schema", nil)
		}
		if taskType == "span" && label.End <= label.Start {
			return nil, Unprocessable("invalid_span", "span labels require end greater than start", nil)
		}
		if taskType == "classification" {
			if label.Start != 0 || label.End != 0 {
				return nil, Unprocessable("invalid_classification", "classification labels cannot contain character offsets", nil)
			}
			if seenClassification[label.UnitKey] {
				return nil, Unprocessable("duplicate_unit_rating", "classification units may be rated once per annotation set", nil)
			}
			seenClassification[label.UnitKey] = true
		}
		normalized = append(normalized, label)
	}
	return normalized, nil
}

func annotationResponse(annotation model.AnnotationSet) dto.AnnotationSetResponse {
	labels := []dto.AnnotationLabel{}
	_ = json.Unmarshal([]byte(annotation.LabelsJSON), &labels)
	return dto.AnnotationSetResponse{
		ID: annotation.ID, DatasetID: annotation.DatasetID, DatasetCode: annotation.Dataset.DatasetCode,
		SchemaID: annotation.SchemaID, SchemaCode: annotation.Schema.SchemaCode, SchemaVersion: annotation.Schema.Version,
		AnnotatorID: annotation.AnnotatorID, Annotator: annotation.Annotator.Username, ItemKey: annotation.ItemKey,
		Labels: labels, SourceChecksum: annotation.SourceChecksum, AnnotationState: annotation.AnnotationState,
		SubmittedAt: annotation.SubmittedAt, SupersedesID: annotation.SupersedesID, QualityNote: annotation.QualityNote,
		SchemaPinStatus: annotation.SchemaPinStatus, PinnedSchemaCode: annotation.PinnedSchemaCode,
		PinnedSchemaVersion: annotation.PinnedSchemaVersion, PinnedSourceChecksum: annotation.PinnedSourceChecksum,
		PinnedAt: annotation.PinnedAt, BackfillNote: annotation.BackfillNote,
		CreatedAt: annotation.CreatedAt, UpdatedAt: annotation.UpdatedAt,
	}
}

func labelCount(encoded string) int {
	labels := []dto.AnnotationLabel{}
	_ = json.Unmarshal([]byte(encoded), &labels)
	return len(labels)
}

func annotationSummary(annotation model.AnnotationSet, labels int) map[string]any {
	return map[string]any{
		"dataset_id": annotation.DatasetID, "schema_id": annotation.SchemaID, "annotator_id": annotation.AnnotatorID,
		"item_key": annotation.ItemKey, "annotation_state": annotation.AnnotationState,
		"label_count": labels, "source_checksum": annotation.SourceChecksum,
		"schema_pin_status":  annotation.SchemaPinStatus,
		"pinned_schema_code": annotation.PinnedSchemaCode, "pinned_schema_version": annotation.PinnedSchemaVersion,
	}
}

// BackfillSchemaVersions upgrades historical result sets that predate schema
// version pinning. It belongs to the operations side: it reads schema center
// versions but never modifies them. A group is filled only when every result
// set for an item references one surviving schema version; mixed versions are
// isolated as unresolved.
func (service *AnnotationSetService) BackfillSchemaVersions(actor dto.Actor, requestID string) (dto.BackfillReportResponse, error) {
	groups, err := service.repository.LegacyGroups()
	if err != nil {
		return dto.BackfillReportResponse{}, Internal("could not load legacy annotation groups", err)
	}
	report := dto.BackfillReportResponse{RanAt: time.Now().UTC(), Details: []dto.BackfillGroupView{}}
	for _, group := range groups {
		report.ProcessedGroups++
		view := dto.BackfillGroupView{DatasetID: group.DatasetID, ItemKey: group.ItemKey, RowCount: group.RowCount}
		if dataset, datasetErr := service.datasets.Get(group.DatasetID); datasetErr == nil {
			view.DatasetCode = dataset.DatasetCode
		}
		markUnresolved := func(note string) error {
			if txErr := service.db.Transaction(func(tx *gorm.DB) error {
				return service.repository.WithDB(tx).BackfillGroup(group.DatasetID, group.ItemKey, map[string]any{
					"schema_pin_status": constants.SchemaPinUnresolved, "backfill_note": note,
				})
			}); txErr != nil {
				return Internal("could not isolate unresolved annotation group", txErr)
			}
			return nil
		}
		if group.SchemaCount != 1 || group.SchemaID == 0 {
			report.UnresolvedItems++
			view.Outcome, view.Note = "unresolved", "item result sets reference more than one schema version"
			report.Details = append(report.Details, view)
			if err := markUnresolved(view.Note); err != nil {
				return dto.BackfillReportResponse{}, err
			}
			continue
		}
		schema, schemaErr := service.schemas.Get(group.SchemaID)
		if schemaErr != nil {
			report.UnresolvedItems++
			view.Outcome, view.Note = "unresolved", "referenced schema version no longer exists"
			report.Details = append(report.Details, view)
			if err := markUnresolved(view.Note); err != nil {
				return dto.BackfillReportResponse{}, err
			}
			continue
		}
		updates := map[string]any{
			"pinned_schema_code": schema.SchemaCode, "pinned_schema_version": schema.Version,
			"pinned_source_checksum": gorm.Expr("source_checksum"), "schema_pin_status": constants.SchemaPinPinned,
			"backfill_note": "backfilled by dataset code and item key",
		}
		if txErr := service.db.Transaction(func(tx *gorm.DB) error {
			return service.repository.WithDB(tx).BackfillGroup(group.DatasetID, group.ItemKey, updates)
		}); txErr != nil {
			return dto.BackfillReportResponse{}, Internal("could not backfill annotation pin group", txErr)
		}
		report.PinnedGroups++
		report.PinnedRows += group.RowCount
		view.Outcome, view.Note = "pinned", schema.SchemaCode
		report.Details = append(report.Details, view)
	}
	if unresolved, listErr := service.repository.LegacyUnresolved(); listErr == nil {
		report.UnresolvedItems = len(unresolved)
	}
	if actor.ID != 0 {
		_ = service.system.RecordAudit(actor, requestID, "annotation_pin.backfilled", "annotation_set", "backfill",
			map[string]any{"processed_groups": report.ProcessedGroups, "pinned_groups": report.PinnedGroups,
				"pinned_rows": report.PinnedRows, "unresolved_items": report.UnresolvedItems}, nil, nil)
	}
	return report, nil
}

// UnresolvedPage exposes the backfill manual-review queue.
func (service *AnnotationSetService) UnresolvedPage(page, pageSize int) ([]dto.AnnotationSetResponse, dto.PageMeta, error) {
	items, total, err := service.repository.PinStatusPage(page, pageSize, constants.SchemaPinUnresolved)
	if err != nil {
		return nil, dto.PageMeta{}, Internal("could not list unresolved annotation sets", err)
	}
	responses := make([]dto.AnnotationSetResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, annotationResponse(item))
	}
	return responses, PageMeta(page, pageSize, total), nil
}

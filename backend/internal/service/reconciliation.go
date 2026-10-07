package service

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

// ReconciliationService reconciles the schema center with the annotation
// operations console. The reconciliation key is dataset code plus item key;
// cases whose two sides disagree are suspended rather than silently scored.
type ReconciliationService struct {
	db          *gorm.DB
	cases       *repository.AdjudicationCaseRepository
	annotations *repository.AnnotationSetRepository
	schemas     *repository.AnnotationSchemaRepository
	datasets    *repository.CorpusDatasetRepository
	system      *SystemService
}

func NewReconciliationService(db *gorm.DB, cases *repository.AdjudicationCaseRepository, annotations *repository.AnnotationSetRepository, schemas *repository.AnnotationSchemaRepository, datasets *repository.CorpusDatasetRepository, system *SystemService) *ReconciliationService {
	return &ReconciliationService{db: db, cases: cases, annotations: annotations, schemas: schemas, datasets: datasets, system: system}
}

const (
	ScopeAll     = "all"
	ScopeDataset = "dataset"
	ScopeItem    = "item"

	ReasonCaseSnapshotMissing   = "case_annotation_sets_missing"
	ReasonDatasetCodeMismatch   = "dataset_code_mismatch"
	ReasonItemKeyMismatch       = "item_key_mismatch"
	ReasonSchemaVersionUnpinned = "schema_version_unpinned"
	ReasonSchemaDeprecated      = "schema_deprecated"
)

// Run inspects open cases and suspends mismatches. Suspended cases whose
// underlying mismatch has been repaired are resumed in the same pass, except
// deprecated-schema holds which require re-annotation and a new computation.
func (service *ReconciliationService) Run(request dto.ReconcileRequest, actor dto.Actor, requestID string) (dto.ReconciliationRunResponse, error) {
	openCases, err := service.cases.OpenCases(request.DatasetID, strings.TrimSpace(request.ItemKey))
	if err != nil {
		return dto.ReconciliationRunResponse{}, Internal("could not load adjudication cases for reconciliation", err)
	}
	suspendedCases, err := service.suspendedCases(request.DatasetID, strings.TrimSpace(request.ItemKey))
	if err != nil {
		return dto.ReconciliationRunResponse{}, Internal("could not load suspended adjudication cases", err)
	}
	scope := ScopeAll
	if request.DatasetID > 0 {
		scope = ScopeDataset
	}
	if strings.TrimSpace(request.ItemKey) != "" {
		scope = ScopeItem
	}
	report := dto.ReconciliationRunResponse{
		Scope: scope, DatasetID: request.DatasetID, ItemKey: strings.TrimSpace(request.ItemKey),
		Findings: []dto.ReconciliationFinding{}, TriggeredBy: actor.ID,
	}
	now := time.Now().UTC()
	err = service.db.Transaction(func(tx *gorm.DB) error {
		cases := service.cases.WithDB(tx)
		for _, openCase := range openCases {
			report.CasesChecked++
			code, reason, matched := service.evaluate(openCase)
			if matched {
				continue
			}
			if suspendErr := cases.Suspend(openCase.ID, openCase.CaseState, code, reason, now); suspendErr != nil {
				return Conflict("state_conflict", "case changed while reconciliation suspended it", suspendErr)
			}
			report.CasesSuspended++
			report.OpenMismatches++
			report.Findings = append(report.Findings, finding(openCase.ID, openCase.DatasetID, openCase.Dataset.DatasetCode,
				openCase.ItemKey, "suspended", code, reason, openCase.CaseState))
		}
		for _, suspended := range suspendedCases {
			report.CasesChecked++
			// Deprecated-schema holds are released only through re-annotation
			// and a fresh computation, never by another reconciliation pass.
			if suspended.SuspendReasonCode == ReasonSchemaDeprecated {
				report.OpenMismatches++
				report.Findings = append(report.Findings, finding(suspended.ID, suspended.DatasetID, suspended.Dataset.DatasetCode,
					suspended.ItemKey, "holds_deprecated_schema", ReasonSchemaDeprecated,
					"deprecated schema results must be re-annotated against the current schema", suspended.StateBeforeSuspend))
				continue
			}
			code, reason, matched := service.evaluate(suspended)
			if !matched {
				report.OpenMismatches++
				report.Findings = append(report.Findings, finding(suspended.ID, suspended.DatasetID, suspended.Dataset.DatasetCode,
					suspended.ItemKey, "still_suspended", code, reason, suspended.StateBeforeSuspend))
				continue
			}
			previous := suspended.StateBeforeSuspend
			if !constants.CanResumeSuspendedCase(previous) {
				report.OpenMismatches++
				continue
			}
			if resumeErr := cases.Resume(suspended.ID, previous, now); resumeErr != nil {
				return Conflict("state_conflict", "case changed while reconciliation resumed it", resumeErr)
			}
			report.CasesResumed++
			report.Findings = append(report.Findings, finding(suspended.ID, suspended.DatasetID, suspended.Dataset.DatasetCode,
				suspended.ItemKey, "resumed", "", "", previous))
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "reconciliation.run", "adjudication_case", "run",
			map[string]any{
				"scope": report.Scope, "dataset_id": report.DatasetID, "item_key": report.ItemKey,
				"checked": report.CasesChecked, "suspended": report.CasesSuspended,
				"resumed": report.CasesResumed, "open_mismatches": report.OpenMismatches,
			}, nil, nil)
	})
	if err != nil {
		return dto.ReconciliationRunResponse{}, err
	}
	report.CompletedAt, report.CreatedAt = now, now
	return report, nil
}

func finding(caseID, datasetID uint, datasetCode, itemKey, action, code, reason, previous string) dto.ReconciliationFinding {
	return dto.ReconciliationFinding{
		CaseID: caseID, DatasetID: datasetID, DatasetCode: datasetCode, ItemKey: itemKey,
		Action: action, ReasonCode: code, Reason: reason, PreviousState: previous,
	}
}

// Resume manually releases one suspended case after the operations team has
// repaired the underlying data. Deprecated-schema cases stay blocked: those
// results are retained but must be re-annotated under the current schema.
func (service *ReconciliationService) Resume(caseID uint, request dto.ResumeCaseRequest, actor dto.Actor, requestID string) (dto.AdjudicationCaseResponse, error) {
	before, err := service.cases.Get(caseID)
	if err != nil {
		return dto.AdjudicationCaseResponse{}, MapRepositoryError("adjudication case", err)
	}
	if before.CaseState != constants.CaseSuspended {
		return dto.AdjudicationCaseResponse{}, Conflict("case_not_suspended", "only a suspended case can be resumed", repository.ErrStateConflict)
	}
	if before.SuspendReasonCode == ReasonSchemaDeprecated {
		return dto.AdjudicationCaseResponse{}, Unprocessable("schema_deprecated",
			"deprecated schema results must be re-annotated and recomputed; the held case is retained", nil)
	}
	previous := before.StateBeforeSuspend
	if !constants.CanResumeSuspendedCase(previous) {
		return dto.AdjudicationCaseResponse{}, Conflict("invalid_case_resume", "the remembered case state cannot be resumed", repository.ErrStateConflict)
	}
	if _, _, matched := service.evaluate(before); !matched {
		return dto.AdjudicationCaseResponse{}, Conflict("reconciliation_mismatch_open",
			"the case still fails dataset/item reconciliation", repository.ErrStateConflict)
	}
	err = service.db.Transaction(func(tx *gorm.DB) error {
		if resumeErr := service.cases.WithDB(tx).Resume(caseID, previous, time.Now().UTC()); resumeErr != nil {
			return Conflict("state_conflict", "case changed while it was resumed", resumeErr)
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "adjudication_case.resumed", "adjudication_case", auditID(caseID),
			map[string]any{"note_present": strings.TrimSpace(request.Note) != ""}, caseSummary(before), map[string]any{"case_state": previous})
	})
	if err != nil {
		return dto.AdjudicationCaseResponse{}, err
	}
	loaded, err := service.cases.Get(caseID)
	if err != nil {
		return dto.AdjudicationCaseResponse{}, MapRepositoryError("adjudication case", err)
	}
	return adjudicationResponse(loaded), nil
}

func (service *ReconciliationService) suspendedCases(datasetID uint, itemKey string) ([]model.AdjudicationCase, error) {
	var cases []model.AdjudicationCase
	query := service.db.Preload("Dataset").Where("case_state = ?", constants.CaseSuspended)
	if datasetID > 0 {
		query = query.Where("dataset_id = ?", datasetID)
	}
	if itemKey != "" {
		query = query.Where("item_key = ?", itemKey)
	}
	if err := query.Order("id ASC").Find(&cases).Error; err != nil {
		return nil, err
	}
	return cases, nil
}

// evaluate applies the dataset-code + item-key reconciliation rule and the
// schema-version pin checks.
func (service *ReconciliationService) evaluate(adjudication model.AdjudicationCase) (string, string, bool) {
	ids := []uint{}
	if err := json.Unmarshal([]byte(adjudication.AnnotationSetIDsJSON), &ids); err != nil || len(ids) == 0 {
		return ReasonCaseSnapshotMissing, "case does not reference any annotation result sets", false
	}
	annotations, err := service.annotations.ByIDs(ids)
	if err != nil || len(annotations) != len(uniqueIDs(ids)) {
		return ReasonCaseSnapshotMissing, "one or more referenced annotation result sets are missing", false
	}
	dataset, err := service.datasets.Get(adjudication.DatasetID)
	if err != nil {
		return ReasonDatasetCodeMismatch, "case dataset no longer exists", false
	}
	for _, annotation := range annotations {
		if annotation.DatasetID != adjudication.DatasetID || annotation.Dataset.DatasetCode != dataset.DatasetCode {
			return ReasonDatasetCodeMismatch, "annotation result set dataset code does not reconcile with the case", false
		}
		if annotation.ItemKey != adjudication.ItemKey {
			return ReasonItemKeyMismatch, "annotation result set item key does not reconcile with the case", false
		}
		if annotation.SchemaPinStatus != constants.SchemaPinPinned {
			return ReasonSchemaVersionUnpinned, "annotation result set has no pinned schema version", false
		}
		if annotation.PinnedSchemaCode == "" || annotation.PinnedSchemaVersion == 0 {
			return ReasonSchemaVersionUnpinned, "pinned schema version is incomplete", false
		}
		schema, schemaErr := service.schemas.Get(annotation.SchemaID)
		if schemaErr != nil || schema.SchemaCode != annotation.PinnedSchemaCode || schema.Version != annotation.PinnedSchemaVersion {
			return ReasonSchemaVersionUnpinned, "pinned schema version no longer matches the schema center", false
		}
		if schema.SchemaState == constants.SchemaDeprecated {
			return ReasonSchemaDeprecated, "the pinned schema version has been deprecated", false
		}
	}
	return "", "", true
}

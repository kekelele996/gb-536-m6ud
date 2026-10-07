package service

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

// ReconciliationService reconciles the schema center and the annotation
// operation console. The shared account key is (dataset code, item key);
// adjudication cases that cannot be matched to console result sets are
// suspended rather than deleted.
type ReconciliationService struct {
	db          *gorm.DB
	datasets    *repository.CorpusDatasetRepository
	annotations *repository.AnnotationSetRepository
	cases       *repository.AdjudicationCaseRepository
	schemas     *repository.AnnotationSchemaRepository
	system      *SystemService
}

func NewReconciliationService(db *gorm.DB, datasets *repository.CorpusDatasetRepository, annotations *repository.AnnotationSetRepository,
	cases *repository.AdjudicationCaseRepository, schemas *repository.AnnotationSchemaRepository, system *SystemService) *ReconciliationService {
	return &ReconciliationService{
		db: db, datasets: datasets, annotations: annotations, cases: cases, schemas: schemas, system: system,
	}
}

func (service *ReconciliationService) Reconcile(request dto.ReconcileRequest, actor dto.Actor, requestID string) (dto.ReconcileResponse, error) {
	dataset, err := service.datasets.Get(request.DatasetID)
	if err != nil {
		return dto.ReconcileResponse{}, MapRepositoryError("corpus dataset", err)
	}
	itemKey := request.ItemKey
	annotationSets, err := service.annotations.ByDatasetAndItemKey(dataset.ID, itemKey)
	if err != nil {
		return dto.ReconcileResponse{}, Internal("could not load console result sets for reconciliation", err)
	}
	cases, err := service.cases.ByDatasetAndItemKey(dataset.ID, itemKey)
	if err != nil {
		return dto.ReconcileResponse{}, Internal("could not load adjudication cases for reconciliation", err)
	}
	response := dto.ReconcileResponse{
		DatasetID: dataset.ID, DatasetCode: dataset.DatasetCode, ItemKey: itemKey,
		AnnotationSets: []dto.ReconciledSet{}, Cases: []dto.ReconciledCase{},
		Findings: []dto.ReconcileFinding{}, Suspended: []uint{},
		ReconciledAt: time.Now().UTC(),
	}
	matched := len(annotationSets) > 0
	for _, annotation := range annotationSets {
		code := annotation.SchemaCode
		version := annotation.SchemaVersion
		if code == "" {
			code = annotation.Schema.SchemaCode
			version = annotation.Schema.Version
		}
		response.AnnotationSets = append(response.AnnotationSets, dto.ReconciledSet{
			ID: annotation.ID, Annotator: annotation.Annotator.Username, SchemaCode: code,
			SchemaVersion: version, State: annotation.AnnotationState, BindingState: annotation.SchemaBindingState,
		})
	}
	for _, adjudication := range cases {
		caseEntry := dto.ReconciledCase{
			ID: adjudication.ID, CaseState: adjudication.CaseState, SchemaCode: adjudication.SchemaCode,
			SchemaVersion: adjudication.SchemaVersion, Score: adjudication.AgreementScore,
			SuspendedReason: adjudication.SuspendedReason,
		}
		// The console must contain the result sets referenced by the case.
		missing := service.missingAnnotationSets(adjudication, annotationSets)
		if len(missing) > 0 {
			matched = false
			finding := dto.ReconcileFinding{
				Code: "case_result_set_missing", Message: fmt.Sprintf("case references missing result set ids %v", missing),
				AdjudicationID: adjudication.ID, ExpectedDataset: dataset.DatasetCode, ExpectedItemKey: itemKey,
			}
			response.Findings = append(response.Findings, finding)
			if constants.CanSuspendCase(adjudication.CaseState) {
				if suspendErr := service.cases.Suspend(adjudication.ID, adjudication.CaseState, finding.Message); suspendErr != nil {
					return response, Conflict("state_conflict", "case changed during reconciliation", suspendErr)
				}
				caseEntry.CaseState = constants.CaseSuspended
				caseEntry.SuspendedReason = finding.Message
				response.Suspended = append(response.Suspended, adjudication.ID)
				if auditErr := service.system.RecordAudit(actor, requestID, "adjudication_case.suspended", "adjudication_case", auditID(adjudication.ID),
					map[string]any{"via": "reconciliation", "dataset_code": dataset.DatasetCode, "item_key": itemKey},
					nil, map[string]any{"case_state": constants.CaseSuspended, "reason": finding.Message}); auditErr != nil {
					return response, auditErr
				}
			}
		}
		// The score's pinned schema version must still exist on the center side.
		if adjudication.SchemaCode != "" {
			if _, schemaErr := service.schemas.VersionByCode(dataset.ID, adjudication.SchemaCode, adjudication.SchemaVersion); schemaErr != nil {
				matched = false
				response.Findings = append(response.Findings, dto.ReconcileFinding{
					Code: "pinned_schema_version_missing",
					Message: fmt.Sprintf("pinned schema %s v%d is not registered in the schema center",
						adjudication.SchemaCode, adjudication.SchemaVersion),
					AdjudicationID: adjudication.ID, ExpectedDataset: dataset.DatasetCode, ExpectedItemKey: itemKey,
				})
			}
		}
		response.Cases = append(response.Cases, caseEntry)
	}
	if len(cases) == 0 && len(annotationSets) > 0 {
		response.Findings = append(response.Findings, dto.ReconcileFinding{
			Code: "case_missing_for_item", Message: "console result sets have no adjudication case under this key",
			ExpectedDataset: dataset.DatasetCode, ExpectedItemKey: itemKey,
		})
	}
	response.Matched = matched && len(response.Findings) == 0
	return response, nil
}

func (service *ReconciliationService) missingAnnotationSets(adjudication model.AdjudicationCase, available []model.AnnotationSet) []uint {
	ids := []uint{}
	if decodeErr := json.Unmarshal([]byte(adjudication.AnnotationSetIDsJSON), &ids); decodeErr != nil {
		return []uint{}
	}
	present := map[uint]bool{}
	for _, annotation := range available {
		present[annotation.ID] = true
	}
	missing := make([]uint, 0)
	for _, id := range ids {
		if !present[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// ResumeCase re-opens a suspended case once the two sides reconcile.
func (service *ReconciliationService) ResumeCase(id uint, request dto.ResumeRequest, actor dto.Actor, requestID string) (dto.AdjudicationCaseResponse, error) {
	before, err := service.cases.Get(id)
	if err != nil {
		return dto.AdjudicationCaseResponse{}, MapRepositoryError("adjudication case", err)
	}
	if before.CaseState != constants.CaseSuspended {
		return dto.AdjudicationCaseResponse{}, Conflict("invalid_case_transition", "only suspended cases can be resumed", repository.ErrStateConflict)
	}
	err = service.db.Transaction(func(tx *gorm.DB) error {
		if resumeErr := service.cases.WithDB(tx).Resume(id); resumeErr != nil {
			return Conflict("state_conflict", "case changed during resume", resumeErr)
		}
		after, reloadErr := service.cases.WithDB(tx).Get(id)
		if reloadErr != nil {
			return Internal("could not reload adjudication case", reloadErr)
		}
		return service.system.RecordAuditTx(tx, actor, requestID, "adjudication_case.resumed", "adjudication_case", auditID(id),
			map[string]any{"note_present": request.Note != ""}, nil, caseSummary(after))
	})
	if err != nil {
		return dto.AdjudicationCaseResponse{}, err
	}
	return service.getCaseResponse(id)
}

func (service *ReconciliationService) getCaseResponse(id uint) (dto.AdjudicationCaseResponse, error) {
	adjudication, err := service.cases.Get(id)
	if err != nil {
		return dto.AdjudicationCaseResponse{}, MapRepositoryError("adjudication case", err)
	}
	return adjudicationResponse(adjudication), nil
}

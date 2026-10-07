package service

import (
	"testing"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

func TestDeprecationKeepsResultSetsAndRejectsScoringUntilRework(t *testing.T) {
	db := testServiceDBFull(t)
	manager := dto.Actor{ID: 7, Username: "manager", Role: constants.RoleDataManager}
	annotator := dto.Actor{ID: 9, Username: "annotator", Role: constants.RoleAnnotator}
	dataset := model.CorpusDataset{
		DatasetCode: "DEP-DS", Name: "Deprecation corpus", Language: "en", Domain: "quality",
		ContentMaskPolicy: "Masked", DatasetState: constants.DatasetFrozen, Version: 1,
		OwnerTeam: "Quality", CreatedBy: manager.ID,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	schemaV1 := model.AnnotationSchema{
		DatasetID: dataset.ID, SchemaCode: "DEP-SCHEMA", Version: 1,
		LabelDefinitionsJSON: classificationDefinitions(), SpanPolicy: "Half-open offsets.",
		OverlapPolicy: "No nested spans.", ExamplesJSON: "[]",
		SchemaState: constants.SchemaPublished, CreatedBy: manager.ID,
	}
	if err := db.Create(&schemaV1).Error; err != nil {
		t.Fatalf("create schema v1: %v", err)
	}
	annotationSetService := NewAnnotationSetService(db,
		repository.NewAnnotationSetRepository(db),
		repository.NewCorpusDatasetRepository(db),
		repository.NewAnnotationSchemaRepository(db),
		NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0))

	checksum := repeatChecksum("a")
	created, err := annotationSetService.Create(dto.CreateAnnotationSetRequest{
		DatasetID: dataset.ID, SchemaID: schemaV1.ID, ItemKey: "DEP-ITEM-1",
		Labels:         []dto.AnnotationLabel{{UnitKey: "u1", Label: "RISK"}},
		SourceChecksum: checksum,
	}, annotator, "req-create")
	if err != nil {
		t.Fatalf("create result set: %v", err)
	}
	if _, err := annotationSetService.Transition(created.ID,
		dto.AnnotationTransitionRequest{TargetState: constants.AnnotationSubmitted}, annotator, "req-submit"); err != nil {
		t.Fatalf("submit result set: %v", err)
	}
	// New drafts against v1 are rejected once it is deprecated.
	schemaService := NewAnnotationSchemaService(db,
		repository.NewAnnotationSchemaRepository(db), repository.NewSchemaPublishRepository(db),
		repository.NewCorpusDatasetRepository(db),
		NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0))
	if _, _, err := schemaService.Deprecate(schemaV1.ID, manager, "req-deprecate"); err != nil {
		t.Fatalf("deprecate schema: %v", err)
	}
	_, err = annotationSetService.Create(dto.CreateAnnotationSetRequest{
		DatasetID: dataset.ID, SchemaID: schemaV1.ID, ItemKey: "DEP-ITEM-2",
		Labels:         []dto.AnnotationLabel{{UnitKey: "u1", Label: "RISK"}},
		SourceChecksum: repeatChecksum("b"),
	}, annotator, "req-create-denied")
	if appErr := requireAppStatus(t, err, 422); appErr == nil {
		t.Fatal("deprecated schema must not start new drafts")
	}
	// The submitted result set is retained and still readable.
	if _, err := annotationSetService.Get(created.ID); err != nil {
		t.Fatalf("pinned result set must remain readable after deprecation: %v", err)
	}
}

func TestReconciliationSuspendsCaseMissingResultSets(t *testing.T) {
	db := testServiceDBFull(t)
	manager := dto.Actor{ID: 7, Username: "manager", Role: constants.RoleDataManager}
	dataset := model.CorpusDataset{
		DatasetCode: "REC-DS", Name: "Reconcile corpus", Language: "en", Domain: "quality",
		ContentMaskPolicy: "Masked", DatasetState: constants.DatasetFrozen, Version: 1,
		OwnerTeam: "Quality", CreatedBy: manager.ID,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	adjudication := model.AdjudicationCase{
		DatasetID: dataset.ID, ItemKey: "REC-ITEM-1", AnnotationSetIDsJSON: "[51,52]",
		AgreementMetric: "cohen_kappa", Applicability: "nominal", DisagreementType: "label",
		ConfusionSnapshotJSON: "[]", EvidenceSnapshotJSON: "[]", ClusterKey: "schema:label:a~b",
		CaseState: constants.CaseOpen, FinalLabelsJSON: "[]", InputHash: "h", AlgorithmVersion: "v1",
		SchemaCode: "REC-SCHEMA", SchemaVersion: 1, IdempotencyKey: "rec-key", CreatedBy: manager.ID,
	}
	if err := db.Create(&adjudication).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}
	reconciliation := NewReconciliationService(db,
		repository.NewCorpusDatasetRepository(db),
		repository.NewAnnotationSetRepository(db),
		repository.NewAdjudicationCaseRepository(db),
		repository.NewAnnotationSchemaRepository(db),
		NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0))
	report, err := reconciliation.Reconcile(dto.ReconcileRequest{DatasetID: dataset.ID, ItemKey: "REC-ITEM-1"}, manager, "req-reconcile")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Matched {
		t.Fatal("a case missing console result sets must not be matched")
	}
	if len(report.Suspended) != 1 || report.Suspended[0] != adjudication.ID {
		t.Fatalf("expected the orphan case to be suspended, got %v", report.Suspended)
	}
	var reloaded model.AdjudicationCase
	if err := db.First(&reloaded, adjudication.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if reloaded.CaseState != constants.CaseSuspended {
		t.Fatalf("case state = %s, want suspended", reloaded.CaseState)
	}
}

func TestLegacyBackfillResolvesByItemKeyAndListsAmbiguous(t *testing.T) {
	db := testServiceDBFull(t)
	managerID := uint(7)
	dataset := model.CorpusDataset{
		DatasetCode: "UPG-DS", Name: "Upgrade corpus", Language: "en", Domain: "quality",
		ContentMaskPolicy: "Masked", DatasetState: constants.DatasetFrozen, Version: 1,
		OwnerTeam: "Quality", CreatedBy: managerID,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	schema := model.AnnotationSchema{
		DatasetID: dataset.ID, SchemaCode: "UPG-SCHEMA", Version: 1,
		LabelDefinitionsJSON: classificationDefinitions(), SpanPolicy: "Half-open offsets.",
		OverlapPolicy: "No nested spans.", ExamplesJSON: "[]",
		SchemaState: constants.SchemaPublished, CreatedBy: managerID,
	}
	if err := db.Create(&schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	// A legacy row with no version snapshot, plus a modern sibling on same key.
	legacy := model.AnnotationSet{
		DatasetID: dataset.ID, SchemaID: schema.ID, AnnotatorID: 11, ItemKey: "UPG-ITEM-1",
		LabelsJSON: `[{"unit_key":"u1","label":"RISK"}]`, SourceChecksum: repeatChecksum("l"),
		AnnotationState: constants.AnnotationSubmitted, SchemaBindingState: constants.BindingCurrent,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy row: %v", err)
	}
	if err := db.Exec("UPDATE annotation_sets SET schema_version = 0, schema_code = '' WHERE id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("simulate legacy row: %v", err)
	}
	upgrade := NewUpgradeService(db,
		repository.NewAnnotationSetRepository(db),
		repository.NewAnnotationSchemaRepository(db),
		repository.NewLegacyBindingRepository(db))
	report, err := upgrade.BackfillLegacySchemaBindings()
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if report.Scanned != 1 || report.Resolved != 1 || report.Unresolved != 0 {
		t.Fatalf("unexpected backfill report: %+v", report)
	}
	var updated model.AnnotationSet
	if err := db.First(&updated, legacy.ID).Error; err != nil {
		t.Fatalf("reload legacy row: %v", err)
	}
	if updated.SchemaCode != "UPG-SCHEMA" || updated.SchemaVersion != 1 {
		t.Fatalf("legacy row not backfilled: code=%s version=%d", updated.SchemaCode, updated.SchemaVersion)
	}
}

func classificationDefinitions() string {
	return `[{"code":"RISK","display_name":"Risk","task_type":"classification","description":"Material risk."},{"code":"CLEAR","display_name":"Clear","task_type":"classification","description":"No risk."}]`
}

func repeatChecksum(char string) string {
	checksum := ""
	for len(checksum) < 64 {
		checksum += char
	}
	return checksum[:64]
}

func requireAppStatus(t *testing.T, err error, status int) *AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with status %d", status)
	}
	var appErr *AppError
	if !asAppError(err, &appErr) {
		t.Fatalf("expected AppError, got %T %v", err, err)
	}
	if appErr.Status != status {
		t.Fatalf("error status = %d, want %d (%v)", appErr.Status, status, err)
	}
	return appErr
}

func asAppError(err error, target **AppError) bool {
	for err != nil {
		if appErr, ok := err.(*AppError); ok {
			*target = appErr
			return true
		}
		type unwrapper interface{ Unwrap() error }
		unwrap, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = unwrap.Unwrap()
	}
	return false
}

package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

func legacySubmittedAnnotation(db *gorm.DB, fixture pinFixture, annotatorID uint, schemaID uint, itemKey, checksum string) {
	submittedAt := time.Now().UTC().Add(-2 * time.Hour)
	annotation := model.AnnotationSet{
		DatasetID: fixture.dataset.ID, SchemaID: schemaID, AnnotatorID: annotatorID,
		ItemKey: itemKey, LabelsJSON: `[{"unit_key":"u1","label":"RISK"}]`,
		SourceChecksum: checksum, AnnotationState: constants.AnnotationSubmitted, SubmittedAt: &submittedAt,
		QualityNote: "legacy", SchemaPinStatus: constants.SchemaPinLegacy,
	}
	if err := db.Create(&annotation).Error; err != nil {
		panic(fmt.Sprintf("create legacy annotation: %v", err))
	}
}

func TestBackfillPinsUnambiguousItemsAndIsolatesMixedVersions(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaPublished)
	secondAnnotator := model.User{Username: "ann-backfill-b", PasswordHash: "x", Role: constants.RoleAnnotator, Active: true}
	if err := db.Create(&secondAnnotator).Error; err != nil {
		t.Fatalf("create annotator: %v", err)
	}
	// Unambiguous item: both result sets reference the same schema.
	legacySubmittedAnnotation(db, fixture, fixture.annotator.ID, fixture.schemaV1.ID, "ITEM-CLEAN", "cs-1")
	legacySubmittedAnnotation(db, fixture, secondAnnotator.ID, fixture.schemaV1.ID, "ITEM-CLEAN", "cs-2")
	// Mixed item: a v2 schema for the same schema code.
	v2 := fixture.schemaV1
	v2.ID = 0
	v2.Version = 2
	if err := db.Create(&v2).Error; err != nil {
		t.Fatalf("create v2 schema: %v", err)
	}
	legacySubmittedAnnotation(db, fixture, fixture.annotator.ID, fixture.schemaV1.ID, "ITEM-MIXED", "cm-1")
	legacySubmittedAnnotation(db, fixture, secondAnnotator.ID, v2.ID, "ITEM-MIXED", "cm-2")

	backfill := NewAnnotationSetService(db, repository.NewAnnotationSetRepository(db),
		repository.NewCorpusDatasetRepository(db), repository.NewAnnotationSchemaRepository(db),
		NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0))
	report, err := backfill.BackfillSchemaVersions(dto.Actor{}, "startup-test")
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if report.PinnedGroups != 1 || report.PinnedRows != 2 {
		t.Fatalf("expected one pinned group with two rows, got %+v", report)
	}
	if report.UnresolvedItems != 1 {
		t.Fatalf("expected one unresolved item, got %d", report.UnresolvedItems)
	}
	var clean []model.AnnotationSet
	db.Where("item_key = ?", "ITEM-CLEAN").Find(&clean)
	for _, row := range clean {
		if row.SchemaPinStatus != constants.SchemaPinPinned || row.PinnedSchemaVersion != 1 ||
			row.PinnedSourceChecksum != row.SourceChecksum {
			t.Fatalf("clean row was not pinned correctly: %+v", row)
		}
	}
	var mixed []model.AnnotationSet
	db.Where("item_key = ?", "ITEM-MIXED").Find(&mixed)
	for _, row := range mixed {
		if row.SchemaPinStatus != constants.SchemaPinUnresolved {
			t.Fatalf("mixed row must be isolated unresolved: %+v", row)
		}
	}
}

func openCase(t *testing.T, db *gorm.DB, fixture pinFixture, ids []uint, key string) model.AdjudicationCase {
	t.Helper()
	encoded := "["
	for index, id := range ids {
		if index > 0 {
			encoded += ","
		}
		encoded += fmt.Sprintf("%d", id)
	}
	encoded += "]"
	caseRecord := model.AdjudicationCase{
		DatasetID: fixture.dataset.ID, ItemKey: key, AnnotationSetIDsJSON: encoded,
		AgreementMetric: "cohen_kappa", AgreementScore: 0.5, ObservedAgreement: 0.6, ChanceAgreement: 0.2,
		SampleSize: 1, CoderCount: 2, Applicability: "nominal", DisagreementType: constants.DisagreementLabel,
		ConfusionSnapshotJSON: "[]", EvidenceSnapshotJSON: "[]", ClusterKey: "PIN-SCHEMA:label:RISK~CLEAR",
		CaseState: constants.CaseOpen, FinalLabelsJSON: "[]", InputHash: "hash-" + key,
		AlgorithmVersion: "v1", IdempotencyKey: "idem-" + key, CreatedBy: fixture.manager.ID,
	}
	if err := db.Create(&caseRecord).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}
	return caseRecord
}

func TestReconciliationSuspendsMismatchedCaseAndResumesAfterRepair(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaPublished)
	secondAnnotator := model.User{Username: "ann-rec-b", PasswordHash: "x", Role: constants.RoleAnnotator, Active: true}
	if err := db.Create(&secondAnnotator).Error; err != nil {
		t.Fatalf("create annotator: %v", err)
	}
	// Legacy unpinned rows feeding an open case: reconciliation must hold it.
	first := createDraftAnnotation(t, db, fixture, "rec-1", "ITEM-REC")
	second := createDraftAnnotation(t, db, fixture, "rec-2", "ITEM-REC")
	if err := db.Model(&model.AnnotationSet{}).Where("id = ?", second.ID).Update("annotator_id", secondAnnotator.ID).Error; err != nil {
		t.Fatalf("reassign second annotation: %v", err)
	}
	caseRecord := openCase(t, db, fixture, []uint{first.ID, second.ID}, "ITEM-REC")

	system := NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0)
	reconciliation := NewReconciliationService(db,
		repository.NewAdjudicationCaseRepository(db), repository.NewAnnotationSetRepository(db),
		repository.NewAnnotationSchemaRepository(db), repository.NewCorpusDatasetRepository(db), system)
	manager := dto.Actor{ID: fixture.manager.ID, Username: "mgr-pin", Role: constants.RoleDataManager}
	run, err := reconciliation.Run(dto.ReconcileRequest{}, manager, "req-rec")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if run.CasesSuspended != 1 {
		t.Fatalf("expected one suspended case, got %+v", run)
	}
	var suspended model.AdjudicationCase
	if err := db.First(&suspended, caseRecord.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if suspended.CaseState != constants.CaseSuspended || suspended.StateBeforeSuspend != constants.CaseOpen ||
		suspended.SuspendReasonCode != ReasonSchemaVersionUnpinned {
		t.Fatalf("case hold details incorrect: %+v", suspended)
	}
	// Repairing by pinning both rows allows the next run to resume the case.
	pinnedAt := time.Now().UTC()
	db.Model(&model.AnnotationSet{}).Where("id IN ?", []uint{first.ID, second.ID}).Updates(map[string]any{
		"annotation_state":       constants.AnnotationLocked,
		"schema_pin_status":      constants.SchemaPinPinned,
		"pinned_schema_code":     fixture.schemaV1.SchemaCode,
		"pinned_schema_version":  fixture.schemaV1.Version,
		"pinned_source_checksum": gorm.Expr("source_checksum"),
		"pinned_at":              pinnedAt,
	})
	run, err = reconciliation.Run(dto.ReconcileRequest{}, manager, "req-rec-2")
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if run.CasesResumed != 1 {
		t.Fatalf("expected one resumed case, got %+v", run)
	}
	var resumed model.AdjudicationCase
	if err := db.First(&resumed, caseRecord.ID).Error; err != nil {
		t.Fatalf("reload resumed case: %v", err)
	}
	if resumed.CaseState != constants.CaseOpen || resumed.SuspendReasonCode != "" {
		t.Fatalf("case did not return to open: %+v", resumed)
	}
}

func TestDeprecatedSchemaCaseStaysSuspendedUntilReannotation(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaDeprecated)
	secondAnnotator := model.User{Username: "ann-dep-b", PasswordHash: "x", Role: constants.RoleAnnotator, Active: true}
	if err := db.Create(&secondAnnotator).Error; err != nil {
		t.Fatalf("create annotator: %v", err)
	}
	first := pinnedAnnotation(t, db, fixture, fixture.annotator.ID, "ITEM-DEPRECATED", "d-1")
	second := pinnedAnnotation(t, db, fixture, secondAnnotator.ID, "ITEM-DEPRECATED", "d-2")
	caseRecord := openCase(t, db, fixture, []uint{first.ID, second.ID}, "ITEM-DEPRECATED")

	system := NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0)
	reconciliation := NewReconciliationService(db,
		repository.NewAdjudicationCaseRepository(db), repository.NewAnnotationSetRepository(db),
		repository.NewAnnotationSchemaRepository(db), repository.NewCorpusDatasetRepository(db), system)
	manager := dto.Actor{ID: fixture.manager.ID, Username: "mgr-pin", Role: constants.RoleDataManager}
	run, err := reconciliation.Run(dto.ReconcileRequest{}, manager, "req-dep-hold")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if run.CasesSuspended != 1 {
		t.Fatalf("deprecated case must suspend, got %+v", run)
	}
	// A second run must not auto-release the hold.
	run, err = reconciliation.Run(dto.ReconcileRequest{}, manager, "req-dep-hold-2")
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if run.CasesResumed != 0 {
		t.Fatalf("deprecated hold must not auto-resume, got %+v", run)
	}
	_, err = reconciliation.Resume(caseRecord.ID, dto.ResumeCaseRequest{Note: "attempt manual release anyway now"}, manager, "req-manual")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != "schema_deprecated" {
		t.Fatalf("manual resume of deprecated hold must be rejected, got %v", err)
	}
}

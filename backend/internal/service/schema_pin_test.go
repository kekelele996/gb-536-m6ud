package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

func pinTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.CorpusDataset{}, &model.AnnotationSchema{},
		&model.AnnotationSet{}, &model.AdjudicationCase{}, &model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

type pinFixture struct {
	db        *gorm.DB
	dataset   model.CorpusDataset
	schemaV1  model.AnnotationSchema
	annotator model.User
	manager   model.User
}

func newPinFixture(t *testing.T, db *gorm.DB, schemaState string) pinFixture {
	t.Helper()
	annotator := model.User{Username: "ann-pin", PasswordHash: "x", Role: constants.RoleAnnotator, Active: true}
	manager := model.User{Username: "mgr-pin", PasswordHash: "x", Role: constants.RoleDataManager, Active: true}
	if err := db.Create(&annotator).Error; err != nil {
		t.Fatalf("create annotator: %v", err)
	}
	if err := db.Create(&manager).Error; err != nil {
		t.Fatalf("create manager: %v", err)
	}
	dataset := model.CorpusDataset{
		DatasetCode: "PIN-DS", Name: "Pin corpus", Language: "en", Domain: "quality",
		ContentMaskPolicy: "Masked", DatasetState: constants.DatasetFrozen, Version: 1,
		OwnerTeam: "Quality", CreatedBy: manager.ID,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	publishedAt := time.Now().UTC().Add(-time.Hour)
	schema := model.AnnotationSchema{
		DatasetID: dataset.ID, SchemaCode: "PIN-SCHEMA", Version: 1,
		LabelDefinitionsJSON: `[{"code":"RISK","task_type":"classification"},{"code":"CLEAR","task_type":"classification"}]`,
		SpanPolicy:           "half-open offsets documented here", OverlapPolicy: "nested spans are forbidden by policy",
		ExamplesJSON: `[{"item_key":"EX","masked_text":"[MASK] ok","labels":[]}]`,
		SchemaState:  schemaState,
		CreatedBy:    manager.ID,
		PublishedAt:  &publishedAt,
	}
	if err := db.Create(&schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return pinFixture{db: db, dataset: dataset, schemaV1: schema, annotator: annotator, manager: manager}
}

func classificationLabels(value string) []dto.AnnotationLabel {
	return []dto.AnnotationLabel{{UnitKey: "u1", Label: value}}
}

func pinServices(db *gorm.DB) (*SystemService, *AnnotationSetService, *AnnotationSchemaService) {
	system := NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0)
	annotations := repository.NewAnnotationSetRepository(db)
	schemas := repository.NewAnnotationSchemaRepository(db)
	datasets := repository.NewCorpusDatasetRepository(db)
	annotationService := NewAnnotationSetService(db, annotations, datasets, schemas, system)
	schemaService := NewAnnotationSchemaService(db, schemas, datasets, system)
	return system, annotationService, schemaService
}

func createDraftAnnotation(t *testing.T, db *gorm.DB, fixture pinFixture, checksum string, itemKey string) model.AnnotationSet {
	t.Helper()
	annotation := model.AnnotationSet{
		DatasetID: fixture.dataset.ID, SchemaID: fixture.schemaV1.ID, AnnotatorID: fixture.annotator.ID,
		ItemKey: itemKey, LabelsJSON: `[{"unit_key":"u1","label":"RISK"}]`,
		SourceChecksum: checksum, AnnotationState: constants.AnnotationDraft,
		QualityNote: "draft", SchemaPinStatus: constants.SchemaPinLegacy,
	}
	if err := db.Create(&annotation).Error; err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	return annotation
}

func TestSubmitPinsSchemaVersionAndSourceChecksum(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaPublished)
	system, annotationService, _ := pinServices(db)
	annotation := createDraftAnnotation(t, db, fixture, "aabb", "ITEM-PIN")

	_, err := annotationService.Transition(annotation.ID, dto.AnnotationTransitionRequest{
		TargetState: constants.AnnotationSubmitted, SourceChecksum: "aabb",
	}, dto.Actor{ID: fixture.annotator.ID, Role: constants.RoleAnnotator}, "req-pin")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	var stored model.AnnotationSet
	if err := db.First(&stored, annotation.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.SchemaPinStatus != constants.SchemaPinPinned || stored.PinnedSchemaCode != "PIN-SCHEMA" ||
		stored.PinnedSchemaVersion != 1 || stored.PinnedSourceChecksum != "aabb" || stored.PinnedAt == nil {
		t.Fatalf("pin snapshot not recorded: %+v", stored)
	}
	var auditCount int64
	db.Model(&model.AuditEvent{}).Where("action = ? AND request_id = ?", "annotation_set.submitted", "req-pin").Count(&auditCount)
	if auditCount != 1 {
		t.Fatalf("expected one submit audit event, got %d", auditCount)
	}
	_ = system
}

func TestSubmitRejectsMismatchedSourceChecksum(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaPublished)
	_, annotationService, _ := pinServices(db)
	annotation := createDraftAnnotation(t, db, fixture, "stored-checksum", "ITEM-CHECKSUM")

	_, err := annotationService.Transition(annotation.ID, dto.AnnotationTransitionRequest{
		TargetState: constants.AnnotationSubmitted, SourceChecksum: "different-checksum",
	}, dto.Actor{ID: fixture.annotator.ID, Role: constants.RoleAnnotator}, "req-checksum")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict || appErr.Code != "source_checksum_mismatch" {
		t.Fatalf("expected source_checksum_mismatch conflict, got %v", err)
	}
	var stored model.AnnotationSet
	if err := db.First(&stored, annotation.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.AnnotationState != constants.AnnotationDraft || stored.SchemaPinStatus == constants.SchemaPinPinned {
		t.Fatalf("failed submit must leave draft legacy row untouched: %+v", stored)
	}
}

func TestSubmitBlockedWhenSchemaDeprecated(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaDeprecated)
	_, annotationService, _ := pinServices(db)
	annotation := createDraftAnnotation(t, db, fixture, "checksum-dep", "ITEM-DEP")

	_, err := annotationService.Transition(annotation.ID, dto.AnnotationTransitionRequest{
		TargetState: constants.AnnotationSubmitted, SourceChecksum: "checksum-dep",
	}, dto.Actor{ID: fixture.annotator.ID, Role: constants.RoleAnnotator}, "req-dep")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity || appErr.Code != "schema_not_current" {
		t.Fatalf("expected schema_not_current 422, got %v", err)
	}
}

func TestDeprecatedSchemaScoringRequiresReannotation(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaDeprecated)
	system := NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0)
	caseService := NewAdjudicationCaseService(db, repository.NewAdjudicationCaseRepository(db), repository.NewAnnotationSetRepository(db), system, "v1")

	first := pinnedAnnotation(t, db, fixture, fixture.annotator.ID, "ITEM-DEP-SCORE", "checksum-1")
	other := model.User{Username: "ann-other", PasswordHash: "x", Role: constants.RoleAnnotator, Active: true}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create second annotator: %v", err)
	}
	second := pinnedAnnotation(t, db, fixture, other.ID, "ITEM-DEP-SCORE", "checksum-2")

	_, _, err := caseService.Compute(dto.ComputeAdjudicationRequest{
		DatasetID: fixture.dataset.ID, ItemKey: "ITEM-DEP-SCORE",
		AnnotationSetIDs: []uint{first.ID, second.ID}, Metric: "auto",
	}, "idem-dep", dto.Actor{ID: fixture.manager.ID, Role: constants.RoleDataManager}, "req-dep-score")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != "schema_deprecated" {
		t.Fatalf("expected schema_deprecated scoring rejection, got %v", err)
	}
	var caseCount int64
	db.Model(&model.AdjudicationCase{}).Count(&caseCount)
	if caseCount != 0 {
		t.Fatalf("deprecated scoring must not persist a case, count=%d", caseCount)
	}
}

func pinnedAnnotation(t *testing.T, db *gorm.DB, fixture pinFixture, annotatorID uint, itemKey, checksum string) model.AnnotationSet {
	t.Helper()
	pinnedAt := time.Now().UTC().Add(-time.Hour)
	submittedAt := pinnedAt
	annotation := model.AnnotationSet{
		DatasetID: fixture.dataset.ID, SchemaID: fixture.schemaV1.ID, AnnotatorID: annotatorID,
		ItemKey: itemKey, LabelsJSON: `[{"unit_key":"u1","label":"RISK"}]`,
		SourceChecksum: checksum, AnnotationState: constants.AnnotationLocked, SubmittedAt: &submittedAt,
		QualityNote: "pinned", SchemaPinStatus: constants.SchemaPinPinned,
		PinnedSchemaCode: fixture.schemaV1.SchemaCode, PinnedSchemaVersion: fixture.schemaV1.Version,
		PinnedSourceChecksum: checksum, PinnedAt: &pinnedAt,
	}
	if err := db.Create(&annotation).Error; err != nil {
		t.Fatalf("create pinned annotation: %v", err)
	}
	return annotation
}

func TestSchemaDeprecateDoesNotDeleteSubmittedResults(t *testing.T) {
	db := pinTestDB(t)
	fixture := newPinFixture(t, db, constants.SchemaPublished)
	system, _, schemaService := pinServices(db)
	annotation := createDraftAnnotation(t, db, fixture, "keep-me", "ITEM-KEEP")
	if err := db.Model(&model.AnnotationSet{}).Where("id = ?", annotation.ID).
		Updates(map[string]any{
			"annotation_state":       constants.AnnotationSubmitted,
			"schema_pin_status":      constants.SchemaPinPinned,
			"pinned_schema_code":     fixture.schemaV1.SchemaCode,
			"pinned_schema_version":  1,
			"pinned_source_checksum": "keep-me",
		}).Error; err != nil {
		t.Fatalf("force submitted pinned state: %v", err)
	}

	managerActor := dto.Actor{ID: fixture.manager.ID, Username: "mgr-pin", Role: constants.RoleDataManager}
	if _, err := schemaService.Transition(fixture.schemaV1.ID, constants.SchemaDeprecated, managerActor, "req-deprecate"); err != nil {
		t.Fatalf("deprecate schema: %v", err)
	}
	var stored model.AnnotationSet
	if err := db.First(&stored, annotation.ID).Error; err != nil {
		t.Fatalf("submitted result must still exist after deprecation: %v", err)
	}
	if stored.PinnedSchemaCode != "PIN-SCHEMA" {
		t.Fatalf("pinned snapshot was altered by deprecation: %+v", stored)
	}
	var schema model.AnnotationSchema
	if err := db.First(&schema, fixture.schemaV1.ID).Error; err != nil || schema.SchemaState != constants.SchemaDeprecated {
		t.Fatalf("schema not deprecated: %v %+v", err, schema)
	}
	_ = system
}

func TestFailedSchemaPublicationLeavesSubmittedResultsUntouched(t *testing.T) {
	db := pinTestDB(t)
	// The fixture schema starts validated so publication is the next state.
	fixture := newPinFixture(t, db, constants.SchemaValidated)
	_, annotationService, schemaService := pinServices(db)

	// Publishing requires a published schema for submission, so first publish
	// successfully and submit an operations-console result against it.
	managerActor := dto.Actor{ID: fixture.manager.ID, Username: "mgr-pin", Role: constants.RoleDataManager}
	if _, err := schemaService.Transition(fixture.schemaV1.ID, constants.SchemaPublished, managerActor, "req-publish-ok"); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	annotation := createDraftAnnotation(t, db, fixture, "ops-checksum", "ITEM-OPS")
	if _, err := annotationService.Transition(annotation.ID, dto.AnnotationTransitionRequest{
		TargetState: constants.AnnotationSubmitted, SourceChecksum: "ops-checksum",
	}, dto.Actor{ID: fixture.annotator.ID, Role: constants.RoleAnnotator}, "req-ops-submit"); err != nil {
		t.Fatalf("operations submit: %v", err)
	}

	// Simulate a schema-center failure on the next publication attempt of a
	// copied version; the operations side must not roll back.
	copySchema := model.AnnotationSchema{
		DatasetID: fixture.dataset.ID, SchemaCode: "PIN-SCHEMA", Version: 2,
		LabelDefinitionsJSON: fixture.schemaV1.LabelDefinitionsJSON,
		SpanPolicy:           fixture.schemaV1.SpanPolicy, OverlapPolicy: fixture.schemaV1.OverlapPolicy,
		ExamplesJSON: fixture.schemaV1.ExamplesJSON, SchemaState: constants.SchemaValidated,
		CreatedBy: fixture.manager.ID,
	}
	if err := db.Create(&copySchema).Error; err != nil {
		t.Fatalf("create validated copy: %v", err)
	}
	failAuditWrites(t, db)
	if _, err := schemaService.Transition(copySchema.ID, constants.SchemaPublished, managerActor, "req-publish-fail"); err == nil {
		t.Fatal("expected the schema-center publication to fail")
	}
	var reloadedSchema model.AnnotationSchema
	if err := db.First(&reloadedSchema, copySchema.ID).Error; err != nil {
		t.Fatalf("reload schema: %v", err)
	}
	if reloadedSchema.SchemaState != constants.SchemaValidated {
		t.Fatalf("failed publication must not change schema state, got %s", reloadedSchema.SchemaState)
	}
	var reloadedAnnotation model.AnnotationSet
	if err := db.First(&reloadedAnnotation, annotation.ID).Error; err != nil {
		t.Fatalf("reload annotation: %v", err)
	}
	if reloadedAnnotation.AnnotationState != constants.AnnotationSubmitted || reloadedAnnotation.SchemaPinStatus != constants.SchemaPinPinned ||
		reloadedAnnotation.PinnedSchemaVersion != 1 || reloadedAnnotation.PinnedSourceChecksum != "ops-checksum" {
		t.Fatalf("operations-console result was altered by a schema-center failure: %+v", reloadedAnnotation)
	}
}

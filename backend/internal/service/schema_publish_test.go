package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"corpus-annotation-agreement-control/backend/internal/constants"
	"corpus-annotation-agreement-control/backend/internal/dto"
	"corpus-annotation-agreement-control/backend/internal/model"
	"corpus-annotation-agreement-control/backend/internal/repository"
)

type publishFixture struct {
	db      *gorm.DB
	manager dto.Actor
	schema  *model.AnnotationSchema
	service *AnnotationSchemaService
}

func setupPublishFixture(t *testing.T) publishFixture {
	t.Helper()
	db := testServiceDBFull(t)
	manager := dto.Actor{ID: 7, Username: "manager", Role: constants.RoleDataManager}
	dataset := model.CorpusDataset{
		DatasetCode: "PUB-DS", Name: "Publication corpus", Language: "en", Domain: "quality",
		ContentMaskPolicy: "Masked", DatasetState: constants.DatasetFrozen, Version: 1,
		OwnerTeam: "Quality", CreatedBy: manager.ID,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	definitionsJSON, _ := json.Marshal([]dto.LabelDefinition{
		{Code: "RISK", DisplayName: "Risk", TaskType: "classification", Description: "Material risk."},
		{Code: "CLEAR", DisplayName: "Clear", TaskType: "classification", Description: "No risk."},
	})
	schema := model.AnnotationSchema{
		DatasetID: dataset.ID, SchemaCode: "PUB-SCHEMA", Version: 1,
		LabelDefinitionsJSON: string(definitionsJSON), SpanPolicy: "Half-open offsets.",
		OverlapPolicy: "No nested spans.", ExamplesJSON: "[]",
		SchemaState: constants.SchemaValidated, CreatedBy: manager.ID,
	}
	if err := db.Create(&schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	system := NewSystemService(repository.NewSystemRepository(db), "test-secret-with-at-least-24-bytes", 0)
	service := NewAnnotationSchemaService(db,
		repository.NewAnnotationSchemaRepository(db),
		repository.NewSchemaPublishRepository(db),
		repository.NewCorpusDatasetRepository(db), system)
	return publishFixture{db: db, manager: manager, schema: &schema, service: service}
}

func TestSchemaPublishFailureOnlyRetriesSchemaSide(t *testing.T) {
	fixture := setupPublishFixture(t)

	_, err := fixture.service.Publish(fixture.schema.ID,
		dto.PublishSchemaRequest{FailureInjection: "downstream-registry-unavailable"}, fixture.manager, "req-fail")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("expected conflict on injected publish failure, got %v", err)
	}
	var reloaded model.AnnotationSchema
	if err := fixture.db.First(&reloaded, fixture.schema.ID).Error; err != nil {
		t.Fatalf("reload schema: %v", err)
	}
	if reloaded.SchemaState != constants.SchemaValidated {
		t.Fatalf("failed publish must leave schema validated, got %s", reloaded.SchemaState)
	}
	attempts, err := fixture.service.PublishAttempts(fixture.schema.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Status != constants.PublishFailed {
		t.Fatalf("expected one failed ledger attempt, attempts=%v err=%v", attempts, err)
	}

	// Retry is schema-center-only: no console input, and it succeeds.
	published, err := fixture.service.Publish(fixture.schema.ID, dto.PublishSchemaRequest{}, fixture.manager, "req-retry")
	if err != nil {
		t.Fatalf("retry publish: %v", err)
	}
	if published.SchemaState != constants.SchemaPublished {
		t.Fatalf("expected published after retry, got %s", published.SchemaState)
	}
	attempts, _ = fixture.service.PublishAttempts(fixture.schema.ID)
	if len(attempts) != 2 {
		t.Fatalf("expected two ledger attempts after retry, got %d", len(attempts))
	}
	statuses := map[string]bool{}
	for _, attempt := range attempts {
		statuses[attempt.Status] = true
	}
	if !statuses[constants.PublishFailed] || !statuses[constants.PublishSucceeded] {
		t.Fatalf("ledger must retain failed then succeeded attempts, got %v", attempts)
	}
}

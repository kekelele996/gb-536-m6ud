package service

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"corpus-annotation-agreement-control/backend/internal/model"
)

// testServiceDBFull migrates every table for cross-domain service tests.
func testServiceDBFull(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-full?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.CorpusDataset{}, &model.AnnotationSchema{},
		&model.AnnotationSet{}, &model.AdjudicationCase{}, &model.AuditEvent{},
		&model.SchemaPublishAttempt{}, &model.LegacySchemaBinding{},
	); err != nil {
		t.Fatalf("migrate full test database: %v", err)
	}
	return db
}

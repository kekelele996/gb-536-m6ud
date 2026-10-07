package model

import "time"

// LegacySchemaBinding lists annotation sets created before result sets recorded
// their effective schema version. Backfill resolves the effective version by
// item key; rows that cannot be resolved are kept here instead of being guessed.
type LegacySchemaBinding struct {
	ID              uint      `gorm:"primaryKey"`
	AnnotationSetID uint      `gorm:"uniqueIndex;not null"`
	DatasetID       uint      `gorm:"index:idx_legacy_binding,unique;not null"`
	DatasetCode     string    `gorm:"size:64;index;not null"`
	ItemKey         string    `gorm:"size:120;index:idx_legacy_binding,unique;not null"`
	AnnotatorID     uint      `gorm:"index;not null"`
	Reason          string    `gorm:"size:300;not null;default:''"`
	Resolved        bool      `gorm:"not null;default:false;index"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

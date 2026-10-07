package model

import "time"

type AnnotationSet struct {
	ID                      uint              `gorm:"primaryKey"`
	DatasetID               uint              `gorm:"uniqueIndex:idx_annotation_revision;not null"`
	SchemaID                uint              `gorm:"uniqueIndex:idx_annotation_revision;not null"`
	AnnotatorID             uint              `gorm:"uniqueIndex:idx_annotation_revision;not null"`
	ItemKey                 string            `gorm:"size:120;uniqueIndex:idx_annotation_revision;index:idx_annotation_item;not null"`
	LabelsJSON              string            `gorm:"type:text;not null"`
	SourceChecksum          string            `gorm:"size:64;uniqueIndex:idx_annotation_revision;not null"`
	AnnotationState         string            `gorm:"size:24;index;not null"`
	SubmittedAt             *time.Time        `gorm:"index"`
	SupersedesID            *uint             `gorm:"index"`
	QualityNote             string            `gorm:"size:600;not null"`
	SchemaCode              string            `gorm:"size:64;index;not null;default:''"`
	SchemaVersion           int               `gorm:"not null;default:0"`
	PinnedSchemaID          *uint             `gorm:"index"`
	PinnedAt                *time.Time        `gorm:"index"`
	SubmittedSourceChecksum string            `gorm:"size:64;not null;default:''"`
	SchemaBindingState      string            `gorm:"size:24;index;not null;default:'current'"`
	CreatedAt               time.Time         `gorm:"not null"`
	UpdatedAt               time.Time         `gorm:"not null"`
	Dataset                 CorpusDataset     `gorm:"foreignKey:DatasetID"`
	Schema                  AnnotationSchema  `gorm:"foreignKey:SchemaID"`
	PinnedSchema            *AnnotationSchema `gorm:"foreignKey:PinnedSchemaID"`
	Annotator               User              `gorm:"foreignKey:AnnotatorID"`
}

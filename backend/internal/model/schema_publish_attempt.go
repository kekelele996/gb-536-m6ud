package model

import "time"

// SchemaPublishAttempt is the schema-center-side publication ledger. It records
// release attempts exclusively for the schema domain: a failed publication is
// retried from this ledger without touching annotation console data.
type SchemaPublishAttempt struct {
	ID            uint   `gorm:"primaryKey"`
	SchemaID      uint   `gorm:"uniqueIndex:idx_publish_attempt,unique;not null"`
	AttemptNo     int    `gorm:"uniqueIndex:idx_publish_attempt,unique;not null;default:1"`
	Status        string `gorm:"size:24;index;not null"`
	FailureReason string `gorm:"size:500;not null;default:''"`
	AttemptedBy   uint   `gorm:"index;not null"`
	PublishedAt   *time.Time
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

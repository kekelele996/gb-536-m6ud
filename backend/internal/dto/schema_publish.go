package dto

import "time"

type PublishSchemaRequest struct {
	// FailureInjection is a non-empty failure code used only to drive the
	// schema-center-side failure and retry path without touching the console.
	FailureInjection string `json:"failure_injection" validate:"max=120"`
}

type SchemaPublishAttemptResponse struct {
	ID            uint       `json:"id"`
	SchemaID      uint       `json:"schema_id"`
	AttemptNo     int        `json:"attempt_no"`
	Status        string     `json:"status"`
	FailureReason string     `json:"failure_reason"`
	AttemptedBy   uint       `json:"attempted_by"`
	PublishedAt   *time.Time `json:"published_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

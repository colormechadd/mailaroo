package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Ingestion struct {
	ID          uuid.UUID `db:"id" json:"id"`
	MessageID   *string   `db:"message_id" json:"message_id"`
	FromAddress *string   `db:"from_address" json:"from_address"`
	ToAddress   *string   `db:"to_address" json:"to_address"`
	Status      string    `db:"status" json:"status"`
}

type IngestionStep struct {
	ID          uuid.UUID       `db:"id" json:"id"`
	IngestionID uuid.UUID       `db:"ingestion_id" json:"ingestion_id"`
	StepName    string          `db:"step_name" json:"step_name"`
	Status      string          `db:"status" json:"status"`
	Details     json.RawMessage `db:"details" json:"details"`
	DurationMS  int             `db:"duration_ms" json:"duration_ms"`
}

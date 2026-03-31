package models

import (
	"time"

	"github.com/google/uuid"
)

type IPBlock struct {
	ID             uuid.UUID  `db:"id"`
	IPAddress      string     `db:"ip_address"`
	Reason         *string    `db:"reason"`
	BlockedUntil   *time.Time `db:"blocked_until"`
	IsPermanent    bool       `db:"is_permanent"`
	CreateDatetime time.Time  `db:"create_datetime"`
}

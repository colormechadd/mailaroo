package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type User struct {
	ID              uuid.UUID      `db:"id" json:"id"`
	Username        string         `db:"username" json:"username"`
	PasswordHash    string         `db:"password_hash" json:"-"`
	IsActive        bool           `db:"is_active" json:"is_active"`
	IsAdmin         bool           `db:"is_admin" json:"is_admin"`
	TOTPEnabled     bool           `db:"totp_enabled" json:"totp_enabled"`
	TOTPSecret      *string        `db:"totp_secret" json:"-"`
	TOTPBackupCodes pq.StringArray `db:"totp_backup_codes" json:"-"`
}

type WebmailSession struct {
	ID              uuid.UUID `db:"id" json:"id"`
	UserID          uuid.UUID `db:"user_id" json:"user_id"`
	Token           string    `db:"token" json:"token"`
	RemoteIP        *string   `db:"remote_ip" json:"remote_ip"`
	UserAgent       *string   `db:"user_agent" json:"user_agent"`
	ExpiresDatetime time.Time `db:"expires_datetime" json:"expires_datetime"`
}

type TOTPPending struct {
	ID              uuid.UUID `db:"id"`
	UserID          uuid.UUID `db:"user_id"`
	Token           string    `db:"token"`
	ExpiresDatetime time.Time `db:"expires_datetime"`
}

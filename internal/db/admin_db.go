package db

import (
	"context"

	"github.com/colormechadd/maileroo/pkg/models"
	"github.com/google/uuid"
)

type AdminDB interface {
	CreateUser(ctx context.Context, username, passwordHash string) (*models.User, error)
	SetUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	SetUserActive(ctx context.Context, userID uuid.UUID, active bool) error
	CreateMailbox(ctx context.Context, userID uuid.UUID, name string) (*models.Mailbox, error)
	CreateAddressMapping(ctx context.Context, mailboxID uuid.UUID, pattern string, priority int) (*models.AddressMapping, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
}

func (db *DB) CreateUser(ctx context.Context, username, passwordHash string) (*models.User, error) {
	user := &models.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: passwordHash,
		IsActive:     true,
	}
	_, err := db.NamedExecContext(ctx, `
		INSERT INTO "user" (id, username, password_hash, is_active)
		VALUES (:id, :username, :password_hash, :is_active)
	`, user)
	return user, err
}

func (db *DB) SetUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := db.ExecContext(ctx, `UPDATE "user" SET password_hash = $1, update_datetime = CURRENT_TIMESTAMP WHERE id = $2`, passwordHash, userID)
	return err
}

func (db *DB) SetUserActive(ctx context.Context, userID uuid.UUID, active bool) error {
	_, err := db.ExecContext(ctx, `UPDATE "user" SET is_active = $1, update_datetime = CURRENT_TIMESTAMP WHERE id = $2`, active, userID)
	return err
}

func (db *DB) CreateMailbox(ctx context.Context, userID uuid.UUID, name string) (*models.Mailbox, error) {
	mb := &models.Mailbox{
		ID:     uuid.New(),
		UserID: userID,
		Name:   name,
	}
	_, err := db.NamedExecContext(ctx, `
		INSERT INTO mailbox (id, user_id, name)
		VALUES (:id, :user_id, :name)
	`, mb)
	return mb, err
}

func (db *DB) CreateAddressMapping(ctx context.Context, mailboxID uuid.UUID, pattern string, priority int) (*models.AddressMapping, error) {
	am := &models.AddressMapping{
		ID:             uuid.New(),
		AddressPattern: pattern,
		MailboxID:      mailboxID,
		Priority:       priority,
		IsActive:       true,
	}
	_, err := db.NamedExecContext(ctx, `
		INSERT INTO address_mapping (id, address_pattern, mailbox_id, priority, is_active)
		VALUES (:id, :address_pattern, :mailbox_id, :priority, :is_active)
	`, am)
	return am, err
}

package db

import (
	"context"

	"github.com/colormechadd/maileroo/pkg/models"
	"github.com/google/uuid"
)

type AdminDB interface {
	CreateUser(ctx context.Context, user *models.User) error
	CreateMailbox(ctx context.Context, mb *models.Mailbox) error
	CreateAddressMapping(ctx context.Context, am *models.AddressMapping) error
	ListUsers(ctx context.Context) ([]models.User, error)
	ListMailboxes(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	DeleteMailbox(ctx context.Context, mailboxID uuid.UUID) error
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	
	AddSendingAddress(ctx context.Context, sa *models.SendingAddress) error
	ListSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error)
	DeactivateSendingAddress(ctx context.Context, saID uuid.UUID) error
}

func (db *DB) CreateUser(ctx context.Context, user *models.User) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO "user" (id, username, password_hash, is_active) VALUES ($1, $2, $3, $4)`, user.ID, user.Username, user.PasswordHash, user.IsActive)
	if err != nil {
		return err
	}

	// Create default mailboxes
	defaults := []string{"Inbox"}
	for _, name := range defaults {
		_, err = tx.ExecContext(ctx, "INSERT INTO mailbox (id, user_id, name) VALUES ($1, $2, $3)", uuid.New(), user.ID, name)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) CreateMailbox(ctx context.Context, mb *models.Mailbox) error {
	_, err := db.ExecContext(ctx, "INSERT INTO mailbox (id, user_id, name) VALUES ($1, $2, $3)", mb.ID, mb.UserID, mb.Name)
	return err
}

func (db *DB) CreateAddressMapping(ctx context.Context, am *models.AddressMapping) error {
	_, err := db.ExecContext(ctx, "INSERT INTO address_mapping (id, address_pattern, mailbox_id, priority) VALUES ($1, $2, $3, $4)", am.ID, am.AddressPattern, am.MailboxID, am.Priority)
	return err
}

func (db *DB) ListUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User
	err := db.SelectContext(ctx, &users, `SELECT id, username, password_hash, is_active FROM "user" ORDER BY username ASC`)
	return users, err
}

func (db *DB) ListMailboxes(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error) {
	var mailboxes []models.Mailbox
	err := db.SelectContext(ctx, &mailboxes, "SELECT id, user_id, name FROM mailbox WHERE user_id = $1 ORDER BY name ASC", userID)
	return mailboxes, err
}

func (db *DB) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `DELETE FROM "user" WHERE id = $1`, userID)
	return err
}

func (db *DB) DeleteMailbox(ctx context.Context, mailboxID uuid.UUID) error {
	_, err := db.ExecContext(ctx, "DELETE FROM mailbox WHERE id = $1", mailboxID)
	return err
}

func (db *DB) AddSendingAddress(ctx context.Context, sa *models.SendingAddress) error {
	_, err := db.ExecContext(ctx, "INSERT INTO sending_address (id, user_id, mailbox_id, address, is_active) VALUES ($1, $2, $3, $4, $5)", sa.ID, sa.UserID, sa.MailboxID, sa.Address, sa.IsActive)
	return err
}

func (db *DB) ListSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error) {
	var addresses []models.SendingAddress
	err := db.SelectContext(ctx, &addresses, "SELECT id, user_id, mailbox_id, address, is_active FROM sending_address WHERE user_id = $1 ORDER BY address ASC", userID)
	return addresses, err
}

func (db *DB) DeactivateSendingAddress(ctx context.Context, saID uuid.UUID) error {
	_, err := db.ExecContext(ctx, "UPDATE sending_address SET is_active = FALSE WHERE id = $1", saID)
	return err
}

func (db *DB) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := db.GetContext(ctx, &user, `SELECT id, username, password_hash, is_active FROM "user" WHERE username = $1`, username)
	return &user, err
}

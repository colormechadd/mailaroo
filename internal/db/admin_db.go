package db

import (
	"context"

	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/google/uuid"
)

type AdminDB interface {
	CreateUser(ctx context.Context, user *models.User) error
	CreateMailbox(ctx context.Context, mb *models.Mailbox, userID uuid.UUID) error
	CreateAddressMapping(ctx context.Context, am *models.AddressMapping) error
	ListUsers(ctx context.Context) ([]models.User, error)
	ListMailboxes(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	DeleteMailbox(ctx context.Context, mailboxID uuid.UUID) error
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)

	AddUserToMailbox(ctx context.Context, mailboxID, userID uuid.UUID) error
	RemoveUserFromMailbox(ctx context.Context, mailboxID, userID uuid.UUID) error

	AddSendingAddress(ctx context.Context, sa *models.SendingAddress) error
	ListSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error)
	DeactivateSendingAddress(ctx context.Context, saID uuid.UUID) error
	ReactivateSendingAddress(ctx context.Context, saID uuid.UUID) error
	ListSendingAddressesByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.SendingAddressWithUser, error)
	CreateUserNoMailbox(ctx context.Context, user *models.User) error

	InsertDKIMKey(ctx context.Context, key *models.DKIMKey) error
	GetActiveDKIMKey(ctx context.Context, domain string, selector *string) (*models.DKIMKey, error)
	ListDKIMKeys(ctx context.Context) ([]models.DKIMKey, error)
	UpdateDKIMKeyData(ctx context.Context, id uuid.UUID, keyData []byte) error

	ToggleUserAdmin(ctx context.Context, userID uuid.UUID) error
	ToggleUserActive(ctx context.Context, userID uuid.UUID) error
	ListAllMailboxes(ctx context.Context) ([]models.Mailbox, error)
	ListAllSendingAddresses(ctx context.Context) ([]models.SendingAddressWithUser, error)
	GetAddressMappingsByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.AddressMapping, error)
	GetMailboxByID(ctx context.Context, mailboxID uuid.UUID) (*models.Mailbox, error)
	UpdateUserRecoveryEmail(ctx context.Context, userID uuid.UUID, email string) error
}

func (db *DB) CreateUser(ctx context.Context, user *models.User) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO "user" (id, username, password_hash, recovery_email, is_active, is_admin) VALUES ($1, $2, $3, $4, $5, $6)`, user.ID, user.Username, user.PasswordHash, user.RecoveryEmail, user.IsActive, user.IsAdmin)
	if err != nil {
		return err
	}

	// Create default mailboxes and associate with the user
	defaults := []string{"Inbox"}
	for _, name := range defaults {
		mbID := uuid.New()
		_, err = tx.ExecContext(ctx, "INSERT INTO mailbox (id, name) VALUES ($1, $2)", mbID, name)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO mailbox_user (mailbox_id, user_id) VALUES ($1, $2)", mbID, user.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) CreateMailbox(ctx context.Context, mb *models.Mailbox, userID uuid.UUID) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "INSERT INTO mailbox (id, name) VALUES ($1, $2)", mb.ID, mb.Name)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO mailbox_user (mailbox_id, user_id) VALUES ($1, $2)", mb.ID, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) CreateAddressMapping(ctx context.Context, am *models.AddressMapping) error {
	_, err := db.ExecContext(ctx, "INSERT INTO address_mapping (id, address_pattern, mailbox_id, priority) VALUES ($1, $2, $3, $4)", am.ID, am.AddressPattern, am.MailboxID, am.Priority)
	return err
}

func (db *DB) ListUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User
	err := db.SelectContext(ctx, &users, `SELECT id, username, password_hash, recovery_email, is_active, is_admin, custom_css FROM "user" ORDER BY username ASC`)
	return users, err
}

func (db *DB) ListMailboxes(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error) {
	var mailboxes []models.Mailbox
	err := db.SelectContext(ctx, &mailboxes, `
		SELECT m.id, m.name FROM mailbox m
		JOIN mailbox_user mu ON mu.mailbox_id = m.id
		WHERE mu.user_id = $1 AND mu.is_active = TRUE
		ORDER BY m.name ASC
	`, userID)
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

func (db *DB) AddUserToMailbox(ctx context.Context, mailboxID, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO mailbox_user (mailbox_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, mailboxID, userID)
	return err
}

func (db *DB) RemoveUserFromMailbox(ctx context.Context, mailboxID, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `DELETE FROM mailbox_user WHERE mailbox_id = $1 AND user_id = $2`, mailboxID, userID)
	return err
}

func (db *DB) AddSendingAddress(ctx context.Context, sa *models.SendingAddress) error {
	_, err := db.ExecContext(ctx, "INSERT INTO sending_address (id, user_id, mailbox_id, address, display_name, is_active) VALUES ($1, $2, $3, $4, $5, $6)", sa.ID, sa.UserID, sa.MailboxID, sa.Address, sa.DisplayName, sa.IsActive)
	return err
}

func (db *DB) ListSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error) {
	var addresses []models.SendingAddress
	err := db.SelectContext(ctx, &addresses, "SELECT id, user_id, mailbox_id, address, display_name, is_active FROM sending_address WHERE user_id = $1 ORDER BY address ASC", userID)
	return addresses, err
}

func (db *DB) DeactivateSendingAddress(ctx context.Context, saID uuid.UUID) error {
	_, err := db.ExecContext(ctx, "UPDATE sending_address SET is_active = FALSE WHERE id = $1", saID)
	return err
}

func (db *DB) ReactivateSendingAddress(ctx context.Context, saID uuid.UUID) error {
	_, err := db.ExecContext(ctx, "UPDATE sending_address SET is_active = TRUE WHERE id = $1", saID)
	return err
}

func (db *DB) CreateUserNoMailbox(ctx context.Context, user *models.User) error {
	_, err := db.ExecContext(ctx, `INSERT INTO "user" (id, username, password_hash, recovery_email, is_active, is_admin) VALUES ($1, $2, $3, $4, $5, $6)`, user.ID, user.Username, user.PasswordHash, user.RecoveryEmail, user.IsActive, user.IsAdmin)
	return err
}

func (db *DB) ListSendingAddressesByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.SendingAddressWithUser, error) {
	var addresses []models.SendingAddressWithUser
	err := db.SelectContext(ctx, &addresses, `
		SELECT sa.id, sa.user_id, sa.mailbox_id, sa.address, sa.display_name, sa.is_active, u.username, m.name AS mailbox_name
		FROM sending_address sa
		JOIN "user" u ON sa.user_id = u.id
		JOIN mailbox m ON sa.mailbox_id = m.id
		WHERE sa.mailbox_id = $1
		ORDER BY sa.address ASC
	`, mailboxID)
	return addresses, err
}

func (db *DB) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := db.GetContext(ctx, &user, `SELECT id, username, password_hash, recovery_email, is_active, is_admin, totp_enabled, totp_secret, totp_backup_codes, custom_css FROM "user" WHERE username = $1`, username)
	return &user, err
}

func (db *DB) InsertDKIMKey(ctx context.Context, key *models.DKIMKey) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO dkim_key (id, domain, selector, key_data, is_active) VALUES ($1, $2, $3, $4, $5)",
		key.ID, key.Domain, key.Selector, key.KeyData, key.IsActive,
	)
	return err
}

func (db *DB) GetActiveDKIMKey(ctx context.Context, domain string, selector *string) (*models.DKIMKey, error) {
	var key models.DKIMKey
	var err error
	if selector != nil {
		err = db.GetContext(ctx, &key,
			"SELECT id, domain, selector, key_data, is_active FROM dkim_key WHERE domain = $1 AND selector = $2 AND is_active = TRUE",
			domain, *selector,
		)
	} else {
		err = db.GetContext(ctx, &key,
			"SELECT id, domain, selector, key_data, is_active FROM dkim_key WHERE domain = $1 AND is_active = TRUE ORDER BY create_datetime DESC LIMIT 1",
			domain,
		)
	}
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (db *DB) ListDKIMKeys(ctx context.Context) ([]models.DKIMKey, error) {
	var keys []models.DKIMKey
	err := db.SelectContext(ctx, &keys,
		"SELECT id, domain, selector, key_data, is_active FROM dkim_key ORDER BY domain ASC",
	)
	return keys, err
}

func (db *DB) UpdateDKIMKeyData(ctx context.Context, id uuid.UUID, keyData []byte) error {
	_, err := db.ExecContext(ctx,
		"UPDATE dkim_key SET key_data = $1, update_datetime = NOW() WHERE id = $2",
		keyData, id,
	)
	return err
}

func (db *DB) ToggleUserAdmin(ctx context.Context, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `UPDATE "user" SET is_admin = NOT is_admin, update_datetime = NOW() WHERE id = $1`, userID)
	return err
}

func (db *DB) ToggleUserActive(ctx context.Context, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `UPDATE "user" SET is_active = NOT is_active, update_datetime = NOW() WHERE id = $1`, userID)
	return err
}

func (db *DB) ListAllMailboxes(ctx context.Context) ([]models.Mailbox, error) {
	var mailboxes []models.Mailbox
	err := db.SelectContext(ctx, &mailboxes, "SELECT id, name FROM mailbox ORDER BY name ASC")
	return mailboxes, err
}

func (db *DB) ListAllSendingAddresses(ctx context.Context) ([]models.SendingAddressWithUser, error) {
	var addresses []models.SendingAddressWithUser
	err := db.SelectContext(ctx, &addresses, `
		SELECT sa.id, sa.user_id, sa.mailbox_id, sa.address, sa.display_name, sa.is_active, u.username, m.name AS mailbox_name
		FROM sending_address sa
		JOIN "user" u ON sa.user_id = u.id
		JOIN mailbox m ON sa.mailbox_id = m.id
		ORDER BY sa.address ASC
	`)
	return addresses, err
}

func (db *DB) GetAddressMappingsByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.AddressMapping, error) {
	var mappings []models.AddressMapping
	err := db.SelectContext(ctx, &mappings,
		"SELECT id, address_pattern, mailbox_id, priority, is_active FROM address_mapping WHERE mailbox_id = $1 ORDER BY priority ASC",
		mailboxID,
	)
	return mappings, err
}

func (db *DB) GetMailboxByID(ctx context.Context, mailboxID uuid.UUID) (*models.Mailbox, error) {
	var mb models.Mailbox
	err := db.GetContext(ctx, &mb, "SELECT id, name FROM mailbox WHERE id = $1", mailboxID)
	if err != nil {
		return nil, err
	}
	return &mb, nil
}

func (db *DB) UpdateUserRecoveryEmail(ctx context.Context, userID uuid.UUID, email string) error {
	_, err := db.ExecContext(ctx, `UPDATE "user" SET recovery_email = $2, update_datetime = NOW() WHERE id = $1`, userID, email)
	return err
}


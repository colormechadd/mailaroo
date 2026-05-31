package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// EmailFilter specifies criteria for SearchEmails. Zero values mean no constraint for that field.
type EmailFilter struct {
	View        string // mailbox view: "mail", "unread", "read", "starred", "quarantined", "deleted", "sent", "all"
	From        string // ILIKE match on from_address
	To          string // ILIKE match on to_address
	Participant string // ILIKE match on from_address OR to_address
	Subject     string // ILIKE match on subject
	Text        string // full-text search via search_vector
	Category    string // exact match on category
}

type WebDB interface {
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	CreateWebmailSession(ctx context.Context, userID uuid.UUID, token string, remoteIP, userAgent string, expires time.Time) error
	GetWebmailSession(ctx context.Context, token string) (*models.WebmailSession, error)
	ExpireWebmailSession(ctx context.Context, token string) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetMailboxesByUserID(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error)
	SearchEmails(ctx context.Context, mailboxID, userID uuid.UUID, f EmailFilter, limit int, cursorTime *time.Time, cursorID *uuid.UUID) ([]models.Email, error)
	GetEmailByID(ctx context.Context, emailID uuid.UUID) (*models.Email, error)
	GetEmailByIDForUser(ctx context.Context, emailID, userID uuid.UUID) (*models.Email, error)
	GetAttachmentsByEmailID(ctx context.Context, emailID uuid.UUID) ([]models.EmailAttachment, error)
	GetAttachmentByIDForUser(ctx context.Context, attachmentID, userID uuid.UUID) (*models.EmailAttachment, error)
	GetIngestionStepsByEmailID(ctx context.Context, emailID, userID uuid.UUID) ([]models.IngestionStep, error)

	CountEmailsByMailboxID(ctx context.Context, mailboxID uuid.UUID, filter string) (int, error)
	CountEmailsByCategory(ctx context.Context, mailboxID, userID uuid.UUID, view string) (map[string]int, error)
	MarkEmailRead(ctx context.Context, emailID, userID uuid.UUID, read bool) error
	MarkEmailStarred(ctx context.Context, emailID, userID uuid.UUID, starred bool) error
	UpdateEmailStatus(ctx context.Context, emailID, userID uuid.UUID, status models.EmailStatus) error
	UpdateEmailCategory(ctx context.Context, emailID, userID uuid.UUID, category string) error
	BulkMarkEmailRead(ctx context.Context, ids []uuid.UUID, userID uuid.UUID, read bool) error
	BulkUpdateEmailStatus(ctx context.Context, ids []uuid.UUID, userID uuid.UUID, status models.EmailStatus) error

	GetActiveSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error)
	IsAuthorizedSendingAddress(ctx context.Context, userID uuid.UUID, address string) (bool, error)
	GetSendingAddressByID(ctx context.Context, id, userID uuid.UUID) (*models.SendingAddress, error)
	UpdateSendingAddressDisplayName(ctx context.Context, id, userID uuid.UUID, displayName string) error

	InsertOutboundJob(ctx context.Context, emailID *uuid.UUID, fromAddress string, recipients []string, rawMessage []byte) (*models.OutboundJob, error)
	GetOutboundJobsByEmailID(ctx context.Context, emailID uuid.UUID) ([]models.OutboundJob, error)
	GetOutboundJobAttemptsByJobID(ctx context.Context, jobID uuid.UUID) ([]models.OutboundJobAttempt, error)

	CreateDraft(ctx context.Context, draft models.Draft) (*models.Draft, error)
	UpdateDraft(ctx context.Context, draft models.Draft) error
	GetDraftByIDForUser(ctx context.Context, draftID, userID uuid.UUID) (*models.Draft, error)
	DeleteDraft(ctx context.Context, draftID, userID uuid.UUID) error
	GetDraftsByMailboxID(ctx context.Context, mailboxID, userID uuid.UUID) ([]models.Draft, error)
	CountDraftsByMailboxID(ctx context.Context, mailboxID, userID uuid.UUID) (int, error)

	ListContacts(ctx context.Context, mailboxID uuid.UUID) ([]models.Contact, error)
	SearchContacts(ctx context.Context, mailboxID uuid.UUID, query string) ([]models.Contact, error)
	SearchContactsForUser(ctx context.Context, userID uuid.UUID, query string) ([]models.Contact, error)
	GetContactByID(ctx context.Context, contactID, mailboxID uuid.UUID) (*models.Contact, error)
	GetContactByEmail(ctx context.Context, mailboxID uuid.UUID, email string) (*models.Contact, error)
	CreateContact(ctx context.Context, c models.Contact) (*models.Contact, error)
	UpdateContact(ctx context.Context, c models.Contact) error
	DeleteContact(ctx context.Context, contactID, mailboxID uuid.UUID) error
	ToggleContactFavorite(ctx context.Context, contactID, mailboxID uuid.UUID) error
	UpsertContactFromEmail(ctx context.Context, mailboxID uuid.UUID, email, firstName, lastName string) error


	CreateBlockRule(ctx context.Context, mailboxID uuid.UUID, userID uuid.UUID, addressPattern string) error
	ListBlockRules(ctx context.Context, mailboxID uuid.UUID) ([]*models.MailboxBlockRule, error)
	DeleteBlockRule(ctx context.Context, ruleID, mailboxID uuid.UUID) error
	IsBlockedByMailboxRules(ctx context.Context, mailboxID uuid.UUID, fromAddress string) (*models.MailboxBlockRule, error)

	GetMailboxUsers(ctx context.Context, mailboxID uuid.UUID) ([]models.User, error)
	GetSendingAddressesByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.SendingAddress, error)

	ListFilterRules(ctx context.Context, mailboxID uuid.UUID) ([]*models.FilterRule, error)
	GetFilterRuleByID(ctx context.Context, ruleID, mailboxID uuid.UUID) (*models.FilterRule, error)
	CreateFilterRule(ctx context.Context, rule *models.FilterRule) error
	UpdateFilterRule(ctx context.Context, rule *models.FilterRule) error
	DeleteFilterRule(ctx context.Context, ruleID, mailboxID uuid.UUID) error
	ReorderFilterRules(ctx context.Context, mailboxID uuid.UUID, orderedIDs []uuid.UUID) error

	CreateTOTPPending(ctx context.Context, userID uuid.UUID, token string, expires time.Time) error
	GetTOTPPendingByToken(ctx context.Context, token string) (*models.TOTPPending, error)
	DeleteTOTPPending(ctx context.Context, token string) error
	SaveTOTPSetup(ctx context.Context, userID uuid.UUID, secret string, hashedCodes []string) error
	DisableTOTP(ctx context.Context, userID uuid.UUID) error
	RemoveTOTPBackupCode(ctx context.Context, userID uuid.UUID, index int) error
	RegenerateTOTPBackupCodes(ctx context.Context, userID uuid.UUID, hashedCodes []string) error
}

func (db *DB) CreateWebmailSession(ctx context.Context, userID uuid.UUID, token string, remoteIP, userAgent string, expires time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO webmail_session (user_id, token, remote_ip, user_agent, expires_datetime)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, token, remoteIP, userAgent, expires)
	return err
}

func (db *DB) GetWebmailSession(ctx context.Context, token string) (*models.WebmailSession, error) {
	var session models.WebmailSession
	err := db.GetContext(ctx, &session, `SELECT id, user_id, token, remote_ip, user_agent, expires_datetime FROM webmail_session WHERE token = $1`, token)
	return &session, err
}

func (db *DB) ExpireWebmailSession(ctx context.Context, token string) error {
	_, err := db.ExecContext(ctx, `UPDATE webmail_session SET expires_datetime = CURRENT_TIMESTAMP WHERE token = $1`, token)
	return err
}

func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := db.GetContext(ctx, &user, `SELECT id, username, password_hash, is_active, is_admin, totp_enabled, totp_secret, totp_backup_codes FROM "user" WHERE id = $1`, id)
	return &user, err
}

func (db *DB) GetMailboxesByUserID(ctx context.Context, userID uuid.UUID) ([]models.Mailbox, error) {
	var mailboxes []models.Mailbox
	err := db.SelectContext(ctx, &mailboxes, `
		SELECT m.id, m.name FROM mailbox m
		JOIN mailbox_user mu ON mu.mailbox_id = m.id
		WHERE mu.user_id = $1 AND mu.is_active = TRUE
		ORDER BY m.name ASC
	`, userID)
	return mailboxes, err
}

func viewConditions(view string) []string {
	switch view {
	case "unread":
		return []string{"e.is_read = FALSE", "e.status = 'INBOX'", "e.direction = 'INBOUND'"}
	case "read":
		return []string{"e.is_read = TRUE", "e.status = 'INBOX'", "e.direction = 'INBOUND'"}
	case "starred":
		return []string{"e.is_star = TRUE", "e.status != 'DELETED'"}
	case "quarantined":
		return []string{"e.status = 'QUARANTINED'"}
	case "deleted":
		return []string{"e.status = 'DELETED'"}
	case "sent":
		return []string{"e.direction = 'OUTBOUND'", "e.status != 'DELETED'"}
	case "all":
		return []string{"e.status = 'INBOX'"}
	case "mail":
		return []string{"e.status = 'INBOX'", "e.direction = 'INBOUND'"}
	default:
		return []string{"e.status != 'DELETED'"}
	}
}

func (db *DB) SearchEmails(ctx context.Context, mailboxID, userID uuid.UUID, f EmailFilter, limit int, cursorTime *time.Time, cursorID *uuid.UUID) ([]models.Email, error) {
	args := []interface{}{mailboxID, userID}
	conditions := []string{
		"e.mailbox_id = $1",
		"mu.user_id = $2",
		"mu.is_active = TRUE",
	}

	conditions = append(conditions, viewConditions(f.View)...)

	next := func() int { return len(args) + 1 }

	if f.From != "" {
		conditions = append(conditions, fmt.Sprintf("e.from_address ILIKE $%d", next()))
		args = append(args, "%"+f.From+"%")
	}
	if f.To != "" {
		conditions = append(conditions, fmt.Sprintf("e.to_address ILIKE $%d", next()))
		args = append(args, "%"+f.To+"%")
	}
	if f.Participant != "" {
		n := next()
		conditions = append(conditions, fmt.Sprintf("(e.from_address ILIKE $%d OR e.to_address ILIKE $%d)", n, n))
		args = append(args, "%"+f.Participant+"%")
	}
	if f.Subject != "" {
		conditions = append(conditions, fmt.Sprintf("e.subject ILIKE $%d", next()))
		args = append(args, "%"+f.Subject+"%")
	}
	if f.Category != "" {
		if f.Category == "none" {
			conditions = append(conditions, fmt.Sprintf("(e.category IS NULL OR e.category = '')"))
		} else {
			conditions = append(conditions, fmt.Sprintf("e.category = $%d", next()))
			args = append(args, f.Category)
		}
	}
	if f.Text != "" {
		conditions = append(conditions, fmt.Sprintf("e.search_vector @@ plainto_tsquery('english', $%d)", next()))
		args = append(args, f.Text)
	}

	where := strings.Join(conditions, "\n  AND ")

	limitN := next()
	args = append(args, limit)

	var cursorClause string
	if cursorTime != nil && cursorID != nil {
		n := next()
		cursorClause = fmt.Sprintf("AND (e.receive_datetime, e.id) < ($%d, $%d)", n, n+1)
		args = append(args, cursorTime, cursorID)
	}

	sql := fmt.Sprintf(`
		SELECT
			e.id, e.mailbox_id, e.thread_id, e.address_mapping_id, e.ingestion_id, e.message_id,
			e.in_reply_to, e."references", e.subject, e.from_address, e.to_address,
			e.reply_to_address, e.storage_key, e.size, e.receive_datetime, e.is_read, e.is_star,
			e.direction, e.status, e.sending_address_id, e.user_id, e.body_plain, e.category
		FROM email e
		JOIN mailbox_user mu ON e.mailbox_id = mu.mailbox_id
		WHERE %s
		%s
		ORDER BY e.receive_datetime DESC, e.id DESC
		LIMIT $%d
	`, where, cursorClause, limitN)

	var emails []models.Email
	err := db.SelectContext(ctx, &emails, sql, args...)
	return emails, err
}

func (db *DB) CountEmailsByCategory(ctx context.Context, mailboxID, userID uuid.UUID, view string) (map[string]int, error) {
	args := []interface{}{mailboxID, userID}
	conditions := []string{
		"e.mailbox_id = $1",
		"mu.user_id = $2",
		"mu.is_active = TRUE",
	}

	conditions = append(conditions, viewConditions(view)...)

	where := strings.Join(conditions, "\n  AND ")

	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(e.category, ''), 'none') AS cat, COUNT(*) AS cnt
		FROM email e
		JOIN mailbox_user mu ON e.mailbox_id = mu.mailbox_id
		WHERE %s
		GROUP BY cat
	`, where)

	type row struct {
		Cat string `db:"cat"`
		Cnt int    `db:"cnt"`
	}
	var rows []row
	err := db.SelectContext(ctx, &rows, query, args...)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.Cat] = r.Cnt
	}
	return counts, nil
}

func (db *DB) CountEmailsByMailboxID(ctx context.Context, mailboxID uuid.UUID, filter string) (int, error) {
	var count int
	whereClause := "mailbox_id = $1 AND status = 'INBOX' AND direction = 'INBOUND'"

	switch filter {
	case "unread":
		whereClause = "mailbox_id = $1 AND is_read = FALSE AND status = 'INBOX' AND direction = 'INBOUND'"
	case "read":
		whereClause = "mailbox_id = $1 AND is_read = TRUE AND status = 'INBOX' AND direction = 'INBOUND'"
	case "starred":
		whereClause = "mailbox_id = $1 AND is_star = TRUE AND status != 'DELETED'"
	case "quarantined":
		whereClause = "mailbox_id = $1 AND status = 'QUARANTINED'"
	case "deleted":
		whereClause = "mailbox_id = $1 AND status = 'DELETED'"
	case "sent":
		whereClause = "mailbox_id = $1 AND direction = 'OUTBOUND' AND status != 'DELETED'"
	case "all":
		whereClause = "mailbox_id = $1 AND status = 'INBOX'"
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM email WHERE %s", whereClause)
	err := db.GetContext(ctx, &count, query, mailboxID)
	return count, err
}

func (db *DB) GetEmailByID(ctx context.Context, emailID uuid.UUID) (*models.Email, error) {
	var email models.Email
	err := db.GetContext(ctx, &email, `
		SELECT
			id, mailbox_id, thread_id, address_mapping_id, ingestion_id, message_id,
			in_reply_to, "references", subject, from_address, to_address,
			reply_to_address, storage_key, size, receive_datetime, is_read, is_star, direction, status, sending_address_id, user_id, body_plain
		FROM email
		WHERE id = $1
	`, emailID)
	return &email, err
}

func (db *DB) GetEmailByIDForUser(ctx context.Context, emailID, userID uuid.UUID) (*models.Email, error) {
	var email models.Email
	err := db.GetContext(ctx, &email, `
		SELECT
			e.id, e.mailbox_id, e.thread_id, e.address_mapping_id, e.ingestion_id, e.message_id,
			e.in_reply_to, e."references", e.subject, e.from_address, e.to_address,
			e.reply_to_address, e.storage_key, e.size, e.receive_datetime, e.is_read, e.is_star, e.direction, e.status, e.sending_address_id, e.user_id, e.body_plain, e.category
		FROM email e
		JOIN mailbox_user mu ON e.mailbox_id = mu.mailbox_id
		WHERE e.id = $1 AND mu.user_id = $2 AND mu.is_active = TRUE
	`, emailID, userID)
	return &email, err
}

func (db *DB) GetAttachmentsByEmailID(ctx context.Context, emailID uuid.UUID) ([]models.EmailAttachment, error) {
	var attachments []models.EmailAttachment
	err := db.SelectContext(ctx, &attachments, "SELECT id, email_id, filename, content_type, size, storage_key FROM email_attachment WHERE email_id = $1", emailID)
	return attachments, err
}

func (db *DB) GetAttachmentByIDForUser(ctx context.Context, attachmentID, userID uuid.UUID) (*models.EmailAttachment, error) {
	var att models.EmailAttachment
	err := db.GetContext(ctx, &att, `
		SELECT
			a.id, a.email_id, a.filename, a.content_type, a.size, a.storage_key
		FROM email_attachment a
		JOIN email e ON a.email_id = e.id
		JOIN mailbox_user mu ON e.mailbox_id = mu.mailbox_id
		WHERE a.id = $1 AND mu.user_id = $2 AND mu.is_active = TRUE
	`, attachmentID, userID)
	return &att, err
}

func (db *DB) GetIngestionStepsByEmailID(ctx context.Context, emailID, userID uuid.UUID) ([]models.IngestionStep, error) {
	var steps []models.IngestionStep
	err := db.SelectContext(ctx, &steps, `
		SELECT
			s.id, s.ingestion_id, s.step_name, s.status, s.details, s.duration_ms
		FROM ingestion_step s
		JOIN email e ON s.ingestion_id = e.ingestion_id
		JOIN mailbox_user mu ON e.mailbox_id = mu.mailbox_id
		WHERE e.id = $1 AND mu.user_id = $2 AND mu.is_active = TRUE
		ORDER BY s.create_datetime ASC
	`, emailID, userID)
	return steps, err
}

func (db *DB) MarkEmailRead(ctx context.Context, emailID, userID uuid.UUID, read bool) error {
	var readTime *time.Time
	if read {
		now := time.Now()
		readTime = &now
	}
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET is_read = $1, read_datetime = $2
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = $3 AND mu.user_id = $4 AND mu.is_active = TRUE
	`, read, readTime, emailID, userID)
	return err
}

func (db *DB) MarkEmailStarred(ctx context.Context, emailID, userID uuid.UUID, starred bool) error {
	var starTime *time.Time
	if starred {
		now := time.Now()
		starTime = &now
	}
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET is_star = $1, star_datetime = $2
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = $3 AND mu.user_id = $4 AND mu.is_active = TRUE
	`, starred, starTime, emailID, userID)
	return err
}

func (db *DB) UpdateEmailStatus(ctx context.Context, emailID, userID uuid.UUID, status models.EmailStatus) error {
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET status = $1
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = $2 AND mu.user_id = $3 AND mu.is_active = TRUE
	`, status, emailID, userID)
	return err
}

func (db *DB) UpdateEmailCategory(ctx context.Context, emailID, userID uuid.UUID, category string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET category = $1
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = $2 AND mu.user_id = $3 AND mu.is_active = TRUE
	`, category, emailID, userID)
	return err
}

func (db *DB) BulkMarkEmailRead(ctx context.Context, ids []uuid.UUID, userID uuid.UUID, read bool) error {
	var readTime *time.Time
	if read {
		now := time.Now()
		readTime = &now
	}
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET is_read = $1, read_datetime = $2
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = ANY($3) AND mu.user_id = $4 AND mu.is_active = TRUE
	`, read, readTime, ids, userID)
	return err
}

func (db *DB) BulkUpdateEmailStatus(ctx context.Context, ids []uuid.UUID, userID uuid.UUID, status models.EmailStatus) error {
	_, err := db.ExecContext(ctx, `
		UPDATE email e
		SET status = $1
		FROM mailbox_user mu
		WHERE e.mailbox_id = mu.mailbox_id AND e.id = ANY($2) AND mu.user_id = $3 AND mu.is_active = TRUE
	`, status, ids, userID)
	return err
}

func (db *DB) GetActiveSendingAddresses(ctx context.Context, userID uuid.UUID) ([]models.SendingAddress, error) {
	var addresses []models.SendingAddress
	err := db.SelectContext(ctx, &addresses, "SELECT id, user_id, mailbox_id, address, display_name, is_active FROM sending_address WHERE user_id = $1 AND is_active = TRUE ORDER BY address ASC", userID)
	return addresses, err
}

func (db *DB) IsAuthorizedSendingAddress(ctx context.Context, userID uuid.UUID, address string) (bool, error) {
	var count int
	err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM sending_address WHERE user_id = $1 AND address = $2 AND is_active = TRUE", userID, address)
	return count > 0, err
}

func (db *DB) GetSendingAddressByID(ctx context.Context, id, userID uuid.UUID) (*models.SendingAddress, error) {
	var sa models.SendingAddress
	err := db.GetContext(ctx, &sa, "SELECT id, user_id, mailbox_id, address, display_name, is_active FROM sending_address WHERE id = $1 AND user_id = $2 AND is_active = TRUE", id, userID)
	return &sa, err
}

func (db *DB) UpdateSendingAddressDisplayName(ctx context.Context, id, userID uuid.UUID, displayName string) error {
	_, err := db.ExecContext(ctx, "UPDATE sending_address SET display_name = $1 WHERE id = $2 AND user_id = $3", displayName, id, userID)
	return err
}

func (db *DB) GetMailboxUsers(ctx context.Context, mailboxID uuid.UUID) ([]models.User, error) {
	var users []models.User
	err := db.SelectContext(ctx, &users, `
		SELECT u.id, u.username, u.password_hash, u.is_active, u.is_admin
		FROM "user" u
		JOIN mailbox_user mu ON mu.user_id = u.id
		WHERE mu.mailbox_id = $1 AND mu.is_active = TRUE
		ORDER BY u.username ASC
	`, mailboxID)
	return users, err
}

func (db *DB) GetSendingAddressesByMailboxID(ctx context.Context, mailboxID uuid.UUID) ([]models.SendingAddress, error) {
	var addresses []models.SendingAddress
	err := db.SelectContext(ctx, &addresses, `
		SELECT id, user_id, mailbox_id, address, display_name, is_active
		FROM sending_address
		WHERE mailbox_id = $1 AND is_active = TRUE
		ORDER BY address ASC
	`, mailboxID)
	return addresses, err
}

func (db *DB) CreateTOTPPending(ctx context.Context, userID uuid.UUID, token string, expires time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO totp_pending (user_id, token, expires_datetime)
		VALUES ($1, $2, $3)
	`, userID, token, expires)
	return err
}

func (db *DB) GetTOTPPendingByToken(ctx context.Context, token string) (*models.TOTPPending, error) {
	var p models.TOTPPending
	err := db.GetContext(ctx, &p, `
		SELECT id, user_id, token, expires_datetime
		FROM totp_pending
		WHERE token = $1 AND expires_datetime > CURRENT_TIMESTAMP
	`, token)
	return &p, err
}

func (db *DB) DeleteTOTPPending(ctx context.Context, token string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM totp_pending WHERE token = $1`, token)
	return err
}

func (db *DB) SaveTOTPSetup(ctx context.Context, userID uuid.UUID, secret string, hashedCodes []string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE "user"
		SET totp_enabled = true, totp_secret = $2, totp_backup_codes = $3
		WHERE id = $1
	`, userID, secret, pq.StringArray(hashedCodes))
	return err
}

func (db *DB) DisableTOTP(ctx context.Context, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `
		UPDATE "user"
		SET totp_enabled = false, totp_secret = NULL, totp_backup_codes = '{}'
		WHERE id = $1
	`, userID)
	return err
}

func (db *DB) RemoveTOTPBackupCode(ctx context.Context, userID uuid.UUID, index int) error {
	var codes pq.StringArray
	if err := db.GetContext(ctx, &codes, `SELECT totp_backup_codes FROM "user" WHERE id = $1`, userID); err != nil {
		return err
	}
	if index >= 0 && index < len(codes) {
		codes = append(codes[:index], codes[index+1:]...)
	}
	_, err := db.ExecContext(ctx, `UPDATE "user" SET totp_backup_codes = $2 WHERE id = $1`, userID, codes)
	return err
}

func (db *DB) RegenerateTOTPBackupCodes(ctx context.Context, userID uuid.UUID, hashedCodes []string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE "user" SET totp_backup_codes = $2 WHERE id = $1
	`, userID, pq.StringArray(hashedCodes))
	return err
}


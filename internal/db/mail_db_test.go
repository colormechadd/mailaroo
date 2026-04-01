package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestLookupMailboxByAddress(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Setup seed data
	userID := uuid.New()
	_, err := db.ExecContext(ctx, `INSERT INTO "user" (id, username, password_hash) VALUES ($1, $2, $3)`, userID, "testuser", "hash")
	assert.NoError(t, err)

	mailboxID := uuid.New()
	_, err = db.ExecContext(ctx, `INSERT INTO mailbox (id, name) VALUES ($1, $2)`, mailboxID, "Inbox")
	assert.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO mailbox_user (mailbox_id, user_id) VALUES ($1, $2)`, mailboxID, userID)
	assert.NoError(t, err)

	mappingID := uuid.New()
	_, err = db.ExecContext(ctx, `INSERT INTO address_mapping (id, address_pattern, mailbox_id, priority) VALUES ($1, $2, $3, $4)`, mappingID, `.*@example.com`, mailboxID, 10)
	assert.NoError(t, err)

	t.Run("successful match", func(t *testing.T) {
		mb, mID, err := db.LookupMailboxByAddress(ctx, "hello@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, mb)
		assert.Equal(t, mailboxID, mb.ID)
		assert.Equal(t, mappingID, mID)
	})

	t.Run("no match", func(t *testing.T) {
		mb, mID, err := db.LookupMailboxByAddress(ctx, "hello@other.com")
		assert.Error(t, err)
		assert.Nil(t, mb)
		assert.Equal(t, uuid.Nil, mID)
	})

	t.Run("inactive mapping", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `UPDATE address_mapping SET is_active = FALSE WHERE id = $1`, mappingID)
		assert.NoError(t, err)

		mb, _, err := db.LookupMailboxByAddress(ctx, "hello@example.com")
		assert.Error(t, err)
		assert.Nil(t, mb)
	})
}

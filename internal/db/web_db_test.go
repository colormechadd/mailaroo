package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestWebDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed user
	userID := uuid.New()
	_, err := db.ExecContext(ctx, `INSERT INTO "user" (id, username, password_hash) VALUES ($1, $2, $3)`, userID, "webuser", "hash")
	assert.NoError(t, err)

	t.Run("Create and Get WebmailSession", func(t *testing.T) {
		token := "test-token"
		remoteIP := "127.0.0.1"
		userAgent := "Mozilla/5.0"
		expires := time.Now().Add(1 * time.Hour).Round(time.Microsecond)

		err := db.CreateWebmailSession(ctx, userID, token, remoteIP, userAgent, expires)
		assert.NoError(t, err)

		session, err := db.GetWebmailSession(ctx, token)
		assert.NoError(t, err)
		assert.NotNil(t, session)
		assert.Equal(t, userID, session.UserID)
		assert.Equal(t, token, session.Token)
		assert.Equal(t, remoteIP, *session.RemoteIP)
		assert.Equal(t, userAgent, *session.UserAgent)
		assert.True(t, expires.Equal(session.ExpiresDatetime))
	})

	t.Run("Expire WebmailSession", func(t *testing.T) {
		token := "expire-me"
		err := db.CreateWebmailSession(ctx, userID, token, "::1", "curl", time.Now().Add(1*time.Hour))
		assert.NoError(t, err)

		err = db.ExpireWebmailSession(ctx, token)
		assert.NoError(t, err)

		session, err := db.GetWebmailSession(ctx, token)
		assert.NoError(t, err)
		assert.True(t, session.ExpiresDatetime.Before(time.Now().Add(time.Second)))
	})
}

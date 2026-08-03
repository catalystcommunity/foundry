package webui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExchangeAndSessionExpiry(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewAuthStore()
	store.now = func() time.Time { return now }
	store.AddToken("admin", "a-long-test-token", time.Hour)

	_, ok := store.exchange("wrong-token", time.Hour)
	assert.False(t, ok)
	session, ok := store.exchange("a-long-test-token", time.Hour)
	require.True(t, ok)
	require.NotEmpty(t, session)

	record, ok := store.authenticateSession(session, 15*time.Minute)
	require.True(t, ok)
	assert.Equal(t, "admin", record.TokenName)

	now = now.Add(16 * time.Minute)
	_, ok = store.authenticateSession(session, 15*time.Minute)
	assert.False(t, ok)
}

func TestExpiredAccessTokenIsRejected(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewAuthStore()
	store.now = func() time.Time { return now }
	store.AddToken("admin", "a-long-test-token", time.Minute)
	now = now.Add(2 * time.Minute)

	_, ok := store.authenticateToken("a-long-test-token")
	assert.False(t, ok)
}

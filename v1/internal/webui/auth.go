package webui

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const sessionCookieName = "foundry_session"

type tokenRecord struct {
	Name      string
	Hash      [sha256.Size]byte
	ExpiresAt time.Time
}

type sessionRecord struct {
	TokenName string
	ExpiresAt time.Time
	LastSeen  time.Time
}

// AuthStore holds high-entropy access tokens and revocable browser sessions.
// It stores token hashes instead of token plaintext.
type AuthStore struct {
	mu       sync.Mutex
	tokens   []tokenRecord
	sessions map[[sha256.Size]byte]sessionRecord
	now      func() time.Time
}

// NewAuthStore returns an empty authentication store.
func NewAuthStore() *AuthStore {
	return &AuthStore{sessions: make(map[[sha256.Size]byte]sessionRecord), now: time.Now}
}

// NewToken creates and registers a high-entropy access token.
func (s *AuthStore) NewToken(name string, lifetime time.Duration) (string, error) {
	raw, err := randomToken()
	if err != nil {
		return "", err
	}
	s.AddToken(name, raw, lifetime)
	return raw, nil
}

// AddToken registers an existing high-entropy access token.
func (s *AuthStore) AddToken(name, token string, lifetime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, tokenRecord{Name: name, Hash: sha256.Sum256([]byte(token)), ExpiresAt: s.now().Add(lifetime)})
}

func (s *AuthStore) exchange(token string, lifetime time.Duration) (string, bool) {
	tokenHash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	name := ""
	for _, record := range s.tokens {
		if now.After(record.ExpiresAt) {
			continue
		}
		if subtle.ConstantTimeCompare(tokenHash[:], record.Hash[:]) == 1 {
			name = record.Name
			break
		}
	}
	if name == "" {
		return "", false
	}

	session, err := randomToken()
	if err != nil {
		return "", false
	}
	s.sessions[sha256.Sum256([]byte(session))] = sessionRecord{
		TokenName: name,
		ExpiresAt: now.Add(lifetime),
		LastSeen:  now,
	}
	return session, true
}

func (s *AuthStore) authenticateSession(token string, idleTimeout time.Duration) (sessionRecord, bool) {
	hash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[hash]
	if !ok {
		return sessionRecord{}, false
	}
	now := s.now()
	if now.After(record.ExpiresAt) || now.Sub(record.LastSeen) > idleTimeout {
		delete(s.sessions, hash)
		return sessionRecord{}, false
	}
	record.LastSeen = now
	s.sessions[hash] = record
	return record, true
}

func (s *AuthStore) authenticateToken(token string) (tokenRecord, bool) {
	hash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, record := range s.tokens {
		if now.After(record.ExpiresAt) {
			continue
		}
		if subtle.ConstantTimeCompare(hash[:], record.Hash[:]) == 1 {
			return record, true
		}
	}
	return tokenRecord{}, false
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

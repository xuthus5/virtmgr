package consoletoken

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

type Store struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	grants map[string]domain.ConsoleToken
}

func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Store{ttl: ttl, now: time.Now, grants: make(map[string]domain.ConsoleToken)}
}

func (s *Store) Issue(vmUUID string, userID int64) (domain.ConsoleToken, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return domain.ConsoleToken{}, fmt.Errorf("generate console token: %w", err)
	}
	grant := domain.ConsoleToken{
		Token:     base64.RawURLEncoding.EncodeToString(buf),
		VMUUID:    vmUUID,
		UserID:    userID,
		ExpiresAt: s.now().Add(s.ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked()
	s.grants[grant.Token] = grant
	return grant, nil
}

func (s *Store) Consume(token string) (domain.ConsoleToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[token]
	if !ok {
		return domain.ConsoleToken{}, domain.ErrConsoleToken
	}
	delete(s.grants, token)
	if !s.now().Before(grant.ExpiresAt) {
		return domain.ConsoleToken{}, domain.ErrConsoleToken
	}
	return grant, nil
}

func (s *Store) removeExpiredLocked() {
	now := s.now()
	for token, grant := range s.grants {
		if !now.Before(grant.ExpiresAt) {
			delete(s.grants, token)
		}
	}
}

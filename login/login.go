// Package login layers login sessions over the auth package: a Manager
// authenticates credentials, mints a random token per login, and expires
// sessions after an idle timeout. The clock is injectable so timeout behavior
// is testable without sleeping. The package depends only on the standard
// library and auth.
package login

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/uk0/silk/auth"
)

// Session is an authenticated login. Token is the opaque handle a client
// presents; LastSeen advances on each successful Validate to keep the session
// alive within the idle timeout.
type Session struct {
	Token    string
	User     *auth.User
	Created  time.Time
	LastSeen time.Time
}

// Manager issues and tracks sessions backed by an auth.Store. It is safe for
// concurrent use.
type Manager struct {
	store   *auth.Store
	timeout time.Duration

	// Now returns the current time; it defaults to time.Now and may be
	// replaced (typically in tests) to drive timeout behavior deterministically.
	Now func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager returns a Manager that authenticates against store and expires
// sessions idle for longer than timeout.
func NewManager(store *auth.Store, timeout time.Duration) *Manager {
	return &Manager{
		store:    store,
		timeout:  timeout,
		Now:      time.Now,
		sessions: make(map[string]*Session),
	}
}

// newToken returns a hex-encoded 16-byte random token.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Login authenticates name/password against the store and, on success, mints a
// new session with a random token. It returns an error on bad credentials.
func (m *Manager) Login(name, password string) (*Session, error) {
	u, ok := m.store.Authenticate(name, password)
	if !ok {
		return nil, errors.New("session: invalid credentials")
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := m.Now()
	s := &Session{
		Token:    token,
		User:     u,
		Created:  now,
		LastSeen: now,
	}
	m.mu.Lock()
	m.sessions[token] = s
	m.mu.Unlock()
	return s, nil
}

// Validate reports whether token names a live session. A session is live when
// it has been seen within timeout; Validate then refreshes LastSeen and returns
// it. An expired (or unknown) token yields false, and an expired session is
// removed.
func (m *Manager) Validate(token string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return nil, false
	}
	now := m.Now()
	if now.Sub(s.LastSeen) > m.timeout {
		delete(m.sessions, token)
		return nil, false
	}
	s.LastSeen = now
	return s, true
}

// Logout invalidates token. It is a no-op for an unknown token.
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// Current returns the user behind a live token, refreshing the session. It
// returns false when the token is unknown or expired.
func (m *Manager) Current(token string) (*auth.User, bool) {
	s, ok := m.Validate(token)
	if !ok {
		return nil, false
	}
	return s.User, true
}

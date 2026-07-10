package login

import (
	"testing"
	"time"

	"github.com/uk0/silk/auth"
)

// newTestManager builds a Manager with one user ("alice"/"pw") and a pinned,
// advanceable clock. Advance the returned *time.Time to move time forward.
func newTestManager(t *testing.T, timeout time.Duration) (*Manager, *time.Time) {
	t.Helper()
	store := auth.NewStore()
	if err := store.AddUser("alice", "pw", auth.Operator); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	m := NewManager(store, timeout)
	clock := time.Unix(1_000, 0)
	m.Now = func() time.Time { return clock }
	return m, &clock
}

func TestNewManagerDefaultClock(t *testing.T) {
	m := NewManager(auth.NewStore(), time.Minute)
	if m.Now == nil {
		t.Fatal("NewManager left Now nil")
	}
}

func TestLoginSuccess(t *testing.T) {
	m, clock := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if len(s.Token) != 32 { // 16 random bytes hex-encoded
		t.Fatalf("token length = %d, want 32 (%q)", len(s.Token), s.Token)
	}
	if s.User == nil || s.User.Name != "alice" {
		t.Fatalf("session user = %+v, want alice", s.User)
	}
	if !s.Created.Equal(*clock) || !s.LastSeen.Equal(*clock) {
		t.Fatalf("Created=%v LastSeen=%v, want both %v", s.Created, s.LastSeen, *clock)
	}
}

func TestLoginWrongCredentials(t *testing.T) {
	m, _ := newTestManager(t, time.Minute)
	if _, err := m.Login("alice", "nope"); err == nil {
		t.Fatal("wrong password: expected error")
	}
	if _, err := m.Login("ghost", "pw"); err == nil {
		t.Fatal("unknown user: expected error")
	}
}

func TestValidateFreshRefreshes(t *testing.T) {
	m, clock := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	*clock = clock.Add(30 * time.Second)
	got, ok := m.Validate(s.Token)
	if !ok {
		t.Fatal("fresh token: expected valid")
	}
	if !got.LastSeen.Equal(*clock) {
		t.Fatalf("LastSeen=%v, want refreshed to %v", got.LastSeen, *clock)
	}

	// The refresh reset the idle window: 40s more (70s since Created but only
	// 40s since the refresh) is still under the 1m timeout.
	*clock = clock.Add(40 * time.Second)
	if _, ok := m.Validate(s.Token); !ok {
		t.Fatal("expected still valid after refresh reset the idle window")
	}
}

func TestValidateBoundary(t *testing.T) {
	m, clock := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Exactly at timeout is still valid (Now-LastSeen <= timeout).
	*clock = clock.Add(time.Minute)
	if _, ok := m.Validate(s.Token); !ok {
		t.Fatal("at exactly timeout: expected valid")
	}
}

func TestValidateTimeoutRemoves(t *testing.T) {
	m, clock := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	*clock = clock.Add(time.Minute + time.Second)
	if _, ok := m.Validate(s.Token); ok {
		t.Fatal("past timeout: expected invalid")
	}

	// Validate must have deleted the expired entry.
	m.mu.Lock()
	_, present := m.sessions[s.Token]
	m.mu.Unlock()
	if present {
		t.Fatal("expired session was not removed from the map")
	}
}

func TestLogoutInvalidates(t *testing.T) {
	m, _ := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	m.Logout(s.Token)
	if _, ok := m.Validate(s.Token); ok {
		t.Fatal("after logout: expected invalid")
	}
	m.Logout(s.Token) // no-op on unknown token, must not panic
}

func TestLoginDistinctTokens(t *testing.T) {
	m, _ := newTestManager(t, time.Minute)
	s1, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login 1: %v", err)
	}
	s2, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login 2: %v", err)
	}
	if s1.Token == s2.Token {
		t.Fatalf("two logins produced the same token: %q", s1.Token)
	}
}

func TestCurrent(t *testing.T) {
	m, clock := newTestManager(t, time.Minute)
	s, err := m.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	u, ok := m.Current(s.Token)
	if !ok || u == nil || u.Name != "alice" {
		t.Fatalf("Current(live) = %v, %v; want alice, true", u, ok)
	}
	if _, ok := m.Current("unknown"); ok {
		t.Fatal("Current(unknown): expected false")
	}

	*clock = clock.Add(2 * time.Minute)
	if _, ok := m.Current(s.Token); ok {
		t.Fatal("Current(expired): expected false")
	}
}

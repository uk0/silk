// Package auth provides in-memory user credentials and role-based action
// permissions, modeled on FameView (杰控) 用户权限: operators, engineers and
// admins with gated actions. Credentials are salted SHA-256 hashes; the
// package depends only on the standard library.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
)

// Role is a privilege level. Higher values grant more privilege.
type Role int

const (
	Operator Role = iota // run screens, write tags
	Engineer             // edit configuration
	Admin                // manage users, everything
)

// String returns the human-readable role name.
func (r Role) String() string {
	switch r {
	case Operator:
		return "Operator"
	case Engineer:
		return "Engineer"
	case Admin:
		return "Admin"
	default:
		return "Unknown"
	}
}

// User is a named account with a role. Credentials (salt and hash) are
// unexported and never leave the package.
type User struct {
	Name string
	Role Role

	salt string // hex-encoded random salt
	hash string // hex-encoded sha256(salt + password)
}

// Store is an in-memory set of users keyed by name.
type Store struct {
	users map[string]*User
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{users: make(map[string]*User)}
}

// newSalt returns a hex-encoded 16-byte random salt.
func newSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashPassword returns the hex-encoded sha256 of salt concatenated with the
// password.
func hashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(sum[:])
}

// AddUser registers a new user with the given role. The name must be non-empty
// and not already present. The password is stored as a salted hash.
func (s *Store) AddUser(name, password string, role Role) error {
	if name == "" {
		return errors.New("auth: empty user name")
	}
	if _, exists := s.users[name]; exists {
		return errors.New("auth: duplicate user: " + name)
	}
	salt, err := newSalt()
	if err != nil {
		return err
	}
	s.users[name] = &User{
		Name: name,
		Role: role,
		salt: salt,
		hash: hashPassword(password, salt),
	}
	return nil
}

// Authenticate returns the user and true when name exists and password matches.
// The stored hash is compared in constant time; a missing user still performs a
// hash so timing does not reveal whether the account exists.
func (s *Store) Authenticate(name, password string) (*User, bool) {
	u, ok := s.users[name]
	if !ok {
		_ = hashPassword(password, "")
		return nil, false
	}
	got := hashPassword(password, u.salt)
	if subtle.ConstantTimeCompare([]byte(got), []byte(u.hash)) == 1 {
		return u, true
	}
	return nil, false
}

// SetPassword replaces an existing user's password, generating a fresh salt.
func (s *Store) SetPassword(name, password string) error {
	u, ok := s.users[name]
	if !ok {
		return errors.New("auth: unknown user: " + name)
	}
	salt, err := newSalt()
	if err != nil {
		return err
	}
	u.salt = salt
	u.hash = hashPassword(password, salt)
	return nil
}

// Action names a gated operation.
type Action string

const (
	ActionViewScreen  Action = "view_screen"  // observe a running screen
	ActionWriteTag    Action = "write_tag"    // write a value to a tag
	ActionEditConfig  Action = "edit_config"  // change project configuration
	ActionManageUsers Action = "manage_users" // add/remove users, set passwords
)

// actionMinRole maps each action to the minimum role that may perform it.
var actionMinRole = map[Action]Role{
	ActionViewScreen:  Operator,
	ActionWriteTag:    Operator,
	ActionEditConfig:  Engineer,
	ActionManageUsers: Admin,
}

// Can reports whether user u may perform action a. A nil user, or an unknown
// action, is denied.
func Can(u *User, a Action) bool {
	if u == nil {
		return false
	}
	required, ok := actionMinRole[a]
	if !ok {
		return false
	}
	return u.Role >= required
}

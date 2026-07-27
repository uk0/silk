package runconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Store is the ordered collection of a project's named configurations.
// It enforces two invariants on every mutation: names are unique
// (compared after trimming, case-sensitively), and at most one Config
// carries the Default flag.
//
// The zero Store is an empty, usable store.
type Store struct {
	cfgs []Config
}

// storeFile is the on-disk shape. Keeping it separate from Store means
// the slice stays unexported and every write goes through the invariant
// checks.
type storeFile struct {
	Configs []Config `json:"configs"`
}

// NewStore returns an empty store.
func NewStore() *Store { return &Store{} }

// StorePath returns the conventional location of a project's run
// configurations, given the project's root directory.
func StorePath(projectDir string) string {
	return filepath.Join(projectDir, ".silkide", "runconfig.json")
}

// Len returns the number of configurations.
func (s *Store) Len() int { return len(s.cfgs) }

// List returns every configuration in store order, deep-copied so the
// caller cannot mutate the store through the result.
func (s *Store) List() []Config {
	out := make([]Config, 0, len(s.cfgs))
	for _, c := range s.cfgs {
		out = append(out, c.Clone())
	}
	return out
}

// Get returns a copy of the configuration named name.
func (s *Store) Get(name string) (Config, bool) {
	i := s.indexOf(name)
	if i < 0 {
		return Config{}, false
	}
	return s.cfgs[i].Clone(), true
}

// Add appends c. It fails with a validation error from Config.Validate,
// or ErrDuplicateName when the name is already taken. When c is flagged
// Default the flag is cleared on every other configuration.
func (s *Store) Add(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	name := strings.TrimSpace(c.Name)
	if s.indexOf(name) >= 0 {
		return ErrDuplicateName
	}
	stored := c.Clone()
	stored.Name = name
	if stored.Default {
		s.clearDefault()
	}
	s.cfgs = append(s.cfgs, stored)
	return nil
}

// Update replaces the configuration named name with c, keeping its
// position in the list. c may carry a different Name — a rename — as
// long as the new name is free. Returns ErrNotFound when name is
// unknown, ErrDuplicateName when the rename collides, or a validation
// error from Config.Validate; the store is untouched in every failure
// case.
//
// Clearing c.Default drops the explicit default; Default then falls back
// to the first configuration.
func (s *Store) Update(name string, c Config) error {
	i := s.indexOf(name)
	if i < 0 {
		return ErrNotFound
	}
	if err := c.Validate(); err != nil {
		return err
	}
	newName := strings.TrimSpace(c.Name)
	if j := s.indexOf(newName); j >= 0 && j != i {
		return ErrDuplicateName
	}
	stored := c.Clone()
	stored.Name = newName
	if stored.Default {
		s.clearDefault()
	}
	s.cfgs[i] = stored
	return nil
}

// Remove deletes the configuration named name, reporting whether one was
// removed. Removing the default leaves no explicit default, so Default
// falls back to the first remaining configuration.
func (s *Store) Remove(name string) bool {
	i := s.indexOf(name)
	if i < 0 {
		return false
	}
	s.cfgs = append(s.cfgs[:i], s.cfgs[i+1:]...)
	return true
}

// SetDefault makes name the default configuration, clearing the flag
// everywhere else. Reports whether name exists.
func (s *Store) SetDefault(name string) bool {
	i := s.indexOf(name)
	if i < 0 {
		return false
	}
	s.clearDefault()
	s.cfgs[i].Default = true
	return true
}

// Default returns the configuration to launch when the caller does not
// name one: the flagged one, or the first in list order when no flag is
// set. Reports false only for an empty store.
func (s *Store) Default() (Config, bool) {
	if len(s.cfgs) == 0 {
		return Config{}, false
	}
	for _, c := range s.cfgs {
		if c.Default {
			return c.Clone(), true
		}
	}
	return s.cfgs[0].Clone(), true
}

// Save writes the store to path as indented JSON, creating the parent
// directory when needed (path is normally under a project-local dot
// directory that may not exist yet).
func (s *Store) Save(path string) error {
	data, err := json.MarshalIndent(storeFile{Configs: s.cfgs}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads a store previously written by Save. Every entry is replayed
// through Add, so a hand-edited file that violates validation or repeats
// a name is reported instead of silently loaded half-broken.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	s := NewStore()
	for _, c := range f.Configs {
		if err := s.Add(c); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// indexOf returns the position of the configuration named name, or -1.
func (s *Store) indexOf(name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for i := range s.cfgs {
		if s.cfgs[i].Name == name {
			return i
		}
	}
	return -1
}

// clearDefault drops the Default flag from every configuration.
func (s *Store) clearDefault() {
	for i := range s.cfgs {
		s.cfgs[i].Default = false
	}
}

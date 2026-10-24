// Package filetxn stages generated file replacements with optimistic conflict
// checks and rollback on ordinary I/O failure. A set of files is NOT crash-
// atomic, and Rename is not guaranteed atomic on every OS/filesystem. Recovery
// copies are retained if rollback cannot safely restore a previous file.
package filetxn

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrConflict = errors.New("filetxn: file changed since snapshot")

// Snapshot captures a regular file (or an absent path); symlinks are rejected.
// Keep snapshots on the caller's serialized save path. These checks cannot lock
// out an unrelated process writing in the narrow check/rename interval.
type Snapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func Read(path string) (Snapshot, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{path: path, mode: 0644}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !info.Mode().IsRegular() {
		return s, fmt.Errorf("filetxn: not a regular file: %s", path)
	}
	s.data, err = os.ReadFile(path)
	if err != nil {
		return s, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return s, err
	}
	if !os.SameFile(info, after) || !after.Mode().IsRegular() || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
		return s, fmt.Errorf("%w: %s", ErrConflict, path)
	}
	s.mode = info.Mode().Perm()
	s.exists = true
	return s, nil
}
func (s Snapshot) Bytes() []byte { return bytes.Clone(s.data) }
func (s Snapshot) Exists() bool  { return s.exists }
func (s Snapshot) Path() string  { return s.path }

type Change struct {
	Before Snapshot
	Data   []byte
}
type stage struct {
	change         Change
	temp, backup   string
	stagedMode     os.FileMode
	preserveBackup bool
}

func matches(a, b Snapshot) bool {
	return a.exists == b.exists && (!a.exists || (a.mode == b.mode && bytes.Equal(a.data, b.data)))
}
func unchanged(s Snapshot) error {
	actual, err := Read(s.path)
	if err != nil {
		return err
	}
	if !matches(s, actual) {
		return fmt.Errorf("%w: %s", ErrConflict, s.path)
	}
	return nil
}
func tempFile(path, prefix string, data []byte, mode os.FileMode) (name string, err error) {
	f, err := os.CreateTemp(filepath.Dir(path), prefix)
	if err != nil {
		return "", err
	}
	name = f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(name)
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return name, err
	}
	if _, err = f.Write(data); err != nil {
		return name, err
	}
	if err = f.Sync(); err != nil {
		return name, err
	}
	err = f.Close()
	return name, err
}

func Commit(changes ...Change) error { return commit(changes, os.Rename) }
func commit(changes []Change, rename func(string, string) error) (err error) {
	var staged []*stage
	defer func() {
		for _, s := range staged {
			if s.temp != "" {
				_ = os.Remove(s.temp)
			}
			if s.backup != "" && !s.preserveBackup {
				_ = os.Remove(s.backup)
			}
		}
	}()
	seen := map[string]bool{}
	// Validate the complete plan before touching any destination.
	for _, change := range changes {
		s := change.Before
		if s.path == "" || seen[s.path] {
			return errors.New("filetxn: invalid or duplicate destination")
		}
		seen[s.path] = true
		if err := unchanged(s); err != nil {
			return err
		}
	}
	for _, change := range changes {
		s := change.Before
		if s.exists && bytes.Equal(s.data, change.Data) {
			continue
		}
		part := &stage{change: change}
		staged = append(staged, part)
		if part.temp, err = tempFile(s.path, ".silk-stage-*", change.Data, s.mode); err != nil {
			return err
		}
		// Record what the filesystem actually applied, not the requested Unix
		// mode. Windows maps writable 0644/0600 files to 0666; comparing a new
		// destination against its synthetic absent-path mode blocks rollback.
		info, statErr := os.Lstat(part.temp)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("filetxn: staged output is not regular: %s", part.temp)
		}
		part.stagedMode = info.Mode().Perm()
		if s.exists {
			if part.backup, err = tempFile(s.path, ".silk-backup-*", s.data, s.mode); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		if err := unchanged(change.Before); err != nil {
			return err
		}
	}
	applied := 0
	for _, part := range staged {
		if err = unchanged(part.change.Before); err == nil {
			err = rename(part.temp, part.change.Before.path)
		}
		if err != nil {
			break
		}
		part.temp = ""
		applied++
	}
	if err == nil {
		return nil
	}
	failure := err
	for i := applied - 1; i >= 0; i-- {
		part := staged[i]
		before := part.change.Before
		actual, readErr := Read(before.path)
		if readErr != nil || !actual.exists || actual.mode != part.stagedMode || !bytes.Equal(actual.data, part.change.Data) {
			part.preserveBackup = true
			failure = errors.Join(failure, fmt.Errorf("filetxn: rollback conflict at %s; recovery copy %s", before.path, part.backup))
			continue
		}
		var restoreErr error
		if before.exists {
			restoreErr = rename(part.backup, before.path)
			if restoreErr == nil {
				part.backup = ""
			}
		} else {
			restoreErr = os.Remove(before.path)
		}
		if restoreErr != nil {
			part.preserveBackup = true
			failure = errors.Join(failure, fmt.Errorf("filetxn: rollback %s: %w; recovery copy %s", before.path, restoreErr, part.backup))
		}
	}
	return failure
}

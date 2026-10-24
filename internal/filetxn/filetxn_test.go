package filetxn

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func snapshot(t *testing.T, path, text string) Snapshot {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func assertContents(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != want {
		t.Fatalf("%s: %q %v, want %q", path, b, err, want)
	}
}
func TestCommitConflictNoopAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machine.go")
	s := snapshot(t, path, "old")
	before, _ := os.Stat(path)
	if err := Commit(Change{s, []byte("old")}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || before.ModTime() != after.ModTime() {
		t.Fatal("unchanged output replaced")
	}
	if err := Commit(Change{s, []byte("new")}); err != nil {
		t.Fatal(err)
	}
	assertContents(t, path, "new")
	after, _ = os.Stat(path)
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("permission changed")
	}
	if err := Commit(Change{s, []byte("stale")}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	assertContents(t, path, "new")
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("temp files leaked", files)
	}
}
func TestCommitPreflightAndRollback(t *testing.T) {
	dir := t.TempDir()
	a := snapshot(t, filepath.Join(dir, "a.go"), "a-old")
	b := snapshot(t, filepath.Join(dir, "b.go"), "b-old")
	changes := []Change{{a, []byte("a-new")}, {b, []byte("b-new")}}
	injected := errors.New("injected rename failure")
	err := commit(changes, func(from, to string) error {
		if to == b.path {
			return injected
		}
		return os.Rename(from, to)
	})
	if !errors.Is(err, injected) {
		t.Fatal(err)
	}
	assertContents(t, a.path, "a-old")
	assertContents(t, b.path, "b-old")
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatal("ordinary rollback leaked backups", files)
	}
	missing, err := Read(filepath.Join(dir, "missing", "file.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Commit(Change{a, []byte("changed")}, Change{missing, []byte("new")}); err == nil {
		t.Fatal("missing directory accepted")
	}
	assertContents(t, a.path, "a-old")
}
func TestCommitRollbackKeepsConcurrentEditAndRecovery(t *testing.T) {
	dir := t.TempDir()
	a := snapshot(t, filepath.Join(dir, "a.go"), "original")
	b := snapshot(t, filepath.Join(dir, "b.go"), "user")
	err := commit([]Change{{a, []byte("generated")}, {b, []byte("appended")}}, func(from, to string) error {
		if to == b.path {
			if err := os.WriteFile(a.path, []byte("external change"), 0600); err != nil {
				t.Fatal(err)
			}
			return errors.New("failed second replacement")
		}
		return os.Rename(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "recovery copy") {
		t.Fatal(err)
	}
	assertContents(t, a.path, "external change")
	assertContents(t, b.path, "user")
	backups, _ := filepath.Glob(filepath.Join(dir, ".silk-backup-*"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	assertContents(t, backups[0], "original")
}
func TestCommitRejectsSymlinksAndDuplicatePaths(t *testing.T) {
	dir := t.TempDir()
	a := snapshot(t, filepath.Join(dir, "a.go"), "user")
	if err := Commit(Change{a, []byte("one")}, Change{a, []byte("two")}); err == nil {
		t.Fatal("duplicate destinations accepted")
	}
	assertContents(t, a.path, "user")
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(a.path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("host does not permit symlinks")
		}
		t.Fatal(err)
	}
	if _, err := Read(link); err == nil {
		t.Fatal("symlink accepted")
	}
	assertContents(t, a.path, "user")
}
func TestRollbackRemovesOnlyNewDestination(t *testing.T) {
	dir := t.TempDir()
	a, err := Read(filepath.Join(dir, "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	b := snapshot(t, filepath.Join(dir, "b.go"), "user")
	err = commit([]Change{{a, []byte("new")}, {b, []byte("changed")}}, func(from, to string) error {
		if to == b.path {
			return errors.New("failed")
		}
		return os.Rename(from, to)
	})
	if err == nil {
		t.Fatal("missing error")
	}
	if _, err := os.Stat(a.path); !os.IsNotExist(err) {
		t.Fatal("new output survived rollback", err)
	}
	assertContents(t, b.path, "user")
}

func TestRollbackPreservesConcurrentNewDestinationEdit(t *testing.T) {
	dir := t.TempDir()
	a, err := Read(filepath.Join(dir, "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	b := snapshot(t, filepath.Join(dir, "b.go"), "user")
	err = commit([]Change{{a, []byte("generated")}, {b, []byte("changed")}}, func(from, to string) error {
		if to == b.path {
			if err := os.WriteFile(a.path, []byte("external change"), 0600); err != nil {
				t.Fatal(err)
			}
			return errors.New("failed second replacement")
		}
		return os.Rename(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("missing rollback conflict: %v", err)
	}
	assertContents(t, a.path, "external change")
	assertContents(t, b.path, "user")
}

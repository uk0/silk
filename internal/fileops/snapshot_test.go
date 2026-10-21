package fileops

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotReaderRetainsOldFileAcrossReplacement(t *testing.T) {
	dir := t.TempDir()
	to, from := filepath.Join(dir, "target"), filepath.Join(dir, "new")
	if err := os.WriteFile(to, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(from, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenSnapshot(to)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = ReplaceSnapshotFile(from, to); err != nil {
		t.Fatal(err)
	}
	held, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(held, []byte("old")) {
		t.Fatalf("held=%q %v", held, err)
	}
	current, err := ReadSnapshot(to)
	if err != nil || !bytes.Equal(current, []byte("new")) {
		t.Fatalf("current=%q %v", current, err)
	}
	if _, err = os.Stat(from); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source survived replacement: %v", err)
	}
}
func TestSnapshotErrorsRetainPathAndCause(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if _, err := OpenSnapshot(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := ReplaceSnapshotFile(path, path+"2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestSnapshotLongUnicodeAndRelativeNames(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 9; i++ {
		dir = filepath.Join(dir, "long-document-component-0123456789")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	to, from := filepath.Join(dir, "中文 settings.tdoc"), filepath.Join(dir, "replacement.tdoc")
	if err := os.WriteFile(to, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(from, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, to)
	if err != nil {
		t.Fatal(err)
	}
	held, err := OpenSnapshot(relative)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err = ReplaceSnapshotFile(from, to); err != nil {
		t.Fatal(err)
	}
	data, err := ReadSnapshot(relative)
	if err != nil || string(data) != "new" {
		t.Fatalf("long relative path=%q: %v", data, err)
	}
	old, err := io.ReadAll(held)
	if err != nil || string(old) != "old" {
		t.Fatalf("old long path=%q: %v", old, err)
	}
	if _, err = OpenSnapshot(""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty path error=%v", err)
	}
}

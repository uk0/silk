package ged

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewItemDir: a directory entry creates into itself, a file entry
// creates into its parent, and a nil entry (empty-space right-click)
// creates into the explorer root.
func TestNewItemDir(t *testing.T) {
	root := "/proj"
	dirEntry := &fileEntry{path: "/proj/pkg", isDir: true}
	fileEntry := &fileEntry{path: "/proj/pkg/main.go", isDir: false}

	if got := newItemDir(dirEntry, root); got != "/proj/pkg" {
		t.Errorf("dir entry: got %q, want /proj/pkg", got)
	}
	if got := newItemDir(fileEntry, root); got != "/proj/pkg" {
		t.Errorf("file entry: got %q, want /proj/pkg (parent dir)", got)
	}
	if got := newItemDir(nil, root); got != root {
		t.Errorf("nil entry: got %q, want %q", got, root)
	}
}

// TestValidateFileName rejects the names that would escape the target
// directory or alias the dot entries, and accepts ordinary components.
func TestValidateFileName(t *testing.T) {
	bad := []string{"", "   ", ".", "..", "a/b", `a\b`, "/etc", "sub/file.go"}
	for _, n := range bad {
		if err := validateFileName(n); err == nil {
			t.Errorf("validateFileName(%q) = nil, want error", n)
		}
	}
	good := []string{"main.go", "README.md", "my-folder", "a.b.c", " trimmed.go "}
	for _, n := range good {
		if err := validateFileName(n); err != nil {
			t.Errorf("validateFileName(%q) = %v, want nil", n, err)
		}
	}
}

// TestFsCreateFile creates a file, confirms it lands on disk empty, and
// that a second create at the same name fails (O_EXCL, never truncates).
func TestFsCreateFile(t *testing.T) {
	dir := t.TempDir()
	path, err := fsCreateFile(dir, "new.go")
	if err != nil {
		t.Fatalf("fsCreateFile: %v", err)
	}
	if path != filepath.Join(dir, "new.go") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir, "new.go"))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat created file: %v", err)
	}
	if info.IsDir() || info.Size() != 0 {
		t.Errorf("created file should be empty regular file, got dir=%v size=%d", info.IsDir(), info.Size())
	}

	// Second create must fail rather than truncate.
	if _, err := fsCreateFile(dir, "new.go"); err == nil {
		t.Error("second fsCreateFile at same name should fail (already exists)")
	}
	// Invalid name rejected before touching disk.
	if _, err := fsCreateFile(dir, "a/b.go"); err == nil {
		t.Error("fsCreateFile with path separator should fail")
	}
}

// TestFsCreateFolder mirrors the file test for directories.
func TestFsCreateFolder(t *testing.T) {
	dir := t.TempDir()
	path, err := fsCreateFolder(dir, "pkg")
	if err != nil {
		t.Fatalf("fsCreateFolder: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected created dir at %q, err=%v", path, err)
	}
	if _, err := fsCreateFolder(dir, "pkg"); err == nil {
		t.Error("second fsCreateFolder at same name should fail")
	}
}

// TestFsRename renames within the same parent, treats a same-name
// rename as a no-op success, and rejects a collision.
func TestFsRename(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.go")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	np, err := fsRename(old, "renamed.go")
	if err != nil {
		t.Fatalf("fsRename: %v", err)
	}
	if np != filepath.Join(dir, "renamed.go") {
		t.Errorf("new path = %q", np)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old path should no longer exist after rename")
	}
	if _, err := os.Stat(np); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}

	// No-op rename (same base) succeeds and keeps the path.
	if got, err := fsRename(np, "renamed.go"); err != nil || got != np {
		t.Errorf("no-op rename: got %q err %v, want %q nil", got, err, np)
	}

	// Collision with an existing sibling fails.
	other := filepath.Join(dir, "other.go")
	if err := os.WriteFile(other, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fsRename(np, "other.go"); err == nil {
		t.Error("rename onto an existing name should fail")
	}
}

// TestFsDelete removes both files and directory trees, and rejects an
// empty path.
func TestFsDelete(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "f.go")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsDelete(file); err != nil {
		t.Fatalf("fsDelete file: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("file should be gone after delete")
	}

	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested", "x.go"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsDelete(sub); err != nil {
		t.Fatalf("fsDelete dir tree: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("directory tree should be gone after delete")
	}

	if err := fsDelete("   "); err == nil {
		t.Error("fsDelete on blank path should error")
	}
}

// TestRefreshKeepingExpansionPreservesOpenFolders builds a small tree on
// disk, expands a nested folder, then refreshes after creating a sibling
// file and confirms the nested folder stays expanded (unlike the plain
// Refresh()/SetRootDir which collapse everything but the root).
func TestRefreshKeepingExpansionPreservesOpenFolders(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "a.go"), []byte("package pkg"), 0o644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExplorer()
	fe.SetRootDir(root)

	// Expand the nested "pkg" folder in the model.
	fe.expandDir(pkg)
	fe.rebuildFlatList()

	// Create a new sibling file and refresh preserving expansion.
	if _, err := fsCreateFile(root, "new.go"); err != nil {
		t.Fatal(err)
	}
	fe.refreshKeepingExpansion()

	// Find the pkg node post-refresh and confirm it is still expanded.
	var found *fileEntry
	var walk func(e *fileEntry)
	walk = func(e *fileEntry) {
		if e == nil {
			return
		}
		if e.path == pkg {
			found = e
			return
		}
		for _, c := range e.children {
			walk(c)
		}
	}
	walk(fe.root)
	if found == nil {
		t.Fatal("pkg node missing after refresh")
	}
	if !found.expanded {
		t.Error("refreshKeepingExpansion collapsed a folder that was open before the refresh")
	}
	// The new file must appear in the rescanned tree.
	if _, err := os.Stat(filepath.Join(root, "new.go")); err != nil {
		t.Errorf("new.go missing on disk: %v", err)
	}
}

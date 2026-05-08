package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWalkProjectFilesSkipsHiddenAndVendor: build a small temp tree
// with a regular file, a .git dir (should be skipped), a vendor dir
// (should be skipped), and a regular subdir, and confirm the walker
// returns exactly the regular file plus anything in the regular subdir.
func TestWalkProjectFilesSkipsHiddenAndVendor(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "main.go"))
	mkfile(t, filepath.Join(root, ".git", "config"))
	mkfile(t, filepath.Join(root, "vendor", "lib.go"))
	mkfile(t, filepath.Join(root, "node_modules", "foo.js"))
	mkfile(t, filepath.Join(root, "internal", "util.go"))
	mkfile(t, filepath.Join(root, ".gitignore")) // hidden file at root

	got := walkProjectFiles(root)
	displays := displays(got)

	wantPresent := []string{"main.go", filepath.Join("internal", "util.go")}
	for _, w := range wantPresent {
		if !contains(displays, w) {
			t.Errorf("walkProjectFiles missing %q; got %v", w, displays)
		}
	}
	wantAbsent := []string{
		filepath.Join(".git", "config"),
		filepath.Join("vendor", "lib.go"),
		filepath.Join("node_modules", "foo.js"),
		".gitignore",
	}
	for _, w := range wantAbsent {
		if contains(displays, w) {
			t.Errorf("walkProjectFiles should have skipped %q; got %v", w, displays)
		}
	}
}

// TestWalkProjectFilesSkipsBinaryExts: image / archive / binary
// extensions clutter type-ahead in a code editor. The walker drops
// them so common queries like "main" don't have to compete with
// dozens of "main.png" / "main.zip" entries.
func TestWalkProjectFilesSkipsBinaryExts(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "icon.png"))
	mkfile(t, filepath.Join(root, "archive.zip"))
	mkfile(t, filepath.Join(root, "lib.dylib"))
	mkfile(t, filepath.Join(root, "doc.pdf"))
	mkfile(t, filepath.Join(root, "real.go"))
	mkfile(t, filepath.Join(root, "config.silkui")) // .silkui must NOT be skipped

	got := walkProjectFiles(root)
	displays := displays(got)

	if !contains(displays, "real.go") {
		t.Errorf("walkProjectFiles dropped real.go; got %v", displays)
	}
	if !contains(displays, "config.silkui") {
		t.Errorf("walkProjectFiles dropped .silkui; got %v", displays)
	}
	for _, bad := range []string{"icon.png", "archive.zip", "lib.dylib", "doc.pdf"} {
		if contains(displays, bad) {
			t.Errorf("walkProjectFiles should have skipped %q; got %v", bad, displays)
		}
	}
}

// TestWalkProjectFilesEmptyRootReturnsNil: empty root means "no
// project context" — caller should not see a popup with garbage.
func TestWalkProjectFilesEmptyRootReturnsNil(t *testing.T) {
	got := walkProjectFiles("")
	if got != nil {
		t.Errorf("expected nil for empty root, got %v", got)
	}
}

// TestWalkProjectFilesUnreadableSubtree: WalkDir's error path
// should not abort the whole walk. We can't easily make a dir
// truly unreadable in a test, but we can confirm walking a root
// that doesn't exist returns empty (not panic).
func TestWalkProjectFilesNonexistentRoot(t *testing.T) {
	got := walkProjectFiles(filepath.Join(t.TempDir(), "definitely-not-here"))
	if got != nil {
		t.Errorf("nonexistent root should yield nil, got %v", got)
	}
}

// TestFilterFilesSubsequence: the subsequence filter must match
// "gb" against "gui/button.go" (g→b in order) and reject "bg"
// (no b before g in any file's path).
func TestFilterFilesSubsequence(t *testing.T) {
	files := []fileEntry{
		{Display: "main.go"},
		{Display: filepath.Join("gui", "button.go")},
		{Display: filepath.Join("paint", "icon.go")},
		{Display: "README.md"},
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{
			"main.go",
			filepath.Join("gui", "button.go"),
			filepath.Join("paint", "icon.go"),
			"README.md",
		}},
		{"main", []string{"main.go"}},
		{"gb", []string{filepath.Join("gui", "button.go")}}, // g→b subseq
		{"go", []string{ // shortest first; among equal-length, input order
			"main.go",
			filepath.Join("gui", "button.go"),
			filepath.Join("paint", "icon.go"),
		}},
		{"xyz", nil},
	}
	for _, c := range cases {
		got := filterFiles(files, c.query)
		if len(got) != len(c.want) {
			t.Errorf("filterFiles(%q) returned %d, want %d:\n got %v\nwant %v",
				c.query, len(got), len(c.want), displays(got), c.want)
			continue
		}
		for i, g := range got {
			if g.Display != c.want[i] {
				t.Errorf("filterFiles(%q)[%d] = %q, want %q",
					c.query, i, g.Display, c.want[i])
			}
		}
	}
}

// TestFilterFilesShorterFirst: when several entries match, the
// shorter Display wins. Otherwise typing "main" in a deep tree
// would surface "internal/cmd/some/path/main.go" before "main.go".
func TestFilterFilesShorterFirst(t *testing.T) {
	files := []fileEntry{
		{Display: filepath.Join("a", "b", "c", "main.go")},
		{Display: "main.go"},
		{Display: filepath.Join("internal", "main.go")},
	}
	got := filterFiles(files, "main")
	if len(got) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(got))
	}
	if got[0].Display != "main.go" {
		t.Errorf("shortest match should rank first; got %q", got[0].Display)
	}
}

// TestFilterFilesCaseInsensitive: query and Display lower-case
// alignment. "BUTTON" and "button" should match the same files.
func TestFilterFilesCaseInsensitive(t *testing.T) {
	files := []fileEntry{{Display: filepath.Join("gui", "Button.go")}}
	for _, q := range []string{"button", "BUTTON", "BuTtOn"} {
		got := filterFiles(files, q)
		if len(got) != 1 {
			t.Errorf("filterFiles(%q) = %v, want 1 hit", q, displays(got))
		}
	}
}

// TestWalkProjectFilesPreservesAbsolutePath: openFileInEditor expects
// an absolute path that os.ReadFile can resolve. The walker must
// stamp Path with the absolute form from filepath.WalkDir, even
// when root itself is relative.
func TestWalkProjectFilesPreservesAbsolutePath(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "main.go"))
	got := walkProjectFiles(root)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if !strings.HasPrefix(got[0].Path, root) {
		t.Errorf("Path %q does not begin with root %q", got[0].Path, root)
	}
	if got[0].Display != "main.go" {
		t.Errorf("Display should be relative form 'main.go', got %q", got[0].Display)
	}
}

func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func displays(es []fileEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Display
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

package ged

import (
	"path/filepath"
	"testing"
)

// TestParseGitPorcelain feeds a realistic multi-line `git status
// --porcelain` sample (modified, untracked, staged-add, deleted, renamed,
// and a nested path) and checks the resulting map: keys are absolute
// paths rooted at the git toplevel, values are the collapsed M/A/?/D
// badge runes. No real git invocation is needed — parseGitPorcelain is a
// pure function.
func TestParseGitPorcelain(t *testing.T) {
	gitRoot := filepath.Join("/Users", "dev", "proj")

	// XY + space + path. Mirrors what `git status --porcelain` emits.
	output := "" +
		" M gui/file-explorer.go\n" + // worktree-modified
		"?? notes.txt\n" + // untracked
		"A  ged/new_feature.go\n" + // staged add
		" D old/removed.go\n" + // worktree delete
		"D  staged_delete.go\n" + // staged delete
		"MM both_modified.go\n" + // staged + worktree modified
		"R  ged/old_name.go -> ged/new_name.go\n" + // rename: key the new path
		"" // trailing blank line (real porcelain ends with \n)

	got := parseGitPorcelain(output, gitRoot)

	want := map[string]rune{
		filepath.Join(gitRoot, "gui/file-explorer.go"): 'M',
		filepath.Join(gitRoot, "notes.txt"):            '?',
		filepath.Join(gitRoot, "ged/new_feature.go"):   'A',
		filepath.Join(gitRoot, "old/removed.go"):       'D',
		filepath.Join(gitRoot, "staged_delete.go"):     'D',
		filepath.Join(gitRoot, "both_modified.go"):     'M',
		filepath.Join(gitRoot, "ged/new_name.go"):      'A', // staged rename -> new path
	}

	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for path, badge := range want {
		g, ok := got[path]
		if !ok {
			t.Errorf("missing key %q in result", path)
			continue
		}
		if g != badge {
			t.Errorf("path %q: got badge %q, want %q", path, g, badge)
		}
	}

	// The old rename path must NOT appear — only the new path is keyed.
	if _, ok := got[filepath.Join(gitRoot, "ged/old_name.go")]; ok {
		t.Errorf("rename old path should not be keyed")
	}
}

// TestParseGitPorcelainEmpty: empty output (a clean tree) yields an empty
// map, never a nil deref, and skips short/garbage lines.
func TestParseGitPorcelainEmpty(t *testing.T) {
	if got := parseGitPorcelain("", "/repo"); len(got) != 0 {
		t.Errorf("empty output: got %v, want empty map", got)
	}
	// A line shorter than the "XY path" minimum is ignored.
	if got := parseGitPorcelain("\nx\n", "/repo"); len(got) != 0 {
		t.Errorf("garbage lines: got %v, want empty map", got)
	}
}

// TestGitStatusFor checks the lookup: a populated map returns the badge,
// a missing path returns 0, and a nil map (no repo) returns 0 without
// panicking.
func TestGitStatusFor(t *testing.T) {
	this := &FileExplorer{}
	if got := this.gitStatusFor("/anything"); got != 0 {
		t.Errorf("nil map: got %q, want 0", got)
	}

	this.gitStatus = map[string]rune{"/repo/a.go": 'M'}
	if got := this.gitStatusFor("/repo/a.go"); got != 'M' {
		t.Errorf("present: got %q, want 'M'", got)
	}
	if got := this.gitStatusFor("/repo/missing.go"); got != 0 {
		t.Errorf("absent: got %q, want 0", got)
	}
}

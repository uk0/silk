package ged

import (
	"reflect"
	"testing"

	"silk/core"
	"silk/paint"
)

// sampleEntries is a small change set spanning the status shapes the
// panel renders: an unstaged modify, a staged add, an untracked file,
// and a rename.
func sampleEntries() []core.GitStatusEntry {
	return []core.GitStatusEntry{
		{Staged: ' ', Unstaged: 'M', Path: "gui/widget.go"},
		{Staged: 'A', Unstaged: ' ', Path: "ged/new-file.go"},
		{Staged: '?', Unstaged: '?', Path: "scratch.txt"},
		{Staged: 'R', Unstaged: ' ', Path: "core/git2.go", OrigPath: "core/git.go"},
	}
}

// TestGitChangesSetEntriesRoundTrip verifies SetEntries stores the rows
// and Entries() returns an equal — but independent — copy.
func TestGitChangesSetEntriesRoundTrip(t *testing.T) {
	p := NewGitChangesPanel()
	in := sampleEntries()
	p.SetEntries(in)

	got := p.Entries()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Entries() = %+v\nwant %+v", got, in)
	}

	// Mutating the returned copy must not disturb the panel's state.
	got[0].Path = "MUTATED"
	if p.Entries()[0].Path != "gui/widget.go" {
		t.Error("Entries() returned an aliasing slice, not a copy")
	}

	// Mutating the input slice after SetEntries must not disturb the
	// panel either — SetEntries copies on the way in.
	in[1].Path = "MUTATED"
	if p.Entries()[1].Path != "ged/new-file.go" {
		t.Error("SetEntries stored the caller's slice instead of a copy")
	}
}

// TestGitChangesClear verifies Clear empties the list.
func TestGitChangesClear(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetEntries(sampleEntries())
	p.Clear()
	if got := p.Entries(); len(got) != 0 {
		t.Fatalf("after Clear, Entries() = %+v, want empty", got)
	}
}

// TestStatusLetterPrecedence covers the documented precedence: untracked
// wins, else the unstaged (worktree) column, else the staged (index)
// column.
func TestStatusLetterPrecedence(t *testing.T) {
	cases := []struct {
		name  string
		entry core.GitStatusEntry
		want  string
	}{
		{"modified unstaged", core.GitStatusEntry{Staged: ' ', Unstaged: 'M'}, "M"},
		{"added staged, clean worktree", core.GitStatusEntry{Staged: 'A', Unstaged: ' '}, "A"},
		{"untracked", core.GitStatusEntry{Staged: '?', Unstaged: '?'}, "?"},
		{"renamed", core.GitStatusEntry{Staged: 'R', Unstaged: ' ', Path: "b", OrigPath: "a"}, "R"},
		// Both columns set: unstaged (worktree) wins over staged (index).
		{"staged-add + unstaged-modify -> unstaged wins", core.GitStatusEntry{Staged: 'A', Unstaged: 'M'}, "M"},
		// Both columns empty -> no glyph.
		{"empty", core.GitStatusEntry{Staged: ' ', Unstaged: ' '}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusLetter(c.entry); got != c.want {
				t.Errorf("statusLetter(%+v) = %q, want %q", c.entry, got, c.want)
			}
		})
	}
}

// TestStatusColor checks each known letter maps to a distinct accent and
// an unknown letter falls back to the neutral grey.
func TestStatusColor(t *testing.T) {
	known := map[string]paint.Color{
		"M": {R: 230, G: 180, B: 60, A: 255},
		"A": {R: 110, G: 200, B: 110, A: 255},
		"D": {R: 230, G: 80, B: 80, A: 255},
		"?": {R: 140, G: 140, B: 150, A: 255},
		"R": {R: 120, G: 170, B: 230, A: 255},
	}
	for letter, want := range known {
		if got := statusColor(letter); got != want {
			t.Errorf("statusColor(%q) = %+v, want %+v", letter, got, want)
		}
	}

	fallback := paint.Color{R: 200, G: 200, B: 210, A: 255}
	if got := statusColor("X"); got != fallback {
		t.Errorf("statusColor(default) = %+v, want %+v", got, fallback)
	}
}

// TestRowLabel covers a plain path and a rename's "orig -> path" arrow.
func TestRowLabel(t *testing.T) {
	plain := core.GitStatusEntry{Path: "gui/widget.go"}
	if got, want := rowLabel(plain), "gui/widget.go"; got != want {
		t.Errorf("rowLabel(plain) = %q, want %q", got, want)
	}

	rename := core.GitStatusEntry{Path: "core/git2.go", OrigPath: "core/git.go"}
	if got, want := rowLabel(rename), "core/git.go -> core/git2.go"; got != want {
		t.Errorf("rowLabel(rename) = %q, want %q", got, want)
	}
}

// TestRowAtY exercises the pure hit-test: in-range rows, the header band
// above the rows, and a coordinate past the last row.
func TestRowAtY(t *testing.T) {
	const top, rowH, count = 22, 20, 4

	// First row: y just below the header.
	if got := rowAtY(top, top, rowH, count); got != 0 {
		t.Errorf("rowAtY(top) = %d, want 0", got)
	}
	// Third row: top + 2*rowH lands inside row index 2.
	if got := rowAtY(top+2*rowH+5, top, rowH, count); got != 2 {
		t.Errorf("rowAtY(row 2) = %d, want 2", got)
	}
	// Header band: above the rows -> -1.
	if got := rowAtY(top-1, top, rowH, count); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	// Past the last row -> -1.
	if got := rowAtY(top+count*rowH, top, rowH, count); got != -1 {
		t.Errorf("rowAtY(past end) = %d, want -1", got)
	}
}

// TestGitChangesClickActivates drives the click path through the hit-test
// helper + the wired signal: a click on row 2 must fire SigFileActivated
// with that row's entry.
func TestGitChangesClickActivates(t *testing.T) {
	p := NewGitChangesPanel()
	in := sampleEntries()
	p.SetEntries(in)

	var got core.GitStatusEntry
	fired := false
	p.SigFileActivated(func(e core.GitStatusEntry) {
		fired = true
		got = e
	})

	// rowAt folds the scroll offset (0 here) and defers to rowAtY; pick a
	// y inside row index 2 and confirm the hit-test agrees before clicking.
	const top, rowH = int(gitChangesHeaderH), 20
	y := float64(top + 2*rowH + 3)
	if idx := p.rowAt(y); idx != 2 {
		t.Fatalf("rowAt(%v) = %d, want 2", y, idx)
	}

	p.OnLeftDown(5, y)
	if !fired {
		t.Fatal("SigFileActivated did not fire on click")
	}
	if !reflect.DeepEqual(got, in[2]) {
		t.Errorf("activated entry = %+v, want %+v", got, in[2])
	}
}

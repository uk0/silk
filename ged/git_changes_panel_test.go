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

// TestGitChangesStageToggle checks SetStaged / StagedPaths: a fresh panel
// has none staged, staging a path lists it, unstaging drops it.
func TestGitChangesStageToggle(t *testing.T) {
	p := NewGitChangesPanel()
	if got := p.StagedPaths(); len(got) != 0 {
		t.Fatalf("fresh panel StagedPaths() = %v, want empty", got)
	}

	p.SetStaged("a.go", true)
	if got, want := p.StagedPaths(), []string{"a.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after stage StagedPaths() = %v, want %v", got, want)
	}

	// Idempotent re-stage keeps a single entry.
	p.SetStaged("a.go", true)
	if got, want := p.StagedPaths(), []string{"a.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after re-stage StagedPaths() = %v, want %v", got, want)
	}

	p.SetStaged("a.go", false)
	if got := p.StagedPaths(); len(got) != 0 {
		t.Fatalf("after unstage StagedPaths() = %v, want empty", got)
	}
}

// TestGitChangesStagedPathsOrder verifies StagedPaths returns a stable
// lexical order regardless of stage order, so a commit's file set is
// deterministic.
func TestGitChangesStagedPathsOrder(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetStaged("c.go", true)
	p.SetStaged("a.go", true)
	p.SetStaged("b.go", true)
	if got, want := p.StagedPaths(), []string{"a.go", "b.go", "c.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("StagedPaths() = %v, want %v (lexical)", got, want)
	}
}

// TestGitChangesClearStaged checks ClearStaged unchecks everything.
func TestGitChangesClearStaged(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetStaged("a.go", true)
	p.SetStaged("b.go", true)
	p.ClearStaged()
	if got := p.StagedPaths(); len(got) != 0 {
		t.Errorf("after ClearStaged StagedPaths() = %v, want empty", got)
	}
}

// TestCheckboxHitX exercises the pure checkbox-vs-body column split: the
// drawn box column is a hit, the thin left margin and the path area are
// not — and crucially the x the activation test clicks (5) is NOT a
// checkbox hit, so that row-body click keeps working.
func TestCheckboxHitX(t *testing.T) {
	if !checkboxHitX(gitCheckboxX + 1) {
		t.Errorf("checkboxHitX(%v) = false, want true (inside box column)", gitCheckboxX+1)
	}
	if checkboxHitX(gitCheckboxX - 1) {
		t.Errorf("checkboxHitX(%v) = true, want false (left margin is row body)", gitCheckboxX-1)
	}
	if checkboxHitX(gitRowPathX + 10) {
		t.Errorf("checkboxHitX(%v) = true, want false (path area is row body)", gitRowPathX+10)
	}
	if checkboxHitX(5) {
		t.Error("checkboxHitX(5) = true; the existing SigFileActivated click would break")
	}
}

// TestGitChangesCheckboxClickTogglesRow drives the click path: a click in
// a row's checkbox column toggles that row's stage state (and does NOT
// activate the row), while a click on the row body activates it without
// changing stage state.
func TestGitChangesCheckboxClickTogglesRow(t *testing.T) {
	p := NewGitChangesPanel()
	in := sampleEntries()
	p.SetEntries(in)

	// Row index 2 is scratch.txt. Confirm the shared hit-test agrees before
	// clicking (unrealized widget: no commit band, rows run full height).
	const top, rowH = int(gitChangesHeaderH), 20
	y := float64(top + 2*rowH + 3)
	if idx := p.rowAt(y); idx != 2 {
		t.Fatalf("rowAt(%v) = %d, want 2", y, idx)
	}

	activated := false
	p.SigFileActivated(func(core.GitStatusEntry) { activated = true })

	// Checkbox-column click: toggles stage, no activation.
	p.OnLeftDown(gitCheckboxX+2, y)
	if activated {
		t.Error("checkbox click fired SigFileActivated, want checkbox-only")
	}
	if got, want := p.StagedPaths(), []string{"scratch.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after checkbox click StagedPaths() = %v, want %v", got, want)
	}

	// Second checkbox click toggles it back off.
	p.OnLeftDown(gitCheckboxX+2, y)
	if got := p.StagedPaths(); len(got) != 0 {
		t.Errorf("after second checkbox click StagedPaths() = %v, want empty", got)
	}

	// Row-body click (past the checkbox column): activates, no stage change.
	p.OnLeftDown(gitRowPathX+4, y)
	if !activated {
		t.Error("row-body click did not fire SigFileActivated")
	}
	if got := p.StagedPaths(); len(got) != 0 {
		t.Errorf("row-body click changed stage state: %v", got)
	}
}

// TestGitChangesSetEntriesPrunesStaged checks a re-push drops staged paths
// that are no longer present while keeping the ones that survive.
func TestGitChangesSetEntriesPrunesStaged(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetEntries(sampleEntries())
	p.SetStaged("gui/widget.go", true)
	p.SetStaged("scratch.txt", true)
	if got, want := p.StagedPaths(), []string{"gui/widget.go", "scratch.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StagedPaths() = %v, want %v", got, want)
	}

	// Re-push without gui/widget.go: it is pruned; scratch.txt survives.
	p.SetEntries([]core.GitStatusEntry{
		{Staged: '?', Unstaged: '?', Path: "scratch.txt"},
	})
	if got, want := p.StagedPaths(), []string{"scratch.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after prune StagedPaths() = %v, want %v", got, want)
	}

	// Minimal case from the spec: stage a path, re-push entries without it
	// -> empty (even though it was never in the entry list).
	p2 := NewGitChangesPanel()
	p2.SetStaged("a.go", true)
	p2.SetEntries([]core.GitStatusEntry{{Path: "other.go"}})
	if got := p2.StagedPaths(); len(got) != 0 {
		t.Errorf("staged path absent from new entries not pruned: %v", got)
	}
}

// TestGitChangesClearResetsStaged checks Clear drops the stage selection.
func TestGitChangesClearResetsStaged(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetEntries(sampleEntries())
	p.SetStaged("gui/widget.go", true)
	p.Clear()
	if got := p.StagedPaths(); len(got) != 0 {
		t.Errorf("after Clear StagedPaths() = %v, want empty", got)
	}
}

// TestGitChangesSubmitCommit checks the submit handler fires SigCommit with
// the trimmed message and the lexically ordered staged paths, then clears
// the message but leaves the stage set for the host to prune via SetEntries.
func TestGitChangesSubmitCommit(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetStaged("b.go", true)
	p.SetStaged("a.go", true)
	p.commitMsg = "  initial commit  " // trimmed on submit

	var gotMsg string
	var gotPaths []string
	fired := 0
	p.SigCommit(func(m string, paths []string) {
		fired++
		gotMsg = m
		gotPaths = paths
	})

	p.submitCommit()
	if fired != 1 {
		t.Fatalf("SigCommit fired %d times, want 1", fired)
	}
	if gotMsg != "initial commit" {
		t.Errorf("commit message = %q, want %q", gotMsg, "initial commit")
	}
	if want := []string{"a.go", "b.go"}; !reflect.DeepEqual(gotPaths, want) {
		t.Errorf("staged paths = %v, want %v (lexical)", gotPaths, want)
	}
	if p.commitMsg != "" {
		t.Errorf("commitMsg after submit = %q, want empty", p.commitMsg)
	}
	// The panel does not touch the stage set on submit — the host prunes it
	// through the post-commit SetEntries refresh.
	if got, want := p.StagedPaths(), []string{"a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("StagedPaths() after submit = %v, want %v", got, want)
	}
}

// TestGitChangesSubmitCommitNoop checks submit is inert when the message is
// blank OR nothing is staged — SigCommit must not fire in either case.
func TestGitChangesSubmitCommitNoop(t *testing.T) {
	// Whitespace-only message + a staged path -> no fire.
	p := NewGitChangesPanel()
	p.SetStaged("a.go", true)
	p.commitMsg = "   "
	fired := false
	p.SigCommit(func(string, []string) { fired = true })
	p.submitCommit()
	if fired {
		t.Error("SigCommit fired on empty message, want no-op")
	}

	// Non-empty message + zero staged -> no fire.
	p2 := NewGitChangesPanel()
	p2.commitMsg = "has a message"
	fired2 := false
	p2.SigCommit(func(string, []string) { fired2 = true })
	p2.submitCommit()
	if fired2 {
		t.Error("SigCommit fired with zero staged paths, want no-op")
	}
}

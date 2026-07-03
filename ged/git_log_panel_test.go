package ged

import (
	"testing"

	"github.com/uk0/silk/core"
)

// sampleCommits is a representative history: three commits with short
// hashes, subjects, authors and short dates. It exercises the round-trip,
// the label formatting and the hit-test geometry in one fixture.
func sampleCommits() []core.GitCommit {
	return []core.GitCommit{
		{Hash: "a1b2c3d", Subject: "fix build", Author: "alice", Date: "2026-07-01"},
		{Hash: "e4f5a6b", Subject: "add git log panel", Author: "bob", Date: "2026-06-30"},
		{Hash: "0c1d2e3", Subject: "polish welcome screen", Author: "carol", Date: "2026-06-29"},
	}
}

// TestGitLogPanelSetGetRoundTrip checks SetCommits followed by Commits()
// returns the same rows field-by-field.
func TestGitLogPanelSetGetRoundTrip(t *testing.T) {
	p := NewGitLogPanel()
	in := sampleCommits()
	p.SetCommits(in)

	got := p.Commits()
	if len(got) != len(in) {
		t.Fatalf("Commits() returned %d rows, want %d: %+v", len(got), len(in), got)
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("commit[%d] = %+v, want %+v", i, got[i], in[i])
		}
	}
}

// TestGitLogPanelCopySemantics verifies the panel keeps its own copy: a)
// mutating the input slice AFTER SetCommits does not change internal
// state, and b) mutating the slice returned by Commits() does not either.
func TestGitLogPanelCopySemantics(t *testing.T) {
	p := NewGitLogPanel()
	in := sampleCommits()
	p.SetCommits(in)

	// (a) Mutate the caller's slice after handing it in.
	in[0].Hash = "MUTATED"
	in[0].Subject = "MUTATED"
	if got := p.Commits(); got[0].Hash != "a1b2c3d" || got[0].Subject != "fix build" {
		t.Errorf("input mutation leaked into panel: row 0 = %+v", got[0])
	}

	// (b) Mutate the slice the panel handed back.
	out := p.Commits()
	out[1].Subject = "MUTATED"
	if got := p.Commits(); got[1].Subject != "add git log panel" {
		t.Errorf("returned-slice mutation leaked into panel: row 1 = %+v", got[1])
	}
}

// TestGitLogPanelClear verifies Clear empties the list.
func TestGitLogPanelClear(t *testing.T) {
	p := NewGitLogPanel()
	p.SetCommits(sampleCommits())
	p.Clear()
	if got := p.Commits(); len(got) != 0 {
		t.Errorf("Commits() after Clear = %d rows, want 0: %+v", len(got), got)
	}
}

// TestGitLogRowAtY exercises the pure hit-test helper directly: rows start
// at topOffset, rowH tall; header / out-of-range / degenerate inputs
// yield -1.
func TestGitLogRowAtY(t *testing.T) {
	const (
		top = 22.0
		rh  = 20.0
		n   = 3
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above rows (header)", 10, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 2", top + 2*rh + rh/2, 2},
		{"last pixel of row 2", top + 3*rh - 0.5, 2},
		{"just past last row", top + 3*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := logRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: logRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	// Degenerate row height must not divide by zero — return -1.
	if got := logRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("logRowAtY with rowH=0 = %d, want -1", got)
	}
	// Empty list: every y is out of range.
	if got := logRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("logRowAtY with count=0 = %d, want -1", got)
	}
}

// TestGitLogRowLabel checks the "<shorthash>  <subject>" formatting.
func TestGitLogRowLabel(t *testing.T) {
	cases := []struct {
		c    core.GitCommit
		want string
	}{
		{core.GitCommit{Hash: "a1b2c3d", Subject: "fix build"}, "a1b2c3d  fix build"},
		{core.GitCommit{Hash: "0c1d2e3", Subject: ""}, "0c1d2e3  "},
	}
	for _, tc := range cases {
		if got := logRowLabel(tc.c); got != tc.want {
			t.Errorf("logRowLabel(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

// TestGitLogTruncate checks the rune-based truncation helper: no-op when it
// fits, ellipsis when it must cut, empty for non-positive budgets, and no
// mid-character splitting of multi-byte runes.
func TestGitLogTruncate(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"}, // fits, untouched
		{"hello", 5, "hello"},  // exact fit, untouched
		{"hello", 4, "hel…"},   // cut: 3 kept + ellipsis
		{"hello", 1, "…"},      // budget of 1 is just the ellipsis
		{"hello", 0, ""},       // no room
		{"hello", -3, ""},      // negative budget
		{"历史记录表", 3, "历史…"},    // multi-byte, cut on a rune boundary
	}
	for _, tc := range cases {
		if got := logTruncate(tc.s, tc.max); got != tc.want {
			t.Errorf("logTruncate(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
		}
	}
}

// TestGitLogCountLabel checks the header tally text.
func TestGitLogCountLabel(t *testing.T) {
	if got, want := logCountLabel(3), "历史 / History (3)"; got != want {
		t.Errorf("logCountLabel(3) = %q, want %q", got, want)
	}
	if got, want := logCountLabel(0), "历史 / History (0)"; got != want {
		t.Errorf("logCountLabel(0) = %q, want %q", got, want)
	}
}

// TestGitLogPanelRowClickActivates drives a click on row 2 through the
// hit-test + signal path (no GL) and checks SigCommitActivated fires with
// the right commit.
//
// Geometry: sized 300x400, rows start at gitLogHeaderH=22 with
// rowHeight=20 and no scroll, so row 2 occupies y in [22+40, 22+60) =
// [62, 82); click the middle.
func TestGitLogPanelRowClickActivates(t *testing.T) {
	p := NewGitLogPanel()
	p.SetSize(300, 400)
	commits := sampleCommits()
	p.SetCommits(commits)

	var (
		got   core.GitCommit
		fired bool
	)
	p.SigCommitActivated(func(commit core.GitCommit) {
		got = commit
		fired = true
	})

	y := gitLogHeaderH + 2*p.rowHeight + p.rowHeight/2 // 72
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigCommitActivated")
	}
	if want := commits[2]; got != want {
		t.Errorf("SigCommitActivated = %+v, want %+v", got, want)
	}
}

// TestGitLogPanelHeaderClickNoop verifies a click in the header band does
// not fire the activation signal.
func TestGitLogPanelHeaderClickNoop(t *testing.T) {
	p := NewGitLogPanel()
	p.SetSize(300, 400)
	p.SetCommits(sampleCommits())

	fired := false
	p.SigCommitActivated(func(core.GitCommit) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown in header region fired SigCommitActivated")
	}
}

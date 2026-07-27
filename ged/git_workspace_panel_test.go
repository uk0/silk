package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// Tests for the source-control workspace: the pure grouping/collapse model
// (git-workspace-model.go), the GitWorkspacePanel interaction logic, and
// GitChangesPanel's index-driven mode, which consumes the same
// []GitFileState. Everything here is GL-free — no Frame, no Draw, no font
// measurement: the panels' geometry helpers are pure functions of a
// coordinate, and every click is driven through OnLeftDown.

// workspaceFixture is a change set spanning all four index buckets, pushed
// in a deliberately unsorted order so the model's per-group sort shows up.
func workspaceFixture() []GitFileState {
	return []GitFileState{
		{Entry: core.GitStatusEntry{Staged: ' ', Unstaged: 'M', Path: "gui/widget.go"}, State: GitUnstaged},
		{Entry: core.GitStatusEntry{Staged: 'A', Unstaged: ' ', Path: "ged/new-file.go"}, State: GitStaged},
		{Entry: core.GitStatusEntry{Staged: '?', Unstaged: '?', Path: "scratch.txt"}, State: GitUntracked},
		{Entry: core.GitStatusEntry{Staged: 'U', Unstaged: 'U', Path: "core/git.go"}, State: GitConflict},
		{Entry: core.GitStatusEntry{Staged: 'M', Unstaged: ' ', Path: "core/doc.go"}, State: GitStaged},
	}
}

// --- Model: classification ---

// TestClassifyGitEntry covers the documented precedence: unmerged states
// win, then untracked, then a real index column, else worktree-only.
func TestClassifyGitEntry(t *testing.T) {
	cases := []struct {
		name  string
		entry core.GitStatusEntry
		want  GitIndexState
	}{
		{"worktree modify", core.GitStatusEntry{Staged: ' ', Unstaged: 'M'}, GitUnstaged},
		{"index modify", core.GitStatusEntry{Staged: 'M', Unstaged: ' '}, GitStaged},
		{"index add", core.GitStatusEntry{Staged: 'A', Unstaged: ' '}, GitStaged},
		{"rename staged", core.GitStatusEntry{Staged: 'R', Unstaged: ' '}, GitStaged},
		{"partially staged", core.GitStatusEntry{Staged: 'M', Unstaged: 'M'}, GitStaged},
		{"untracked", core.GitStatusEntry{Staged: '?', Unstaged: '?'}, GitUntracked},
		{"both modified", core.GitStatusEntry{Staged: 'U', Unstaged: 'U'}, GitConflict},
		{"added by us", core.GitStatusEntry{Staged: 'A', Unstaged: 'U'}, GitConflict},
		{"deleted by them", core.GitStatusEntry{Staged: 'U', Unstaged: 'D'}, GitConflict},
		{"both added", core.GitStatusEntry{Staged: 'A', Unstaged: 'A'}, GitConflict},
		{"both deleted", core.GitStatusEntry{Staged: 'D', Unstaged: 'D'}, GitConflict},
		{"clean", core.GitStatusEntry{Staged: ' ', Unstaged: ' '}, GitUnstaged},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyGitEntry(c.entry); got != c.want {
				t.Errorf("ClassifyGitEntry(%+v) = %v, want %v", c.entry, got, c.want)
			}
		})
	}
}

// TestGitFileStatesFromEntries verifies the bulk mapping keeps input order
// and buckets each entry through ClassifyGitEntry.
func TestGitFileStatesFromEntries(t *testing.T) {
	in := []core.GitStatusEntry{
		{Staged: '?', Unstaged: '?', Path: "a.txt"},
		{Staged: 'M', Unstaged: ' ', Path: "b.go"},
		{Staged: 'U', Unstaged: 'U', Path: "c.go"},
	}
	got := GitFileStatesFromEntries(in)
	want := []GitFileState{
		{Entry: in[0], State: GitUntracked},
		{Entry: in[1], State: GitStaged},
		{Entry: in[2], State: GitConflict},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GitFileStatesFromEntries = %+v\nwant %+v", got, want)
	}
	if got := GitFileStatesFromEntries(nil); len(got) != 0 {
		t.Errorf("GitFileStatesFromEntries(nil) = %+v, want empty", got)
	}
}

// --- Model: grouping and collapse ---

// TestGitWorkspaceGroup verifies each bucket collects only its own files
// and sorts them by path regardless of the push order.
func TestGitWorkspaceGroup(t *testing.T) {
	m := NewGitWorkspaceModel()
	m.SetFiles(workspaceFixture())

	staged := m.Group(GitStaged)
	wantPaths := []string{"core/doc.go", "ged/new-file.go"}
	if len(staged) != len(wantPaths) {
		t.Fatalf("Group(staged) has %d files, want %d", len(staged), len(wantPaths))
	}
	for i, w := range wantPaths {
		if staged[i].Entry.Path != w {
			t.Errorf("Group(staged)[%d] = %q, want %q (lexical)", i, staged[i].Entry.Path, w)
		}
	}

	for _, c := range []struct {
		state GitIndexState
		want  int
	}{{GitConflict, 1}, {GitStaged, 2}, {GitUnstaged, 1}, {GitUntracked, 1}} {
		if got := m.Count(c.state); got != c.want {
			t.Errorf("Count(%v) = %d, want %d", c.state, got, c.want)
		}
	}
}

// TestGitWorkspaceRows pins the whole flattened layout: groups in
// gitGroupOrder (conflict, staged, unstaged, untracked), a header carrying
// its tally, then that group's files sorted by path.
func TestGitWorkspaceRows(t *testing.T) {
	fx := workspaceFixture()
	m := NewGitWorkspaceModel()
	m.SetFiles(fx)

	got := m.Rows()
	want := []GitWorkspaceRow{
		{Kind: GitWorkspaceGroupRow, State: GitConflict, Count: 1},
		{Kind: GitWorkspaceFileRow, State: GitConflict, File: fx[3]},
		{Kind: GitWorkspaceGroupRow, State: GitStaged, Count: 2},
		{Kind: GitWorkspaceFileRow, State: GitStaged, File: fx[4]},
		{Kind: GitWorkspaceFileRow, State: GitStaged, File: fx[1]},
		{Kind: GitWorkspaceGroupRow, State: GitUnstaged, Count: 1},
		{Kind: GitWorkspaceFileRow, State: GitUnstaged, File: fx[0]},
		{Kind: GitWorkspaceGroupRow, State: GitUntracked, Count: 1},
		{Kind: GitWorkspaceFileRow, State: GitUntracked, File: fx[2]},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows() = %+v\nwant %+v", got, want)
	}
}

// TestGitWorkspaceRowsSkipsEmptyGroups verifies a bucket with no files
// contributes no header at all — an empty model has no rows.
func TestGitWorkspaceRowsSkipsEmptyGroups(t *testing.T) {
	m := NewGitWorkspaceModel()
	if got := m.Rows(); len(got) != 0 {
		t.Fatalf("empty model Rows() = %+v, want none", got)
	}

	only := core.GitStatusEntry{Staged: '?', Unstaged: '?', Path: "scratch.txt"}
	m.SetFiles([]GitFileState{{Entry: only, State: GitUntracked}})
	got := m.Rows()
	want := []GitWorkspaceRow{
		{Kind: GitWorkspaceGroupRow, State: GitUntracked, Count: 1},
		{Kind: GitWorkspaceFileRow, State: GitUntracked, File: GitFileState{Entry: only, State: GitUntracked}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows() = %+v\nwant %+v (untracked group only)", got, want)
	}
}

// TestGitWorkspaceCollapse verifies a folded group keeps its header (marked
// collapsed) and drops its file rows, and that unfolding restores them.
func TestGitWorkspaceCollapse(t *testing.T) {
	m := NewGitWorkspaceModel()
	m.SetFiles(workspaceFixture())
	full := len(m.Rows())

	if m.Collapsed(GitStaged) {
		t.Fatal("groups should start expanded")
	}
	m.SetCollapsed(GitStaged, true)
	if !m.Collapsed(GitStaged) {
		t.Fatal("SetCollapsed(staged, true) did not take")
	}

	rows := m.Rows()
	if got, want := len(rows), full-2; got != want {
		t.Fatalf("collapsed staged: %d rows, want %d (2 file rows folded away)", got, want)
	}
	for _, r := range rows {
		if r.Kind == GitWorkspaceFileRow && r.State == GitStaged {
			t.Errorf("collapsed staged group still emits file row %q", r.File.Entry.Path)
		}
		if r.Kind == GitWorkspaceGroupRow && r.State == GitStaged {
			if !r.Collapsed {
				t.Error("staged header row Collapsed = false, want true")
			}
			if r.Count != 2 {
				t.Errorf("collapsed staged header Count = %d, want 2 (tally survives folding)", r.Count)
			}
		}
	}

	m.ToggleCollapsed(GitStaged)
	if m.Collapsed(GitStaged) {
		t.Fatal("ToggleCollapsed did not unfold the group")
	}
	if got := len(m.Rows()); got != full {
		t.Errorf("after unfold: %d rows, want %d", got, full)
	}
}

// TestGitWorkspaceSetFilesKeepsCollapse verifies a refresh (the host
// re-reading git after a stage) does not re-open folded groups, and that
// SetFiles / Files copy defensively.
func TestGitWorkspaceSetFilesKeepsCollapse(t *testing.T) {
	m := NewGitWorkspaceModel()
	in := workspaceFixture()
	m.SetFiles(in)
	m.SetCollapsed(GitUntracked, true)

	m.SetFiles(workspaceFixture())
	if !m.Collapsed(GitUntracked) {
		t.Error("SetFiles re-opened a folded group")
	}

	// Mutating the pushed slice must not disturb the model, and the copy
	// handed back must not alias it either.
	in[0].Entry.Path = "MUTATED"
	if got := m.Files()[0].Entry.Path; got != "gui/widget.go" {
		t.Errorf("SetFiles stored the caller's slice: path = %q", got)
	}
	out := m.Files()
	out[0].Entry.Path = "MUTATED"
	if got := m.Files()[0].Entry.Path; got != "gui/widget.go" {
		t.Errorf("Files() returned an aliasing slice: path = %q", got)
	}

	m.Clear()
	if got := m.Rows(); len(got) != 0 {
		t.Errorf("after Clear Rows() = %+v, want none", got)
	}
	if !m.Collapsed(GitUntracked) {
		t.Error("Clear dropped the fold state")
	}
}

// TestGitWorkspaceRowAt covers the indexed row accessor's bounds.
func TestGitWorkspaceRowAt(t *testing.T) {
	m := NewGitWorkspaceModel()
	m.SetFiles(workspaceFixture())

	row, ok := m.RowAt(1)
	if !ok {
		t.Fatal("RowAt(1) not ok")
	}
	if row.Kind != GitWorkspaceFileRow || row.File.Entry.Path != "core/git.go" {
		t.Errorf("RowAt(1) = %+v, want the conflict file row", row)
	}
	if _, ok := m.RowAt(-1); ok {
		t.Error("RowAt(-1) reported ok")
	}
	if _, ok := m.RowAt(len(m.Rows())); ok {
		t.Error("RowAt(past end) reported ok")
	}
}

// --- Model: branch bar formatting ---

// TestGitAheadBehindLabel covers the divergence marker in every direction,
// including in-sync (no marker) and nonsense negative counts.
func TestGitAheadBehindLabel(t *testing.T) {
	cases := []struct {
		ahead, behind int
		want          string
	}{
		{0, 0, ""},
		{2, 0, "↑2"},
		{0, 3, "↓3"},
		{2, 3, "↑2 ↓3"},
		{12, 7, "↑12 ↓7"},
		{-1, -4, ""},
	}
	for _, c := range cases {
		if got := gitAheadBehindLabel(c.ahead, c.behind); got != c.want {
			t.Errorf("gitAheadBehindLabel(%d, %d) = %q, want %q", c.ahead, c.behind, got, c.want)
		}
	}
}

// TestGitBranchBarText covers the branch bar label: bare branch, tracked
// branch with divergence, detached HEAD, and the empty fallback.
func TestGitBranchBarText(t *testing.T) {
	cases := []struct {
		name string
		info GitBranchInfo
		want string
	}{
		{"in sync, no upstream", GitBranchInfo{Name: "main"}, "main"},
		{"tracked, in sync", GitBranchInfo{Name: "main", Upstream: "origin/main"}, "main → origin/main"},
		{"ahead", GitBranchInfo{Name: "main", Upstream: "origin/main", Ahead: 2}, "main → origin/main ↑2"},
		{"diverged", GitBranchInfo{Name: "dev", Upstream: "origin/dev", Ahead: 2, Behind: 1}, "dev → origin/dev ↑2 ↓1"},
		{"detached", GitBranchInfo{Name: "a1b2c3d", Detached: true}, "a1b2c3d (detached)"},
		{"empty", GitBranchInfo{}, "(no branch)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gitBranchBarText(c.info); got != c.want {
				t.Errorf("gitBranchBarText(%+v) = %q, want %q", c.info, got, c.want)
			}
		})
	}
}

// TestGitGroupHeaderText verifies each bucket's header carries its own label
// plus the tally.
func TestGitGroupHeaderText(t *testing.T) {
	cases := []struct {
		state GitIndexState
		want  string
	}{
		{GitConflict, "冲突 / Conflicts (2)"},
		{GitStaged, "已暂存 / Staged (2)"},
		{GitUnstaged, "未暂存 / Changes (2)"},
		{GitUntracked, "未跟踪 / Untracked (2)"},
	}
	for _, c := range cases {
		if got := gitGroupHeaderText(c.state, 2); got != c.want {
			t.Errorf("gitGroupHeaderText(%v, 2) = %q, want %q", c.state, got, c.want)
		}
	}
	if got, want := GitConflict.String(), "conflict"; got != want {
		t.Errorf("GitConflict.String() = %q, want %q", got, want)
	}
}

// --- Panel geometry ---

// TestGitWorkspaceBarAt exercises the pure top-bar split: the branch bar,
// the two chip strips, and everything below them.
func TestGitWorkspaceBarAt(t *testing.T) {
	cases := []struct {
		y    float64
		want int
	}{
		{0, 0},
		{gitWsBarH - 1, 0},
		{gitWsBarH, 1},
		{2*gitWsBarH - 1, 1},
		{2 * gitWsBarH, 2},
		{gitWsListTop - 1, 2},
		{gitWsListTop, -1},
		{gitWsListTop + 40, -1},
		{-1, -1},
	}
	for _, c := range cases {
		if got := gitWsBarAt(c.y); got != c.want {
			t.Errorf("gitWsBarAt(%v) = %d, want %d", c.y, got, c.want)
		}
	}
}

// TestGitWorkspaceChipAt exercises the pure chip hit-test: inside each chip,
// in the gap between two, and left of / past the strip.
func TestGitWorkspaceChipAt(t *testing.T) {
	for i := 0; i < 3; i++ {
		x0, x1 := gitWsChipRect(i)
		if got := gitWsChipAt(x0, 3); got != i {
			t.Errorf("gitWsChipAt(chip %d left edge) = %d, want %d", i, got, i)
		}
		if got := gitWsChipAt(x1-0.5, 3); got != i {
			t.Errorf("gitWsChipAt(chip %d right edge) = %d, want %d", i, got, i)
		}
	}
	_, x1 := gitWsChipRect(0)
	if got := gitWsChipAt(x1+1, 3); got != -1 {
		t.Errorf("gitWsChipAt(gap) = %d, want -1", got)
	}
	if got := gitWsChipAt(0, 3); got != -1 {
		t.Errorf("gitWsChipAt(left of strip) = %d, want -1", got)
	}
	_, last := gitWsChipRect(2)
	if got := gitWsChipAt(last+10, 3); got != -1 {
		t.Errorf("gitWsChipAt(past strip) = %d, want -1", got)
	}
	// A 3-chip x must not resolve against a shorter strip.
	if got := gitWsChipAt(last-1, 2); got != -1 {
		t.Errorf("gitWsChipAt(chip 2, count 2) = %d, want -1", got)
	}
}

// TestGitWorkspaceMarkHitX verifies the left mark column (the conflict
// "resolve" affordance) stops before the status letter.
func TestGitWorkspaceMarkHitX(t *testing.T) {
	if !gitWsMarkHitX(gitWsGlyphX) {
		t.Errorf("gitWsMarkHitX(%v) = false, want true", gitWsGlyphX)
	}
	if gitWsMarkHitX(gitWsLetterX) {
		t.Errorf("gitWsMarkHitX(%v) = true, want false (status letter column)", gitWsLetterX)
	}
	if gitWsMarkHitX(gitWsTextX + 5) {
		t.Error("gitWsMarkHitX(path column) = true, want false")
	}
}

// wsRowY is the y coordinate inside flattened row idx of an unrealized
// panel, where the list runs from gitWsListTop to the bottom edge.
func wsRowY(p *GitWorkspacePanel, idx int) float64 {
	return gitWsListTop + float64(idx)*p.rowHeight + 3
}

// --- Panel interaction ---

// TestGitWorkspaceHeaderClickCollapses verifies a click on a group header
// row folds that group and drops its file rows from the rendered list.
func TestGitWorkspaceHeaderClickCollapses(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetFiles(workspaceFixture())
	full := len(p.Rows())

	// Row 2 is the 已暂存 header (row 0 conflict header, row 1 its file).
	if row, ok := p.model.RowAt(2); !ok || row.Kind != GitWorkspaceGroupRow || row.State != GitStaged {
		t.Fatalf("precondition: row 2 = %+v, want the staged group header", row)
	}
	p.OnLeftDown(5, wsRowY(p, 2))

	if !p.Collapsed(GitStaged) {
		t.Fatal("header click did not fold the staged group")
	}
	if got, want := len(p.Rows()), full-2; got != want {
		t.Errorf("after fold: %d rows, want %d", got, want)
	}

	// Clicking it again unfolds — the header stays at row 2 either way.
	p.OnLeftDown(5, wsRowY(p, 2))
	if p.Collapsed(GitStaged) {
		t.Error("second header click did not unfold the staged group")
	}
}

// TestGitWorkspaceFileRowActivates verifies a file row's body fires
// SigFileActivated with that file's path, and that a group header click
// does not.
func TestGitWorkspaceFileRowActivates(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetFiles(workspaceFixture())

	var got string
	fired := 0
	p.SigFileActivated(func(path string) {
		fired++
		got = path
	})

	// Row 6 is gui/widget.go, the sole 未暂存 file.
	p.OnLeftDown(gitWsTextX+4, wsRowY(p, 6))
	if fired != 1 {
		t.Fatalf("SigFileActivated fired %d times, want 1", fired)
	}
	if got != "gui/widget.go" {
		t.Errorf("activated path = %q, want %q", got, "gui/widget.go")
	}

	// The mark column of a non-conflict row still activates the row: only
	// conflicts carry the resolve affordance.
	p.OnLeftDown(gitWsGlyphX, wsRowY(p, 6))
	if fired != 2 {
		t.Errorf("mark-column click on a non-conflict row fired %d times total, want 2", fired)
	}

	// A header click is not an activation.
	fired = 0
	p.OnLeftDown(5, wsRowY(p, 5))
	if fired != 0 {
		t.Errorf("header click fired SigFileActivated %d times, want 0", fired)
	}
}

// TestGitWorkspaceResolveMark verifies a click in a conflict row's mark
// column fires SigResolve with the path instead of opening the file.
func TestGitWorkspaceResolveMark(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetFiles(workspaceFixture())

	var resolved string
	resolveFired := 0
	activated := 0
	p.SigResolve(func(path string) {
		resolveFired++
		resolved = path
	})
	p.SigFileActivated(func(string) { activated++ })

	// Row 1 is core/git.go, the sole 冲突 file.
	p.OnLeftDown(gitWsGlyphX, wsRowY(p, 1))
	if resolveFired != 1 {
		t.Fatalf("SigResolve fired %d times, want 1", resolveFired)
	}
	if resolved != "core/git.go" {
		t.Errorf("resolved path = %q, want %q", resolved, "core/git.go")
	}
	if activated != 0 {
		t.Errorf("mark-column click also fired SigFileActivated %d times, want 0", activated)
	}

	// The row body still opens the conflicted file.
	p.OnLeftDown(gitWsTextX+4, wsRowY(p, 1))
	if activated != 1 {
		t.Errorf("conflict row body fired SigFileActivated %d times, want 1", activated)
	}
	if resolveFired != 1 {
		t.Errorf("conflict row body fired SigResolve %d times, want 0 more", resolveFired-1)
	}
}

// TestGitWorkspaceActionChips verifies each chip fires exactly its own
// callback: fetch / pull / push on the first strip, stash / stash-pop on the
// second (its first chip opens the new-branch input, covered separately).
func TestGitWorkspaceActionChips(t *testing.T) {
	p := NewGitWorkspacePanel()

	counts := map[string]int{}
	p.SigFetch(func() { counts["fetch"]++ })
	p.SigPull(func() { counts["pull"]++ })
	p.SigPush(func() { counts["push"]++ })
	p.SigStash(func() { counts["stash"]++ })
	p.SigStashPop(func() { counts["pop"]++ })

	// Strip 1 sits in bar 1, strip 2 in bar 2.
	remoteY := gitWsBarH + gitWsBarH/2
	branchY := 2*gitWsBarH + gitWsBarH/2

	cases := []struct {
		name string
		x, y float64
		want string
	}{
		{"fetch", chipX(0), remoteY, "fetch"},
		{"pull", chipX(1), remoteY, "pull"},
		{"push", chipX(2), remoteY, "push"},
		{"stash", chipX(1), branchY, "stash"},
		{"stash pop", chipX(2), branchY, "pop"},
	}
	for _, c := range cases {
		before := counts[c.want]
		p.OnLeftDown(c.x, c.y)
		if got := counts[c.want]; got != before+1 {
			t.Errorf("%s chip: callback fired %d times, want %d", c.name, got, before+1)
		}
	}

	want := map[string]int{"fetch": 1, "pull": 1, "push": 1, "stash": 1, "pop": 1}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("chip callbacks = %v, want %v (no cross-talk)", counts, want)
	}

	// A click in the gap between chips is inert.
	_, x1 := gitWsChipRect(0)
	p.OnLeftDown(x1+1, remoteY)
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("chip-gap click fired something: %v", counts)
	}

	// Unregistered callbacks must not panic.
	bare := NewGitWorkspacePanel()
	bare.OnLeftDown(chipX(0), remoteY)
	bare.OnLeftDown(chipX(2), branchY)
}

// chipX is an x coordinate inside chip i of a left-aligned strip.
func chipX(i int) float64 {
	x0, _ := gitWsChipRect(i)
	return x0 + 2
}

// TestGitWorkspaceCheckoutInput verifies clicking the branch bar opens the
// inline input and Enter fires SigCheckout with the typed branch.
func TestGitWorkspaceCheckoutInput(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetBranch(GitBranchInfo{Name: "main", Upstream: "origin/main", Ahead: 1})

	var got string
	fired := 0
	p.SigCheckout(func(branch string) {
		fired++
		got = branch
	})

	// Typing before the input opens is ignored.
	p.OnTextInput("noise")
	if p.inputText != "" {
		t.Fatalf("text landed in a closed input: %q", p.inputText)
	}

	p.OnLeftDown(5, gitWsBarH/2) // branch bar
	if p.inputMode != gitWsInputCheckout {
		t.Fatalf("branch bar click left inputMode = %v, want checkout", p.inputMode)
	}

	p.OnTextInput("feature/x")
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnTextInput("y")
	p.OnKeyDown(gui.KeyEnter, false)

	if fired != 1 {
		t.Fatalf("SigCheckout fired %d times, want 1", fired)
	}
	if got != "feature/y" {
		t.Errorf("checkout branch = %q, want %q", got, "feature/y")
	}
	if p.inputMode != gitWsInputNone {
		t.Error("input stayed open after submit")
	}

	// A blank submit is a no-op and leaves the input open.
	p.OnLeftDown(5, gitWsBarH/2)
	p.OnTextInput("   ")
	p.OnKeyDown(gui.KeyEnter, false)
	if fired != 1 {
		t.Errorf("blank submit fired SigCheckout (%d times total)", fired)
	}
	if p.inputMode != gitWsInputCheckout {
		t.Error("blank submit closed the input")
	}

	// Esc cancels.
	p.OnKeyDown(gui.KeyEsc, false)
	if p.inputMode != gitWsInputNone || p.inputText != "" {
		t.Errorf("Esc left inputMode = %v, text = %q", p.inputMode, p.inputText)
	}
}

// TestGitWorkspaceCreateBranchInput verifies the [新建] chip opens the same
// inline input in create mode and Enter fires SigCreateBranch, not
// SigCheckout.
func TestGitWorkspaceCreateBranchInput(t *testing.T) {
	p := NewGitWorkspacePanel()

	var got string
	created := 0
	checkedOut := 0
	p.SigCreateBranch(func(name string) {
		created++
		got = name
	})
	p.SigCheckout(func(string) { checkedOut++ })

	p.OnLeftDown(chipX(0), 2*gitWsBarH+gitWsBarH/2)
	if p.inputMode != gitWsInputCreate {
		t.Fatalf("new-branch chip left inputMode = %v, want create", p.inputMode)
	}

	p.OnTextInput(" release/1.0 ")
	p.OnKeyDown(gui.KeyEnter, false)

	if created != 1 {
		t.Fatalf("SigCreateBranch fired %d times, want 1", created)
	}
	if got != "release/1.0" {
		t.Errorf("created branch = %q, want %q (trimmed)", got, "release/1.0")
	}
	if checkedOut != 0 {
		t.Errorf("create submit also fired SigCheckout %d times", checkedOut)
	}
}

// TestGitWorkspaceListClickClosesInput verifies a click below the bars blurs
// the inline input, so a stale name never submits later.
func TestGitWorkspaceListClickClosesInput(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetFiles(workspaceFixture())
	p.OnLeftDown(5, gitWsBarH/2)
	p.OnTextInput("wip")

	p.OnLeftDown(gitWsTextX+4, wsRowY(p, 6))
	if p.inputMode != gitWsInputNone || p.inputText != "" {
		t.Errorf("list click left inputMode = %v, text = %q", p.inputMode, p.inputText)
	}
}

// TestGitWorkspaceRemotes verifies the remotes round-trip, the band's row
// math, and that its header click folds it. The band is hidden on a short
// (or unrealized) widget, so the list keeps the whole area — the panel is
// given a size here to exercise the band's geometry.
func TestGitWorkspaceRemotes(t *testing.T) {
	p := NewGitWorkspacePanel()
	in := []GitRemote{{Name: "origin", URL: "git@example.com:a/b.git"}, {Name: "fork", URL: "https://example.com/c/d.git"}}
	p.SetRemotes(in)

	if got := p.Remotes(); !reflect.DeepEqual(got, in) {
		t.Fatalf("Remotes() = %+v, want %+v", got, in)
	}
	in[0].Name = "MUTATED"
	if got := p.Remotes()[0].Name; got != "origin" {
		t.Errorf("SetRemotes stored the caller's slice: name = %q", got)
	}

	if got, want := p.remotesBandRows(), 3; got != want {
		t.Errorf("remotesBandRows() = %d, want %d (header + 2 remotes)", got, want)
	}

	// Unrealized: no band, so the list owns the full height.
	if _, ok := p.remotesBand(); ok {
		t.Error("remotesBand shown on a zero-height widget")
	}
	if p.remotesHeaderAt(0) {
		t.Error("remotesHeaderAt true while the band is hidden")
	}

	p.SetSize(300, 400)
	top, ok := p.remotesBand()
	if !ok {
		t.Fatal("remotesBand hidden on a 400px-tall widget")
	}
	if want := 400 - p.remotesBandHeight(); top != want {
		t.Errorf("remotesBand top = %v, want %v", top, want)
	}
	if !p.remotesHeaderAt(top + 2) {
		t.Error("remotesHeaderAt false just below the band's separator")
	}

	// A click on the band's header folds it down to that header.
	p.OnLeftDown(5, top+2)
	if !p.remotesCollapsed {
		t.Fatal("header click did not fold the remotes band")
	}
	if got, want := p.remotesBandRows(), 1; got != want {
		t.Errorf("folded remotesBandRows() = %d, want %d", got, want)
	}

	// Rows inside the band never resolve to file rows.
	p.SetFiles(workspaceFixture())
	newTop, _ := p.remotesBand()
	if got := p.rowAt(newTop + 2); got != -1 {
		t.Errorf("rowAt(inside remotes band) = %d, want -1", got)
	}
}

// TestGitWorkspaceClear verifies Clear drops the file rows and closes the
// inline input while leaving the repository-level state alone.
func TestGitWorkspaceClear(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetFiles(workspaceFixture())
	p.SetBranch(GitBranchInfo{Name: "main"})
	p.SetRemotes([]GitRemote{{Name: "origin", URL: "u"}})
	p.OnLeftDown(5, gitWsBarH/2)
	p.OnTextInput("wip")

	p.Clear()
	if got := p.Rows(); len(got) != 0 {
		t.Errorf("after Clear Rows() = %+v, want none", got)
	}
	if p.inputMode != gitWsInputNone || p.inputText != "" {
		t.Errorf("Clear left inputMode = %v, text = %q", p.inputMode, p.inputText)
	}
	if p.Branch().Name != "main" {
		t.Error("Clear dropped the branch info")
	}
	if len(p.Remotes()) != 1 {
		t.Error("Clear dropped the remotes list")
	}
}

// TestGitWorkspaceSetEntriesClassifies verifies the porcelain convenience
// push buckets rows through ClassifyGitEntry.
func TestGitWorkspaceSetEntriesClassifies(t *testing.T) {
	p := NewGitWorkspacePanel()
	p.SetEntries([]core.GitStatusEntry{
		{Staged: 'U', Unstaged: 'U', Path: "core/git.go"},
		{Staged: 'M', Unstaged: ' ', Path: "a.go"},
		{Staged: ' ', Unstaged: 'M', Path: "b.go"},
		{Staged: '?', Unstaged: '?', Path: "c.txt"},
	})
	for _, c := range []struct {
		state GitIndexState
		want  int
	}{{GitConflict, 1}, {GitStaged, 1}, {GitUnstaged, 1}, {GitUntracked, 1}} {
		if got := p.model.Count(c.state); got != c.want {
			t.Errorf("Count(%v) = %d, want %d", c.state, got, c.want)
		}
	}
	if got, want := len(p.Rows()), 8; got != want {
		t.Errorf("Rows() = %d rows, want %d (4 headers + 4 files)", got, want)
	}
}

// TestGitWorkspaceFactoryRegistered verifies the panel is creatable through
// the factory and shows up as a tool view, so silkide can dock it.
func TestGitWorkspaceFactoryRegistered(t *testing.T) {
	const id = "ged.GitWorkspacePanel"
	if f := core.FindFactory(id); f == nil {
		t.Fatalf("factory %q is not registered", id)
	}
	if _, ok := core.New(id).(*GitWorkspacePanel); !ok {
		t.Fatalf("core.New(%q) did not yield a *GitWorkspacePanel", id)
	}
	def, ok := gui.GetToolViewDef(id)
	if !ok {
		t.Fatalf("tool view %q is not registered", id)
	}
	if def.Name == "" {
		t.Error("tool view registered with an empty Name")
	}
}

// --- GitChangesPanel: index-driven mode ---

// TestGitChangesSetFilesIndexState verifies the flat panel reads its
// checkbox state out of the index the host reported, not out of a local
// selection: only the staged files count as staged, whatever the user
// clicked before.
func TestGitChangesSetFilesIndexState(t *testing.T) {
	p := NewGitChangesPanel()

	// A stale local selection from a legacy push must not survive.
	p.SetEntries(sampleEntries())
	p.SetStaged("gui/widget.go", true)

	p.SetFiles(workspaceFixture())

	if got, want := p.StagedPaths(), []string{"core/doc.go", "ged/new-file.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StagedPaths() = %v, want %v (the real index)", got, want)
	}
	if got, want := p.stagedCount(), 2; got != want {
		t.Errorf("stagedCount() = %d, want %d", got, want)
	}
	cases := []struct {
		path string
		want GitIndexState
	}{
		{"core/git.go", GitConflict},
		{"core/doc.go", GitStaged},
		{"ged/new-file.go", GitStaged},
		{"gui/widget.go", GitUnstaged},
		{"scratch.txt", GitUntracked},
		{"never/pushed.go", GitUnstaged},
	}
	for _, c := range cases {
		if got := p.IndexState(c.path); got != c.want {
			t.Errorf("IndexState(%q) = %v, want %v", c.path, got, c.want)
		}
		if got, want := p.isStaged(c.path), c.want == GitStaged; got != want {
			t.Errorf("isStaged(%q) = %v, want %v", c.path, got, want)
		}
	}
	if got, want := len(p.Entries()), len(workspaceFixture()); got != want {
		t.Errorf("Entries() = %d rows, want %d", got, want)
	}
}

// TestGitChangesIndexModeCheckboxRequests verifies a checkbox click in
// index-driven mode asks the host to change the index — SigStage /
// SigUnstage carrying that row's path — and mutates nothing locally.
func TestGitChangesIndexModeCheckboxRequests(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetFiles(workspaceFixture())

	var staged, unstaged []string
	p.SigStage(func(path string) { staged = append(staged, path) })
	p.SigUnstage(func(path string) { unstaged = append(unstaged, path) })

	before := p.StagedPaths()

	// Row 0 is gui/widget.go (unstaged) -> a stage request.
	y0 := gitChangesHeaderH + 3
	if idx := p.rowAt(y0); idx != 0 {
		t.Fatalf("rowAt(%v) = %d, want 0", y0, idx)
	}
	p.OnLeftDown(gitCheckboxX+2, y0)

	// Row 1 is ged/new-file.go (staged) -> an unstage request.
	y1 := gitChangesHeaderH + p.rowHeight + 3
	p.OnLeftDown(gitCheckboxX+2, y1)

	if want := []string{"gui/widget.go"}; !reflect.DeepEqual(staged, want) {
		t.Errorf("SigStage paths = %v, want %v", staged, want)
	}
	if want := []string{"ged/new-file.go"}; !reflect.DeepEqual(unstaged, want) {
		t.Errorf("SigUnstage paths = %v, want %v", unstaged, want)
	}
	if got := p.StagedPaths(); !reflect.DeepEqual(got, before) {
		t.Errorf("checkbox click mutated local state: StagedPaths() = %v, want %v (unchanged until the host re-pushes)", got, before)
	}

	// The host re-reads git and pushes the new truth; only then does the
	// checkbox move.
	files := workspaceFixture()
	files[0].State = GitStaged // gui/widget.go is now in the index
	p.SetFiles(files)
	if !p.isStaged("gui/widget.go") {
		t.Error("after the host's refresh gui/widget.go still reads unstaged")
	}
}

// TestGitChangesIndexModeStageAll verifies the header affordances turn into
// per-file index requests: StageAll asks for the files git does not have
// staged yet, UnstageAll asks for the ones it does.
func TestGitChangesIndexModeStageAll(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetFiles(workspaceFixture())

	var staged, unstaged []string
	p.SigStage(func(path string) { staged = append(staged, path) })
	p.SigUnstage(func(path string) { unstaged = append(unstaged, path) })

	p.StageAll()
	want := []string{"gui/widget.go", "scratch.txt", "core/git.go"} // entry order, staged ones skipped
	if !reflect.DeepEqual(staged, want) {
		t.Errorf("StageAll requests = %v, want %v", staged, want)
	}
	if len(unstaged) != 0 {
		t.Errorf("StageAll fired SigUnstage: %v", unstaged)
	}

	staged = nil
	p.UnstageAll()
	if wantU := []string{"core/doc.go", "ged/new-file.go"}; !reflect.DeepEqual(unstaged, wantU) {
		t.Errorf("UnstageAll requests = %v, want %v (staged set, lexical)", unstaged, wantU)
	}
	if len(staged) != 0 {
		t.Errorf("UnstageAll fired SigStage: %v", staged)
	}

	// The local selection is never touched, so the panel still mirrors git.
	if got, want := p.StagedPaths(), []string{"core/doc.go", "ged/new-file.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("StagedPaths() = %v, want %v", got, want)
	}
}

// TestGitChangesLegacyModeUnaffected verifies a SetEntries push after a
// SetFiles push drops back to the local-selection model: checkbox clicks
// flip local state and fire no index request.
func TestGitChangesLegacyModeUnaffected(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetFiles(workspaceFixture())
	p.SetEntries(sampleEntries())

	requests := 0
	p.SigStage(func(string) { requests++ })
	p.SigUnstage(func(string) { requests++ })

	y := gitChangesHeaderH + 3 // row 0 = gui/widget.go
	p.OnLeftDown(gitCheckboxX+2, y)

	if requests != 0 {
		t.Errorf("legacy checkbox click fired %d index requests, want 0", requests)
	}
	if got, want := p.StagedPaths(), []string{"gui/widget.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("legacy StagedPaths() = %v, want %v", got, want)
	}
	if got := p.IndexState("gui/widget.go"); got != GitUnstaged {
		t.Errorf("legacy IndexState = %v, want %v (no index state reported)", got, GitUnstaged)
	}
}

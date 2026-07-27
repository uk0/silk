package ged

import (
	"sort"
	"strconv"

	"github.com/uk0/silk/core"
)

// This file is the pure, GL-free model behind GitWorkspacePanel (and the
// index-driven half of GitChangesPanel): where each changed file sits
// relative to the git index, how those files group under collapsible
// headers, and how the branch bar's divergence marker reads. No widget, no
// painter, no git — every function here is a data transform, so the
// grouping / collapse / formatting rules are directly unit-testable.

// GitIndexState is where a changed file sits relative to the git index —
// the fact a source-control view groups by.
//
// It is the *host's* reading of git, never a UI toggle: the host runs
// core.GitStatusPorcelain, buckets every entry (ClassifyGitEntry is the
// default rule) and pushes the result into the panels. The panels render
// that state and ask the host to change the index (SigStage / SigUnstage /
// SigResolve); the host runs git and pushes a fresh set back. Nothing in
// the UI guesses what the index now holds.
//
// GitUnstaged is the zero value, so a GitFileState built without an
// explicit state reads as a plain worktree change.
type GitIndexState int

const (
	GitUnstaged  GitIndexState = iota // worktree change that is not in the index
	GitStaged                         // change recorded in the index, ready to commit
	GitConflict                       // unmerged path (merge / rebase conflict)
	GitUntracked                      // path git does not track at all
)

// String returns the state's stable lowercase name ("staged", "unstaged",
// "conflict", "untracked"). Unknown values read as "unstaged", matching the
// zero value. Used in diagnostics and test failure messages.
func (s GitIndexState) String() string {
	switch s {
	case GitStaged:
		return "staged"
	case GitConflict:
		return "conflict"
	case GitUntracked:
		return "untracked"
	}
	return "unstaged"
}

// gitGroupOrder is the top-to-bottom order of the group headers: conflicts
// first because they block everything else, then the index, then the
// worktree, then untracked files last — the order VS Code's SCM view and
// Qt Creator's status output both use.
var gitGroupOrder = []GitIndexState{GitConflict, GitStaged, GitUnstaged, GitUntracked}

// gitGroupHeaderText is a group header's label plus its tally, e.g.
// "已暂存 / Staged (3)". Pure so the header text is testable without the
// renderer.
func gitGroupHeaderText(state GitIndexState, count int) string {
	var label string
	switch state {
	case GitConflict:
		label = "冲突 / Conflicts"
	case GitStaged:
		label = "已暂存 / Staged"
	case GitUntracked:
		label = "未跟踪 / Untracked"
	default:
		label = "未暂存 / Changes"
	}
	return label + " (" + strconv.Itoa(count) + ")"
}

// GitFileState is one changed file as the host reports it: the raw
// porcelain entry plus the index bucket it belongs to. Entry carries the
// path, the rename source and both status columns, so a row renderer can
// reuse statusLetter / statusColor / rowLabel unchanged.
type GitFileState struct {
	Entry core.GitStatusEntry
	State GitIndexState
}

// ClassifyGitEntry buckets a `git status --porcelain` entry into an index
// state. Precedence, highest first:
//
//  1. Conflict — either column is 'U', or the pair is "AA" / "DD": git's
//     unmerged states. A conflict outranks everything else; it has to be
//     resolved before the remaining state means anything.
//  2. Untracked — '?' in either column (git emits "??").
//  3. Staged — the X (index) column carries a real code. A partially staged
//     file ("MM": a staged edit plus a newer worktree edit) lands here,
//     because that is the change `git commit` would capture.
//  4. Unstaged — everything else, i.e. a worktree-only change.
//
// One bucket per file is the model: a host that wants a different split can
// build GitFileStates itself and push those, this is only the default
// translation.
func ClassifyGitEntry(entry core.GitStatusEntry) GitIndexState {
	if entry.Staged == 'U' || entry.Unstaged == 'U' ||
		(entry.Staged == 'A' && entry.Unstaged == 'A') ||
		(entry.Staged == 'D' && entry.Unstaged == 'D') {
		return GitConflict
	}
	if entry.Staged == '?' || entry.Unstaged == '?' {
		return GitUntracked
	}
	if isStatusCode(entry.Staged) {
		return GitStaged
	}
	return GitUnstaged
}

// GitFileStatesFromEntries maps porcelain entries through ClassifyGitEntry,
// preserving input order. The convenience path for a host that already has
// a []core.GitStatusEntry and wants the default bucketing.
func GitFileStatesFromEntries(entries []core.GitStatusEntry) []GitFileState {
	out := make([]GitFileState, len(entries))
	for i, e := range entries {
		out[i] = GitFileState{Entry: e, State: ClassifyGitEntry(e)}
	}
	return out
}

// GitBranchInfo is what the branch bar shows: the checked-out branch, its
// upstream, and how far the two have diverged. The host fills it in from
// core.GitCurrentBranch plus a divergence count (e.g. `git rev-list
// --left-right --count @{u}...HEAD`); the panel only formats it.
type GitBranchInfo struct {
	Name     string // current branch, or the short hash on a detached HEAD
	Upstream string // tracking branch, e.g. "origin/main"; "" when untracked
	Ahead    int    // commits on the branch the upstream lacks
	Behind   int    // commits on the upstream the branch lacks
	Detached bool   // HEAD is not on a branch
}

// GitRemote is one configured remote (`git remote -v`), listed in the
// panel's bottom band.
type GitRemote struct {
	Name string // e.g. "origin"
	URL  string // fetch URL
}

// gitAheadBehindLabel formats the divergence marker: "↑2 ↓3" when the
// branch has both unpushed and unpulled commits, "↑2" / "↓3" when only one
// side diverges, and "" when the branch is in sync (or the counts are
// missing / negative). Pure so the branch bar's formatting is testable
// without the renderer.
func gitAheadBehindLabel(ahead, behind int) string {
	switch {
	case ahead > 0 && behind > 0:
		return "↑" + strconv.Itoa(ahead) + " ↓" + strconv.Itoa(behind)
	case ahead > 0:
		return "↑" + strconv.Itoa(ahead)
	case behind > 0:
		return "↓" + strconv.Itoa(behind)
	}
	return ""
}

// gitBranchBarText is the branch bar's label: the branch name, then the
// upstream when one is configured, then the divergence marker when the two
// have diverged — e.g. "main → origin/main ↑2 ↓1". A detached HEAD is
// tagged as such, and an empty name reads "(no branch)" so the bar never
// renders blank.
func gitBranchBarText(info GitBranchInfo) string {
	out := info.Name
	if out == "" {
		out = "(no branch)"
	}
	if info.Detached {
		out += " (detached)"
	}
	if info.Upstream != "" {
		out += " → " + info.Upstream
	}
	if ab := gitAheadBehindLabel(info.Ahead, info.Behind); ab != "" {
		out += " " + ab
	}
	return out
}

// GitWorkspaceRowKind distinguishes the two kinds of line the grouped file
// list renders.
type GitWorkspaceRowKind int

const (
	GitWorkspaceGroupRow GitWorkspaceRowKind = iota // collapsible group header
	GitWorkspaceFileRow                             // a file inside an expanded group
)

// GitWorkspaceRow is one flattened line of the grouped file list. Splitting
// the row sequence out of the widget keeps the layout decision (which lines
// show, in what order) pure and testable — the same split packages-panel.go
// makes with packageRow.
type GitWorkspaceRow struct {
	Kind      GitWorkspaceRowKind
	State     GitIndexState // the group this row belongs to
	File      GitFileState  // file rows only; zero on a group row
	Count     int           // group rows only: files in the group
	Collapsed bool          // group rows only: whether the group is folded
}

// GitWorkspaceModel is the row model behind GitWorkspacePanel: the changed
// files the host pushed plus the per-group collapse state, flattened into
// GitWorkspaceRows on demand. It holds no widget and touches no git, so
// grouping and collapse can be tested directly.
//
// Zero value is usable: the collapse map is created on first write and read
// as "everything expanded" until then.
type GitWorkspaceModel struct {
	files     []GitFileState
	collapsed map[GitIndexState]bool
}

// NewGitWorkspaceModel returns an empty model with every group expanded.
func NewGitWorkspaceModel() *GitWorkspaceModel {
	return new(GitWorkspaceModel)
}

// SetFiles replaces the file set with a defensive copy, so a later host
// refresh cannot mutate rows out from under a paint. Collapse state is
// deliberately kept: a refresh after a stage must not re-open groups the
// user folded.
func (this *GitWorkspaceModel) SetFiles(files []GitFileState) {
	out := make([]GitFileState, len(files))
	copy(out, files)
	this.files = out
}

// Files returns a defensive copy of the file set in the order the host
// pushed it (ungrouped).
func (this *GitWorkspaceModel) Files() []GitFileState {
	out := make([]GitFileState, len(this.files))
	copy(out, this.files)
	return out
}

// Clear drops every file. Collapse state survives, for the same reason
// SetFiles keeps it.
func (this *GitWorkspaceModel) Clear() {
	this.files = nil
}

// Group returns the files bucketed into state, sorted by path so the group
// order is deterministic regardless of how git emitted them (a rename sorts
// by its target path, which is what the row shows first).
func (this *GitWorkspaceModel) Group(state GitIndexState) []GitFileState {
	out := make([]GitFileState, 0, len(this.files))
	for _, f := range this.files {
		if f.State == state {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Entry.Path < out[j].Entry.Path })
	return out
}

// Count is how many files sit in a group.
func (this *GitWorkspaceModel) Count(state GitIndexState) int {
	n := 0
	for _, f := range this.files {
		if f.State == state {
			n++
		}
	}
	return n
}

// Collapsed reports whether a group is folded. Safe on the zero model.
func (this *GitWorkspaceModel) Collapsed(state GitIndexState) bool {
	return this.collapsed[state]
}

// SetCollapsed folds (on) or unfolds (off) a group.
func (this *GitWorkspaceModel) SetCollapsed(state GitIndexState, on bool) {
	if this.collapsed == nil {
		this.collapsed = make(map[GitIndexState]bool, len(gitGroupOrder))
	}
	this.collapsed[state] = on
}

// ToggleCollapsed flips a group's fold state.
func (this *GitWorkspaceModel) ToggleCollapsed(state GitIndexState) {
	this.SetCollapsed(state, !this.Collapsed(state))
}

// Rows flattens the file set + collapse state into the line sequence the
// panel draws and hit-tests. Groups appear in gitGroupOrder; an empty group
// contributes nothing (no header for a bucket with no files); a folded
// group contributes only its header.
func (this *GitWorkspaceModel) Rows() []GitWorkspaceRow {
	rows := make([]GitWorkspaceRow, 0, len(this.files)+len(gitGroupOrder))
	for _, state := range gitGroupOrder {
		files := this.Group(state)
		if len(files) == 0 {
			continue
		}
		collapsed := this.Collapsed(state)
		rows = append(rows, GitWorkspaceRow{
			Kind:      GitWorkspaceGroupRow,
			State:     state,
			Count:     len(files),
			Collapsed: collapsed,
		})
		if collapsed {
			continue
		}
		for _, f := range files {
			rows = append(rows, GitWorkspaceRow{Kind: GitWorkspaceFileRow, State: state, File: f})
		}
	}
	return rows
}

// RowAt returns the row at idx, or ok=false when idx is out of range.
func (this *GitWorkspaceModel) RowAt(idx int) (row GitWorkspaceRow, ok bool) {
	rows := this.Rows()
	if idx < 0 || idx >= len(rows) {
		return GitWorkspaceRow{}, false
	}
	return rows[idx], true
}

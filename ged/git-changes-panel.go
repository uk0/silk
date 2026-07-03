package ged

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.GitChangesPanel", gui.TypeOf(GitChangesPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GitChangesPanel",
		Name: "更改 / Git Changes",
		Icon: "edit",
		Desc: "未提交改动列表（工作树相对索引的状态）",
	})
}

// GitChangesPanel is the IDE's "Source Control / Changes" pane: the list
// of files with uncommitted modifications, like the left-rail list in
// VS Code's SCM view or Qt Creator's "Git > status". It is a pure
// display/interaction widget — it never shells out to git itself. The
// host (silkide) drives core.GitStatusPorcelain(dir) and pushes the
// entries in via SetEntries; the panel renders them and emits several
// signals back:
//
//	SigFileActivated — a row's body was single-clicked. The host opens the
//	                   file in the editor.
//	SigFileDiff      — a row's body was double-clicked. The host shows the
//	                   file's diff.
//	SigCommit        — the user submitted a commit (Enter in the message
//	                   line or the Commit button) with a non-empty message
//	                   and at least one staged path. The host runs
//	                   core.GitStage on the paths, core.GitCommit with the
//	                   message, then re-pushes the refreshed status via
//	                   SetEntries. The panel never shells out to git itself.
//
// The framework has no native double-click event, so consecutive clicks
// on the same row are timed here, the same idiom file-explorer.go and
// debug-panel.go use. A double-click therefore fires SigFileActivated on
// the first click and SigFileDiff on the second — which mirrors VS Code,
// where a single click opens the file and a double click opens its diff.
//
// Each row carries a stage checkbox in its left gutter: clicking the
// checkbox column toggles the row's stage state (tracked in `staged`,
// keyed by entry.Path) without opening the file, while clicking the row
// body still activates it. The message line + Commit button live in a
// band at the bottom, a rolled text line in the same idiom as
// debug-panel.go's watch input (no embedded gui.Edit). The header carries a
// "Stage All" toggle that checks / clears every row's stage state, and an
// "Unstage" button that fires SigUnstageRequested over the checked set so the
// host can drop those paths from the git index (core.GitUnstage). The commit
// input takes Shift+Enter for a newline, so a message carries a subject and
// body; plain Enter still submits.
//
// v1 is a single flat list with a per-row status letter, deliberately
// not grouped: splitting the rows into "Staged" vs "Changes"
// (index-side vs worktree-side) sections is a follow-up that re-buckets
// the same data without changing the data-push contract.
type GitChangesPanel struct {
	gui.Widget

	entries   []core.GitStatusEntry
	scrollY   float64
	hoverIdx  int
	rowHeight float64

	// Per-row stage state: presence of a path (value true) means the user
	// has checked it for the next commit. Keyed by entry.Path and pruned by
	// SetEntries to the paths still present, so a committed / vanished file
	// never lingers checked.
	staged map[string]bool

	// Commit band at the bottom: a rolled message input line (mirroring
	// debug-panel.go's watch input) plus a Commit button. commitMsg is the
	// in-progress message; commitFocused is whether the input holds focus.
	commitMsg     string
	commitFocused bool

	// Double-click detection, mirroring file-explorer.go / debug-panel.go.
	lastClickIdx  int
	lastClickTime time.Time

	cbActivated  func(entry core.GitStatusEntry)
	cbDiff       func(entry core.GitStatusEntry)
	cbCommit     func(message string, stagedPaths []string)
	cbUnstageReq func(paths []string)
}

// NewGitChangesPanel creates an empty changes panel.
func NewGitChangesPanel() *GitChangesPanel {
	p := new(GitChangesPanel)
	p.Init(p)
	return p
}

func (this *GitChangesPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.lastClickIdx = -1
}

// SetEntries replaces the change list with a defensive copy of entries
// and resets the view. The host calls this after every
// core.GitStatusPorcelain refresh — the panel keeps its own copy rather
// than borrowing the host's slice so a later host refresh can't mutate
// rows out from under a paint.
func (this *GitChangesPanel) SetEntries(entries []core.GitStatusEntry) {
	out := make([]core.GitStatusEntry, len(entries))
	copy(out, entries)
	this.entries = out
	this.scrollY = 0
	this.hoverIdx = -1
	this.lastClickIdx = -1
	this.pruneStaged()
	this.Self().Update()
}

// pruneStaged drops staged paths that no longer appear in the current
// entry set. The host re-pushes entries via SetEntries after every
// refresh (including right after a commit); pruning here keeps a
// committed / vanished file from lingering as a checked stage entry.
func (this *GitChangesPanel) pruneStaged() {
	if len(this.staged) == 0 {
		return
	}
	present := make(map[string]bool, len(this.entries))
	for _, e := range this.entries {
		present[e.Path] = true
	}
	for p := range this.staged {
		if !present[p] {
			delete(this.staged, p)
		}
	}
}

// Entries returns a defensive copy of the change rows in arrival order.
// A copy keeps the host from mutating (or truncating) the panel's
// backing slice.
func (this *GitChangesPanel) Entries() []core.GitStatusEntry {
	out := make([]core.GitStatusEntry, len(this.entries))
	copy(out, this.entries)
	return out
}

// Clear empties the change list and resets the view. The host calls this
// when the working tree is clean or the repo context goes away.
func (this *GitChangesPanel) Clear() {
	this.entries = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.lastClickIdx = -1
	this.staged = nil
	this.Self().Update()
}

// SigFileActivated registers the callback fired when the user
// single-clicks a row. The host opens the file in the editor. The
// callback receives a copy of the entry so the host can hold onto it
// past a later Clear without aliasing the panel's slice.
func (this *GitChangesPanel) SigFileActivated(fn func(entry core.GitStatusEntry)) {
	this.cbActivated = fn
}

// SigFileDiff registers the callback fired when the user double-clicks a
// row. The host renders the file's diff (e.g. core.GitDiffFile +
// ParseUnifiedDiff).
func (this *GitChangesPanel) SigFileDiff(fn func(entry core.GitStatusEntry)) {
	this.cbDiff = fn
}

// SigCommit registers the callback fired when the user submits a commit —
// Enter in the message line or a click on the Commit button — with a
// non-empty (trimmed) message AND at least one staged path. The callback
// receives the message and a copy of the staged paths; the host stages
// them (core.GitStage), commits (core.GitCommit), and re-pushes the
// refreshed status via SetEntries. Submitting with an empty message or
// zero staged paths is a no-op and never fires.
func (this *GitChangesPanel) SigCommit(fn func(message string, stagedPaths []string)) {
	this.cbCommit = fn
}

// SigUnstageRequested registers the callback fired by the header "Unstage"
// button. It receives the currently-checked paths (StagedPaths order); the
// host moves them OUT of the git index (core.GitUnstage) and re-pushes the
// refreshed status via SetEntries. This is distinct from the local
// commit-selection toggles (StageAll / UnstageAll / SetStaged): those only
// flip the in-panel `staged` set, while this asks the host to touch the real
// git index. Fires only when at least one path is checked.
func (this *GitChangesPanel) SigUnstageRequested(fn func(paths []string)) {
	this.cbUnstageReq = fn
}

// requestUnstage fires SigUnstageRequested with the checked set. A no-op —
// nothing fires — when nothing is checked, mirroring submitCommit, so the host
// only ever gets a runnable request.
func (this *GitChangesPanel) requestUnstage() {
	paths := this.StagedPaths()
	if len(paths) == 0 {
		return
	}
	if this.cbUnstageReq != nil {
		this.cbUnstageReq(paths)
	}
}

// StagedPaths returns the paths the user has checked for staging, in
// lexical order (a copy the host can hold onto). The order is stable so a
// commit's file set is deterministic; git does not care about the order.
func (this *GitChangesPanel) StagedPaths() []string {
	out := make([]string, 0, len(this.staged))
	for p := range this.staged {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// SetStaged checks (on) or unchecks (off) a path for staging. Unchecking
// removes the key so StagedPaths stays a clean set. Host-callable so a
// host UI (or a test) can drive the checkboxes without a click. Repaints
// only when the state actually changes.
func (this *GitChangesPanel) SetStaged(path string, on bool) {
	if on {
		if this.staged[path] {
			return
		}
		if this.staged == nil {
			this.staged = make(map[string]bool)
		}
		this.staged[path] = true
	} else {
		if !this.staged[path] {
			return
		}
		delete(this.staged, path)
	}
	this.Self().Update()
}

// ClearStaged unchecks every path. The host calls this to reset the stage
// selection (e.g. after driving its own commit). A no-op when nothing is
// staged.
func (this *GitChangesPanel) ClearStaged() {
	if len(this.staged) == 0 {
		return
	}
	this.staged = nil
	this.Self().Update()
}

// StageAll checks every current entry's path in the local commit-selection
// set — the header "Stage All" affordance's on-state. Paths already checked
// stay checked; a no-op on an empty change list. Repaints once when anything
// changed.
func (this *GitChangesPanel) StageAll() {
	if len(this.entries) == 0 {
		return
	}
	if this.staged == nil {
		this.staged = make(map[string]bool, len(this.entries))
	}
	changed := false
	for _, e := range this.entries {
		if !this.staged[e.Path] {
			this.staged[e.Path] = true
			changed = true
		}
	}
	if changed {
		this.Self().Update()
	}
}

// UnstageAll unchecks every path, emptying the local commit-selection set —
// the header affordance's off-state and the counterpart to StageAll. It only
// touches the in-panel `staged` set (same effect as ClearStaged); dropping
// files from the git index is the host's job via SigUnstageRequested.
func (this *GitChangesPanel) UnstageAll() {
	this.ClearStaged()
}

// allStaged reports whether every current entry is checked. False on an empty
// list, so the header toggle reads "stage all" rather than "clear" when there
// is nothing to stage. Drives the toggle's direction and label.
func (this *GitChangesPanel) allStaged() bool {
	if len(this.entries) == 0 {
		return false
	}
	for _, e := range this.entries {
		if !this.staged[e.Path] {
			return false
		}
	}
	return true
}

// toggleStageAll is the header "Stage All" affordance's click action: stage
// every row when not all are checked, otherwise clear the selection.
func (this *GitChangesPanel) toggleStageAll() {
	if this.allStaged() {
		this.UnstageAll()
	} else {
		this.StageAll()
	}
}

// isStaged reports whether a path is currently checked for staging. Safe
// on a nil map.
func (this *GitChangesPanel) isStaged(path string) bool {
	return this.staged[path]
}

// toggleStagedAt flips the stage state of the entry at idx, keyed by its
// path. Out-of-range indices are ignored.
func (this *GitChangesPanel) toggleStagedAt(idx int) {
	if idx < 0 || idx >= len(this.entries) {
		return
	}
	path := this.entries[idx].Path
	this.SetStaged(path, !this.isStaged(path))
}

// statusLetter reduces an entry's two status columns to the single
// letter shown at the row's left gutter.
//
// Precedence, in order:
//  1. Untracked ('?') in either column wins — git emits "??" for an
//     untracked file, and "untracked" is the headline fact about it.
//  2. Otherwise prefer the Unstaged (worktree, Y) column when it carries
//     a real change — that is the state on disk the user is most likely
//     acting on (open / diff the working copy).
//  3. Otherwise fall back to the Staged (index, X) column — a file that
//     is staged with a clean worktree (e.g. "A " added, "M " modified
//     then fully staged) still belongs in the list.
//
// A "real change" means the column is neither a space nor a NUL — git
// uses a space for "unmodified in this column"; a zero byte only shows
// up on a malformed/short line GitStatusPorcelain already guards
// against, but we treat it as empty for safety. When both columns are
// empty the result is "" (no glyph).
func statusLetter(entry core.GitStatusEntry) string {
	if entry.Staged == '?' || entry.Unstaged == '?' {
		return "?"
	}
	if isStatusCode(entry.Unstaged) {
		return string(entry.Unstaged)
	}
	if isStatusCode(entry.Staged) {
		return string(entry.Staged)
	}
	return ""
}

// isStatusCode reports whether a porcelain status byte denotes a real
// change (not the " " / NUL "unmodified" markers).
func isStatusCode(b byte) bool {
	return b != ' ' && b != 0
}

// statusColor maps a status letter to its gutter colour, mirroring the
// VS Code / Qt Creator palette: M amber, A green, D red, ? gray,
// R blue. Unknown letters fall back to neutral grey.
func statusColor(letter string) paint.Color {
	switch letter {
	case "M":
		return paint.Color{R: 230, G: 180, B: 60, A: 255} // amber — modified
	case "A":
		return paint.Color{R: 110, G: 200, B: 110, A: 255} // green — added
	case "D":
		return paint.Color{R: 230, G: 80, B: 80, A: 255} // red — deleted
	case "?":
		return paint.Color{R: 140, G: 140, B: 150, A: 255} // gray — untracked
	case "R":
		return paint.Color{R: 120, G: 170, B: 230, A: 255} // blue — renamed
	}
	return paint.Color{R: 200, G: 200, B: 210, A: 255}
}

// rowLabel is the path text for a row. A plain change shows just the
// path; a rename shows "orig -> path" so the move is visible at a
// glance. Kept as a free function so the label rule is pure and testable
// without a widget or GL context.
func rowLabel(entry core.GitStatusEntry) string {
	if entry.OrigPath != "" {
		return entry.OrigPath + " -> " + entry.Path
	}
	return entry.Path
}

// rowAtY maps a y coordinate to a change-row index for a list whose rows
// start at topOffset with count rows of height rowH. The caller folds
// the scroll offset into y before calling. Returns -1 when y lands above
// the rows or past the last row. Pure so the hit-test is testable
// without a live widget.
func rowAtY(y, topOffset, rowH, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := (y - topOffset) / rowH
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// checkboxHitX reports whether an x coordinate lands on a row's stage
// checkbox — the drawn box bounds in the left gutter, spanning
// [gitCheckboxX, gitRowLetterX) (up to where the status letter begins).
// The thin margin left of the box (x < gitCheckboxX) belongs to the row
// body, so a click there activates the row rather than toggling the box.
// Pure so the checkbox-vs-body disambiguation is testable without a live
// widget.
func checkboxHitX(x float64) bool {
	return x >= gitCheckboxX && x < gitRowLetterX
}

// --- Drawing ---

const gitChangesHeaderH = 22.0

// Row gutter geometry. The stage checkbox sits at the far left; the
// status letter and path text are shifted right to make room for it.
const (
	gitCheckboxX  = 6.0  // checkbox box left inset
	gitCheckboxSz = 13.0 // checkbox box side length
	gitRowLetterX = 26.0 // status-letter x (right of the checkbox column)
	gitRowPathX   = 42.0 // path-text x
)

// Header action-button geometry. The two buttons are right-aligned in the
// header band as [Stage All][gap][Unstage], ending gitHeaderPad from the
// right edge. Widths are fixed so the hit-test is a pure function of the
// widget width (mirroring checkboxHitX / rowAtY) — no font measurement off
// the paint path.
const (
	gitHeaderPad    = 6.0  // right-edge inset
	gitHeaderBtnGap = 6.0  // gap between the two header buttons
	gitStageAllBtnW = 50.0 // "Stage All" toggle width
	gitUnstageBtnW  = 50.0 // "Unstage" button width
)

// headerUnstageRect returns the [x0, x1) span of the header "Unstage" button
// for a widget of width w.
func headerUnstageRect(w float64) (x0, x1 float64) {
	x1 = w - gitHeaderPad
	x0 = x1 - gitUnstageBtnW
	return
}

// headerStageAllRect returns the [x0, x1) span of the header "Stage All"
// toggle, sitting just left of the Unstage button.
func headerStageAllRect(w float64) (x0, x1 float64) {
	ux0, _ := headerUnstageRect(w)
	x1 = ux0 - gitHeaderBtnGap
	x0 = x1 - gitStageAllBtnW
	return
}

// Draw renders a count header followed by one row per change: a status
// letter in its accent colour, then the path with the basename
// emphasised and the directory dimmed.
func (this *GitChangesPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (log/problems/debug).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header: "更改 / Changes (N)".
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, gitChangesHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, "更改 / Changes ("+strconv.Itoa(len(this.entries))+")")

	// Header action buttons on the right (Stage All toggle + Unstage).
	this.drawHeaderButtons(g, font, w)

	if len(this.entries) > 0 {
		rh := this.rowHeight
		areaTop := gitChangesHeaderH
		// Rows stop at the commit band when the widget is tall enough to show
		// one; otherwise they run to the bottom edge.
		rowsBottom := h
		if top, ok := this.commitBand(); ok {
			rowsBottom = top
		}
		startIdx := int(this.scrollY / rh)
		if startIdx < 0 {
			startIdx = 0
		}
		visibleCount := int((rowsBottom-areaTop)/rh) + 2

		for i := startIdx; i < startIdx+visibleCount && i < len(this.entries); i++ {
			y := areaTop + float64(i)*rh - this.scrollY
			if y >= rowsBottom {
				break
			}
			e := this.entries[i]

			// Alternating row tint; hover wins over the stripe.
			if i == this.hoverIdx {
				g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
				g.Rectangle(0, y, w, rh)
				g.Fill()
			} else if i%2 == 1 {
				g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
				g.Rectangle(0, y, w, rh)
				g.Fill()
			}

			// Stage checkbox in the left gutter.
			this.drawCheckbox(g, gitCheckboxX, y+(rh-gitCheckboxSz)/2, this.isStaged(e.Path))

			// Status letter in its accent colour, right of the checkbox.
			letter := statusLetter(e)
			g.SetBrush1(statusColor(letter))
			g.DrawText1(gitRowLetterX, y+fe.Ascent+2, letter)

			// Path: dimmed directory + emphasised basename. For a rename the
			// whole "orig -> path" string is the label and the basename split
			// operates on its last path element.
			x := gitRowPathX
			dir, base := splitPathLabel(rowLabel(e))
			if dir != "" {
				g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
				g.DrawText1(x, y+fe.Ascent+2, dir)
				x += font.TextExtents(dir).Width
			}
			g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, base)
		}
	}

	// Commit band at the bottom, painted last so it sits over the row-area
	// edge. Shown whenever the widget is tall enough — even with no changes.
	if top, ok := this.commitBand(); ok {
		this.drawCommitArea(g, font, w, top, h-top)
	}
}

// drawCheckbox paints a stage checkbox at (x, y): a stroked empty box when
// unchecked, or an accent-filled box with a light check mark when checked.
func (this *GitChangesPanel) drawCheckbox(g paint.Painter, x, y float64, checked bool) {
	if checked {
		g.SetBrush1(paint.Color{R: 90, G: 140, B: 210, A: 255})
		g.Rectangle(x, y, gitCheckboxSz, gitCheckboxSz)
		g.Fill()
		g.SetPen1(paint.Color{R: 235, G: 240, B: 250, A: 255}, 1.6)
		g.MoveTo(x+2.5, y+gitCheckboxSz*0.55)
		g.LineTo(x+gitCheckboxSz*0.42, y+gitCheckboxSz-3)
		g.LineTo(x+gitCheckboxSz-2, y+3)
		g.Stroke()
	} else {
		g.SetPen1(paint.Color{R: 120, G: 130, B: 145, A: 255}, 1)
		g.Rectangle(x, y, gitCheckboxSz, gitCheckboxSz)
		g.Stroke()
	}
}

// drawHeaderButtons paints the two right-aligned header affordances: a "Stage
// All" toggle whose label reflects whether a click will stage every row or
// clear the selection, and an "Unstage" button that asks the host to drop the
// checked set from the git index. Each dims when it has nothing to act on.
func (this *GitChangesPanel) drawHeaderButtons(g paint.Painter, font paint.Font, w float64) {
	sx0, sx1 := headerStageAllRect(w)
	stageLabel := "全选"
	if this.allStaged() {
		stageLabel = "清空"
	}
	this.drawHeaderButton(g, font, sx0, sx1, stageLabel, len(this.entries) > 0,
		paint.Color{R: 70, G: 100, B: 150, A: 255})

	ux0, ux1 := headerUnstageRect(w)
	this.drawHeaderButton(g, font, ux0, ux1, "撤出", len(this.staged) > 0,
		paint.Color{R: 135, G: 100, B: 60, A: 255})
}

// drawHeaderButton paints one header chip in [x0, x1): an accent (or muted,
// when disabled) fill with a centred label, matching the Commit button's
// fill-plus-centred-label idiom.
func (this *GitChangesPanel) drawHeaderButton(g paint.Painter, font paint.Font, x0, x1 float64, label string, enabled bool, accent paint.Color) {
	fe := font.FontExtents()
	const m = 3.0 // vertical inset within the header band
	if enabled {
		g.SetBrush1(accent)
	} else {
		g.SetBrush1(paint.Color{R: 40, G: 44, B: 52, A: 255})
	}
	g.Rectangle(x0, m, x1-x0, gitChangesHeaderH-2*m)
	g.Fill()
	if enabled {
		g.SetBrush1(paint.Color{R: 225, G: 232, B: 240, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 120, G: 128, B: 138, A: 255})
	}
	lw := font.TextExtents(label).Width
	g.DrawText1(x0+(x1-x0-lw)/2, fe.Ascent+4, label)
}

// drawCommitArea paints the bottom commit band at y=top: a hairline
// separator, the message input line (a caret + typed text when focused, a
// dim prompt when empty), then the Commit button, tinted green and
// showing the staged count when a commit is submittable and muted grey
// otherwise.
func (this *GitChangesPanel) drawCommitArea(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()
	rh := this.rowHeight

	// Hairline separator above the band.
	g.SetBrush1(paint.Color{R: 45, G: 45, B: 54, A: 255})
	g.Rectangle(0, top, w, 1)
	g.Fill()

	// Message input: one or more rows (Shift+Enter appends a body line).
	inputY := top + 1
	inputH := rh * float64(this.commitInputRows())
	if this.commitFocused {
		g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 30, G: 30, B: 36, A: 255})
	}
	g.Rectangle(0, inputY, w, inputH)
	g.Fill()
	if this.commitMsg == "" && !this.commitFocused {
		g.SetBrush1(paint.Color{R: 110, G: 120, B: 135, A: 255})
		g.DrawText1(8, inputY+fe.Ascent+2, "提交信息 / commit message")
	} else {
		lines := strings.Split(this.commitMsg, "\n")
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		for i, ln := range lines {
			g.DrawText1(8, inputY+float64(i)*rh+fe.Ascent+2, ln)
		}
		if this.commitFocused {
			last := lines[len(lines)-1]
			cx := 8 + font.TextExtents(last).Width + 1
			cy := inputY + float64(len(lines)-1)*rh
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, cy+3, 1.5, rh-6)
			g.Fill()
		}
	}

	// Commit button below the (possibly multi-row) input.
	btnY := inputY + inputH
	n := len(this.staged)
	enabled := n > 0 && strings.TrimSpace(this.commitMsg) != ""
	if enabled {
		g.SetBrush1(paint.Color{R: 45, G: 110, B: 70, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 40, G: 44, B: 52, A: 255})
	}
	g.Rectangle(0, btnY, w, rh)
	g.Fill()
	if enabled {
		g.SetBrush1(paint.Color{R: 220, G: 235, B: 225, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 120, G: 128, B: 138, A: 255})
	}
	label := "提交 / Commit (" + strconv.Itoa(n) + ")"
	lw := font.TextExtents(label).Width
	g.DrawText1((w-lw)/2, btnY+fe.Ascent+2, label)
}

// splitPathLabel splits a row label into a dimmed leading directory part
// (including the trailing slash) and the emphasised basename. The split
// is on the last "/" of the label so a rename's "a/b -> c/d" keeps
// "c/d"'s directory dimmed and "d" emphasised. A label with no slash is
// all basename.
func splitPathLabel(label string) (dir, base string) {
	return filepath.Split(label)
}

// --- Commit band geometry ---

// commitInputRows is the number of text rows the commit message input
// occupies — one per line of commitMsg (a blank message still gets one row).
// Lets the input grow as Shift+Enter appends body lines.
func (this *GitChangesPanel) commitInputRows() int {
	return strings.Count(this.commitMsg, "\n") + 1
}

// commitBandHeight is the pixel height reserved at the bottom for the commit
// area: the message input (commitInputRows rows) plus the Commit button (one
// row) and a 1px separator on top. It grows as the message gains body lines.
func (this *GitChangesPanel) commitBandHeight() float64 {
	return this.rowHeight*float64(this.commitInputRows()+1) + 1
}

// commitBand returns the y where the commit band starts and whether it is
// shown. It is shown only when the widget is tall enough to leave the
// header (and some rows) above it; on a zero-sized / very short widget it
// is hidden so the row list keeps the whole area.
func (this *GitChangesPanel) commitBand() (top float64, ok bool) {
	_, h := this.Size()
	top = h - this.commitBandHeight()
	ok = top >= gitChangesHeaderH
	return
}

// commitInputAt reports whether y lands on the commit message input line
// (the row just under the separator).
func (this *GitChangesPanel) commitInputAt(y float64) bool {
	top, ok := this.commitBand()
	if !ok {
		return false
	}
	inputY := top + 1
	return y >= inputY && y < inputY+this.rowHeight*float64(this.commitInputRows())
}

// commitButtonAt reports whether y lands on the Commit button (the row
// below the message input line).
func (this *GitChangesPanel) commitButtonAt(y float64) bool {
	top, ok := this.commitBand()
	if !ok {
		return false
	}
	btnY := top + 1 + this.rowHeight*float64(this.commitInputRows())
	return y >= btnY && y < btnY+this.rowHeight
}

// focusCommit sets whether the commit message input holds focus,
// repainting on a change so the caret / placeholder swap.
func (this *GitChangesPanel) focusCommit(on bool) {
	if this.commitFocused == on {
		return
	}
	this.commitFocused = on
	this.Self().Update()
}

// submitCommit fires SigCommit with the trimmed message and the staged
// paths, then clears the message. It is a no-op — nothing fires — when the
// message is blank OR no path is staged, so the host only ever gets a
// runnable commit. Like the rest of the panel it does not touch git or the
// entry list itself: the host stages + commits and re-pushes the refreshed
// status via SetEntries (which prunes the now-committed paths from
// `staged`).
func (this *GitChangesPanel) submitCommit() {
	msg := strings.TrimSpace(this.commitMsg)
	if msg == "" {
		return
	}
	paths := this.StagedPaths()
	if len(paths) == 0 {
		return
	}
	this.commitMsg = ""
	this.Self().Update()
	if this.cbCommit != nil {
		this.cbCommit(msg, paths)
	}
}

// onCommitEnter handles the Enter key while the commit input is focused:
// Shift+Enter appends a newline so the message can grow a body, plain Enter
// submits. Split out of OnKeyDown because the shift-gated branch cannot be
// exercised through OnKeyDown headlessly — gui.IsKeyDown polls live window
// state — so tests drive this helper with an explicit shift flag.
func (this *GitChangesPanel) onCommitEnter(shift bool) {
	if shift {
		this.commitMsg += "\n"
		this.Self().Update()
		return
	}
	this.submitCommit()
}

// --- Events ---

// OnLeftDown routes a click. The bottom commit band takes it first: the
// message line focuses the input, the Commit button submits. Otherwise a
// click in a row's left checkbox column toggles that row's stage state,
// while a click on the row body fires SigFileActivated — and a quick
// second body-click on the same row fires SigFileDiff, the same
// double-click idiom as file-explorer.go / debug-panel.go.
func (this *GitChangesPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	// Commit band at the bottom takes clicks first.
	if this.commitInputAt(y) {
		this.focusCommit(true)
		return
	}
	if this.commitButtonAt(y) {
		this.focusCommit(false)
		this.submitCommit()
		return
	}
	// Any other click blurs the message input.
	this.focusCommit(false)

	// Header action buttons (Stage All toggle, Unstage) take clicks in the
	// header band.
	if y < gitChangesHeaderH {
		w, _ := this.Size()
		if x0, x1 := headerStageAllRect(w); x >= x0 && x < x1 {
			this.toggleStageAll()
		} else if x0, x1 := headerUnstageRect(w); x >= x0 && x < x1 {
			this.requestUnstage()
		}
		return
	}

	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.entries) {
		return
	}

	// A click in the left checkbox column toggles the row's stage state
	// instead of activating the row body.
	if checkboxHitX(x) {
		this.toggleStagedAt(idx)
		return
	}

	now := time.Now()
	if idx == this.lastClickIdx && now.Sub(this.lastClickTime) < 400*time.Millisecond {
		this.lastClickTime = time.Time{} // reset to avoid triple-click
		if this.cbDiff != nil {
			this.cbDiff(this.entries[idx])
		}
		return
	}
	this.lastClickTime = now
	this.lastClickIdx = idx

	if this.cbActivated != nil {
		this.cbActivated(this.entries[idx])
	}
}

// OnKeyDown drives the commit message input while it holds focus: Enter
// submits the commit, Esc unfocuses, Backspace deletes a rune. Keys are
// ignored when the input is unfocused (the panel has no other key
// handling).
func (this *GitChangesPanel) OnKeyDown(key int, repeat bool) {
	if !this.commitFocused {
		return
	}
	switch key {
	case gui.KeyEnter:
		// Shift+Enter inserts a newline (multi-line body); plain Enter submits.
		this.onCommitEnter(gui.IsKeyDown(gui.KeyShift))
	case gui.KeyEsc:
		this.focusCommit(false)
	case gui.KeyBackSpace:
		if r := []rune(this.commitMsg); len(r) > 0 {
			this.commitMsg = string(r[:len(r)-1])
			this.Self().Update()
		}
	}
}

// OnTextInput feeds typed characters into the commit message input while
// it holds focus. Enter / Backspace arrive via OnKeyDown, not here; when
// the input is unfocused, typing is ignored.
func (this *GitChangesPanel) OnTextInput(s string) {
	if !this.commitFocused {
		return
	}
	if s == "\r" || s == "\n" {
		return
	}
	this.commitMsg += s
	this.Self().Update()
}

// OnMouseMove tracks hover state for the row highlight.
func (this *GitChangesPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.entries) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *GitChangesPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically.
func (this *GitChangesPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	viewH := h - gitChangesHeaderH
	if top, ok := this.commitBand(); ok {
		viewH = top - gitChangesHeaderH
	}
	maxScroll := float64(len(this.entries))*this.rowHeight - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a change-row index, or
// -1 when y lands on the header band. Folds the scroll offset in and
// defers to the pure rowAtY helper.
func (this *GitChangesPanel) rowAt(y float64) int {
	return rowAtY(int(y+this.scrollY), int(gitChangesHeaderH), int(this.rowHeight), len(this.entries))
}

func (this *GitChangesPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

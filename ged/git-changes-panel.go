package ged

import (
	"path/filepath"
	"strconv"
	"time"

	"silk/core"
	"silk/gui"
	"silk/paint"
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
// entries in via SetEntries; the panel renders them and emits two
// signals back:
//
//	SigFileActivated — a row was single-clicked. The host opens the file
//	                   in the editor.
//	SigFileDiff      — a row was double-clicked. The host shows the
//	                   file's diff.
//
// The framework has no native double-click event, so consecutive clicks
// on the same row are timed here, the same idiom file-explorer.go and
// debug-panel.go use. A double-click therefore fires SigFileActivated on
// the first click and SigFileDiff on the second — which mirrors VS Code,
// where a single click opens the file and a double click opens its diff.
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

	// Double-click detection, mirroring file-explorer.go / debug-panel.go.
	lastClickIdx  int
	lastClickTime time.Time

	cbActivated func(entry core.GitStatusEntry)
	cbDiff      func(entry core.GitStatusEntry)
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
	this.Self().Update()
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

// --- Drawing ---

const gitChangesHeaderH = 22.0

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

	if len(this.entries) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := gitChangesHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.entries); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
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

		// Status letter in its accent colour, left gutter.
		letter := statusLetter(e)
		g.SetBrush1(statusColor(letter))
		g.DrawText1(8, y+fe.Ascent+2, letter)

		// Path: dimmed directory + emphasised basename. For a rename the
		// whole "orig -> path" string is the label and the basename split
		// operates on its last path element.
		x := 24.0
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

// splitPathLabel splits a row label into a dimmed leading directory part
// (including the trailing slash) and the emphasised basename. The split
// is on the last "/" of the label so a rename's "a/b -> c/d" keeps
// "c/d"'s directory dimmed and "d" emphasised. A label with no slash is
// all basename.
func splitPathLabel(label string) (dir, base string) {
	return filepath.Split(label)
}

// --- Events ---

// OnLeftDown fires SigFileActivated for the clicked row, and treats a
// quick second click on the same row as a diff request (SigFileDiff),
// the same double-click idiom as file-explorer.go / debug-panel.go.
func (this *GitChangesPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.entries) {
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
	maxScroll := float64(len(this.entries))*this.rowHeight - (h - gitChangesHeaderH)
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

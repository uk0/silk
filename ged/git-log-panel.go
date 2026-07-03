package ged

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.GitLogPanel", gui.TypeOf(GitLogPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GitLogPanel",
		Name: "历史 / Git Log",
		Icon: "tree-view",
		Desc: "最近提交历史(hash / 主题 / 作者 / 日期)",
	})
}

// GitLogPanel is the bottom-dock "History" pane: the list of recent
// commits, like Qt Creator's "Git > Log" or VS Code's timeline. It is a
// pure display/interaction widget — it never shells out to git itself.
// The host (silkide) drives core.GitShortLog(dir, n) (repo history) or
// core.GitLogFile(dir, file, n) (single-file history) and pushes the
// commits in via SetCommits; the panel renders them and emits one signal
// back:
//
//	SigCommitActivated — a row was clicked. The host acts on the commit
//	                     (show it, or open its diff).
//
// core.GitCommit is a plain {Hash, Subject, Author, Date} data struct, so
// it is used directly on the public API surface — the same choice
// GitChangesPanel makes with core.GitStatusEntry, and unlike
// ReferencesPanel / TodoPanel which keep their own flat row types because
// they translate away from wire shapes (LSP locations, marker scans).
type GitLogPanel struct {
	gui.Widget

	commits   []core.GitCommit
	scrollY   float64
	hoverIdx  int
	selected  int // index of the last-activated row, -1 when none
	rowHeight float64

	cbActivate func(commit core.GitCommit)
}

// NewGitLogPanel creates an empty history panel.
func NewGitLogPanel() *GitLogPanel {
	p := new(GitLogPanel)
	p.Init(p)
	return p
}

func (this *GitLogPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.selected = -1
}

// SetCommits replaces the history rows with a defensive copy and resets
// the view. A copy is taken so the host can keep mutating (or reuse) the
// slice it handed in — e.g. a later GitShortLog refresh — without
// corrupting the panel's state mid-paint.
func (this *GitLogPanel) SetCommits(commits []core.GitCommit) {
	out := make([]core.GitCommit, len(commits))
	copy(out, commits)
	this.commits = out
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// Commits returns a defensive copy of the history rows in display order.
// Returning the backing slice would let callers mutate the panel's state
// from the outside.
func (this *GitLogPanel) Commits() []core.GitCommit {
	out := make([]core.GitCommit, len(this.commits))
	copy(out, this.commits)
	return out
}

// Clear removes all history rows and resets the view.
func (this *GitLogPanel) Clear() {
	this.commits = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// SigCommitActivated registers the callback fired when the user clicks a
// commit row. It receives the whole commit so the host can show it or its
// diff.
func (this *GitLogPanel) SigCommitActivated(fn func(commit core.GitCommit)) {
	this.cbActivate = fn
}

// --- Pure helpers (GL-free, unit-testable) ---

// logRowAtY maps a y coordinate to a commit-row index for a list whose
// rows start at topOffset, with count rows of height rowH. The caller
// folds the scroll offset into y before calling. It returns -1 when y
// lands above the rows (the header band), past the last row, or when rowH
// is degenerate. Pure so the hit-test needs no widget or GL. (Named
// logRowAtY, not rowAtY, because git-changes-panel.go already owns a
// package-level rowAtY — same namespacing as references-panel.go's
// refRowAtY and todo-panel.go's todoRowAtY.)
func logRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// logRowLabel formats a commit's compact one-line label as
// "<shorthash>  <subject>" (e.g. "a1b2c3d  fix build"). Hash comes from
// git's %h so it is already abbreviated. Pure and testable. (Named
// logRowLabel to avoid git-changes-panel.go's package-level rowLabel.)
func logRowLabel(c core.GitCommit) string {
	return c.Hash + "  " + c.Subject
}

// logTruncate shortens s to at most maxRunes runes, replacing the tail
// with a single "…" when it must cut. maxRunes <= 0 yields "". Rune-based
// so a multi-byte subject is never split mid-character. Pure and testable.
func logTruncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(r[:maxRunes-1]) + "…"
}

// logCountLabel renders the header tally, e.g. "历史 / History (3)". Kept
// as a free function so the header text is pure and testable without the
// renderer.
func logCountLabel(count int) string {
	return "历史 / History (" + strconv.Itoa(count) + ")"
}

// --- Drawing ---

const gitLogHeaderH = 22.0

// Draw renders the count header followed by one row per commit: the short
// hash in a dim gold accent, the subject in primary grey (truncated to the
// space before the meta column), and the author + date dim and
// right-aligned. Alternating tint, plus a hover / selection highlight.
func (this *GitLogPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (changes/references/todo).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band with the commit count.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, gitLogHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, logCountLabel(len(this.commits)))

	if len(this.commits) == 0 {
		return
	}

	// Monospace advance, used to budget the subject's rune width.
	charW := font.TextExtents("0").Width

	rh := this.rowHeight
	areaTop := gitLogHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.commits); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		c := this.commits[i]

		// Selection wins over hover wins over the alternating stripe.
		if i == this.selected {
			g.SetBrush1(paint.Color{R: 55, G: 70, B: 95, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		ty := y + fe.Ascent + 2

		// Meta (author + date) dim and right-aligned; measured first so the
		// subject can be truncated to stop short of it.
		meta := c.Author + "  " + c.Date
		metaExt := font.TextExtents(meta)
		metaX := w - metaExt.Width - 8
		g.SetBrush1(paint.Color{R: 130, G: 140, B: 155, A: 255})
		g.DrawText1(metaX, ty, meta)

		// Compact "<hash>  <subject>" label, truncated to the room left of
		// the meta column. Drawn once in subject-primary, then the hash
		// prefix is over-drawn in the gold accent — a cheap way to two-tone
		// one logical label without splitting it into two positioned spans.
		label := logRowLabel(c)
		if charW > 0 {
			avail := metaX - 8 - 12
			label = logTruncate(label, int(avail/charW))
		}
		g.SetBrush1(paint.Color{R: 205, G: 205, B: 215, A: 255})
		g.DrawText1(8, ty, label)

		// Accent only the portion of the hash that survived truncation, so
		// the gold never overhangs the label at very narrow widths.
		labelR := []rune(label)
		n := len([]rune(c.Hash))
		if n > len(labelR) {
			n = len(labelR)
		}
		g.SetBrush1(paint.Color{R: 214, G: 184, B: 110, A: 255})
		g.DrawText1(8, ty, string(labelR[:n]))
	}
}

// --- Events ---

// OnLeftDown fires the activated callback for the clicked commit row (the
// host shows the commit or its diff) and highlights it. Clicks in the
// header band are inert.
func (this *GitLogPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.commits) {
		return
	}
	this.selected = idx
	this.Self().Update()
	if this.cbActivate != nil {
		this.cbActivate(this.commits[idx])
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *GitLogPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.commits) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *GitLogPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically, clamped to the content.
func (this *GitLogPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.commits))*this.rowHeight - (h - gitLogHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a commit index, or -1
// when y lands on the header band or past the last row. It folds the
// scroll offset into y and defers to the pure logRowAtY helper.
func (this *GitLogPanel) rowAt(y float64) int {
	return logRowAtY(y+this.scrollY, gitLogHeaderH, this.rowHeight, len(this.commits))
}

func (this *GitLogPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

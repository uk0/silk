package ged

import (
	"path/filepath"
	"strconv"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.ReferencesPanel", gui.TypeOf(ReferencesPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ReferencesPanel",
		Name: "引用 / References",
		Icon: "search",
		Desc: "符号的所有引用 (查找全部引用)",
	})
}

// ReferenceLoc is one usage site of a symbol — a single row in the
// "Find All References" list. It is the panel's own flat shape, NOT
// core.LSPLocation: the host (silkide) drives core.LSPClient.References,
// then converts each LSPLocation (uri + 0-based range) into a
// ReferenceLoc (file path + 1-based line + column + the source line
// text) before pushing the slice in via SetLocations. Keeping the panel
// on its own struct means it never has to know about LSP wire types or
// 0-vs-1-based conventions — that translation lives in the host.
type ReferenceLoc struct {
	File    string // absolute or workspace-relative path to the source file
	Line    int    // 1-based line number, ready to display/jump to
	Col     int    // column on that line (host's choice of base; passed through to the jump callback)
	Preview string // the source line's text, trimmed, shown alongside the locator
}

// ReferencesPanel is the bottom-dock pane that lists every usage of a
// symbol, modelled on VS Code's references view and on the sibling
// ProblemsPanel: a counted header, one row per location, a per-row
// file:line locator, alternating row tint, wheel scroll and a hover/
// selection highlight. Clicking a row emits SigLocationActivated so the
// host can open that file at that line.
//
// Like DebugPanel it is a pure display/interaction widget: it never
// talks to gopls itself. The host fetches the references, converts them
// to []ReferenceLoc and calls SetLocations; the panel only renders and
// reports clicks back.
//
// v1 is a flat list. Grouping the rows by file (a collapsible
// file-header tree, the way VS Code stacks "5 references in foo.go") is
// a deliberate follow-up — the data-push API and the row geometry stay
// the same, only Draw and the hit-test change.
type ReferencesPanel struct {
	gui.Widget

	locs      []ReferenceLoc
	scrollY   float64
	hoverIdx  int
	selected  int // index of the last-activated row, -1 when none
	rowHeight float64

	cbActivate func(file string, line, col int)
}

// NewReferencesPanel creates an empty references panel.
func NewReferencesPanel() *ReferencesPanel {
	p := new(ReferencesPanel)
	p.Init(p)
	return p
}

func (this *ReferencesPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.selected = -1
}

// SetLocations replaces the reference rows with a defensive copy and
// resets the view. A copy is taken so the host can keep mutating (or
// reuse) the slice it handed in without corrupting the panel's state.
func (this *ReferencesPanel) SetLocations(locs []ReferenceLoc) {
	this.locs = make([]ReferenceLoc, len(locs))
	copy(this.locs, locs)
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// Locations returns a defensive copy of the reference rows in display
// order. Returning the backing slice would let callers mutate the
// panel's state from the outside.
func (this *ReferencesPanel) Locations() []ReferenceLoc {
	out := make([]ReferenceLoc, len(this.locs))
	copy(out, this.locs)
	return out
}

// Clear removes all reference rows and resets the view.
func (this *ReferencesPanel) Clear() {
	this.locs = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// SigLocationActivated registers the callback fired when the user clicks
// a reference row. It receives the target file, the 1-based line, and
// the column — the host opens file:line in the editor.
func (this *ReferencesPanel) SigLocationActivated(fn func(file string, line, col int)) {
	this.cbActivate = fn
}

// --- Pure helpers (GL-free, unit-testable) ---

// refRowAtY maps a y coordinate to a reference-row index for a list
// whose rows start at topOffset, with count rows of height rowH. The
// caller folds the scroll offset into y before calling. It returns -1
// when y lands above the rows (the header band), past the last row, or
// when rowH is degenerate. Pure so the hit-test needs no widget or GL.
// (Named refRowAtY, not rowAtY, because git-changes-panel.go already
// owns a package-level rowAtY — same namespacing as debug-panel.go's
// frameRowAtY.)
func refRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// refRowLabel formats a location's left-hand locator as "basename:line"
// (e.g. "foo.go:42"), dropping the directory so the list stays scannable
// regardless of how deep the file lives. Pure and testable. (Named
// refRowLabel to avoid git-changes-panel.go's package-level rowLabel.)
func refRowLabel(loc ReferenceLoc) string {
	return filepath.Base(loc.File) + ":" + strconv.Itoa(loc.Line)
}

// referenceCountLabel renders the header tally, e.g. "引用 / References
// (3)". Kept as a free function so the header text is pure and testable
// without the renderer.
func referenceCountLabel(count int) string {
	return "引用 / References (" + strconv.Itoa(count) + ")"
}

// --- Drawing ---

const referencesHeaderH = 22.0

// Draw renders the count header followed by one row per reference: a
// dimmed "basename:line" locator on the left and the trimmed source-line
// preview after it, with alternating tint and a hover/selection
// highlight.
func (this *ReferencesPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (problems/log/debug).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band with the reference count.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, referencesHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, referenceCountLabel(len(this.locs)))

	if len(this.locs) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := referencesHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.locs); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		loc := this.locs[i]

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

		// Locator "basename:line" in muted blue-grey.
		label := refRowLabel(loc)
		g.SetBrush1(paint.Color{R: 120, G: 160, B: 210, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, label)
		labelExt := font.TextExtents(label)

		// Source-line preview in light grey, after the locator.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(8+labelExt.Width+12, y+fe.Ascent+2, loc.Preview)
	}
}

// --- Events ---

// OnLeftDown fires the activated callback for the clicked reference row
// (the host opens file:line) and highlights it. Clicks in the header
// band are inert.
func (this *ReferencesPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.locs) {
		return
	}
	this.selected = idx
	this.Self().Update()
	if this.cbActivate != nil {
		loc := this.locs[idx]
		this.cbActivate(loc.File, loc.Line, loc.Col)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *ReferencesPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.locs) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *ReferencesPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically, clamped to the content.
func (this *ReferencesPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.locs))*this.rowHeight - (h - referencesHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a reference index, or
// -1 when y lands on the header band or past the last row. It folds the
// scroll offset into y and defers to the pure rowAtY helper.
func (this *ReferencesPanel) rowAt(y float64) int {
	return refRowAtY(y+this.scrollY, referencesHeaderH, this.rowHeight, len(this.locs))
}

func (this *ReferencesPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

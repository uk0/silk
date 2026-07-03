package ged

import (
	"path/filepath"
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.TodoPanel", gui.TypeOf(TodoPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TodoPanel",
		Name: "待办 / TODO",
		Icon: "document",
		Desc: "项目中的 TODO/FIXME 标记列表",
	})
}

// TodoRow is one TODO/FIXME marker — a single row in the待办 list. It is
// the panel's own flat display shape, deliberately NOT the core scanner's
// result type: the host (silkide) runs the marker scan and converts each
// hit into a TodoRow before pushing the slice in via SetRows. Keeping the
// panel on its own struct means it never imports the scanner and never has
// to know how markers are discovered — that translation lives in the host,
// exactly as ReferencesPanel stays off core.LSPLocation.
type TodoRow struct {
	File string // absolute or workspace-relative path to the source file
	Line int    // 1-based line number, ready to display / jump to
	Kind string // marker kind: TODO / FIXME / XXX / HACK / NOTE
	Text string // the marker comment text, trimmed
}

// TodoPanel is the bottom-dock pane that lists every TODO/FIXME marker in
// the project, modelled on the sibling ReferencesPanel and ProblemsPanel:
// a counted header, one row per marker, a per-row "basename:line" locator,
// alternating row tint, wheel scroll and a hover/selection highlight. Each
// row leads with a colour-coded Kind badge (TODO amber, FIXME red,
// XXX/HACK orange, NOTE grey). Clicking a row emits SigRowActivated so the
// host can open that file at that line.
//
// Like ReferencesPanel it is a pure display/interaction widget: it never
// scans for markers itself. The host gathers them, converts to []TodoRow
// and calls SetRows; the panel only renders and reports clicks back.
type TodoPanel struct {
	gui.Widget

	rows      []TodoRow
	scrollY   float64
	hoverIdx  int
	selected  int // index of the last-activated row, -1 when none
	rowHeight float64

	cbActivate func(file string, line int)
}

// NewTodoPanel creates an empty TODO panel.
func NewTodoPanel() *TodoPanel {
	p := new(TodoPanel)
	p.Init(p)
	return p
}

func (this *TodoPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.selected = -1
}

// SetRows replaces the marker rows with a defensive copy and resets the
// view. A copy is taken so the host can keep mutating (or reuse) the slice
// it handed in without corrupting the panel's state.
func (this *TodoPanel) SetRows(rows []TodoRow) {
	this.rows = make([]TodoRow, len(rows))
	copy(this.rows, rows)
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// Rows returns a defensive copy of the marker rows in display order.
// Returning the backing slice would let callers mutate the panel's state
// from the outside.
func (this *TodoPanel) Rows() []TodoRow {
	out := make([]TodoRow, len(this.rows))
	copy(out, this.rows)
	return out
}

// Clear removes all marker rows and resets the view.
func (this *TodoPanel) Clear() {
	this.rows = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// SigRowActivated registers the callback fired when the user clicks a
// marker row. It receives the target file and the 1-based line — the host
// opens file:line in the editor.
func (this *TodoPanel) SigRowActivated(fn func(file string, line int)) {
	this.cbActivate = fn
}

// --- Pure helpers (GL-free, unit-testable) ---

// todoRowAtY maps a y coordinate to a marker-row index for a list whose
// rows start at topOffset, with count rows of height rowH. The caller
// folds the scroll offset into y before calling. It returns -1 when y
// lands above the rows (the header band), past the last row, or when rowH
// is degenerate. Pure so the hit-test needs no widget or GL. (Named
// todoRowAtY, not rowAtY, because git-changes-panel.go already owns a
// package-level rowAtY — same namespacing as references-panel.go's
// refRowAtY.)
func todoRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// todoKindColor maps a marker kind to its badge colour: TODO amber, FIXME
// red, XXX/HACK orange, NOTE grey, and a neutral grey for anything else.
// Kept as a free function so the palette is pure and testable without the
// renderer.
func todoKindColor(kind string) paint.Color {
	switch kind {
	case "TODO":
		return paint.Color{R: 230, G: 180, B: 60, A: 255} // amber
	case "FIXME":
		return paint.Color{R: 230, G: 80, B: 80, A: 255} // red
	case "XXX", "HACK":
		return paint.Color{R: 230, G: 140, B: 60, A: 255} // orange
	case "NOTE":
		return paint.Color{R: 150, G: 150, B: 160, A: 255} // grey
	}
	return paint.Color{R: 130, G: 130, B: 140, A: 255} // default neutral grey
}

// todoRowLabel formats a marker's left-hand locator as "basename:line"
// (e.g. "foo.go:42"), dropping the directory so the list stays scannable
// regardless of how deep the file lives. Pure and testable. (Named
// todoRowLabel to avoid git-changes-panel.go's package-level rowLabel.)
func todoRowLabel(r TodoRow) string {
	return filepath.Base(r.File) + ":" + strconv.Itoa(r.Line)
}

// todoCountLabel renders the header tally, e.g. "待办 / TODO (3)". Kept as
// a free function so the header text is pure and testable without the
// renderer.
func todoCountLabel(count int) string {
	return "待办 / TODO (" + strconv.Itoa(count) + ")"
}

// --- Drawing ---

const todoHeaderH = 22.0

// Draw renders the count header followed by one row per marker: a
// colour-coded Kind badge, a dimmed "basename:line" locator and the marker
// text, with alternating tint and a hover/selection highlight.
func (this *TodoPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (references/problems/log).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band with the marker count.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, todoHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, todoCountLabel(len(this.rows)))

	if len(this.rows) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := todoHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	const badgePadX = 6.0

	for i := startIdx; i < startIdx+visibleCount && i < len(this.rows); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		r := this.rows[i]

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

		// Kind badge: a filled pill in the kind's colour with the kind
		// text in near-black for contrast against the light badge fill.
		kindExt := font.TextExtents(r.Kind)
		badgeW := kindExt.Width + badgePadX*2
		g.SetBrush1(todoKindColor(r.Kind))
		g.Rectangle(8, y+4, badgeW, rh-8)
		g.Fill()
		g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
		g.DrawText1(8+badgePadX, y+fe.Ascent+2, r.Kind)

		// Locator "basename:line" in dim grey, after the badge.
		locX := 8 + badgeW + 8
		label := todoRowLabel(r)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(locX, y+fe.Ascent+2, label)
		labelExt := font.TextExtents(label)

		// Marker text in light grey, after the locator.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(locX+labelExt.Width+12, y+fe.Ascent+2, r.Text)
	}
}

// --- Events ---

// OnLeftDown fires the activated callback for the clicked marker row (the
// host opens file:line) and highlights it. Clicks in the header band are
// inert.
func (this *TodoPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	this.selected = idx
	this.Self().Update()
	if this.cbActivate != nil {
		r := this.rows[idx]
		this.cbActivate(r.File, r.Line)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *TodoPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *TodoPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically, clamped to the content.
func (this *TodoPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.rows))*this.rowHeight - (h - todoHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a marker index, or -1
// when y lands on the header band or past the last row. It folds the
// scroll offset into y and defers to the pure todoRowAtY helper.
func (this *TodoPanel) rowAt(y float64) int {
	return todoRowAtY(y+this.scrollY, todoHeaderH, this.rowHeight, len(this.rows))
}

func (this *TodoPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

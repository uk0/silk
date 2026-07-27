package ged

import (
	"path/filepath"
	"strconv"
	"strings"

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
	Kind string // marker kind: TODO / FIXME / XXX / HACK / NOTE / BUG
	Text string // the marker comment text, trimmed
}

// TodoGroupMode is how TodoPanel arranges its rows.
type TodoGroupMode int

const (
	TodoGroupNone   TodoGroupMode = iota // flat, in the order the host supplied
	TodoGroupByTag                       // one group per marker kind
	TodoGroupByFile                      // one group per file
)

// TodoPanel is the bottom-dock pane that lists every TODO/FIXME marker in
// the project, modelled on the sibling ReferencesPanel and ProblemsPanel:
// a counted header, one row per marker, a per-row "basename:line" locator,
// alternating row tint, wheel scroll and a hover/selection highlight. Each
// row leads with a colour-coded Kind badge (TODO amber, FIXME red,
// XXX/HACK orange, NOTE grey). Clicking a row emits SigActivate so the
// host can open that file at that line.
//
// A flat list stops being readable at a few hundred markers, so the header
// band carries two controls: a group toggle (flat / by tag / by file, which
// inserts a counted group header row before each group) and a filter box
// that keeps only the rows matching a substring. Both are view state — the
// row set the host handed in via SetRows is never modified, and Rows()
// always returns all of it. Grouping and filtering are recomputed into
// visRows, which is what the panel draws and hit-tests.
//
// Like ReferencesPanel it is a pure display/interaction widget: it never
// scans for markers itself. The host gathers them, converts to []TodoRow
// and calls SetRows; the panel only renders and reports clicks back.
type TodoPanel struct {
	gui.Widget

	rows      []TodoRow
	visRows   []todoVisRow // grouped + filtered display rows
	groupBy   TodoGroupMode
	filter    []rune // filter-box text
	filterHot bool   // true when the filter box holds keyboard focus

	scrollY   float64
	hoverIdx  int // index into visRows, -1 when none
	selected  int // index into rows of the last-activated marker, -1 when none
	rowHeight float64

	cbActivate func(file string, line int)
}

// todoVisRow is one drawn line: either a group header or a marker row that
// points back into TodoPanel.rows.
type todoVisRow struct {
	header bool
	title  string // group title (header rows only)
	count  int    // markers in the group (header rows only)
	idx    int    // index into TodoPanel.rows; -1 on header rows
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
	this.rebuildVisRows()
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
	this.visRows = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// SigActivate registers the callback fired when the user clicks a marker
// row. It receives the target file and the 1-based line — the host opens
// file:line in the editor.
func (this *TodoPanel) SigActivate(fn func(file string, line int)) {
	this.cbActivate = fn
}

// SigRowActivated is the original name of SigActivate, kept for the hosts
// already wired to it.
func (this *TodoPanel) SigRowActivated(fn func(file string, line int)) {
	this.SigActivate(fn)
}

// SetGroupBy switches the row arrangement (flat / by tag / by file) and
// rebuilds the display rows. Selection survives: it indexes rows, not the
// display order.
func (this *TodoPanel) SetGroupBy(m TodoGroupMode) {
	this.groupBy = m
	this.scrollY = 0
	this.hoverIdx = -1
	this.rebuildVisRows()
	this.Self().Update()
}

// GroupBy returns the current grouping mode.
func (this *TodoPanel) GroupBy() TodoGroupMode {
	return this.groupBy
}

// ToggleGroupBy cycles flat -> by tag -> by file -> flat. It is what the
// header's group button does.
func (this *TodoPanel) ToggleGroupBy() {
	switch this.groupBy {
	case TodoGroupNone:
		this.SetGroupBy(TodoGroupByTag)
	case TodoGroupByTag:
		this.SetGroupBy(TodoGroupByFile)
	default:
		this.SetGroupBy(TodoGroupNone)
	}
}

// SetFilter keeps only the marker rows matching s (case-insensitive
// substring over kind, text and file path). An empty filter shows all rows.
func (this *TodoPanel) SetFilter(s string) {
	this.filter = []rune(s)
	this.scrollY = 0
	this.hoverIdx = -1
	this.rebuildVisRows()
	this.Self().Update()
}

// Filter returns the current filter text.
func (this *TodoPanel) Filter() string {
	return string(this.filter)
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

// todoKindColor maps a marker kind to its badge colour: TODO amber,
// FIXME/BUG red, XXX/HACK orange, NOTE grey, and a neutral grey for anything
// else. Kept as a free function so the palette is pure and testable without
// the renderer.
func todoKindColor(kind string) paint.Color {
	switch kind {
	case "TODO":
		return paint.Color{R: 230, G: 180, B: 60, A: 255} // amber
	case "FIXME", "BUG":
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

// todoRowMatches reports whether a marker row survives the filter. The
// filter is a case-insensitive substring tested against the kind, the
// marker text and the file path, so "fixme", "scroll" and "ged/" all work.
// lowerFilter must already be lowercased and trimmed; empty matches all.
func todoRowMatches(r TodoRow, lowerFilter string) bool {
	if lowerFilter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Kind), lowerFilter) ||
		strings.Contains(strings.ToLower(r.Text), lowerFilter) ||
		strings.Contains(strings.ToLower(r.File), lowerFilter)
}

// todoGroupKey is the group a row belongs to under mode m.
func todoGroupKey(r TodoRow, m TodoGroupMode) string {
	if m == TodoGroupByFile {
		return r.File
	}
	return r.Kind
}

// todoGroupHeaderLabel renders a group header, e.g. "TODO (3)" or
// "ged/foo.go (2)".
func todoGroupHeaderLabel(title string, count int) string {
	return title + " (" + strconv.Itoa(count) + ")"
}

// todoGroupLabel is the group button's caption for a mode.
func todoGroupLabel(m TodoGroupMode) string {
	switch m {
	case TodoGroupByTag:
		return "按标记"
	case TodoGroupByFile:
		return "按文件"
	}
	return "平铺"
}

// rebuildVisRows recomputes the drawn rows from rows + filter + groupBy.
// Groups keep the order their first member appeared in, and members keep
// the host's order inside a group, so a stable input list yields a stable
// display without a sort.
func (this *TodoPanel) rebuildVisRows() {
	this.visRows = nil
	lower := strings.ToLower(strings.TrimSpace(string(this.filter)))

	keep := make([]int, 0, len(this.rows))
	for i := range this.rows {
		if todoRowMatches(this.rows[i], lower) {
			keep = append(keep, i)
		}
	}

	if this.groupBy == TodoGroupNone {
		for _, i := range keep {
			this.visRows = append(this.visRows, todoVisRow{idx: i})
		}
		return
	}

	var keys []string
	members := make(map[string][]int, len(keep))
	for _, i := range keep {
		k := todoGroupKey(this.rows[i], this.groupBy)
		if _, seen := members[k]; !seen {
			keys = append(keys, k)
		}
		members[k] = append(members[k], i)
	}
	for _, k := range keys {
		this.visRows = append(this.visRows, todoVisRow{
			header: true,
			title:  k,
			count:  len(members[k]),
			idx:    -1,
		})
		for _, i := range members[k] {
			this.visRows = append(this.visRows, todoVisRow{idx: i})
		}
	}
}

// Header-band chrome: the group button and the filter box sit inside the
// existing 22px header, right-aligned, so adding them does not move the
// row list down.
const (
	todoChromeH   = 16.0  // control height inside the header band
	todoChromePad = 6.0   // gap between controls / to the right edge
	todoFilterW   = 120.0 // filter box width
	todoGroupBtnW = 56.0  // group button width
)

// todoFilterRect is the filter box rect for a panel of width w.
func todoFilterRect(w float64) (x, y, bw, bh float64) {
	return w - todoChromePad - todoFilterW, (todoHeaderH - todoChromeH) / 2, todoFilterW, todoChromeH
}

// todoGroupButtonRect is the group button rect, left of the filter box.
func todoGroupButtonRect(w float64) (x, y, bw, bh float64) {
	fx, fy, _, bh := todoFilterRect(w)
	return fx - todoChromePad - todoGroupBtnW, fy, todoGroupBtnW, bh
}

// todoInRect is a plain point-in-rect test for the header controls.
func todoInRect(x, y, rx, ry, rw, rh float64) bool {
	return x >= rx && x <= rx+rw && y >= ry && y <= ry+rh
}

// --- Drawing ---

const todoHeaderH = 22.0

// Draw renders the count header (with the group toggle and filter box) and
// then the display rows: a group header row per group when grouping is on,
// and per marker a colour-coded Kind badge, a dimmed "basename:line"
// locator and the marker text, with alternating tint and a hover/selection
// highlight.
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

	// Group toggle: a small button carrying the current mode's caption.
	gx, gy, gw, gh := todoGroupButtonRect(w)
	g.SetBrush1(paint.Color{R: 60, G: 60, B: 72, A: 255})
	g.Rectangle(gx, gy, gw, gh)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(gx+4, gy+gh-4, todoGroupLabel(this.groupBy))

	// Filter box: brighter border while focused, placeholder while empty.
	fx, fy, fw, fh := todoFilterRect(w)
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(fx, fy, fw, fh)
	g.Fill()
	if this.filterHot {
		g.SetPen1(paint.Color{R: 100, G: 140, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 70, G: 70, B: 82, A: 255}, 1)
	}
	g.Rectangle(fx, fy, fw, fh)
	g.Stroke()
	if len(this.filter) > 0 {
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(fx+4, fy+fh-4, string(this.filter))
	} else {
		g.SetBrush1(paint.Color{R: 120, G: 120, B: 135, A: 255})
		g.DrawText1(fx+4, fy+fh-4, "过滤...")
	}

	if len(this.visRows) == 0 {
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

	for i := startIdx; i < startIdx+visibleCount && i < len(this.visRows); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		vr := this.visRows[i]

		// Group header: its own band plus "title (count)".
		if vr.header {
			g.SetBrush1(paint.Color{R: 44, G: 44, B: 54, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
			g.SetBrush1(paint.Color{R: 170, G: 185, B: 205, A: 255})
			g.DrawText1(8, y+fe.Ascent+2, todoGroupHeaderLabel(vr.title, vr.count))
			continue
		}

		r := this.rows[vr.idx]

		// Selection wins over hover wins over the alternating stripe.
		if vr.idx == this.selected {
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
// host opens file:line) and highlights it. In the header band it drives the
// group toggle and focuses the filter box; a click anywhere else in the
// header, or on a group header row, is inert.
func (this *TodoPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if y < todoHeaderH {
		w := this.Width()
		gx, gy, gw, gh := todoGroupButtonRect(w)
		fx, fy, fw, fh := todoFilterRect(w)
		switch {
		case todoInRect(x, y, gx, gy, gw, gh):
			this.ToggleGroupBy()
		case todoInRect(x, y, fx, fy, fw, fh):
			this.filterHot = true
			this.Self().Update()
		}
		return
	}

	this.filterHot = false
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		this.Self().Update()
		return
	}
	this.selected = idx
	this.Self().Update()
	if this.cbActivate != nil {
		r := this.rows[idx]
		this.cbActivate(r.File, r.Line)
	}
}

// OnTextInput appends typed text to the filter box while it holds focus and
// re-filters the list. Typing with the box unfocused is ignored.
func (this *TodoPanel) OnTextInput(s string) {
	if !this.filterHot {
		return
	}
	this.filter = append(this.filter, []rune(s)...)
	this.scrollY = 0
	this.hoverIdx = -1
	this.rebuildVisRows()
	this.Self().Update()
}

// OnKeyDown edits the filter box: Backspace deletes the last rune, Esc
// clears the filter and drops focus. Keys are ignored while the box is not
// focused.
func (this *TodoPanel) OnKeyDown(key int, repeat bool) {
	if !this.filterHot {
		return
	}
	switch key {
	case gui.KeyBackSpace:
		if len(this.filter) == 0 {
			return
		}
		this.filter = this.filter[:len(this.filter)-1]
	case gui.KeyEsc:
		this.filter = nil
		this.filterHot = false
	default:
		return
	}
	this.scrollY = 0
	this.hoverIdx = -1
	this.rebuildVisRows()
	this.Self().Update()
}

// OnMouseMove tracks hover state for the row highlight.
func (this *TodoPanel) OnMouseMove(x, y float64) {
	idx := this.visRowAt(y)
	if idx >= 0 && this.visRows[idx].header {
		idx = -1 // group headers do not highlight
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
	maxScroll := float64(len(this.visRows))*this.rowHeight - (h - todoHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// visRowAt maps a y coordinate (below the header) to a display-row index,
// or -1 when y lands on the header band or past the last row. It folds the
// scroll offset into y and defers to the pure todoRowAtY helper.
func (this *TodoPanel) visRowAt(y float64) int {
	return todoRowAtY(y+this.scrollY, todoHeaderH, this.rowHeight, len(this.visRows))
}

// rowAt maps a y coordinate to an index into rows, or -1 when y lands on
// the header band, on a group header row, or past the last row. Ungrouped
// and unfiltered, display rows and marker rows line up one-to-one.
func (this *TodoPanel) rowAt(y float64) int {
	i := this.visRowAt(y)
	if i < 0 || this.visRows[i].header {
		return -1
	}
	return this.visRows[i].idx
}

func (this *TodoPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

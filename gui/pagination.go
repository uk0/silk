package gui

import (
	"math"
	"strconv"

	"silk/core"
	"silk/paint"
)

func init() {
	core.RegisterFactory("gui.Pagination", core.TypeOf((*Pagination)(nil)))
}

// pageEllipsis is the sentinel paginationItems uses in its returned
// slice to mark a "…" gap between page-number runs. Any value < 1
// would do; -1 reads clearly at the call sites.
const pageEllipsis = -1

// Pagination is a page-navigation control: a row of square cells
// holding a previous-arrow, page numbers (with "…" gaps collapsed
// for large page counts), and a next-arrow. The active page is
// highlighted in the theme accent colour; the prev/next arrows grey
// out at the first / last page.
//
// Usage:
//
//	pg := gui.NewPagination()
//	pg.SetTotalPages(20)
//	pg.SetCurrentPage(1)
//	pg.SigChange(func(page int) { table.LoadPage(page) })
//
// The page model is 1-based throughout — SetCurrentPage(1) is the
// first page, matching how users think about pagination and how the
// numbers render.
type Pagination struct {
	Widget

	total    int // total page count; <1 renders an empty control
	current  int // active page, 1-based, clamped to [1, total]
	sibling  int // pages shown either side of current (default 1)
	boundary int // pages pinned at each end (default 1)

	hover    int // index into the live cell list, -1 when none
	cbChange func(page int)
}

// NewPagination creates a pager with sensible defaults: 1 total page,
// current page 1, one sibling each side of the current page, and one
// boundary page pinned at each end. Callers set the real total via
// SetTotalPages once the data size is known.
func NewPagination() *Pagination {
	p := new(Pagination)
	p.Init(p)
	p.total = 1
	p.current = 1
	p.sibling = 1
	p.boundary = 1
	p.hover = -1
	return p
}

// TotalPages returns the configured page count.
func (this *Pagination) TotalPages() int { return this.total }

// SetTotalPages sets the page count. Values < 1 clamp to 0 (an empty
// control). If the current page now exceeds the new total it snaps to
// the last page so the highlight never points past the end.
func (this *Pagination) SetTotalPages(n int) {
	if n < 0 {
		n = 0
	}
	if n == this.total {
		return
	}
	this.total = n
	if this.current > n {
		this.current = n
	}
	if this.current < 1 && n >= 1 {
		this.current = 1
	}
	this.Self().Update()
}

// CurrentPage returns the active (1-based) page.
func (this *Pagination) CurrentPage() int { return this.current }

// SetCurrentPage moves the active page, clamped to [1, total]. Fires
// the SigChange callback only when the page actually changes, so a
// redundant SetCurrentPage(current) is a cheap no-op.
func (this *Pagination) SetCurrentPage(page int) {
	if this.total < 1 {
		return
	}
	if page < 1 {
		page = 1
	}
	if page > this.total {
		page = this.total
	}
	if page == this.current {
		return
	}
	this.current = page
	if this.cbChange != nil {
		this.cbChange(page)
	}
	this.Self().Update()
}

// SiblingCount returns how many pages flank the current one.
func (this *Pagination) SiblingCount() int { return this.sibling }

// SetSiblingCount sets how many pages show either side of the current
// page. 0 shows only the current page between the ellipses; the
// default 1 gives the familiar "4 [5] 6" cluster.
func (this *Pagination) SetSiblingCount(n int) {
	if n < 0 {
		n = 0
	}
	this.sibling = n
	this.Self().Update()
}

// BoundaryCount returns how many pages are pinned at each end.
func (this *Pagination) BoundaryCount() int { return this.boundary }

// SetBoundaryCount sets how many pages stay pinned at the start and
// end (the "1 …" and "… 20" anchors). Default 1.
func (this *Pagination) SetBoundaryCount(n int) {
	if n < 0 {
		n = 0
	}
	this.boundary = n
	this.Self().Update()
}

// SigChange registers the page-change callback. Receives the new
// 1-based page number.
func (this *Pagination) SigChange(fn func(page int)) {
	this.cbChange = fn
}

// paginationItems computes the visible token list for a pager: page
// numbers interleaved with pageEllipsis sentinels. The result always
// includes the first `boundary` and last `boundary` pages, the
// `current` page, and `sibling` pages either side of current. Gaps
// wider than one page collapse to a single pageEllipsis; a gap of
// exactly one page is filled with that page instead (so the control
// shows "1 2 3" rather than the nonsensical "1 … 3").
//
// Pulled out as a free function so the (fiddly) range logic is unit-
// testable without standing up a GL context or a real widget.
func paginationItems(total, current, sibling, boundary int) []int {
	if total <= 0 {
		return nil
	}
	if current < 1 {
		current = 1
	}
	if current > total {
		current = total
	}
	if sibling < 0 {
		sibling = 0
	}
	if boundary < 0 {
		boundary = 0
	}

	show := make(map[int]bool, total)
	mark := func(p int) {
		if p >= 1 && p <= total {
			show[p] = true
		}
	}
	for i := 1; i <= boundary; i++ {
		mark(i)
	}
	for i := total - boundary + 1; i <= total; i++ {
		mark(i)
	}
	for i := current - sibling; i <= current+sibling; i++ {
		mark(i)
	}
	mark(current)

	var out []int
	prev := 0
	for p := 1; p <= total; p++ {
		if !show[p] {
			continue
		}
		if prev != 0 {
			switch p - prev {
			case 1:
				// adjacent, nothing to insert
			case 2:
				out = append(out, prev+1) // fill the lone gap
			default:
				out = append(out, pageEllipsis)
			}
		}
		out = append(out, p)
		prev = p
	}
	return out
}

// cellWidth is the side length of each pager cell. Fixed square cells
// keep arrows and page numbers visually aligned; three-digit page
// numbers still fit at the default theme font size.
const paginationCellW = 30.0

// cell describes one drawable/clickable slot in the pager. kind
// distinguishes arrows from numbers and ellipses; page is the target
// page for number/arrow cells (0 for ellipsis, which is inert).
type paginationCellKind int

const (
	cellPrev paginationCellKind = iota
	cellNext
	cellPage
	cellGap
)

type paginationCell struct {
	kind paginationCellKind
	page int // target page for cellPage / cellPrev / cellNext
}

// cells returns the full clickable cell list left-to-right: prev
// arrow, the paginationItems tokens, next arrow. Drawing and hit-
// testing both derive from this single source so a click always lands
// on the cell the user sees.
func (this *Pagination) cells() []paginationCell {
	out := []paginationCell{{kind: cellPrev, page: this.current - 1}}
	for _, p := range paginationItems(this.total, this.current, this.sibling, this.boundary) {
		if p == pageEllipsis {
			out = append(out, paginationCell{kind: cellGap})
		} else {
			out = append(out, paginationCell{kind: cellPage, page: p})
		}
	}
	out = append(out, paginationCell{kind: cellNext, page: this.current + 1})
	return out
}

// --- Events ---

func (this *Pagination) OnMouseLeave() {
	if this.hover != -1 {
		this.hover = -1
		this.Self().Update()
	}
}

func (this *Pagination) OnMouseMove(x, y float64) {
	idx := this.hitTest(x)
	if idx != this.hover {
		this.hover = idx
		this.Self().Update()
	}
}

func (this *Pagination) OnLeftDown(x, y float64) {
	if this.total < 1 {
		return
	}
	idx := this.hitTest(x)
	if idx < 0 {
		return
	}
	cells := this.cells()
	if idx >= len(cells) {
		return
	}
	c := cells[idx]
	switch c.kind {
	case cellGap:
		return
	case cellPrev, cellPage, cellNext:
		this.SetFocus()
		this.SetCurrentPage(c.page)
	}
}

// hitTest maps an x coordinate to a cell index, or -1 when x falls
// outside the laid-out cells.
func (this *Pagination) hitTest(x float64) int {
	if x < 0 {
		return -1
	}
	idx := int(x / paginationCellW)
	if idx >= len(this.cells()) {
		return -1
	}
	return idx
}

// --- Drawing ---

func (this *Pagination) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Size()
	if this.total < 1 {
		return
	}
	cells := this.cells()
	g.SetFont(t.Font)
	f := t.Font

	for i, c := range cells {
		x := float64(i) * paginationCellW
		this.drawCell(g, t, f, c, i, x, h)
	}
}

func (this *Pagination) drawCell(g paint.Painter, t *defaultTheme, f paint.Font, c paginationCell, idx int, x, h float64) {
	cw := paginationCellW
	pad := 2.0
	r := 4.0
	active := c.kind == cellPage && c.page == this.current
	hovered := idx == this.hover && c.kind != cellGap
	// Arrows at the ends are disabled when there's nowhere to go.
	disabled := (c.kind == cellPrev && this.current <= 1) ||
		(c.kind == cellNext && this.current >= this.total)

	// Cell background: accent for the active page, a subtle hover
	// wash otherwise. Ellipses and disabled arrows stay flat.
	if active {
		paginationRoundRect(g, x+pad, pad, cw-2*pad, h-2*pad, r)
		g.SetBrush1(t.HighLightColor)
		g.Fill()
	} else if hovered && !disabled && c.kind != cellGap {
		paginationRoundRect(g, x+pad, pad, cw-2*pad, h-2*pad, r)
		g.SetBrush1(t.FormDarkColor)
		g.Fill()
	}

	// Foreground colour: white on the active accent, greyed for
	// disabled arrows, normal text otherwise.
	fg := t.TextColor
	if active {
		fg = paint.Color{255, 255, 255, 255}
	} else if disabled {
		fg = t.FormDarkColor
	}

	switch c.kind {
	case cellGap:
		this.drawCenteredText(g, f, "…", fg, x, cw, h)
	case cellPage:
		this.drawCenteredText(g, f, strconv.Itoa(c.page), fg, x, cw, h)
	case cellPrev:
		this.drawArrow(g, fg, x, cw, h, true)
	case cellNext:
		this.drawArrow(g, fg, x, cw, h, false)
	}
}

func (this *Pagination) drawCenteredText(g paint.Painter, f paint.Font, text string, fg paint.Color, x, cw, h float64) {
	ext := f.TextExtents(text)
	tx := x + (cw-ext.Width)/2 - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.SetBrush1(fg)
	g.DrawText1(tx, ty, text)
}

// drawArrow paints a small chevron centred in the cell. left=true
// points "‹" (previous), false points "›" (next).
func (this *Pagination) drawArrow(g paint.Painter, fg paint.Color, x, cw, h float64, left bool) {
	cx := x + cw/2
	cy := h / 2
	d := 4.0
	if left {
		g.MoveTo(cx+d*0.5, cy-d)
		g.LineTo(cx-d*0.5, cy)
		g.LineTo(cx+d*0.5, cy+d)
	} else {
		g.MoveTo(cx-d*0.5, cy-d)
		g.LineTo(cx+d*0.5, cy)
		g.LineTo(cx-d*0.5, cy+d)
	}
	g.SetPen1(fg, 1.5)
	g.Stroke()
}

// paginationRoundRect emits a rounded-rect path (shared shape helper,
// local to keep the pager self-contained).
func paginationRoundRect(g paint.Painter, x, y, w, h, r float64) {
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(x+r, y)
}

// --- SizeHints ---

func (this *Pagination) SizeHints() SizeHints {
	n := len(this.cells())
	w := float64(n) * paginationCellW
	return SizeHints{Width: w, Height: 32}
}

func (this *Pagination) EnumProperties(list core.IPropertyList) {
	list.AddProperty("总页数", this.TotalPages, this.SetTotalPages)
	list.AddProperty("当前页", this.CurrentPage, this.SetCurrentPage)
	list.AddProperty("相邻页数", this.SiblingCount, this.SetSiblingCount)
	list.AddProperty("边界页数", this.BoundaryCount, this.SetBoundaryCount)
}

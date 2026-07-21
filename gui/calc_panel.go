package gui

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// FormulaRow is one plain, already-formatted row for CalcPanel. The host owns
// all evaluation: it turns a backend formula (package calc) into these three
// display strings before handing a slice to the panel. Output is the target the
// formula writes, Expr the formula source, and Status a host-formatted result —
// "ok" for a clean evaluation or an error message otherwise. Keeping the row a
// bag of strings is what lets the panel stay decoupled from the calc backend (it
// never imports it) and GL-free testable.
type FormulaRow struct {
	Output string // target tag / cell the formula writes, e.g. "TIC-101.SP"
	Expr   string // formula source, e.g. "A + B * 2"
	Status string // host-formatted result: "ok", or an error like "div by zero"
}

// CalcPanel is a decoupled 公式 (formula/calc) operator panel: a scrollable list
// of host-supplied FormulaRows, each shown as "Output = Expr" with a status
// cell, plus a footer with 新增(Add) / 删除(Remove) actions. It holds nothing but
// plain view-model data fed through SetFormulas and emits the operator's intent
// through the Sig* callbacks: Add asks the host to create a formula, Remove
// carries the selected row's Output. The host wires those back to the calc
// store; the panel never imports it, so gui stays light and this file is GL-free
// unit-testable (only Draw touches the painter).
type CalcPanel struct {
	Widget

	formulas  []FormulaRow
	selected  int // index of the highlighted row, -1 when nothing is selected
	scrollY   float64
	rowHeight float64

	cbAdd    func(output, expr string)
	cbRemove func(output string)
}

func init() {
	core.RegisterFactory("gui.CalcPanel", core.TypeOf((*CalcPanel)(nil)))
}

// NewCalcPanel creates an empty formula panel with no selection.
func NewCalcPanel() *CalcPanel {
	p := new(CalcPanel)
	p.Init(p)
	return p
}

func (this *CalcPanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.selected = -1
}

// SetFormulas replaces the displayed formulas with a defensive copy of in.
// FormulaRow is a value type (all strings), so the shallow copy fully isolates
// the panel from later mutation of the caller's slice. The selection is reset (a
// new list invalidates any prior index), and the scroll offset is clamped to the
// new content rather than reset.
func (this *CalcPanel) SetFormulas(in []FormulaRow) {
	cp := make([]FormulaRow, len(in))
	copy(cp, in)
	this.formulas = cp
	this.selected = -1
	this.clampScroll()
	this.Self().Update()
}

// Formulas returns a defensive copy of the displayed formulas in order.
func (this *CalcPanel) Formulas() []FormulaRow {
	out := make([]FormulaRow, len(this.formulas))
	copy(out, this.formulas)
	return out
}

// Selected returns the index of the currently selected row, or -1 when no row is
// selected (or the selection has fallen out of range).
func (this *CalcPanel) Selected() int {
	if this.selected < 0 || this.selected >= len(this.formulas) {
		return -1
	}
	return this.selected
}

// SigAdd registers the callback fired when the operator clicks 新增(Add). For v1
// it passes empty output/expr — the host opens its own input to fill them in.
func (this *CalcPanel) SigAdd(fn func(output, expr string)) { this.cbAdd = fn }

// SigRemove registers the callback fired when the operator clicks 删除(Remove). It
// receives the selected row's Output; it does not fire when nothing is selected.
func (this *CalcPanel) SigRemove(fn func(output string)) { this.cbRemove = fn }

// selectedOutput returns the Output of the selected row and true, or ("", false)
// when no row is selected (or the selection has fallen out of range).
func (this *CalcPanel) selectedOutput() (string, bool) {
	if this.selected < 0 || this.selected >= len(this.formulas) {
		return "", false
	}
	return this.formulas[this.selected].Output, true
}

// calcStatusWarn reports whether a status cell should burn the warning colour:
// any non-empty status other than "ok". Pure — no theme or GL dependency — so
// the rule is unit-testable headless.
func calcStatusWarn(status string) bool {
	return status != "" && status != "ok"
}

// calcWarnColor tints a non-ok status cell amber, matching the alarm / eventlog
// panels' warning accent. Fixed (not theme-derived) so a bad formula reads the
// same in both light and dark chrome.
var calcWarnColor = paint.Color{R: 230, G: 180, B: 60, A: 255}

// --- Layout constants ---

const (
	calcHeaderH     = 22.0 // header band height
	calcButtonH     = 28.0 // footer action-button row height
	calcButtonCount = 2     // Add / Remove
)

const (
	calcBtnAdd = iota
	calcBtnRemove
)

var calcButtonLabels = [calcButtonCount]string{
	calcBtnAdd:    "新增(Add)",
	calcBtnRemove: "删除(Remove)",
}

// --- Drawing ---

// Draw renders a title/count header, the scrollable formula list with the
// selected row highlighted and a per-row status cell, and the footer
// action-button row. All colours come from the active Theme() (only the status
// warning accent is fixed) so the panel reads correctly in the dark IDE theme.
func (this *CalcPanel) Draw(g paint.Painter) {
	th := Theme()
	w, h := this.Size()

	// View background.
	g.SetBrush1(th.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := th.Font
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band: title + count.
	g.SetBrush1(th.FormColor)
	g.Rectangle(0, 0, w, calcHeaderH)
	g.Fill()
	g.SetBrush1(th.TextColor)
	g.DrawText1(8, fe.Ascent+4, "公式 Calc · "+strconv.Itoa(len(this.formulas)))

	// Scrollable formula list between the header and the footer, clipped so a
	// partially-scrolled top row cannot bleed into the header band.
	listTop := calcHeaderH
	listBottom := h - calcButtonH
	if listBottom > listTop && len(this.formulas) > 0 {
		rh := this.rowHeight
		startIdx := int(this.scrollY / rh)
		if startIdx < 0 {
			startIdx = 0
		}
		visibleCount := int((listBottom-listTop)/rh) + 2

		g.Save()
		g.Rectangle(0, listTop, w, listBottom-listTop)
		g.Clip()
		for i := startIdx; i < startIdx+visibleCount && i < len(this.formulas); i++ {
			y := listTop + float64(i)*rh - this.scrollY
			baseline := y + fe.Ascent + 4
			f := this.formulas[i]

			if i == this.selected {
				// HighLightColor is the theme's selection accent (blue in both modes).
				g.SetBrush1(th.HighLightColor)
				g.Rectangle(0, y, w, rh)
				g.Fill()
				g.SetBrush1(th.MenuActiveTextColor)
			} else {
				g.SetBrush1(th.TextColor)
			}
			g.DrawText1(10, baseline, f.Output+" = "+f.Expr)

			// Status cell, right-aligned. A non-ok/non-empty status burns amber;
			// "ok" stays muted so only problems draw the operator's eye.
			if f.Status != "" {
				ext := g.Font().TextExtents(f.Status)
				sx := w - 10 - ext.Width
				if calcStatusWarn(f.Status) {
					g.SetBrush1(calcWarnColor)
				} else if i == this.selected {
					g.SetBrush1(th.MenuActiveTextColor)
				} else {
					g.SetBrush1(th.MenuGrayTextColor)
				}
				g.DrawText1(sx, baseline, f.Status)
			}
		}
		g.Restore()
	}

	this.drawButtons(g, w, h)
}

// drawButtons paints the footer action-button row: a border-topped band split
// into calcButtonCount equal cells, each a rounded button face with a centred
// label. Colours are theme-derived so the row matches the dark chrome.
func (this *CalcPanel) drawButtons(g paint.Painter, w, h float64) {
	th := Theme()
	top := h - calcButtonH

	g.SetBrush1(th.FormColor)
	g.Rectangle(0, top, w, calcButtonH)
	g.Fill()
	g.SetPen1(th.BorderColor, 1)
	g.Line(0, top, w, top)
	g.Stroke()

	cell := w / float64(calcButtonCount)
	pad := 3.0
	for i := 0; i < calcButtonCount; i++ {
		bx := float64(i) * cell
		roundedRect(g, bx+pad, top+pad, cell-2*pad, calcButtonH-2*pad, 4)
		g.SetBrush1(th.FormLightColor)
		g.FillPreserve()
		g.SetPen1(th.BorderColor, 1)
		g.Stroke()

		label := calcButtonLabels[i]
		ext := g.Font().TextExtents(label)
		tx := bx + (cell-ext.Width)*0.5 - ext.XBearing
		ty := top + 0.5*(calcButtonH+ext.YBearing) - ext.YBearing
		g.SetBrush1(th.TextColor)
		g.DrawText1(tx, ty, label)
	}
}

// --- Events ---

// OnLeftDown routes a click: a hit in the footer band fires the matching action
// (Add unconditionally with empty strings, Remove on the selected row's Output);
// a hit in the list body selects that row. Clicks on the header or past the last
// row are ignored.
func (this *CalcPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	w, h := this.Size()

	if y >= h-calcButtonH {
		switch calcButtonAtX(x, w) {
		case calcBtnAdd:
			if this.cbAdd != nil {
				this.cbAdd("", "")
			}
		case calcBtnRemove:
			if out, ok := this.selectedOutput(); ok && this.cbRemove != nil {
				this.cbRemove(out)
			}
		}
		return
	}

	idx := this.rowAtY(y)
	if idx < 0 || idx >= len(this.formulas) {
		return
	}
	this.selected = idx
	this.Self().Update()
}

// OnMouseWheel scrolls the formula list vertically.
func (this *CalcPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to a formula-row index, accounting for the header
// band and the current scroll offset; it returns -1 when y lands on the header.
// Pure geometry (only scrollY and rowHeight), so it is unit-testable headless.
// The index may exceed the row count for a click below the last row — callers
// bound-check against len(formulas).
func (this *CalcPanel) rowAtY(y float64) int {
	if y < calcHeaderH {
		return -1
	}
	return int((y - calcHeaderH + this.scrollY) / this.rowHeight)
}

// calcButtonAtX maps an x coordinate to a footer button index (0..1) for a panel
// of width w, or -1 when x is outside the panel. The footer is split into
// calcButtonCount equal cells that tile the full width. Kept pure so the click
// routing is unit-testable headless.
func calcButtonAtX(x, w float64) int {
	if w <= 0 || x < 0 || x >= w {
		return -1
	}
	idx := int(x / (w / float64(calcButtonCount)))
	if idx >= calcButtonCount {
		idx = calcButtonCount - 1
	}
	return idx
}

// clampScroll pins scrollY within [0, maxScroll] for the current content and the
// viewport height left after the header and footer bands.
func (this *CalcPanel) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.formulas))*this.rowHeight - (h - calcHeaderH - calcButtonH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
}

func (this *CalcPanel) SizeHints() SizeHints {
	return SizeHints{MinWidth: 280, MinHeight: 120}
}

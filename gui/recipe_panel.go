package gui

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// RecipePanel is a 配方 (recipe) operator panel for SCADA / 组态 screens: a
// scrollable list of named recipes with a selectable row and a footer row of
// action buttons — 应用(Apply), 抓取(Capture), 保存(Save), 加载(Load).
//
// It is deliberately decoupled from the backend recipe package. The panel holds
// only a plain []string of recipe names fed via SetRecipes, and the operator's
// intent leaves through the Sig* callbacks: Apply/Capture carry the currently
// selected recipe name, Save/Load carry nothing. The host wires those callbacks
// to the recipe store; the panel never imports it, so gui stays light and this
// file is GL-free unit-testable (only Draw touches the painter).
type RecipePanel struct {
	Widget

	recipes   []string
	selected  int // index of the highlighted row, -1 when nothing is selected
	scrollY   float64
	rowHeight float64

	cbApply   func(name string)
	cbCapture func(name string)
	cbSave    func()
	cbLoad    func()
}

func init() {
	core.RegisterFactory("gui.RecipePanel", core.TypeOf((*RecipePanel)(nil)))
}

// NewRecipePanel creates an empty recipe panel with no selection.
func NewRecipePanel() *RecipePanel {
	p := new(RecipePanel)
	p.Init(p)
	return p
}

func (this *RecipePanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.selected = -1
}

// SetRecipes replaces the displayed recipe list with a defensive copy of in.
// The selection is reset (a new list invalidates any prior index), and the
// scroll offset is clamped to the new content rather than reset.
func (this *RecipePanel) SetRecipes(in []string) {
	cp := make([]string, len(in))
	copy(cp, in)
	this.recipes = cp
	this.selected = -1
	this.clampScroll()
	this.Self().Update()
}

// Recipes returns a defensive copy of the displayed recipe names in order.
func (this *RecipePanel) Recipes() []string {
	out := make([]string, len(this.recipes))
	copy(out, this.recipes)
	return out
}

// Selected returns the name of the currently selected recipe, or "" when no row
// is selected (or the selection has fallen out of range).
func (this *RecipePanel) Selected() string {
	if this.selected < 0 || this.selected >= len(this.recipes) {
		return ""
	}
	return this.recipes[this.selected]
}

// SigApply registers the callback fired when the operator clicks 应用(Apply). It
// receives the selected recipe name; it does not fire when nothing is selected.
func (this *RecipePanel) SigApply(fn func(name string)) { this.cbApply = fn }

// SigCapture registers the callback fired when the operator clicks 抓取(Capture).
// It receives the selected recipe name; it does not fire when nothing is selected.
func (this *RecipePanel) SigCapture(fn func(name string)) { this.cbCapture = fn }

// SigSave registers the callback fired when the operator clicks 保存(Save).
func (this *RecipePanel) SigSave(fn func()) { this.cbSave = fn }

// SigLoad registers the callback fired when the operator clicks 加载(Load).
func (this *RecipePanel) SigLoad(fn func()) { this.cbLoad = fn }

// --- Layout constants ---

const (
	recipeHeaderH     = 22.0 // header band height
	recipeButtonH     = 28.0 // footer action-button row height
	recipeButtonCount = 4    // Apply / Capture / Save / Load
)

const (
	recipeBtnApply = iota
	recipeBtnCapture
	recipeBtnSave
	recipeBtnLoad
)

var recipeButtonLabels = [recipeButtonCount]string{
	recipeBtnApply:   "应用(Apply)",
	recipeBtnCapture: "抓取(Capture)",
	recipeBtnSave:    "保存(Save)",
	recipeBtnLoad:    "加载(Load)",
}

// --- Drawing ---

// Draw renders a title/count header, the scrollable recipe list with the
// selected row highlighted, and the footer action-button row. All colours come
// from the active Theme() so the panel reads correctly in the dark IDE theme.
func (this *RecipePanel) Draw(g paint.Painter) {
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
	g.Rectangle(0, 0, w, recipeHeaderH)
	g.Fill()
	g.SetBrush1(th.TextColor)
	g.DrawText1(8, fe.Ascent+4, "配方 Recipes · "+strconv.Itoa(len(this.recipes)))

	// Scrollable name list between the header and the footer, clipped so a
	// partially-scrolled top row cannot bleed into the header band.
	listTop := recipeHeaderH
	listBottom := h - recipeButtonH
	if listBottom > listTop && len(this.recipes) > 0 {
		rh := this.rowHeight
		startIdx := int(this.scrollY / rh)
		if startIdx < 0 {
			startIdx = 0
		}
		visibleCount := int((listBottom-listTop)/rh) + 2

		g.Save()
		g.Rectangle(0, listTop, w, listBottom-listTop)
		g.Clip()
		for i := startIdx; i < startIdx+visibleCount && i < len(this.recipes); i++ {
			y := listTop + float64(i)*rh - this.scrollY
			baseline := y + fe.Ascent + 4
			if i == this.selected {
				// HighLightColor is the theme's selection accent (blue in both modes).
				g.SetBrush1(th.HighLightColor)
				g.Rectangle(0, y, w, rh)
				g.Fill()
				g.SetBrush1(th.MenuActiveTextColor)
			} else {
				g.SetBrush1(th.TextColor)
			}
			g.DrawText1(10, baseline, this.recipes[i])
		}
		g.Restore()
	}

	this.drawButtons(g, w, h)
}

// drawButtons paints the footer action-button row: a border-topped band split
// into recipeButtonCount equal cells, each a rounded button face with a centred
// label. Colours are theme-derived so the row matches the dark chrome.
func (this *RecipePanel) drawButtons(g paint.Painter, w, h float64) {
	th := Theme()
	top := h - recipeButtonH

	g.SetBrush1(th.FormColor)
	g.Rectangle(0, top, w, recipeButtonH)
	g.Fill()
	g.SetPen1(th.BorderColor, 1)
	g.Line(0, top, w, top)
	g.Stroke()

	cell := w / float64(recipeButtonCount)
	pad := 3.0
	for i := 0; i < recipeButtonCount; i++ {
		bx := float64(i) * cell
		roundedRect(g, bx+pad, top+pad, cell-2*pad, recipeButtonH-2*pad, 4)
		g.SetBrush1(th.FormLightColor)
		g.FillPreserve()
		g.SetPen1(th.BorderColor, 1)
		g.Stroke()

		label := recipeButtonLabels[i]
		ext := g.Font().TextExtents(label)
		tx := bx + (cell-ext.Width)*0.5 - ext.XBearing
		ty := top + 0.5*(recipeButtonH+ext.YBearing) - ext.YBearing
		g.SetBrush1(th.TextColor)
		g.DrawText1(tx, ty, label)
	}
}

// --- Events ---

// OnLeftDown routes a click: a hit in the footer band fires the matching action
// (Apply/Capture on the selected recipe, Save/Load unconditionally); a hit in
// the list body selects that row. Clicks on the header or past the last row are
// ignored.
func (this *RecipePanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	w, h := this.Size()

	if y >= h-recipeButtonH {
		switch recipeButtonAtX(x, w) {
		case recipeBtnApply:
			if name := this.Selected(); name != "" && this.cbApply != nil {
				this.cbApply(name)
			}
		case recipeBtnCapture:
			if name := this.Selected(); name != "" && this.cbCapture != nil {
				this.cbCapture(name)
			}
		case recipeBtnSave:
			if this.cbSave != nil {
				this.cbSave()
			}
		case recipeBtnLoad:
			if this.cbLoad != nil {
				this.cbLoad()
			}
		}
		return
	}

	idx := this.rowAtY(y)
	if idx < 0 || idx >= len(this.recipes) {
		return
	}
	this.selected = idx
	this.Self().Update()
}

// OnMouseWheel scrolls the recipe list vertically.
func (this *RecipePanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to a recipe-row index, accounting for the header
// band and the current scroll offset; it returns -1 when y lands on the header.
// Pure geometry (only scrollY and rowHeight), so it is unit-testable headless.
// The index may exceed the row count for a click below the last row — callers
// bound-check against len(recipes).
func (this *RecipePanel) rowAtY(y float64) int {
	if y < recipeHeaderH {
		return -1
	}
	return int((y - recipeHeaderH + this.scrollY) / this.rowHeight)
}

// recipeButtonAtX maps an x coordinate to a footer button index (0..3) for a
// panel of width w, or -1 when x is outside the panel. The footer is split into
// recipeButtonCount equal cells that tile the full width. Kept pure so the click
// routing is unit-testable headless.
func recipeButtonAtX(x, w float64) int {
	if w <= 0 || x < 0 || x >= w {
		return -1
	}
	idx := int(x / (w / float64(recipeButtonCount)))
	if idx >= recipeButtonCount {
		idx = recipeButtonCount - 1
	}
	return idx
}

// clampScroll pins scrollY within [0, maxScroll] for the current content and the
// viewport height left after the header and footer bands.
func (this *RecipePanel) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.recipes))*this.rowHeight - (h - recipeHeaderH - recipeButtonH)
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

func (this *RecipePanel) SizeHints() SizeHints {
	return SizeHints{MinWidth: 280, MinHeight: 120}
}

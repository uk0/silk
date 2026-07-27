package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.TypeHierarchyPanel", gui.TypeOf(TypeHierarchyPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TypeHierarchyPanel",
		Name: "类型层次",
		Icon: "tree-view",
		Desc: "父类型 / 子类型 / 实现 导航",
	})
}

// TypeHierarchyPanel is the type-hierarchy tool view, modelled on Qt Creator's
// "Type Hierarchy" pane plus its "Find Implementations": a direction selector
// (父类型 / 子类型 / 实现) over an indented, lazily-expanded tree of types.
//
// Like the sibling ReferencesPanel it is a pure display/interaction widget that
// never talks to gopls. All state lives in TypeHierarchyModel; the panel adds
// only geometry and three intents:
//
//   - SigMode      — the user picked a different direction; the tree was reset
//     and the host must re-resolve the hierarchy for it.
//   - Model().SigExpand — the user expanded a node whose relatives were never
//     fetched; the host resolves them and calls Model().SetChildren.
//   - SigActivate  — the user clicked a row with a source location; the host
//     opens file:line.
type TypeHierarchyPanel struct {
	gui.Widget

	model     *TypeHierarchyModel
	scrollY   float64
	hoverIdx  int
	selected  int // flat index of the last-clicked row, -1 when none
	rowHeight float64

	cbMode     func(mode string)
	cbActivate func(file string, line int)
}

// NewTypeHierarchyPanel creates an empty panel in supertypes mode.
func NewTypeHierarchyPanel() *TypeHierarchyPanel {
	p := new(TypeHierarchyPanel)
	p.Init(p)
	return p
}

func (this *TypeHierarchyPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.model = NewTypeHierarchyModel()
	this.rowHeight = 22
	this.hoverIdx = -1
	this.selected = -1
}

// Model returns the tree behind the panel; the host pushes results in through
// it (SetTargets / SetChildren) and wires the lazy fetch via its SigExpand.
func (this *TypeHierarchyPanel) Model() *TypeHierarchyModel { return this.model }

// SetTargets installs the host's resolved candidates and resets the view. See
// TypeHierarchyModel.SetTargets for the multiple-targets root.
func (this *TypeHierarchyPanel) SetTargets(targets []*TypeNode) {
	this.model.SetTargets(targets)
	this.resetView()
	this.Self().Update()
}

// SetMode switches direction programmatically. It resets the tree exactly as a
// click does but does NOT fire SigMode — that signal reports user intent, and
// a host that set the mode itself already knows.
func (this *TypeHierarchyPanel) SetMode(mode string) bool {
	if !this.model.SetMode(mode) {
		return false
	}
	this.resetView()
	this.Self().Update()
	return true
}

// Mode returns the active direction.
func (this *TypeHierarchyPanel) Mode() string { return this.model.Mode() }

// SigMode registers the callback fired when the user picks a direction in the
// selector. The tree has already been cleared; the host re-resolves for the new
// mode and calls SetTargets.
func (this *TypeHierarchyPanel) SigMode(fn func(mode string)) { this.cbMode = fn }

// SigActivate registers the callback fired when the user clicks a row that has
// a source location. It receives the file and the 1-based line.
func (this *TypeHierarchyPanel) SigActivate(fn func(file string, line int)) {
	this.cbActivate = fn
}

// resetView drops scroll/hover/selection after the row list changes wholesale.
func (this *TypeHierarchyPanel) resetView() {
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
}

// --- Geometry (pure, so hit-testing is unit-testable headless) ---

const (
	typeHierHeaderH  = 22.0                               // title band
	typeHierModeBarH = 24.0                               // direction-selector band
	typeHierRowsTop  = typeHierHeaderH + typeHierModeBarH // first row's top edge
	typeHierModePadX = 8.0                                // left inset of the first mode button
	typeHierModeBtnW = 64.0                               // mode button width
	typeHierModeGap  = 4.0                                // gap between mode buttons
	typeHierRowPadX  = 8.0                                // left inset of a depth-0 row
	typeHierIndent   = 16.0                               // per-depth indent
	typeHierTwistyW  = 12.0                               // expand/collapse hit width
)

// typeHierModeX is the left edge of the i-th mode button.
func typeHierModeX(i int) float64 {
	return typeHierModePadX + float64(i)*(typeHierModeBtnW+typeHierModeGap)
}

// typeHierModeAt maps a point to an index into typeHierarchyModes, or -1 when it
// misses every button — including the title band above and the rows below.
func typeHierModeAt(x, y float64) int {
	if y < typeHierHeaderH || y >= typeHierRowsTop {
		return -1
	}
	for i := range typeHierarchyModes {
		x0 := typeHierModeX(i)
		if x >= x0 && x < x0+typeHierModeBtnW {
			return i
		}
	}
	return -1
}

// typeHierRowAtY maps a y coordinate to a row index for rows starting at
// topOffset, count rows of height rowH. The caller folds the scroll offset into
// y. It returns -1 above the rows, past the last row, or for a degenerate rowH.
func typeHierRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// typeHierRowX is the left edge of a row's content at the given depth — where
// the twisty sits, with the glyph and name after it.
func typeHierRowX(depth int) float64 {
	return typeHierRowPadX + float64(depth)*typeHierIndent
}

// typeHierTwistyHit reports whether x lands on the expand/collapse triangle of a
// row at the given depth. A hit toggles instead of navigating.
func typeHierTwistyHit(x float64, depth int) bool {
	x0 := typeHierRowX(depth)
	return x >= x0 && x < x0+typeHierTwistyW
}

// typeHierKindColor tints the kind glyph: interfaces blue, structs green,
// methods purple, the synthetic root grey.
func typeHierKindColor(kind string) paint.Color {
	switch kind {
	case TypeKindInterface:
		return paint.Color{R: 70, G: 130, B: 200, A: 255}
	case TypeKindStruct:
		return paint.Color{R: 70, G: 160, B: 100, A: 255}
	case TypeKindMethod:
		return paint.Color{R: 150, G: 110, B: 190, A: 255}
	}
	return paint.Color{R: 140, G: 140, B: 150, A: 255}
}

// --- Drawing ---

// Draw renders the title, the three direction buttons, and one indented row per
// visible node: a twisty when it has (or may have) relatives, a kind glyph, the
// name, and a dimmed "pkg · file:line" locator. Cyclic stubs are suffixed with
// a recursion marker.
func (this *TypeHierarchyPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Title band.
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, typeHierHeaderH)
	g.Fill()
	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, typeHierHeaderH-5, typeHierarchyTitle(this.model.Mode(), this.model.RowCount()))

	// Direction selector.
	modeFont := paint.NewFont(t.Font.Family(), 11, false, false)
	g.SetFont(modeFont)
	for i, mode := range typeHierarchyModes {
		x0 := typeHierModeX(i)
		active := mode == this.model.Mode()
		if active {
			g.SetBrush1(t.HighLightColor)
		} else {
			g.SetBrush1(paint.Color{R: 226, G: 230, B: 238, A: 255})
		}
		g.Rectangle(x0, typeHierHeaderH+3, typeHierModeBtnW, typeHierModeBarH-6)
		g.Fill()
		if active {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.DrawText1(x0+8, typeHierRowsTop-8, typeHierarchyModeLabel(mode))
	}
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, typeHierRowsTop)
	g.LineTo(w, typeHierRowsTop)
	g.Stroke()

	rows := this.model.Rows()
	nameFont := paint.NewFont(t.Font.Family(), 11, false, false)
	if len(rows) == 0 {
		g.SetFont(nameFont)
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 200})
		g.DrawText1(8, typeHierRowsTop+20, "No hierarchy")
		return
	}

	glyphFont := paint.NewFont(t.Font.Family(), 10, true, false)
	detailFont := paint.NewFont(t.Font.Family(), 10, false, false)
	fe := nameFont.FontExtents()
	rh := this.rowHeight

	for i, row := range rows {
		rowY := typeHierRowsTop + float64(i)*rh - this.scrollY
		if rowY+rh < typeHierRowsTop || rowY > h {
			continue
		}

		// Selection wins over hover wins over the alternating stripe.
		if i == this.selected {
			g.SetBrush1(paint.Color{R: 51, G: 120, B: 215, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		} else if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 245, G: 247, B: 250, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		textY := rowY + fe.Ascent + (rh-fe.Ascent-fe.Descent)/2
		x := typeHierRowX(row.Depth)

		// Twisty: filled triangle, down when expanded, right when collapsed.
		if !row.Cyclic && typeNodeExpandable(row.Node) {
			cy := rowY + rh/2
			if row.Node.Expanded {
				g.MoveTo(x, cy-3)
				g.LineTo(x+7, cy-3)
				g.LineTo(x+3.5, cy+3)
			} else {
				g.MoveTo(x, cy-4)
				g.LineTo(x+6, cy)
				g.LineTo(x, cy+4)
			}
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			g.Fill()
		}
		x += typeHierTwistyW

		// Kind glyph.
		g.SetFont(glyphFont)
		g.SetBrush1(typeHierKindColor(row.Node.Kind))
		g.DrawText1(x, textY, typeHierKindGlyph(row.Node.Kind))
		x += 14

		// Name, plus a recursion marker on a repeated ancestor.
		name := row.Node.Name
		if row.Cyclic {
			name += " ↻"
		}
		g.SetFont(nameFont)
		if i == this.selected {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.DrawText1(x, textY, name)
		x += nameFont.TextExtents(name).Width + 10

		// Dimmed locator.
		if detail := typeHierRowDetail(row.Node); detail != "" {
			g.SetFont(detailFont)
			if i == this.selected {
				g.SetBrush1(paint.Color{R: 220, G: 230, B: 245, A: 255})
			} else {
				g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 255})
			}
			g.DrawText1(x, textY, detail)
		}
	}
}

// --- Events ---

// OnLeftDown routes a click: the direction selector switches mode (resetting the
// tree and firing SigMode), a twisty expands/collapses, and any other row hit
// fires SigActivate. Rows without a source location — the synthetic
// multiple-targets root — toggle instead of navigating. The title band is inert.
func (this *TypeHierarchyPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if idx := typeHierModeAt(x, y); idx >= 0 {
		mode := typeHierarchyModes[idx]
		if !this.model.SetMode(mode) {
			return
		}
		this.resetView()
		this.Self().Update()
		if this.cbMode != nil {
			this.cbMode(mode)
		}
		return
	}

	i := this.rowAt(y)
	row, ok := this.model.RowAt(i)
	if !ok {
		return
	}
	this.selected = i

	if typeHierTwistyHit(x, row.Depth) || row.Node.File == "" {
		this.model.ToggleRow(i)
		this.Self().Update()
		return
	}
	this.Self().Update()
	if this.cbActivate != nil {
		this.cbActivate(row.Node.File, row.Node.Line)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *TypeHierarchyPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *TypeHierarchyPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the tree vertically, clamped to the content.
func (this *TypeHierarchyPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(this.model.RowCount())*this.rowHeight - (h - typeHierRowsTop)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate to a flat row index, folding in the scroll offset.
func (this *TypeHierarchyPanel) rowAt(y float64) int {
	return typeHierRowAtY(y+this.scrollY, typeHierRowsTop, this.rowHeight, this.model.RowCount())
}

func (this *TypeHierarchyPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 220, MinHeight: 100}
}

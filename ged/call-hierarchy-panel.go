package ged

import (
	"path/filepath"
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.CallHierarchyPanel", gui.TypeOf(CallHierarchyPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.CallHierarchyPanel",
		Name: "调用层次 / Call Hierarchy",
		Icon: "tree-view",
		Desc: "符号的调用者 / 被调用者树, 按需逐层展开",
	})
}

// CallHierarchyPanel is the Qt Creator-style call hierarchy view: a tree
// of callers (incoming) or callees (outgoing) around one root symbol, with
// a direction toggle in the header and one level resolved at a time as the
// user opens nodes.
//
// Like ReferencesPanel and TodoPanel it is a pure display/interaction
// widget — it never talks to gopls. All the tree logic lives in
// CallHierarchyModel (lazy expansion, cycle detection, stale-fetch
// rejection); the panel flattens the model into rows, renders them with
// indentation and expander triangles, and reports interaction back:
//
//	SigExpand    — a node was opened and needs its children (host fetches,
//	               then calls SetChildren)
//	SigActivate  — a row was clicked (host opens file:line)
//	SigDirection — the header toggle flipped incoming/outgoing (host
//	               re-resolves the first level)
type CallHierarchyPanel struct {
	gui.Widget

	model     *CallHierarchyModel
	rows      []CallRow
	scrollY   float64
	hoverIdx  int
	selected  int // index of the last-activated row, -1 when none
	rowHeight float64

	cbActivate  func(file string, line int)
	cbDirection func(incoming bool)
}

// NewCallHierarchyPanel creates an empty call hierarchy panel.
func NewCallHierarchyPanel() *CallHierarchyPanel {
	p := new(CallHierarchyPanel)
	p.Init(p)
	return p
}

func (this *CallHierarchyPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.selected = -1
	this.model = NewCallHierarchyModel()
}

// Model returns the underlying tree so the host can drive it directly
// (SigExpand, Expand, Cancel, …). Mutating it needs a Refresh() afterwards
// for the panel to pick up the new row list.
func (this *CallHierarchyPanel) Model() *CallHierarchyModel { return this.model }

// SetRoot replaces the tree with a new root symbol, invalidating any
// in-flight fetch, and resets the view.
func (this *CallHierarchyPanel) SetRoot(root *CallNode) {
	this.model.SetRoot(root)
	this.reset()
}

// Clear empties the panel and invalidates any in-flight fetch.
func (this *CallHierarchyPanel) Clear() {
	this.model.Clear()
	this.reset()
}

// SetChildren forwards one resolved level to the model and refreshes the
// rows. It returns false when the fetch was stale (see
// CallHierarchyModel.SetChildren), in which case nothing changes.
func (this *CallHierarchyPanel) SetChildren(node *CallNode, children []*CallNode) bool {
	if !this.model.SetChildren(node, children) {
		return false
	}
	this.Refresh()
	return true
}

// Refresh re-flattens the model into rows and repaints. Call it after
// driving the model directly.
func (this *CallHierarchyPanel) Refresh() {
	this.rows = this.model.Rows()
	if this.selected >= len(this.rows) {
		this.selected = -1
	}
	if this.hoverIdx >= len(this.rows) {
		this.hoverIdx = -1
	}
	this.Self().Update()
}

// Rows returns a defensive copy of the flattened rows in display order.
func (this *CallHierarchyPanel) Rows() []CallRow {
	out := make([]CallRow, len(this.rows))
	copy(out, this.rows)
	return out
}

// Incoming reports whether the panel is showing callers (true) or callees.
func (this *CallHierarchyPanel) Incoming() bool { return this.model.Incoming() }

// SetIncoming switches direction programmatically. It resets the tree to
// the bare root (the host must re-resolve) but does NOT fire SigDirection
// — the signal reports user interaction, and the caller here already knows.
func (this *CallHierarchyPanel) SetIncoming(incoming bool) {
	d := CallOutgoing
	if incoming {
		d = CallIncoming
	}
	if this.model.SetDirection(d) {
		this.reset()
	}
}

// ToggleDirection flips incoming/outgoing and fires SigDirection so the
// host can re-resolve the first level. This is what the header toggle does.
func (this *CallHierarchyPanel) ToggleDirection() {
	incoming := !this.model.Incoming()
	this.SetIncoming(incoming)
	if this.cbDirection != nil {
		this.cbDirection(incoming)
	}
}

// SigExpand registers the callback fired when a node needs its children
// resolved. The host fetches them and answers with SetChildren.
func (this *CallHierarchyPanel) SigExpand(fn func(node *CallNode)) {
	this.model.SigExpand(fn)
}

// SigActivate registers the callback fired when the user clicks a row (not
// its expander). It receives the node's file and 1-based line — the host
// opens file:line in the editor.
func (this *CallHierarchyPanel) SigActivate(fn func(file string, line int)) {
	this.cbActivate = fn
}

// SigDirection registers the callback fired when the user flips the
// direction toggle. It receives the new direction: true == incoming.
func (this *CallHierarchyPanel) SigDirection(fn func(incoming bool)) {
	this.cbDirection = fn
}

// reset re-flattens the rows and drops the view state (scroll, hover,
// selection) after the tree was replaced wholesale.
func (this *CallHierarchyPanel) reset() {
	this.rows = this.model.Rows()
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// --- Pure helpers (GL-free, unit-testable) ---

const (
	callHierarchyHeaderH = 22.0
	callRowIndentBase    = 8.0
	callRowIndentStep    = 14.0
	callExpanderSize     = 10.0

	callToggleX = 6.0
	callToggleY = 3.0
	callToggleW = 96.0
	callToggleH = 16.0
)

// callRowAtY maps a y coordinate to a row index for a list whose rows
// start at topOffset, with count rows of height rowH. The caller folds the
// scroll offset into y before calling. It returns -1 when y lands above
// the rows (the header band), past the last row, or when rowH is
// degenerate. Pure so the hit-test needs no widget or GL. (Named
// callRowAtY, not rowAtY, because git-changes-panel.go already owns a
// package-level rowAtY — same namespacing as references-panel.go's
// refRowAtY.)
func callRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// callRowIndent returns the x offset of a row's expander box for a given
// tree depth; the row's text starts one expander width further right.
func callRowIndent(depth int) float64 {
	if depth < 0 {
		depth = 0
	}
	return callRowIndentBase + float64(depth)*callRowIndentStep
}

// callExpanderHit reports whether x lands on the expander triangle of a
// row at the given depth. Clicks there toggle the node; clicks anywhere
// else on the row activate it. Depth-only (no y) because the caller has
// already resolved which row y belongs to.
func callExpanderHit(x float64, depth int) bool {
	left := callRowIndent(depth)
	return x >= left && x < left+callExpanderSize
}

// callToggleHit reports whether a point lands on the header's direction
// toggle button. The button is pinned to the left of the header so the
// hit-test stays independent of the panel width (and of font metrics).
func callToggleHit(x, y float64) bool {
	return x >= callToggleX && x < callToggleX+callToggleW &&
		y >= callToggleY && y < callToggleY+callToggleH
}

// callDirectionLabel renders the toggle's caption for the active direction.
func callDirectionLabel(incoming bool) string {
	if incoming {
		return "调用者 / Incoming"
	}
	return "被调用 / Outgoing"
}

// callHierarchyTitle renders the header caption: the root symbol's name
// when there is one, otherwise the bare panel title.
func callHierarchyTitle(root *CallNode) string {
	if root == nil || root.Name == "" {
		return "调用层次 / Call Hierarchy"
	}
	return "调用层次: " + root.Name
}

// callRowLocator formats a node's right-hand locator as "basename:line"
// (e.g. "frame.go:42"), dropping the directory so deep paths stay
// scannable. Same convention as refRowLabel.
func callRowLocator(n *CallNode) string {
	if n == nil || n.File == "" {
		return ""
	}
	return filepath.Base(n.File) + ":" + strconv.Itoa(n.Line)
}

// callRowName renders a node's primary text: its name, with a cycle marker
// appended when the node closes a recursion.
func callRowName(n *CallNode) string {
	if n == nil {
		return ""
	}
	if n.Recursive {
		return n.Name + " ↻"
	}
	return n.Name
}

// --- Drawing ---

// Draw renders the header (direction toggle + root caption) and one row
// per visible node: an expander triangle at the node's indentation, the
// symbol name, its dimmed detail, and a "basename:line" locator flushed
// right.
func (this *CallHierarchyPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont(t.Font.Family(), 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band: direction toggle then the root caption.
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, callHierarchyHeaderH)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.MoveTo(0, callHierarchyHeaderH)
	g.LineTo(w, callHierarchyHeaderH)
	g.Stroke()

	g.SetBrush1(t.HighLightColor)
	g.Rectangle(callToggleX, callToggleY, callToggleW, callToggleH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.DrawText1(callToggleX+6, callToggleY+fe.Ascent+1, callDirectionLabel(this.model.Incoming()))

	g.SetBrush1(t.TextColor)
	g.DrawText1(callToggleX+callToggleW+10, fe.Ascent+3, callHierarchyTitle(this.model.Root()))

	if len(this.rows) == 0 {
		return
	}

	dim := t.TextColor
	dim.A = 150
	accent := t.HighLightColor
	hover := t.HighLightColor
	hover.A = 40

	rh := this.rowHeight
	areaTop := callHierarchyHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.rows); i++ {
		row := this.rows[i]
		if row.Node == nil {
			continue
		}
		y := areaTop + float64(i)*rh - this.scrollY

		// Selection wins over hover.
		textColor := t.TextColor
		if i == this.selected {
			g.SetBrush1(accent)
			g.Rectangle(0, y, w, rh)
			g.Fill()
			textColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
		} else if i == this.hoverIdx {
			g.SetBrush1(hover)
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		x := callRowIndent(row.Depth)

		// Expander triangle: down when open, right when closed.
		if row.Expandable {
			cx, cy := x+1, y+rh/2
			if row.Node.Expanded {
				g.MoveTo(cx, cy-2)
				g.LineTo(cx+7, cy-2)
				g.LineTo(cx+3.5, cy+3)
			} else {
				g.MoveTo(cx, cy-4)
				g.LineTo(cx+6, cy)
				g.LineTo(cx, cy+4)
			}
			g.SetBrush1(textColor)
			g.Fill()
		}
		x += callExpanderSize

		// Symbol name (plus the cycle marker for a recursive node).
		name := callRowName(row.Node)
		g.SetBrush1(textColor)
		g.DrawText1(x, y+fe.Ascent+2, name)
		x += font.TextExtents(name).Width + 8

		// Dimmed detail after the name.
		if row.Node.Detail != "" {
			if i == this.selected {
				g.SetBrush1(textColor)
			} else {
				g.SetBrush1(dim)
			}
			g.DrawText1(x, y+fe.Ascent+2, row.Node.Detail)
		}

		// Locator flushed right.
		if loc := callRowLocator(row.Node); loc != "" {
			lw := font.TextExtents(loc).Width
			if i == this.selected {
				g.SetBrush1(textColor)
			} else {
				g.SetBrush1(dim)
			}
			g.DrawText1(w-lw-8, y+fe.Ascent+2, loc)
		}
	}
}

// --- Events ---

// OnLeftDown routes a click: the header's direction toggle flips the
// direction, an expander triangle opens/closes that node (fetching its
// children on first open), and anywhere else on a row activates it.
func (this *CallHierarchyPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if y < callHierarchyHeaderH {
		if callToggleHit(x, y) {
			this.ToggleDirection()
		}
		return
	}

	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	row := this.rows[idx]
	if row.Node == nil {
		return
	}

	if row.Expandable && callExpanderHit(x, row.Depth) {
		this.model.Toggle(row.Node)
		this.Refresh()
		return
	}

	this.selected = idx
	this.Self().Update()
	if this.cbActivate != nil {
		this.cbActivate(row.Node.File, row.Node.Line)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *CallHierarchyPanel) OnMouseMove(x, y float64) {
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
func (this *CallHierarchyPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the tree vertically, clamped to the content.
func (this *CallHierarchyPanel) OnMouseWheel(x, y, z float64) {
	_, h := this.Size()
	this.scrollY = clampScroll(this.scrollY-z*3*this.rowHeight,
		float64(len(this.rows))*this.rowHeight, h-callHierarchyHeaderH)
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a row index, or -1 when
// y lands on the header band or past the last row. It folds the scroll
// offset into y and defers to the pure callRowAtY helper.
func (this *CallHierarchyPanel) rowAt(y float64) int {
	return callRowAtY(y+this.scrollY, callHierarchyHeaderH, this.rowHeight, len(this.rows))
}

func (this *CallHierarchyPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 220, MinHeight: 80}
}

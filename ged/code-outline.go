package ged

import (
	"math"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.CodeOutlinePanel", gui.TypeOf(CodeOutlinePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.CodeOutlinePanel",
		Name: "大纲",
		Icon: "tree",
		Desc: "代码大纲面板",
	})
}

// outlineNode represents one entry in the outline tree.
type outlineNode struct {
	symbol   gui.CodeSymbol
	children []outlineNode
	depth    int
	expanded bool
}

// CodeOutlinePanel is a persistent symbol tree panel that shows all
// functions, types, variables, and constants from the active code editor.
type CodeOutlinePanel struct {
	gui.Widget

	symbols     []gui.CodeSymbol
	tree        []outlineNode
	flatList    []outlineNode
	hoverIdx    int
	selectedIdx int
	scrollY     float64
	rowHeight   float64
	editor      *gui.CodeEditor
	cbNavigate  func(line int)

	// cache: last known text hash to avoid re-parsing when content hasn't changed
	lastTextLen int
	lastLineCount int
}

func NewCodeOutlinePanel() *CodeOutlinePanel {
	p := new(CodeOutlinePanel)
	p.Init(p)
	return p
}

func (this *CodeOutlinePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.hoverIdx = -1
	this.selectedIdx = -1
}

// SetEditor binds the outline panel to a code editor and refreshes symbols.
func (this *CodeOutlinePanel) SetEditor(editor *gui.CodeEditor) {
	this.editor = editor
	this.RefreshSymbols()
	this.Self().Update()
}

// SetNavigateCallback sets the callback invoked when a symbol is clicked.
func (this *CodeOutlinePanel) SetNavigateCallback(fn func(line int)) {
	this.cbNavigate = fn
}

// RefreshSymbols re-parses symbols from the current editor.
func (this *CodeOutlinePanel) RefreshSymbols() {
	this.symbols = nil
	this.tree = nil
	this.flatList = nil

	if this.editor == nil {
		return
	}

	this.symbols = this.editor.ParseSymbols()
	this.buildTree()
	this.rebuildFlatList()
}

// buildTree organizes symbols into a tree: types group their methods.
func (this *CodeOutlinePanel) buildTree() {
	this.tree = nil

	// First pass: collect types
	typeMap := make(map[string]*outlineNode)
	var topLevel []outlineNode

	for _, sym := range this.symbols {
		if sym.Kind == gui.SymType {
			node := outlineNode{
				symbol:   sym,
				depth:    0,
				expanded: true,
			}
			topLevel = append(topLevel, node)
			typeMap[sym.Name] = &topLevel[len(topLevel)-1]
		}
	}

	// Second pass: group methods under their receiver types
	for _, sym := range this.symbols {
		if sym.Kind == gui.SymMethod && sym.Receiver != "" {
			child := outlineNode{
				symbol: sym,
				depth:  1,
			}
			if parent, ok := typeMap[sym.Receiver]; ok {
				parent.children = append(parent.children, child)
				continue
			}
		}
		// Non-method, or method without matching type: add to top level
		if sym.Kind != gui.SymType {
			topLevel = append(topLevel, outlineNode{
				symbol:   sym,
				depth:    0,
				expanded: true,
			})
		}
	}

	this.tree = topLevel
}

// rebuildFlatList flattens the tree respecting expanded state.
func (this *CodeOutlinePanel) rebuildFlatList() {
	this.flatList = nil
	for _, node := range this.tree {
		this.flatList = append(this.flatList, node)
		if node.expanded && len(node.children) > 0 {
			for _, child := range node.children {
				child.depth = 1
				this.flatList = append(this.flatList, child)
			}
		}
	}
}

// currentSymbolIndex returns the flat list index of the symbol containing the
// editor cursor, or -1.
func (this *CodeOutlinePanel) currentSymbolIndex() int {
	if this.editor == nil || len(this.flatList) == 0 {
		return -1
	}
	cursorLine := this.editor.CursorLine()
	bestIdx := -1
	for i, node := range this.flatList {
		if node.symbol.Line <= cursorLine {
			bestIdx = i
		}
	}
	return bestIdx
}

// Draw renders the code outline panel.
func (this *CodeOutlinePanel) Draw(g paint.Painter) {
	// Auto-refresh when content changes
	if this.editor != nil {
		lines := this.editor.Lines()
		lineCount := len(lines)
		textLen := 0
		for _, l := range lines {
			textLen += len(l)
		}
		if textLen != this.lastTextLen || lineCount != this.lastLineCount {
			this.lastTextLen = textLen
			this.lastLineCount = lineCount
			this.RefreshSymbols()
		}
	}

	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := 22.0
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headerH-5, "大纲")

	if len(this.flatList) == 0 {
		normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
		g.SetFont(normalFont)
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 200})
		g.DrawText1(8, headerH+20, "No symbols")
		return
	}

	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	kindFont := paint.NewFont(t.Font.Family(), 10, true, false)
	rh := this.rowHeight
	startY := headerH - this.scrollY
	curIdx := this.currentSymbolIndex()

	for i, node := range this.flatList {
		rowY := startY + float64(i)*rh
		if rowY+rh < headerH || rowY > h {
			continue
		}

		indent := float64(node.depth) * 16.0
		textX := 8 + indent

		// Current symbol highlight (blue background)
		if i == curIdx {
			g.SetBrush1(paint.Color{R: 40, G: 80, B: 160, A: 60})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Selected highlight
		if i == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 51, G: 120, B: 215, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		} else if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Expand/collapse triangle for type nodes with children
		if node.depth == 0 && len(node.children) > 0 {
			triX := textX
			triY := rowY + rh/2
			if node.expanded {
				g.MoveTo(triX, triY-3)
				g.LineTo(triX+6, triY-3)
				g.LineTo(triX+3, triY+3)
			} else {
				g.MoveTo(triX, triY-4)
				g.LineTo(triX+6, triY)
				g.LineTo(triX, triY+4)
			}
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			g.Fill()
			textX += 10
		} else {
			textX += 10
		}

		// Kind icon
		kindColor := gui.SymbolKindColor(node.symbol.Kind)
		kindLabel := gui.SymbolKindLabel(node.symbol.Kind)
		g.SetFont(kindFont)
		g.SetBrush1(kindColor)
		g.DrawText1(textX, rowY+rh-6, kindLabel)
		textX += 14

		// Symbol name
		isType := node.symbol.Kind == gui.SymType
		if isType {
			g.SetFont(boldFont)
		} else {
			g.SetFont(normalFont)
		}
		if i == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.DrawText1(textX, rowY+rh-6, node.symbol.Name)
	}
}

// hitTest returns the flat list index for a given y coordinate, or -1.
func (this *CodeOutlinePanel) hitTest(y float64) int {
	headerH := 22.0
	if y < headerH {
		return -1
	}
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.flatList) {
		return -1
	}
	return idx
}

// OnLeftDown handles click events.
func (this *CodeOutlinePanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.hitTest(y)
	if idx < 0 {
		return
	}

	node := this.flatList[idx]

	// Toggle expand/collapse for types with children
	if node.depth == 0 && len(node.children) > 0 {
		// Find the matching tree node and toggle
		for i := range this.tree {
			if this.tree[i].symbol.Name == node.symbol.Name && this.tree[i].symbol.Line == node.symbol.Line {
				this.tree[i].expanded = !this.tree[i].expanded
				break
			}
		}
		this.rebuildFlatList()
	}

	this.selectedIdx = idx

	// Navigate to symbol
	if this.cbNavigate != nil {
		this.cbNavigate(node.symbol.Line)
	}

	this.Self().Update()
}

// OnMouseMove updates hover state.
func (this *CodeOutlinePanel) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover.
func (this *CodeOutlinePanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *CodeOutlinePanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	headerH := 22.0
	totalRows := float64(len(this.flatList))
	maxScroll := totalRows*this.rowHeight - (this.Height() - headerH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

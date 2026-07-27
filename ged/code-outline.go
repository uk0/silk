package ged

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.CodeOutlinePanel", gui.TypeOf(CodeOutlinePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.CodeOutlinePanel",
		Name: "大纲",
		Icon: "tree-view",
		Desc: "代码大纲面板",
	})
}

// Sort modes accepted by SetSortMode.
const (
	// OutlineSortByPosition keeps declaration order: rows follow the file
	// top to bottom. This is the default, and what an outline is for.
	OutlineSortByPosition = 0
	// OutlineSortByName sorts every level alphabetically (case-insensitive,
	// ties broken by line) — the "sort alphabetically" toggle.
	OutlineSortByName = 1
)

// OutlineSymbol is one node of a semantic document outline, pushed in by
// the host through SetSymbols. It is deliberately provider-agnostic: the
// host drives core.LSPClient.DocumentSymbol and converts every LSPSymbol
// into an OutlineSymbol (kind number → kind name, range → Line/EndLine)
// before handing the slice over, so the panel never has to know about LSP
// wire types or numeric kind enums — the same split ReferencesPanel uses
// for ReferenceLoc. When no symbols are supplied the panel falls back to
// the heuristic CodeEditor.ParseSymbols scan.
//
// Field semantics:
//
//   - Name     display name of the declaration
//   - Detail   declaration summary (signature); informational for now
//   - Kind     kind name, matched case-insensitively (see outlineKindOf)
//   - Line     0-based first line of the declaration (range.start.line)
//   - EndLine  0-based last line, 0 when the provider gave no range
//   - Children nested members at arbitrary depth (methods of a type, fields of a struct)
//   - Exported whether the symbol belongs to the file's public API
//
// Kind takes both LSP and Go spellings ("Function"/"func", "Struct"/"type",
// "Method", "Field"/"var", "EnumMember"/"const"); anything unknown renders
// as a function. EndLine, when present, lets the cursor-follow highlight
// pick the innermost symbol containing the caret. Exported is the host's
// call — the rule is language-specific (in Go: a capitalised name) — and it
// is what the exported-only toggle filters on.
type OutlineSymbol struct {
	Name     string
	Detail   string
	Kind     string
	Line     int
	EndLine  int
	Children []OutlineSymbol
	Exported bool
}

// outlineNode represents one entry in the outline tree.
type outlineNode struct {
	symbol   gui.CodeSymbol
	children []outlineNode
	depth    int
	expanded bool
	endLine  int   // 0-based last line of the declaration, 0 when unknown
	exported bool  // part of the file's public API
	path     []int // index path into tree, so nested rows can be collapsed
}

// CodeOutlinePanel is a persistent symbol tree panel that shows all
// functions, types, variables, and constants from the active code editor.
type CodeOutlinePanel struct {
	gui.Widget

	symbols     []gui.CodeSymbol
	semantic    []OutlineSymbol // host-supplied outline; wins over the heuristic scan
	tree        []outlineNode
	flatList    []outlineNode
	hoverIdx    int
	selectedIdx int
	scrollY     float64
	rowHeight   float64
	editor      *gui.CodeEditor
	cbNavigate  func(line int)

	// view options: how the symbol set is narrowed and ordered
	filter       string
	sortMode     int
	exportedOnly bool

	// cache: last known text size to avoid re-parsing when content hasn't
	// changed, plus the host's document revision. The size fingerprint is
	// blind to same-length edits ("aa" → "bb"), so revision is the
	// authoritative invalidation signal; dirty records that one landed.
	lastTextLen   int
	lastLineCount int
	revision      int
	dirty         bool
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
	this.seedContentCache()
	this.Self().Update()
}

// SetNavigateCallback sets the callback invoked when a symbol is clicked.
func (this *CodeOutlinePanel) SetNavigateCallback(fn func(line int)) {
	this.cbNavigate = fn
}

// SetSymbols installs a semantic outline (a documentSymbol response the
// host already converted to []OutlineSymbol) and rebuilds the tree from
// that hierarchy. It takes a deep copy, so the host may keep reusing or
// mutating the slice it handed in. Passing nil (or an empty slice) drops
// the semantic outline and falls back to the heuristic ParseSymbols scan,
// which is also what an editor with no language server keeps using.
func (this *CodeOutlinePanel) SetSymbols(syms []OutlineSymbol) {
	this.semantic = cloneOutlineSymbols(syms)
	this.RefreshSymbols()
	this.seedContentCache()
	this.Self().Update()
}

// SetFilter narrows the tree to symbols whose name contains text
// (case-insensitive). A match keeps its ancestors as context, so a hit on
// a method still shows the type it hangs under. Empty text clears it.
func (this *CodeOutlinePanel) SetFilter(text string) {
	if text == this.filter {
		return
	}
	this.filter = text
	this.applyView()
}

// Filter returns the active name filter ("" when unfiltered).
func (this *CodeOutlinePanel) Filter() string {
	return this.filter
}

// SetSortMode switches between OutlineSortByPosition (declaration order,
// the default) and OutlineSortByName (alphabetical). The mode applies to
// every level of the tree, not just the top one.
func (this *CodeOutlinePanel) SetSortMode(mode int) {
	if mode != OutlineSortByName {
		mode = OutlineSortByPosition
	}
	if mode == this.sortMode {
		return
	}
	this.sortMode = mode
	this.applyView()
}

// SortMode returns the active sort mode.
func (this *CodeOutlinePanel) SortMode() int {
	return this.sortMode
}

// SetExportedOnly toggles the visibility filter that hides symbols not
// part of the file's public API. A non-exported parent survives when one
// of its children is exported, otherwise the child would be orphaned.
func (this *CodeOutlinePanel) SetExportedOnly(on bool) {
	if on == this.exportedOnly {
		return
	}
	this.exportedOnly = on
	this.applyView()
}

// ExportedOnly reports whether the exported-only filter is on.
func (this *CodeOutlinePanel) ExportedOnly() bool {
	return this.exportedOnly
}

// SetRevision records the host's document revision for the bound buffer
// (an editor change counter, or the LSP version the host sends with
// didChange). Any change marks the outline stale so the next
// RefreshIfStale re-reads it — the text-size fingerprint alone misses
// same-length edits, which used to leave the outline showing symbols that
// no longer exist.
func (this *CodeOutlinePanel) SetRevision(rev int) {
	if rev == this.revision {
		return
	}
	this.revision = rev
	this.dirty = true
}

// RefreshIfStale re-reads the outline when the bound buffer looks changed
// — either the revision moved (SetRevision) or the cheap text-size
// fingerprint did. Returns true when a refresh actually ran. Draw calls
// it every frame; a host that edits the buffer itself may call it too.
func (this *CodeOutlinePanel) RefreshIfStale() bool {
	textLen, lineCount := this.contentMetrics()
	if !this.dirty && textLen == this.lastTextLen && lineCount == this.lastLineCount {
		return false
	}
	this.lastTextLen = textLen
	this.lastLineCount = lineCount
	this.dirty = false
	this.RefreshSymbols()
	return true
}

// contentMetrics returns the bound editor's total text length and line
// count — the cheap "did the buffer change" fingerprint.
func (this *CodeOutlinePanel) contentMetrics() (int, int) {
	if this.editor == nil {
		return 0, 0
	}
	lines := this.editor.Lines()
	textLen := 0
	for _, l := range lines {
		textLen += len(l)
	}
	return textLen, len(lines)
}

// seedContentCache records the current buffer size as the baseline for
// RefreshIfStale, so a refresh that just ran is not repeated next frame.
func (this *CodeOutlinePanel) seedContentCache() {
	this.lastTextLen, this.lastLineCount = this.contentMetrics()
	this.dirty = false
}

// applyView rebuilds the tree after a view option (filter, sort order,
// exported-only) changed and pulls the scroll back to the top so the new
// first row is on screen.
func (this *CodeOutlinePanel) applyView() {
	this.buildTree()
	this.rebuildFlatList()
	this.scrollY = 0
	this.Self().Update()
}

// RefreshSymbols rebuilds the outline from its current source: the
// semantic symbols pushed in by the host when there are any, otherwise a
// heuristic re-parse of the bound editor.
func (this *CodeOutlinePanel) RefreshSymbols() {
	this.symbols = nil
	this.tree = nil
	this.flatList = nil

	if len(this.semantic) == 0 {
		if this.editor == nil {
			return
		}
		this.symbols = this.editor.ParseSymbols()
	}

	this.buildTree()
	this.rebuildFlatList()
}

// buildTree organizes the current symbol source into the display tree:
// the host-supplied semantic hierarchy when there is one, otherwise the
// heuristic grouping (types own their receiver methods). The raw tree is
// then narrowed (filter + exported-only), ordered (sort mode) and stamped
// with per-row depth/path.
func (this *CodeOutlinePanel) buildTree() {
	var raw []outlineNode
	if len(this.semantic) > 0 {
		raw = outlineNodesFromSymbols(this.semantic)
	} else {
		raw = this.heuristicNodes()
	}

	raw = this.narrowNodes(raw)
	sortOutlineNodes(raw, this.sortMode)
	stampOutlineNodes(raw, 0, nil)

	this.tree = raw
}

// outlineNodesFromSymbols converts a semantic outline into display nodes,
// preserving the provider's hierarchy at any depth.
func outlineNodesFromSymbols(syms []OutlineSymbol) []outlineNode {
	out := make([]outlineNode, 0, len(syms))
	for _, s := range syms {
		node := outlineNode{
			symbol: gui.CodeSymbol{
				Name:   s.Name,
				Kind:   outlineKindOf(s.Kind),
				Line:   s.Line,
				Detail: s.Detail,
			},
			expanded: true,
			endLine:  s.EndLine,
			exported: s.Exported,
		}
		if len(s.Children) > 0 {
			node.children = outlineNodesFromSymbols(s.Children)
		}
		out = append(out, node)
	}
	return out
}

// heuristicNodes builds the fallback two-level tree out of the line-based
// CodeEditor.ParseSymbols scan: types at the top, their receiver methods
// underneath, everything else at the top. Parents are addressed by index
// rather than by pointer — appending to the top-level slice can move its
// backing array, which used to drop methods on the floor.
func (this *CodeOutlinePanel) heuristicNodes() []outlineNode {
	typeIdx := make(map[string]int)
	var top []outlineNode

	for _, sym := range this.symbols {
		if sym.Kind == gui.SymType {
			typeIdx[sym.Name] = len(top)
			top = append(top, outlineHeuristicNode(sym))
		}
	}

	for _, sym := range this.symbols {
		if sym.Kind == gui.SymType {
			continue
		}
		if sym.Kind == gui.SymMethod && sym.Receiver != "" {
			if idx, ok := typeIdx[sym.Receiver]; ok {
				top[idx].children = append(top[idx].children, outlineHeuristicNode(sym))
				continue
			}
		}
		top = append(top, outlineHeuristicNode(sym))
	}

	return top
}

// outlineHeuristicNode wraps one parsed symbol. The scan has no ranges, so
// endLine stays 0; exported-ness follows the Go rule.
func outlineHeuristicNode(sym gui.CodeSymbol) outlineNode {
	return outlineNode{
		symbol:   sym,
		expanded: true,
		exported: outlineIsExported(sym.Name),
	}
}

// outlineKindOf maps a provider-agnostic kind name onto the gui.Sym* enum
// the row label and colour are drawn from.
func outlineKindOf(kind string) int {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "type", "struct", "interface", "class", "enum":
		return gui.SymType
	case "method", "constructor":
		return gui.SymMethod
	case "var", "variable", "field", "property", "parameter":
		return gui.SymVar
	case "const", "constant", "enummember":
		return gui.SymConst
	default:
		return gui.SymFunc
	}
}

// outlineIsExported applies the Go visibility rule: a leading upper-case
// letter means the declaration is part of the package's public API.
func outlineIsExported(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

// narrowNodes applies the filter and the exported-only toggle.
func (this *CodeOutlinePanel) narrowNodes(nodes []outlineNode) []outlineNode {
	needle := strings.ToLower(strings.TrimSpace(this.filter))
	if needle == "" && !this.exportedOnly {
		return nodes
	}
	return filterOutlineNodes(nodes, needle, this.exportedOnly)
}

// filterOutlineNodes keeps a node when it passes both gates itself, or
// when any descendant does — a surviving child keeps its parents as
// context instead of being lifted to the top level.
func filterOutlineNodes(nodes []outlineNode, needle string, exportedOnly bool) []outlineNode {
	var out []outlineNode
	for _, node := range nodes {
		kids := filterOutlineNodes(node.children, needle, exportedOnly)
		keep := true
		if needle != "" && !strings.Contains(strings.ToLower(node.symbol.Name), needle) {
			keep = false
		}
		if exportedOnly && !node.exported {
			keep = false
		}
		if !keep && len(kids) == 0 {
			continue
		}
		node.children = kids
		out = append(out, node)
	}
	return out
}

// sortOutlineNodes orders every level of the tree in place.
func sortOutlineNodes(nodes []outlineNode, mode int) {
	if mode == OutlineSortByName {
		sort.SliceStable(nodes, func(i, j int) bool {
			a := strings.ToLower(nodes[i].symbol.Name)
			b := strings.ToLower(nodes[j].symbol.Name)
			if a != b {
				return a < b
			}
			return nodes[i].symbol.Line < nodes[j].symbol.Line
		})
	} else {
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].symbol.Line < nodes[j].symbol.Line
		})
	}
	for i := range nodes {
		sortOutlineNodes(nodes[i].children, mode)
	}
}

// stampOutlineNodes records each node's indent depth and its index path
// into the tree, so a click on any row (not just a top-level one) can find
// the node it has to expand or collapse.
func stampOutlineNodes(nodes []outlineNode, depth int, prefix []int) {
	for i := range nodes {
		path := make([]int, len(prefix)+1)
		copy(path, prefix)
		path[len(prefix)] = i
		nodes[i].depth = depth
		nodes[i].path = path
		stampOutlineNodes(nodes[i].children, depth+1, path)
	}
}

// cloneOutlineSymbols deep-copies a semantic outline so the panel owns its
// input, children included.
func cloneOutlineSymbols(syms []OutlineSymbol) []OutlineSymbol {
	if len(syms) == 0 {
		return nil
	}
	out := make([]OutlineSymbol, len(syms))
	for i, s := range syms {
		out[i] = s
		out[i].Children = cloneOutlineSymbols(s.Children)
	}
	return out
}

// nodeAtPath resolves an index path recorded by stampOutlineNodes back to
// the live tree node, or nil when the tree has been rebuilt since.
func (this *CodeOutlinePanel) nodeAtPath(path []int) *outlineNode {
	nodes := this.tree
	var found *outlineNode
	for _, idx := range path {
		if idx < 0 || idx >= len(nodes) {
			return nil
		}
		found = &nodes[idx]
		nodes = found.children
	}
	return found
}

// rebuildFlatList flattens the tree respecting expanded state, at any depth.
func (this *CodeOutlinePanel) rebuildFlatList() {
	this.flatList = nil
	this.appendFlat(this.tree)
	if this.selectedIdx >= len(this.flatList) {
		this.selectedIdx = -1
	}
	if this.hoverIdx >= len(this.flatList) {
		this.hoverIdx = -1
	}
}

// appendFlat walks one level of the tree into the flat row list.
func (this *CodeOutlinePanel) appendFlat(nodes []outlineNode) {
	for _, node := range nodes {
		this.flatList = append(this.flatList, node)
		if node.expanded && len(node.children) > 0 {
			this.appendFlat(node.children)
		}
	}
}

// currentSymbolIndex returns the flat list index of the symbol containing the
// editor cursor, or -1. When the provider gave ranges the innermost symbol
// whose [Line, EndLine] spans the cursor wins (a method beats the type it
// sits in); without ranges it falls back to the last symbol declared at or
// above the cursor.
func (this *CodeOutlinePanel) currentSymbolIndex() int {
	if this.editor == nil || len(this.flatList) == 0 {
		return -1
	}
	cursorLine := this.editor.CursorLine()
	bestIdx := -1
	bestDepth := -1
	aboveIdx := -1
	for i, node := range this.flatList {
		if node.symbol.Line <= cursorLine {
			aboveIdx = i
		}
		if node.endLine <= 0 || node.symbol.Line > cursorLine || cursorLine > node.endLine {
			continue
		}
		if node.depth >= bestDepth {
			bestDepth = node.depth
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return bestIdx
	}
	return aboveIdx
}

// Draw renders the code outline panel.
func (this *CodeOutlinePanel) Draw(g paint.Painter) {
	// Auto-refresh when the revision moved or the content size changed
	this.RefreshIfStale()

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

		// Expand/collapse triangle for any node with children
		if len(node.children) > 0 {
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

	// Toggle expand/collapse for any node with children, at any depth
	if len(node.children) > 0 {
		if target := this.nodeAtPath(node.path); target != nil {
			target.expanded = !target.expanded
			this.rebuildFlatList()
		}
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

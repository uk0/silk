package ged

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.CodeOutlinePanel", gui.TypeOf(CodeOutlinePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.CodeOutlinePanel",
		Name: "大纲",
		Icon: "outline",
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
// the tolerant CodeEditor.ParseSymbols scan.
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
	symbol     gui.CodeSymbol
	children   []outlineNode
	depth      int
	expanded   bool
	endLine    int   // 0-based last line of the declaration, 0 when unknown
	exported   bool  // part of the file's public API
	path       []int // index path into tree, so nested rows can be collapsed
	text       string
	textWidth  float64
	fontSerial uint64
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

	// The editor owns its content revision, so unchanged Draw calls check one
	// integer instead of walking every line. Hosts can additionally invalidate
	// semantic data through SetRevision without changing the editor text.
	lastTextRevision    uint64
	lastSymbolsRevision uint64
	revision            int
	dirty               bool

	// Fonts and visible labels are cached; idle paints do not allocate fonts,
	// remeasure long names, or scan every symbol to follow an unchanged caret.
	normalFont, boldFont    paint.Font
	fontSerial              uint64
	cursorCached            bool
	cursorLine, cursorIndex int
}

func NewCodeOutlinePanel() *CodeOutlinePanel {
	p := new(CodeOutlinePanel)
	p.Init(p)
	return p
}

func (this *CodeOutlinePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.SetFocusPolicy(gui.TabFocus)
	this.hoverIdx = -1
	this.selectedIdx = -1
}

// SetEditor binds the outline panel to a code editor and refreshes symbols.
func (this *CodeOutlinePanel) SetEditor(editor *gui.CodeEditor) {
	if editor != nil && this.editor == editor {
		if this.RefreshIfStale() {
			this.Self().Update()
		}
		return
	}
	this.editor = editor
	this.semantic = nil
	this.scrollY = 0
	this.RefreshSymbols()
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
	this.seedContentCache()
	this.RefreshSymbols()
	this.Self().Update()
}

// SetSymbolsForEditor installs a response only if it still belongs to the bound
// editor and exact text revision captured when the request started. Call on the
// UI thread after marshaling the asynchronous result. A stale response is ignored
// without changing the view. SetSymbols remains the synchronous compatibility API;
// hosts with asynchronous providers should use this guarded form instead.
func (this *CodeOutlinePanel) SetSymbolsForEditor(editor *gui.CodeEditor, revision uint64, syms []OutlineSymbol) bool {
	if editor == nil || editor != this.editor || revision != editor.TextRevision() {
		return false
	}
	this.SetSymbols(syms)
	return true
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
// didChange). Any change marks the outline stale so the next RefreshIfStale
// re-reads it. Ordinary editor mutations are tracked automatically; this
// optional host revision also supports out-of-band semantic updates.
func (this *CodeOutlinePanel) SetRevision(rev int) {
	if rev == this.revision {
		return
	}
	this.revision = rev
	this.dirty = true
}

// RefreshIfStale re-reads the outline when the editor content or host revision
// changes. The unchanged Draw path is O(1), including for large documents and
// same-length edits which the old text-size fingerprint could not detect.
func (this *CodeOutlinePanel) RefreshIfStale() bool {
	revision := this.editorTextRevision()
	if !this.dirty && revision == this.lastTextRevision && this.editorSymbolsRevision() == this.lastSymbolsRevision {
		return false
	}
	this.RefreshSymbols()
	return true
}

func (this *CodeOutlinePanel) editorTextRevision() uint64 {
	if this.editor == nil {
		return 0
	}
	return this.editor.TextRevision()
}

func (this *CodeOutlinePanel) editorSymbolsRevision() uint64 {
	if this.editor == nil {
		return 0
	}
	return this.editor.SymbolsRevision()
}

// seedContentCache records the current editor revision as the baseline for
// RefreshIfStale, so a refresh that just ran is not repeated next frame.
func (this *CodeOutlinePanel) seedContentCache() {
	this.lastTextRevision = this.editorTextRevision()
	this.lastSymbolsRevision = this.editorSymbolsRevision()
	this.dirty = false
}

// applyView rebuilds the tree after a view option (filter, sort order,
// exported-only) changed and pulls the scroll back to the top so the new
// first row is on screen.
func (this *CodeOutlinePanel) applyView() {
	this.selectedIdx, this.hoverIdx = -1, -1
	this.buildTree()
	this.rebuildFlatList()
	this.scrollY = 0
	this.Self().Update()
}

// RefreshSymbols rebuilds the outline from its current source: the
// semantic symbols pushed in by the host when there are any, otherwise a
// heuristic re-parse of the bound editor.
func (this *CodeOutlinePanel) RefreshSymbols() {
	if this.dirty || this.editorTextRevision() != this.lastTextRevision {
		// A server response for old text must never hide the edited declarations.
		this.semantic = nil
	}
	this.seedContentCache()
	this.selectedIdx, this.hoverIdx = -1, -1
	this.cursorCached = false
	this.symbols = nil
	this.tree = nil
	this.flatList = nil

	if len(this.semantic) == 0 {
		if this.editor == nil {
			this.scrollY = 0
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

// heuristicNodes builds the fallback two-level tree out of the tolerant
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
			receiver := strings.TrimPrefix(strings.TrimSpace(sym.Receiver), "*")
			if bracket := strings.IndexByte(receiver, '['); bracket >= 0 {
				receiver = strings.TrimSpace(receiver[:bracket])
			}
			if idx, ok := typeIdx[receiver]; ok {
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
	case "macro":
		return gui.SymMacro
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
	this.cursorCached = false
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
	if this.cursorCached && this.cursorLine == cursorLine {
		return this.cursorIndex
	}
	this.cursorCached, this.cursorLine = true, cursorLine
	bestIdx := -1
	bestDepth := -1
	aboveIdx := -1
	for i, node := range this.flatList {
		if node.symbol.Line <= cursorLine && (aboveIdx < 0 || node.symbol.Line >= this.flatList[aboveIdx].symbol.Line) {
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
	this.cursorIndex = aboveIdx
	if bestIdx >= 0 {
		this.cursorIndex = bestIdx
	}
	return this.cursorIndex
}

func (this *CodeOutlinePanel) ensureFonts() {
	f := gui.Theme().Font
	if this.normalFont != nil && this.normalFont.Family() == f.Family() && this.normalFont.Size() == f.Size() && this.normalFont.Bold() == f.Bold() && this.normalFont.Italic() == f.Italic() {
		return
	}
	this.normalFont = f.Dup()
	this.boldFont = paint.NewFont(f.Family(), f.Size(), true, f.Italic())
	this.rowHeight = math.Ceil(math.Max(22, math.Max(this.normalFont.FontExtents().Height, this.boldFont.FontExtents().Height)+8))
	this.fontSerial++
}

func (this *CodeOutlinePanel) headerHeight() float64 {
	this.ensureFonts()
	return this.rowHeight
}

func (this *CodeOutlinePanel) clampScroll() {
	this.ensureFonts()
	body := math.Max(0, this.Height()-this.rowHeight)
	this.scrollY = math.Max(0, math.Min(this.scrollY, math.Max(0, float64(len(this.flatList))*this.rowHeight-body)))
}

func (this *CodeOutlinePanel) visibleRows(height float64) (int, int) {
	first := max(0, min(len(this.flatList), int(math.Floor(this.scrollY/this.rowHeight))))
	last := max(first, min(len(this.flatList), int(math.Ceil((this.scrollY+height-this.headerHeight())/this.rowHeight))))
	return first, last
}

// Draw clips both the dock and its scrollable body. Only visible rows are
// visited; dark/light themes and user font preferences share the same geometry
// with hit-testing and keyboard navigation.
func (this *CodeOutlinePanel) Draw(g paint.Painter) {
	this.RefreshIfStale()
	this.clampScroll()
	w, h := this.Size()
	if w <= 0 || h <= 0 {
		return
	}
	t := gui.Theme()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	hh := this.headerHeight()
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, hh)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.MoveTo(0, hh-.5)
	g.LineTo(w, hh-.5)
	g.Stroke()
	drawPanelText(g, this.boldFont, i18n.T("Outline"), 8, 0, w-16, hh, t.TextColor)
	if h <= hh {
		return
	}
	g.Rectangle(0, hh, w, h-hh)
	g.Clip()
	if len(this.flatList) == 0 {
		drawPanelText(g, this.normalFont, i18n.T("No symbols"), 8, hh, w-16, this.rowHeight, t.TextColor)
		return
	}
	curIdx := this.currentSymbolIndex()
	first, last := this.visibleRows(h)
	for i := first; i < last; i++ {
		node := &this.flatList[i]
		rh := this.rowHeight
		y := hh - this.scrollY + float64(i)*rh
		selected := i == this.selectedIdx
		if selected {
			g.SetBrush1(t.HighLightColor)
		} else {
			g.SetBrush1(t.ButtonHoverBGColor)
		}
		if selected || i == this.hoverIdx || i == curIdx {
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}
		color := t.TextColor
		if selected {
			color = t.MenuActiveTextColor
		}
		x := 8 + float64(node.depth)*16
		if len(node.children) > 0 {
			cy := y + rh/2
			if node.expanded {
				g.MoveTo(x, cy-3)
				g.LineTo(x+6, cy-3)
				g.LineTo(x+3, cy+3)
			} else {
				g.MoveTo(x, cy-4)
				g.LineTo(x+6, cy)
				g.LineTo(x, cy+4)
			}
			g.SetBrush1(color)
			g.Fill()
		}
		x += 10
		label := gui.SymbolKindLabel(node.symbol.Kind)
		kindWidth := math.Max(14, explorerTextWidth(this.boldFont, label)+4)
		// A theme text color also keeps badges legible in light/high-contrast
		// themes and while selected, unlike fixed dark-theme-only accent colors.
		drawPanelText(g, this.boldFont, label, x, y, math.Min(kindWidth, w-8-x), rh, color)
		x += kindWidth
		font := this.normalFont
		if node.symbol.Kind == gui.SymType {
			font = this.boldFont
		}
		width := math.Max(0, w-8-x)
		if node.fontSerial != this.fontSerial || node.textWidth != width {
			node.text = explorerElide(font, explorerDisplayName(node.symbol.Name), width)
			node.fontSerial, node.textWidth = this.fontSerial, width
		}
		drawPanelText(g, font, node.text, x, y, width, rh, color)
	}
}

// hitTest returns the flat list index for a given y coordinate, or -1.
func (this *CodeOutlinePanel) hitTest(y float64) int {
	this.clampScroll()
	headerH := this.headerHeight()
	if y < headerH || y >= this.Height() {
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
	this.RefreshIfStale()
	if x < 0 || x >= this.Width() {
		return
	}
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
	this.RefreshIfStale()
	idx := -1
	if x >= 0 && x < this.Width() {
		idx = this.hitTest(y)
	}
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
	this.RefreshIfStale()
	if math.IsNaN(z) || math.IsInf(z, 0) {
		return
	}
	this.scrollY -= z * 3
	this.clampScroll()
	this.Self().Update()
}

func (this *CodeOutlinePanel) ensureSelectionVisible() {
	this.clampScroll()
	if this.selectedIdx < 0 {
		return
	}
	top := float64(this.selectedIdx) * this.rowHeight
	body := math.Max(this.rowHeight, this.Height()-this.headerHeight())
	if top < this.scrollY {
		this.scrollY = top
	} else if top+this.rowHeight > this.scrollY+body {
		this.scrollY = top + this.rowHeight - body
	}
	this.clampScroll()
}

// Arrow keys select/expand without stealing focus back to the editor. Enter
// explicitly navigates. This also makes deep outlines usable without a mouse.
func (this *CodeOutlinePanel) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	this.RefreshIfStale()
	this.ensureFonts()
	if len(this.flatList) == 0 {
		return
	}
	i := this.selectedIdx
	page := max(1, int((this.Height()-this.headerHeight())/this.rowHeight))
	switch key {
	case gui.KeyDown:
		i = min(len(this.flatList)-1, i+1)
	case gui.KeyUp:
		if i < 0 {
			i = 0
		} else {
			i = max(0, i-1)
		}
	case gui.KeyHome:
		i = 0
	case gui.KeyEnd:
		i = len(this.flatList) - 1
	case gui.KeyPageDown:
		i = min(len(this.flatList)-1, max(0, i)+page)
	case gui.KeyPageUp:
		i = max(0, i-page)
	case gui.KeyRight, gui.KeyLeft:
		if i < 0 {
			i = 0
			break
		}
		node := this.flatList[i]
		if target := this.nodeAtPath(node.path); target != nil {
			if key == gui.KeyRight {
				if len(node.children) > 0 && !target.expanded {
					target.expanded = true
					this.rebuildFlatList()
				} else if len(node.children) > 0 {
					i++
				}
			} else if len(node.children) > 0 && target.expanded {
				target.expanded = false
				this.rebuildFlatList()
			} else {
				for j := i - 1; j >= 0; j-- {
					if this.flatList[j].depth < node.depth {
						i = j
						break
					}
				}
			}
		}
	case gui.KeyEnter:
		if !repeat && i >= 0 && this.cbNavigate != nil {
			this.cbNavigate(this.flatList[i].symbol.Line)
		}
		return
	default:
		return
	}
	this.selectedIdx, this.hoverIdx = i, -1
	this.ensureSelectionVisible()
	this.Self().Update()
}

package gui

import (
	"fmt"
	"github.com/uk0/silk/paint"
	"strconv"
	"strings"
	"unicode"
)

// --- Symbol Types ---

// CodeSymbol represents a parsed symbol declaration in Go source code.
type CodeSymbol struct {
	Name     string
	Kind     int    // 0=func, 1=type, 2=var, 3=const, 4=method
	Line     int    // 0-based line index
	Detail   string // e.g., "func(x int) string" or "type struct"
	Receiver string // for methods: receiver type name
}

const (
	SymFunc   = 0
	SymType   = 1
	SymVar    = 2
	SymConst  = 3
	SymMethod = 4
)

// SymbolKindLabel returns a short display label for a symbol kind (exported).
func SymbolKindLabel(kind int) string {
	return symbolKindLabel(kind)
}

// SymbolKindColor returns the color for a symbol kind label (exported).
func SymbolKindColor(kind int) paint.Color {
	return symbolKindColor(kind)
}

// symbolKindLabel returns a short display label for a symbol kind.
func symbolKindLabel(kind int) string {
	switch kind {
	case SymFunc:
		return "f"
	case SymType:
		return "T"
	case SymVar:
		return "v"
	case SymConst:
		return "c"
	case SymMethod:
		return "m"
	default:
		return " "
	}
}

// symbolKindColor returns the color for a symbol kind label.
func symbolKindColor(kind int) paint.Color {
	switch kind {
	case SymFunc:
		return paint.Color{R: 86, G: 156, B: 214, A: 255} // blue
	case SymType:
		return paint.Color{R: 78, G: 201, B: 176, A: 255} // green/teal
	case SymVar:
		return paint.Color{R: 206, G: 145, B: 120, A: 255} // orange
	case SymConst:
		return paint.Color{R: 181, G: 137, B: 214, A: 255} // purple
	case SymMethod:
		return paint.Color{R: 78, G: 201, B: 176, A: 255} // teal
	default:
		return paint.Color{R: 180, G: 180, B: 180, A: 255}
	}
}

// --- Symbol Parser ---

// ParseSymbols scans the current editor content and returns all top-level
// function, type, variable, and constant declarations found by simple
// line-by-line pattern matching.
func (this *CodeEditor) ParseSymbols() []CodeSymbol {
	var symbols []CodeSymbol
	inVarBlock := false
	inConstBlock := false

	for i, line := range this.lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Detect var/const block open
		if trimmed == "var (" {
			inVarBlock = true
			continue
		}
		if trimmed == "const (" {
			inConstBlock = true
			continue
		}

		// Detect block close
		if (inVarBlock || inConstBlock) && trimmed == ")" {
			inVarBlock = false
			inConstBlock = false
			continue
		}

		// Inside var block
		if inVarBlock {
			name := extractBlockDeclName(trimmed)
			if name != "" {
				symbols = append(symbols, CodeSymbol{
					Name:   name,
					Kind:   SymVar,
					Line:   i,
					Detail: "var",
				})
			}
			continue
		}

		// Inside const block
		if inConstBlock {
			name := extractBlockDeclName(trimmed)
			if name != "" {
				symbols = append(symbols, CodeSymbol{
					Name:   name,
					Kind:   SymConst,
					Line:   i,
					Detail: "const",
				})
			}
			continue
		}

		// func declarations
		if strings.HasPrefix(trimmed, "func ") {
			sym := parseFuncLine(trimmed, i)
			if sym != nil {
				symbols = append(symbols, *sym)
			}
			continue
		}

		// type declarations
		if strings.HasPrefix(trimmed, "type ") {
			sym := parseTypeLine(trimmed, i)
			if sym != nil {
				symbols = append(symbols, *sym)
			}
			continue
		}

		// Single-line var
		if strings.HasPrefix(trimmed, "var ") {
			sym := parseVarLine(trimmed, i)
			if sym != nil {
				symbols = append(symbols, *sym)
			}
			continue
		}

		// Single-line const
		if strings.HasPrefix(trimmed, "const ") {
			sym := parseConstLine(trimmed, i)
			if sym != nil {
				symbols = append(symbols, *sym)
			}
			continue
		}
	}
	return symbols
}

// extractBlockDeclName extracts the identifier from a line inside a var/const block.
// e.g. "  FooBar = 42" → "FooBar", "  x int" → "x", "  // comment" → ""
func extractBlockDeclName(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
		return ""
	}
	// The first word is the identifier
	runes := []rune(trimmed)
	if !isIdentStart(runes[0]) {
		return ""
	}
	end := 0
	for end < len(runes) && isIdentPart(runes[end]) {
		end++
	}
	name := string(runes[:end])
	// Skip iota-only entries like "_"
	if name == "_" {
		return ""
	}
	return name
}

// parseFuncLine parses a line starting with "func " and returns a CodeSymbol.
// Handles both regular functions and methods with receivers.
func parseFuncLine(line string, lineIdx int) *CodeSymbol {
	rest := strings.TrimPrefix(line, "func ")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}

	// Check for method receiver: func (r *Type) Name(...)
	if rest[0] == '(' {
		// Find the closing paren of the receiver
		depth := 1
		j := 1
		runes := []rune(rest)
		for j < len(runes) && depth > 0 {
			if runes[j] == '(' {
				depth++
			} else if runes[j] == ')' {
				depth--
			}
			j++
		}
		receiverPart := string(runes[1 : j-1]) // contents inside parens
		receiver := extractReceiverType(receiverPart)

		// After receiver, skip space, get method name
		remaining := strings.TrimSpace(string(runes[j:]))
		name := extractIdentifier(remaining)
		if name == "" {
			return nil
		}

		// Build detail: everything after the name
		detail := "func " + strings.TrimSpace(string(runes[:j])) + " " + name
		parenIdx := strings.Index(remaining, "(")
		if parenIdx >= 0 {
			detail = "func" + remaining[parenIdx:]
		}

		return &CodeSymbol{
			Name:     name,
			Kind:     SymMethod,
			Line:     lineIdx,
			Detail:   detail,
			Receiver: receiver,
		}
	}

	// Regular function: func Name(...)
	name := extractIdentifier(rest)
	if name == "" {
		return nil
	}

	detail := "func"
	parenIdx := strings.Index(rest, "(")
	if parenIdx >= 0 {
		detail = "func" + rest[parenIdx:]
	}

	return &CodeSymbol{
		Name:   name,
		Kind:   SymFunc,
		Line:   lineIdx,
		Detail: detail,
	}
}

// extractReceiverType extracts the type name from a receiver declaration.
// e.g. "this *Foo" → "Foo", "f Foo" → "Foo", "*Bar" → "Bar"
func extractReceiverType(recv string) string {
	recv = strings.TrimSpace(recv)
	// Remove leading variable name if present
	parts := strings.Fields(recv)
	typePart := recv
	if len(parts) >= 2 {
		typePart = parts[len(parts)-1]
	}
	// Remove pointer star
	typePart = strings.TrimPrefix(typePart, "*")
	return typePart
}

// extractIdentifier extracts the first identifier from a string.
func extractIdentifier(s string) string {
	runes := []rune(s)
	if len(runes) == 0 || !isIdentStart(runes[0]) {
		return ""
	}
	end := 0
	for end < len(runes) && isIdentPart(runes[end]) {
		end++
	}
	return string(runes[:end])
}

// parseTypeLine parses "type TypeName struct/interface/..." lines.
func parseTypeLine(line string, lineIdx int) *CodeSymbol {
	rest := strings.TrimPrefix(line, "type ")
	rest = strings.TrimSpace(rest)
	name := extractIdentifier(rest)
	if name == "" {
		return nil
	}
	// Get the kind word (struct, interface, etc.)
	remaining := strings.TrimSpace(strings.TrimPrefix(rest, name))
	kindWord := extractIdentifier(remaining)
	detail := "type"
	if kindWord != "" {
		detail = "type " + kindWord
	}
	return &CodeSymbol{
		Name:   name,
		Kind:   SymType,
		Line:   lineIdx,
		Detail: detail,
	}
}

// parseVarLine parses "var VarName ..." lines.
func parseVarLine(line string, lineIdx int) *CodeSymbol {
	rest := strings.TrimPrefix(line, "var ")
	rest = strings.TrimSpace(rest)
	if rest == "(" {
		return nil // block open, handled elsewhere
	}
	name := extractIdentifier(rest)
	if name == "" || name == "_" {
		return nil
	}
	return &CodeSymbol{
		Name:   name,
		Kind:   SymVar,
		Line:   lineIdx,
		Detail: "var",
	}
}

// parseConstLine parses "const ConstName ..." lines.
func parseConstLine(line string, lineIdx int) *CodeSymbol {
	rest := strings.TrimPrefix(line, "const ")
	rest = strings.TrimSpace(rest)
	if rest == "(" {
		return nil // block open, handled elsewhere
	}
	name := extractIdentifier(rest)
	if name == "" || name == "_" {
		return nil
	}
	return &CodeSymbol{
		Name:   name,
		Kind:   SymConst,
		Line:   lineIdx,
		Detail: "const",
	}
}

// --- Word Extraction Helpers ---

// wordAtPosition extracts the Go identifier at the given line and column.
func (this *CodeEditor) wordAtPosition(line, col int) string {
	if line < 0 || line >= len(this.lines) {
		return ""
	}
	runes := []rune(this.lines[line])
	if col < 0 || col >= len(runes) {
		return ""
	}
	if !isIdentStart(runes[col]) && !unicode.IsDigit(runes[col]) {
		return ""
	}
	start := col
	for start > 0 && isIdentPart(runes[start-1]) {
		start--
	}
	end := col
	for end < len(runes) && isIdentPart(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

// wordAtCursor extracts the Go identifier at the current cursor position.
func (this *CodeEditor) wordAtCursor() string {
	this.clampCursor()
	col := this.cursorCol
	if col > 0 {
		// If cursor is right after a word, look at previous char
		runes := []rune(this.lines[this.cursorLine])
		if col >= len(runes) || !isIdentPart(runes[col]) {
			col--
		}
	}
	return this.wordAtPosition(this.cursorLine, col)
}

// --- Go To Definition ---

// goToDefinition finds the definition of the word under cursor and jumps to it.
func (this *CodeEditor) goToDefinition() {
	word := this.wordAtCursor()
	if word == "" {
		return
	}
	symbols := this.ParseSymbols()
	for _, s := range symbols {
		if s.Name == word {
			this.goToLine(s.Line)
			return
		}
	}
}

// goToLine scrolls to a line and places cursor there.
func (this *CodeEditor) goToLine(line int) {
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	this.cursorLine = line
	this.cursorCol = 0
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

// --- Current Function Detection ---

// currentFunction returns the symbol that the cursor is currently inside of.
// It walks the parsed symbols and finds the last one whose Line <= cursorLine.
func (this *CodeEditor) currentFunction() *CodeSymbol {
	symbols := this.ParseSymbols()
	var best *CodeSymbol
	for i := range symbols {
		s := &symbols[i]
		if s.Kind == SymFunc || s.Kind == SymMethod {
			if s.Line <= this.cursorLine {
				best = s
			} else {
				break
			}
		}
	}
	return best
}

// --- Symbol Popup ---

// SymbolPopup provides a "Go to Symbol" overlay for quick navigation.
type SymbolPopup struct {
	symbols     []CodeSymbol
	filtered    []CodeSymbol
	filterText  string
	filterRunes []rune
	selectedIdx int
	scrollY     int
	visible     bool
	maxVisible  int
}

// NewSymbolPopup creates a new symbol navigation popup.
func NewSymbolPopup() *SymbolPopup {
	return &SymbolPopup{
		maxVisible: 12,
	}
}

// Show opens the symbol popup, parsing symbols from the editor.
func (this *SymbolPopup) Show(editor *CodeEditor) {
	this.symbols = editor.ParseSymbols()
	this.filterText = ""
	this.filterRunes = nil
	this.selectedIdx = 0
	this.scrollY = 0
	this.filter()
	this.visible = true
}

// Dismiss closes the symbol popup.
func (this *SymbolPopup) Dismiss() {
	this.visible = false
	this.filterText = ""
	this.filterRunes = nil
}

// filter narrows symbols by the current filter text (case-insensitive prefix/substring).
func (this *SymbolPopup) filter() {
	this.filtered = nil
	if this.filterText == "" {
		this.filtered = append(this.filtered, this.symbols...)
	} else {
		lower := strings.ToLower(this.filterText)
		for _, sym := range this.symbols {
			nameLower := strings.ToLower(sym.Name)
			if strings.Contains(nameLower, lower) {
				this.filtered = append(this.filtered, sym)
			}
		}
	}
	if this.selectedIdx >= len(this.filtered) {
		this.selectedIdx = 0
	}
	this.scrollY = 0
}

// SelectNext moves selection down.
func (this *SymbolPopup) SelectNext() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectedIdx++
	if this.selectedIdx >= len(this.filtered) {
		this.selectedIdx = 0
	}
	if this.selectedIdx >= this.scrollY+this.maxVisible {
		this.scrollY = this.selectedIdx - this.maxVisible + 1
	}
	if this.selectedIdx < this.scrollY {
		this.scrollY = this.selectedIdx
	}
}

// SelectPrev moves selection up.
func (this *SymbolPopup) SelectPrev() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectedIdx--
	if this.selectedIdx < 0 {
		this.selectedIdx = len(this.filtered) - 1
	}
	if this.selectedIdx < this.scrollY {
		this.scrollY = this.selectedIdx
	}
	if this.selectedIdx >= this.scrollY+this.maxVisible {
		this.scrollY = this.selectedIdx - this.maxVisible + 1
	}
}

// Accept jumps to the selected symbol and closes the popup.
func (this *SymbolPopup) Accept(editor *CodeEditor) {
	if !this.visible || len(this.filtered) == 0 {
		return
	}
	sym := this.filtered[this.selectedIdx]
	this.Dismiss()
	editor.goToLine(sym.Line)
}

// OnTextInput handles typing in the symbol filter field.
func (this *SymbolPopup) OnTextInput(s string) {
	this.filterRunes = append(this.filterRunes, []rune(s)...)
	this.filterText = string(this.filterRunes)
	this.filter()
}

// OnBackspace handles backspace in the symbol filter field.
func (this *SymbolPopup) OnBackspace() {
	if len(this.filterRunes) > 0 {
		this.filterRunes = this.filterRunes[:len(this.filterRunes)-1]
		this.filterText = string(this.filterRunes)
		this.filter()
	}
}

// drawPopup renders the symbol popup overlay on the editor.
func (this *SymbolPopup) drawPopup(g paint.Painter, editor *CodeEditor) {
	if !this.visible {
		return
	}

	fe := editor.font.FontExtents()
	edW, edH := editor.Size()

	// Popup dimensions: 60% width, up to 50% height
	popupW := edW * 0.6
	if popupW < 300 {
		popupW = 300
	}
	if popupW > edW-20 {
		popupW = edW - 20
	}

	itemH := fe.Height + 8
	inputH := fe.Height + 12
	visibleCount := len(this.filtered)
	if visibleCount > this.maxVisible {
		visibleCount = this.maxVisible
	}
	contentH := float64(visibleCount) * itemH
	popupH := inputH + contentH + 4
	maxH := edH * 0.5
	if popupH > maxH {
		popupH = maxH
		contentH = popupH - inputH - 4
		visibleCount = int(contentH / itemH)
	}

	// Center horizontally, offset from top
	px := (edW - popupW) / 2
	py := 40.0 // slight offset from top

	// Shadow
	g.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 80})
	g.Rectangle(px+3, py+3, popupW, popupH)
	g.Fill()

	// Background
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 45, A: 250})
	g.Rectangle(px, py, popupW, popupH)
	g.Fill()

	// Border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 100, A: 255}, 1)
	g.Rectangle(px, py, popupW, popupH)
	g.Stroke()

	g.SetFont(editor.font)

	// --- Filter input field ---
	inputX := px + 8
	inputY := py + 4
	inputW := popupW - 16

	// Input background
	g.SetBrush1(paint.Color{R: 28, G: 28, B: 35, A: 255})
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Fill()
	g.SetPen1(paint.Color{R: 80, G: 80, B: 120, A: 255}, 1)
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Stroke()

	// Filter text
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 220, A: 255})
	displayText := this.filterText
	if displayText == "" {
		// Placeholder
		g.SetBrush1(paint.Color{R: 100, G: 100, B: 120, A: 180})
		displayText = "Type to filter symbols..."
	}
	g.DrawText1(inputX+6, inputY+fe.Ascent+4, displayText)

	// Filter cursor
	if this.filterText != "" || true {
		prefix := this.filterText
		fcx := inputX + 6 + editor.measureText(prefix)
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
		g.Rectangle(fcx, inputY+3, 1, inputH-6)
		g.Fill()
	}

	// --- Symbol list ---
	listY := inputY + inputH + 2

	// Count label
	countStr := fmt.Sprintf("%d symbols", len(this.filtered))
	g.SetBrush1(paint.Color{R: 100, G: 100, B: 130, A: 200})
	countExt := editor.font.TextExtents(countStr)
	g.DrawText1(px+popupW-countExt.XAdvance-10, inputY+fe.Ascent+4, countStr)

	for vi := 0; vi < visibleCount; vi++ {
		idx := this.scrollY + vi
		if idx >= len(this.filtered) {
			break
		}
		sym := this.filtered[idx]
		iy := listY + float64(vi)*itemH

		// Selection highlight
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 50, G: 80, B: 160, A: 255})
			g.Rectangle(px+2, iy, popupW-4, itemH)
			g.Fill()
		}

		// Kind icon
		g.SetBrush1(symbolKindColor(sym.Kind))
		g.DrawText1(px+12, iy+fe.Ascent+4, symbolKindLabel(sym.Kind))

		// Symbol name
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 215, A: 255})
		}
		g.DrawText1(px+30, iy+fe.Ascent+4, sym.Name)

		// Receiver for methods
		detailStr := sym.Detail
		if sym.Receiver != "" {
			detailStr = sym.Receiver + "." + sym.Name
		}

		// Detail (right side)
		g.SetBrush1(paint.Color{R: 110, G: 110, B: 135, A: 200})
		detailExt := editor.font.TextExtents(detailStr)
		detailX := px + popupW - detailExt.XAdvance - 60
		nameExt := editor.font.TextExtents(sym.Name)
		if detailX < px+30+nameExt.XAdvance+10 {
			detailX = px + 30 + nameExt.XAdvance + 10
		}
		g.DrawText1(detailX, iy+fe.Ascent+4, detailStr)

		// Line number (far right)
		lineStr := fmt.Sprintf(":%d", sym.Line+1)
		g.SetBrush1(paint.Color{R: 90, G: 90, B: 110, A: 200})
		lineExt := editor.font.TextExtents(lineStr)
		g.DrawText1(px+popupW-lineExt.XAdvance-10, iy+fe.Ascent+4, lineStr)
	}

	// Scroll indicator
	if len(this.filtered) > this.maxVisible {
		scrollRatio := float64(this.scrollY) / float64(len(this.filtered)-this.maxVisible)
		scrollBarH := contentH * float64(this.maxVisible) / float64(len(this.filtered))
		if scrollBarH < 10 {
			scrollBarH = 10
		}
		scrollBarY := listY + scrollRatio*(contentH-scrollBarH)
		g.SetBrush1(paint.Color{R: 80, G: 80, B: 100, A: 180})
		g.Rectangle(px+popupW-5, scrollBarY, 3, scrollBarH)
		g.Fill()
	}
}

// --- Breadcrumb Drawing ---

// drawBreadcrumb draws the current function indicator bar at the top of the editor.
func (this *CodeEditor) drawBreadcrumb(g paint.Painter, topY, w float64) {
	h := this.breadcrumbHeight
	if h <= 0 {
		return
	}

	// Background: slightly different from editor bg
	g.SetBrush1(paint.Color{R: 36, G: 36, B: 42, A: 255})
	g.Rectangle(0, topY, w, h)
	g.Fill()

	// Bottom border
	g.SetPen1(paint.Color{R: 50, G: 50, B: 60, A: 255}, 1)
	g.Line(0, topY+h, w, topY+h)
	g.Stroke()

	fe := this.font.FontExtents()
	g.SetFont(this.font)

	// Determine current function
	sym := this.currentFunction()
	label := ""
	if sym != nil {
		if sym.Kind == SymMethod && sym.Receiver != "" {
			label = "func " + sym.Receiver + "." + sym.Name
		} else {
			label = "func " + sym.Name
		}
	}

	if label != "" {
		g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 220})
		g.DrawText1(this.gutterW+10, topY+fe.Ascent+(h-fe.Height)/2, label)
	} else {
		g.SetBrush1(paint.Color{R: 100, G: 100, B: 120, A: 140})
		g.DrawText1(this.gutterW+10, topY+fe.Ascent+(h-fe.Height)/2, "(top level)")
	}
}

// --- Go To Line Dialog ---

// drawGotoLine draws the go-to-line input bar at the top.
func (this *CodeEditor) drawGotoLine(g paint.Painter, w float64) {
	fbH := this.findBarHeight
	// Background
	g.SetBrush1(paint.Color{R: 50, G: 50, B: 60, A: 255})
	g.Rectangle(0, 0, w, fbH)
	g.Fill()
	// Bottom border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 85, A: 255}, 1)
	g.Line(0, fbH, w, fbH)
	g.Stroke()

	fe := this.font.FontExtents()
	g.SetFont(this.font)

	// Label
	label := fmt.Sprintf("Go to Line (1-%d):", len(this.lines))
	g.SetBrush1(paint.Color{R: 180, G: 180, B: 190, A: 255})
	g.DrawText1(10, fbH/2+fe.Ascent/2-1, label)

	// Input background
	labelExt := this.font.TextExtents(label)
	inputX := labelExt.XAdvance + 20
	inputW := 120.0
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(inputX, 4, inputW, fbH-8)
	g.Fill()
	g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
	g.Rectangle(inputX, 4, inputW, fbH-8)
	g.Stroke()

	// Input text
	g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
	if this.gotoLineText != "" {
		g.DrawText1(inputX+4, fbH/2+fe.Ascent/2-1, this.gotoLineText)
	}

	// Cursor
	prefix := this.gotoLineText
	fcx := inputX + 4 + this.measureText(prefix)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
	g.Rectangle(fcx, 6, 1, fbH-12)
	g.Fill()
}

// gotoLineAccept parses the go-to-line text and jumps to that line.
func (this *CodeEditor) gotoLineAccept() {
	if this.gotoLineText == "" {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(this.gotoLineText))
	if err != nil || n < 1 {
		return
	}
	// Convert 1-based to 0-based
	line := n - 1
	this.gotoLineActive = false
	this.gotoLineText = ""
	this.gotoLineCursor = 0
	this.goToLine(line)
}

// topOffset returns the total offset from the top of the editor
// accounting for find bar, go-to-line bar, and breadcrumb bar.
func (this *CodeEditor) topOffset() float64 {
	off := 0.0
	if this.findActive {
		off += this.findBarHeight
	}
	if this.gotoLineActive {
		off += this.findBarHeight
	}
	off += this.breadcrumbHeight
	return off
}

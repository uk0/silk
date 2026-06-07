package gui

import (
	"silk/paint"
	"sort"
	"strings"
)

// CompletionItemKind classifies the kind of completion suggestion.
const (
	CikKeyword  = 0
	CikType     = 1
	CikFunction = 2
	CikVariable = 3
)

// CompletionItem represents a single auto-completion suggestion.
type CompletionItem struct {
	Text   string
	Kind   int // CikKeyword, CikType, CikFunction, CikVariable
	Detail string
}

// CompletionPopup manages the auto-completion dropdown for a CodeEditor.
type CompletionPopup struct {
	items       []CompletionItem
	filtered    []CompletionItem
	selectedIdx int
	prefix      string
	visible     bool
	maxVisible  int
	scrollY     int
}

// Built-in completion source lists.
var completionKeywords []CompletionItem
var completionTypes []CompletionItem
var completionGuiItems []CompletionItem

func init() {
	// Go keywords
	kws := []string{
		"break", "case", "chan", "const", "continue", "default", "defer",
		"else", "fallthrough", "for", "func", "go", "goto", "if",
		"import", "interface", "map", "package", "range", "return",
		"select", "struct", "switch", "type", "var",
	}
	for _, k := range kws {
		completionKeywords = append(completionKeywords, CompletionItem{
			Text: k, Kind: CikKeyword, Detail: "keyword",
		})
	}

	// Go builtin types
	types := []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128",
		"bool", "byte", "rune", "error", "any",
	}
	for _, t := range types {
		completionTypes = append(completionTypes, CompletionItem{
			Text: t, Kind: CikType, Detail: "type",
		})
	}

	// gui.* prefixed items
	guiFuncs := []string{
		"gui.NewButton", "gui.NewLabel", "gui.NewEdit", "gui.NewCodeEditor",
		"gui.NewComboBox", "gui.NewCheckBox", "gui.NewRadioButton",
		"gui.NewScrollArea", "gui.NewTabWidget", "gui.NewSplitter",
		"gui.NewTreeView", "gui.NewListWidget", "gui.NewProgressBar",
		"gui.NewSlider", "gui.NewGroupBox", "gui.NewDialog",
		"gui.NewToolBar", "gui.NewStatusBar", "gui.NewMenu",
		"gui.Theme", "gui.SetThemeMode",
	}
	for _, f := range guiFuncs {
		completionGuiItems = append(completionGuiItems, CompletionItem{
			Text: f, Kind: CikFunction, Detail: "gui",
		})
	}
}

// NewCompletionPopup creates a new completion popup for the given editor.
func NewCompletionPopup(editor *CodeEditor) *CompletionPopup {
	p := &CompletionPopup{
		maxVisible: 8,
	}
	return p
}

// Show opens the completion popup with the given prefix, filtering candidates.
func (this *CompletionPopup) Show(prefix string, editor *CodeEditor) {
	this.prefix = prefix
	this.buildItems(editor)
	this.filter()
	if len(this.filtered) == 0 {
		this.visible = false
		return
	}
	this.visible = true
	this.selectedIdx = 0
	this.scrollY = 0
}

// Dismiss hides the completion popup.
func (this *CompletionPopup) Dismiss() {
	this.visible = false
}

// SelectNext moves selection down.
func (this *CompletionPopup) SelectNext() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectedIdx++
	if this.selectedIdx >= len(this.filtered) {
		this.selectedIdx = 0
	}
	// Scroll to keep selected visible
	if this.selectedIdx >= this.scrollY+this.maxVisible {
		this.scrollY = this.selectedIdx - this.maxVisible + 1
	}
	if this.selectedIdx < this.scrollY {
		this.scrollY = this.selectedIdx
	}
}

// SelectPrev moves selection up.
func (this *CompletionPopup) SelectPrev() {
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

// Accept inserts the selected completion into the editor.
func (this *CompletionPopup) Accept(editor *CodeEditor) {
	if !this.visible || len(this.filtered) == 0 {
		return
	}
	item := this.filtered[this.selectedIdx]
	this.visible = false

	// Replace the current prefix with the completion text
	prefixLen := len([]rune(this.prefix))
	if prefixLen > 0 {
		// Delete the prefix characters before cursor
		runes := []rune(editor.lines[editor.cursorLine])
		start := editor.cursorCol - prefixLen
		if start < 0 {
			start = 0
		}
		newRunes := append(runes[:start], runes[editor.cursorCol:]...)
		editor.lines[editor.cursorLine] = string(newRunes)
		editor.cursorCol = start
	}
	// Insert the completion text
	editor.insertTextAtCursor(item.Text)
	editor.rebuildText()
	editor.ensureCursorVisible()
}

// buildItems gathers all completion candidates from static lists and file identifiers.
func (this *CompletionPopup) buildItems(editor *CodeEditor) {
	this.items = nil

	// Static sources
	this.items = append(this.items, completionKeywords...)
	this.items = append(this.items, completionTypes...)
	this.items = append(this.items, completionGuiItems...)

	// Extract identifiers from current file
	seen := make(map[string]bool)
	for _, item := range this.items {
		seen[item.Text] = true
	}
	for _, line := range editor.lines {
		runes := []rune(line)
		i := 0
		for i < len(runes) {
			if isIdentStart(runes[i]) {
				start := i
				for i < len(runes) && isIdentPart(runes[i]) {
					i++
				}
				word := string(runes[start:i])
				if len(word) >= 2 && !seen[word] {
					seen[word] = true
					this.items = append(this.items, CompletionItem{
						Text: word, Kind: CikVariable, Detail: "identifier",
					})
				}
			} else {
				i++
			}
		}
	}
}

// filter narrows items by the current prefix using fuzzy subsequence matching
// (see RankCompletions / fuzzyMatch) and ranks survivors by score.
func (this *CompletionPopup) filter() {
	this.filtered = RankCompletions(this.items, this.prefix)
	// Limit to a reasonable number
	if len(this.filtered) > 50 {
		this.filtered = this.filtered[:50]
	}
}

// fuzzyMatch scores `query` against `candidate` and reports whether every rune
// of query appears in candidate in order (subsequence). A non-negative score is
// returned only when matched is true; higher means a stronger match.
//
// Scoring tiers (highest first):
//   - exact equality                  → 1_000_000
//   - case-insensitive exact equality →   900_000
//   - case-sensitive prefix           →   100_000 - len(candidate)
//   - case-insensitive prefix         →    50_000 - len(candidate)
//   - subsequence (in order)          → sum of per-char bonuses:
//       +12 per matched rune
//       +20 extra when the match is on a consecutive run with the previous one
//       +25 extra when the matched rune sits at a word boundary
//             (start of string, after '.' / '_' / '/', or camelCase transition)
//       +15 extra when the case matches exactly
//       -2 per skipped (gap) rune in candidate
//     plus a small -len(candidate)/4 penalty so shorter candidates rank higher
//     on otherwise-equal scores.
//
// Empty query is treated as "matches everything" with score 1 so callers can
// use the same filter path without a special case.
func fuzzyMatch(candidate, query string) (score int, matched bool) {
	if query == "" {
		return 1, true
	}
	if candidate == query {
		return 1_000_000, true
	}
	cLower := strings.ToLower(candidate)
	qLower := strings.ToLower(query)
	if cLower == qLower {
		return 900_000, true
	}
	if strings.HasPrefix(candidate, query) {
		s := 100_000 - len(candidate)
		if s < 1 {
			s = 1
		}
		return s, true
	}
	if strings.HasPrefix(cLower, qLower) {
		s := 50_000 - len(candidate)
		if s < 1 {
			s = 1
		}
		return s, true
	}

	// Subsequence walk over runes.
	cRunes := []rune(candidate)
	qRunes := []rune(query)
	qi := 0
	prevMatched := -2 // index in cRunes of last matched rune
	total := 0
	for ci := 0; ci < len(cRunes) && qi < len(qRunes); ci++ {
		cr := cRunes[ci]
		qr := qRunes[qi]
		if toLowerRune(cr) != toLowerRune(qr) {
			continue
		}
		bonus := 12
		if ci == prevMatched+1 {
			bonus += 20 // consecutive run
		}
		if isWordBoundary(cRunes, ci) {
			bonus += 25
		}
		if cr == qr {
			bonus += 15 // exact case
		}
		// Gap penalty: skipped chars since last match (excluding the consecutive case).
		if prevMatched >= 0 && ci > prevMatched+1 {
			bonus -= 2 * (ci - prevMatched - 1)
		}
		total += bonus
		prevMatched = ci
		qi++
	}
	if qi < len(qRunes) {
		return 0, false
	}
	// Length tie-breaker baked into the score: shorter wins.
	total -= len(cRunes) / 4
	if total < 1 {
		total = 1
	}
	return total, true
}

// RankCompletions filters items by fuzzyMatch against query and returns a new
// slice sorted by descending score; ties are broken by ascending candidate
// length, then by original input order (stable sort). An empty query returns
// a copy of items with their original ordering preserved.
func RankCompletions(items []CompletionItem, query string) []CompletionItem {
	if query == "" {
		out := make([]CompletionItem, len(items))
		copy(out, items)
		return out
	}
	type scored struct {
		item  CompletionItem
		score int
		order int
	}
	survivors := make([]scored, 0, len(items))
	for i, it := range items {
		s, ok := fuzzyMatch(it.Text, query)
		if !ok || s == 0 {
			continue
		}
		survivors = append(survivors, scored{item: it, score: s, order: i})
	}
	sort.SliceStable(survivors, func(i, j int) bool {
		if survivors[i].score != survivors[j].score {
			return survivors[i].score > survivors[j].score
		}
		li := len(survivors[i].item.Text)
		lj := len(survivors[j].item.Text)
		if li != lj {
			return li < lj
		}
		return survivors[i].order < survivors[j].order
	})
	out := make([]CompletionItem, len(survivors))
	for i, s := range survivors {
		out[i] = s.item
	}
	return out
}

// isWordBoundary reports whether the rune at index i in runes starts a new
// "word" for ranking purposes: start of string, after a separator
// ('.', '_', '/', '-', ' '), or a camelCase transition (lower→upper).
func isWordBoundary(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev := runes[i-1]
	switch prev {
	case '.', '_', '/', '-', ' ':
		return true
	}
	cur := runes[i]
	if cur >= 'A' && cur <= 'Z' && prev >= 'a' && prev <= 'z' {
		return true
	}
	return false
}

// toLowerRune is the ASCII fast-path lowercaser used by fuzzyMatch; falls back
// to a string-based path for non-ASCII so we still get correct unicode folding
// without pulling in unicode imports for hot scoring.
func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	if r < 128 {
		return r
	}
	return []rune(strings.ToLower(string(r)))[0]
}

// kindIcon returns a short label for the completion item kind.
func kindIcon(kind int) string {
	switch kind {
	case CikKeyword:
		return "K"
	case CikType:
		return "T"
	case CikFunction:
		return "F"
	case CikVariable:
		return "V"
	default:
		return " "
	}
}

// kindColor returns the color for each completion kind label.
func kindColor(kind int) paint.Color {
	switch kind {
	case CikKeyword:
		return paint.Color{R: 86, G: 156, B: 214, A: 255} // blue
	case CikType:
		return paint.Color{R: 78, G: 201, B: 176, A: 255} // teal
	case CikFunction:
		return paint.Color{R: 220, G: 220, B: 170, A: 255} // yellow
	case CikVariable:
		return paint.Color{R: 200, G: 200, B: 210, A: 255} // light gray
	default:
		return paint.Color{R: 180, G: 180, B: 180, A: 255}
	}
}

// drawPopup renders the completion popup on top of the editor content.
func (this *CompletionPopup) drawPopup(g paint.Painter, editor *CodeEditor) {
	if !this.visible || len(this.filtered) == 0 {
		return
	}

	fe := editor.font.FontExtents()
	lh := fe.Height + 2
	itemH := fe.Height + 6
	visibleCount := len(this.filtered)
	if visibleCount > this.maxVisible {
		visibleCount = this.maxVisible
	}

	popupW := 300.0
	popupH := float64(visibleCount) * itemH

	// Position below cursor
	editor.clampCursor()
	line := editor.lines[editor.cursorLine]
	lineRunes := []rune(line)
	prefixEnd := editor.cursorCol
	if prefixEnd > len(lineRunes) {
		prefixEnd = len(lineRunes)
	}
	cx := editor.gutterW + 10 - editor.scrollX + editor.measureText(string(lineRunes[:prefixEnd]))

	topOff := editor.topOffset()
	cy := float64(editor.cursorLine+1)*lh - editor.scrollY + topOff

	// Ensure popup doesn't go off-screen
	_, edH := editor.Size()
	edW, _ := editor.Size()
	if cx+popupW > edW {
		cx = edW - popupW - 5
	}
	if cx < editor.gutterW+5 {
		cx = editor.gutterW + 5
	}
	if cy+popupH > edH {
		// Show above cursor
		cy = float64(editor.cursorLine)*lh - editor.scrollY + topOff - popupH
	}

	// Background
	g.SetBrush1(paint.Color{R: 40, G: 40, B: 50, A: 245})
	g.Rectangle(cx, cy, popupW, popupH)
	g.Fill()

	// Border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 90, A: 255}, 1)
	g.Rectangle(cx, cy, popupW, popupH)
	g.Stroke()

	g.SetFont(editor.font)

	for vi := 0; vi < visibleCount; vi++ {
		idx := this.scrollY + vi
		if idx >= len(this.filtered) {
			break
		}
		item := this.filtered[idx]
		iy := cy + float64(vi)*itemH

		// Selection highlight
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 60, G: 100, B: 180, A: 255})
			g.Rectangle(cx+1, iy, popupW-2, itemH)
			g.Fill()
		}

		// Kind indicator
		g.SetBrush1(kindColor(item.Kind))
		g.DrawText1(cx+6, iy+fe.Ascent+3, kindIcon(item.Kind))

		// Item text
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		}
		g.DrawText1(cx+24, iy+fe.Ascent+3, item.Text)

		// Detail text (right-aligned)
		if item.Detail != "" {
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 255})
			detailExt := editor.font.TextExtents(item.Detail)
			g.DrawText1(cx+popupW-detailExt.XAdvance-8, iy+fe.Ascent+3, item.Detail)
		}
	}

	// Scroll indicator if needed
	if len(this.filtered) > this.maxVisible {
		scrollRatio := float64(this.scrollY) / float64(len(this.filtered)-this.maxVisible)
		scrollBarH := popupH * float64(this.maxVisible) / float64(len(this.filtered))
		if scrollBarH < 10 {
			scrollBarH = 10
		}
		scrollBarY := cy + scrollRatio*(popupH-scrollBarH)
		g.SetBrush1(paint.Color{R: 80, G: 80, B: 100, A: 180})
		g.Rectangle(cx+popupW-4, scrollBarY, 3, scrollBarH)
		g.Fill()
	}
}

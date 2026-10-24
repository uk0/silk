package gui

import (
	"context"
	"github.com/uk0/silk/paint"
	"math"
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
	apply  *completionApply
	Text   string
	Kind   int // CikKeyword, CikType, CikFunction, CikVariable
	Detail string
	Source string // optional provider name, shown in the popup footer
}

type completionApply struct{ run func(*CodeEditor) }

// CompletionPopup manages the auto-completion dropdown for a CodeEditor.
type CompletionPopup struct {
	items                    []CompletionItem
	filtered                 []CompletionItem
	selectedIdx              int
	prefix                   string
	visible                  bool
	maxVisible               int
	scrollY                  int
	pageRows                 int    // actual row capacity after viewport clipping
	status                   string // asynchronous provider feedback; never starts a timer
	open                     bool   // user intent, distinct from an empty/temporarily hidden result
	localReady, localPending bool
	localKey                 completionIndexRequest
	localGeneration          uint64
	localItems               []CompletionItem
	indexWorker              *completionIndexWorker
	indexPost                func(func()) // nil uses Post; injected dispatcher for deterministic tests
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
	this.open = true
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
	this.visible, this.open = false, false
	this.cancelLocal(false)
}

// SelectNext moves selection down.
func (this *CompletionPopup) SelectNext() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectIndex((this.selectedIdx + 1) % len(this.filtered))
}

// SelectPrev moves selection up.
func (this *CompletionPopup) SelectPrev() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectIndex((this.selectedIdx - 1 + len(this.filtered)) % len(this.filtered))
}

// Accept inserts the selected completion into the editor.
func (this *CompletionPopup) Accept(editor *CodeEditor) {
	if !this.visible || len(this.filtered) == 0 {
		return
	}
	item := this.filtered[this.selectedIdx]
	this.Dismiss()
	if item.apply != nil {
		item.apply.run(editor)
		return
	}

	// Prefix replacement must be one undoable edit, including the removed
	// prefix. Mutating lines before insertTextAtCursor lost the prefix on Undo.
	editor.clampCursor()
	line := editor.cursorLine
	runes := []rune(editor.lines[line])
	start := max(0, editor.cursorCol-len([]rune(this.prefix)))
	lines := append([]string(nil), editor.lines...)
	lines[line] = string(runes[:start]) + item.Text + string(runes[editor.cursorCol:])
	inserted := strings.Split(item.Text, "\n")
	col := len([]rune(inserted[len(inserted)-1]))
	if len(inserted) == 1 {
		col += start
	}
	editor.ReplaceAllTextAt(strings.Join(lines, "\n"), line+len(inserted)-1, col)
}

// buildItems gathers all completion candidates from static lists and file identifiers.
func (this *CompletionPopup) buildItems(editor *CodeEditor) {
	text := editor.Text()
	if len(text) > completionAsyncBytes {
		this.items = this.asynchronousItems(editor, text)
		return
	}
	this.cancelLocal(true)
	this.items, _ = scanCompletionItemsForLanguage(context.Background(), text, editor.Language())
}

// filter narrows items by the current prefix using fuzzy subsequence matching
// (see RankCompletions / fuzzyMatch) and ranks survivors by score.
func (this *CompletionPopup) filter() {
	this.filtered = RankCompletions(this.items, this.prefix)
	// Semantic provider edits are authoritative for this position. Local
	// identifier guesses include the fragment currently being typed (e.g. Pr),
	// whose exact fuzzy match would otherwise hide Println behind a no-op.
	// Keep fuzzy ordering within each group and preserve standalone ranking.
	sort.SliceStable(this.filtered, func(i, j int) bool {
		return this.filtered[i].apply != nil && this.filtered[j].apply == nil
	})
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
//     +12 per matched rune
//     +20 extra when the match is on a consecutive run with the previous one
//     +25 extra when the matched rune sits at a word boundary
//     (start of string, after '.' / '_' / '/', or camelCase transition)
//     +15 extra when the case matches exactly
//     -2 per skipped (gap) rune in candidate
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

	layout := this.layout(editor)
	if layout.rows == 0 {
		return
	}
	fe := editor.font.FontExtents()
	cx, cy, popupW, popupH := layout.x, layout.y, layout.w, layout.h
	itemH, visibleCount := layout.rowH, layout.rows
	g.Save()
	defer g.Restore()
	g.Rectangle(cx, cy, popupW, popupH)
	g.Clip()

	// Background
	g.SetBrush1(paint.Color{R: 40, G: 40, B: 50, A: 255})
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
		label, detail, detailX := completionRowText(editor.font, item, popupW)
		g.DrawText1(cx+24, iy+fe.Ascent+3, label)

		// Detail occupies only the remaining column, never the label or code.
		if detail != "" {
			g.SetBrush1(paint.Color{R: 170, G: 180, B: 200, A: 255})
			g.DrawText1(cx+detailX, iy+fe.Ascent+3, detail)
		}
	}

	// Scroll indicator if needed
	if len(this.filtered) > visibleCount {
		listH := float64(visibleCount) * itemH
		scrollRatio := float64(this.scrollY) / float64(len(this.filtered)-visibleCount)
		scrollBarH := listH * float64(visibleCount) / float64(len(this.filtered))
		if scrollBarH < 10 {
			scrollBarH = 10
		}
		scrollBarY := cy + scrollRatio*(listH-scrollBarH)
		g.SetBrush1(paint.Color{R: 80, G: 80, B: 100, A: 180})
		g.Rectangle(cx+popupW-4, scrollBarY, 3, scrollBarH)
		g.Fill()
	}
	// A fixed footer makes local fallback, provider waiting, and timeout
	// distinguishable without animation or an independent floating window.
	fy := cy + float64(visibleCount)*itemH
	g.SetPen1(paint.Color{R: 70, G: 70, B: 90, A: 255}, 1)
	g.MoveTo(cx, fy)
	g.LineTo(cx+popupW, fy)
	g.Stroke()
	g.SetBrush1(paint.Color{R: 170, G: 180, B: 200, A: 255})
	g.DrawText1(cx+6, fy+fe.Ascent+3, completionElide(editor.font, this.footerText(), popupW-12))

}

func completionRowText(font paint.Font, item CompletionItem, width float64) (label, detail string, detailX float64) {
	label = completionElide(font, strings.Join(strings.Fields(item.Text), " "), math.Max(0, width-32))
	start := 24 + completionTextWidth(font, label) + 16
	text := strings.Join(strings.Fields(item.Detail), " ")
	detail = completionElide(font, text, math.Max(0, width-8-start))
	detailX = math.Max(start, width-8-completionTextWidth(font, detail))
	return
}

// Fallback fonts can advance farther than their ink bounds (notably CJK on
// Windows). Reserve both, so right-aligned details cannot invade the label.
func completionTextWidth(font paint.Font, text string) float64 {
	e := font.TextExtents(text)
	return math.Max(e.XAdvance, e.XBearing+e.Width)
}

func completionElide(font paint.Font, text string, width float64) string {
	if completionTextWidth(font, text) <= width {
		return text
	}
	if completionTextWidth(font, elideMark) > width {
		return ""
	}
	// Measure only logarithmically many prefixes of long server signatures.
	// Retain a measured, rune-safe candidate rather than estimating glyph widths.
	runes := []rune(text)
	lo, hi := 0, len(runes)
	fit := elideMark
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		candidate := string(runes[:mid]) + elideMark
		if completionTextWidth(font, candidate) <= width {
			lo, fit = mid, candidate
		} else {
			hi = mid
		}
	}
	return fit
}

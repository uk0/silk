package gui

// Visual lines and syntax-aware folding — the two pieces of editor layout that
// the CodeEditor's Draw path currently approximates:
//
//  1. Word wrap. CodeEditor.wordWrap only paints a "»" overflow marker at the
//     right edge (see the wordWrap branch in Draw): a long line still occupies
//     exactly one screen row and scrolls out of view. WrapLayout is the missing
//     model — it maps every source line to the sequence of visual rows that
//     display it, with a lossless mapping in both directions so a caret, a
//     selection or a scroll offset can be expressed in either space.
//
//  2. Folding. computeFoldRegions is a raw rune scan: it counts '{' and '}'
//     inside strings, runes and comments, it only opens a region when '{' is the
//     last visible rune of the line, and it knows nothing about import groups or
//     comment blocks. ScanGoFoldRegions tokenizes with go/scanner instead, so
//     literals and comments cannot contribute braces, a mid-line '{' still opens
//     a block, and import/comment folds fall out of the same pass.
//
// Both halves are pure: no font metrics, no graphics context, no CodeEditor
// state. Wiring them into Draw is a separate step; nothing here touches the
// existing foldRegion/computeFoldRegions API.

import (
	"go/scanner"
	gotoken "go/token"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Wrap layout
// ---------------------------------------------------------------------------

// WrapWidthFunc reports the display width of one rune, in the same unit as the
// max width handed to NewWrapLayout. Passing nil selects unitRuneWidth, which
// returns 1 for every rune — the max width is then a plain column count. A
// caller holding a real font can pass a metric-based function later without
// touching the wrap algorithm; a tab is just another rune to this function.
type WrapWidthFunc func(r rune) float64

// unitRuneWidth is the character-cell default: every rune is one column wide.
func unitRuneWidth(rune) float64 { return 1 }

// VisualRow is one on-screen row of a wrapped view: the source runes
// [StartCol, EndCol) of source line Line. The rows of a line partition that
// line exactly — no rune is dropped and none is duplicated — which is what
// makes SourceToVisual/VisualToSource lossless.
type VisualRow struct {
	Line     int
	StartCol int
	EndCol   int
}

// WrapLayout is the wrap of a whole buffer: source lines in, visual rows out.
// Build cost is O(total runes) and the result is immutable, so rebuild it when
// the text or the wrap width changes. The layout keeps a reference to the lines
// slice it was given rather than copying it; feeding it a slice that is mutated
// afterwards yields stale rows (RowText degrades gracefully, it never panics).
type WrapLayout struct {
	lines    []string
	rows     []VisualRow
	firstRow []int // source line -> index of its first row in rows
	lineRows []int // source line -> number of rows it occupies (>= 1)
}

// NewWrapLayout wraps lines so that no row exceeds maxWidth, measuring runes
// with widthOf (nil = one column per rune). A maxWidth <= 0 disables wrapping:
// every source line becomes exactly one visual row, which is the "wrap off"
// layout and lets a caller use the same mapping in both modes.
func NewWrapLayout(lines []string, maxWidth float64, widthOf WrapWidthFunc) *WrapLayout {
	if widthOf == nil {
		widthOf = unitRuneWidth
	}
	this := &WrapLayout{
		lines:    lines,
		rows:     make([]VisualRow, 0, len(lines)),
		firstRow: make([]int, len(lines)),
		lineRows: make([]int, len(lines)),
	}
	for i, line := range lines {
		this.firstRow[i] = len(this.rows)
		this.appendRows(i, []rune(line), maxWidth, widthOf)
		this.lineRows[i] = len(this.rows) - this.firstRow[i]
	}
	return this
}

// appendRows breaks one source line into rows. Greedy fill: consume runes until
// the next non-blank rune would overflow, then cut at the last word boundary
// inside the row; when the row contains no boundary — a single token wider than
// maxWidth — cut hard at the overflow point so progress is always made. Blanks
// are exempt from the overflow test, so a trailing space overhangs the right
// margin instead of being pushed onto the next row as a leading space.
func (this *WrapLayout) appendRows(line int, runes []rune, maxWidth float64, widthOf WrapWidthFunc) {
	if len(runes) == 0 || maxWidth <= 0 {
		this.rows = append(this.rows, VisualRow{Line: line, StartCol: 0, EndCol: len(runes)})
		return
	}
	start := 0
	for start < len(runes) {
		width := 0.0
		lastBreak := -1 // exclusive end of the last word boundary seen in this row
		i := start
		for ; i < len(runes); i++ {
			blank := isWrapBlank(runes[i])
			w := widthOf(runes[i])
			if !blank && i > start && width+w > maxWidth {
				break
			}
			width += w
			// A break opportunity sits after a run of blanks.
			if blank && i+1 < len(runes) && !isWrapBlank(runes[i+1]) {
				lastBreak = i + 1
			}
		}
		end := i
		if end < len(runes) && lastBreak > start {
			end = lastBreak
		}
		this.rows = append(this.rows, VisualRow{Line: line, StartCol: start, EndCol: end})
		start = end
	}
}

// isWrapBlank reports whether r is an intra-line blank, i.e. a rune a row may
// break after and may overhang the right margin with.
func isWrapBlank(r rune) bool { return r == ' ' || r == '\t' }

// RowCount is the number of visual rows in the layout — the scrollable height
// of the wrapped view, in rows.
func (this *WrapLayout) RowCount() int { return len(this.rows) }

// LineCount is the number of source lines the layout was built from.
func (this *WrapLayout) LineCount() int { return len(this.lines) }

// Row returns the row at index i, or false when i is out of range.
func (this *WrapLayout) Row(i int) (VisualRow, bool) {
	if i < 0 || i >= len(this.rows) {
		return VisualRow{}, false
	}
	return this.rows[i], true
}

// RowsForLine returns the index of a source line's first visual row and the
// number of rows it occupies. count is 0 when line is out of range.
func (this *WrapLayout) RowsForLine(line int) (first, count int) {
	if line < 0 || line >= len(this.lines) {
		return 0, 0
	}
	return this.firstRow[line], this.lineRows[line]
}

// RowText returns the slice of source text drawn on a visual row, or "" when
// the row index is out of range.
func (this *WrapLayout) RowText(row int) string {
	r, ok := this.Row(row)
	if !ok {
		return ""
	}
	runes := []rune(this.lines[r.Line])
	start, end := r.StartCol, r.EndCol
	if start > len(runes) {
		return ""
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}

// SourceToVisual maps a source position — line index plus rune offset — to the
// visual row and the rune offset within that row. Out-of-range inputs are
// clamped into the buffer. A position that falls exactly on a wrap boundary is
// reported on the continuation row at offset 0, which is where a caret moving
// forward through the wrap lands.
func (this *WrapLayout) SourceToVisual(line, col int) (row, rowCol int) {
	if len(this.rows) == 0 {
		return 0, 0
	}
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	first, count := this.RowsForLine(line)
	if count == 0 {
		return 0, 0
	}
	last := first + count - 1
	if col < 0 {
		col = 0
	}
	if col > this.rows[last].EndCol {
		col = this.rows[last].EndCol
	}
	for i := first; i <= last; i++ {
		if col < this.rows[i].EndCol {
			return i, col - this.rows[i].StartCol
		}
	}
	return last, col - this.rows[last].StartCol
}

// VisualToSource maps a visual row plus a rune offset within it back to the
// source line and rune offset it stands for. Out-of-range inputs are clamped —
// an offset past the end of the row clamps to the row's end — so
// VisualToSource(SourceToVisual(line, col)) == (line, col) for every in-range
// source position.
func (this *WrapLayout) VisualToSource(row, rowCol int) (line, col int) {
	if len(this.rows) == 0 {
		return 0, 0
	}
	if row < 0 {
		row = 0
	}
	if row >= len(this.rows) {
		row = len(this.rows) - 1
	}
	r := this.rows[row]
	if rowCol < 0 {
		rowCol = 0
	}
	col = r.StartCol + rowCol
	if col > r.EndCol {
		col = r.EndCol
	}
	return r.Line, col
}

// ---------------------------------------------------------------------------
// Syntax-aware fold regions
// ---------------------------------------------------------------------------

// FoldKind classifies what a fold region encloses.
type FoldKind string

const (
	FoldKindBlock   FoldKind = "block"   // a { ... } brace block
	FoldKindImport  FoldKind = "import"  // an import ( ... ) group
	FoldKindComment FoldKind = "comment" // a run of whole-line comments
)

// SyntaxFoldRegion is a foldable span of source lines. Both bounds are 0-based
// and inclusive, and only multi-line spans are produced (EndLine > StartLine).
type SyntaxFoldRegion struct {
	StartLine int
	EndLine   int
	Kind      FoldKind
}

// ScanGoFoldRegionsLines is ScanGoFoldRegions over an editor line buffer. It
// carries the same pathological-input guard as computeFoldRegions: at or above
// maxFoldComputeLines it returns nil, so a giant buffer loses folding rather
// than paying for a full tokenization on every Draw.
func ScanGoFoldRegionsLines(lines []string) []SyntaxFoldRegion {
	if len(lines) >= maxFoldComputeLines {
		return nil
	}
	return ScanGoFoldRegions(strings.Join(lines, "\n"))
}

// ScanGoFoldRegions tokenizes src with go/scanner and returns every foldable
// region, ordered by StartLine with the outer region first on a tie:
//
//   - FoldKindBlock — a matched '{' ... '}' pair spanning more than one line.
//     Because the input is tokenized, braces inside string literals, raw
//     strings, rune literals and comments do not count, and a '{' that is not
//     the last thing on its line still opens a block.
//   - FoldKindImport — an "import ( ... )" group spanning more than one line.
//   - FoldKindComment — a run of comments on consecutive lines, each starting
//     its own line. A comment trailing code neither starts nor extends a run; a
//     blank line ends one. A single /* ... */ comment spanning lines is a run of
//     its own.
//
// Malformed input is tolerated: scan errors are swallowed and whatever regions
// were closed before and after the bad token are still returned. Unbalanced
// openers are dropped, stray closers are ignored.
func ScanGoFoldRegions(src string) []SyntaxFoldRegion {
	lines := strings.Split(src, "\n")
	fset := gotoken.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	// Swallow errors: a half-typed buffer is the normal state of an open editor,
	// and the tokens around the offending rune are still worth folding.
	s.Init(file, []byte(src), func(gotoken.Position, string) {}, scanner.ScanComments)

	// openParen remembers whether a '(' belongs to an import group, so nested
	// parens inside the group cannot be mistaken for its closer.
	type openParen struct {
		line     int
		isImport bool
	}
	var (
		braces      []int // line of each unmatched '{'
		parens      []openParen
		regions     []SyntaxFoldRegion
		afterImport bool // the previous non-comment token was `import`
		runStart    = -1 // first line of the comment run being accumulated
		runEnd      = -1 // last line of that run
	)
	flushComments := func() {
		if runStart >= 0 && runEnd > runStart {
			regions = append(regions, SyntaxFoldRegion{
				StartLine: runStart, EndLine: runEnd, Kind: FoldKindComment,
			})
		}
		runStart, runEnd = -1, -1
	}

	for {
		pos, tok, lit := s.Scan()
		if tok == gotoken.EOF {
			break
		}
		p := file.Position(pos)
		line := p.Line - 1

		if tok == gotoken.COMMENT {
			// A block comment's literal carries its newlines, so counting them
			// gives the last line it covers.
			end := line + strings.Count(lit, "\n")
			own := startsLine(lines, line, p.Column)
			switch {
			case own && runStart >= 0 && line == runEnd+1:
				runEnd = end
			case own:
				flushComments()
				runStart, runEnd = line, end
			default:
				flushComments()
			}
			continue
		}
		flushComments()

		switch tok {
		case gotoken.LBRACE:
			braces = append(braces, line)
		case gotoken.RBRACE:
			if n := len(braces); n > 0 {
				start := braces[n-1]
				braces = braces[:n-1]
				if line > start {
					regions = append(regions, SyntaxFoldRegion{
						StartLine: start, EndLine: line, Kind: FoldKindBlock,
					})
				}
			}
		case gotoken.LPAREN:
			parens = append(parens, openParen{line: line, isImport: afterImport})
		case gotoken.RPAREN:
			if n := len(parens); n > 0 {
				op := parens[n-1]
				parens = parens[:n-1]
				if op.isImport && line > op.line {
					regions = append(regions, SyntaxFoldRegion{
						StartLine: op.line, EndLine: line, Kind: FoldKindImport,
					})
				}
			}
		}
		afterImport = tok == gotoken.IMPORT
	}
	flushComments()

	sort.SliceStable(regions, func(a, b int) bool {
		if regions[a].StartLine != regions[b].StartLine {
			return regions[a].StartLine < regions[b].StartLine
		}
		return regions[a].EndLine > regions[b].EndLine
	})
	return regions
}

// startsLine reports whether a token at 1-based byte column col of 0-based line
// is the first thing on that line (only whitespace to its left).
func startsLine(lines []string, line, col int) bool {
	if line < 0 || line >= len(lines) {
		return false
	}
	if col < 1 {
		return true
	}
	prefix := lines[line]
	if col-1 < len(prefix) {
		prefix = prefix[:col-1]
	}
	return strings.TrimSpace(prefix) == ""
}

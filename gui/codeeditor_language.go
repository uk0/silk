package gui

import (
	"slices"
	"sort"
	"unicode/utf8"
)

// SyntaxKind selects the editor's theme-aware palette, including custom colors.
type SyntaxKind int

const (
	SyntaxKeyword SyntaxKind = iota
	SyntaxString
	SyntaxComment
	SyntaxNumber
	SyntaxType
	SyntaxFunction
)

// SyntaxHighlight coordinates are zero-based Unicode rune columns, never bytes
// or UTF-16. One range must remain on one source line. End is exclusive.
type SyntaxHighlight struct {
	Line, StartCol, EndCol int
	Kind                   SyntaxKind
}

// SetSyntaxHighlights installs an independent semantic overlay for an exact
// text revision. Stale snapshots are rejected and old overlays are never used
// after an edit. Call only on the UI thread. Overlapping/invalid ranges are
// discarded deterministically; oversized batches are rejected as a whole.
func (e *CodeEditor) SetSyntaxHighlights(revision uint64, spans []SyntaxHighlight) bool {
	if revision != e.TextRevision() || len(spans) > 100000 {
		return false
	}
	copySpans := append([]SyntaxHighlight(nil), spans...)
	sort.SliceStable(copySpans, func(i, j int) bool {
		a, b := copySpans[i], copySpans[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.StartCol < b.StartCol
	})
	e.syntaxHighlights = map[int][]SyntaxHighlight{}
	lineWidths := map[int]int{}
	for _, s := range copySpans {
		if s.Line < 0 || s.Line >= len(e.lines) || len(e.lines[s.Line]) >= maxHighlightLineLength || s.StartCol < 0 || s.EndCol <= s.StartCol || s.Kind < SyntaxKeyword || s.Kind > SyntaxFunction {
			continue
		}
		width, known := lineWidths[s.Line]
		if !known {
			width = utf8.RuneCountInString(e.lines[s.Line])
			lineWidths[s.Line] = width
		}
		if s.EndCol > width {
			continue
		}
		row := e.syntaxHighlights[s.Line]
		if len(row) > 0 && s.StartCol < row[len(row)-1].EndCol {
			continue
		}
		e.syntaxHighlights[s.Line] = append(row, s)
	}
	e.syntaxRevision = revision
	e.clearTokenCache()
	e.Update()
	return true
}
func (e *CodeEditor) overlayTokens(line int, tokens []token) []token {
	spans := e.syntaxHighlights[line]
	if e.syntaxRevision != e.TextRevision() || len(spans) == 0 {
		return tokens
	}
	kinds := []tokenType{tokKeyword, tokString, tokComment, tokNumber, tokType, tokFunction}
	var out []token
	col, si := 0, 0
	// Split only at semantic boundaries, preserving every source byte and the
	// tokenizer's original classifications outside the overlay.
	for _, t := range tokens {
		runes := []rune(t.text)
		for start := 0; start < len(runes); {
			for si < len(spans) && spans[si].EndCol <= col {
				si++
			}
			end, kind := len(runes), t.typ
			if si < len(spans) {
				s := spans[si]
				if col >= s.StartCol {
					kind = kinds[s.Kind]
					end = min(end, start+s.EndCol-col)
				} else {
					end = min(end, start+s.StartCol-col)
				}
			}
			out = append(out, token{text: string(runes[start:end]), typ: kind})
			col += end - start
			start = end
		}
	}
	return out
}

// SetAdditionalSymbols adds language-provider symbols to the built-in Go
// outline. As with highlights, ownership is copied and stale revisions rejected.
func (e *CodeEditor) SetAdditionalSymbols(revision uint64, symbols []CodeSymbol) bool {
	if revision != e.TextRevision() || len(symbols) > 100000 {
		return false
	}
	var accepted []CodeSymbol
	for _, s := range symbols {
		if s.Line >= 0 && s.Line < len(e.lines) && s.Name != "" {
			accepted = append(accepted, s)
		}
	}
	if e.additionalSymbolsRevision == revision && slices.Equal(e.additionalSymbols, accepted) {
		return true
	}
	e.additionalSymbols = accepted
	e.additionalSymbolsRevision = revision
	e.symbolsRevision++
	e.symbolCache.valid = false
	e.Update()
	return true
}

// SymbolsRevision also changes when an asynchronous provider publishes symbols
// for unchanged text. UI-thread consumers can check it in O(1) without parsing.
func (e *CodeEditor) SymbolsRevision() uint64 {
	e.Text()
	return e.symbolsRevision
}

// LanguageHintRevision is a UI-thread token for dropping delayed hover and
// signature responses after input, navigation, focus loss or pointer departure.
func (e *CodeEditor) LanguageHintRevision() uint64 { return e.languageHintRevision }

func (e *CodeEditor) DismissLanguageHints() {
	e.languageHintRevision++
	e.hoverReqLine, e.hoverReqCol = -1, -1
	e.hoverErrorLine = -1
	HideToolTipFor(e.Self())
}

func (e *CodeEditor) OnFocusChanged(newFocusWidget, oldFocusWidget IWidget) {
	if newFocusWidget != e.Self() {
		e.DismissLanguageHints()
	}
	e.Widget.OnFocusChanged(newFocusWidget, oldFocusWidget)
}

package gui

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type goOutlineSpan struct{ start, end int }
type goOutlineMetadata struct {
	valid    bool
	comments []goOutlineSpan // line-comment payloads only; never the // delimiter
	headers  []goOutlineSpan // function/receiver details can include comment text
}

func (m *goOutlineMetadata) finish() {
	if !m.valid {
		m.comments, m.headers = nil, nil
		return
	}
	// Both streams are in physical source order. Never do a quadratic walk
	// over every function for every comment in a generated/annotated document.
	out, header := m.comments[:0], 0
	for _, comment := range m.comments {
		for header < len(m.headers) && m.headers[header].end <= comment.start {
			header++
		}
		if header < len(m.headers) && m.headers[header].start < comment.end && comment.start < m.headers[header].end {
			continue
		}
		out = append(out, comment)
	}
	m.comments, m.headers = out, nil
}

// Worker-owned incremental projection, never a mutable editor buffer or AST.
// A successful parse records where line-comment payload edits cannot change
// ANY exposed symbol field. Every structural/uncertain edit falls back to the
// original tolerant AST parser; this is not a substitute regex parser.
type goSymbolIndex struct {
	valid    bool
	text     string
	symbols  []CodeSymbol // immutable; may also be held by a delivered cache
	comments []goOutlineSpan
}

// A delivered projection is immutable. The UI can advance a proven trivia
// edit immediately, without reading/mutating the worker's private index or
// waiting through another background debounce/placeholder frame.
type goSymbolProjection struct {
	text     string
	comments []goOutlineSpan
}

func (p *goSymbolIndex) projection() *goSymbolProjection {
	if !p.valid {
		return nil
	}
	return &goSymbolProjection{p.text, slices.Clone(p.comments)}
}

func (p *goSymbolProjection) advance(text string) *goSymbolProjection {
	i, delta, ok := goCommentEdit(p.text, text, p.comments)
	if !ok {
		return nil
	}
	if i < 0 {
		return p
	}
	comments := slices.Clone(p.comments)
	rebaseGoComments(comments, i, delta)
	return &goSymbolProjection{text, comments}
}

func (p *goSymbolIndex) parse(text string) []CodeSymbol {
	if p.reuseCommentEdit(text) {
		return p.symbols
	}
	var metadata goOutlineMetadata
	symbols := symbolsFromGoDeclarations(parseGoDeclarations(text, &metadata))
	*p = goSymbolIndex{}
	if metadata.valid {
		comments := metadata.comments[:0]
		for _, span := range metadata.comments {
			if !goOutlineDirective(text[span.start:span.end]) {
				comments = append(comments, span)
			}
		}
		p.valid, p.text, p.symbols, p.comments = true, text, symbols, comments
	}
	return symbols
}

func (p *goSymbolIndex) reuseCommentEdit(text string) bool {
	if !p.valid {
		return false
	}
	i, delta, ok := goCommentEdit(p.text, text, p.comments)
	if !ok {
		return false
	}
	if i >= 0 {
		rebaseGoComments(p.comments, i, delta)
	}
	p.text = text
	return true
}

func goCommentEdit(old, text string, comments []goOutlineSpan) (int, int, bool) {
	if text == old {
		return -1, 0, true
	}
	start := 0
	for start < len(text) && start < len(old) && text[start] == old[start] {
		start++
	}
	oldEnd, newEnd := len(old), len(text)
	for oldEnd > start && newEnd > start && old[oldEnd-1] == text[newEnd-1] {
		oldEnd--
		newEnd--
	}
	i := sort.Search(len(comments), func(i int) bool { return comments[i].end >= start })
	if i == len(comments) || start < comments[i].start || oldEnd > comments[i].end ||
		strings.ContainsAny(old[start:oldEnd], "\r\n") || strings.ContainsAny(text[start:newEnd], "\r\n") {
		return 0, 0, false
	}
	delta := len(text) - len(old)
	payload := text[comments[i].start : comments[i].end+delta]
	// Directives can change scanner/parser error recovery even without new
	// lines. Invalid encoding, NUL and interior BOMs can do the same. They
	// therefore use the full parser rather than an assumed trivia fast path.
	if goOutlineDirective(payload) || !utf8.ValidString(payload) || strings.ContainsAny(payload, "\x00\ufeff") {
		return 0, 0, false
	}
	// Tokens outside the existing line comment are byte-for-byte unchanged,
	// physical line numbers do not move, and no signature includes this range.
	// Unicode, braces, quotes and /* */ within its payload remain comment text.
	return i, delta, true
}

func rebaseGoComments(comments []goOutlineSpan, i, delta int) {
	comments[i].end += delta
	for j := i + 1; j < len(comments); j++ {
		comments[j].start += delta
		comments[j].end += delta
	}
}

func goOutlineDirective(payload string) bool {
	return strings.HasPrefix(payload, "line") || strings.HasPrefix(payload, "go:")
}

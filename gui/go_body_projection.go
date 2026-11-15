package gui

import (
	"go/scanner"
	gotoken "go/token"
	"sort"
	"strings"
)

// Limit synchronous lexical work and immutable span copying, not language
// features. Larger/uncertain edits keep the existing synchronous-small or
// bounded-background-large AST fallback.
const goBodyProjectionUIBytes = 8 << 10
const goProjectionUISpans = 16 << 10 // at most 256 KiB of span copies on 64-bit

// goBodyEdit re-lexes the entire candidate body with the same Go scanner as
// outline recovery. It must close exactly at the original outer delimiter,
// report no lexical errors and preserve physical line numbers. Strings,
// comments, nested literals and closures are tokens, not brace substrings.
// The proof concerns exposed outline fields, NOT body type checking.
func goBodyEdit(old, text string, bodies []goOutlineSpan, maxBytes int) (int, int, bool) {
	start := 0
	for start < len(old) && start < len(text) && old[start] == text[start] {
		start++
	}
	oldEnd, newEnd := len(old), len(text)
	for oldEnd > start && newEnd > start && old[oldEnd-1] == text[newEnd-1] {
		oldEnd--
		newEnd--
	}
	i := sort.Search(len(bodies), func(i int) bool { return bodies[i].end >= start })
	if i == len(bodies) || start < bodies[i].start || oldEnd > bodies[i].end || strings.ContainsAny(old[start:oldEnd], "\r\n") || strings.ContainsAny(text[start:newEnd], "\r\n") {
		return 0, 0, false
	}
	delta := len(text) - len(old)
	body := text[bodies[i].start : bodies[i].end+delta]
	if maxBytes > 0 && len(body) > maxBytes {
		return 0, 0, false
	}
	wrapped := make([]byte, len(body)+2)
	wrapped[0] = '{'
	copy(wrapped[1:], body)
	wrapped[len(wrapped)-1] = '}'
	var scan scanner.Scanner
	file := gotoken.NewFileSet().AddFile("", -1, len(wrapped))
	valid := true
	scan.Init(file, wrapped, func(gotoken.Position, string) { valid = false }, 0)
	depth := 0
	closed := false
	for {
		pos, tok, _ := scan.Scan()
		if tok == gotoken.EOF {
			break
		}
		switch tok {
		case gotoken.LBRACE:
			depth++
		case gotoken.RBRACE:
			depth--
			if depth == 0 {
				if file.Offset(pos) != len(wrapped)-1 {
					return 0, 0, false
				}
				closed = true
			}
		}
		if depth < 0 {
			return 0, 0, false
		}
	}
	return i, delta, valid && closed && depth == 0
}
func rebaseBodySpans(spans []goOutlineSpan, start, end, delta int) {
	for i := range spans {
		if spans[i].start <= start && spans[i].end >= end {
			spans[i].end += delta
		} else if spans[i].start >= end {
			spans[i].start += delta
			spans[i].end += delta
		}
	}
}
func rebaseBodyComments(comments []goOutlineSpan, body goOutlineSpan, delta int) []goOutlineSpan {
	out := comments[:0]
	for _, c := range comments {
		if c.start >= body.start && c.end <= body.end {
			continue
		}
		if c.start >= body.end {
			c.start += delta
			c.end += delta
		}
		out = append(out, c)
	}
	return out
}

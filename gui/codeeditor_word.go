package gui

import "unicode/utf8"

// WordAtCursor returns the identifier-like word at (or immediately before) the
// primary caret without moving it. This is lexical, not a semantic symbol test:
// digits, keywords and words inside comments are also returned. Rune/byte/UTF-16
// positions stay unchanged. It scans only the current prefix and adjacent word,
// allocates nothing, and is UI-thread-only like other editor accessors. Columns
// clamp to the line bounds for lookup only; invalid lines return an empty word.
func (e *CodeEditor) WordAtCursor() string {
	line, ok := e.LineText(e.cursorLine)
	if !ok {
		return ""
	}
	p := textPositionInLine(line, e.cursorCol)
	at := p.ByteColumn
	if at > 0 {
		r, _ := utf8.DecodeRuneInString(line[at:])
		if at == len(line) || !isIdentPart(r) {
			_, n := utf8.DecodeLastRuneInString(line[:at])
			at -= n
		}
	}
	return wordAtBytePosition(line, at)
}
func wordAtBytePosition(line string, at int) string {
	if at < 0 || at >= len(line) {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(line[at:])
	if !isIdentPart(r) {
		return ""
	}
	start, end := at, at
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:start])
		if !isIdentPart(r) {
			break
		}
		start -= n
	}
	for end < len(line) {
		r, n := utf8.DecodeRuneInString(line[end:])
		if !isIdentPart(r) {
			break
		}
		end += n
	}
	return line[start:end]
}

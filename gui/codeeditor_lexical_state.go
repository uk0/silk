package gui

import "strings"

// Only block comments and raw strings survive a Go source line boundary.
// This metadata belongs to the UI thread, just like the document and tokens.
type goLexState uint8

const (
	goLexNormal goLexState = iota
	goLexBlock
	goLexRaw
)

func goStateFromBlock(block bool) goLexState {
	if block {
		return goLexBlock
	}
	return goLexNormal
}

// scanGoLineState skips bytes without allocating tokens or decoding runes.
// Go delimiters are ASCII; UTF-8 continuation bytes cannot masquerade as one.
// Invalid/unclosed quoted strings and rune literals stop at the line boundary,
// but an unclosed raw string or block comment continues onto the next line.
func scanGoLineState(line string, state goLexState) goLexState {
	for i := 0; i < len(line); {
		switch state {
		case goLexBlock:
			end := strings.Index(line[i:], "*/")
			if end < 0 {
				return goLexBlock
			}
			i += end + 2
			state = goLexNormal
		case goLexRaw:
			end := strings.IndexByte(line[i:], '`')
			if end < 0 {
				return goLexRaw
			}
			i += end + 1
			state = goLexNormal
		default:
			switch line[i] {
			case '/':
				if i+1 < len(line) {
					if line[i+1] == '/' {
						return goLexNormal
					}
					if line[i+1] == '*' {
						state = goLexBlock
						i += 2
						continue
					}
				}
				i++
			case '`':
				state = goLexRaw
				i++
			case '"', '\'':
				quote := line[i]
				i++
				for i < len(line) {
					ch := line[i]
					i++
					if ch == '\\' && i < len(line) {
						i++
					} else if ch == quote {
						break
					}
				}
			default:
				i++
			}
		}
	}
	return state
}

type goLineState struct {
	text string
	in   goLexState
	out  goLexState
}

type goLineStateCache struct{ lines []goLineState }

// update reuses the common prefix/suffix, rescanning changed lines and then
// propagating until the incoming state converges. String comparisons remain
// O(line count) per publication; stable Draw does no prefix work at all.
// The result counts rescanned lines for regression tests, not paint telemetry.
func (c *goLineStateCache) update(lines []string) int {
	oldLen, newLen := len(c.lines), len(lines)
	prefix := 0
	for prefix < oldLen && prefix < newLen && c.lines[prefix].text == lines[prefix] {
		// Rebind equal bytes too: replacing a buffer must not pin its old backing
		// storage just because these particular lines have not changed.
		c.lines[prefix].text = lines[prefix]
		prefix++
	}
	oldEnd, newEnd := oldLen, newLen
	for oldEnd > prefix && newEnd > prefix && c.lines[oldEnd-1].text == lines[newEnd-1] {
		oldEnd--
		newEnd--
		c.lines[oldEnd].text = lines[newEnd]
	}
	if newLen > oldLen {
		c.lines = append(c.lines, make([]goLineState, newLen-oldLen)...)
	}
	copy(c.lines[newEnd:newLen], c.lines[oldEnd:oldLen])
	if newLen < oldLen {
		clear(c.lines[newLen:])
		c.lines = c.lines[:newLen]
	}
	state := goLexNormal
	if prefix > 0 {
		state = c.lines[prefix-1].out
	}
	scanned := 0
	for i := prefix; i < newLen; i++ {
		if i >= newEnd && c.lines[i].in == state {
			break // unchanged suffix, and its dependency has converged
		}
		next := scanGoLineState(lines[i], state)
		c.lines[i] = goLineState{text: lines[i], in: state, out: next}
		state = next
		scanned++
	}
	return scanned
}

func (e *CodeEditor) updateGoLineStates() {
	if !e.usesGoSyntax() {
		e.goLineStates = goLineStateCache{}
		return
	}
	e.goLineStates.update(e.lines)
}

// tokensForLine uses source indices, not the order in which visible rows are
// drawn. Folding, scrolling and minimap rendering therefore share the exact
// same state, including delimiters on hidden or very long lines.
func (e *CodeEditor) tokensForLine(idx int) []token {
	e.Text() // publish any pending low-level splice before reading its metadata
	state := goLexNormal
	if idx < len(e.goLineStates.lines) {
		state = e.goLineStates.lines[idx].in
	}
	tokens, _ := e.tokenizeGoLineCached(idx, e.lines[idx], state)
	return tokens
}

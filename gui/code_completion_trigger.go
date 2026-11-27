package gui

import (
	"unicode"
	"unicode/utf8"
)

// Match only a short suffix. A minified file or a very long generated
// identifier should not make automatic probing scan the entire line.
const maxAutomaticCompletionWordBytes = 1024

// AutomaticCodeCompletionPrefix reports the identifier prefix for an automatic
// completion at a UTF-8 byte column. A member-access dot returns an empty useful
// prefix; number literals (including decimal/exponent fragments) do not open
// suggestions. This does not apply to explicit/manual completion requests.
func AutomaticCodeCompletionPrefix(line string, byteColumn int) (prefix string, ok bool) {
	if byteColumn <= 0 || byteColumn > len(line) || (byteColumn < len(line) && !utf8.RuneStart(line[byteColumn])) {
		return "", false
	}
	end := byteColumn
	r, _ := utf8.DecodeLastRuneInString(line[:end])
	if r == '.' {
		return "", automaticSelectorDot(line, end-1)
	}
	if !codeCompletionWordRune(r) {
		return "", false
	}
	start := automaticCompletionWordStart(line, end)
	if start < 0 {
		return "", false
	}
	first, _ := utf8.DecodeRuneInString(line[start:end])
	if !codeCompletionIdentStart(first) {
		return "", false
	}
	// Examples: 1.e or 0x1.p are partial floats, not a field name.
	if start > 0 && line[start-1] == '.' && !automaticSelectorDot(line, start-1) {
		return "", false
	}
	return line[start:end], true
}

func codeCompletionIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func codeCompletionWordRune(r rune) bool   { return codeCompletionIdentStart(r) || unicode.IsDigit(r) }

func automaticCompletionWordStart(line string, end int) int {
	start := end
	for start > 0 {
		r, w := utf8.DecodeLastRuneInString(line[:start])
		if !codeCompletionWordRune(r) {
			break
		}
		start -= w
		if end-start > maxAutomaticCompletionWordBytes {
			return -1
		}
	}
	return start
}

func automaticSelectorDot(line string, dot int) bool {
	if dot == 0 || line[dot-1] == '.' {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(line[:dot])
	if !codeCompletionWordRune(r) {
		// Accept selectors after whitespace, indexes, calls, and composite
		// literals as in the ordinary editing path.
		return true
	}
	start := automaticCompletionWordStart(line, dot)
	if start < 0 {
		return false
	}
	r, _ = utf8.DecodeRuneInString(line[start:dot])
	return codeCompletionIdentStart(r) // distinct from 12., 1e2., 0x1., etc.
}

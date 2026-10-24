package gui

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// EditorFindOptions controls literal, single-line Unicode search. Zero values
// preserve case-insensitive substring matching. All editor APIs are UI-thread only.
type EditorFindOptions struct{ MatchCase, WholeWord bool }

const editorFindAsyncBytes = 32 << 10
const editorFindHitLimit = 100000
const editorFindQueryLimit = 1 << 20
const editorFindReplacementLimit = 128 << 20

var ErrEditorFindQueryTooLarge = errors.New("find query exceeds 1 MiB")
var ErrEditorFindMultilineQuery = errors.New("find query must be a single line")
var ErrEditorFindReplacementTooLarge = errors.New("replacement result exceeds 128 MiB")

type editorFindRequest struct {
	text, query, replacement string
	options                  EditorFindOptions
	revision, generation     uint64
	replace                  bool
}
type editorFindResult struct {
	request editorFindRequest
	matches []findMatch
	count   int
	text    string
	err     error
}

// A rune value above MaxRune preserves the identity of malformed UTF-8 bytes;
// searching for a literal U+FFFD must not match unrelated invalid source bytes.
func findRune(s string) (rune, int) {
	r, w := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && w == 1 {
		return utf8.MaxRune + 1 + rune(s[0]), w
	}
	return r, w
}
func foldFindRune(r rune) rune {
	if r < utf8.RuneSelf {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	if r > utf8.MaxRune {
		return r
	}
	best := unicode.ToLower(r)
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		best = min(best, unicode.ToLower(f))
	}
	return best
}
func findWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
func findWholeBoundary(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if findWordRune(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if findWordRune(r) {
			return false
		}
	}
	return true
}

// scanEditorFind uses KMP, O(document+query) time, with bounded cancellation
// intervals even for a single minified line. No []rune(document), lowercased
// document copy, per-line allocation or quadratic prefix rescanning is needed.
func scanEditorFind(ctx context.Context, text, query string, options EditorFindOptions, hit func(findMatch, int, int) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(query) > editorFindQueryLimit {
		return ErrEditorFindQueryTooLarge
	}
	if strings.ContainsAny(query, "\r\n") {
		return ErrEditorFindMultilineQuery
	}
	if query == "" {
		return nil
	}
	q := make([]rune, 0, utf8.RuneCountInString(query))
	for pos := 0; pos < len(query); {
		if len(q)&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		r, w := findRune(query[pos:])
		pos += w
		if !options.MatchCase {
			r = foldFindRune(r)
		}
		q = append(q, r)
	}
	prefix := make([]int, len(q))
	starts := make([]int, len(q))
	for i, j := 1, 0; i < len(q); i++ {
		if i&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for j > 0 && q[i] != q[j] {
			j = prefix[j-1]
		}
		if q[i] == q[j] {
			j++
		}
		prefix[i] = j
	}
	line, col, index, j := 0, 0, 0, 0
	for pos := 0; pos < len(text); {
		if index&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		r, w := findRune(text[pos:])
		end := pos + w
		if r == '\n' {
			line++
			col = 0
			j = 0
			pos = end
			index++
			continue
		}
		starts[index%len(q)] = pos
		if !options.MatchCase {
			r = foldFindRune(r)
		}
		for j > 0 && r != q[j] {
			j = prefix[j-1]
		}
		if r == q[j] {
			j++
		}
		if j == len(q) {
			start := starts[(index-len(q)+1)%len(q)]
			if !options.WholeWord || findWholeBoundary(text, start, end) {
				if err := hit(findMatch{line, col - len(q) + 1, col + 1}, start, end); err != nil {
					return err
				}
				j = 0 // accepted hits never overlap and never re-scan replacement text
			} else {
				j = prefix[j-1]
			}
		}
		col++
		index++
		pos = end
	}
	return ctx.Err()
}

func computeEditorFind(ctx context.Context, r editorFindRequest) editorFindResult {
	result := editorFindResult{request: r}
	result.request.text = ""
	copied := 0
	var out strings.Builder
	write := func(s string) error {
		if len(s) > editorFindReplacementLimit-out.Len() {
			return ErrEditorFindReplacementTooLarge
		}
		out.WriteString(s)
		return nil
	}
	result.err = scanEditorFind(ctx, r.text, r.query, r.options, func(m findMatch, start, end int) error {
		result.count++
		if r.replace {
			if err := write(r.text[copied:start]); err != nil {
				return err
			}
			if err := write(r.replacement); err != nil {
				return err
			}
			copied = end
		} else if len(result.matches) < editorFindHitLimit {
			result.matches = append(result.matches, m)
		}
		return nil
	})
	if result.err == nil && r.replace && result.count > 0 {
		result.err = write(r.text[copied:])
		if result.err == nil {
			result.text = out.String()
		}
	}
	if result.err != nil {
		result.matches = nil
		result.text = ""
		result.count = 0
	}
	return result
}

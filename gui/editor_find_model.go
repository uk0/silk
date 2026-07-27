package gui

// Local find/replace model for the code editor.
//
// The editor's own scanner (findMatches in codeeditor.go) is a fixed
// case-insensitive literal walk: no regex, no case toggle, no whole-word, no
// "search in selection". FindModel is the full model behind a Qt-Creator style
// find bar, kept deliberately separate from the widget: text and options in,
// matches and rewritten text out. No Widget, no painter, no font metrics, so it
// is unit-testable without GL.
//
// Offsets are byte offsets into the searched text (half-open [Start,End)),
// which is what a splice needs; Line/Col are carried alongside for the editor's
// rune-indexed cursor.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FindOptions is one configuration of the find bar. The zero value is a
// case-insensitive literal search over the whole text, which is what the find
// bar starts out as.
//
// Regex reads Query as a regular expression (Go RE2 syntax) instead of a
// literal. CaseSensitive turns off case folding for both modes. WholeWord
// keeps only matches bounded by non-word runes (Unicode aware, see
// findWordBounded). InSelection scopes the search to the caller's selection.
// PreserveCase affects the Replace* methods only: the replacement takes on the
// case pattern of the text it overwrites.
type FindOptions struct {
	Query         string
	Regex         bool
	CaseSensitive bool
	WholeWord     bool
	InSelection   bool
	PreserveCase  bool
}

// FindRange is a half-open byte span [Start,End) of some text: the caller's
// selection on input, an applied replacement on output.
type FindRange struct {
	Start int
	End   int
}

// FindMatch is one search hit. Start/End are byte offsets into the searched
// text (half-open, so text[Start:End] is the matched span). Line is the
// 0-based line the match starts on and Col the 0-based *rune* column within
// that line, matching the editor's cursorLine/cursorCol convention.
type FindMatch struct {
	Start int
	End   int
	Line  int
	Col   int
}

// FindModel searches and rewrites text according to Options. It holds no
// document and no cursor: every method takes the text and the caret it should
// work from, so the same model can be pointed at any buffer and nothing can go
// stale behind the caller's back.
type FindModel struct {
	Options FindOptions
}

// NewFindModel returns a model configured with opt.
func NewFindModel(opt FindOptions) *FindModel {
	return &FindModel{Options: opt}
}

// Search returns every match in text, ordered by offset and non-overlapping.
// sel is the caller's selection and is only consulted when Options.InSelection
// is set, in which case only matches fully inside the selection are kept — a
// match straddling a selection edge is dropped, and an empty selection finds
// nothing.
//
// An empty Query yields no matches (never one hit per position). An invalid
// regular expression yields an error and no matches; a literal query can never
// fail to compile because it is quoted.
//
// Zero-width patterns ("a*", "^", "\b") terminate: the scan advances one rune
// past an empty match instead of retrying at the same offset, and an empty
// match abutting the previous match is dropped. Matching runs against the
// whole text rather than a re-sliced tail, so ^, $ and \b see their real
// neighbours; ^/$ are per-line (the pattern is compiled with (?m)), which is
// what a find bar user expects from "^func".
func (this *FindModel) Search(text string, sel FindRange) ([]FindMatch, error) {
	re, err := this.compile()
	if err != nil || re == nil {
		return nil, err
	}
	lo, hi := this.scope(text, sel)
	if this.Options.InSelection && lo >= hi {
		return nil, nil
	}
	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return nil, nil
	}
	starts := findLineStarts(text)
	out := make([]FindMatch, 0, len(locs))
	for _, loc := range locs {
		s, e := loc[0], loc[1]
		if s < lo || e > hi {
			continue
		}
		if this.Options.WholeWord && !findWordBounded(text, s, e) {
			continue
		}
		line := sort.SearchInts(starts, s+1) - 1
		out = append(out, FindMatch{
			Start: s,
			End:   e,
			Line:  line,
			Col:   utf8.RuneCountInString(text[starts[line]:s]),
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Next returns the first match starting strictly after caret, wrapping around
// to the first match when caret sits at or past the last one. ok is false only
// when there is nothing to find. The comparison is strict so a zero-width
// match sitting on the caret cannot pin the cursor to itself.
func (this *FindModel) Next(text string, sel FindRange, caret int) (FindMatch, bool, error) {
	ms, err := this.Search(text, sel)
	if err != nil || len(ms) == 0 {
		return FindMatch{}, false, err
	}
	for _, m := range ms {
		if m.Start > caret {
			return m, true, nil
		}
	}
	return ms[0], true, nil
}

// Prev returns the last match starting strictly before caret, wrapping around
// to the last match when caret sits at or before the first one.
func (this *FindModel) Prev(text string, sel FindRange, caret int) (FindMatch, bool, error) {
	ms, err := this.Search(text, sel)
	if err != nil || len(ms) == 0 {
		return FindMatch{}, false, err
	}
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Start < caret {
			return ms[i], true, nil
		}
	}
	return ms[len(ms)-1], true, nil
}

// ReplaceOne replaces the first match starting at or after caret — the match a
// find bar has highlighted when the user hits Replace — wrapping around to the
// first match when caret is past the last one. It returns the new text and the
// span the replacement now occupies *in that new text*, so the caller can put
// its caret at the end of what it just wrote. applied holds at most one range
// and is nil when nothing matched.
func (this *FindModel) ReplaceOne(text, repl string, sel FindRange, caret int) (string, []FindRange, error) {
	ms, err := this.Search(text, sel)
	if err != nil || len(ms) == 0 {
		return text, nil, err
	}
	hit := ms[0]
	for _, m := range ms {
		if m.Start >= caret {
			hit = m
			break
		}
	}
	out := this.replacement(text[hit.Start:hit.End], repl)
	return text[:hit.Start] + out + text[hit.End:],
		[]FindRange{{Start: hit.Start, End: hit.Start + len(out)}}, nil
}

// ReplaceAll replaces every match in one pass and returns the new text plus
// the span each replacement occupies in it. The returned ranges are offsets
// into the *new* text, so they stay correct when the replacement is longer or
// shorter than what it replaced. Matches are non-overlapping and ascending, so
// the rewrite is a single left-to-right splice: nothing written can be matched
// again and no offset drifts.
func (this *FindModel) ReplaceAll(text, repl string, sel FindRange) (string, []FindRange, error) {
	ms, err := this.Search(text, sel)
	if err != nil || len(ms) == 0 {
		return text, nil, err
	}
	var b strings.Builder
	b.Grow(len(text))
	applied := make([]FindRange, 0, len(ms))
	prev := 0
	for _, m := range ms {
		b.WriteString(text[prev:m.Start])
		start := b.Len()
		b.WriteString(this.replacement(text[m.Start:m.End], repl))
		applied = append(applied, FindRange{Start: start, End: b.Len()})
		prev = m.End
	}
	b.WriteString(text[prev:])
	return b.String(), applied, nil
}

// compile turns Options into a matcher. A literal query is quoted so regex
// metacharacters in it are just characters; (?m) makes ^/$ per-line and (?i)
// folds case. It returns (nil, nil) for an empty query: nothing to search for
// is not an error.
func (this *FindModel) compile() (*regexp.Regexp, error) {
	if this.Options.Query == "" {
		return nil, nil
	}
	pat := this.Options.Query
	if !this.Options.Regex {
		pat = regexp.QuoteMeta(pat)
	}
	flags := "(?m)"
	if !this.Options.CaseSensitive {
		flags = "(?mi)"
	}
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		return nil, fmt.Errorf("find: invalid regular expression %q: %w", this.Options.Query, err)
	}
	return re, nil
}

// scope returns the byte range the search is restricted to: the whole text
// unless InSelection is set, in which case sel normalized (reversed selections
// swapped) and clamped into the text.
func (this *FindModel) scope(text string, sel FindRange) (int, int) {
	if !this.Options.InSelection {
		return 0, len(text)
	}
	lo, hi := sel.Start, sel.End
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(text) {
		hi = len(text)
	}
	if lo > hi {
		lo = hi
	}
	return lo, hi
}

// replacement is repl, optionally recased to match the text it overwrites.
func (this *FindModel) replacement(matched, repl string) string {
	if !this.Options.PreserveCase {
		return repl
	}
	return findApplyCase(findCaseOf(matched), repl)
}

// findLineStarts returns the byte offset of every line start in text. Index i
// is the start of line i, and the slice is ascending, so a binary search over
// it maps a match offset back to its line.
func findLineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// findWordBounded reports whether text[start:end) is a whole word. Boundaries
// are Unicode word runes (letters, digits, underscore — see isWordRune), not
// RE2's ASCII-only \b: "caf" is *not* a whole word inside "café" even though
// \b would say it is. A boundary is only enforced where it can be crossed, so
// a match that already begins or ends on punctuation ("-", "(") does not need
// punctuation outside it. Zero-width matches are always bounded.
func findWordBounded(text string, start, end int) bool {
	if start >= end {
		return true
	}
	if start > 0 {
		prev, _ := utf8.DecodeLastRuneInString(text[:start])
		first, _ := utf8.DecodeRuneInString(text[start:end])
		if isWordRune(prev) && isWordRune(first) {
			return false
		}
	}
	if end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		last, _ := utf8.DecodeLastRuneInString(text[start:end])
		if isWordRune(next) && isWordRune(last) {
			return false
		}
	}
	return true
}

// findCase is the case pattern of a matched span, used by PreserveCase.
type findCase int

const (
	findCaseOther findCase = iota // mixed case, or no cased runes at all
	findCaseLower                 // "foo"
	findCaseTitle                 // "Foo", and a lone "F"
	findCaseUpper                 // "FOO" (two or more upper, none lower)
)

// findCaseOf classifies s. Uncased runes (digits, "_", punctuation) are
// ignored, so "FOO_BAR" is upper and "Foo_bar" is title. A single uppercase
// letter is title rather than upper, because replacing "I" with "you" should
// give "You" and not "YOU". Anything else — camelCase, "fOo", a span with no
// letters — is findCaseOther and leaves the replacement alone.
func findCaseOf(s string) findCase {
	upper, lower := 0, 0
	firstUpper, seen := false, false
	for _, r := range s {
		isUp, isLo := unicode.IsUpper(r), unicode.IsLower(r)
		if !isUp && !isLo {
			continue
		}
		if isUp {
			upper++
		} else {
			lower++
		}
		if !seen {
			seen, firstUpper = true, isUp
		}
	}
	switch {
	case !seen:
		return findCaseOther
	case lower == 0 && upper >= 2:
		return findCaseUpper
	case firstUpper && upper == 1:
		return findCaseTitle
	case !firstUpper && upper == 0:
		return findCaseLower
	}
	return findCaseOther
}

// findApplyCase recases repl into c. Title only touches the first rune, so a
// replacement typed as "fooBar" becomes "FooBar" and keeps its own humps.
func findApplyCase(c findCase, repl string) string {
	switch c {
	case findCaseUpper:
		return strings.ToUpper(repl)
	case findCaseLower:
		return strings.ToLower(repl)
	case findCaseTitle:
		r, n := utf8.DecodeRuneInString(repl)
		if n == 0 {
			return repl
		}
		return string(unicode.ToUpper(r)) + repl[n:]
	}
	return repl
}

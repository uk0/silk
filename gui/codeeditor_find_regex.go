package gui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"regexp/syntax"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const editorRegexQueryLimit = 64 << 10
const editorRegexProgramLimit = 32768
const editorRegexCaptureLimit = 128
const editorRegexTimeLimit = 5 * time.Second

var ErrEditorRegexComplexity = errors.New("regular expression exceeds the 64 KiB / 32768 instruction / 128 capture limit")
var ErrEditorFindStaleMatch = errors.New("search match is no longer current")

type editorFindSpan struct{ start, end, endLine, endCol int }
type editorRegex struct {
	first, next  *regexp.Regexp
	names        []string
	wrappedFirst bool
}

// Reader-based execution keeps Go's matching semantics and makes even a long
// no-match scan cancellable. EOF caused by cancellation NEVER becomes a result:
// every engine call checks ctx.Err before accepting any provisional match.
type editorRegexReader struct {
	ctx        context.Context
	text       string
	pos, reads int
}

func (r *editorRegexReader) ReadRune() (rune, int, error) {
	if r.reads&255 == 0 {
		if err := r.ctx.Err(); err != nil {
			return 0, 0, io.EOF
		}
	}
	r.reads++
	if r.pos >= len(r.text) {
		return 0, 0, io.EOF
	}
	ch, n := utf8.DecodeRuneInString(r.text[r.pos:])
	r.pos += n
	return ch, n, nil
}

// Bound repeated-expression expansion BEFORE Simplify/Compile allocate it.
func editorRegexCost(r *syntax.Regexp) int {
	cap := editorRegexProgramLimit + 1
	add := func(a, b int) int {
		if a >= cap-b {
			return cap
		}
		return a + b
	}
	sum := 0
	for _, s := range r.Sub {
		sum = add(sum, editorRegexCost(s))
	}
	switch r.Op {
	case syntax.OpLiteral:
		return min(cap, len(r.Rune)+1)
	case syntax.OpRepeat:
		n := r.Max
		if n < 0 {
			n = r.Min + 1
		}
		unit := add(sum, 2)
		if n > 0 && unit > cap/n {
			return cap
		}
		return min(cap, unit*n+2)
	default:
		return add(sum, len(r.Sub)+2)
	}
}
func suppressBeginText(r *syntax.Regexp) {
	if r.Op == syntax.OpBeginText {
		r.Op = syntax.OpNoMatch
		r.Sub = nil
		r.Rune = nil
		return
	}
	for _, s := range r.Sub {
		suppressBeginText(s)
	}
}
func compileEditorRegex(ctx context.Context, query string, options EditorFindOptions) (*editorRegex, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(query) > editorRegexQueryLimit {
		return nil, ErrEditorRegexComplexity
	}
	flags := syntax.Perl
	if !options.MatchCase {
		flags |= syntax.FoldCase
	}
	ast, err := syntax.Parse(query, flags)
	if err != nil {
		return nil, err
	}
	if ast.MaxCap() > editorRegexCaptureLimit || editorRegexCost(ast) > editorRegexProgramLimit {
		return nil, ErrEditorRegexComplexity
	}
	result := &editorRegex{names: ast.CapNames(), wrappedFirst: options.WholeWord}
	pattern := ast.String()
	// Consumed boundary context stays OUTSIDE capture 1. Advancing by that
	// capture's end, not the wrapper's end, lets adjacent whole words share a
	// separator without losing it. The regex engine can still try alternatives.
	const boundary = `[^\p{L}\p{Nd}\p{M}_]`
	first := pattern
	if options.WholeWord {
		first = `(?:\A|` + boundary + `)(` + pattern + `)(?:\z|` + boundary + `)`
	}
	result.first, err = regexp.Compile(first)
	if err != nil {
		return nil, err
	}
	// Go 1.26 has no public start-offset iterator. A one-rune prefix carries the
	// REAL left context into subsequent searches (\b, multiline ^). \A and
	// non-multiline ^ must become impossible away from byte zero. Never slice the
	// subject and pretend that the new slice is the start of the document.
	suppressBeginText(ast)
	prefix := `(?s:.)`
	suffix := ""
	if options.WholeWord {
		prefix = boundary
		suffix = `(?:\z|` + boundary + `)`
	}
	result.next, err = regexp.Compile(prefix + `(` + ast.String() + `)` + suffix)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
func (r *editorRegex) at(ctx context.Context, text string, pos int) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pos < 0 || pos > len(text) {
		return nil, nil
	}
	base := 0
	re := r.first
	wrapped := r.wrappedFirst
	if pos > 0 {
		_, width := utf8.DecodeLastRuneInString(text[:pos])
		base = pos - width
		re = r.next
		wrapped = true
	}
	m := re.FindReaderSubmatchIndex(&editorRegexReader{ctx: ctx, text: text[base:]})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	if wrapped {
		m = m[2:]
	}
	for i := range m {
		if m[i] >= 0 {
			m[i] += base
		}
	}
	return m, nil
}
func (r *editorRegex) scan(ctx context.Context, text string, hit func([]int) error) error {
	previousEnd := -1
	for pos := 0; pos <= len(text); {
		m, err := r.at(ctx, text, pos)
		if err != nil {
			return err
		}
		if m == nil {
			break
		}
		accept := true
		if m[1] == pos {
			accept = m[0] != previousEnd
			if pos == len(text) {
				pos++
			} else {
				_, width := utf8.DecodeRuneInString(text[pos:])
				pos += width
			}
		} else {
			pos = m[1]
		}
		previousEnd = m[1]
		if accept {
			if err := hit(m); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

type editorReplacementPart struct {
	literal      string
	group        int
	alternatives []int
}

// Parse once per request. Expansion streams into a bounded writer; using
// ExpandString first and checking its length afterwards could allocate GiB.
func editorReplacementParts(template string, names []string) []editorReplacementPart {
	var parts []editorReplacementPart
	literal := func(s string) {
		if s != "" {
			parts = append(parts, editorReplacementPart{literal: s, group: -1})
		}
	}
	for pos := 0; pos < len(template); {
		dollar := strings.IndexByte(template[pos:], '$')
		if dollar < 0 {
			literal(template[pos:])
			break
		}
		dollar += pos
		literal(template[pos:dollar])
		pos = dollar + 1
		if pos < len(template) && template[pos] == '$' {
			literal("$")
			pos++
			continue
		}
		start := pos
		braced := start < len(template) && template[start] == '{'
		if braced {
			start++
		}
		end := start
		for end < len(template) {
			r, w := utf8.DecodeRuneInString(template[end:])
			if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			end += w
		}
		if end == start || (braced && (end == len(template) || template[end] != '}')) {
			literal("$")
			continue
		}
		name := template[start:end]
		pos = end
		if braced {
			pos++
		}
		group := 0
		for i := 0; i < len(name); i++ {
			if name[i] < '0' || name[i] > '9' || group >= 1e8 {
				group = -1
				break
			}
			group = group*10 + int(name[i]-'0')
		}
		if len(name) > 1 && name[0] == '0' {
			group = -1
		}
		if group >= 0 {
			if group < len(names) {
				parts = append(parts, editorReplacementPart{group: group})
			}
		} else {
			var groups []int
			for i, s := range names {
				if s == name {
					groups = append(groups, i)
				}
			}
			if len(groups) > 0 {
				parts = append(parts, editorReplacementPart{group: groups[0], alternatives: groups[1:]})
			}
		}
	}
	return parts
}
func expandEditorReplacement(ctx context.Context, parts []editorReplacementPart, text string, m []int, write func(string) error) error {
	for i, p := range parts {
		if i&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		s := p.literal
		if p.group >= 0 {
			n := 2 * p.group
			for _, g := range p.alternatives {
				if n+1 < len(m) && m[n] >= 0 {
					break
				}
				n = 2 * g
			}
			if n+1 >= len(m) || m[n] < 0 {
				continue
			}
			s = text[m[n]:m[n+1]]
		}
		if err := write(s); err != nil {
			return err
		}
	}
	return nil
}

type editorRegexPosition struct{ offset, line, col, reads int }

func (p *editorRegexPosition) advance(ctx context.Context, text string, target int) error {
	for p.offset < target {
		if p.reads&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		r, w := utf8.DecodeRuneInString(text[p.offset:])
		p.offset += w
		p.reads++
		if r == '\n' {
			p.line++
			p.col = 0
		} else {
			p.col++
		}
	}
	return ctx.Err()
}
func computeEditorRegex(parent context.Context, r editorFindRequest) (result editorFindResult) {
	result.request = r
	result.request.text = ""
	ctx, cancel := context.WithTimeout(parent, editorRegexTimeLimit)
	defer cancel()
	defer func() {
		if result.err != nil {
			result.matches = nil
			result.spans = nil
			result.text = ""
			result.count = 0
		}
	}()
	if r.query == "" {
		return result
	}
	re, err := compileEditorRegex(ctx, r.query, r.options)
	if err != nil {
		result.err = err
		return result
	}
	var out strings.Builder
	limit := editorReplacementBudget(r)
	if r.one {
		limit -= len(r.text) - (r.target.end - r.target.start)
	}
	write := func(s string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(s) > limit-out.Len() {
			return ErrEditorFindReplacementTooLarge
		}
		out.WriteString(s)
		return nil
	}
	var parts []editorReplacementPart
	if r.replace {
		if len(r.replacement) > editorFindQueryLimit {
			result.err = errors.New("regex replacement template exceeds 1 MiB")
			return result
		}
		parts = editorReplacementParts(r.replacement, re.names)
	}
	if r.one {
		m, err := re.at(ctx, r.text, r.target.start)
		if err != nil {
			result.err = err
			return result
		}
		if len(m) < 2 || m[0] != r.target.start || m[1] != r.target.end {
			result.err = ErrEditorFindStaleMatch
			return result
		}
		result.err = expandEditorReplacement(ctx, parts, r.text, m, write)
		if result.err == nil {
			result.text = out.String()
			result.count = 1
		}
		return result
	}
	copied := 0
	position := editorRegexPosition{}
	result.err = re.scan(ctx, r.text, func(m []int) error {
		result.count++
		if r.replace {
			if err := write(r.text[copied:m[0]]); err != nil {
				return err
			}
			if err := expandEditorReplacement(ctx, parts, r.text, m, write); err != nil {
				return err
			}
			copied = m[1]
		} else if len(result.matches) < editorFindHitLimit {
			if err := position.advance(ctx, r.text, m[0]); err != nil {
				return err
			}
			line, col := position.line, position.col
			if err := position.advance(ctx, r.text, m[1]); err != nil {
				return err
			}
			result.matches = append(result.matches, findMatch{line, col, position.col})
			result.spans = append(result.spans, editorFindSpan{m[0], m[1], position.line, position.col})
		}
		return nil
	})
	if result.err == nil && r.replace && result.count > 0 {
		result.err = write(r.text[copied:])
		if result.err == nil {
			result.text = out.String()
		}
	}
	if errors.Is(result.err, context.DeadlineExceeded) {
		result.err = fmt.Errorf("regular expression search exceeded %s: %w", editorRegexTimeLimit, result.err)
	}
	return result
}

package gui

import (
	"context"
	"errors"
	"fmt"
	"github.com/uk0/silk/internal/regexscan"
	"strings"
	"time"
	"unicode/utf8"
)

const editorRegexQueryLimit = 64 << 10
const editorRegexProgramLimit = 32768
const editorRegexCaptureLimit = 128
const editorRegexTimeLimit = 5 * time.Second

var ErrEditorRegexComplexity = regexscan.ErrComplexity
var ErrEditorFindStaleMatch = errors.New("search match is no longer current")

type editorFindSpan struct{ start, end, endLine, endCol int }
type editorRegex struct {
	pattern *regexscan.Pattern
	names   []string
}

func compileEditorRegex(ctx context.Context, query string, options EditorFindOptions) (*editorRegex, error) {
	p, err := regexscan.Compile(ctx, query, regexscan.Options{MatchCase: options.MatchCase, WholeWord: options.WholeWord})
	if err != nil {
		return nil, err
	}
	return &editorRegex{pattern: p, names: p.Names()}, nil
}
func (r *editorRegex) at(ctx context.Context, text string, pos int) ([]int, error) {
	return r.pattern.At(ctx, text, pos)
}
func (r *editorRegex) scan(ctx context.Context, text string, hit func([]int) error) error {
	return r.pattern.Scan(ctx, text, hit)
}

type editorReplacementPart = regexscan.ReplacementPart

func editorReplacementParts(template string, names []string) []editorReplacementPart {
	return regexscan.ReplacementParts(template, names)
}
func expandEditorReplacement(ctx context.Context, parts []editorReplacementPart, text string, m []int, write func(string) error) error {
	return regexscan.ExpandReplacement(ctx, parts, text, m, write)
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

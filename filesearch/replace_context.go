package filesearch

import (
	"context"
	"errors"
	"github.com/uk0/silk/internal/regexscan"
	"regexp"
	"strings"
)

var ErrReplacementLimit = errors.New("workspace replacement exceeds output or line limit")

// ReplaceTextContext uses SearchContext's line/positive-width semantics. It
// preserves original LF/CRLF/no-final-newline bytes, bounds output before writing,
// and returns no partial proposal on cancellation/error. No filesystem writes.
func ReplaceTextContext(ctx context.Context, text, pattern, replacement string, opt ContextOptions, maxBytes int) (string, int, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if pattern == "" {
		return text, 0, nil
	}
	if len(pattern) > regexscan.QueryLimit {
		return "", 0, regexscan.ErrComplexity
	}
	if len(replacement) > 1<<20 {
		return "", 0, ErrReplacementLimit
	}
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	expression := pattern
	if !opt.Regex {
		expression = regexp.QuoteMeta(pattern)
	}
	re, err := regexscan.Compile(ctx, expression, regexscan.Options{MatchCase: !opt.IgnoreCase, WholeWord: opt.WholeWord})
	if err != nil {
		return "", 0, err
	}
	parts := regexscan.ReplacementParts(replacement, re.Names())
	var out strings.Builder
	count := 0
	write := func(s string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(s) > maxBytes-out.Len() {
			return ErrReplacementLimit
		}
		out.WriteString(s)
		return nil
	}
	for rest := text; rest != ""; {
		line, term := rest, ""
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, term, rest = rest[:i], "\n", rest[i+1:]
			if strings.HasSuffix(line, "\r") {
				line, term = line[:len(line)-1], "\r\n"
			}
		} else {
			rest = ""
		}
		if len(line) > opt.Limits.normalized().LineBytes {
			return "", 0, ErrReplacementLimit
		}
		copied := 0
		err = re.Scan(ctx, line, func(m []int) error {
			if m[0] == m[1] {
				return nil
			}
			if err := write(line[copied:m[0]]); err != nil {
				return err
			}
			var err error
			if opt.Regex {
				err = regexscan.ExpandReplacement(ctx, parts, line, m, write)
			} else {
				err = write(replacement)
			}
			if err != nil {
				return err
			}
			copied = m[1]
			count++
			return nil
		})
		if err == nil {
			err = write(line[copied:])
		}
		if err == nil {
			err = write(term)
		}
		if err != nil {
			return "", 0, err
		}
	}
	return out.String(), count, nil
}

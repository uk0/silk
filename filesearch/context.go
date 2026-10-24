package filesearch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/uk0/silk/internal/regexscan"
)

var ErrSearchLimit = errors.New("workspace search resource limit reached")

// SearchLimits bounds computation and retained results. Nonpositive fields use
// defaults: 10000 matches/files, 100000 entries, depth 64, 8 MiB/file, 256 KiB/line,
// 128 MiB read and 16 MiB retained. Cancellation cannot interrupt an OS read that
// is itself blocked; callers must also bound concurrent workers.
type SearchLimits struct{ Matches, Files, Entries, Depth, FileBytes, LineBytes, TotalBytes, ResultBytes int }

func (l SearchLimits) normalized() SearchLimits {
	v := []*int{&l.Matches, &l.Files, &l.Entries, &l.Depth, &l.FileBytes, &l.LineBytes, &l.TotalBytes, &l.ResultBytes}
	defaults := []int{10000, 10000, 100000, 64, 8 << 20, 256 << 10, 128 << 20, 16 << 20}
	for i, p := range v {
		if *p <= 0 {
			*p = defaults[i]
		}
	}
	return l
}

// ContextOptions extends the legacy scanner without changing Options literals.
// Excludes match a basename or any relative path segment and prune directories.
type ContextOptions struct {
	Options
	WholeWord bool
	Exclude   []string
	Limits    SearchLimits
}

// Occurrence retains exact byte length, avoiding re-matching a sliced suffix
// whose ^ or word-boundary context differs from the original line.
type Occurrence struct {
	Match
	Length int
}
type SearchResult struct {
	Matches                                         []Occurrence
	Files, Entries, BytesRead                       int
	SkippedLargeFiles, SkippedLongLines, ReadErrors int
	Truncated                                       bool
}

func (r SearchResult) Complete() bool {
	return !r.Truncated && r.SkippedLargeFiles == 0 && r.SkippedLongLines == 0 && r.ReadErrors == 0
}

func excludedRelative(path string, patterns []string) bool {
	for _, pat := range patterns {
		for _, part := range strings.Split(filepath.ToSlash(path), "/") {
			if ok, _ := filepath.Match(pat, part); ok {
				return true
			}
		}
	}
	return false
}
func validateExcludes(patterns []string) error {
	for _, p := range patterns {
		if _, err := filepath.Match(p, ""); err != nil {
			return fmt.Errorf("exclude %q: %w", p, err)
		}
	}
	return nil
}

// SearchContext scans disk text, not unsaved editor buffers. Positive-width hits
// only, one line at a time; errors and partial coverage are explicit. Directory
// batches, line assembly and retained matches are bounded. A cancellation returns
// no publishable matches. Results follow directory stream order (not sorted).
func SearchContext(ctx context.Context, root, pattern string, opt ContextOptions) (result SearchResult, err error) {
	defer func() {
		if ctx.Err() != nil {
			result.Matches = nil
			err = ctx.Err()
		}
	}()
	if err = ctx.Err(); err != nil {
		return
	}
	if err = validateExcludes(opt.Exclude); err != nil {
		return
	}
	if pattern == "" {
		return
	}
	if len(pattern) > regexscan.QueryLimit {
		err = regexscan.ErrComplexity
		return
	}
	expression := pattern
	if !opt.Regex {
		expression = regexp.QuoteMeta(pattern)
	}
	re, compileErr := regexscan.Compile(ctx, expression, regexscan.Options{MatchCase: !opt.IgnoreCase, WholeWord: opt.WholeWord})
	if compileErr != nil {
		err = compileErr
		return
	}
	limit := opt.Limits.normalized()
	retained := 0
	limitError := func() error { result.Truncated = true; return ErrSearchLimit }
	scan := func(path string, info fs.FileInfo) error {
		if !includeFile(path, opt.Include) {
			return nil
		}
		if result.Files >= limit.Files {
			return limitError()
		}
		result.Files++
		if info.Size() > int64(limit.FileBytes) {
			result.SkippedLargeFiles++
			return nil
		}
		f, e := os.Open(path)
		if e != nil {
			result.ReadErrors++
			return nil
		}
		defer f.Close()
		// Reject file swaps to a nonregular entry; no device/FIFO scanning.
		actual, e := f.Stat()
		if e != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
			result.ReadErrors++
			return nil
		}
		br := bufio.NewReaderSize(searchReadFunc(func(b []byte) (int, error) {
			if e := ctx.Err(); e != nil {
				return 0, e
			}
			room := limit.TotalBytes - result.BytesRead
			if room < 0 {
				return 0, ErrSearchLimit
			}
			b = b[:min(len(b), room+1)]
			n, e := f.Read(b)
			result.BytesRead += n
			if result.BytesRead > limit.TotalBytes {
				return n, ErrSearchLimit
			}
			return n, e
		}), readerSize)
		if opt.SkipBinary {
			head, peekErr := br.Peek(peekSize)
			if errors.Is(peekErr, ErrSearchLimit) || result.BytesRead > limit.TotalBytes {
				return limitError()
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if containsNUL(head) {
				return nil
			}
		}
		var line []byte
		long := false
		lineNo, read := 0, 0
		for {
			if e = ctx.Err(); e != nil {
				return e
			}
			chunk, readErr := br.ReadSlice('\n')
			read += len(chunk)
			if errors.Is(readErr, ErrSearchLimit) {
				return limitError()
			}
			if result.BytesRead > limit.TotalBytes {
				return limitError()
			}
			if read > limit.FileBytes {
				result.SkippedLargeFiles++
				return nil
			}
			if len(line)+len(chunk) > limit.LineBytes {
				long = true
				line = nil
			}
			if !long {
				line = append(line, chunk...)
			}
			if readErr == bufio.ErrBufferFull {
				continue
			}
			if len(chunk) > 0 || len(line) > 0 || long {
				lineNo++
				if long {
					result.SkippedLongLines++
				} else {
					text := trimEOL(string(line))
					keptLine := false
					e = re.Scan(ctx, text, func(m []int) error {
						if m[0] == m[1] {
							return nil
						}
						cost := 64 + len(path)
						if !keptLine {
							cost += len(text)
						}
						if len(result.Matches) >= limit.Matches || cost > limit.ResultBytes-retained {
							return limitError()
						}
						result.Matches = append(result.Matches, Occurrence{Match{Path: path, Line: lineNo, Col: m[0] + 1, Text: text}, m[1] - m[0]})
						retained += cost
						keptLine = true
						return nil
					})
					if e != nil {
						return e
					}
				}
			}
			line = line[:0]
			long = false
			if readErr != nil {
				if readErr != io.EOF {
					result.ReadErrors++
				}
				return nil
			}
		}
	}
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if depth > limit.Depth {
			return limitError()
		}
		info, e := os.Lstat(path)
		if e != nil {
			if depth == 0 {
				return e
			}
			result.ReadErrors++
			return nil
		}
		if info.Mode().IsRegular() {
			return scan(path, info)
		}
		if !info.IsDir() {
			if depth == 0 {
				return fmt.Errorf("search root is not a regular file or directory: %s", path)
			}
			return nil
		}
		d, e := os.Open(path)
		if e != nil {
			if depth == 0 {
				return e
			}
			result.ReadErrors++
			return nil
		}
		defer d.Close()
		actual, e := d.Stat()
		if e != nil || !actual.IsDir() || !os.SameFile(info, actual) {
			return fmt.Errorf("directory changed during search: %s", path)
		}
		for {
			if e := ctx.Err(); e != nil {
				return e
			}
			batch, e := d.ReadDir(128)
			for _, entry := range batch {
				result.Entries++
				if result.Entries > limit.Entries {
					return limitError()
				}
				if entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				if excludedRelative(entry.Name(), opt.Exclude) || (entry.IsDir() && skipDir(entry.Name())) {
					continue
				}
				if err := walk(filepath.Join(path, entry.Name()), depth+1); err != nil {
					return err
				}
			}
			if e != nil {
				if e != io.EOF {
					result.ReadErrors++
				}
				return nil
			}
		}
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return
	}
	err = walk(root, 0)
	return
}

type searchReadFunc func([]byte) (int, error)

func (f searchReadFunc) Read(b []byte) (int, error) { return f(b) }

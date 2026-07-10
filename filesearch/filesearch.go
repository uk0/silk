// Package filesearch is Silk's find-in-files engine — the UI-agnostic
// core behind an IDE's "Find in Files" (Qt Creator's Search > Find in
// Files). It walks a directory tree, scans each eligible text file line
// by line, and reports every match; a companion Replace computes the
// rewritten line for a match without touching disk (a caller applies
// the edit).
//
// Design notes:
//
//   - Column is a 1-based BYTE offset. Match.Col is the byte position of
//     the match start within the line (first byte = 1), not a rune or
//     display column. For multibyte/UTF-8 text a caller that needs a
//     character column can derive it from Match.Text[:Match.Col-1]. Byte
//     offsets are what the scanner and regexp engine report natively, so
//     they are exact and cheap.
//
//   - Every occurrence is reported. A line containing the pattern N times
//     yields N Matches that share Path/Line/Text and differ only in Col
//     (left-to-right, non-overlapping), mirroring how Qt Creator lists
//     each hit as its own result. Zero-width regex matches (e.g. "a*"
//     against a line with no "a") are ignored.
//
//   - Skipped by the walk: .git, vendor and node_modules directories,
//     and any hidden directory (name starting with "."). Hidden *files*
//     are still scanned. With Options.SkipBinary set, a file whose first
//     chunk contains a NUL byte is treated as binary and skipped.
//
//   - The only error Search returns (besides an unreadable root) is a
//     regexp compile error for a malformed pattern. Individual files that
//     cannot be opened or read are skipped, not fatal. An empty pattern
//     matches nothing.
package filesearch

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options configures a Search (and the matcher Replace reconstructs).
type Options struct {
	// Regex interprets pattern as an RE2 regular expression. When false,
	// pattern is matched literally.
	Regex bool
	// IgnoreCase folds case for both literal and regex matching.
	IgnoreCase bool
	// Include restricts the scan to files whose extension is one of these,
	// e.g. []string{".go", ".txt"}. A leading dot is optional and the
	// comparison is case-insensitive. Empty means every text file.
	Include []string
	// SkipBinary skips a file whose first chunk contains a NUL byte.
	SkipBinary bool
}

// Match is a single occurrence of the pattern.
type Match struct {
	Path string // file path as walked from root
	Line int    // 1-based line number
	Col  int    // 1-based BYTE offset of the match start within the line
	Text string // the full line, with any trailing CR/LF removed
}

const (
	// readerSize is the per-file buffered-reader size. Large enough that
	// peekSize never overflows it and big files stream efficiently.
	readerSize = 64 * 1024
	// peekSize is how much of a file's head is inspected for a NUL byte
	// when SkipBinary is set ("first chunk"). Matches git's blob probe.
	peekSize = 8000
)

// Search walks root and returns every match of pattern under the rules
// in opt. Matches are ordered by walk order, then Line, then Col. A
// malformed regex (when opt.Regex) returns a compile error and no
// matches; an empty pattern matches nothing.
func Search(root, pattern string, opt Options) ([]Match, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := buildMatcher(pattern, opt)
	if err != nil {
		return nil, err
	}

	var matches []Match
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err // root itself is unreadable — report it
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil // skip an unreadable entry, keep going
		}
		if d.IsDir() {
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // don't follow symlinks / scan devices
		}
		if !includeFile(p, opt.Include) {
			return nil
		}
		matches = append(matches, scanFile(p, re, pattern, opt)...)
		return nil
	})
	if walkErr != nil {
		return matches, walkErr
	}
	return matches, nil
}

// Replace computes m's line with pattern replaced by replacement, using
// the same Regex/IgnoreCase semantics as the Search that produced m. It
// is pure: it reads only m.Text and never touches disk.
//
// Regex mode expands replacement templates ($1, ${name}); literal mode
// inserts replacement verbatim (a "$" stays a "$"). Every occurrence on
// the line is replaced, so a caller should apply Replace once per line,
// not once per Match.
func Replace(m Match, pattern, replacement string, opt Options) (newLine string) {
	if pattern == "" {
		return m.Text
	}
	if opt.Regex {
		expr := pattern
		if opt.IgnoreCase {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return m.Text
		}
		return re.ReplaceAllString(m.Text, replacement)
	}
	if opt.IgnoreCase {
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
		if err != nil {
			return m.Text
		}
		return re.ReplaceAllLiteralString(m.Text, replacement)
	}
	return strings.ReplaceAll(m.Text, pattern, replacement)
}

// buildMatcher returns the regexp to scan lines with, or (nil, nil) for
// the literal case-sensitive fast path (which uses strings.Index).
func buildMatcher(pattern string, opt Options) (*regexp.Regexp, error) {
	switch {
	case opt.Regex:
		expr := pattern
		if opt.IgnoreCase {
			expr = "(?i)" + expr
		}
		return regexp.Compile(expr)
	case opt.IgnoreCase:
		return regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
	default:
		return nil, nil
	}
}

// skipDir reports whether a directory of this name should be pruned.
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules":
		return true
	}
	return len(name) > 1 && name[0] == '.' // hidden dir: .idea, .svn, ...
}

// includeFile reports whether path's extension passes the Include filter.
func includeFile(path string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, inc := range include {
		want := strings.ToLower(inc)
		if want == "" {
			continue
		}
		if want[0] != '.' {
			want = "." + want
		}
		if ext == want {
			return true
		}
	}
	return false
}

// scanFile scans one file and returns its matches. Unopenable files and
// (when opt.SkipBinary) binary files yield no matches.
func scanFile(path string, re *regexp.Regexp, pattern string, opt Options) []Match {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, readerSize)
	if opt.SkipBinary {
		head, _ := br.Peek(peekSize) // a partial head + EOF for small files is fine
		if containsNUL(head) {
			return nil
		}
	}

	var out []Match
	line := 0
	for {
		s, err := br.ReadString('\n')
		if len(s) > 0 {
			line++
			appendLineMatches(&out, path, line, trimEOL(s), re, pattern)
		}
		if err != nil {
			break // EOF or read error: stop this file
		}
	}
	return out
}

// appendLineMatches appends every non-overlapping match on one line.
func appendLineMatches(out *[]Match, path string, line int, text string, re *regexp.Regexp, pattern string) {
	if re != nil {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if loc[0] == loc[1] {
				continue // ignore zero-width matches
			}
			*out = append(*out, Match{Path: path, Line: line, Col: loc[0] + 1, Text: text})
		}
		return
	}
	for from := 0; from <= len(text); {
		i := strings.Index(text[from:], pattern)
		if i < 0 {
			break
		}
		col := from + i
		*out = append(*out, Match{Path: path, Line: line, Col: col + 1, Text: text})
		from = col + len(pattern) // advance past this match (non-overlapping)
	}
}

func containsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

// trimEOL removes a trailing "\n" and an optional preceding "\r".
func trimEOL(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

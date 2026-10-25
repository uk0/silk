package locator

import (
	"errors"
	"strconv"
	"strings"
)

// MaxFileQueryLength bounds interactive parsing work, including pasted input.
const MaxFileQueryLength = 4096

// ErrFileQuerySyntax means a location suffix or quoted path is incomplete/invalid.
var ErrFileQuerySyntax = errors.New("use file or file:line[:column]; line and column must be positive integers")

// FileQuery separates a file search from an optional one-based line[:column].
// Path is still a search string, not a filesystem authorization or resolved path.
// Location must be validated against the destination's live buffer before use.
type FileQuery struct {
	Path     string
	Location string
}

// ParseFileQuery accepts file:line[:column], including Windows drive/UNC paths.
// Quoting the path makes colons literal: "report:2026" or "report:2026":3:2.
// Unquoted trailing numeric fields are locations, not part of the filename.
// It does no I/O, normalization, environment expansion or buffer range checking.
// Empty input is a valid empty search. Errors return no actionable destination.
func ParseFileQuery(text string) (FileQuery, error) {
	bad := func() (FileQuery, error) { return FileQuery{}, ErrFileQuerySyntax }
	if len(text) > MaxFileQueryLength {
		return bad()
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return FileQuery{}, nil
	}
	q := FileQuery{Path: text}
	if text[0] == '"' {
		end := strings.IndexByte(text[1:], '"')
		if end < 0 {
			return bad()
		}
		end++
		q.Path = text[1:end]
		rest := strings.TrimSpace(text[end+1:])
		if rest != "" {
			if rest[0] != ':' {
				return bad()
			}
			q.Location = rest[1:]
			if q.Location == "" {
				return bad()
			}
		}
	} else {
		cut := strings.LastIndexByte(text, ':')
		if cut >= 0 && !driveColon(text, cut) {
			tail := text[cut+1:]
			// Alphabetic colon-bearing filenames remain ordinary searches. A numeric,
			// signed or empty trailing field expresses location intent and is checked.
			if !strings.ContainsAny(tail, `/\`) && (tail == "" || tail[0] >= '0' && tail[0] <= '9' || tail[0] == '+' || tail[0] == '-') {
				start := cut
				before := text[:cut]
				previous := strings.LastIndexByte(before, ':')
				if previous >= 0 && !driveColon(before, previous) && decimalField(before[previous+1:]) {
					start = previous
				}
				q.Path = strings.TrimSpace(text[:start])
				q.Location = text[start+1:]
				if q.Location == "" {
					return bad()
				}
			}
		}
	}
	if q.Path == "" {
		return bad()
	}
	if q.Location != "" {
		if len(q.Location) > 64 {
			return bad()
		}
		line, column, hasColumn := strings.Cut(q.Location, ":")
		if !positiveField(line) || hasColumn && !positiveField(column) {
			return bad()
		}
	}
	return q, nil
}
func driveColon(s string, i int) bool {
	return i == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}
func decimalField(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
func positiveField(s string) bool {
	if !decimalField(s) {
		return false
	}
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

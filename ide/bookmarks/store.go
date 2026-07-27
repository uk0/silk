// Package bookmarks is the persistent store behind an IDE's code
// bookmarks: (file, line) marks with a note, written to one JSON file and
// re-anchored to the text of their line so a mark survives edits made
// above it — including edits made while the IDE was closed.
//
// It is UI-free. ged.BookmarksPanel renders whatever flat list the host
// pushes into it and gui.CodeEditor keeps per-buffer marks that die with
// the buffer; this package is the part that outlives both, and is the only
// one that has to be deterministic on disk.
package bookmarks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxDrift caps how far Reanchor will look for a line that moved. Beyond
// a couple hundred lines a same-text hit says more about the language
// (a lone "}" repeats everywhere) than about the bookmark, so a stale
// line number is the better answer.
const maxDrift = 200

// Bookmark is one mark.
//
// Line is 1-based, matching what a gutter shows and how Reanchor indexes
// the file's lines (lines[Line-1]). Context is the text of that line when
// the mark was set — Reanchor's only way to find the line again after it
// moved — and Note is the user's label.
type Bookmark struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Context string `json:"context,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Store is a set of bookmarks, unique by (Path, Line). It is not
// goroutine-safe: the IDE drives it from the UI thread.
type Store struct {
	marks []Bookmark
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{}
}

// Add inserts b, or refreshes the Context and Note of the mark already at
// (b.Path, b.Line). A mark with no Path or a Line below 1 is rejected: it
// can be neither jumped to nor re-anchored. Reports whether a new mark
// was inserted.
func (s *Store) Add(b Bookmark) bool {
	if b.Path == "" || b.Line < 1 {
		return false
	}
	if i := s.indexOf(b.Path, b.Line); i >= 0 {
		s.marks[i].Context = b.Context
		s.marks[i].Note = b.Note
		return false
	}
	s.marks = append(s.marks, b)
	return true
}

// Remove deletes the mark at (path, line) and reports whether one was
// there.
func (s *Store) Remove(path string, line int) bool {
	i := s.indexOf(path, line)
	if i < 0 {
		return false
	}
	s.marks = append(s.marks[:i], s.marks[i+1:]...)
	return true
}

// Toggle drops the mark at (b.Path, b.Line) when one exists and adds b
// otherwise. It reports whether the mark is set afterwards, which is what
// a gutter click needs in order to redraw itself.
func (s *Store) Toggle(b Bookmark) bool {
	if s.Remove(b.Path, b.Line) {
		return false
	}
	return s.Add(b)
}

// List returns every mark ordered by path then line. The slice is a copy
// and the order does not depend on insertion order, so the result can be
// rendered, diffed or written to disk as-is.
func (s *Store) List() []Bookmark {
	return sorted(s.marks)
}

// ByFile returns path's marks, ordered by line.
func (s *Store) ByFile(path string) []Bookmark {
	var out []Bookmark
	for _, m := range s.marks {
		if m.Path == path {
			out = append(out, m)
		}
	}
	return sorted(out)
}

// Save writes the store to path as a JSON array in List order, creating
// the parent directory when needed. The bytes go to a sibling temp file
// that is renamed into place, so a crash mid-write cannot truncate the
// bookmark file.
func (s *Store) Save(path string) error {
	data, err := json.MarshalIndent(s.List(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Load reads a bookmark file written by Save. Entries go through Add, so a
// hand-edited file with duplicate or unusable marks loads clean instead of
// poisoning the store. A missing file is returned as an error — check
// os.IsNotExist to tell "no bookmarks yet" from "unreadable file".
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := NewStore()
	if len(strings.TrimSpace(string(data))) == 0 {
		return s, nil
	}
	var marks []Bookmark
	if err := json.Unmarshal(data, &marks); err != nil {
		return nil, err
	}
	for _, m := range marks {
		s.Add(m)
	}
	return s, nil
}

// Reanchor re-points path's bookmarks at the file's current text, lines
// being the file split into lines (lines[0] is line 1). A mark whose
// Context still matches its line is left alone; one whose line no longer
// carries its Context is moved to the nearest line within maxDrift that
// does, closest first and the lower line number on a tie. A mark with no
// Context, or whose Context is gone from that window, keeps its line — a
// stale line number is more useful than a silently dropped bookmark.
//
// Matching ignores leading and trailing whitespace, so a re-indented line
// still counts as the same line. When a move lands on a line that already
// has a mark the two are merged, keeping the older one. Returns the number
// of marks that moved.
func (s *Store) Reanchor(path string, lines []string) int {
	moved := 0
	for i := range s.marks {
		m := &s.marks[i]
		if m.Path != path {
			continue
		}
		want := strings.TrimSpace(m.Context)
		if want == "" {
			continue
		}
		if matchesLine(lines, m.Line, want) {
			continue
		}
		line, ok := findNearest(lines, m.Line, want)
		if !ok {
			continue
		}
		m.Line = line
		moved++
	}
	if moved > 0 {
		s.dedupe()
	}
	return moved
}

// matchesLine reports whether the 1-based line carries want, ignoring the
// whitespace around it. An out-of-range line never matches.
func matchesLine(lines []string, line int, want string) bool {
	if line < 1 || line > len(lines) {
		return false
	}
	return strings.TrimSpace(lines[line-1]) == want
}

// findNearest returns the 1-based line closest to from whose text is want,
// scanning outward up to maxDrift lines each way. Ties go to the lower
// line number.
func findNearest(lines []string, from int, want string) (int, bool) {
	for d := 1; d <= maxDrift; d++ {
		if up := from - d; matchesLine(lines, up, want) {
			return up, true
		}
		if down := from + d; matchesLine(lines, down, want) {
			return down, true
		}
		// Both ends have left the file — nothing further to scan.
		if from-d < 1 && from+d > len(lines) {
			break
		}
	}
	return 0, false
}

// dedupe collapses marks that ended up sharing (Path, Line) after a
// Reanchor move, keeping the first — the older — one.
func (s *Store) dedupe() {
	type key struct {
		path string
		line int
	}
	seen := make(map[key]bool, len(s.marks))
	out := s.marks[:0]
	for _, m := range s.marks {
		k := key{m.Path, m.Line}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, m)
	}
	s.marks = out
}

// indexOf returns the slice index of the mark at (path, line), or -1.
func (s *Store) indexOf(path string, line int) int {
	for i := range s.marks {
		if s.marks[i].Path == path && s.marks[i].Line == line {
			return i
		}
	}
	return -1
}

// sorted returns a copy of in ordered by path then line.
func sorted(in []Bookmark) []Bookmark {
	out := make([]Bookmark, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}

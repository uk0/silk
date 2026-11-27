package gui

import (
	"slices"
	"strings"
)

const (
	editorSnapshotBlockLines = 128
	editorSnapshotMinLines   = 1024
)

// editorTextSnapshot indexes blocks of the line model that produced the last
// published immutable string. A local edit replaces a single body in that
// snapshot; nothing writes through to the prior string held by host callbacks.
type editorTextSnapshot struct {
	blocks []int // Fenwick tree of byte lengths including a separator per row
	rows   int
	edit   editorSnapshotLineEdit
}

type editorSnapshotLineEdit struct {
	row, offset int
	before      string
	local, bulk bool
}

func (s *editorTextSnapshot) reset(lines []string) {
	s.edit = editorSnapshotLineEdit{}
	if len(lines) < editorSnapshotMinLines {
		s.blocks, s.rows = nil, 0
		return
	}
	n := (len(lines)-1)/editorSnapshotBlockLines + 2 // slot 0 is empty
	s.blocks = slices.Grow(s.blocks[:0], n)[:n]
	clear(s.blocks)
	s.rows = len(lines)
	for row, line := range lines {
		s.blocks[row/editorSnapshotBlockLines+1] += len(line) + 1
	}
	for i := 1; i < n; i++ {
		if parent := i + (i & -i); parent < n {
			s.blocks[parent] += s.blocks[i]
		}
	}
}

func (s *editorTextSnapshot) lineStart(lines []string, row int) int {
	b := row / editorSnapshotBlockLines
	start := 0
	for i := b; i > 0; i -= i & -i {
		start += s.blocks[i]
	}
	for _, line := range lines[b*editorSnapshotBlockLines : row] {
		start += len(line) + 1
	}
	return start
}

func (s *editorTextSnapshot) markBulk() {
	s.edit = editorSnapshotLineEdit{bulk: true}
}

func (s *editorTextSnapshot) note(lines []string, row int, before, after []string) {
	if s.rows == 0 || s.edit.bulk {
		return
	}
	if s.rows != len(lines) || len(before) != 1 || len(after) != 1 ||
		(s.edit.local && s.edit.row != row) {
		s.markBulk()
		return
	}
	if !s.edit.local {
		s.edit = editorSnapshotLineEdit{row: row, offset: s.lineStart(lines, row), before: before[0], local: true}
	}
}

// publish combines the affected body with two slices from the old snapshot.
// Structural edits fall back to the complete line model and rebuild the
// small index. A previously published string remains valid across both paths.
func (s *editorTextSnapshot) publish(previous string, lines []string) (text string, changed bool) {
	edit := s.edit
	if edit.local && len(lines) == s.rows && edit.row < len(lines) {
		end := edit.offset + len(edit.before)
		if end <= len(previous) && previous[edit.offset:end] == edit.before {
			s.edit = editorSnapshotLineEdit{}
			body := lines[edit.row]
			if body == edit.before {
				return previous, false
			}
			delta := len(body) - len(edit.before)
			for i := edit.row/editorSnapshotBlockLines + 1; i < len(s.blocks); i += i & -i {
				s.blocks[i] += delta
			}
			return previous[:edit.offset] + body + previous[end:], true
		}
	}

	text = strings.Join(lines, "\n")
	s.reset(lines)
	return text, text != previous
}

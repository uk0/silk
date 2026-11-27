package gui

import (
	"slices"
	"strings"
)

const (
	editorSnapshotMinLines     = 1024
	editorSnapshotMaxLocalRows = 16
)

// editorTextSnapshot indexes blocks of the line model that produced the last
// published immutable string. A small set of local edits replaces only their
// bodies; the prior string held by host callbacks is always immutable.
type editorTextSnapshot struct {
	editorLineByteIndex
	edit editorSnapshotEdit
}

type editorSnapshotLineEdit struct {
	row, offset int
	before      string
}

type editorSnapshotEdit struct {
	first       editorSnapshotLineEdit
	additional  []editorSnapshotLineEdit
	local, bulk bool
}

func (s *editorTextSnapshot) reset(lines []string) {
	s.edit = editorSnapshotEdit{}
	if len(lines) < editorSnapshotMinLines {
		s.editorLineByteIndex = editorLineByteIndex{}
		return
	}
	s.editorLineByteIndex.reset(lines)
}

func (s *editorTextSnapshot) markBulk() {
	s.edit = editorSnapshotEdit{bulk: true}
}

func (s *editorTextSnapshot) note(lines []string, row int, before, after []string) {
	if s.rows == 0 || s.edit.bulk {
		return
	}
	if s.rows != len(lines) || len(before) != len(after) || len(before) > editorSnapshotMaxLocalRows {
		s.markBulk()
		return
	}
	for i, old := range before {
		s.noteLocalRow(lines, row+i, old)
		if s.edit.bulk {
			return
		}
	}
}

func (s *editorTextSnapshot) noteLocalRow(lines []string, row int, before string) {
	if !s.edit.local {
		s.edit = editorSnapshotEdit{
			first: editorSnapshotLineEdit{row: row, offset: s.lineStart(lines, row), before: before},
			local: true,
		}
		return
	}
	if s.edit.first.row == row {
		return
	}
	offset := s.lineStart(lines, row)
	blockStart := row / editorLineByteBlockLines * editorLineByteBlockLines
	// The tree stores published block sizes, but lineStart reads the live
	// bodies inside this block. Correct only earlier edits in the same block.
	// Some callers stage several writes before mutating any rows, so consult
	// the current model instead of guessing which of those writes has landed.
	if first := s.edit.first; first.row >= blockStart && first.row < row {
		offset += len(first.before) - len(lines[first.row])
	}
	for _, other := range s.edit.additional {
		if other.row == row {
			return
		}
		if other.row >= blockStart && other.row < row {
			offset += len(other.before) - len(lines[other.row])
		}
	}
	if len(s.edit.additional)+1 >= editorSnapshotMaxLocalRows {
		s.markBulk()
		return
	}
	s.edit.additional = append(s.edit.additional, editorSnapshotLineEdit{row, offset, before})
}

func (s *editorTextSnapshot) publishSparse(previous string, lines []string) (text string, changed, ok bool) {
	var local [editorSnapshotMaxLocalRows]editorSnapshotLineEdit
	edits := local[:len(s.edit.additional)+1]
	edits[0] = s.edit.first
	copy(edits[1:], s.edit.additional)
	slices.SortFunc(edits, func(a, b editorSnapshotLineEdit) int { return a.row - b.row })
	size, cursor := len(previous), 0
	for _, edit := range edits {
		end := edit.offset + len(edit.before)
		if edit.offset < cursor || end > len(previous) || previous[edit.offset:end] != edit.before {
			return "", false, false
		}
		body := lines[edit.row]
		changed = changed || body != edit.before
		size += len(body) - len(edit.before)
		cursor = end
	}
	s.edit = editorSnapshotEdit{}
	if !changed {
		return previous, false, true
	}

	var next strings.Builder
	next.Grow(size)
	cursor = 0
	for _, edit := range edits {
		body := lines[edit.row]
		next.WriteString(previous[cursor:edit.offset])
		next.WriteString(body)
		cursor = edit.offset + len(edit.before)
		s.applyRowByteDelta(edit.row, len(body)-len(edit.before))
	}
	next.WriteString(previous[cursor:])
	return next.String(), true, true
}

// publish combines edited bodies with slices from the old immutable string.
// Large or structurally untracked edits reconcile the complete line model.
func (s *editorTextSnapshot) publish(previous string, lines []string) (text string, changed bool) {
	if s.edit.local && len(lines) == s.rows {
		if len(s.edit.additional) != 0 {
			if text, changed, ok := s.publishSparse(previous, lines); ok {
				return text, changed
			}
		} else if edit := s.edit.first; edit.row < len(lines) {
			end := edit.offset + len(edit.before)
			if end <= len(previous) && previous[edit.offset:end] == edit.before {
				s.edit = editorSnapshotEdit{}
				body := lines[edit.row]
				if body == edit.before {
					return previous, false
				}
				s.applyRowByteDelta(edit.row, len(body)-len(edit.before))
				return previous[:edit.offset] + body + previous[end:], true
			}
		}
	}

	text = strings.Join(lines, "\n")
	s.reset(lines)
	return text, text != previous
}

package gui

import (
	"fmt"
	"sort"
)

// --- Multi-Cursor / Rectangular Selection Engine ---
//
// This file is a pure model: it owns no widget, draws nothing and measures no
// font, so it is testable without a GL context. CodeEditor tracks a primary
// caret (cursorLine/cursorCol), one selection (selStart*/selEnd*) and a flat
// []cursorPos of secondary carets with NO selection of their own. This engine is
// the richer model behind Qt Creator's multi-cursor and block (Alt-drag)
// selection: every cursor carries its own selection range, a sticky column for
// vertical motion, and the set knows how to hand the editor a batch of edits in
// an order that stays valid while it is applied.
//
// Coordinates match CodeEditor everywhere: 0-based line index, 0-based rune
// column (NOT byte offset), end column exclusive.

// SelectionMetrics supplies the buffer geometry the engine needs to clamp
// cursors: how many lines exist and how many runes each line holds. CodeEditor
// satisfies it via SelectionLines(this.lines).
type SelectionMetrics interface {
	// LineCount returns the number of lines; always >= 1 for a valid buffer.
	LineCount() int
	// LineRunes returns the rune length of line, or 0 when line is out of range.
	LineRunes(line int) int
}

// SelectionLines adapts a plain []string buffer (CodeEditor.lines) to
// SelectionMetrics.
type SelectionLines []string

// LineCount returns the line count, reporting 1 for an empty slice because an
// empty buffer still holds one empty line.
func (s SelectionLines) LineCount() int {
	if len(s) == 0 {
		return 1
	}
	return len(s)
}

// LineRunes returns the rune length of line, or 0 when line is out of range.
func (s SelectionLines) LineRunes(line int) int {
	if line < 0 || line >= len(s) {
		return 0
	}
	return len([]rune(s[line]))
}

// EditorCursor is one caret plus the selection anchor that belongs to it. The
// selection spans anchor..caret in either direction; an anchor equal to the
// caret means "no selection" (the same rule CodeEditor uses for hasSelection).
//
// DesiredCol is the sticky column vertical motion aims for. Moving down onto a
// short line clamps Col to that line's length but leaves DesiredCol alone, so
// continuing onto a longer line restores the original column. A negative
// DesiredCol means "unset": treat Col as the sticky column.
type EditorCursor struct {
	Line int
	Col  int

	AnchorLine int
	AnchorCol  int

	DesiredCol int
}

// NewEditorCursor returns a collapsed cursor at (line, col) whose sticky column
// is col.
func NewEditorCursor(line, col int) EditorCursor {
	return EditorCursor{Line: line, Col: col, AnchorLine: line, AnchorCol: col, DesiredCol: col}
}

// HasSelection reports whether the cursor covers a non-empty range.
func (c EditorCursor) HasSelection() bool {
	return c.AnchorLine != c.Line || c.AnchorCol != c.Col
}

// Range returns the cursor's selection normalized so start <= end. A collapsed
// cursor returns its caret position twice (a zero-width range).
func (c EditorCursor) Range() (startLine, startCol, endLine, endCol int) {
	if selPosCmp(c.AnchorLine, c.AnchorCol, c.Line, c.Col) <= 0 {
		return c.AnchorLine, c.AnchorCol, c.Line, c.Col
	}
	return c.Line, c.Col, c.AnchorLine, c.AnchorCol
}

// StickyCol returns the column vertical motion should aim for: DesiredCol when
// it is set, otherwise the current Col.
func (c EditorCursor) StickyCol() int {
	if c.DesiredCol < 0 {
		return c.Col
	}
	return c.DesiredCol
}

// Collapse drops the selection by moving the anchor onto the caret.
func (c *EditorCursor) Collapse() {
	c.AnchorLine, c.AnchorCol = c.Line, c.Col
}

// reversed reports whether the caret sits before its anchor (a backwards drag).
func (c EditorCursor) reversed() bool {
	return selPosCmp(c.Line, c.Col, c.AnchorLine, c.AnchorCol) < 0
}

// selPosCmp orders two (line, col) positions: -1 when a is before b, 0 when
// they are equal, +1 when a is after b.
func selPosCmp(aLine, aCol, bLine, bCol int) int {
	if aLine != bLine {
		if aLine < bLine {
			return -1
		}
		return 1
	}
	if aCol != bCol {
		if aCol < bCol {
			return -1
		}
		return 1
	}
	return 0
}

// SelectionSet is a primary cursor plus any number of secondary cursors, each
// with its own selection. It keeps itself normalized: cursors stay clamped to
// the buffer, secondary cursors stay ordered top-down, and cursors whose ranges
// overlap or touch are merged into one.
//
// column records that the current set came from AddCursorsForColumnBlock, i.e.
// it is a rectangular (block) selection rather than a scatter of carets. It only
// affects the undo description CollectEdits produces.
type SelectionSet struct {
	metrics   SelectionMetrics
	primary   EditorCursor
	secondary []EditorCursor
	column    bool
}

// NewSelectionSet returns a single-cursor set at (line, col), clamped to m.
// A nil m behaves as a one-line empty buffer.
func NewSelectionSet(m SelectionMetrics, line, col int) *SelectionSet {
	s := &SelectionSet{metrics: m}
	s.primary = s.clamp(NewEditorCursor(line, col))
	return s
}

// SetMetrics swaps the buffer geometry (call it after the text changed) and
// re-clamps every cursor to the new bounds.
func (s *SelectionSet) SetMetrics(m SelectionMetrics) {
	s.metrics = m
	s.Normalize()
}

// Primary returns the primary cursor — the one that owns the visible caret and
// that keyboard-driven motion reports back to the editor.
func (s *SelectionSet) Primary() EditorCursor { return s.primary }

// SetPrimary replaces the primary cursor, then re-normalizes the set.
func (s *SelectionSet) SetPrimary(c EditorCursor) {
	s.primary = c
	s.Normalize()
}

// Secondary returns a copy of the secondary cursors, ordered top-down.
func (s *SelectionSet) Secondary() []EditorCursor {
	if len(s.secondary) == 0 {
		return nil
	}
	return append([]EditorCursor(nil), s.secondary...)
}

// Cursors returns every cursor (primary included) ordered top-down by range
// start — the order to walk for rendering.
func (s *SelectionSet) Cursors() []EditorCursor {
	out := make([]EditorCursor, 0, len(s.secondary)+1)
	out = append(out, s.primary)
	out = append(out, s.secondary...)
	sortCursorsTopDown(out)
	return out
}

// Count returns the number of cursors, always at least 1.
func (s *SelectionSet) Count() int { return len(s.secondary) + 1 }

// ColumnMode reports whether the set is a rectangular block selection.
func (s *SelectionSet) ColumnMode() bool { return s.column }

// HasSelection reports whether any cursor covers a non-empty range.
func (s *SelectionSet) HasSelection() bool {
	if s.primary.HasSelection() {
		return true
	}
	for _, c := range s.secondary {
		if c.HasSelection() {
			return true
		}
	}
	return false
}

// AddCursor adds a secondary caret at (line, col) with no selection. A caret
// that lands on another cursor, or inside another cursor's selection, is merged
// away by Normalize, so adding the same position twice is a no-op.
func (s *SelectionSet) AddCursor(line, col int) {
	s.secondary = append(s.secondary, s.clamp(NewEditorCursor(line, col)))
	s.Normalize()
}

// AddSelection adds a secondary cursor selecting anchor..caret, with its caret
// at (line, col) so a following MoveAll extends from the right end. Used for
// "select next occurrence", where every match gets its own range.
func (s *SelectionSet) AddSelection(anchorLine, anchorCol, line, col int) {
	c := EditorCursor{Line: line, Col: col, AnchorLine: anchorLine, AnchorCol: anchorCol, DesiredCol: col}
	s.secondary = append(s.secondary, s.clamp(c))
	s.Normalize()
}

// AddCursorsForColumnBlock replaces the set with the rectangular block spanned
// by the corners (l1,c1) and (l2,c2): one cursor per line of the block, each
// selecting the block's column span. Ragged lines clamp — a line shorter than
// the block's left column collapses to an empty range at its end — so a block
// over uneven text still yields exactly one cursor per line.
//
// Every caret sits at the block's right column with its anchor at the left one,
// regardless of drag direction, and every cursor's sticky column is the block's
// (unclamped) right column so moving the block vertically keeps its shape. The
// primary lands on l2, the line where the drag ended. ColumnMode reports true
// afterwards.
func (s *SelectionSet) AddCursorsForColumnBlock(l1, c1, l2, c2 int) {
	top, bottom := s.clampLine(l1), s.clampLine(l2)
	if top > bottom {
		top, bottom = bottom, top
	}
	left, right := c1, c2
	if left > right {
		left, right = right, left
	}
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	primaryLine := s.clampLine(l2)

	sec := make([]EditorCursor, 0, bottom-top)
	for line := top; line <= bottom; line++ {
		lr := s.lineRunes(line)
		a, b := left, right
		if a > lr {
			a = lr
		}
		if b > lr {
			b = lr
		}
		c := EditorCursor{Line: line, Col: b, AnchorLine: line, AnchorCol: a, DesiredCol: right}
		if line == primaryLine {
			s.primary = c
			continue
		}
		sec = append(sec, c)
	}
	s.secondary = sec
	s.column = true
	s.Normalize()
}

// ClearSecondary drops every secondary cursor and leaves column mode, keeping
// the primary cursor and its selection. This is the Esc path back to
// single-cursor editing.
func (s *SelectionSet) ClearSecondary() {
	s.secondary = nil
	s.column = false
}

// MoveAll moves every cursor by (dLine, dCol) and re-normalizes the set.
//
// Horizontal motion steps across line boundaries the way Left/Right does and
// resets each cursor's sticky column to where it landed. Vertical motion aims at
// the sticky column, clamping only the resulting Col so DesiredCol survives a
// short line. When both deltas are non-zero the horizontal step runs first.
//
// extend=true keeps each cursor's anchor, growing (or shrinking) its own
// selection; extend=false collapses every cursor onto its new caret.
func (s *SelectionSet) MoveAll(dLine, dCol int, extend bool) {
	s.primary = s.moveCursor(s.primary, dLine, dCol, extend)
	for i := range s.secondary {
		s.secondary[i] = s.moveCursor(s.secondary[i], dLine, dCol, extend)
	}
	s.Normalize()
}

// moveCursor applies one (dLine, dCol) step to a single cursor.
func (s *SelectionSet) moveCursor(c EditorCursor, dLine, dCol int, extend bool) EditorCursor {
	if dCol != 0 {
		line, col := c.Line, c.Col+dCol
		for col < 0 && line > 0 {
			line--
			col += s.lineRunes(line) + 1
		}
		for line < s.lineCount()-1 && col > s.lineRunes(line) {
			col -= s.lineRunes(line) + 1
			line++
		}
		c.Line, c.Col = s.clampPos(line, col)
		c.DesiredCol = c.Col
	}
	if dLine != 0 {
		line := s.clampLine(c.Line + dLine)
		col := c.StickyCol()
		if lr := s.lineRunes(line); col > lr {
			col = lr
		}
		if col < 0 {
			col = 0
		}
		// DesiredCol is deliberately left alone: the clamp above is what a
		// short line does to Col, not a change of intent.
		c.Line, c.Col = line, col
	}
	if !extend {
		c.Collapse()
	}
	return c
}

// Normalize clamps every cursor to the buffer, orders the set top-down and
// merges cursors whose ranges overlap or touch (which is also how duplicate
// carets are deduped). The primary cursor survives every merge it takes part in,
// keeping its own drag direction. Normalize is idempotent.
func (s *SelectionSet) Normalize() {
	type entry struct {
		c       EditorCursor
		primary bool
	}
	all := make([]entry, 0, len(s.secondary)+1)
	all = append(all, entry{c: s.clamp(s.primary), primary: true})
	for _, c := range s.secondary {
		all = append(all, entry{c: s.clamp(c)})
	}
	// Sort by range start then range end; stable so the primary (appended
	// first) wins ties and merge direction stays deterministic.
	sort.SliceStable(all, func(i, j int) bool {
		isl, isc, iel, iec := all[i].c.Range()
		jsl, jsc, jel, jec := all[j].c.Range()
		if cmp := selPosCmp(isl, isc, jsl, jsc); cmp != 0 {
			return cmp < 0
		}
		return selPosCmp(iel, iec, jel, jec) < 0
	})

	merged := make([]entry, 0, len(all))
	for _, e := range all {
		if len(merged) == 0 {
			merged = append(merged, e)
			continue
		}
		prev := &merged[len(merged)-1]
		psl, psc, pel, pec := prev.c.Range()
		sl, sc, el, ec := e.c.Range()
		if selPosCmp(sl, sc, pel, pec) > 0 {
			merged = append(merged, e)
			continue
		}
		// Overlapping or touching: fuse into the union of both ranges. prev
		// starts first by sort order, so only the end can grow.
		if selPosCmp(el, ec, pel, pec) > 0 {
			pel, pec = el, ec
		}
		reversed := prev.c.reversed()
		desired := prev.c.DesiredCol
		if e.primary {
			reversed = e.c.reversed()
			desired = e.c.DesiredCol
		}
		if reversed {
			prev.c = EditorCursor{Line: psl, Col: psc, AnchorLine: pel, AnchorCol: pec, DesiredCol: desired}
		} else {
			prev.c = EditorCursor{Line: pel, Col: pec, AnchorLine: psl, AnchorCol: psc, DesiredCol: desired}
		}
		prev.primary = prev.primary || e.primary
	}

	pi := 0
	for i := range merged {
		if merged[i].primary {
			pi = i
			break
		}
	}
	s.primary = merged[pi].c
	if len(merged) == 1 {
		s.secondary = nil
		return
	}
	sec := make([]EditorCursor, 0, len(merged)-1)
	for i := range merged {
		if i != pi {
			sec = append(sec, merged[i].c)
		}
	}
	s.secondary = sec
}

// SelectionEdit is one buffer replacement: the normalized range
// [(StartLine,StartCol) .. (EndLine,EndCol)) and the text that replaces it. A
// zero-width range is a plain insertion at that position.
type SelectionEdit struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	Text      string
}

// Empty reports whether the edit's range is zero-width (a pure insertion).
func (e SelectionEdit) Empty() bool {
	return e.StartLine == e.EndLine && e.StartCol == e.EndCol
}

// CompoundEdit is a whole multi-cursor edit: Edits in the order they must be
// applied, plus one Description so the batch collapses into a single undo step.
type CompoundEdit struct {
	// Edits is ordered bottom-up (last position first). Applying them in this
	// order keeps every later entry's coordinates valid, because an edit can
	// only shift text that comes after it.
	Edits []SelectionEdit
	// Description labels the batch for the undo stack, e.g.
	// "column replace at 4 cursors".
	Description string
}

// CollectEdits turns the set into the replacement of every cursor's range with
// replacement, in safe apply order. Collapsed cursors yield zero-width
// insertions, so typing a character and replacing three selections both come
// out as one CompoundEdit.
func (s *SelectionSet) CollectEdits(replacement string) CompoundEdit {
	cursors := s.Cursors()
	edits := make([]SelectionEdit, 0, len(cursors))
	for _, c := range cursors {
		sl, sc, el, ec := c.Range()
		edits = append(edits, SelectionEdit{
			StartLine: sl,
			StartCol:  sc,
			EndLine:   el,
			EndCol:    ec,
			Text:      replacement,
		})
	}
	// Bottom-up: descending by range start.
	sort.SliceStable(edits, func(i, j int) bool {
		return selPosCmp(edits[i].StartLine, edits[i].StartCol, edits[j].StartLine, edits[j].StartCol) > 0
	})
	return CompoundEdit{Edits: edits, Description: s.editDescription(replacement, edits)}
}

// editDescription builds the compound-undo label for a batch.
func (s *SelectionSet) editDescription(replacement string, edits []SelectionEdit) string {
	allEmpty := true
	for _, e := range edits {
		if !e.Empty() {
			allEmpty = false
			break
		}
	}
	verb := "replace"
	switch {
	case allEmpty:
		verb = "insert"
	case replacement == "":
		verb = "delete"
	}
	noun := "cursors"
	if len(edits) == 1 {
		noun = "cursor"
	}
	prefix := ""
	if s.column {
		prefix = "column "
	}
	return fmt.Sprintf("%s%s at %d %s", prefix, verb, len(edits), noun)
}

// sortCursorsTopDown orders cursors by range start then range end.
func sortCursorsTopDown(cs []EditorCursor) {
	sort.SliceStable(cs, func(i, j int) bool {
		isl, isc, iel, iec := cs[i].Range()
		jsl, jsc, jel, jec := cs[j].Range()
		if cmp := selPosCmp(isl, isc, jsl, jsc); cmp != 0 {
			return cmp < 0
		}
		return selPosCmp(iel, iec, jel, jec) < 0
	})
}

// clamp pins a cursor's caret and anchor inside the buffer. DesiredCol is NOT
// clamped — surviving a short line is its whole purpose — but a negative
// (unset) DesiredCol is resolved to the caret column.
func (s *SelectionSet) clamp(c EditorCursor) EditorCursor {
	c.Line, c.Col = s.clampPos(c.Line, c.Col)
	c.AnchorLine, c.AnchorCol = s.clampPos(c.AnchorLine, c.AnchorCol)
	if c.DesiredCol < 0 {
		c.DesiredCol = c.Col
	}
	return c
}

// clampPos pins a (line, col) position inside the buffer.
func (s *SelectionSet) clampPos(line, col int) (int, int) {
	line = s.clampLine(line)
	if col < 0 {
		col = 0
	}
	if lr := s.lineRunes(line); col > lr {
		col = lr
	}
	return line, col
}

// clampLine pins a line index inside the buffer.
func (s *SelectionSet) clampLine(line int) int {
	if line < 0 {
		return 0
	}
	if n := s.lineCount(); line >= n {
		return n - 1
	}
	return line
}

// lineCount returns the buffer's line count, treating a nil or empty metrics
// source as one empty line.
func (s *SelectionSet) lineCount() int {
	if s.metrics == nil {
		return 1
	}
	if n := s.metrics.LineCount(); n > 0 {
		return n
	}
	return 1
}

// lineRunes returns the rune length of line, never negative.
func (s *SelectionSet) lineRunes(line int) int {
	if s.metrics == nil {
		return 0
	}
	if n := s.metrics.LineRunes(line); n > 0 {
		return n
	}
	return 0
}

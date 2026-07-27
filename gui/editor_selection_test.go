package gui

import (
	"strings"
	"testing"
)

// Pure model tests: no Frame, no Draw, no font measuring. The selection engine
// only knows line counts and rune lengths, so everything here runs headless.

// applySelectionEdits applies ce.Edits to lines in the order they were handed
// out, mirroring what the editor does with a CompoundEdit. Ranges may span lines
// and Text may contain newlines. Test helper only — the engine itself never
// touches text.
func applySelectionEdits(lines []string, ce CompoundEdit) []string {
	out := append([]string(nil), lines...)
	for _, e := range ce.Edits {
		head := string([]rune(out[e.StartLine])[:e.StartCol])
		tail := string([]rune(out[e.EndLine])[e.EndCol:])
		next := append([]string(nil), out[:e.StartLine]...)
		next = append(next, strings.Split(head+e.Text+tail, "\n")...)
		next = append(next, out[e.EndLine+1:]...)
		out = next
	}
	return out
}

// reverseCompound returns ce with its edits in the opposite (unsafe, top-down)
// order, so tests can prove the order the engine picked actually matters.
func reverseCompound(ce CompoundEdit) CompoundEdit {
	out := CompoundEdit{Description: ce.Description}
	for i := len(ce.Edits) - 1; i >= 0; i-- {
		out.Edits = append(out.Edits, ce.Edits[i])
	}
	return out
}

func joinLines(lines []string) string { return strings.Join(lines, "|") }

// checkRange asserts a cursor's normalized selection range.
func checkRange(t *testing.T, label string, c EditorCursor, wsl, wsc, wel, wec int) {
	t.Helper()
	sl, sc, el, ec := c.Range()
	if sl != wsl || sc != wsc || el != wel || ec != wec {
		t.Errorf("%s: range = (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)",
			label, sl, sc, el, ec, wsl, wsc, wel, wec)
	}
}

// --- SelectionLines / metrics ---

func TestSelectionLinesMetrics(t *testing.T) {
	if got := (SelectionLines{}).LineCount(); got != 1 {
		t.Errorf("empty buffer LineCount = %d, want 1", got)
	}
	buf := SelectionLines{"héllo", ""}
	if got := buf.LineCount(); got != 2 {
		t.Errorf("LineCount = %d, want 2", got)
	}
	// Rune columns, not bytes: "héllo" is 6 bytes but 5 runes.
	if got := buf.LineRunes(0); got != 5 {
		t.Errorf("LineRunes(0) = %d, want 5", got)
	}
	if got := buf.LineRunes(1); got != 0 {
		t.Errorf("LineRunes(1) = %d, want 0", got)
	}
	if got := buf.LineRunes(-1); got != 0 {
		t.Errorf("LineRunes(-1) = %d, want 0", got)
	}
	if got := buf.LineRunes(9); got != 0 {
		t.Errorf("LineRunes(9) = %d, want 0", got)
	}
}

func TestSelectionSetNilMetricsClampsToOrigin(t *testing.T) {
	s := NewSelectionSet(nil, 7, 9)
	p := s.Primary()
	if p.Line != 0 || p.Col != 0 {
		t.Fatalf("primary = (%d,%d), want (0,0)", p.Line, p.Col)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
}

func TestEditorCursorRangeAndSticky(t *testing.T) {
	fwd := EditorCursor{Line: 2, Col: 6, AnchorLine: 1, AnchorCol: 3, DesiredCol: 6}
	checkRange(t, "forward", fwd, 1, 3, 2, 6)
	if fwd.reversed() {
		t.Error("forward cursor reported reversed")
	}
	back := EditorCursor{Line: 1, Col: 3, AnchorLine: 2, AnchorCol: 6, DesiredCol: 3}
	checkRange(t, "backward", back, 1, 3, 2, 6)
	if !back.reversed() {
		t.Error("backward cursor not reported reversed")
	}
	if !fwd.HasSelection() || !back.HasSelection() {
		t.Error("non-empty cursors report HasSelection false")
	}
	c := NewEditorCursor(4, 11)
	if c.HasSelection() {
		t.Error("fresh cursor has a selection")
	}
	if c.StickyCol() != 11 {
		t.Errorf("StickyCol = %d, want 11", c.StickyCol())
	}
	c.DesiredCol = -1
	if c.StickyCol() != 11 {
		t.Errorf("unset DesiredCol StickyCol = %d, want Col 11", c.StickyCol())
	}
}

// --- dedup / merge ---

func TestSelectionSetDedupDuplicateCarets(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"alpha", "beta"}, 0, 1)
	s.AddCursor(1, 2)
	s.AddCursor(1, 2) // exact duplicate of the secondary
	s.AddCursor(0, 1) // exact duplicate of the primary
	if s.Count() != 2 {
		t.Fatalf("Count = %d, want 2 (primary + one secondary)", s.Count())
	}
	sec := s.Secondary()
	if len(sec) != 1 || sec[0].Line != 1 || sec[0].Col != 2 {
		t.Fatalf("secondary = %+v, want single (1,2)", sec)
	}
	if p := s.Primary(); p.Line != 0 || p.Col != 1 {
		t.Errorf("primary moved to (%d,%d), want (0,1)", p.Line, p.Col)
	}
}

func TestSelectionSetMergeAbsorbsCaretInsideSelection(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"one two three"}, 0, 0)
	s.SetPrimary(EditorCursor{Line: 0, Col: 8, AnchorLine: 0, AnchorCol: 2, DesiredCol: 8})
	s.AddCursor(0, 5) // caret inside the primary's selection
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (caret absorbed)", s.Count())
	}
	checkRange(t, "primary", s.Primary(), 0, 2, 0, 8)
}

func TestSelectionSetMergeOverlappingSelections(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"one two three four"}, 0, 0)
	// Primary selects "one two" (forward), secondary selects "two three".
	s.SetPrimary(EditorCursor{Line: 0, Col: 7, AnchorLine: 0, AnchorCol: 0, DesiredCol: 7})
	s.AddSelection(0, 4, 0, 13)
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (overlap merged)", s.Count())
	}
	p := s.Primary()
	checkRange(t, "merged", p, 0, 0, 0, 13)
	if p.reversed() {
		t.Error("merged cursor flipped direction; primary was forward")
	}
	if p.Col != 13 || p.AnchorCol != 0 {
		t.Errorf("merged caret/anchor = %d/%d, want 13/0", p.Col, p.AnchorCol)
	}
}

func TestSelectionSetMergeKeepsPrimaryDirection(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"one two three four"}, 0, 0)
	// Primary is a backwards drag over "two three": caret left of the anchor.
	s.SetPrimary(EditorCursor{Line: 0, Col: 4, AnchorLine: 0, AnchorCol: 13, DesiredCol: 4})
	// A forward secondary that starts earlier, so it sorts first.
	s.AddSelection(0, 0, 0, 7)
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1", s.Count())
	}
	p := s.Primary()
	checkRange(t, "merged", p, 0, 0, 0, 13)
	if !p.reversed() {
		t.Error("merged cursor lost the primary's backwards direction")
	}
	if p.Col != 0 || p.AnchorCol != 13 {
		t.Errorf("merged caret/anchor = %d/%d, want 0/13", p.Col, p.AnchorCol)
	}
}

func TestSelectionSetMergeTouchingRanges(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"abcdefgh"}, 0, 0)
	s.SetPrimary(EditorCursor{Line: 0, Col: 3, AnchorLine: 0, AnchorCol: 1, DesiredCol: 3})
	s.AddSelection(0, 3, 0, 6) // starts exactly where the primary ends
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (touching ranges merge)", s.Count())
	}
	checkRange(t, "merged", s.Primary(), 0, 1, 0, 6)
}

func TestSelectionSetDisjointCursorsStayOrderedTopDown(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"aaaa", "bbbb", "cccc"}, 2, 1)
	s.AddCursor(0, 3)
	s.AddCursor(1, 0)
	if s.Count() != 3 {
		t.Fatalf("Count = %d, want 3", s.Count())
	}
	got := s.Cursors()
	want := [][2]int{{0, 3}, {1, 0}, {2, 1}}
	for i, w := range want {
		if got[i].Line != w[0] || got[i].Col != w[1] {
			t.Errorf("Cursors()[%d] = (%d,%d), want (%d,%d)", i, got[i].Line, got[i].Col, w[0], w[1])
		}
	}
	sec := s.Secondary()
	if len(sec) != 2 || sec[0].Line != 0 || sec[1].Line != 1 {
		t.Errorf("Secondary() not ordered top-down: %+v", sec)
	}
}

func TestSelectionSetAddCursorClampsOutOfRange(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"ab", "cdef"}, 0, 0)
	s.AddCursor(99, 99)
	if s.Count() != 2 {
		t.Fatalf("Count = %d, want 2", s.Count())
	}
	sec := s.Secondary()
	if sec[0].Line != 1 || sec[0].Col != 4 {
		t.Errorf("clamped cursor = (%d,%d), want (1,4)", sec[0].Line, sec[0].Col)
	}
}

func TestSelectionSetSetMetricsReclampsAndMerges(t *testing.T) {
	s := NewSelectionSet(SelectionLines{"aaaa", "bbbb", "cccc"}, 0, 0)
	s.AddCursor(1, 2)
	s.AddCursor(2, 3)
	if s.Count() != 3 {
		t.Fatalf("Count = %d, want 3", s.Count())
	}
	// Buffer shrank to a single empty line: every cursor collapses onto (0,0)
	// and the duplicates merge away.
	s.SetMetrics(SelectionLines{""})
	if s.Count() != 1 {
		t.Fatalf("Count after shrink = %d, want 1", s.Count())
	}
	if p := s.Primary(); p.Line != 0 || p.Col != 0 {
		t.Errorf("primary = (%d,%d), want (0,0)", p.Line, p.Col)
	}
}

// --- rectangular / column block ---

func TestSelectionSetColumnBlockRaggedLinesClamp(t *testing.T) {
	buf := SelectionLines{"abcdefghij", "abc", "", "xyz12345"}
	s := NewSelectionSet(buf, 0, 0)
	s.AddCursorsForColumnBlock(0, 2, 3, 6)

	if !s.ColumnMode() {
		t.Error("ColumnMode false after a column block")
	}
	if s.Count() != 4 {
		t.Fatalf("Count = %d, want 4 (one cursor per block line)", s.Count())
	}
	cs := s.Cursors()
	want := [][4]int{
		{0, 2, 0, 6}, // full width
		{1, 2, 1, 3}, // clamped to len("abc")
		{2, 0, 2, 0}, // empty line: collapses to an empty range
		{3, 2, 3, 6}, // full width
	}
	for i, w := range want {
		checkRange(t, "block line", cs[i], w[0], w[1], w[2], w[3])
		if cs[i].Line != i {
			t.Errorf("cursor %d on line %d, want %d", i, cs[i].Line, i)
		}
		if cs[i].DesiredCol != 6 {
			t.Errorf("cursor %d DesiredCol = %d, want the block's right column 6", i, cs[i].DesiredCol)
		}
	}
	if cs[2].HasSelection() {
		t.Error("cursor on the empty line reports a selection")
	}
	if p := s.Primary(); p.Line != 3 || p.Col != 6 {
		t.Errorf("primary = (%d,%d), want the drag end (3,6)", p.Line, p.Col)
	}
}

func TestSelectionSetColumnBlockUpwardDragPrimaryAtDragEnd(t *testing.T) {
	buf := SelectionLines{"0123456789", "0123456789", "0123456789", "0123456789"}
	s := NewSelectionSet(buf, 0, 0)
	// Dragged from (3,6) up-left to (1,2): corners normalize, primary stays on
	// the line the drag ended on.
	s.AddCursorsForColumnBlock(3, 6, 1, 2)
	if s.Count() != 3 {
		t.Fatalf("Count = %d, want 3", s.Count())
	}
	if p := s.Primary(); p.Line != 1 {
		t.Errorf("primary line = %d, want 1", p.Line)
	}
	for _, c := range s.Cursors() {
		checkRange(t, "block line", c, c.Line, 2, c.Line, 6)
	}
}

func TestSelectionSetColumnBlockClampsCornersToBuffer(t *testing.T) {
	buf := SelectionLines{"aaaa", "bbbb", "cccc"}
	s := NewSelectionSet(buf, 0, 0)
	s.AddCursorsForColumnBlock(-5, 3, 99, 1)
	if s.Count() != 3 {
		t.Fatalf("Count = %d, want 3 (block clamped to the buffer)", s.Count())
	}
	for _, c := range s.Cursors() {
		checkRange(t, "clamped block", c, c.Line, 1, c.Line, 3)
	}
	if p := s.Primary(); p.Line != 2 {
		t.Errorf("primary line = %d, want 2", p.Line)
	}
}

func TestSelectionSetColumnBlockMoveKeepsColumn(t *testing.T) {
	// Middle line is short, so the block clamps there and must recover below.
	buf := SelectionLines{"0123456789", "ab", "0123456789"}
	s := NewSelectionSet(buf, 0, 0)
	s.AddCursorsForColumnBlock(0, 4, 1, 8)
	cs := s.Cursors()
	checkRange(t, "line 0", cs[0], 0, 4, 0, 8)
	checkRange(t, "line 1 clamped", cs[1], 1, 2, 1, 2)

	// Push the block down one line: the caret that was clamped on the short line
	// recovers the block's column on the long line below.
	s.MoveAll(1, 0, false)
	cs = s.Cursors()
	if len(cs) != 2 {
		t.Fatalf("cursor count = %d, want 2", len(cs))
	}
	if cs[0].Line != 1 || cs[1].Line != 2 {
		t.Fatalf("block lines = %d,%d, want 1,2", cs[0].Line, cs[1].Line)
	}
	if cs[0].Col != 2 {
		t.Errorf("top caret col = %d, want 2 (clamped to the short line)", cs[0].Col)
	}
	if cs[1].Col != 8 {
		t.Errorf("bottom caret col = %d, want the sticky 8", cs[1].Col)
	}
	for i, c := range cs {
		if c.DesiredCol != 8 {
			t.Errorf("cursor %d DesiredCol = %d, want 8", i, c.DesiredCol)
		}
	}
}

func TestSelectionSetColumnBlockVerticalExtendMergesRanges(t *testing.T) {
	// Extending a block downward makes each cursor's range reach into the next
	// cursor's, so the merge rule fuses the whole block into one selection.
	// Growing a block is AddCursorsForColumnBlock with a new corner, not MoveAll.
	buf := SelectionLines{"0123456789", "ab", "0123456789"}
	s := NewSelectionSet(buf, 0, 0)
	s.AddCursorsForColumnBlock(0, 4, 1, 8)
	s.MoveAll(1, 0, true)
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (extended ranges merge)", s.Count())
	}
	checkRange(t, "merged block", s.Primary(), 0, 4, 2, 8)
}

func TestSelectionSetClearSecondaryKeepsPrimarySelection(t *testing.T) {
	buf := SelectionLines{"0123456789", "0123456789", "0123456789"}
	s := NewSelectionSet(buf, 0, 0)
	s.AddCursorsForColumnBlock(0, 1, 2, 4)
	if s.Count() != 3 || !s.ColumnMode() {
		t.Fatalf("setup: Count = %d, ColumnMode = %v", s.Count(), s.ColumnMode())
	}
	s.ClearSecondary()
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1", s.Count())
	}
	if s.ColumnMode() {
		t.Error("ColumnMode still set after ClearSecondary")
	}
	if s.Secondary() != nil {
		t.Error("Secondary() not empty after ClearSecondary")
	}
	checkRange(t, "primary", s.Primary(), 2, 1, 2, 4)
	if !s.HasSelection() {
		t.Error("primary selection dropped by ClearSecondary")
	}
}

// --- MoveAll ---

func TestSelectionSetMoveAllDesiredColSurvivesShortLine(t *testing.T) {
	buf := SelectionLines{"0123456789", "ab", "0123456789"}
	s := NewSelectionSet(buf, 0, 8)

	s.MoveAll(1, 0, false)
	p := s.Primary()
	if p.Line != 1 || p.Col != 2 {
		t.Fatalf("after down: (%d,%d), want (1,2) clamped to the short line", p.Line, p.Col)
	}
	if p.DesiredCol != 8 {
		t.Fatalf("DesiredCol = %d, want 8 preserved across the short line", p.DesiredCol)
	}

	s.MoveAll(1, 0, false)
	p = s.Primary()
	if p.Line != 2 || p.Col != 8 {
		t.Fatalf("after second down: (%d,%d), want (2,8) restored", p.Line, p.Col)
	}
	if p.DesiredCol != 8 {
		t.Errorf("DesiredCol = %d, want 8", p.DesiredCol)
	}

	// Going back up through the short line restores the column too.
	s.MoveAll(-2, 0, false)
	p = s.Primary()
	if p.Line != 0 || p.Col != 8 {
		t.Errorf("after up 2: (%d,%d), want (0,8)", p.Line, p.Col)
	}
}

func TestSelectionSetMoveAllHorizontalResetsDesiredCol(t *testing.T) {
	buf := SelectionLines{"0123456789", "ab", "0123456789"}
	s := NewSelectionSet(buf, 0, 8)
	s.MoveAll(0, -3, false)
	p := s.Primary()
	if p.Col != 5 || p.DesiredCol != 5 {
		t.Fatalf("after left 3: col/desired = %d/%d, want 5/5", p.Col, p.DesiredCol)
	}
	s.MoveAll(1, 0, false)
	if p = s.Primary(); p.Line != 1 || p.Col != 2 || p.DesiredCol != 5 {
		t.Errorf("after down: (%d,%d) desired %d, want (1,2) desired 5", p.Line, p.Col, p.DesiredCol)
	}
}

func TestSelectionSetMoveAllHorizontalCrossesLines(t *testing.T) {
	buf := SelectionLines{"ab", "cd"}
	s := NewSelectionSet(buf, 0, 2) // end of line 0
	s.MoveAll(0, 1, false)
	if p := s.Primary(); p.Line != 1 || p.Col != 0 {
		t.Fatalf("right off line end = (%d,%d), want (1,0)", p.Line, p.Col)
	}
	s.MoveAll(0, -1, false)
	if p := s.Primary(); p.Line != 0 || p.Col != 2 {
		t.Fatalf("left off line start = (%d,%d), want (0,2)", p.Line, p.Col)
	}
	// Clamped at the very start of the buffer.
	s.MoveAll(0, -99, false)
	if p := s.Primary(); p.Line != 0 || p.Col != 0 {
		t.Errorf("left past start = (%d,%d), want (0,0)", p.Line, p.Col)
	}
}

func TestSelectionSetMoveAllExtendVersusMove(t *testing.T) {
	buf := SelectionLines{"hello world"}
	s := NewSelectionSet(buf, 0, 5)

	s.MoveAll(0, 3, true)
	p := s.Primary()
	if !p.HasSelection() {
		t.Fatal("extend did not create a selection")
	}
	checkRange(t, "extended", p, 0, 5, 0, 8)

	// Extending further keeps the same anchor.
	s.MoveAll(0, 2, true)
	checkRange(t, "extended twice", s.Primary(), 0, 5, 0, 10)

	// A plain move collapses onto the new caret.
	s.MoveAll(0, 1, false)
	p = s.Primary()
	if p.HasSelection() {
		t.Errorf("plain move kept a selection: %+v", p)
	}
	if p.Col != 11 {
		t.Errorf("caret col = %d, want 11", p.Col)
	}
}

func TestSelectionSetMoveAllExtendPerCursorAnchors(t *testing.T) {
	buf := SelectionLines{"0123456789", "0123456789"}
	s := NewSelectionSet(buf, 0, 2)
	s.AddCursor(1, 6)
	s.MoveAll(0, 2, true)

	cs := s.Cursors()
	if len(cs) != 2 {
		t.Fatalf("cursor count = %d, want 2", len(cs))
	}
	checkRange(t, "cursor 0", cs[0], 0, 2, 0, 4)
	checkRange(t, "cursor 1", cs[1], 1, 6, 1, 8)
	for i, c := range cs {
		if !c.HasSelection() {
			t.Errorf("cursor %d has no selection after extend", i)
		}
	}

	// Now collapse both with a plain move.
	s.MoveAll(0, 1, false)
	for i, c := range s.Cursors() {
		if c.HasSelection() {
			t.Errorf("cursor %d still selected after a plain move: %+v", i, c)
		}
	}
}

func TestSelectionSetMoveAllMergesCollidingCursors(t *testing.T) {
	buf := SelectionLines{"abc", "abc"}
	s := NewSelectionSet(buf, 1, 1)
	s.AddCursor(1, 2)
	if s.Count() != 2 {
		t.Fatalf("setup Count = %d, want 2", s.Count())
	}
	// Both run into the end of the last line and collapse onto each other.
	s.MoveAll(0, 5, false)
	if s.Count() != 1 {
		t.Fatalf("Count = %d, want 1 after both clamped to the buffer end", s.Count())
	}
	if p := s.Primary(); p.Line != 1 || p.Col != 3 {
		t.Errorf("primary = (%d,%d), want (1,3)", p.Line, p.Col)
	}
}

// --- CollectEdits ---

func TestSelectionSetCollectEditsBottomUpSameLine(t *testing.T) {
	lines := []string{"one two three"}
	s := NewSelectionSet(SelectionLines(lines), 0, 0)
	s.SetPrimary(EditorCursor{Line: 0, Col: 7, AnchorLine: 0, AnchorCol: 4, DesiredCol: 7}) // "two"
	s.AddSelection(0, 0, 0, 3)                                                              // "one"
	if s.Count() != 2 {
		t.Fatalf("Count = %d, want 2", s.Count())
	}

	ce := s.CollectEdits("LONGWORD")
	if len(ce.Edits) != 2 {
		t.Fatalf("edits = %d, want 2", len(ce.Edits))
	}
	if ce.Edits[0].StartCol != 4 || ce.Edits[1].StartCol != 0 {
		t.Fatalf("edits not bottom-up: %+v", ce.Edits)
	}
	if got := joinLines(applySelectionEdits(lines, ce)); got != "LONGWORD LONGWORD three" {
		t.Errorf("applied in order = %q, want %q", got, "LONGWORD LONGWORD three")
	}
	// Proof the order matters: top-down application corrupts the buffer.
	if got := joinLines(applySelectionEdits(lines, reverseCompound(ce))); got == "LONGWORD LONGWORD three" {
		t.Error("top-down application produced the same result; ordering is not being exercised")
	}
}

func TestSelectionSetCollectEditsBottomUpAcrossLines(t *testing.T) {
	lines := []string{"a1", "b2", "c3"}
	s := NewSelectionSet(SelectionLines(lines), 0, 2)
	s.AddCursor(2, 2)

	ce := s.CollectEdits("\n+")
	if len(ce.Edits) != 2 {
		t.Fatalf("edits = %d, want 2", len(ce.Edits))
	}
	if ce.Edits[0].StartLine != 2 || ce.Edits[1].StartLine != 0 {
		t.Fatalf("edits not bottom-up: %+v", ce.Edits)
	}
	if got := joinLines(applySelectionEdits(lines, ce)); got != "a1|+|b2|c3|+" {
		t.Errorf("applied in order = %q, want %q", got, "a1|+|b2|c3|+")
	}
	// Applying top-down shifts line indices under the later edit.
	if got := joinLines(applySelectionEdits(lines, reverseCompound(ce))); got == "a1|+|b2|c3|+" {
		t.Error("top-down application matched; the line-shift hazard is not covered")
	}
}

func TestSelectionSetCollectEditsColumnBlock(t *testing.T) {
	lines := []string{"0123456789", "abc", "0123456789"}
	s := NewSelectionSet(SelectionLines(lines), 0, 0)
	s.AddCursorsForColumnBlock(0, 2, 2, 5)

	ce := s.CollectEdits("#")
	if len(ce.Edits) != 3 {
		t.Fatalf("edits = %d, want 3", len(ce.Edits))
	}
	for i := 1; i < len(ce.Edits); i++ {
		prev, cur := ce.Edits[i-1], ce.Edits[i]
		if selPosCmp(cur.StartLine, cur.StartCol, prev.StartLine, prev.StartCol) >= 0 {
			t.Fatalf("edits not strictly bottom-up at %d: %+v", i, ce.Edits)
		}
	}
	// The short middle line only had cols 2..3 to give up.
	if got := joinLines(applySelectionEdits(lines, ce)); got != "01#56789|ab#|01#56789" {
		t.Errorf("applied = %q, want %q", got, "01#56789|ab#|01#56789")
	}
	if ce.Description != "column replace at 3 cursors" {
		t.Errorf("Description = %q, want %q", ce.Description, "column replace at 3 cursors")
	}
}

func TestSelectionSetCollectEditsDescriptions(t *testing.T) {
	buf := SelectionLines{"0123456789", "0123456789"}

	carets := NewSelectionSet(buf, 0, 1)
	carets.AddCursor(0, 4)
	carets.AddCursor(1, 4)
	if got := carets.CollectEdits("x").Description; got != "insert at 3 cursors" {
		t.Errorf("collapsed carets Description = %q, want %q", got, "insert at 3 cursors")
	}
	for _, e := range carets.CollectEdits("x").Edits {
		if !e.Empty() {
			t.Errorf("collapsed caret produced a non-empty range: %+v", e)
		}
	}

	one := NewSelectionSet(buf, 0, 0)
	one.SetPrimary(EditorCursor{Line: 0, Col: 4, AnchorLine: 0, AnchorCol: 1, DesiredCol: 4})
	if got := one.CollectEdits("").Description; got != "delete at 1 cursor" {
		t.Errorf("single selection Description = %q, want %q", got, "delete at 1 cursor")
	}

	two := NewSelectionSet(buf, 0, 0)
	two.SetPrimary(EditorCursor{Line: 0, Col: 4, AnchorLine: 0, AnchorCol: 1, DesiredCol: 4})
	two.AddSelection(1, 1, 1, 4)
	if got := two.CollectEdits("y").Description; got != "replace at 2 cursors" {
		t.Errorf("two selections Description = %q, want %q", got, "replace at 2 cursors")
	}
}

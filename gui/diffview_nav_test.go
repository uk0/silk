package gui

import (
	"testing"
)

// makeNavView builds a DiffView whose diffRows are set directly to the
// supplied fixture. We skip SetTexts here so the tests pin Next/Prev
// behaviour against a known row layout without depending on the LCS
// pairing rules.
func makeNavView(rows []DiffRow) *DiffView {
	dv := NewDiffView()
	dv.diffRows = rows
	return dv
}

// statusFixture is the canonical mixed-status fixture used by the
// Next/Prev tests: indices 0=Same, 1=Added, 2=Same, 3=Removed, 4=Same,
// 5=Modified, 6=Same. Three change rows: 1, 3, 5.
func statusFixture() []DiffRow {
	return []DiffRow{
		{OldLine: "a", NewLine: "a", Status: DiffSame},
		{NewLine: "b", Status: DiffAdded},
		{OldLine: "c", NewLine: "c", Status: DiffSame},
		{OldLine: "d", Status: DiffRemoved},
		{OldLine: "e", NewLine: "e", Status: DiffSame},
		{OldLine: "f", NewLine: "F", Status: DiffModified},
		{OldLine: "g", NewLine: "g", Status: DiffSame},
	}
}

// TestNextChangeRowFromBeforeStart locks the "from < 0" convention:
// NextChangeRow(-1) searches from row 0 inclusive and returns the very
// first change row. That's the semantics JumpToNextChange leans on for
// a fresh view (activeChangeRow == -1).
func TestDiffViewNextChangeRowFromBeforeStart(t *testing.T) {
	dv := makeNavView(statusFixture())
	if got := dv.NextChangeRow(-1); got != 1 {
		t.Errorf("NextChangeRow(-1) = %d, want 1", got)
	}
	if got := dv.NextChangeRow(-99); got != 1 {
		t.Errorf("NextChangeRow(-99) = %d, want 1", got)
	}
}

// TestNextChangeRowSkipsSameRows confirms NextChangeRow walks past Same
// rows to find the next non-Same row. From row 1 (a change) we should
// land on row 3, not row 2 (which is Same).
func TestDiffViewNextChangeRowSkipsSameRows(t *testing.T) {
	dv := makeNavView(statusFixture())
	if got := dv.NextChangeRow(1); got != 3 {
		t.Errorf("NextChangeRow(1) = %d, want 3", got)
	}
	if got := dv.NextChangeRow(2); got != 3 {
		t.Errorf("NextChangeRow(2) = %d, want 3", got)
	}
	if got := dv.NextChangeRow(3); got != 5 {
		t.Errorf("NextChangeRow(3) = %d, want 5", got)
	}
}

// TestNextChangeRowPastEnd pins the no-wrap-around choice: once we're
// past the last change row, NextChangeRow returns -1 rather than
// cycling back to the first change. The doc comment on NextChangeRow
// makes that explicit; this test guards it from accidental regression.
func TestDiffViewNextChangeRowPastEnd(t *testing.T) {
	dv := makeNavView(statusFixture())
	if got := dv.NextChangeRow(5); got != -1 {
		t.Errorf("NextChangeRow(5) = %d, want -1 (no wrap)", got)
	}
	if got := dv.NextChangeRow(6); got != -1 {
		t.Errorf("NextChangeRow(6) = %d, want -1 (no wrap)", got)
	}
	if got := dv.NextChangeRow(100); got != -1 {
		t.Errorf("NextChangeRow(100) = %d, want -1", got)
	}
}

// TestNextChangeRowEmpty handles the degenerate empty-row case: no
// rows, no changes, NextChangeRow always returns -1 regardless of
// `from`.
func TestDiffViewNextChangeRowEmpty(t *testing.T) {
	dv := makeNavView(nil)
	if got := dv.NextChangeRow(-1); got != -1 {
		t.Errorf("NextChangeRow(-1) on empty = %d, want -1", got)
	}
	if got := dv.NextChangeRow(5); got != -1 {
		t.Errorf("NextChangeRow(5) on empty = %d, want -1", got)
	}
}

// TestPrevChangeRowFromPastEnd is the mirror of NextChangeRowFromBeforeStart:
// PrevChangeRow(len+) searches from the end and returns the last change
// row (5 in the fixture).
func TestDiffViewPrevChangeRowFromPastEnd(t *testing.T) {
	dv := makeNavView(statusFixture())
	rows := len(dv.diffRows)
	if got := dv.PrevChangeRow(rows + 1); got != 5 {
		t.Errorf("PrevChangeRow(len+1) = %d, want 5", got)
	}
	if got := dv.PrevChangeRow(999); got != 5 {
		t.Errorf("PrevChangeRow(999) = %d, want 5", got)
	}
	// `from == len` should also land on the last row: the search starts
	// at len-1 inclusive, so we cover both the "exactly at the end" and
	// the "past the end" cases here.
	if got := dv.PrevChangeRow(rows); got != 5 {
		t.Errorf("PrevChangeRow(len) = %d, want 5", got)
	}
}

// TestPrevChangeRowSkipsSameRows confirms the symmetric walk-backwards
// behaviour: from row 5 (a change) we step to row 3, then row 1.
func TestDiffViewPrevChangeRowSkipsSameRows(t *testing.T) {
	dv := makeNavView(statusFixture())
	if got := dv.PrevChangeRow(5); got != 3 {
		t.Errorf("PrevChangeRow(5) = %d, want 3", got)
	}
	if got := dv.PrevChangeRow(4); got != 3 {
		t.Errorf("PrevChangeRow(4) = %d, want 3", got)
	}
	if got := dv.PrevChangeRow(3); got != 1 {
		t.Errorf("PrevChangeRow(3) = %d, want 1", got)
	}
}

// TestPrevChangeRowBeforeStart pins the no-wrap-around choice on the
// previous-direction side: before row 1 (the first change) there is
// nothing, so PrevChangeRow returns -1 rather than cycling.
func TestDiffViewPrevChangeRowBeforeStart(t *testing.T) {
	dv := makeNavView(statusFixture())
	if got := dv.PrevChangeRow(1); got != -1 {
		t.Errorf("PrevChangeRow(1) = %d, want -1 (no wrap)", got)
	}
	if got := dv.PrevChangeRow(0); got != -1 {
		t.Errorf("PrevChangeRow(0) = %d, want -1", got)
	}
}

// TestSetActiveChangeRowClamping verifies SetActiveChangeRow accepts an
// in-range index, stores it, and treats any out-of-range value as
// "clear" (sets back to -1). ActiveChangeRow reads it back.
func TestDiffViewSetActiveChangeRowClamping(t *testing.T) {
	dv := makeNavView(statusFixture())

	dv.SetActiveChangeRow(3)
	if got := dv.ActiveChangeRow(); got != 3 {
		t.Errorf("ActiveChangeRow after Set(3) = %d, want 3", got)
	}

	dv.SetActiveChangeRow(-1)
	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("ActiveChangeRow after Set(-1) = %d, want -1", got)
	}

	dv.SetActiveChangeRow(99)
	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("ActiveChangeRow after out-of-range Set = %d, want -1", got)
	}
}

// TestJumpToNextChangeWalksChanges drives the public navigation API
// end-to-end: from a fresh view JumpToNextChange visits change rows in
// order, then no-ops once past the last one (no wrap-around).
func TestDiffViewJumpToNextChangeWalksChanges(t *testing.T) {
	dv := NewDiffView()
	// Same / Added / Same / Modified / Same — change rows at indices 1, 3.
	dv.SetTexts("a\nb\nc\nd\ne", "a\nbb\nc\nD\ne")

	rows := dv.DiffRows()
	if len(rows) == 0 {
		t.Fatal("SetTexts produced 0 rows; fixture is broken")
	}

	// Collect the indices of the change rows so the test stays robust
	// against any small adjustment to the LCS pairing rules.
	var changes []int
	for i, r := range rows {
		if r.Status != DiffSame {
			changes = append(changes, i)
		}
	}
	if len(changes) < 2 {
		t.Fatalf("expected >=2 change rows, got %d (rows=%+v)", len(changes), rows)
	}

	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("initial ActiveChangeRow = %d, want -1", got)
	}

	for i, want := range changes {
		dv.JumpToNextChange()
		if got := dv.ActiveChangeRow(); got != want {
			t.Errorf("step %d: ActiveChangeRow = %d, want %d", i, got, want)
		}
	}

	// One more jump past the last change must NOT wrap — activeChangeRow
	// stays pinned at the last change row.
	last := changes[len(changes)-1]
	dv.JumpToNextChange()
	if got := dv.ActiveChangeRow(); got != last {
		t.Errorf("past-end JumpToNextChange moved cursor: got %d, want %d (no wrap)", got, last)
	}
}

// TestJumpToPrevChangeFromFreshLandsOnLast confirms the "fresh view
// presses p" path: with activeChangeRow == -1 we treat `from` as
// past-the-end and land on the last change row.
func TestDiffViewJumpToPrevChangeFromFreshLandsOnLast(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb\nc\nd\ne", "a\nbb\nc\nD\ne")

	rows := dv.DiffRows()
	var lastChange int = -1
	for i, r := range rows {
		if r.Status != DiffSame {
			lastChange = i
		}
	}
	if lastChange < 0 {
		t.Fatal("fixture produced no change rows")
	}

	dv.JumpToPrevChange()
	if got := dv.ActiveChangeRow(); got != lastChange {
		t.Errorf("fresh JumpToPrevChange landed on %d, want %d (last change)", got, lastChange)
	}
}

// TestJumpToPrevChangeNoWrap mirrors the next-direction no-wrap test:
// once the cursor is on the first change row, JumpToPrevChange must
// not cycle back to the last change.
func TestDiffViewJumpToPrevChangeNoWrap(t *testing.T) {
	dv := makeNavView(statusFixture())

	dv.SetActiveChangeRow(1) // first change row
	dv.JumpToPrevChange()
	if got := dv.ActiveChangeRow(); got != 1 {
		t.Errorf("past-start JumpToPrevChange moved cursor: got %d, want 1 (no wrap)", got)
	}
}

// TestNoChangeRowsLeavesCursorAlone covers the "two identical strings"
// edge case: with no change rows, JumpToNextChange / JumpToPrevChange
// must be silent no-ops, leaving activeChangeRow at -1.
func TestDiffViewNoChangeRowsLeavesCursorAlone(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb\nc", "a\nb\nc")
	for _, r := range dv.DiffRows() {
		if r.Status != DiffSame {
			t.Fatalf("expected all-same fixture, got %+v", r)
		}
	}

	dv.JumpToNextChange()
	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("JumpToNextChange on no-change view set cursor to %d, want -1", got)
	}
	dv.JumpToPrevChange()
	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("JumpToPrevChange on no-change view set cursor to %d, want -1", got)
	}
}

// TestOnKeyDownNAndP wires the keyboard surface: 'N' steps next, 'P'
// steps prev. Other keys must be ignored (don't accidentally claim
// printable characters that other widgets might want).
func TestDiffViewOnKeyDownNAndP(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb\nc\nd\ne", "a\nbb\nc\nD\ne")

	rows := dv.DiffRows()
	var changes []int
	for i, r := range rows {
		if r.Status != DiffSame {
			changes = append(changes, i)
		}
	}
	if len(changes) < 2 {
		t.Fatalf("expected >=2 change rows, got %d", len(changes))
	}

	dv.OnKeyDown('N', false)
	if got := dv.ActiveChangeRow(); got != changes[0] {
		t.Errorf("after 'N' from fresh: got %d, want %d", got, changes[0])
	}
	dv.OnKeyDown('N', false)
	if got := dv.ActiveChangeRow(); got != changes[1] {
		t.Errorf("after second 'N': got %d, want %d", got, changes[1])
	}
	dv.OnKeyDown('P', false)
	if got := dv.ActiveChangeRow(); got != changes[0] {
		t.Errorf("after 'P': got %d, want %d", got, changes[0])
	}

	// An unrelated key must not move the cursor.
	before := dv.ActiveChangeRow()
	dv.OnKeyDown('X', false)
	if got := dv.ActiveChangeRow(); got != before {
		t.Errorf("'X' moved cursor from %d to %d, want no-op", before, got)
	}
}

// TestSetTextsResetsActiveRow confirms recompute clears the active
// cursor — otherwise host code that swaps the diff content would leave
// a stale marker pointing at a row that may no longer be a change.
func TestDiffViewSetTextsResetsActiveRow(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb\nc", "a\nx\nc")
	dv.JumpToNextChange()
	if dv.ActiveChangeRow() < 0 {
		t.Fatal("setup: expected cursor on a change row after JumpToNextChange")
	}

	dv.SetTexts("p\nq", "p\nq")
	if got := dv.ActiveChangeRow(); got != -1 {
		t.Errorf("SetTexts left ActiveChangeRow = %d, want -1 (reset)", got)
	}
}

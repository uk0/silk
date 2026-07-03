package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// TestLineDiffEmpty locks the "no input → no rows" base case so the
// downstream Draw loop doesn't accidentally render a phantom blank row
// when both sides are empty.
func TestLineDiffEmpty(t *testing.T) {
	rows := lineDiff(nil, nil)
	if len(rows) != 0 {
		t.Errorf("lineDiff(nil, nil) = %d rows, want 0", len(rows))
	}
	rows = lineDiff([]string{}, []string{})
	if len(rows) != 0 {
		t.Errorf("lineDiff([],[]) = %d rows, want 0", len(rows))
	}
}

// TestLineDiffIdentical confirms two identical line slices produce one
// DiffSame row per line — the diff viewer collapses to a plain
// side-by-side viewer in that case.
func TestLineDiffIdentical(t *testing.T) {
	in := []string{"alpha", "beta", "gamma"}
	rows := lineDiff(in, in)
	if len(rows) != 3 {
		t.Fatalf("lineDiff identical: %d rows, want 3", len(rows))
	}
	for i, r := range rows {
		if r.Status != DiffSame {
			t.Errorf("row %d status = %v, want DiffSame", i, r.Status)
		}
		if r.OldLine != in[i] || r.NewLine != in[i] {
			t.Errorf("row %d = (%q, %q), want (%q, %q)", i, r.OldLine, r.NewLine, in[i], in[i])
		}
	}
}

// TestLineDiffPureAddition covers the "empty → 3 lines" path: every row
// is DiffAdded, the old side is empty, the new side carries the new line.
func TestLineDiffPureAddition(t *testing.T) {
	rows := lineDiff(nil, []string{"a", "b", "c"})
	if len(rows) != 3 {
		t.Fatalf("pure-add: %d rows, want 3", len(rows))
	}
	for i, r := range rows {
		if r.Status != DiffAdded {
			t.Errorf("row %d status = %v, want DiffAdded", i, r.Status)
		}
		if r.OldLine != "" {
			t.Errorf("row %d OldLine = %q, want empty", i, r.OldLine)
		}
		if r.NewLine == "" {
			t.Errorf("row %d NewLine empty, want non-empty", i)
		}
	}
}

// TestLineDiffPureDeletion is the mirror of the addition test: every row
// is DiffRemoved with content on the left and nothing on the right.
func TestLineDiffPureDeletion(t *testing.T) {
	rows := lineDiff([]string{"a", "b", "c"}, nil)
	if len(rows) != 3 {
		t.Fatalf("pure-del: %d rows, want 3", len(rows))
	}
	for i, r := range rows {
		if r.Status != DiffRemoved {
			t.Errorf("row %d status = %v, want DiffRemoved", i, r.Status)
		}
		if r.NewLine != "" {
			t.Errorf("row %d NewLine = %q, want empty", i, r.NewLine)
		}
		if r.OldLine == "" {
			t.Errorf("row %d OldLine empty, want non-empty", i)
		}
	}
}

// TestLineDiffMixedModified pins the load-bearing semantic choice: when a
// single line changes in the middle (old [A B C] vs new [A X C]), we emit
// a modified row B/X rather than a remove-then-add pair. That keeps the
// two columns visually aligned, which is the whole point of a
// side-by-side viewer.
func TestLineDiffMixedModified(t *testing.T) {
	rows := lineDiff([]string{"A", "B", "C"}, []string{"A", "X", "C"})
	if len(rows) != 3 {
		t.Fatalf("mixed: %d rows, want 3", len(rows))
	}
	if rows[0].Status != DiffSame || rows[0].OldLine != "A" || rows[0].NewLine != "A" {
		t.Errorf("row 0 = %+v, want {A, A, Same}", rows[0])
	}
	if rows[1].Status != DiffModified || rows[1].OldLine != "B" || rows[1].NewLine != "X" {
		t.Errorf("row 1 = %+v, want {B, X, Modified}", rows[1])
	}
	if rows[2].Status != DiffSame || rows[2].OldLine != "C" || rows[2].NewLine != "C" {
		t.Errorf("row 2 = %+v, want {C, C, Same}", rows[2])
	}
}

// TestLineDiffInsertionMiddle covers a clean append: old [A B], new
// [A B C] — the first two rows are same, the third is added.
func TestLineDiffInsertionMiddle(t *testing.T) {
	rows := lineDiff([]string{"A", "B"}, []string{"A", "B", "C"})
	if len(rows) != 3 {
		t.Fatalf("insertion: %d rows, want 3", len(rows))
	}
	if rows[0].Status != DiffSame || rows[0].OldLine != "A" {
		t.Errorf("row 0 = %+v, want A=same", rows[0])
	}
	if rows[1].Status != DiffSame || rows[1].OldLine != "B" {
		t.Errorf("row 1 = %+v, want B=same", rows[1])
	}
	if rows[2].Status != DiffAdded || rows[2].NewLine != "C" || rows[2].OldLine != "" {
		t.Errorf("row 2 = %+v, want C=added", rows[2])
	}
}

// TestLineDiffUnevenChangeRun checks the run-pairing fall-off: when an
// unmatched run has more new lines than old (or vice versa), the overlap
// pairs into Modified rows and the tail emits as Added/Removed. Here
// old [A] vs new [X Y Z] has zero matched lines so all three rows come
// out: one Modified (A vs X), two Added (Y, Z).
func TestLineDiffUnevenChangeRun(t *testing.T) {
	rows := lineDiff([]string{"A"}, []string{"X", "Y", "Z"})
	if len(rows) != 3 {
		t.Fatalf("uneven: %d rows, want 3", len(rows))
	}
	if rows[0].Status != DiffModified || rows[0].OldLine != "A" || rows[0].NewLine != "X" {
		t.Errorf("row 0 = %+v, want {A, X, Modified}", rows[0])
	}
	if rows[1].Status != DiffAdded || rows[1].NewLine != "Y" {
		t.Errorf("row 1 = %+v, want {Y, Added}", rows[1])
	}
	if rows[2].Status != DiffAdded || rows[2].NewLine != "Z" {
		t.Errorf("row 2 = %+v, want {Z, Added}", rows[2])
	}
}

// TestDiffViewSetTextsUpdatesState exercises the public API: SetTexts
// must update OldText/NewText and the diff-row count atomically.
func TestDiffViewSetTextsUpdatesState(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb\nc", "a\nx\nc")

	if got := dv.OldText(); got != "a\nb\nc" {
		t.Errorf("OldText = %q, want %q", got, "a\nb\nc")
	}
	if got := dv.NewText(); got != "a\nx\nc" {
		t.Errorf("NewText = %q, want %q", got, "a\nx\nc")
	}
	if got := len(dv.DiffRows()); got != 3 {
		t.Errorf("DiffRows count = %d, want 3", got)
	}
}

// TestDiffViewSetOldTextReDiffs verifies SetOldText alone is enough to
// re-run the diff against the existing right side — important for hosts
// that swap one side at a time (e.g. "compare against HEAD").
func TestDiffViewSetOldTextReDiffs(t *testing.T) {
	dv := NewDiffView()
	dv.SetTexts("a\nb", "a\nb")
	if got := len(dv.DiffRows()); got != 2 {
		t.Fatalf("baseline rows = %d, want 2", got)
	}
	for _, r := range dv.DiffRows() {
		if r.Status != DiffSame {
			t.Fatalf("baseline expected all-same, got %+v", r)
		}
	}

	dv.SetOldText("a\nc")
	rows := dv.DiffRows()
	if len(rows) != 2 {
		t.Fatalf("after SetOldText: %d rows, want 2", len(rows))
	}
	if rows[0].Status != DiffSame {
		t.Errorf("row 0 status = %v, want DiffSame", rows[0].Status)
	}
	if rows[1].Status != DiffModified || rows[1].OldLine != "c" || rows[1].NewLine != "b" {
		t.Errorf("row 1 = %+v, want {c, b, Modified}", rows[1])
	}
}

// TestDiffViewDrawNoPanic is the smoke test: Draw across an empty viewer
// and a populated one with all four row kinds must not panic. We drive a
// nil-safe painter shim since there's no GL surface in a unit test
// (matching the calendar_test.go nopPainter pattern).
func TestDiffViewDrawNoPanic(t *testing.T) {
	dv := NewDiffView()
	dv.SetSize(480, 240)

	rec := diffViewNopPainter{}
	dv.Draw(rec) // empty case

	dv.SetTexts("A\nB\nC\nD", "A\nX\nD\nE")
	dv.Draw(rec) // populated case
}

// diffViewNopPainter satisfies paint.Painter with no-op stubs (embeds a
// nil Painter) so Draw can run without a render target. Only the methods
// DiffView.Draw actually calls need bodies; the embedded nil supplies the
// rest, which Draw never reaches.
type diffViewNopPainter struct{ paint.Painter }

func (diffViewNopPainter) Rectangle(x, y, w, h float64)         {}
func (diffViewNopPainter) MoveTo(x, y float64)                  {}
func (diffViewNopPainter) LineTo(x, y float64)                  {}
func (diffViewNopPainter) Fill()                                {}
func (diffViewNopPainter) Stroke()                              {}
func (diffViewNopPainter) SetBrush1(c paint.Color)              {}
func (diffViewNopPainter) SetPen1(c paint.Color, width float64) {}
func (diffViewNopPainter) SetFont(f paint.Font)                 {}
func (diffViewNopPainter) DrawText1(x, y float64, text string)  {}

package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// TestGutterLineNumbers exercises the pure helper that maps a DiffRow
// slice to per-side line-number columns. The load-bearing invariant is:
// a side's counter advances only when that row presents content on that
// side (Same/Modified bump both, Added bumps only right, Removed bumps
// only left). A 0 in the output means "no number on this side".
func TestGutterLineNumbers(t *testing.T) {
	cases := []struct {
		name      string
		rows      []DiffRow
		wantLeft  []int
		wantRight []int
	}{
		{
			name: "all same",
			rows: []DiffRow{
				{Status: DiffSame},
				{Status: DiffSame},
				{Status: DiffSame},
			},
			wantLeft:  []int{1, 2, 3},
			wantRight: []int{1, 2, 3},
		},
		{
			name: "add at end",
			rows: []DiffRow{
				{Status: DiffSame},
				{Status: DiffSame},
				{Status: DiffAdded},
			},
			wantLeft:  []int{1, 2, 0},
			wantRight: []int{1, 2, 3},
		},
		{
			name: "remove at start",
			rows: []DiffRow{
				{Status: DiffRemoved},
				{Status: DiffSame},
			},
			wantLeft:  []int{1, 2},
			wantRight: []int{0, 1},
		},
		{
			name: "modified row",
			rows: []DiffRow{
				{Status: DiffSame},
				{Status: DiffModified},
				{Status: DiffSame},
			},
			wantLeft:  []int{1, 2, 3},
			wantRight: []int{1, 2, 3},
		},
		{
			name: "all added",
			rows: []DiffRow{
				{Status: DiffAdded},
				{Status: DiffAdded},
				{Status: DiffAdded},
				{Status: DiffAdded},
			},
			wantLeft:  []int{0, 0, 0, 0},
			wantRight: []int{1, 2, 3, 4},
		},
		{
			name: "all removed",
			rows: []DiffRow{
				{Status: DiffRemoved},
				{Status: DiffRemoved},
			},
			wantLeft:  []int{1, 2},
			wantRight: []int{0, 0},
		},
		{
			name: "mixed status fixture",
			// Same, Added, Same, Removed, Same, Modified, Same.
			// Left counter advances on Same/Removed/Modified.
			// Right counter advances on Same/Added/Modified.
			rows: []DiffRow{
				{Status: DiffSame},
				{Status: DiffAdded},
				{Status: DiffSame},
				{Status: DiffRemoved},
				{Status: DiffSame},
				{Status: DiffModified},
				{Status: DiffSame},
			},
			wantLeft:  []int{1, 0, 2, 3, 4, 5, 6},
			wantRight: []int{1, 2, 3, 0, 4, 5, 6},
		},
		{
			name:      "empty",
			rows:      nil,
			wantLeft:  []int{},
			wantRight: []int{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			left, right := gutterLineNumbers(c.rows)
			if !equalIntSlice(left, c.wantLeft) {
				t.Errorf("left = %v, want %v", left, c.wantLeft)
			}
			if !equalIntSlice(right, c.wantRight) {
				t.Errorf("right = %v, want %v", right, c.wantRight)
			}
		})
	}
}

// equalIntSlice treats nil and []int{} as equal; the helper allocates a
// zero-length slice for empty input so the test fixtures stay readable.
func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDiffViewShowGutterDefault confirms the gutter is on by default — a
// fresh DiffView renders line numbers without the host opting in.
func TestDiffViewShowGutterDefault(t *testing.T) {
	dv := NewDiffView()
	if !dv.ShowGutter() {
		t.Errorf("ShowGutter() = false, want true (default)")
	}
}

// TestDiffViewSetShowGutterToggles exercises the public toggle: flipping
// it off then back on lands the widget back at the default state.
func TestDiffViewSetShowGutterToggles(t *testing.T) {
	dv := NewDiffView()
	dv.SetShowGutter(false)
	if dv.ShowGutter() {
		t.Errorf("after SetShowGutter(false): ShowGutter() = true, want false")
	}
	dv.SetShowGutter(true)
	if !dv.ShowGutter() {
		t.Errorf("after SetShowGutter(true): ShowGutter() = false, want true")
	}
}

// TestDiffViewDrawGutterNoPanic is the gutter equivalent of
// diffview_test.go's TestDiffViewDrawNoPanic: paint a populated viewer
// with the gutter on, then again with it off, and confirm neither path
// panics. Reuses the same nil-safe painter shim.
func TestDiffViewDrawGutterNoPanic(t *testing.T) {
	dv := NewDiffView()
	dv.SetSize(480, 240)
	dv.SetTexts("A\nB\nC\nD", "A\nX\nD\nE")

	rec := gutterNopPainter{}
	dv.Draw(rec) // gutter on (default)

	dv.SetShowGutter(false)
	dv.Draw(rec) // gutter off
}

// gutterNopPainter mirrors diffview_test.go's diffViewNopPainter — a
// no-op Painter shim so Draw can run without a render target. Kept
// local to this file so the gutter test stays self-contained.
type gutterNopPainter struct{ paint.Painter }

func (gutterNopPainter) Rectangle(x, y, w, h float64)         {}
func (gutterNopPainter) MoveTo(x, y float64)                  {}
func (gutterNopPainter) LineTo(x, y float64)                  {}
func (gutterNopPainter) Fill()                                {}
func (gutterNopPainter) Stroke()                              {}
func (gutterNopPainter) SetBrush1(c paint.Color)              {}
func (gutterNopPainter) SetPen1(c paint.Color, width float64) {}
func (gutterNopPainter) SetFont(f paint.Font)                 {}
func (gutterNopPainter) DrawText1(x, y float64, text string)  {}

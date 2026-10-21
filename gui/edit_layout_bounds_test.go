package gui

import (
	"math"
	"strings"
	"testing"
)

func TestEditLayoutClampsNarrowWidthAndScrollRange(t *testing.T) {
	e := NewEdit()
	e.SetMultiLine(true)
	e.SetWrap(true)
	for _, size := range [][2]float64{{620, 280}, {1, 1}, {8, 180}, {320, 400}} {
		e.SetSize(size[0], size[1])
		for _, text := range []string{strings.Repeat("long line\n", 100), "short", ""} {
			e.SetText(text)
			e.Layout()
			if e.TextBlock.pw < 1 {
				t.Errorf("size %v: negative/narrow layout width %g", size, e.TextBlock.pw)
			}
			lo, hi := e.VertScrollBar().Range()
			if math.IsNaN(hi) || math.IsInf(hi, 0) || lo != 0 || hi < 0 {
				t.Fatalf("invalid range %g..%g", lo, hi)
			}
			e.VertScrollBar().SetValue(1e9)
			if v := e.VertScrollBar().Value(); v < lo || v > hi {
				t.Fatal("scroll value escaped bounds", v)
			}
		}
	}
	if _, hi := e.VertScrollBar().Range(); hi != 0 {
		t.Fatal("empty message kept stale scrolling", hi)
	}
}

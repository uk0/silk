package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestStatsPanelSetStatsStoresAndCopies verifies SetStats keeps the rows in
// order and defensively copies on both the input and output boundaries: mutating
// the caller's slice after SetStats, or the slice returned by Stats(), must not
// disturb the panel's stored state.
func TestStatsPanelSetStatsStoresAndCopies(t *testing.T) {
	in := []StatRow{
		{Tag: "TIC-101", Count: 3, Min: 1.5, Max: 9.5, Avg: 5.0, Last: 7.25},
		{Tag: "LIC-200", Count: 8, Min: -2.0, Max: 4.0, Avg: 1.0, Last: 0.5},
	}
	p := NewStatsPanel()
	p.SetStats(in)

	// Mutating the caller's slice must not reach the panel (input copied).
	in[0].Tag = "MUTATED"
	in[1].Count = 999
	got := p.Stats()
	if len(got) != 2 {
		t.Fatalf("Stats() len = %d, want 2", len(got))
	}
	if got[0].Tag != "TIC-101" || got[1].Tag != "LIC-200" {
		t.Fatalf("order/copy wrong after input mutation: %+v", got)
	}
	if got[0].Count != 3 || got[1].Count != 8 {
		t.Fatalf("count not stored verbatim: %+v", got)
	}
	if got[0].Min != 1.5 || got[0].Max != 9.5 || got[0].Avg != 5.0 || got[0].Last != 7.25 {
		t.Fatalf("float fields not stored verbatim: %+v", got[0])
	}

	// Mutating the returned slice must not reach the panel (output copied).
	got[1].Tag = "MUTATED2"
	again := p.Stats()
	if again[1].Tag != "LIC-200" {
		t.Fatalf("Stats() did not return a copy: %+v", again)
	}
}

// TestStatsPanelRowCount checks RowCount tracks the stored row slice length.
func TestStatsPanelRowCount(t *testing.T) {
	p := NewStatsPanel()
	if got := p.RowCount(); got != 0 {
		t.Fatalf("RowCount() on empty = %d, want 0", got)
	}
	p.SetStats([]StatRow{
		{Tag: "A"}, {Tag: "B"}, {Tag: "C"},
	})
	if got := p.RowCount(); got != 3 {
		t.Fatalf("RowCount() = %d, want 3", got)
	}
}

// TestFmtStat pins the two-decimal float formatting helper, including the
// rounding case from the spec and the sign/zero edges.
func TestFmtStat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.239, "1.24"},
		{0, "0.00"},
		{5, "5.00"},
		{-2.5, "-2.50"},
		{9.999, "10.00"},
	}
	for _, c := range cases {
		if got := fmtStat(c.in); got != c.want {
			t.Errorf("fmtStat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStatsPanelRowAtY checks the pure two-band-header/scroll-aware hit-test: the
// header region maps to -1, the first pixel below it to row 0, and a scroll
// offset shifts the mapping by whole rows.
func TestStatsPanelRowAtY(t *testing.T) {
	p := NewStatsPanel()
	rh := p.rowHeight

	if got := p.rowAtY(statsHeaderTotal - 1); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	if got := p.rowAtY(statsHeaderTotal + 1); got != 0 {
		t.Errorf("rowAtY(first row) = %d, want 0", got)
	}
	if got := p.rowAtY(statsHeaderTotal + rh + 1); got != 1 {
		t.Errorf("rowAtY(second row) = %d, want 1", got)
	}

	p.scrollY = 2 * rh
	if got := p.rowAtY(statsHeaderTotal + 1); got != 2 {
		t.Errorf("rowAtY(first row, scrolled 2) = %d, want 2", got)
	}
}

// TestStatsPanelResetColumnHit pins the pure width-relative reset-column test.
func TestStatsPanelResetColumnHit(t *testing.T) {
	const w = 400.0
	if !statsResetColumnHit(w-8, w) {
		t.Errorf("click near right edge should hit the reset column")
	}
	if statsResetColumnHit(60, w) {
		t.Errorf("click in the tag area should not hit the reset column")
	}
	// Exact boundary: x == w-statsResetColW is the first column pixel.
	if !statsResetColumnHit(w-statsResetColW, w) {
		t.Errorf("left boundary of reset column should hit")
	}
	if statsResetColumnHit(w-statsResetColW-1, w) {
		t.Errorf("one pixel left of the reset column should miss")
	}
}

// TestStatsPanelResetClickFiresSig confirms a click in a row's 清零 column fires
// SigReset with that row's tag, while a click elsewhere on the row does not.
func TestStatsPanelResetClickFiresSig(t *testing.T) {
	p := NewStatsPanel()
	p.SetStats([]StatRow{
		{Tag: "TIC-101", Count: 3, Avg: 5.0},
		{Tag: "LIC-200", Count: 8, Avg: 1.0},
	})
	p.SetBounds(0, 0, 400, 200) // give the panel a width so the reset column exists

	var gotTag string
	fired := false
	p.SigReset(func(tag string) { fired = true; gotTag = tag })

	// Click inside the right-anchored 清零 column on row 1.
	p.OnLeftDown(400-8, statsHeaderTotal+p.rowHeight*1.5)
	if !fired {
		t.Fatal("reset-column click did not fire SigReset")
	}
	if gotTag != "LIC-200" {
		t.Fatalf("SigReset tag = %q, want LIC-200", gotTag)
	}

	// A click on the row body (outside the reset column) must not fire.
	fired = false
	p.OnLeftDown(statsColTag+4, statsHeaderTotal+p.rowHeight*0.5)
	if fired {
		t.Fatal("non-reset row click fired SigReset")
	}

	// A click on the header region must not fire.
	fired = false
	p.OnLeftDown(400-8, statsHeaderTotal-2)
	if fired {
		t.Fatal("header click fired SigReset")
	}
}

// TestStatsPanelFactoryRegistered checks the factory id resolves to a
// constructible *StatsPanel so the designer can place it.
func TestStatsPanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.StatsPanel")
	if _, ok := obj.(*StatsPanel); !ok {
		t.Fatalf("factory gui.StatsPanel built %T, want *StatsPanel", obj)
	}
}

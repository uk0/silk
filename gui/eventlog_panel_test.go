package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// sampleEvents is a mixed-kind fixture used across the filter/scroll tests.
func sampleEvents() []EventRow {
	return []EventRow{
		{Time: "15:04:01", Kind: "alarm", Source: "TIC-101", Message: "HiHi trip"},
		{Time: "15:04:02", Kind: "login", Source: "operator1", Message: "signed in"},
		{Time: "15:04:03", Kind: "write", Source: "FIC-200.SP", Message: "50 -> 60"},
		{Time: "15:04:04", Kind: "alarm", Source: "LIC-300", Message: "Lo warning"},
		{Time: "15:04:05", Kind: "system", Source: "runtime", Message: "scan restarted"},
	}
}

// TestEventLogPanelSetEventsStoresAndCopies verifies SetEvents keeps rows in
// order and defensively copies on both boundaries: mutating the caller's slice
// after SetEvents, or the slice returned by Events(), must not disturb state.
func TestEventLogPanelSetEventsStoresAndCopies(t *testing.T) {
	in := sampleEvents()
	p := NewEventLogPanel()
	p.SetEvents(in)

	// Mutating the caller's slice must not reach the panel (input copied).
	in[0].Source = "MUTATED"
	got := p.Events()
	if len(got) != 5 {
		t.Fatalf("Events() len = %d, want 5", len(got))
	}
	if got[0].Source != "TIC-101" || got[2].Message != "50 -> 60" {
		t.Fatalf("order/copy wrong after input mutation: %+v", got)
	}

	// Mutating the returned slice must not reach the panel (output copied).
	got[1].Source = "MUTATED2"
	again := p.Events()
	if again[1].Source != "operator1" {
		t.Fatalf("Events() did not return a copy: %+v", again)
	}
}

// TestEventLogPanelKindFilter checks SetKindFilter narrows the visible set to
// the matching Kind, and that the empty filter restores every row.
func TestEventLogPanelKindFilter(t *testing.T) {
	p := NewEventLogPanel()
	p.SetEvents(sampleEvents())

	if n := len(p.visibleRows()); n != 5 {
		t.Fatalf("no filter: visible = %d, want 5", n)
	}

	p.SetKindFilter("alarm")
	vis := p.visibleRows()
	if len(vis) != 2 {
		t.Fatalf("kind=alarm: visible = %d, want 2", len(vis))
	}
	for _, e := range vis {
		if e.Kind != "alarm" {
			t.Fatalf("kind=alarm leaked a %q row: %+v", e.Kind, e)
		}
	}
	if vis[0].Source != "TIC-101" || vis[1].Source != "LIC-300" {
		t.Fatalf("kind=alarm content/order wrong: %+v", vis)
	}

	p.SetKindFilter("system")
	if vis := p.visibleRows(); len(vis) != 1 || vis[0].Source != "runtime" {
		t.Fatalf("kind=system: %+v, want single runtime row", vis)
	}

	p.SetKindFilter("")
	if n := len(p.visibleRows()); n != 5 {
		t.Fatalf("cleared filter: visible = %d, want 5", n)
	}
}

// TestEventLogPanelToggleClickFiresSig confirms a click on a kind tab fires
// SigFilter with that tab's mapped kind ("all" -> ""), while a click on a row
// (below the filter bar) does not fire.
func TestEventLogPanelToggleClickFiresSig(t *testing.T) {
	p := NewEventLogPanel()
	p.SetEvents(sampleEvents())
	p.SetBounds(0, 0, 400, 300)

	var gotKind string
	fired := false
	p.SigFilter(func(kind string) { fired = true; gotKind = kind })

	// Mid-band y inside the filter bar.
	yBar := eventHeaderH + eventFilterH*0.5

	// "alarm" is tab index 1: click inside its cell.
	xAlarm := eventToggleX0 + eventToggleW*1.5
	p.OnLeftDown(xAlarm, yBar)
	if !fired {
		t.Fatal("kind-tab click did not fire SigFilter")
	}
	if gotKind != "alarm" {
		t.Fatalf("SigFilter kind = %q, want alarm", gotKind)
	}

	// "all" is tab index 0 and maps to the empty filter.
	fired, gotKind = false, "unset"
	xAll := eventToggleX0 + eventToggleW*0.5
	p.OnLeftDown(xAll, yBar)
	if !fired {
		t.Fatal("all-tab click did not fire SigFilter")
	}
	if gotKind != "" {
		t.Fatalf("all-tab SigFilter kind = %q, want empty", gotKind)
	}

	// A click on a row (below the filter bar) must not fire SigFilter.
	fired = false
	p.OnLeftDown(xAlarm, eventHeaderH+eventFilterH+p.rowHeight*0.5)
	if fired {
		t.Fatal("row click fired SigFilter")
	}
}

// TestEventLogPanelToggleAtX pins the pure filter-bar hit geometry, including
// the out-of-range guards on either side.
func TestEventLogPanelToggleAtX(t *testing.T) {
	if got := eventToggleAtX(eventToggleX0 - 1); got != -1 {
		t.Errorf("left of first tab = %d, want -1", got)
	}
	if got := eventToggleAtX(eventToggleX0 + eventToggleW*0.5); got != 0 {
		t.Errorf("first tab = %d, want 0", got)
	}
	if got := eventToggleAtX(eventToggleX0 + eventToggleW*2.5); got != 2 {
		t.Errorf("third tab = %d, want 2", got)
	}
	past := eventToggleX0 + eventToggleW*float64(len(eventKindTabs))
	if got := eventToggleAtX(past + 1); got != -1 {
		t.Errorf("past last tab = %d, want -1", got)
	}
}

// TestEventLogPanelRowAtY checks the pure header/filter/scroll-aware hit-test:
// both top bands map to -1, the first pixel below them to row 0, and a scroll
// offset shifts the mapping by whole rows.
func TestEventLogPanelRowAtY(t *testing.T) {
	p := NewEventLogPanel()
	rh := p.rowHeight
	top := eventHeaderH + eventFilterH

	if got := p.rowAtY(eventHeaderH - 1); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	if got := p.rowAtY(top - 1); got != -1 {
		t.Errorf("rowAtY(filter bar) = %d, want -1", got)
	}
	if got := p.rowAtY(top + 1); got != 0 {
		t.Errorf("rowAtY(first row) = %d, want 0", got)
	}
	if got := p.rowAtY(top + rh + 1); got != 1 {
		t.Errorf("rowAtY(second row) = %d, want 1", got)
	}

	p.scrollY = 2 * rh
	if got := p.rowAtY(top + 1); got != 2 {
		t.Errorf("rowAtY(first row, scrolled 2) = %d, want 2", got)
	}
}

// TestEventLogPanelScrollClamp verifies clampScroll pins the offset within
// [0, maxScroll] for the current filtered content and viewport height.
func TestEventLogPanelScrollClamp(t *testing.T) {
	p := NewEventLogPanel()
	rows := make([]EventRow, 20)
	for i := range rows {
		rows[i] = EventRow{Time: "t", Kind: "system", Source: "s", Message: "m"}
	}
	p.SetEvents(rows)
	p.SetBounds(0, 0, 300, 120)

	_, h := p.Size()
	want := float64(len(p.visibleRows()))*p.rowHeight - (h - eventHeaderH - eventFilterH)
	if want <= 0 {
		t.Fatalf("test setup: expected positive maxScroll, got %v", want)
	}

	p.scrollY = 99999
	p.clampScroll()
	if p.scrollY != want {
		t.Errorf("over-scroll clamped to %v, want %v", p.scrollY, want)
	}

	p.scrollY = -50
	p.clampScroll()
	if p.scrollY != 0 {
		t.Errorf("under-scroll clamped to %v, want 0", p.scrollY)
	}
}

// TestEventLogPanelFactoryRegistered checks the factory id resolves to a
// constructible *EventLogPanel so the designer can place it.
func TestEventLogPanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.EventLogPanel")
	if _, ok := obj.(*EventLogPanel); !ok {
		t.Fatalf("factory gui.EventLogPanel built %T, want *EventLogPanel", obj)
	}
}

package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestReportViewSetTableStoresAndCopies verifies SetTable keeps headers/rows in
// order and defensively deep-copies on both boundaries: mutating the caller's
// headers, the caller's inner row slice, or the slice returned by Headers(), must
// not disturb the panel's stored state.
func TestReportViewSetTableStoresAndCopies(t *testing.T) {
	headers := []string{"设备", "数值"}
	rows := [][]string{
		{"TIC-101", "95.0"},
		{"LIC-200", "12.3"},
	}
	p := NewReportView()
	p.SetTable(headers, rows)

	if got := p.Headers(); len(got) != 2 || got[0] != "设备" || got[1] != "数值" {
		t.Fatalf("Headers() = %v, want [设备 数值]", got)
	}

	// Mutating the caller's header slice must not reach the panel.
	headers[0] = "MUTATED"
	if got := p.Headers(); got[0] != "设备" {
		t.Fatalf("header not copied on input: got %q", got[0])
	}

	// Mutating a caller inner row slice must not reach the panel (deep copy).
	rows[0][0] = "MUTATED"
	if p.rows[0][0] != "TIC-101" {
		t.Fatalf("row cell not deep-copied on input: got %q", p.rows[0][0])
	}
	// Replacing the outer slice entry must not reach the panel either.
	rows[1] = nil
	if len(p.rows) != 2 || p.rows[1][0] != "LIC-200" {
		t.Fatalf("outer row slice not copied on input: %v", p.rows)
	}

	// Mutating the slice returned by Headers() must not reach the panel.
	out := p.Headers()
	out[1] = "MUTATED2"
	if again := p.Headers(); again[1] != "数值" {
		t.Fatalf("Headers() did not return a copy: %v", again)
	}
}

// TestReportViewRowCount checks RowCount reflects the body row count (headers
// excluded) and updates on re-set.
func TestReportViewRowCount(t *testing.T) {
	p := NewReportView()
	if p.RowCount() != 0 {
		t.Fatalf("empty RowCount() = %d, want 0", p.RowCount())
	}
	p.SetTable([]string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}, {"5", "6"}})
	if p.RowCount() != 3 {
		t.Fatalf("RowCount() = %d, want 3", p.RowCount())
	}
	p.SetTable([]string{"a"}, [][]string{{"1"}})
	if p.RowCount() != 1 {
		t.Fatalf("RowCount() after re-set = %d, want 1", p.RowCount())
	}
}

// TestReportViewExportButtonHitMath pins the pure toolbar hit-test: the CSV
// button (left) maps to "csv", the HTML button (right) to "html", and clicks
// outside the button band map to "".
func TestReportViewExportButtonHitMath(t *testing.T) {
	const w = 320.0
	htmlX1 := w - reportBtnPad - reportBtnW
	csvX2 := htmlX1 - reportBtnSpacing
	csvX1 := csvX2 - reportBtnW
	bandY := (reportToolbarH-reportBtnH)/2 + reportBtnH/2

	csvCenter := (csvX1 + csvX2) / 2
	htmlCenter := (htmlX1 + (w - reportBtnPad)) / 2

	if got := exportButtonAt(csvCenter, bandY, w); got != "csv" {
		t.Errorf("exportButtonAt(csv center) = %q, want csv", got)
	}
	if got := exportButtonAt(htmlCenter, bandY, w); got != "html" {
		t.Errorf("exportButtonAt(html center) = %q, want html", got)
	}
	// Left of the CSV button (over the title area) is a miss.
	if got := exportButtonAt(10, bandY, w); got != "" {
		t.Errorf("exportButtonAt(title area) = %q, want empty", got)
	}
	// Right x but above the button band (y=0) is a miss.
	if got := exportButtonAt(csvCenter, 0, w); got != "" {
		t.Errorf("exportButtonAt(above band) = %q, want empty", got)
	}
}

// TestReportViewExportClickFiresSig confirms a click on the CSV button fires
// SigExport("csv"), a click on the HTML button fires SigExport("html"), and a
// click in the data body fires nothing.
func TestReportViewExportClickFiresSig(t *testing.T) {
	const w = 320.0
	p := NewReportView()
	p.SetBounds(0, 0, w, 200)
	p.SetTable([]string{"a"}, [][]string{{"1"}})

	var got string
	fired := false
	p.SigExport(func(format string) { fired = true; got = format })

	htmlX1 := w - reportBtnPad - reportBtnW
	csvX2 := htmlX1 - reportBtnSpacing
	csvCenter := (csvX2 - reportBtnW + csvX2) / 2
	htmlCenter := (htmlX1 + (w - reportBtnPad)) / 2
	bandY := (reportToolbarH-reportBtnH)/2 + reportBtnH/2

	p.OnLeftDown(csvCenter, bandY)
	if !fired || got != "csv" {
		t.Fatalf("CSV click: fired=%v format=%q, want true/csv", fired, got)
	}

	fired, got = false, ""
	p.OnLeftDown(htmlCenter, bandY)
	if !fired || got != "html" {
		t.Fatalf("HTML click: fired=%v format=%q, want true/html", fired, got)
	}

	// A click in the data body must not fire SigExport.
	fired = false
	p.OnLeftDown(20, reportDataTop()+p.rowHeight*0.5)
	if fired {
		t.Fatal("data-body click fired SigExport")
	}
}

// TestReportViewRowAtY checks the pure band/scroll-aware hit-test: the toolbar +
// header bands map to -1, the first pixel below them to row 0, and a scroll
// offset shifts the mapping by whole rows.
func TestReportViewRowAtY(t *testing.T) {
	p := NewReportView()
	rh := p.rowHeight
	top := reportDataTop()

	if got := p.rowAtY(top - 1); got != -1 {
		t.Errorf("rowAtY(header band) = %d, want -1", got)
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

// TestReportViewScrollClamp checks clampScroll pins scrollY into [0, maxScroll]
// for the current content height and viewport.
func TestReportViewScrollClamp(t *testing.T) {
	p := NewReportView()
	p.SetBounds(0, 0, 300, 200)
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"x"}
	}
	p.SetTable([]string{"h"}, rows)

	maxScroll := float64(20)*p.rowHeight - (200 - reportDataTop())

	p.scrollY = 100000
	p.clampScroll()
	if p.scrollY != maxScroll {
		t.Errorf("over-scroll clamped to %v, want %v", p.scrollY, maxScroll)
	}

	p.scrollY = -50
	p.clampScroll()
	if p.scrollY != 0 {
		t.Errorf("under-scroll clamped to %v, want 0", p.scrollY)
	}
}

// TestReportViewFactoryRegistered checks the factory id resolves to a
// constructible *ReportView so the designer can place it.
func TestReportViewFactoryRegistered(t *testing.T) {
	obj := core.New("gui.ReportView")
	if _, ok := obj.(*ReportView); !ok {
		t.Fatalf("factory gui.ReportView built %T, want *ReportView", obj)
	}
}

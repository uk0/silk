package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// sampleFormulas is a mixed-status fixture used across the CalcPanel tests: a
// clean "ok" row, an error row, and a blank-status row.
func sampleFormulas() []FormulaRow {
	return []FormulaRow{
		{Output: "T1", Expr: "A + B", Status: "ok"},
		{Output: "T2", Expr: "C * D", Status: "div by zero"},
		{Output: "T3", Expr: "E - F", Status: ""},
	}
}

// TestCalcPanelSetFormulasStoresAndCopies verifies SetFormulas keeps the rows in
// order and defensively copies on both boundaries: mutating the caller's slice
// after SetFormulas, or the slice returned by Formulas(), must not disturb the
// panel's stored state. A fresh SetFormulas also clears any prior selection.
func TestCalcPanelSetFormulasStoresAndCopies(t *testing.T) {
	in := sampleFormulas()
	p := NewCalcPanel()
	p.SetFormulas(in)

	// Mutating the caller's slice must not reach the panel (input copied).
	in[0].Output = "MUTATED"
	got := p.Formulas()
	if len(got) != 3 {
		t.Fatalf("Formulas() len = %d, want 3", len(got))
	}
	if got[0].Output != "T1" || got[1].Expr != "C * D" {
		t.Fatalf("order/copy wrong after input mutation: %+v", got)
	}

	// Mutating the returned slice must not reach the panel (output copied).
	got[1].Output = "MUTATED2"
	again := p.Formulas()
	if again[1].Output != "T2" {
		t.Fatalf("Formulas() did not return a copy: %+v", again)
	}

	// A fresh SetFormulas clears any prior selection.
	p.selected = 2
	p.SetFormulas([]FormulaRow{{Output: "X"}})
	if p.Selected() != -1 {
		t.Fatalf("SetFormulas did not reset selection: %d", p.Selected())
	}
}

// TestCalcPanelRowAtY checks the pure header/scroll-aware hit-test: the header
// band maps to -1, the first pixel below it to row 0, and a scroll offset shifts
// the mapping by whole rows.
func TestCalcPanelRowAtY(t *testing.T) {
	p := NewCalcPanel()
	rh := p.rowHeight

	if got := p.rowAtY(calcHeaderH - 1); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	if got := p.rowAtY(calcHeaderH + 1); got != 0 {
		t.Errorf("rowAtY(first row) = %d, want 0", got)
	}
	if got := p.rowAtY(calcHeaderH + rh + 1); got != 1 {
		t.Errorf("rowAtY(second row) = %d, want 1", got)
	}

	p.scrollY = 2 * rh
	if got := p.rowAtY(calcHeaderH + 1); got != 2 {
		t.Errorf("rowAtY(first row, scrolled 2) = %d, want 2", got)
	}
}

// TestCalcPanelButtonAtX pins the pure footer button hit-test: the row splits
// into two equal cells tiling [0,w), and x outside the panel (or a zero width)
// maps to -1.
func TestCalcPanelButtonAtX(t *testing.T) {
	const w = 400.0 // 2 cells of 200px each
	cases := []struct {
		x    float64
		want int
	}{
		{-1, -1},
		{0, calcBtnAdd},
		{100, calcBtnAdd},
		{200, calcBtnRemove},
		{399.9, calcBtnRemove},
		{400, -1}, // x == w is outside
	}
	for _, c := range cases {
		if got := calcButtonAtX(c.x, w); got != c.want {
			t.Errorf("calcButtonAtX(%v, %v) = %d, want %d", c.x, w, got, c.want)
		}
	}
	if got := calcButtonAtX(10, 0); got != -1 {
		t.Errorf("calcButtonAtX with zero width = %d, want -1", got)
	}
}

// TestCalcPanelStatusWarn pins the pure warning-tint rule: any non-empty status
// other than "ok" burns the warning colour.
func TestCalcPanelStatusWarn(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"", false},
		{"ok", false},
		{"err", true},
		{"div by zero", true},
	}
	for _, c := range cases {
		if got := calcStatusWarn(c.status); got != c.want {
			t.Errorf("calcStatusWarn(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

// TestCalcPanelRowClickSelects confirms a click in the list body selects that
// row (by index), while a click on the header or past the last row leaves the
// selection unchanged.
func TestCalcPanelRowClickSelects(t *testing.T) {
	p := NewCalcPanel()
	p.SetFormulas(sampleFormulas())
	p.SetBounds(0, 0, 300, 200)
	rh := p.rowHeight

	if p.Selected() != -1 {
		t.Fatalf("fresh panel Selected() = %d, want -1", p.Selected())
	}

	// Header click selects nothing.
	p.OnLeftDown(10, calcHeaderH-2)
	if p.Selected() != -1 {
		t.Fatalf("header click selected %d, want -1", p.Selected())
	}

	// Row 0, then row 2.
	p.OnLeftDown(10, calcHeaderH+1)
	if p.Selected() != 0 {
		t.Fatalf("row-0 click Selected() = %d, want 0", p.Selected())
	}
	p.OnLeftDown(10, calcHeaderH+2*rh+1)
	if p.Selected() != 2 {
		t.Fatalf("row-2 click Selected() = %d, want 2", p.Selected())
	}

	// A click in the list body past the last row is ignored (selection frozen).
	p.OnLeftDown(10, calcHeaderH+6*rh)
	if p.Selected() != 2 {
		t.Fatalf("past-last-row click changed selection to %d, want frozen 2", p.Selected())
	}
}

// TestCalcPanelActionButtonsFireSig checks the footer routing: 新增(Add) fires
// unconditionally with empty output/expr, and 删除(Remove) fires SigRemove with
// the selected row's Output — but not while nothing is selected.
func TestCalcPanelActionButtonsFireSig(t *testing.T) {
	p := NewCalcPanel()
	p.SetFormulas(sampleFormulas())
	p.SetBounds(0, 0, 300, 200) // 2 footer cells of 150px; footer y >= 172
	rh := p.rowHeight

	const footerY = 190.0

	// Add fires unconditionally, passing empty strings (host fills them in).
	var addOut, addExpr string
	addFired := false
	p.SigAdd(func(o, e string) { addFired = true; addOut = o; addExpr = e })
	p.OnLeftDown(50, footerY) // Add cell [0,150)
	if !addFired || addOut != "" || addExpr != "" {
		t.Fatalf("Add fired=%v out=%q expr=%q, want true empty empty", addFired, addOut, addExpr)
	}

	// Remove with no selection must not fire.
	removeFired := false
	var removeOut string
	p.SigRemove(func(o string) { removeFired = true; removeOut = o })
	p.OnLeftDown(200, footerY) // Remove cell [150,300)
	if removeFired {
		t.Fatalf("Remove fired with no selection (out=%q)", removeOut)
	}

	// Select row 1 ("T2"), then Remove carries that Output.
	p.OnLeftDown(10, calcHeaderH+rh*1.5)
	if p.Selected() != 1 {
		t.Fatalf("setup: Selected() = %d, want 1", p.Selected())
	}
	removeFired, removeOut = false, ""
	p.OnLeftDown(200, footerY)
	if !removeFired || removeOut != "T2" {
		t.Fatalf("Remove fired=%v out=%q, want true T2", removeFired, removeOut)
	}
}

// TestCalcPanelFactoryRegistered checks the factory id resolves to a
// constructible *CalcPanel so the designer can place it.
func TestCalcPanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.CalcPanel")
	if _, ok := obj.(*CalcPanel); !ok {
		t.Fatalf("factory gui.CalcPanel built %T, want *CalcPanel", obj)
	}
}

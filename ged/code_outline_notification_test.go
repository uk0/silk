package ged

import (
	"github.com/uk0/silk/gui"
	"testing"
)

type outlineUpdateProbe struct {
	CodeOutlinePanel
	updates int
}

func (p *outlineUpdateProbe) Update()            { p.updates++; p.Widget.Update() }
func newOutlineUpdateProbe() *outlineUpdateProbe { p := &outlineUpdateProbe{}; p.Init(p); return p }

func TestOutlineProviderInvalidatesItsOwnPaintRegion(t *testing.T) {
	e := gui.NewCodeEditor()
	e.SetText("package p\n")
	p := newOutlineUpdateProbe()
	p.SetEditor(e)
	defer p.SetEditor(nil)
	p.updates = 0
	e.SetAdditionalSymbols(e.TextRevision(), []gui.CodeSymbol{{Name: "Ready", Kind: gui.SymMacro, Line: 0}})
	if p.updates != 1 {
		t.Fatalf("symbol provider invalidated outline %d times, want 1", p.updates)
	}
	p.RefreshIfStale()
	wantOutlineRows(t, &p.CodeOutlinePanel, []string{"0:Ready"})
	p.SetEditor(nil)
	p.updates = 0
	e.SetAdditionalSymbols(e.TextRevision(), nil)
	if p.updates != 0 {
		t.Fatal("detached editor still repaints outline")
	}
}

func TestOutlineEditorSwitchUnsubscribesAndDoesNotDuplicate(t *testing.T) {
	a, b := gui.NewCodeEditor(), gui.NewCodeEditor()
	a.SetText("package a\n")
	b.SetText("package b\n")
	p := newOutlineUpdateProbe()
	p.SetEditor(a)
	p.SetEditor(a)
	p.SetEditor(b)
	defer p.SetEditor(nil)
	p.updates = 0
	a.SetText("package old\n")
	if p.updates != 0 {
		t.Fatal("old editor still invalidates the current outline")
	}
	b.SetText("package current\n")
	if p.updates != 1 {
		t.Fatal("new editor notification missing or duplicated", p.updates)
	}
}

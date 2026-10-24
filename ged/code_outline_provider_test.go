package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

func TestOutlineRefreshesAsyncProviderWithoutTextEdit(t *testing.T) {
	ed := gui.NewCodeEditor()
	ed.SetText("package p\n//silk:macro Declare()\n//silk:end\n")
	p := NewCodeOutlinePanel()
	p.SetEditor(ed)
	rev := ed.TextRevision()
	wantOutlineRows(t, p, nil)
	ed.SetAdditionalSymbols(rev, []gui.CodeSymbol{{Name: "Declare", Kind: gui.SymMacro, Line: 1}})
	if !p.RefreshIfStale() {
		t.Fatal("asynchronous symbols did not invalidate the already-bound outline")
	}
	wantOutlineRows(t, p, []string{"0:Declare"})
	if ed.TextRevision() != rev || p.RefreshIfStale() {
		t.Fatal("provider update changed text or caused perpetual refresh")
	}
	ed.SetAdditionalSymbols(rev, nil)
	if !p.RefreshIfStale() {
		t.Fatal("clearing provider symbols left old outline rows")
	}
	wantOutlineRows(t, p, nil)
}

func TestOutlineLargeFilePendingAndSynchronousNavigation(t *testing.T) {
	ed := gui.NewCodeEditor()
	defer ed.CancelSymbolIndex()
	ed.SetText("package p\n" + strings.Repeat("// padding\n", 4000) + "func TestLarge(){}\n")
	p := NewCodeOutlinePanel()
	p.SetEditor(ed)
	if !p.symbolsLoading || len(p.flatList) != 0 {
		t.Fatal("outline parsed on the UI thread or displayed stale rows")
	}
	if p.RefreshIfStale() {
		t.Fatal("pending state repeatedly refreshes")
	}
	ed.ParseSymbols() // explicit navigation can supersede pending background work
	if !p.RefreshIfStale() || p.symbolsLoading {
		t.Fatal("synchronous parse left outline stuck loading")
	}
	wantOutlineRows(t, p, []string{"0:TestLarge"})
	p.SetEditor(nil)
	if p.symbolsLoading {
		t.Fatal("detached editor is still loading")
	}
}

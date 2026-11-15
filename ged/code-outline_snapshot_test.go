package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func TestOutlineIdenticalSnapshotsPreserveOwnedTreeAndExpansion(t *testing.T) {
	e := gui.NewCodeEditor()
	e.SetText("package p\ntype T struct{}\nfunc (t *T) M() {}\n// comment")
	p := NewCodeOutlinePanel()
	p.SetEditor(e)
	defer p.SetEditor(nil)
	defer e.CancelSymbolIndex()
	p.tree[0].expanded = false
	p.rebuildFlatList()
	row := &p.flatList[0]
	p.selectedIdx = 0
	e.ReplaceAllText(strings.Replace(e.Text(), "comment", "longer comment", 1))
	if !p.RefreshIfStale() || &p.flatList[0] != row || p.tree[0].expanded || p.selectedIdx != 0 {
		t.Fatal("identical declarations rebuilt/collapsed user view state")
	}
	if n := testing.AllocsPerRun(100, func() { p.dirty = true; p.RefreshIfStale() }); n != 0 {
		t.Fatalf("duplicate ready invalidation allocated %v times", n)
	}
	// An unrelated editor can have the same local token. Rebinding must fetch
	// its actual data rather than treating that token as a global identity.
	other := gui.NewCodeEditor()
	other.SetText("package p\nfunc Other() {}")
	p.SetEditor(other)
	wantOutlineRows(t, p, []string{"0:Other"})
}

func TestOutlinePendingRetainsStorageButCannotDrawOrNavigateOldRows(t *testing.T) {
	e := gui.NewCodeEditor()
	e.SetText("package p\nfunc Old() {}")
	p := NewCodeOutlinePanel()
	p.SetSize(320, 240)
	p.SetEditor(e)
	defer p.SetEditor(nil)
	defer e.CancelSymbolIndex()
	row := &p.flatList[0]
	navigations := 0
	p.SetNavigateCallback(func(int) { navigations++ })
	p.selectedIdx = 0
	e.ReplaceAllText("package p\n" + strings.Repeat("// padding\n", 4000) + "func New() {}")
	p.RefreshIfStale()
	if !p.symbolsLoading || &p.flatList[0] != row {
		t.Fatal("pending state discarded reusable owned storage")
	}
	if p.hitTest(p.headerHeight()+4) != -1 {
		t.Fatal("stale rows remain hit-testable")
	}
	pixmap := paint.NewPixmap(320, 240)
	defer paint.CloseSurface(pixmap)
	painter := pixmap.NewPainter()
	defer paint.ClosePainter(painter)
	recording := &explorerDrawRecorder{Painter: painter}
	p.Draw(recording)
	if len(recording.texts) < 2 {
		t.Fatal("pending outline did not draw its header/status")
	}
	for _, text := range recording.texts {
		if strings.Contains(text.text, "Old") {
			t.Fatal("pending outline painted a stale symbol row")
		}
	}
	p.OnLeftDown(20, p.headerHeight()+4)
	for _, key := range []int{gui.KeyDown, gui.KeyHome, gui.KeyEnd, gui.KeyRight, gui.KeyLeft, gui.KeyEnter} {
		p.OnKeyDown(key, false)
	}
	if navigations != 0 {
		t.Fatal("pending stale outline dispatched a jump")
	}
	// A ready replacement must discard old rows and invalidate their token,
	// including cancellation of the still-running large-file worker.
	e.ReplaceAllText("package p\nfunc Ready() {}")
	p.RefreshIfStale()
	if p.symbolsLoading {
		t.Fatal("ready small-file snapshot stayed pending")
	}
	wantOutlineRows(t, p, []string{"0:Ready"})
	p.OnKeyDown(gui.KeyDown, false)
	p.OnKeyDown(gui.KeyEnter, false)
	if navigations != 1 {
		t.Fatal("ready outline no longer navigates")
	}
}

// Measure only outline refresh/copy/build work, not parsing or native paint.
// Duplicate invalidations are common while local and language-server results
// arrive for an unchanged declaration set in a large document.
func BenchmarkOutlineIdenticalSnapshotRefresh(b *testing.B) {
	e := gui.NewCodeEditor()
	e.SetText("package p\n" + strings.Repeat("func Item() {}\n", 5000))
	e.ParseSymbols()
	p := NewCodeOutlinePanel()
	p.SetEditor(e)
	defer p.SetEditor(nil)
	defer e.CancelSymbolIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		p.SetRevision(i + 1)
		p.RefreshIfStale()
	}
}

func TestOutlineBodyEditsKeepExactReadyRowsAndNavigation(t *testing.T) {
	e := gui.NewCodeEditor()
	e.SetText("package p\n" + strings.Repeat("// padding\n", 4000) + "type T struct{}\nfunc(t *T) MethodM(){ _ = \"a\" }\n")
	original := e.ParseSymbols()
	methodLine := -1
	for _, s := range original {
		if s.Name == "MethodM" {
			methodLine = s.Line
		}
	}
	p := NewCodeOutlinePanel()
	p.SetSize(320, 240)
	p.SetEditor(e)
	defer p.SetEditor(nil)
	defer e.CancelSymbolIndex()
	if methodLine < 0 || len(p.flatList) != 2 || p.symbolsLoading {
		t.Fatal("bad ready outline fixture")
	}
	storage := &p.flatList[0]
	p.selectedIdx = 1
	next := strings.Replace(e.Text(), `"a"`, `"longer text"`, 1)
	e.ReplaceAllText(next)
	if !p.RefreshIfStale() || p.symbolsLoading || &p.flatList[0] != storage || p.selectedIdx != 1 || !p.tree[0].expanded {
		t.Fatal("body edit rebuilt or hid exact rows")
	}
	pixmap := paint.NewPixmap(320, 240)
	defer paint.CloseSurface(pixmap)
	painter := pixmap.NewPainter()
	defer paint.ClosePainter(painter)
	recording := &explorerDrawRecorder{Painter: painter}
	p.Draw(recording)
	found := false
	for _, s := range recording.texts {
		if strings.Contains(s.text, "MethodM") {
			found = true
		}
	}
	if !found {
		t.Fatal("ready body edit did not draw its method")
	}
	line := -1
	p.SetNavigateCallback(func(v int) { line = v })
	p.OnLeftDown(40, p.headerHeight()+p.rowHeight+4)
	if line != methodLine {
		t.Fatal("body edit shifted navigation", line, methodLine)
	}
	// Enter/newline is outside the proof: old rows must immediately stop being
	// drawn/hit-testable until fresh physical coordinates are delivered.
	e.ReplaceAllText(strings.Replace(e.Text(), "_ =", "\n_ =", 1))
	p.RefreshIfStale()
	if !p.symbolsLoading || p.hitTest(p.headerHeight()+4) != -1 {
		t.Fatal("newline reused stale ready rows")
	}
}

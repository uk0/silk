package gui

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func completionTestEditor() *CodeEditor {
	ed := NewCodeEditor()
	ed.SetSize(800, 480)
	ed.SetText("Pr\n" + strings.Repeat("someText\n", 60))
	ed.ScrollToLineCol(0, 2)
	p := NewCompletionPopup(ed)
	p.visible = true
	p.prefix = "Pr"
	for i := 0; i < 24; i++ {
		p.filtered = append(p.filtered, CompletionItem{Text: fmt.Sprintf("Print%02d", i), Source: "test provider"})
	}
	ed.completion = p
	return ed
}

func TestCompletionViewportGeometry(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, y := range []float64{0, 40, 170, 280, 999} {
			for _, x := range []float64{-100, 65, 799} {
				l := fitCompletionLayout(x, y*scale, 18*scale, 24*scale, 300*scale, 55*scale, 24*scale, 600*scale, 310*scale, 8)
				if l.rows < 1 || l.x < 55*scale || l.y < 24*scale || l.x+l.w > 600*scale+0.01 || l.y+l.h > 310*scale+0.01 {
					t.Fatalf("out of viewport: scale=%v x=%v y=%v %+v", scale, x, y, l)
				}
				if row, ok := l.hit(l.x+1, l.y+float64(l.rows)*l.rowH+1, 0); !ok || row != -1 {
					t.Fatal("footer is not a shield")
				}
				if _, ok := l.hit(l.x+l.w, l.y, 0); ok {
					t.Fatal("right edge belongs to editor")
				}
			}
		}
	}
	for _, size := range [][2]float64{{70, 30}, {90, 40}, {200, 1}} {
		if l := fitCompletionLayout(60, 30, 18, 24, 300, 55, 24, size[0], size[1], 8); l.rows != 0 {
			t.Fatalf("tiny viewport: %+v", l)
		}
	}
}

func TestCompletionPointerAcceptAndAtomicUndo(t *testing.T) {
	ed := completionTestEditor()
	before := ed.Text()
	l := ed.completion.layout(ed)
	ed.OnMouseMove(l.x+30, l.y+2.5*l.rowH)
	if ed.completion.selectedIdx != 2 || ed.CursorCol() != 2 {
		t.Fatal("hover moved document caret")
	}
	ed.OnLeftDown(l.x+30, l.y+2.5*l.rowH)
	ed.OnLeftUp(l.x+30, l.y+2.5*l.rowH)
	if !strings.HasPrefix(ed.Text(), "Print02\n") || ed.CompletionVisible() || ed.mouseDown {
		t.Fatalf("not accepted: %q", ed.Text())
	}
	ed.Undo()
	if ed.Text() != before || ed.CursorCol() != 2 {
		t.Fatalf("prefix lost on undo: %q", ed.Text())
	}
	ed.Redo()
	if !strings.HasPrefix(ed.Text(), "Print02\n") || ed.CursorCol() != 7 {
		t.Fatal("redo lost text/caret")
	}
}

func TestCompletionPointerProviderAndFooter(t *testing.T) {
	ed := completionTestEditor()
	p := ed.completion
	l := p.layout(ed)
	before := ed.Text()
	ed.OnLeftDown(l.x+30, l.y+float64(l.rows)*l.rowH+2)
	if ed.Text() != before || ed.CursorCol() != 2 || !ed.CompletionVisible() {
		t.Fatal("footer click reached editor")
	}
	called := 0
	p.filtered[1].apply = &completionApply{run: func(target *CodeEditor) {
		if target != ed {
			t.Fatal("wrong target")
		}
		called++
	}}
	ed.OnLeftDown(l.x+30, l.y+1.5*l.rowH)
	if called != 1 || ed.Text() != before || ed.CursorCol() != 2 {
		t.Fatal("provider callback not exclusively invoked")
	}
}

func TestCompletionWheelAndPagesDoNotScrollEditor(t *testing.T) {
	ed := completionTestEditor()
	p := ed.completion
	l := p.layout(ed)
	ed.OnMouseWheel(l.x+30, l.y+12, -1)
	if p.scrollY != 3 || ed.scrollY != 0 || ed.CursorLine() != 0 {
		t.Fatal("wheel scrolled document")
	}
	p.selectIndex(0)
	ed.OnKeyDown(KeyPageDown, false)
	if p.selectedIdx != p.pageSize() || ed.CursorCol() != 2 {
		t.Fatal("PageDown moved caret")
	}
	ed.OnKeyDown(KeyPageUp, false)
	if p.selectedIdx != 0 {
		t.Fatal("PageUp failed")
	}
	p.selectIndex(len(p.filtered) - 1)
	p.SelectNext()
	if p.selectedIdx != 0 || p.scrollY != 0 {
		t.Fatal("wrap failed")
	}
	p.SelectPrev()
	if p.selectedIdx != len(p.filtered)-1 || p.scrollY != len(p.filtered)-p.pageSize() {
		t.Fatal("reverse wrap failed")
	}
	for _, z := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if p.scroll(z) {
			t.Fatal("invalid wheel input")
		}
	}
}

func TestCompletionOutsideDismisses(t *testing.T) {
	ed := completionTestEditor()
	ed.OnLeftDown(350, 420)
	if ed.CompletionVisible() {
		t.Fatal("outside click did not dismiss")
	}
	ed = completionTestEditor()
	ed.OnMouseWheel(750, 420, -1)
	if ed.CompletionVisible() || ed.scrollY == 0 {
		t.Fatal("outside wheel not routed to editor")
	}
}

func TestCompletionViewportShrinksSelectionIntoView(t *testing.T) {
	ed := completionTestEditor()
	p := ed.completion
	p.selectIndex(18)
	ed.SetSize(300, 155)
	l := p.layout(ed)
	if l.rows < 1 || l.rows >= 8 || p.selectedIdx < p.scrollY || p.selectedIdx >= p.scrollY+l.rows {
		t.Fatalf("selection outside: %+v %+v", l, p)
	}
}

func TestCompletionFoldAnchor(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetSize(800, 480)
	ed.SetText("func a() {\n x()\n y()\n}\nPr")
	ed.ToggleFold(0)
	ed.ScrollToLineCol(4, 2)
	ed.TriggerCompletion()
	l := ed.completion.layout(ed)
	_, caretY := ed.caretLocalXY()
	if l.rows == 0 || l.y != caretY {
		t.Fatalf("not anchored to folded caret: %+v y=%v", l, caretY)
	}
}

func TestCompletionStatusDoesNotReopenAndIsBounded(t *testing.T) {
	ed := completionTestEditor()
	p := ed.completion
	ed.SetCompletionStatus("waiting\nfor\tgopls")
	if p.footerText() != "waiting for gopls" {
		t.Fatal(p.footerText())
	}
	p.Dismiss()
	ed.SetCompletionStatus("gopls timed out")
	ed.SetExternalCompletions([]ExternalCompletion{{Label: "Println", Source: "gopls"}})
	if ed.CompletionVisible() {
		t.Fatal("late reply reopened dismissed popup")
	}
	ed.SetCompletionStatus(strings.Repeat("界", 1000))
	if len([]rune(p.status)) != 256 {
		t.Fatal("unbounded status")
	}
	ed.ClearExternalCompletions()
	if p.status != "" {
		t.Fatal("stale status survived clear")
	}
	ed.SetExternalCompletions([]ExternalCompletion{{Label: "Println", Source: "gopls", Apply: func(*CodeEditor) {}}})
	ed.TriggerCompletion()
	if !strings.HasPrefix(p.footerText(), "gopls |") {
		t.Fatal("source missing", p.footerText())
	}
}

func TestCompletionVisibleRowBinarySearch(t *testing.T) {
	ed := NewCodeEditor()
	ed.visLineCache = []int{0, 1, 5, 6, 10}
	ed.visLineValid = true
	for line, want := range map[int]int{-1: 0, 0: 0, 1: 1, 2: 1, 4: 1, 5: 2, 9: 3, 10: 4, 99: 4} {
		if got := ed.lineToVisualRow(line); got != want {
			t.Fatalf("line=%d row=%d want=%d", line, got, want)
		}
	}
}

func BenchmarkCompletionLargeFileHit(b *testing.B) {
	ed := completionTestEditor()
	ed.lines = make([]string, 100000)
	ed.visLineCache = make([]int, len(ed.lines))
	for i := range ed.lines {
		ed.lines[i] = "identifier"
		ed.visLineCache[i] = i
	}
	ed.visLineValid = true
	ed.cursorLine = len(ed.lines) - 1
	ed.scrollY = float64(ed.cursorLine) * (ed.font.FontExtents().Height + 2)
	p := ed.completion
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := p.layout(ed)
		l.hit(l.x+10, l.y+10, p.scrollY)
	}
}

package gui

import (
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func TestSignaturePlacementOppositeSideAndTinyViewport(t *testing.T) {
	area := geom.Rect{X: 25, Y: 30, Width: 600, Height: 340}
	below := geom.Rect{X: 150, Y: 180, Width: 300, Height: 110}
	prefer := geom.Rect{X: 162, Y: 198, Width: 220, Height: 30}
	if got := placeSignatureAlongsideCompletion(prefer, below, area, 180, 20); got != (geom.Rect{X: 162, Y: 126, Width: 220, Height: 30}) {
		t.Fatalf("completion below caret: got %+v", got)
	}
	above := geom.Rect{X: 150, Y: 55, Width: 300, Height: 110}
	if got := placeSignatureAlongsideCompletion(prefer, above, area, 180, 20); got != prefer {
		t.Fatalf("ordinary placement is safe when completion uses other side: %+v", got)
	}
	if got := placeSignatureAlongsideCompletion(prefer, below, geom.Rect{X: 150, Y: 165, Width: 300, Height: 125}, 180, 20); validPopupArea(got) {
		t.Fatalf("did not suppress a colliding hint in a short viewport: %+v", got)
	}
}

func TestSignaturePlacementTracksLateCompletionWithoutEditingItsState(t *testing.T) {
	root, win, ed := editorToolTipFixture(t)
	t.Cleanup(HideToolTip)
	root.SetSize(800, 420)
	ed.SetBounds(100, 40, 650, 360)
	ed.SetText(strings.Repeat("queryFn(sample)\n", 10))
	ed.NavigateToLineCol(4, 8)
	ed.completion = &CompletionPopup{maxVisible: 5, pageRows: 17, filtered: []CompletionItem{{Text: "sample"}, {Text: "sampleRate"}, {Text: "sampleCount"}}}
	x, y := ed.MapToWindow(ed.caretLocalXY())
	tip := newOwnedToolTip(hoverToolTipAnchor{win, x, y}, ed, "queryFn(sample string)")
	if tip == nil {
		t.Fatal("signature missing")
	}
	tip.avoidCompletion = true
	replaceExplicitToolTip(tip)
	pix := paint.NewPixmap(800, 420)
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	full := geom.Rect{Width: 800, Height: 420}
	drawOwnedToolTip(win, g, full)
	original := ownedToolTipFor(win)
	ed.completion.visible, ed.completion.open = true, true
	popup, _, _ := ed.completionAvoidance()
	if !validPopupArea(popup) {
		t.Fatal("test did not open completion")
	}
	drawOwnedToolTip(win, g, full)
	moved := ownedToolTipFor(win)
	if moved == original || moved.bounds.Bottom() > popup.Y || !validPopupArea(moved.bounds) || ed.completion.pageRows != 17 || !ed.completion.visible {
		t.Fatalf("late results did not move a passive tooltip, or mutated completion: before=%+v after=%+v popup=%+v", original.bounds, moved.bounds, popup)
	}
	drawOwnedToolTip(win, g, full)
	if ownedToolTipFor(win) != moved {
		t.Fatal("settled position recreated the overlay on paint")
	}
	ed.completion.Dismiss()
	drawOwnedToolTip(win, g, full)
	if restored := ownedToolTipFor(win); restored.bounds != original.bounds {
		t.Fatalf("did not reuse original position when completion closed: %+v", restored.bounds)
	}
}

func TestOvertypingCallCloserClosesCompletionWithoutEdit(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("f()")
	ed.ScrollToLineCol(0, 2)
	ed.completion = &CompletionPopup{visible: true, open: true, maxVisible: 5, filtered: []CompletionItem{{Text: "sample"}}}
	calls := 0
	ed.SigChanged(func(string) { calls++ })
	ed.OnTextInput(")")
	if ed.Text() != "f()" || ed.CursorCol() != 3 || calls != 0 || ed.completion.visible || ed.completion.open {
		t.Fatalf("skipping a closer retained completions or edited the source: text=%q caret=%d changes=%d", ed.Text(), ed.CursorCol(), calls)
	}
}

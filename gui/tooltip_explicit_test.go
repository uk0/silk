package gui

import (
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func editorToolTipFixture(t *testing.T) (*Form, *Window, *CodeEditor) {
	t.Helper()
	root := NewForm()
	root.SetSize(320, 180)
	root.NakedWidget().setVisible(true)
	win := &Window{widget: root, wt: WtForm}
	ed := NewCodeEditor()
	ed.SetParent(root)
	ed.SetBounds(25, 20, 245, 130)
	root.NakedWidget().win = win
	t.Cleanup(func() {
		root.NakedWidget().win = nil
	})
	return root, win, ed
}

func TestExplicitToolTipRespectsEditorOwnerAndRetiresOnHide(t *testing.T) {
	_, a, ed := editorToolTipFixture(t)
	_, b, other := editorToolTipFixture(t)
	t.Cleanup(HideToolTip)
	tip := newOwnedToolTip(hoverToolTipAnchor{a, 318, 173}, ed, "fn(value string)")
	replaceExplicitToolTip(tip)
	if tip == nil || ownedToolTipFor(a) != tip || ownedToolTipFor(b) != nil ||
		tip.bounds.Right() > 320 || tip.bounds.Bottom() >= 173 || tip.label.Window() != nil || tip.label.Parent() != nil {
		t.Fatal("editor hint was not confined to the correct client viewport")
	}
	HideToolTipFor(other)
	dismissOwnedToolTips(b)
	if !IsToolTipVisible() {
		t.Fatal("independent editor dismissed the active editor's hint")
	}
	ed.Hide()
	if IsToolTipVisible() {
		t.Fatal("hidden editor retained a hint")
	}
}

func TestExplicitToolTipDoesNotEnterPickOrLayoutAndPaintsOnlyOwner(t *testing.T) {
	root, a, ed := editorToolTipFixture(t)
	_, b, _ := editorToolTipFixture(t)
	t.Cleanup(HideToolTip)
	anchor := hoverToolTipAnchor{a, 80, 62}
	tip := newOwnedToolTip(anchor, ed, "definition")
	if tip == nil {
		t.Fatal("missing explicit hint")
	}
	replaceExplicitToolTip(tip)
	w, h := root.Size()
	pix := paint.NewPixmap(int(w), int(h))
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	full := geom.Rect{Width: w, Height: h}
	dx, dy := int(tip.bounds.X)+5, int(tip.bounds.Y)+5
	check := func(want bool) {
		t.Helper()
		pix.Flush()
		img, err := pix.Image()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, alpha := img.At(dx, dy).RGBA(); (alpha != 0) != want {
			t.Fatalf("expected overlay visible=%v, alpha=%d", want, alpha)
		}
	}
	drawOwnedToolTip(b, g, full)
	check(false)
	drawOwnedToolTip(a, g, full)
	check(true)
	if root.FindWidgetAt(float64(dx), float64(dy)) != ed || len(root.Children()) != 1 {
		t.Fatal("overlay mutated the real hit target or layout")
	}
	ed.Detach()
	if IsToolTipVisible() {
		t.Fatal("detaching the editor retained its old overlay")
	}
}

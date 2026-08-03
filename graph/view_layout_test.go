package graph

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// newLayoutTestView builds a headless view of the given pixel size holding a
// scene of the given millimetre size, with the page margin hidden — the
// configuration the GED designer canvas uses.
func newLayoutTestView(viewW, viewH, sceneW, sceneH float64) *GraphView {
	v := NewView()
	sc := new(SceneItem)
	sc.Init(sc)
	sc.SetSize(sceneW, sceneH)
	v.SetScene(sc)
	v.SetPageMarginVisible(false)
	v.SetSize(viewW, viewH)
	v.SetZoomFactor(1)
	v.Layout()
	return v
}

// TestLayoutPageVisibleWhenItFits: a page small enough to need no scrolling
// must be laid out entirely inside the viewport. The page used to be centred
// against a rectangle that always included the page margins, even with the
// margin hidden, so the centring was half a margin wider than the scrollable
// extent — the page (and the form on it) started at a negative offset and its
// left edge sat outside the widget with no scroll range to bring it back.
func TestLayoutPageVisibleWhenItFits(t *testing.T) {
	v := newLayoutTestView(430, 300, 50, 50)

	if v.HorzScrollBar().IsVisible() {
		t.Fatal("precondition: page should fit horizontally, horz scrollbar visible")
	}

	x0, _ := v.PageOriginPx()
	if x0 < 0 {
		t.Errorf("page origin x = %v, want >= 0 (page laid out off the left edge)", x0)
	}

	sceneX, _ := v.SceneOriginPx()
	sceneW := gui.MmToPixel(50 * v.ZoomFactor())
	viewW, _ := v.Size()
	if sceneX < 0 || sceneX+sceneW > viewW {
		t.Errorf("scene spans [%v,%v] px, outside viewport [0,%v]", sceneX, sceneX+sceneW, viewW)
	}
}

// TestLayoutUsesTopPaddingWhenScrolling: with the page taller than the
// viewport the vertical offset is the top padding. The vertical branch used
// the *left* padding, so an asymmetric padding put the page at the wrong
// distance from the top edge.
func TestLayoutUsesTopPaddingWhenScrolling(t *testing.T) {
	v := newLayoutTestView(400, 300, 500, 500)
	v.SetPaddingPx(4, 4, 40, 4)

	if !v.VertScrollBar().IsVisible() {
		t.Fatal("precondition: page should overflow vertically, vert scrollbar hidden")
	}

	_, y0 := v.PageOriginPx()
	if y0 != 40 {
		t.Errorf("page origin y = %v, want 40 (the top padding)", y0)
	}
}

package gui

import (
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func testTooltipWidget() *Widget {
	w := new(Widget)
	w.Init(w)
	return w
}

func TestRegisteredToolTipRespectsHierarchyAndWindowBoundary(t *testing.T) {
	root := NewForm()
	parent, child, isolated := testTooltipWidget(), testTooltipWidget(), NewForm()
	parent.SetParent(root)
	child.SetParent(parent)
	isolated.SetParent(parent)
	isolated.NakedWidget().win = &Window{widget: isolated}
	t.Cleanup(func() { isolated.NakedWidget().win = nil })
	SetToolTip(parent, "ancestor")
	SetToolTip(child, "specific")
	if w, text := registeredToolTip(child); w != child || text != "specific" {
		t.Fatalf("nearest registration: %T, %q", w, text)
	}
	SetToolTip(child, "")
	if w, text := registeredToolTip(child); w != parent || text != "ancestor" || GetToolTip(child) != "" {
		t.Fatalf("fallback after clear: %T, %q", w, text)
	}
	if w, text := registeredToolTip(isolated); w != nil || text != "" {
		t.Fatalf("hint crossed independent window boundary: %T, %q", w, text)
	}
}

func TestRegisteredToolTipFitsClientAndFlipsAtBottom(t *testing.T) {
	b := registeredToolTipBounds(210, 120, 201, 112, SizeHints{Width: 70.2, Height: 29.3})
	if b != (geom.Rect{X: 139, Y: 78, Width: 71, Height: 30}) {
		t.Fatalf("edge placement: %v", b)
	}
	b = registeredToolTipBounds(80, 25, 76, 19, SizeHints{Width: 700, Height: 200})
	if b != (geom.Rect{Width: 80, Height: 25}) {
		t.Fatalf("oversized hint escaped client: %v", b)
	}
}

func TestRegisteredToolTipPaintsOnlyDamageAndNeverInterceptsInput(t *testing.T) {
	root := NewForm()
	root.SetSize(250, 140)
	owner, underlay := testTooltipWidget(), testTooltipWidget()
	owner.SetParent(root)
	owner.SetBounds(3, 9, 40, 26)
	underlay.SetParent(root)
	underlay.SetBounds(65, 57, 160, 70)
	win := &Window{widget: root}
	root.NakedWidget().win = win
	t.Cleanup(func() {
		dismissRegisteredToolTip(nil)
		root.NakedWidget().win = nil
	})

	label := newTooltipWindow("Tooltip")
	label.NakedWidget().w, label.NakedWidget().h = 108, 38
	visibleHoverToolTip = &hoverToolTipOverlay{
		hoverToolTipAnchor: hoverToolTipAnchor{win: win, x: 108, y: 47},
		owner:              owner, label: label, bounds: geom.Rect{X: 120, Y: 65, Width: 108, Height: 38},
	}
	activeHoverToolTipOwner.Store(owner.NakedWidget())
	pix := paint.NewPixmap(250, 140)
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	// Draw only a subset of the hint (as in a Win32 partial repaint).
	drawRegisteredToolTip(win, g, geom.Rect{X: 140, Y: 76, Width: 30, Height: 16})
	pix.Flush()
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(150, 83).RGBA(); a == 0 {
		t.Fatal("hint did not paint its damaged area")
	}
	for _, pt := range [][2]int{{0, 0}, {125, 76}, {172, 83}, {150, 94}} {
		if _, _, _, a := img.At(pt[0], pt[1]).RGBA(); a != 0 {
			t.Fatalf("hint painted outside damage: %v", pt)
		}
	}
	if root.FindWidgetAt(150, 83) != underlay || label.Window() != nil || label.Parent() != nil ||
		len(root.Children()) != 2 {
		t.Fatal("registered hint acquired a native window, layout child or hit target")
	}
	dismissRegisteredToolTip(&Window{})
	if !IsToolTipVisible() {
		t.Fatal("late leave from another window hid the active hint")
	}
	owner.Hide()
	if visibleHoverToolTip != nil || IsToolTipVisible() {
		t.Fatal("hidden owner retained a visible hint")
	}
}

func TestRegisteredToolTipMotionAndClickRetireOnlyAutomaticHint(t *testing.T) {
	root := NewForm()
	win := &Window{widget: root}
	owner, sibling := testTooltipWidget(), testTooltipWidget()
	owner.SetParent(root)
	sibling.SetParent(root)
	current := &hoverToolTipOverlay{hoverToolTipAnchor: hoverToolTipAnchor{win: win}, owner: owner}
	visibleHoverToolTip = current
	activeHoverToolTipOwner.Store(owner.NakedWidget())
	t.Cleanup(func() { dismissRegisteredToolTip(nil) })
	trackRegisteredToolTip(win, owner, 9, 10)
	if visibleHoverToolTip != current || pendingHoverToolTip.x != 9 {
		t.Fatal("movement inside owner disturbed a visible hint")
	}
	trackRegisteredToolTip(win, sibling, 45, 15)
	if visibleHoverToolTip != nil || pendingHoverToolTip.win != win {
		t.Fatal("moving to a different control failed to retire old hint")
	}
	dismissRegisteredToolTip(nil)
	if pendingHoverToolTip.win != nil {
		t.Fatal("click left a pending hint that could appear over a gesture")
	}
}

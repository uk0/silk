package gui

import (
	"math"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestCompactToolbarGeometryAtFontAndIconSizes(t *testing.T) {
	oldFont := Theme().Font
	t.Cleanup(func() { Theme().Font = oldFont })
	for _, fontSize := range []int{12, 16, 24} {
		Theme().Font = paint.NewFont(oldFont.Family(), fontSize, false, false)
		for _, iconSize := range []int{16, 24, 32} {
			bar := NewToolBar()
			bar.SetCompact(true)
			bar.SetIconSize(iconSize)
			bar.SetSpacing(2)
			icon := bar.AddAction("Save", paint.LoadIcon("save"), func() {})
			label := bar.AddAction("Design", nil, func() {})
			bar.AddSeparator()
			both := bar.AddAction("水平布局", paint.LoadIcon("layout-horizontal"), func() {})
			both.SetTextVisible(true)
			for _, width := range []float64{900, 180, 28, 0} {
				bar.SetSize(width, bar.SizeHints().Height)
				bar.Layout()
				last := 0.0
				for _, item := range bar.Items() {
					if !item.IsVisible() {
						continue
					}
					x, y, w, h := item.Bounds()
					if x < last || x+w > width+0.01 || y < 0 || y+h > bar.Height()+0.01 {
						t.Fatalf("font=%v icon=%v width=%v bounds=%v,%v,%v,%v", fontSize, iconSize, width, x, y, w, h)
					}
					last = x + w
				}
				if width == 900 {
					if icon.Width() != icon.Height() || icon.Height() < 28 || icon.Height() != label.Height() || label.Height() != both.Height() {
						t.Fatal("unequal or undersized tool targets")
					}
					ext := Theme().Font.TextExtents(both.Text())
					if both.Width() < float64(iconSize)+math.Max(ext.Width, ext.XAdvance)+22 {
						t.Fatal("icon and label overlap or clip")
					}
				} else if len(bar.OverflowItems()) == 0 {
					t.Fatal("narrow toolbar lost overflow access")
				}
			}
		}
	}
}

func TestCompactMenuInvalidatesCachedHeaderButNotPopupRows(t *testing.T) {
	bar := NewMenuBar()
	_, header := bar.AddSubMenu("File", nil, nil)
	before := header.SizeHints()
	bar.SetCompact(true)
	after := header.SizeHints()
	if after.Height >= before.Height || after.Height < 26 {
		t.Fatal(before, after)
	}
	bar.SetCompact(false)
	if header.SizeHints() != before {
		t.Fatal("menu density is not reversible")
	}
	popup := NewPopupMenu()
	_, entry := popup.AddSubMenu("Long command", nil, nil)
	want := entry.SizeHints()
	popup.SetCompact(true)
	if entry.SizeHints() != want {
		t.Fatal("compact menu bar changed popup targets")
	}
}

type compactActionTestView struct {
	Widget
	actions []IAction
}

func (v *compactActionTestView) Actions() []IAction { return v.actions }

func TestCompactDockAppliesToCachedAndNewViewActions(t *testing.T) {
	f := NewFrame()
	d := f.SuggestDocDock().(*Dock)
	view := func() *compactActionTestView {
		v := &compactActionTestView{}
		v.Init(v)
		a := NewAction()
		a.SetObjName("edit-undo")
		a.SetIcon(paint.LoadIcon("edit-undo"))
		v.actions = []IAction{a}
		return v
	}
	first := view()
	d.AddView(first)
	d.SetCompactActions(true)
	second := view()
	d.AddView(second)
	for _, v := range []*compactActionTestView{first, second} {
		menu := d.menuCache[v]
		if menu == nil || !menu.compact {
			t.Fatal("cached/new strip did not inherit compact chrome")
		}
		button := menu.Items()[0].(*Button)
		h := button.SizeHints()
		if h.Width != h.Height || h.Height < 28 {
			t.Fatal("non-square or undersized canvas action", h)
		}
	}
	d.SetCompactActions(false)
	if d.menuCache[first].compact || d.menuCache[second].compact {
		t.Fatal("density did not reset")
	}
}

func TestToolbarOverflowAnchorsTrailingEdgeWithReadableNames(t *testing.T) {
	bar := NewToolBar()
	bar.SetCompact(true)
	bar.AddAction("Preview", paint.LoadIcon("preview"), func() {})
	bar.AddAction("Properties", paint.LoadIcon("properties"), func() {})
	bar.SetSize(40, bar.SizeHints().Height)
	bar.Layout()
	old := showToolBarPopup
	t.Cleanup(func() { showToolBarPopup = old })
	called := false
	showToolBarPopup = func(menu *Menu, x, y float64) {
		called = true
		button := bar.overflowButton()
		bx, by := button.MapToGlobal(0, 0)
		if math.Abs(x+menu.SizeHints().Width-bx-button.Width()) > 0.01 || y != by+button.Height() {
			t.Fatal("overflow did not align its trailing edge below the chevron", x, y)
		}
		for _, item := range menu.Items() {
			entry := item.(*Button)
			if !entry.IsTextVisible() || entry.Text() == "" {
				t.Fatal("overflow lost readable label")
			}
		}
	}
	bar.showOverflow()
	if !called {
		t.Fatal("overflow menu did not open")
	}
}

package gui

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

func overflowTabs(n int) *TabBar {
	b := NewTabBar()
	for i := 0; i < n; i++ {
		b.tabs = append(b.tabs, &_Tab{data: &tabPageRef{title: fmt.Sprintf("Panel %02d 中文", i)}})
	}
	b.SetBounds(0, 0, 360, 30)
	b.Layout()
	return b
}

func TestTabOverflowActiveVisibilityAndHitGeometry(t *testing.T) {
	b := overflowTabs(12)
	for _, width := range []float64{360, 180, 80, 28, 0, 3000} {
		b.SetSize(width, 30)
		b.Layout()
		for i := range b.tabs {
			b.SetActiveTab(i)
			tab := b.tabs[i]
			left := tab.x - b.scrollX
			right := left + tab.width
			if b.viewportW > 0 && (left < -1e-6 || right > b.viewportW+1e-6) {
				t.Fatalf("width=%v active=%d rect=%v..%v viewport=%v", width, i, left, right, b.viewportW)
			}
			if tab.width > 0 {
				x := left + tab.width*.3
				if got, _ := b.HitTest(x, 15); got != i {
					t.Fatalf("active hit %d want %d", got, i)
				}
				if got := b.DropIndex(left+tab.width*.25, 15); got != i {
					t.Fatalf("drop before=%d want %d", got, i)
				}
				if got := b.DropIndex(left+tab.width*.75, 15); got != i+1 {
					t.Fatalf("drop after=%d want %d", got, i+1)
				}
			}
			if !utf8.ValidString(tab.paintLabel) {
				t.Fatal("split Unicode title")
			}
		}
		if b.overflowBtn != nil && b.overflowBtn.IsVisible() {
			x, _, w, _ := b.overflowBtn.Bounds()
			if x != b.viewportW || x+w > width {
				t.Fatal("overflow overlaps tabs or escapes bounds")
			}
			if hit, close := b.HitTest(x+w/2, 15); hit != -1 || close {
				t.Fatal("chooser hit a hidden tab")
			}
		}
	}
	b.SetActiveTab(100)
	if b.ActiveTab() != 11 {
		t.Fatal("invalid activation changed selection")
	}
}

func TestTabOverflowChooserUsesLiveIdentityAndRetainsOwner(t *testing.T) {
	b := overflowTabs(8)
	b.SetActiveTab(7)
	old := showTabBarPopup
	defer func() { showTabBarPopup = old }()
	var menu *Menu
	showTabBarPopup = func(m *Menu, x, y float64) {
		menu = m
		if len(m.Items()) != 8 || m.Parent() != b {
			t.Fatal("chooser lost items or ownership")
		}
		for i, item := range m.Items() {
			button := item.(*Button)
			if button.Text() != b.tabs[i].realText() || button.IsChecked() != (i == 7) {
				t.Fatal("chooser label/active marker mismatch")
			}
		}
		target := b.tabs[4]
		b.RemoveTab(0)
		m.Items()[4].(*Button).Action().Trigger(nil)
		if b.tabs[b.ActiveTab()] != target {
			t.Fatal("menu used a stale numeric index")
		}
		b.RemoveTab(b.ActiveTab())
		before := b.ActiveTab()
		m.Items()[4].(*Button).Action().Trigger(nil)
		if b.ActiveTab() != before {
			t.Fatal("removed menu target activated a replacement")
		}
	}
	b.overflowBtn.Action().Trigger(nil)
	if menu == nil || menu.Parent() != b {
		t.Fatal("nonblocking popup lost its logical owner while still open")
	}
	first := menu
	showTabBarPopup = func(m *Menu, x, y float64) {
		if m != first || m.Parent() != b || len(m.Items()) != len(b.tabs) {
			t.Fatal("chooser not reused or stale entries retained")
		}
	}
	for i := 0; i < 10; i++ {
		b.showAllTabs()
	}
	if b.overflowBtn.Icon() == nil || b.overflowBtn.FocusPolicy() != TabFocus {
		t.Fatal("missing chooser icon or keyboard focus")
	}
}

func TestTabOverflowKeyboardWheelAndCloseAfterScroll(t *testing.T) {
	b := overflowTabs(10)
	b.OnKeyDown(KeyEnd, false)
	if b.ActiveTab() != 9 {
		t.Fatal("End")
	}
	b.OnKeyDown(KeyLeft, false)
	if b.ActiveTab() != 8 {
		t.Fatal("Left")
	}
	b.OnMouseWheel(1, 1, 1)
	if b.ActiveTab() != 7 {
		t.Fatal("wheel previous")
	}
	b.OnMouseWheel(1, 1, -1)
	if b.ActiveTab() != 8 {
		t.Fatal("wheel next")
	}
	b.OnKeyDown(KeyHome, false)
	if b.ActiveTab() != 0 {
		t.Fatal("Home")
	}
	b.OnKeyDown(KeyRight, false)
	if b.ActiveTab() != 1 {
		t.Fatal("Right")
	}
	b.SetActiveTab(9)
	closed := -1
	b.SetCloseCallback(func(_ *TabBar, idx int) bool { closed = idx; return true })
	tab := b.tabs[9]
	x := tab.x - b.scrollX + tab.width - Theme().TabMargin.R - Theme().TabCloseSize/2
	_, close := b.HitTest(x, 15)
	if !close {
		t.Fatal("scrolled close target lost")
	}
	b.OnLeftDown(x, 15)
	b.OnLeftUp(x, 15)
	if closed != 9 {
		t.Fatal("wrong tab closed")
	}
	b.closeBtn = false
	if _, close = b.HitTest(x, 15); close {
		t.Fatal("disabled close still hit")
	}
}

type tabPaintRecorder struct {
	*labelPaintRecorder
	font paint.Font
}

func (r *tabPaintRecorder) SetFont(f paint.Font)               { r.font = f }
func (r *tabPaintRecorder) Font() paint.Font                   { return r.font }
func (r *tabPaintRecorder) DrawIcon(paint.Icon, float64, bool) {}

func TestTabOverflowDrawOnlyVisibleCells(t *testing.T) {
	b := overflowTabs(1000)
	b.SetActiveTab(999)
	r := &tabPaintRecorder{labelPaintRecorder: newLabelPaintRecorder()}
	b.Draw(r)
	if b.firstVisible < 990 || len(r.texts) == 0 || len(r.texts) > 5 {
		t.Fatalf("rendered hidden tabs: start=%d texts=%d", b.firstVisible, len(r.texts))
	}
	if r.texts[len(r.texts)-1] != b.tabs[999].paintLabel {
		t.Fatal("active title not painted")
	}
	if r.saves != r.restores || r.clips < 2 {
		t.Fatal("unbalanced viewport/cell clips")
	}
	b.SetSize(5000, 30)
	b.Layout()
	b.SetActiveTab(0)
	first := b.tabs[0].SizeHints()
	f := Theme().Font
	copy := f.Dup()
	Theme().Font = copy
	defer func() { Theme().Font = f }()
	copy.SetSize(copy.Size() + 4)
	b.Layout()
	if b.tabs[0].SizeHints().Width <= first.Width {
		t.Fatal("font cache not invalidated")
	}
}

func BenchmarkTabOverflowVisibleDraw(b *testing.B) {
	tabs := overflowTabs(10000)
	tabs.SetActiveTab(9999)
	r := &tabPaintRecorder{labelPaintRecorder: newLabelPaintRecorder()}
	tabs.Draw(r)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.texts = r.texts[:0]
		r.positions = r.positions[:0]
		tabs.Draw(r)
	}
}

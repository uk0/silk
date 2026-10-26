package gui

import (
	"github.com/uk0/silk/paint"
	"testing"
)

func fractionalOverflowTabs() *TabBar {
	b := overflowTabs(5)
	width := b.tabs[0].width + b.tabs[1].width*0.7 + 30
	b.SetSize(width, 30)
	b.Layout()
	return b
}

func TestTabOverflowNeverStartsWithTitleSuffix(t *testing.T) {
	b := fractionalOverflowTabs()
	for _, active := range []int{2, 4, 1, 3, 0} {
		b.SetActiveTab(active)
		if b.firstVisible < len(b.tabs) && b.tabs[b.firstVisible].x != b.scrollX {
			t.Fatalf("active=%d: first cell starts at %v, scroll=%v; title/icon clipped on the left", active, b.tabs[b.firstVisible].x, b.scrollX)
		}
	}
}

func TestTabOverflowTrailingTitleUsesVisibleCellWidth(t *testing.T) {
	b := fractionalOverflowTabs()
	b.SetActiveTab(0)
	tab := b.tabs[1]
	visible := b.viewportW - (tab.x - b.scrollX)
	if visible <= 0 || visible >= tab.width {
		t.Fatal("fixture has no partial trailing cell")
	}
	theme := Theme()
	available := visible - theme.TabMargin.L - theme.TabMargin.R - theme.TabCloseSize
	if got := theme.Font.TextExtents(tab.paintLabel).Width; got > available+0.001 {
		t.Fatalf("trailing title %q is %v px but only %v px available", tab.paintLabel, got, available)
	}
}

type tabIconRecorder struct {
	*tabPaintRecorder
	icons int
}

func (r *tabIconRecorder) DrawIcon(paint.Icon, float64, bool) { r.icons++ }

func TestTabOverflowTinyCellHasNoClippedCloseTarget(t *testing.T) {
	b := overflowTabs(2)
	b.SetSize(38, 30) // eight pixels remain beside the chooser
	b.Layout()
	b.SetActiveTab(0)
	b.hoverTab = 0
	b.hoverCloseBtn = true
	for x := 0.0; x < b.viewportW; x++ {
		if _, close := b.HitTest(x, 15); close {
			t.Errorf("partial close target at x=%v", x)
		}
	}
	r := &tabIconRecorder{tabPaintRecorder: &tabPaintRecorder{labelPaintRecorder: newLabelPaintRecorder()}}
	b.Draw(r)
	if r.icons != 0 {
		t.Fatalf("tiny cell painted %d clipped close glyphs", r.icons)
	}
}

func TestTabOverflowPresentationHitAndDropAgree(t *testing.T) {
	b := overflowTabs(10)
	b.tabs[1].data.(*tabPageRef).icon = paint.LoadIcon("file")
	b.tabs[3].data.(*tabPageRef).title = "属性🙂很长的标题和中文"
	theme := Theme()
	for _, width := range []float64{0, 1, 28, 38, 46, 60, 83, 120, 183.25, 299.5, 600, 2400} {
		b.SetSize(width, 30)
		b.Layout()
		for _, active := range []int{0, 3, 9, 4, 1} {
			b.SetActiveTab(active)
			if b.firstVisible > b.lastVisible || b.lastVisible > len(b.tabs) {
				t.Fatal("invalid visible range")
			}
			for i := b.firstVisible; i < b.lastVisible; i++ {
				tab := b.tabs[i]
				left := tab.x - b.scrollX
				if left < 0 || tab.paintWidth <= 0 || left+tab.paintWidth > b.viewportW+1e-6 {
					t.Fatalf("width=%v active=%d cell=%d escapes viewport", width, active, i)
				}
				for _, fraction := range []float64{0.25, 0.75} {
					x := left + tab.paintWidth*fraction
					if hit, _ := b.HitTest(x, 15); hit != i {
						t.Fatalf("cell %d hit %d", i, hit)
					}
					want := i
					if fraction > 0.5 {
						want++
					}
					if drop := b.DropIndex(x, 15); drop != want {
						t.Fatalf("cell %d drop=%d want=%d", i, drop, want)
					}
				}
				padding := theme.TabMargin.L + theme.TabMargin.R + theme.TabCloseSize
				if tab.icon != nil {
					padding += theme.TabMargin.L + theme.IconSize
				}
				if got := theme.Font.TextExtents(tab.paintLabel).Width; got > max(0, tab.paintWidth-padding)+1e-6 {
					t.Fatal("visible title exceeds its presentation width")
				}
			}
			if b.viewportW > 0 && (active < b.firstVisible || active >= b.lastVisible || b.tabs[active].paintWidth != b.tabs[active].width) {
				t.Fatalf("active %d not fully visible at width %v", active, width)
			}
		}
	}
}

func TestTabOverflowRemainderIsNotAnInvisibleHitTarget(t *testing.T) {
	b := overflowTabs(4)
	first := b.tabs[0].width
	b.SetSize(first+30+3, 30)
	b.Layout()
	b.SetActiveTab(0)
	if b.lastVisible != 1 {
		t.Fatal("tiny trailing fragment was not suppressed")
	}
	for x := first; x < b.viewportW; x += 0.5 {
		if hit, _ := b.HitTest(x, 15); hit != -1 {
			t.Fatalf("blank remainder hit tab %d", hit)
		}
		if drop := b.DropIndex(x, 15); drop != 1 {
			t.Fatalf("blank remainder drop %d should follow the visible tab", drop)
		}
	}
}

func TestTabOverflowTrailingActivationNeverClosesAnotherTab(t *testing.T) {
	b := fractionalOverflowTabs()
	b.SetActiveTab(0)
	tab := b.tabs[1]
	x := tab.x + tab.paintWidth*0.75
	closed := 0
	b.SetCloseCallback(func(*TabBar, int) bool { closed++; return true })
	b.OnLeftDown(x, 15)
	b.OnLeftUp(x, 15)
	if b.ActiveTab() != 1 || closed != 0 {
		t.Fatal("truncated inactive label click closed a tab")
	}
	if b.tabs[1].paintWidth != b.tabs[1].width {
		t.Fatal("new active tab did not expand")
	}
	b.SetSize(3000, 30)
	b.Layout()
	for _, tab := range b.tabs {
		if tab.paintWidth != tab.width || tab.paintLabel != tab.label {
			t.Fatal("growing left a truncated label cached")
		}
	}
}

func TestTabOverflowNarrowIconAndCloseNeverOverlap(t *testing.T) {
	b := overflowTabs(2)
	icon := paint.LoadIcon("file")
	if icon == nil {
		t.Fatal("missing built-in file icon")
	}
	b.tabs[0].data.(*tabPageRef).icon = icon
	for _, available := range []float64{1, 8, 20, 28, 40, 80} {
		b.SetSize(available+30, 30)
		b.Layout()
		b.SetActiveTab(0)
		b.hoverTab = 0
		r := &tabIconRecorder{tabPaintRecorder: &tabPaintRecorder{labelPaintRecorder: newLabelPaintRecorder()}}
		b.Draw(r)
		theme := Theme()
		want := 0
		if available >= theme.TabMargin.L+theme.IconSize+theme.TabMargin.R {
			want++
		}
		if tabCloseFits(theme, icon, available, 30) {
			want++
		}
		if r.icons != want {
			t.Fatalf("width=%v icons=%d want=%d", available, r.icons, want)
		}
	}
}

func TestTabWidgetTinyLayoutWithinOwner(t *testing.T) {
	tabs := NewTabWidget()
	tabs.AddTab(NewEdit(), "Alpha", nil)
	for _, size := range [][2]float64{{0, 0}, {5, 9}, {40, 20}, {100, 35}, {400, 300}} {
		tabs.SetSize(size[0], size[1])
		tabs.Layout()
		for _, child := range []IWidget{tabs.tabBar, tabs.stack} {
			x, y, w, h := child.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatalf("size=%v child=%T bounds=%v,%v %vx%v", size, child, x, y, w, h)
			}
		}
	}
}

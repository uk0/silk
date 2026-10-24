package gui

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/geom"
)

type pixelMenuTestItem struct {
	Widget
	hints SizeHints
}

func (w *pixelMenuTestItem) SizeHints() SizeHints { return w.hints }

func pixelTestMenu(height float64, count int) *Menu {
	m := NewPopupMenu()
	m.setVisible(true)
	for range count {
		w := &pixelMenuTestItem{hints: SizeHints{Width: 147.375, Height: height, Policy: GrowHorizontal}}
		w.Init(w)
		m.AddWidget(w)
	}
	return m
}

// Use deterministic fractional metrics, not a particular platform's font.
// Native backends convert client dimensions to integer coordinate units;
// less than one lost unit used to reserve two full 18-unit scroll bands.
func TestPopupMenuRoundsNaturalSizeBeforeNativeResize(t *testing.T) {
	for _, height := range []float64{24, 24.01, 24.125, 24.5, 24.975, 27.72491455078125} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			m := pixelTestMenu(height, 8)
			hints := m.SizeHints()
			for range 3 { // re-open must not accumulate a rounding/offset error
				m.fitPopupWorkArea(geom.Rect{Width: 1280, Height: 800})
				// Model the authoritative native resize callback, not just hints.
				m.SetSize(math.Floor(m.Width()), math.Floor(m.Height()))
				m.Layout()
				if m.Width() < hints.Width || m.Height() < hints.Height || m.Width()-hints.Width >= 1 || m.Height()-hints.Height >= 1 {
					t.Fatalf("native size %v x %v clips/inflates natural %+v", m.Width(), m.Height(), hints)
				}
				if m.scroll.band != 0 || m.scroll.offset != 0 {
					t.Fatalf("short menu scrolls: natural=%v native=%v band=%v", hints.Height, m.Height(), m.scroll.band)
				}
				last := m.items[len(m.items)-1]
				if last.Y()+last.Height() > m.ChildViewport().Bottom()+1e-9 || last.Width() < 147.375 {
					t.Fatal("last row or fractional-width content clipped")
				}
				if m.FindWidgetAt(last.X()+2, last.Y()+last.Height()-0.1) != last {
					t.Fatal("bottom of last row not hittable")
				}
				m.OnHide()
			}
		})
	}
}

func TestPopupMenuIntegerSizingPreservesWorkAreaLimit(t *testing.T) {
	m := pixelTestMenu(27.375, 80)
	for _, height := range []float64{240, 240.25, 240.999, 1, 0.5} {
		m.fitPopupWorkArea(geom.Rect{X: -640, Y: 24, Width: 640, Height: height})
		want := math.Max(1, math.Floor(height)) // native APIs require at least one unit
		if m.Height() != want || m.scroll.band == 0 && height > 8 {
			t.Fatalf("real overflow lost its limit/scrolling: height=%v menu=%+v", height, m.Bounds1())
		}
	}
	// Changing monitors back to a large work area removes bands and restores
	// every natural row; rounding does not squeeze rows to hide overflow.
	m.fitPopupWorkArea(geom.Rect{Width: 1280, Height: 10000})
	if m.Height() != math.Ceil(m.SizeHints().Height) || m.scroll.band != 0 {
		t.Fatal("large work area retained scroll bands")
	}
	for _, item := range m.items {
		if item.Height() != item.SizeHints().Height {
			t.Fatal("row height changed")
		}
	}
}

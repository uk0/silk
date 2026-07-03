package gui

import (
	"github.com/uk0/silk/geom"
	"testing"
)

// BenchmarkLayoutDeepHierarchy measures Layout() cost on a deeply nested
// VBox/HBox tree with a Label sibling at each level. Stresses the recursive
// layout pass and SizeHints fan-out through the tree.
func BenchmarkLayoutDeepHierarchy(b *testing.B) {
	root := NewVBox()
	cur := root.Self()
	for i := 0; i < 100; i++ {
		next := NewHBox()
		if c, ok := cur.(interface{ AddWidget(IWidget) }); ok {
			c.AddWidget(next)
			c.AddWidget(NewLabel("test"))
		}
		cur = next.Self()
	}
	root.SetSize(800, 600)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		root.Layout()
	}
}

// BenchmarkSizeHintsDeep measures SizeHints() on a flat VBox holding many
// labels — the hot path for layout invalidation.
func BenchmarkSizeHintsDeep(b *testing.B) {
	root := NewVBox()
	for i := 0; i < 50; i++ {
		root.AddWidget(NewLabel("test"))
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = root.SizeHints()
	}
}

// BenchmarkDirtyRegionUnion measures the cost of accumulating dirty
// rectangles via geom.Rect.UniteCopy. The dirty-region tracking path
// runs on every widget Update — it must stay cheap.
func BenchmarkDirtyRegionUnion(b *testing.B) {
	r := geom.Rect{}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r = r.UniteCopy(geom.Rect{X: float64(i % 100), Y: float64(i % 50),
			Width: 64, Height: 32})
	}
	_ = r
}

// BenchmarkGridLayout measures GridLayout.Layout() on an 8x8 grid. The grid is
// large enough that the original code's 4 internal slices were heap-allocated
// (small-grid stack promotion was masking the cost on 3x3).
func BenchmarkGridLayout(b *testing.B) {
	g := NewGridLayout()
	g.SetSize(800, 600)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			g.AddWidgetSpan(NewLabel("item"), r, c, 1, 1)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Layout()
	}
}

// BenchmarkGridLayoutSizeHints measures the SizeHints() path which also
// uses the gridScratch pool.
func BenchmarkGridLayoutSizeHints(b *testing.B) {
	g := NewGridLayout()
	g.SetSize(800, 600)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			g.AddWidgetSpan(NewLabel("item"), r, c, 1, 1)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.SizeHints()
	}
}

// BenchmarkFormLayout measures FormLayout.Layout() on a 10-row form. Uses
// Label children whose SizeHints() are cached so the measurement reflects
// only the layout's own allocation cost (not its children's).
func BenchmarkFormLayout(b *testing.B) {
	f := NewFormLayout()
	f.SetSize(400, 400)
	for i := 0; i < 10; i++ {
		f.AddRow("Field", NewLabel("value"))
	}
	// Warm up Label hint caches so first iteration doesn't skew alloc count.
	f.Layout()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Layout()
	}
}

// BenchmarkFormLayoutWithEdits is the more realistic case where Edit children
// recompute SizeHints on every call. The layout itself remains alloc-free; any
// allocations reported here come from Edit.SizeHints().
func BenchmarkFormLayoutWithEdits(b *testing.B) {
	f := NewFormLayout()
	f.SetSize(400, 400)
	for i := 0; i < 10; i++ {
		f.AddRow("Field", NewEdit())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Layout()
	}
}

// BenchmarkSplitterLayout measures Splitter.Layout() with 4 panes. After the
// childCount/eachChild refactor, the per-call Children() allocation is gone.
func BenchmarkSplitterLayout(b *testing.B) {
	s := NewSplitter(false)
	s.SetSize(800, 400)
	for i := 0; i < 4; i++ {
		s.AddWidget(NewLabel("pane"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Layout()
	}
}

// BenchmarkFlexWrap measures FlexWrap.Layout() with 30 chip-sized children
// across roughly 4–6 wrapped rows. Uses Tag children for a realistic mix
// (Tag.SizeHints() does TextExtents and allocates).
func BenchmarkFlexWrap(b *testing.B) {
	fw := NewFlexWrap()
	fw.SetSize(300, 200)
	for i := 0; i < 30; i++ {
		fw.AddWidget(NewTag("tag"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.Layout()
	}
}

// BenchmarkFlexWrapPure isolates FlexWrap's own allocation cost by using Label
// children whose SizeHints() are cached after the first call. Any allocs here
// come from FlexWrap itself, not its children.
func BenchmarkFlexWrapPure(b *testing.B) {
	fw := NewFlexWrap()
	fw.SetSize(300, 200)
	for i := 0; i < 30; i++ {
		fw.AddWidget(NewLabel("label"))
	}
	fw.Layout() // warm Label hint caches
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.Layout()
	}
}

// BenchmarkVirtualListSizeHints stresses the per-frame SizeHints() on a list
// claiming one million items. Should be a constant-time, alloc-free probe.
func BenchmarkVirtualListSizeHints(b *testing.B) {
	vl := NewVirtualList()
	vl.SetItemCount(1000000)
	vl.SetSize(300, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vl.SizeHints()
	}
}

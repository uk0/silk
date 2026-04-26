package gui

import (
	"silk/geom"
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

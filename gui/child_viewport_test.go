package gui

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	"math"
	"testing"
)

type viewportTestWidget struct {
	Widget
	viewport geom.Rect
	color    paint.Color
}

func (w *viewportTestWidget) ChildViewport() geom.Rect { return w.viewport }
func (w *viewportTestWidget) Draw(g paint.Painter) {
	g.SetBrush1(w.color)
	g.Rectangle(0, 0, w.Width(), w.Height())
	g.Fill()
}
func newViewportTestWidget(v geom.Rect, c paint.Color) *viewportTestWidget {
	w := &viewportTestWidget{viewport: v, color: c}
	w.Init(w)
	return w
}

func TestChildViewportNestedPaintAndHitTestingAgree(t *testing.T) {
	white := paint.Color{R: 255, G: 255, B: 255, A: 255}
	red := paint.Color{R: 255, A: 255}
	blue := paint.Color{B: 255, A: 255}
	root := newViewportTestWidget(geom.Rect{X: 10, Y: 10, Width: 80, Height: 80}, white)
	root.SetSize(100, 100)
	child := newViewportTestWidget(geom.Rect{X: 20, Y: 20, Width: 40, Height: 40}, red)
	child.SetBounds(0, 0, 100, 100)
	child.SetParent(root)
	leaf := newViewportTestWidget(geom.Rect{}, blue)
	leaf.SetBounds(0, 0, 100, 100)
	leaf.SetParent(child)
	pix := paint.NewPixmap(100, 100)
	defer paint.CloseSurface(pix)
	painter := pix.NewPainter()
	defer paint.ClosePainter(painter)
	DrawWidgetAll(root, painter, 0, 0, 0, 0, 100, 100)
	pix.Flush()
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		x, y   int
		widget IWidget
		color  paint.Color
	}{{5, 5, root, white}, {15, 15, child, red}, {40, 40, leaf, blue}, {65, 65, child, red}, {95, 95, root, white}} {
		if got := paint.ColorModel.Convert(img.At(tc.x, tc.y)).(paint.Color); got != tc.color {
			t.Fatalf("pixel (%d,%d)=%+v want %+v", tc.x, tc.y, got, tc.color)
		}
		if got := root.FindWidgetAt(float64(tc.x), float64(tc.y)); got != tc.widget {
			t.Fatalf("hit test at (%d,%d) disagrees with painting", tc.x, tc.y)
		}
	}
	for _, bad := range []geom.Rect{{}, {Width: -1, Height: 10}, {X: math.NaN(), Width: 20, Height: 20}, {Width: math.Inf(1), Height: 20}} {
		root.viewport = bad
		if widgetChildViewport(root) != (geom.Rect{}) || root.FindWidgetAt(40, 40) != root {
			t.Fatal("empty/invalid viewport admitted children", bad)
		}
	}
}

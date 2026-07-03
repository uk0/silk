// Package bench holds shared painter scenarios that drive both Cairo
// and glui paint.Painter implementations. The benchmarks parameterise
// the painter so the same workload runs against both backends; the
// per-backend bench files (glui_bench_test.go, cairo_bench_test.go)
// supply the painter and call into here.
//
// Each scenario is a func that takes a paint.Painter plus the iteration
// budget and exercises a typical UI workload — repeated rect fills, a
// scrolling list, a small form. Scenarios deliberately avoid leaking
// backend-specific paths (e.g. Renderer.End or pixmap.Flush): the
// per-backend bench wrapper handles that.
package bench

import (
	"github.com/uk0/silk/paint"
)

// RectFill paints n axis-aligned rects in a grid, with alternating
// brush colours. This is the dominant UI primitive — every panel,
// button bg, list row, divider goes through this path.
func RectFill(g paint.Painter, n int) {
	colA := &paint.SolidBrush{Color: paint.Color{R: 128, G: 179, B: 255, A: 255}}
	colB := &paint.SolidBrush{Color: paint.Color{R: 255, G: 153, B: 128, A: 255}}
	for i := 0; i < n; i++ {
		x := float64(i%40) * 48
		y := float64(i/40) * 48
		if i&1 == 0 {
			g.SetBrush(colA)
		} else {
			g.SetBrush(colB)
		}
		g.Rectangle(x, y, 40, 40)
		g.Fill()
	}
}

// RoundedRect paints n rounded rects via four arcs + line segments.
// Rounded rects are common chrome (cards, badges, modal headers); they
// stress the curve-emission path on both backends.
func RoundedRect(g paint.Painter, n int) {
	br := &paint.SolidBrush{Color: paint.Color{R: 230, G: 230, B: 230, A: 255}}
	g.SetBrush(br)
	const r = 8.0
	for i := 0; i < n; i++ {
		x := float64(i%40) * 48
		y := float64(i/40) * 48
		w, h := 40.0, 40.0

		g.MoveTo(x+r, y)
		g.LineTo(x+w-r, y)
		g.Arc(x+w-r, y+r, r, -1.5707963, 0)
		g.LineTo(x+w, y+h-r)
		g.Arc(x+w-r, y+h-r, r, 0, 1.5707963)
		g.LineTo(x+r, y+h)
		g.Arc(x+r, y+h-r, r, 1.5707963, 3.1415926)
		g.LineTo(x, y+r)
		g.Arc(x+r, y+r, r, 3.1415926, 4.712389)
		g.Fill()
	}
}

// LinearGradient paints n axis-aligned rects each filled with a two-
// stop linear gradient. Gradients land on the linear-gradient fast path
// in glui (uniform-based) so this measures the gradient brush
// transition cost.
func LinearGradient(g paint.Painter, n int) {
	for i := 0; i < n; i++ {
		x := float64(i%40) * 48
		y := float64(i/40) * 48
		grad := paint.NewLinearGradient(float32(x), float32(y), float32(x), float32(y+40))
		grad.AddStop(0, paint.Color{R: 51, G: 102, B: 179, A: 255})
		grad.AddStop(1, paint.Color{R: 153, G: 204, B: 255, A: 255})
		g.SetBrush(grad)
		g.Rectangle(x, y, 40, 40)
		g.Fill()
	}
}

// TextPaint draws n short labels at varying positions. This benches the
// text path (font lookup, glyph advance, rect emission) — the second-
// dominant cost in real UIs.
func TextPaint(g paint.Painter, n int) {
	br := &paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	g.SetBrush(br)
	const text = "Item 42"
	for i := 0; i < n; i++ {
		x := float64(i%40) * 80
		y := float64(i/40)*24 + 16
		g.MoveTo(x, y)
		g.DrawText(text)
	}
}

// ScrollingList simulates a single scroll frame of a 1000-row list:
// alternating background, divider, label per row. This is closer to a
// real frame than the single-primitive scenarios.
func ScrollingList(g paint.Painter, rows int) {
	bg := &paint.SolidBrush{Color: paint.Color{R: 255, G: 255, B: 255, A: 255}}
	zebra := &paint.SolidBrush{Color: paint.Color{R: 242, G: 242, B: 242, A: 255}}
	div := &paint.SolidBrush{Color: paint.Color{R: 217, G: 217, B: 217, A: 255}}
	text := &paint.SolidBrush{Color: paint.Color{R: 26, G: 26, B: 26, A: 255}}
	for i := 0; i < rows; i++ {
		y := float64(i) * 28
		if i&1 == 0 {
			g.SetBrush(bg)
		} else {
			g.SetBrush(zebra)
		}
		g.Rectangle(0, y, 800, 28)
		g.Fill()

		g.SetBrush(div)
		g.Rectangle(0, y+27, 800, 1)
		g.Fill()

		g.SetBrush(text)
		g.MoveTo(12, y+18)
		g.DrawText("Row label")
	}
}

// TypicalForm draws a small dialog: 1 background, 1 title, 5 rows of
// (label + edit), 2 buttons. Per-frame cost of a typical settings
// dialog is dominated by this kind of mix.
func TypicalForm(g paint.Painter) {
	const W, H = 400.0, 360.0

	bg := &paint.SolidBrush{Color: paint.Color{R: 247, G: 247, B: 247, A: 255}}
	border := &paint.SolidBrush{Color: paint.Color{R: 179, G: 179, B: 179, A: 255}}
	label := &paint.SolidBrush{Color: paint.Color{R: 51, G: 51, B: 51, A: 255}}
	field := &paint.SolidBrush{Color: paint.Color{R: 255, G: 255, B: 255, A: 255}}
	button := &paint.SolidBrush{Color: paint.Color{R: 102, G: 153, B: 230, A: 255}}

	// Background.
	g.SetBrush(bg)
	g.Rectangle(0, 0, W, H)
	g.Fill()

	// Title.
	g.SetBrush(label)
	g.MoveTo(16, 28)
	g.DrawText("Preferences")

	// 5 rows of label + edit.
	for row := 0; row < 5; row++ {
		y := 60.0 + float64(row)*40

		g.SetBrush(label)
		g.MoveTo(16, y+20)
		g.DrawText("Setting:")

		g.SetBrush(field)
		g.Rectangle(120, y+4, 260, 28)
		g.Fill()
		g.SetBrush(border)
		g.Rectangle(120, y+4, 260, 28)
		g.Stroke()
	}

	// Buttons.
	g.SetBrush(button)
	g.Rectangle(W-180, H-44, 80, 28)
	g.Fill()
	g.Rectangle(W-90, H-44, 80, 28)
	g.Fill()

	g.SetBrush(label)
	g.MoveTo(W-160, H-24)
	g.DrawText("Cancel")
	g.MoveTo(W-70, H-24)
	g.DrawText("OK")
}

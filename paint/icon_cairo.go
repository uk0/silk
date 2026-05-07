//go:build !silk_no_cairo

package paint

import "silk/cairo"

// genMissingSubIcon draws a red-cross "missing" icon at the requested
// size. Used when LoadIcon fails to find a named icon. Cairo build
// uses cairo.Context directly to compose the strokes.
func genMissingSubIcon(size int) Pixmap {
	w := float64(size)
	s := NewPixmap(size, size).(*cairoSurface)
	cc := s.Surface.NewContext()
	lw := 1 + w*0.05
	cc.Rectangle(lw, lw, w-lw*2, w-lw*2)
	cc.SetSourceRGBA(1, 1, 1, 0.5)
	cc.SetOperator(cairo.OPERATOR_SOURCE)
	cc.FillPreserve()
	cc.SetOperator(cairo.OPERATOR_OVER)

	cc.MoveTo(lw, lw)
	cc.LineTo(w-lw, w-lw)
	cc.MoveTo(w-lw, lw)
	cc.LineTo(lw, w-lw)
	cc.SetSourceRGB(1, 0, 0)
	cc.SetLineWidth(lw)
	cc.Stroke()

	return s
}

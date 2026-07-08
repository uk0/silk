package gui

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// GL chart-series overlay — the targeted GPU fast-path.
//
// Silk renders every widget with Cairo into a CPU backbuffer that OpenGL then
// uploads as one texture and blits as a fullscreen quad (see gl_renderer.go and
// window_glfw.go). That makes GL a *presenter*, not a *renderer*. A LineChart in
// GPU mode keeps its frame, grid, axes, labels and legend on the Cairo path (so
// they composite in the texture) but hands its DATA POLYLINES to this overlay,
// which draws them as native GL LINE_STRIPs *after* the blit — the hot,
// frequently-updating geometry skips CPU rasterization entirely.
//
// Coordinates: a chart builds points in its own logical coords; buildChartOverlay
// transforms them through the painter's current matrix, which already folds in
// the HiDPI content scale and every ancestor translation (the backbuffer is
// sized in physical framebuffer pixels and blitted 1:1). The overlay therefore
// carries framebuffer-pixel coordinates with a top-left origin — exactly what
// the blit's ortho projection uses.
//
// Compositing order: popups (combo/menu) are separate WtPopup OS windows with
// their own texture + blit, so they are NOT occluded by this overlay. The only
// residual is an in-window transient drawn over the plot in the same Cairo pass
// (e.g. a drag ghost): it would fall under the GL series for that frame —
// acceptable for the fast-path and rare in practice.

// overlayPolyline is one connected series line in framebuffer pixels.
type overlayPolyline struct {
	rgba  [4]float32
	width float32
	pts   []float32 // x0,y0,x1,y1,… top-left origin, framebuffer px
}

// chartOverlay is a per-frame GL draw request scissored to a plot rect.
type chartOverlay struct {
	clip  geom.Rect // framebuffer px, top-left origin
	lines []overlayPolyline
}

// chartSeriesLine is a series polyline in a widget's local logical coords,
// collected during Draw before it is either Cairo-stroked or GPU-drawn.
type chartSeriesLine struct {
	color paint.Color
	pts   []geom.Vec2
}

// buildChartOverlay maps local-coord series lines through the painter matrix m
// into a framebuffer-pixel chartOverlay clipped to plot (also in local coords).
// lineWidth is in logical units and is scaled by the matrix's X scale. It is
// pure and GL-free so the coordinate math is unit-testable. Lines with fewer
// than two points are dropped.
func buildChartOverlay(m geom.Mat3x2, plot geom.Rect, lines []chartSeriesLine, lineWidth float64) chartOverlay {
	x0, y0 := m.Transform(plot.X, plot.Y)
	x1, y1 := m.Transform(plot.X+plot.Width, plot.Y+plot.Height)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	scale := m.Xx
	if scale < 0 {
		scale = -scale
	}
	ov := chartOverlay{clip: geom.Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}}
	for _, ln := range lines {
		if len(ln.pts) < 2 {
			continue
		}
		pl := overlayPolyline{
			rgba: [4]float32{
				float32(ln.color.R) / 255,
				float32(ln.color.G) / 255,
				float32(ln.color.B) / 255,
				float32(ln.color.A) / 255,
			},
			width: float32(lineWidth * scale),
			pts:   make([]float32, 0, len(ln.pts)*2),
		}
		for _, p := range ln.pts {
			fx, fy := m.Transform(p.X, p.Y)
			pl.pts = append(pl.pts, float32(fx), float32(fy))
		}
		ov.lines = append(ov.lines, pl)
	}
	return ov
}

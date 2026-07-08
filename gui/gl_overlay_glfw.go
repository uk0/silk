//go:build !windows

package gui

import "github.com/go-gl/gl/v2.1/gl"

func (this *Window) supportsGLOverlay() bool { return true }

// enqueueChartOverlay queues one chart's series lines for GPU drawing this
// frame. Called from a widget's Draw (Cairo pass); consumed by drawGLOverlays
// after the texture blit.
func (this *Window) enqueueChartOverlay(o chartOverlay) {
	this.glOverlays = append(this.glOverlays, o)
}

// resetGLOverlays clears the per-frame queue. Called at the top of paint before
// the widget tree draws, so each frame re-enqueues fresh geometry.
func (this *Window) resetGLOverlays() {
	this.glOverlays = this.glOverlays[:0]
}

// drawGLOverlays renders queued chart series as native GL line strips, each
// scissored to its plot rect, directly onto the framebuffer after the Cairo
// texture blit. Runs in the same ortho space the blit set up (framebuffer px,
// top-left origin); the scissor rect is flipped to GL's bottom-left origin.
func (this *Window) drawGLOverlays(fbw, fbh int32) {
	if len(this.glOverlays) == 0 {
		return
	}
	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, float64(fbw), float64(fbh), 0, -1, 1)
	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()

	gl.Disable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Enable(gl.LINE_SMOOTH)
	gl.Hint(gl.LINE_SMOOTH_HINT, gl.NICEST)
	gl.Enable(gl.SCISSOR_TEST)

	for oi := range this.glOverlays {
		ov := &this.glOverlays[oi]
		sw := int32(ov.clip.Width)
		sh := int32(ov.clip.Height)
		if sw <= 0 || sh <= 0 {
			continue
		}
		sx := int32(ov.clip.X)
		sy := fbh - (int32(ov.clip.Y) + sh) // GL scissor origin is bottom-left
		gl.Scissor(sx, sy, sw, sh)
		for li := range ov.lines {
			pl := &ov.lines[li]
			if len(pl.pts) < 4 {
				continue
			}
			w := pl.width
			if w < 1 {
				w = 1
			}
			gl.LineWidth(w)
			gl.Color4f(pl.rgba[0], pl.rgba[1], pl.rgba[2], pl.rgba[3])
			gl.Begin(gl.LINE_STRIP)
			for i := 0; i+1 < len(pl.pts); i += 2 {
				gl.Vertex2f(pl.pts[i], pl.pts[i+1])
			}
			gl.End()
		}
	}

	gl.Disable(gl.SCISSOR_TEST)
	gl.Disable(gl.LINE_SMOOTH)
	gl.Color4f(1, 1, 1, 1)
}

//go:build !windows

package gui

import (
	"github.com/go-gl/gl/v2.1/gl"
)

func initGL() {
	gl.Enable(gl.TEXTURE_2D)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.LIGHTING)
	gl.Disable(gl.SCISSOR_TEST)
	gl.Disable(gl.STENCIL_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
}

func drawFullscreenQuad(texture uint32, fbWidth, fbHeight int32) {
	drawFullscreenQuadUV(texture, fbWidth, fbHeight, 1.0, 1.0)
}

// drawFullscreenQuadUV renders a textured quad covering the viewport.
// texU/texV specify the upper texture-coordinate bounds, allowing the
// caller to display only a sub-region of a texture that is larger than
// the viewport (e.g. when the backbuffer was allocated with padding).
func drawFullscreenQuadUV(texture uint32, fbWidth, fbHeight int32, texU, texV float32) {
	gl.ClearColor(0.2, 0.2, 0.2, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, float64(fbWidth), float64(fbHeight), 0, -1, 1)

	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()

	gl.Enable(gl.TEXTURE_2D)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	gl.Color4f(1, 1, 1, 1)
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(0, 0)
	gl.Vertex2f(0, 0)

	gl.TexCoord2f(texU, 0)
	gl.Vertex2f(float32(fbWidth), 0)

	gl.TexCoord2f(texU, texV)
	gl.Vertex2f(float32(fbWidth), float32(fbHeight))

	gl.TexCoord2f(0, texV)
	gl.Vertex2f(0, float32(fbHeight))
	gl.End()

	gl.Disable(gl.TEXTURE_2D)
}

//go:build !windows

package gui

import (
	"silk/core"
	"unsafe"

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

// uploadTextureViaPBO streams a BGRA pixel buffer into the bound texture
// using a Pixel Buffer Object. The classic technique:
//
//  1. BindBuffer + BufferData(...,nil,STREAM_DRAW) "orphans" the PBO,
//     letting the driver hand back fresh memory while the previous
//     buffer is still in flight on the GPU.
//  2. MapBuffer returns a writable mapping. We memcpy our pixels in.
//  3. UnmapBuffer hands the buffer back to the GL.
//  4. TexSubImage2D with nil data + a bound PIXEL_UNPACK_BUFFER reads
//     from the PBO instead of host memory — the actual DMA can run
//     async with the rest of the frame.
//
// On failure (MapBuffer returns nil — older drivers, OpenGL ES profile,
// etc.) we mark PBO unavailable on the window and signal the caller to
// fall back to plain glTexImage2D. Subsequent frames skip PBO entirely.
//
// rowStride is the byte stride of the source data; mapped destination
// is tightly packed at width*4 bytes.
func (this *Window) uploadTextureViaPBO(width, height int32, rowStride int, data unsafe.Pointer) bool {
	size := int(width) * int(height) * 4
	if size <= 0 || data == nil {
		return false
	}

	if this.pbo == 0 {
		gl.GenBuffers(1, &this.pbo)
		if this.pbo == 0 {
			this.pboAvailable = false
			return false
		}
	}

	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, this.pbo)
	// Orphan + (re)allocate. Passing nil for the data pointer tells the
	// driver "don't copy anything", giving us a fresh buffer of `size`.
	gl.BufferData(gl.PIXEL_UNPACK_BUFFER, size, nil, gl.STREAM_DRAW)
	this.pboSize = size

	ptr := gl.MapBuffer(gl.PIXEL_UNPACK_BUFFER, gl.WRITE_ONLY)
	if ptr == nil {
		// Driver returned no mapping; abort PBO path and fall back.
		gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
		core.Warn("MapBuffer returned nil; disabling PBO uploads for this window")
		this.pboAvailable = false
		return false
	}

	// memcpy width*4 bytes per row from data (with rowStride) into the
	// mapping (tightly packed at width*4). When rowStride == width*4 the
	// whole upload is a single memmove; otherwise we copy row-by-row.
	dstStride := int(width) * 4
	if rowStride == dstStride {
		copyBytes(ptr, data, size)
	} else {
		for y := int32(0); y < height; y++ {
			src := unsafe.Pointer(uintptr(data) + uintptr(int(y)*rowStride))
			dst := unsafe.Pointer(uintptr(ptr) + uintptr(int(y)*dstStride))
			copyBytes(dst, src, dstStride)
		}
	}

	if !gl.UnmapBuffer(gl.PIXEL_UNPACK_BUFFER) {
		// Buffer was corrupted (rare; usually means another context did
		// something unexpected). Reupload from CPU memory.
		gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
		core.Warn("UnmapBuffer signalled corruption; falling back this frame")
		return false
	}

	// Make sure the texture storage is sized to (width, height). When the
	// dimensions change we need a fresh TexImage2D allocation; otherwise
	// TexSubImage2D streams into the existing storage.
	if this.pboTexW != width || this.pboTexH != height {
		gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0,
			gl.BGRA, gl.UNSIGNED_BYTE, nil)
		this.pboTexW = width
		this.pboTexH = height
	}

	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, width, height,
		gl.BGRA, gl.UNSIGNED_BYTE, gl.PtrOffset(0))

	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	return true
}

// copyBytes is a typed wrapper around the standard memcpy idiom for raw
// pointers. We use unsafe.Slice + copy so the runtime can pick the most
// efficient implementation (SSE/NEON memmove on supported platforms).
func copyBytes(dst, src unsafe.Pointer, n int) {
	if n <= 0 {
		return
	}
	d := unsafe.Slice((*byte)(dst), n)
	s := unsafe.Slice((*byte)(src), n)
	copy(d, s)
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

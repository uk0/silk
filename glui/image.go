package glui

import (
	"image"

	"github.com/go-gl/gl/v2.1/gl"
)

// Texture is a GPU texture handle wrapping an OpenGL 2D texture.
// Construct via Context.UploadTexture; release via Texture.Free.
type Texture struct {
	id     uint32
	width  int
	height int
}

// ID returns the underlying GL texture ID.
func (t *Texture) ID() uint32 { return t.id }

// Width returns the texture width in pixels.
func (t *Texture) Width() int { return t.width }

// Height returns the texture height in pixels.
func (t *Texture) Height() int { return t.height }

// Free releases the texture. Subsequent calls are no-ops.
func (t *Texture) Free() {
	if t.id != 0 {
		gl.DeleteTextures(1, &t.id)
		t.id = 0
	}
}

// UploadTexture creates a GPU texture from an image. The image is converted
// to RGBA8 if needed. Linear filtering, clamp-to-edge wrap.
func (c *Context) UploadTexture(img image.Image) *Texture {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	rgba, ok := img.(*image.RGBA)
	if !ok || rgba.Stride != w*4 {
		rgba = image.NewRGBA(bounds)
		// Copy pixel by pixel — not fast but correct for any input format.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				rgba.Set(x, y, img.At(x+bounds.Min.X, y+bounds.Min.Y))
			}
		}
	}

	var id uint32
	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(w), int32(h), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba.Pix))

	return &Texture{id: id, width: w, height: h}
}

// DrawImage paints a texture inside dst (logical points). The full texture
// is sampled (UV 0..1).
func (r *Renderer) DrawImage(tex *Texture, dst Rect, tint Color) {
	if tex == nil || tex.id == 0 {
		return
	}
	r.setBatch(kindImage, tex.id)
	base := uint16(len(r.verts))

	p0x, p0y := r.project(dst.X, dst.Y)
	p1x, p1y := r.project(dst.X+dst.W, dst.Y)
	p2x, p2y := r.project(dst.X+dst.W, dst.Y+dst.H)
	p3x, p3y := r.project(dst.X, dst.Y+dst.H)

	r.verts = append(r.verts,
		vertex{p0x, p0y, 0, 0, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p1x, p1y, 1, 0, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p2x, p2y, 1, 1, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p3x, p3y, 0, 1, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
	)
	r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
}

// DrawImageRegion paints part of a texture into dst. src is in pixels.
func (r *Renderer) DrawImageRegion(tex *Texture, src, dst Rect, tint Color) {
	if tex == nil || tex.id == 0 || tex.width == 0 || tex.height == 0 {
		return
	}
	r.setBatch(kindImage, tex.id)
	base := uint16(len(r.verts))

	fw := float32(tex.width)
	fh := float32(tex.height)
	u0 := src.X / fw
	v0 := src.Y / fh
	u1 := (src.X + src.W) / fw
	v1 := (src.Y + src.H) / fh

	p0x, p0y := r.project(dst.X, dst.Y)
	p1x, p1y := r.project(dst.X+dst.W, dst.Y)
	p2x, p2y := r.project(dst.X+dst.W, dst.Y+dst.H)
	p3x, p3y := r.project(dst.X, dst.Y+dst.H)

	r.verts = append(r.verts,
		vertex{p0x, p0y, u0, v0, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p1x, p1y, u1, v0, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p2x, p2y, u1, v1, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
		vertex{p3x, p3y, u0, v1, tint.R, tint.G, tint.B, tint.A, 0, 0, 0, 0},
	)
	r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
}

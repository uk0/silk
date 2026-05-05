package glui

import (
	"hash/fnv"
	"math"

	"github.com/go-gl/gl/v2.1/gl"
)

// GradientStop describes a single colour stop on a multi-stop gradient.
// Position is in [0, 1]; Color is in linear glui Color (RGBA, 0..1).
//
// Mirrors paint.GradientStop's semantics so CairoCompat can route stops
// through one-to-one without translation. Lives here (not types.go) because
// it's only meaningful in the gradient pipeline — keeping the types
// colocated reduces the surface area visible to non-gradient callers.
type GradientStop struct {
	Position float32
	Color    Color
}

// gradientRampSize is the resolution of the CPU-built ramp uploaded as a
// 1-D texture. 256 texels gives one byte of precision per channel, which
// matches the 8-bit framebuffer's discrete output — going wider would only
// help for HDR pipelines we don't support today.
const gradientRampSize = 256

// FillRadialGradientRect paints rc with a radial gradient centred at
// (cx, cy) — given in the same logical coordinate space as rc — fading
// from radius r0 (innermost stop) to r1 (outermost stop). Stops define
// the colour ramp sampled by the fragment shader; the same 256×1 ramp
// texture cache as FillMultiGradientRect is reused, so two rects with
// identical stop lists share the GL texture.
//
// The vertex shader receives per-corner offsets (worldX-cx, worldY-cy)
// in v_uv. GL's barycentric interpolation linearly fills the rect with
// position offsets, so length(v_uv) at any fragment is the true distance
// from the centre — no projection-aware adjustment needed beyond the
// usual NDC project() call on a_pos.
//
// Degenerate cases:
//   - empty stop list: no-op (matches Cairo behaviour).
//   - single stop: solid fill of that colour.
//   - r1 <= r0: shader collapses to a hard step at r0; the outer-most
//     stop fills the rect minus a tiny disc at the centre.
//
// Like FillGradientRect, a change in r0/r1 forces a flush before the new
// quad joins the batch — uniforms are per-program global, so two rects
// with different radii cannot share a draw call. Same-radii rects with
// the same texture (i.e. matching stop hash) batch into one DrawElements.
func (r *Renderer) FillRadialGradientRect(rc Rect, cx, cy, r0, r1 float32, stops []GradientStop) {
	if len(stops) == 0 {
		return
	}
	if len(stops) == 1 {
		r.FillRect(rc, stops[0].Color)
		return
	}

	// Resolve the ramp texture. Off-GL (test) renderers leave ctx==nil; we
	// still emit vertices so state-transition tests can observe the path.
	tex := uint32(0)
	if r.ctx != nil {
		t := r.ctx.uploadGradientRamp(stops)
		if t != nil {
			tex = t.id
		}
	}

	// A new radii pair must flush even when kind/tex are unchanged because
	// u_radii is a uniform-global on the program. setBatch alone wouldn't
	// notice the change.
	if r.curKind != kindGradientRadial || r.curTex != tex || r.radR0 != r0 || r.radR1 != r1 {
		r.flush()
		r.curKind = kindGradientRadial
		r.curTex = tex
		r.radR0 = r0
		r.radR1 = r1
	}

	base := uint16(len(r.verts))
	x0, y0 := r.project(rc.X, rc.Y)
	x1, y1 := r.project(rc.X+rc.W, rc.Y)
	x2, y2 := r.project(rc.X+rc.W, rc.Y+rc.H)
	x3, y3 := r.project(rc.X, rc.Y+rc.H)

	// Pack each corner's offset from the gradient centre into v_uv. The
	// fragment shader takes length(v_uv) per pixel.
	u0, v0 := rc.X-cx, rc.Y-cy
	u1, v1 := rc.X+rc.W-cx, rc.Y-cy
	u2, v2 := rc.X+rc.W-cx, rc.Y+rc.H-cy
	u3, v3 := rc.X-cx, rc.Y+rc.H-cy

	r.verts = append(r.verts,
		vertex{x0, y0, u0, v0, 1, 1, 1, 1, 0, 0, 0, 0},
		vertex{x1, y1, u1, v1, 1, 1, 1, 1, 0, 0, 0, 0},
		vertex{x2, y2, u2, v2, 1, 1, 1, 1, 0, 0, 0, 0},
		vertex{x3, y3, u3, v3, 1, 1, 1, 1, 0, 0, 0, 0},
	)
	r.indices = append(r.indices,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}

// FillMultiGradientRect paints rc with a linear gradient defined by an
// arbitrary number of colour stops. The gradient axis runs left-to-right
// when vertical is false, top-to-bottom when true.
//
// Stops are sampled via a 256-pixel CPU-built ramp uploaded to a 1-D
// texture, cached on Context for reuse across frames. The pipeline:
//
//  1. Build a 256-byte RGBA ramp by walking each stop pair and linearly
//     interpolating colour between them.
//  2. Hash the stop signature (position+colour bytes); if the cache holds a
//     matching texture, reuse it.
//  3. Otherwise upload a fresh 256×1 RGBA texture, bind it to a
//     kindGradientRamp batch keyed on its GL ID, and emit the quad.
//
// Multi-stop gradients now render every stop. Two-stop callers should still
// prefer FillGradientRect — it skips the texture round-trip and runs from
// uniforms.
func (r *Renderer) FillMultiGradientRect(rc Rect, stops []GradientStop, vertical bool) {
	if len(stops) == 0 {
		return
	}
	if len(stops) == 1 {
		r.FillRect(rc, stops[0].Color)
		return
	}

	// Off-GL test renderers (newAdapterTestRenderer / newTestRenderer) leave
	// ctx == nil. Building the ramp is still useful for state-transition
	// tests, but the upload would crash on a real GL call. We branch here:
	// with a real ctx we cache + upload; without one we still set the batch
	// kind and emit vertices so test code can observe the path.
	tex, hash := uint32(0), uint64(0)
	if r.ctx != nil {
		t := r.ctx.uploadGradientRamp(stops)
		if t != nil {
			tex = t.id
		}
		hash = stopsHash(stops)
	} else {
		hash = stopsHash(stops)
	}
	_ = hash // hash currently flows through the cache only, not into the batch key

	// Each unique ramp texture is its own batch. setBatch flushes when the
	// (kind, tex) pair changes, mirroring the existing image-quad path. This
	// is the simplest correct policy: when a frame has many distinct
	// gradients each gets its own draw call, but rows of identical buttons
	// (same stops → same texture ID after dedupe) batch together.
	r.setBatch(kindGradientRamp, tex)

	base := uint16(len(r.verts))
	x0, y0 := r.project(rc.X, rc.Y)
	x1, y1 := r.project(rc.X+rc.W, rc.Y)
	x2, y2 := r.project(rc.X+rc.W, rc.Y+rc.H)
	x3, y3 := r.project(rc.X, rc.Y+rc.H)

	if vertical {
		// t = 0 at top vertices, 1 at bottom. Pack into v_uv.x to match the
		// shader's hard-coded sample axis.
		r.verts = append(r.verts,
			vertex{x0, y0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x1, y1, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x2, y2, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x3, y3, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
		)
	} else {
		r.verts = append(r.verts,
			vertex{x0, y0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x1, y1, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x2, y2, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x3, y3, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
		)
	}
	r.indices = append(r.indices,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}

// buildGradientRamp expands the stops into a tight 256×1 RGBA byte buffer.
// Stops are assumed sorted by Position; if not, the last-pair-wins logic
// will still produce a deterministic ramp but the visual result may be
// surprising. Returning a heap slice (not a fixed-size array) lets the
// upload helper hand the bytes straight to gl.TexImage2D.
//
// Algorithm: for each output texel t in [0,1], find the stop pair (s0, s1)
// that brackets it. If t falls outside the stop range we clamp to the
// extremes — matches Cairo's PAD extend mode, the default for paint.LinearGradient.
func buildGradientRamp(stops []GradientStop) []byte {
	out := make([]byte, gradientRampSize*4)
	if len(stops) == 0 {
		return out
	}
	if len(stops) == 1 {
		c := stops[0].Color
		for i := 0; i < gradientRampSize; i++ {
			out[i*4+0] = floatToByte(c.R)
			out[i*4+1] = floatToByte(c.G)
			out[i*4+2] = floatToByte(c.B)
			out[i*4+3] = floatToByte(c.A)
		}
		return out
	}

	for i := 0; i < gradientRampSize; i++ {
		t := float32(i) / float32(gradientRampSize-1)

		// Locate the bracketing stop pair. Default to the extremes so out-
		// of-range t still reads a defined colour.
		s0 := stops[0]
		s1 := stops[len(stops)-1]
		for j := 0; j+1 < len(stops); j++ {
			if t >= stops[j].Position && t <= stops[j+1].Position {
				s0, s1 = stops[j], stops[j+1]
				break
			}
		}

		var lt float32
		if s1.Position == s0.Position {
			lt = 0
		} else {
			lt = (t - s0.Position) / (s1.Position - s0.Position)
		}
		if lt < 0 {
			lt = 0
		}
		if lt > 1 {
			lt = 1
		}

		col := Color{
			R: s0.Color.R + (s1.Color.R-s0.Color.R)*lt,
			G: s0.Color.G + (s1.Color.G-s0.Color.G)*lt,
			B: s0.Color.B + (s1.Color.B-s0.Color.B)*lt,
			A: s0.Color.A + (s1.Color.A-s0.Color.A)*lt,
		}
		out[i*4+0] = floatToByte(col.R)
		out[i*4+1] = floatToByte(col.G)
		out[i*4+2] = floatToByte(col.B)
		out[i*4+3] = floatToByte(col.A)
	}
	return out
}

// floatToByte converts a 0..1 float to a 0..255 byte with rounding and
// saturation. Defensive against NaN and out-of-range inputs that can sneak
// in from poorly-defined widget palettes.
func floatToByte(v float32) byte {
	if math.IsNaN(float64(v)) {
		return 0
	}
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return byte(v*255 + 0.5)
}

// stopsHash produces a 64-bit fingerprint of a stop list. We hash each
// stop's position + four colour channels, projecting floats into their
// IEEE-754 bit pattern so two visually-identical stop arrays from different
// callers map to the same key.
func stopsHash(stops []GradientStop) uint64 {
	h := fnv.New64a()
	var buf [4]byte
	for _, s := range stops {
		writeFloat32(&buf, s.Position)
		h.Write(buf[:])
		writeFloat32(&buf, s.Color.R)
		h.Write(buf[:])
		writeFloat32(&buf, s.Color.G)
		h.Write(buf[:])
		writeFloat32(&buf, s.Color.B)
		h.Write(buf[:])
		writeFloat32(&buf, s.Color.A)
		h.Write(buf[:])
	}
	return h.Sum64()
}

func writeFloat32(buf *[4]byte, v float32) {
	bits := math.Float32bits(v)
	buf[0] = byte(bits)
	buf[1] = byte(bits >> 8)
	buf[2] = byte(bits >> 16)
	buf[3] = byte(bits >> 24)
}

// uploadGradientRamp returns a cached 256×1 RGBA texture for the given
// stops, uploading on first sight. Returns nil when GL state is unavailable
// (e.g. a Context that hasn't been Init'd yet — happens in unit tests where
// we hand-build a Renderer with ctx=nil bypassing this code path entirely).
//
// The cache is keyed on a hash of the full stop list. Two distinct stop
// lists with identical floats produce the same hash and reuse one texture;
// any change in any field (position or any colour channel) invalidates the
// match and triggers a fresh upload.
func (c *Context) uploadGradientRamp(stops []GradientStop) *Texture {
	if c == nil {
		return nil
	}
	if !c.initialized {
		// Building a ramp without a real GL context would crash inside
		// gl.GenTextures. Return nil so callers know the upload skipped;
		// tests typically run without Init() and take the off-GL branch.
		return nil
	}
	if c.gradientRamps == nil {
		c.gradientRamps = make(map[uint64]*Texture)
	}
	key := stopsHash(stops)
	if tex, ok := c.gradientRamps[key]; ok {
		return tex
	}
	data := buildGradientRamp(stops)
	tex := c.uploadGradientRampTexture(data)
	if tex != nil {
		c.gradientRamps[key] = tex
	}
	return tex
}

// uploadGradientRampTexture allocates a GL texture for a 256×1 RGBA ramp.
// Linear filtering on S keeps the gradient smooth between texels; the
// single-row T axis is irrelevant but we set CLAMP_TO_EDGE to be safe.
//
// Lives separately from UploadTextureBGRA because gradient ramps want
// different filter parameters: image textures clamp the V axis but keep
// linear S filtering to interpolate fragments — exactly what we need here
// — while the data layout (RGBA, no premultiplication) is simpler than the
// pixmap pipeline.
func (c *Context) uploadGradientRampTexture(rgba []byte) *Texture {
	if len(rgba) < gradientRampSize*4 {
		return nil
	}
	var id uint32
	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(gradientRampSize), 1, 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba))
	return &Texture{id: id, width: gradientRampSize, height: 1}
}

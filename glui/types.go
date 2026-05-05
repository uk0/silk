package glui

// Point is a 2D position in logical units (points).
type Point struct{ X, Y float32 }

// Rect is an axis-aligned rectangle: (X, Y) is top-left, (W, H) is size.
type Rect struct{ X, Y, W, H float32 }

// Color is RGBA with each channel in [0, 1].
type Color struct{ R, G, B, A float32 }

// RGBA8 builds a Color from 8-bit channels.
func RGBA8(r, g, b, a uint8) Color {
	return Color{
		R: float32(r) / 255,
		G: float32(g) / 255,
		B: float32(b) / 255,
		A: float32(a) / 255,
	}
}

// vertex is the shared vertex layout used by every shader: 48 bytes,
// 12-float layout (pos.xy, uv.xy, color.rgba, corner.xyzw). The renderer
// never emits any other vertex format, so we can leave VAO state bound
// across draws.
//
// The trailing 4 floats hold rect-shader SDF parameters per vertex:
//   CornerHX, CornerHY — half-size of the rect in points
//   CornerR            — corner radius in points
//   CornerAA           — anti-aliasing width in points
//
// Path/Glyph/Image shaders ignore these — their attribute is simply not
// enabled at flush time. Carrying the same stride for every kind keeps a
// single VBO layout and lets us interleave rect quads with other geometry
// in the same buffer without reconfiguring vertex pulls.
type vertex struct {
	X, Y float32 // position (clip space)
	U, V float32 // uv / rect-centered point coordinates
	R, G, B, A float32 // color
	CornerHX, CornerHY, CornerR, CornerAA float32 // rect SDF data
}

// vertexSize is the byte size of one vertex.
const vertexSize = 48

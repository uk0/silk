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

// vertex is the shared vertex layout used by every shader: 32 bytes,
// 8-float layout (pos.xy, uv.xy, color.rgba). The renderer never emits
// any other vertex format so we can leave VAO state bound across draws.
type vertex struct {
	X, Y float32 // position (clip space)
	U, V float32 // uv / corner-space coordinates
	R, G, B, A float32 // color
}

// vertexSize is the byte size of one vertex.
const vertexSize = 32

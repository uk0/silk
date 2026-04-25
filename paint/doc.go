// Package paint provides 2D drawing primitives built on Cairo.
//
// It wraps the Cairo graphics library with a Go-friendly API for
// drawing shapes, text, images, and managing colors, fonts, and icons.
//
// Key types:
//   - Painter: the main drawing interface (lines, rectangles, arcs, text)
//   - Color: RGBA color with named color parsing
//   - Font: font family, size, bold/italic
//   - Icon: multi-resolution icon with automatic size selection
//   - Pixmap: off-screen image surface for rendering
//   - Surface: Cairo surface abstraction
package paint

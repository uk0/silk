package glui

import "math"

// matrix3 is a column-major 3x3 affine matrix. Only six floats are
// meaningful — the implicit last row is (0, 0, 1) — but we store all nine
// to make multiplication branch-free and easy to inline. Layout:
//
//	[ m[0] m[3] m[6] ]   [ a c tx ]
//	[ m[1] m[4] m[7] ] = [ b d ty ]
//	[ m[2] m[5] m[8] ]   [ 0 0  1 ]
type matrix3 [9]float32

// identityMatrix3 returns the identity transform.
func identityMatrix3() matrix3 {
	return matrix3{
		1, 0, 0,
		0, 1, 0,
		0, 0, 1,
	}
}

// mulMatrix3 returns a * b.
//
// Following the OpenGL post-multiplication convention used by Cairo: an
// existing transform `a` is composed with a new local transform `b`, so
// applying the result to a vector p computes a*(b*p) — i.e. `b` is applied
// first in user-space. This is what makes Translate/Rotate stack the way
// callers expect.
func mulMatrix3(a, b matrix3) matrix3 {
	return matrix3{
		// Column 0
		a[0]*b[0] + a[3]*b[1] + a[6]*b[2],
		a[1]*b[0] + a[4]*b[1] + a[7]*b[2],
		a[2]*b[0] + a[5]*b[1] + a[8]*b[2],
		// Column 1
		a[0]*b[3] + a[3]*b[4] + a[6]*b[5],
		a[1]*b[3] + a[4]*b[4] + a[7]*b[5],
		a[2]*b[3] + a[5]*b[4] + a[8]*b[5],
		// Column 2
		a[0]*b[6] + a[3]*b[7] + a[6]*b[8],
		a[1]*b[6] + a[4]*b[7] + a[7]*b[8],
		a[2]*b[6] + a[5]*b[7] + a[8]*b[8],
	}
}

// Save pushes the current transform onto the stack so a subsequent Restore
// can return to it. Save/Restore pair without bound — every Save MUST be
// matched by a Restore at the same nesting level.
func (r *Renderer) Save() {
	r.xstack = append(r.xstack, r.xform)
}

// Restore pops the transform stack. Unbalanced Restore (more pops than
// pushes) is a no-op so a buggy caller can't corrupt the matrix state.
func (r *Renderer) Restore() {
	n := len(r.xstack)
	if n == 0 {
		return
	}
	r.xform = r.xstack[n-1]
	r.xstack = r.xstack[:n-1]
}

// Translate post-multiplies the current transform by a translation. The new
// origin moves to (tx, ty) in the previous coordinate system.
func (r *Renderer) Translate(tx, ty float32) {
	r.xform = mulMatrix3(r.xform, matrix3{
		1, 0, 0,
		0, 1, 0,
		tx, ty, 1,
	})
}

// Scale post-multiplies the current transform by a non-uniform scale.
func (r *Renderer) Scale(sx, sy float32) {
	r.xform = mulMatrix3(r.xform, matrix3{
		sx, 0, 0,
		0, sy, 0,
		0, 0, 1,
	})
}

// Rotate post-multiplies the current transform by a rotation about the
// current origin. Positive angles rotate clockwise in screen space (the
// renderer uses Y-down logical coordinates).
func (r *Renderer) Rotate(radians float32) {
	s, c := math.Sincos(float64(radians))
	cf, sf := float32(c), float32(s)
	r.xform = mulMatrix3(r.xform, matrix3{
		cf, sf, 0,
		-sf, cf, 0,
		0, 0, 1,
	})
}

// CurrentTransform returns the current modelview matrix. Useful for tests
// and for adapter layers that need to read back the transform.
func (r *Renderer) CurrentTransform() [9]float32 {
	return [9]float32(r.xform)
}

// applyXform returns the point (x, y) transformed by the current modelview
// matrix. Exposed for tests; production code uses project() which composes
// transform + clip-space conversion.
func (r *Renderer) applyXform(x, y float32) (tx, ty float32) {
	tx = r.xform[0]*x + r.xform[3]*y + r.xform[6]
	ty = r.xform[1]*x + r.xform[4]*y + r.xform[7]
	return
}

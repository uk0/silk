package scene3d

import "math"

// Frustum is an immutable, conservative mesh-bounds test for OpenGL-style clip
// coordinates (-w <= x,y,z <= w). It can be shared by independent render owners.
// It is not an occlusion test, an exact mesh intersection, or a resource budget.
type Frustum struct {
	planes [6][4]float64
	absVP  Mat4
	usable bool
}

// NewFrustum prepares a view-projection matrix once per frame. A zero or
// non-finite/out-of-float32-range matrix produces a fail-open test: geometry is retained,
// not hidden. Call Camera.ViewProjection to validate a camera; this optimization
// deliberately does not replace renderer validation or triangle clipping.
func NewFrustum(viewProjection Mat4) Frustum {
	if !viewProjection.valid() {
		return Frustum{}
	}
	f := Frustum{usable: true}
	for i, x := range viewProjection {
		if math.Abs(x) > math.MaxFloat32 {
			return Frustum{}
		}
		f.absVP[i] = math.Abs(x)
	}
	for axis := 0; axis < 3; axis++ {
		for side, sign := range [2]float64{1, -1} {
			for column := 0; column < 4; column++ {
				x := viewProjection[column*4+3] + sign*viewProjection[column*4+axis]
				if !finite(x) {
					return Frustum{}
				}
				f.planes[axis*2+side][column] = x
			}
		}
	}
	return f
}

// Intersects reports whether an affine-transformed mesh might intersect the
// frustum. It only rejects a box wholly outside one of the six clip planes;
// crossing the near plane, enclosing the camera, shear, reflections and zero
// scales are supported. It allocates nothing and never traverses mesh vertices.
//
// The tolerance is based on absolute intermediate magnitudes, not the final
// (possibly cancelled) coordinates. It leaves slack for float32 shader inputs
// and arithmetic as well as the float64 CPU path. Uncertain bounds or values
// outside float32 arithmetic range fail open; renderer validation still runs.
// A nil or empty mesh contains no triangles and returns false.
func (f *Frustum) Intersects(mesh *Mesh, world Mat4) bool {
	if mesh == nil || len(mesh.indices) == 0 {
		return false
	}
	if f == nil || !f.usable || !world.affine() || !mesh.min.valid() || !mesh.max.valid() {
		return true
	}
	for _, x := range world {
		if math.Abs(x) > math.MaxFloat32 {
			return true
		}
	}
	lo, hi := mesh.min, mesh.max
	if lo.X > hi.X || lo.Y > hi.Y || lo.Z > hi.Z {
		return true
	}
	x := math.Max(math.Abs(lo.X), math.Abs(hi.X))
	y := math.Max(math.Abs(lo.Y), math.Abs(hi.Y))
	z := math.Max(math.Abs(lo.Z), math.Abs(hi.Z))
	var worldMagnitude [3]float64
	for row := range worldMagnitude {
		v := math.Abs(world[row])*x + math.Abs(world[4+row])*y + math.Abs(world[8+row])*z + math.Abs(world[12+row])
		if !finite(v) || v > math.MaxFloat32 {
			return true
		}
		worldMagnitude[row] = v
	}
	// Check every clip coordinate before rejecting on any one plane. Otherwise
	// culling an offscreen X could conceal overflow in Z that rendering rejects.
	var clipMagnitude [4]float64
	for row := range clipMagnitude {
		v := f.absVP[row]*worldMagnitude[0] + f.absVP[4+row]*worldMagnitude[1] + f.absVP[8+row]*worldMagnitude[2] + f.absVP[12+row]
		if !finite(v) || v > math.MaxFloat32 {
			return true
		}
		clipMagnitude[row] = v
	}
	for i, p := range f.planes {
		// Transform the plane into mesh coordinates; choose its maximum-support
		// corner without inflating the AABB into a world-aligned box.
		a := p[0]*world[0] + p[1]*world[1] + p[2]*world[2]
		b := p[0]*world[4] + p[1]*world[5] + p[2]*world[6]
		c := p[0]*world[8] + p[1]*world[9] + p[2]*world[10]
		d := p[0]*world[12] + p[1]*world[13] + p[2]*world[14] + p[3]
		corner := lo
		if a > 0 {
			corner.X = hi.X
		}
		if b > 0 {
			corner.Y = hi.Y
		}
		if c > 0 {
			corner.Z = hi.Z
		}
		distance := a*corner.X + b*corner.Y + c*corner.Z + d
		// 128 float32 epsilons plus an absolute floor conservatively retain
		// borderline/denormal geometry instead of creating edge popping.
		tolerance := (clipMagnitude[3]+clipMagnitude[i/2])/65536 + 1e-8
		if !finite(distance) || !finite(tolerance) {
			return true
		}
		if distance < -tolerance {
			return false
		}
	}
	return true
}

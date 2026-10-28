// Package scene3d provides platform-independent scene data and a deterministic
// software reference renderer. Scenes/renderers are single-owner; immutable
// meshes and snapshots can be shared with other goroutines.
package scene3d

import "math"

type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3    { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3    { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Mul(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a Vec3) Length() float64 { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Unit() Vec3 {
	n := a.Length()
	if n == 0 || !finite(n) {
		return Vec3{}
	}
	return a.Mul(1 / n)
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (a Vec3) valid() bool  { return finite(a.X) && finite(a.Y) && finite(a.Z) }

// Mat4 is column-major; Mul composes transforms as a*b, and vectors are columns.
type Mat4 [16]float64

func Identity() Mat4          { return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }
func Translation(v Vec3) Mat4 { m := Identity(); m[12], m[13], m[14] = v.X, v.Y, v.Z; return m }
func Scaling(v Vec3) Mat4     { m := Identity(); m[0], m[5], m[10] = v.X, v.Y, v.Z; return m }

// RotationXYZ composes X, then Y, then Z rotations, in radians.
func RotationXYZ(v Vec3) Mat4 {
	x, y, z := Identity(), Identity(), Identity()
	sx, cx := math.Sincos(v.X)
	sy, cy := math.Sincos(v.Y)
	sz, cz := math.Sincos(v.Z)
	x[5], x[6], x[9], x[10] = cx, sx, -sx, cx
	y[0], y[2], y[8], y[10] = cy, -sy, sy, cy
	z[0], z[1], z[4], z[5] = cz, sz, -sz, cz
	return z.Mul(y).Mul(x)
}
func (a Mat4) Mul(b Mat4) (out Mat4) {
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			for k := 0; k < 4; k++ {
				out[c*4+r] += a[k*4+r] * b[c*4+k]
			}
		}
	}
	return
}
func (a Mat4) Point(v Vec3) Vec3 {
	return Vec3{a[0]*v.X + a[4]*v.Y + a[8]*v.Z + a[12], a[1]*v.X + a[5]*v.Y + a[9]*v.Z + a[13], a[2]*v.X + a[6]*v.Y + a[10]*v.Z + a[14]}
}
func (a Mat4) valid() bool {
	for _, v := range a {
		if !finite(v) {
			return false
		}
	}
	return true
}
func (a Mat4) affine() bool { return a.valid() && a[3] == 0 && a[7] == 0 && a[11] == 0 && a[15] == 1 }

type vec4 struct{ x, y, z, w float64 }

func (a Mat4) project(v Vec3) vec4 {
	return vec4{a[0]*v.X + a[4]*v.Y + a[8]*v.Z + a[12], a[1]*v.X + a[5]*v.Y + a[9]*v.Z + a[13], a[2]*v.X + a[6]*v.Y + a[10]*v.Z + a[14], a[3]*v.X + a[7]*v.Y + a[11]*v.Z + a[15]}
}

type Camera struct {
	Eye, Target, Up Vec3
	FOVY, Near, Far float64
}

func DefaultCamera() Camera {
	return Camera{Eye: Vec3{4, 3, 6}, Up: Vec3{0, 1, 0}, FOVY: math.Pi / 4, Near: .05, Far: 1000}
}
func (c Camera) matrix(aspect float64) (Mat4, error) {
	if !c.Eye.valid() || !c.Target.valid() || !c.Up.valid() || !finite(c.FOVY) || c.FOVY <= .01 || c.FOVY >= math.Pi-.01 || !finite(c.Near) || !finite(c.Far) || c.Near <= 0 || c.Far <= c.Near || !finite(aspect) || aspect <= 0 {
		return Mat4{}, ErrCamera
	}
	f := c.Target.Sub(c.Eye).Unit()
	s := f.Cross(c.Up).Unit()
	u := s.Cross(f)
	if s.Length() == 0 || f.Length() == 0 {
		return Mat4{}, ErrCamera
	}
	v := Mat4{s.X, u.X, -f.X, 0, s.Y, u.Y, -f.Y, 0, s.Z, u.Z, -f.Z, 0, -s.Dot(c.Eye), -u.Dot(c.Eye), f.Dot(c.Eye), 1}
	t := 1 / math.Tan(c.FOVY/2)
	p := Mat4{t / aspect, 0, 0, 0, 0, t, 0, 0, 0, 0, (c.Far + c.Near) / (c.Near - c.Far), -1, 0, 0, 2 * c.Far * c.Near / (c.Near - c.Far), 0}
	m := p.Mul(v)
	if !m.valid() {
		return Mat4{}, ErrCamera
	}
	return m, nil
}

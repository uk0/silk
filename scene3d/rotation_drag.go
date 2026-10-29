package scene3d

import "math"

// RotationAxis is a right-handed rotation about a finite nonzero world axis.
// It normalizes without overflow/underflow and returns exact identity for zero
// radians. Angles are radians; no arbitrary epsilon suppresses small rotations.
func RotationAxis(axis Vec3, radians float64) (Mat4, error) {
	u, ok := projectionUnit(axis)
	if !ok || !finite(radians) {
		return Mat4{}, ErrGeometry
	}
	if radians == 0 {
		return Identity(), nil
	}
	s, c := math.Sincos(radians)
	v := 1 - c
	x, y, z := u.X, u.Y, u.Z
	return Mat4{c + x*x*v, y*x*v + z*s, z*x*v - y*s, 0,
		x*y*v - z*s, c + y*y*v, z*y*v + x*s, 0,
		x*z*v + y*s, y*z*v - x*s, c + z*z*v, 0,
		0, 0, 0, 1}, nil
}

// RotationDrag is an immutable world-plane angle solver. It owns no scene and
// does not accumulate rounded angles. Each caller supplies its previous raw
// angle to unwrap turns; consecutive pointer samples must differ by less than
// pi. Apply snapping only to the returned angle, not to the next previous input.
type RotationDrag struct {
	origin, axis, grab Vec3
	ready              bool
}

func NewRotationDrag(ray Ray, origin, axis Vec3) (RotationDrag, error) {
	u, ok := projectionUnit(axis)
	if !ok || !origin.valid() {
		return RotationDrag{}, ErrGeometry
	}
	hit, ok := ray.IntersectPlane(origin, u)
	if !ok {
		return RotationDrag{}, ErrGeometry
	}
	grab, ok := projectionUnit(hit.Sub(origin))
	if !ok {
		return RotationDrag{}, ErrGeometry
	}
	return RotationDrag{origin: origin, axis: u, grab: grab, ready: true}, nil
}

// Angle rejects invalid, parallel, behind-eye or zero-radius input. A failure
// leaves the solver unchanged. Both directions and multiple turns are supported.
func (d RotationDrag) Angle(ray Ray, previous float64) (float64, error) {
	if !d.ready || !finite(previous) {
		return 0, ErrGeometry
	}
	hit, ok := ray.IntersectPlane(d.origin, d.axis)
	if !ok {
		return 0, ErrGeometry
	}
	current, ok := projectionUnit(hit.Sub(d.origin))
	if !ok {
		return 0, ErrGeometry
	}
	a := math.Atan2(d.axis.Dot(d.grab.Cross(current)), d.grab.Dot(current))
	a += math.Round((previous-a)/(2*math.Pi)) * 2 * math.Pi
	if !finite(a) {
		return 0, ErrGeometry
	}
	return a, nil
}

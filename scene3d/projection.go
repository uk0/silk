package scene3d

import "math"

// Ray describes a world-space half-line. Direction need not be normalized.
// Projection helpers are independent of renderers, widgets and native contexts.
type Ray struct {
	Origin, Direction Vec3
}

func projectionSize(width, height float64) bool {
	return finite(width) && finite(height) && width > 0 && height > 0 && finite(width/height) && width/height > 0
}

// Project maps a world point to top-left-origin viewport coordinates. Z is
// normalized depth (0 at Near, 1 at Far). visible means inside all six clip
// planes. Offscreen points in front of the eye still return their coordinates;
// points on/behind the eye return zero and visible=false. Dimensions may be
// logical pixels: using the same dimensions in Ray keeps HiDPI input consistent.
func (c Camera) Project(point Vec3, width, height float64) (pixel Vec3, visible bool, err error) {
	if !projectionSize(width, height) {
		return Vec3{}, false, ErrCamera
	}
	if !point.valid() {
		return Vec3{}, false, ErrGeometry
	}
	m, err := c.matrix(width / height)
	if err != nil {
		return Vec3{}, false, err
	}
	p := m.project(point)
	if !finite(p.x) || !finite(p.y) || !finite(p.z) || !finite(p.w) {
		return Vec3{}, false, ErrGeometry
	}
	if p.w <= 0 {
		return Vec3{}, false, nil
	}
	pixel = Vec3{(p.x/p.w + 1) * .5 * width, (1 - p.y/p.w) * .5 * height, (p.z/p.w + 1) * .5}
	if !pixel.valid() {
		return Vec3{}, false, ErrGeometry
	}
	visible = p.x >= -p.w && p.x <= p.w && p.y >= -p.w && p.y <= p.w && p.z >= -p.w && p.z <= p.w
	return pixel, visible, nil
}

// Ray returns a normalized world ray through a viewport position. Positions
// outside the viewport are allowed for captured pointer drags. It uses the
// exact perspective/basis convention of ViewProjection, not an inverse of a
// large translated matrix. Near/Far validate the camera but do not clip the ray.
func (c Camera) Ray(x, y, width, height float64) (Ray, error) {
	if !projectionSize(width, height) || !finite(x) || !finite(y) {
		return Ray{}, ErrCamera
	}
	if _, err := c.matrix(width / height); err != nil {
		return Ray{}, err
	}
	forward := c.Target.Sub(c.Eye).Unit()
	right := forward.Cross(c.Up).Unit()
	up := right.Cross(forward)
	tan := math.Tan(c.FOVY / 2)
	direction, ok := projectionUnit(forward.Add(right.Mul((2*x/width - 1) * tan * width / height)).Add(up.Mul((1 - 2*y/height) * tan)))
	if !ok {
		return Ray{}, ErrGeometry
	}
	return Ray{Origin: c.Eye, Direction: direction}, nil
}

// projectionUnit avoids overflow/underflow in the length of a finite vector.
func projectionUnit(v Vec3) (Vec3, bool) {
	if !v.valid() {
		return Vec3{}, false
	}
	scale := max(math.Abs(v.X), math.Abs(v.Y), math.Abs(v.Z))
	if scale == 0 {
		return Vec3{}, false
	}
	v = Vec3{v.X / scale, v.Y / scale, v.Z / scale}
	return v.Mul(1 / v.Length()), true
}

// IntersectPlane intersects the forward half-ray with a world-space plane.
// Invalid, near-parallel, behind-eye or numerically unrepresentable intersections
// return false. Normal and Direction may have arbitrary finite nonzero lengths.
func (r Ray) IntersectPlane(point, normal Vec3) (Vec3, bool) {
	direction, ok := projectionUnit(r.Direction)
	if !ok || !r.Origin.valid() || !point.valid() {
		return Vec3{}, false
	}
	normal, ok = projectionUnit(normal)
	if !ok {
		return Vec3{}, false
	}
	denominator := direction.Dot(normal)
	if math.Abs(denominator) < 1e-10 {
		return Vec3{}, false
	}
	distance := point.Sub(r.Origin).Dot(normal) / denominator
	if !finite(distance) || distance < 0 {
		return Vec3{}, false
	}
	hit := r.Origin.Add(direction.Mul(distance))
	return hit, hit.valid()
}

// AxisDrag is an immutable translation solver. A fixed plane contains the
// axis and faces the initial pointer ray as much as possible. Each Offset is
// measured from the initial grab, not accumulated deltas: no initial jump or
// rounding drift. Independent calls are safe across owners. It owns no scene.
type AxisDrag struct {
	origin, axis, normal, grab Vec3
	ready                      bool
}

// NewAxisDrag starts a world-axis gesture. Axis may be any finite nonzero vector.
// Looking almost directly along the axis is ambiguous and returns ErrGeometry;
// callers should not offer an effectively zero-length screen handle.
func NewAxisDrag(ray Ray, origin, axis Vec3) (AxisDrag, error) {
	axis, axisOK := projectionUnit(axis)
	direction, rayOK := projectionUnit(ray.Direction)
	if !axisOK || !rayOK || !origin.valid() {
		return AxisDrag{}, ErrGeometry
	}
	normal := direction.Sub(axis.Mul(direction.Dot(axis)))
	if normal.Length() < 1e-5 {
		return AxisDrag{}, ErrGeometry
	}
	normal, _ = projectionUnit(normal)
	grab, ok := ray.IntersectPlane(origin, normal)
	if !ok {
		return AxisDrag{}, ErrGeometry
	}
	return AxisDrag{origin: origin, axis: axis, normal: normal, grab: grab, ready: true}, nil
}

// Offset returns the world translation from the initial grab. step=0 disables
// snapping; a positive finite step snaps displacement in world units (halfway
// values away from zero), not the node's absolute position. Negative/nonfinite
// steps and invalid/parallel/behind rays fail without changing the solver.
func (d AxisDrag) Offset(ray Ray, step float64) (Vec3, error) {
	if !d.ready || !finite(step) || step < 0 {
		return Vec3{}, ErrGeometry
	}
	hit, ok := ray.IntersectPlane(d.origin, d.normal)
	if !ok {
		return Vec3{}, ErrGeometry
	}
	distance := hit.Sub(d.grab).Dot(d.axis)
	if step > 0 {
		distance = math.Round(distance/step) * step
	}
	offset := d.axis.Mul(distance)
	if !finite(distance) || !offset.valid() {
		return Vec3{}, ErrGeometry
	}
	return offset, nil
}

package scene3d

// Bounds returns the world-space AABB of visible mesh instances. It includes
// all eight transformed corners, supporting rotated/nonuniform/mirrored models.
// Empty snapshots or nonfinite transformed bounds return ErrGeometry.
func (s *Snapshot) Bounds() (lo, hi Vec3, err error) {
	if s == nil || len(s.instances) == 0 {
		return lo, hi, ErrGeometry
	}
	first := true
	for _, in := range s.instances {
		a, b := in.mesh.Bounds()
		for i := 0; i < 8; i++ {
			p := a
			if i&1 != 0 {
				p.X = b.X
			}
			if i&2 != 0 {
				p.Y = b.Y
			}
			if i&4 != 0 {
				p.Z = b.Z
			}
			p = in.world.Point(p)
			if !p.valid() {
				return Vec3{}, Vec3{}, ErrGeometry
			}
			if first {
				lo, hi = p, p
				first = false
			} else {
				lo = Vec3{min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)}
				hi = Vec3{max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)}
			}
		}
	}
	if !hi.Sub(lo).valid() || !finite(hi.Sub(lo).Length()) {
		return Vec3{}, Vec3{}, ErrGeometry
	}
	return lo, hi, nil
}

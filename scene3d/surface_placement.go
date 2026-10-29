package scene3d

import "math"

// SurfacePlacementOptions locates a template-space Anchor at a world surface.
// Offset is a signed world distance along the supplied normal. Heading rotates
// around local +Y in radians before optional alignment of +Y with that normal.
// Zero options keep the authored upright orientation and place its origin.
// These are static transforms, not collision, persistent attachment or physics.
type SurfacePlacementOptions struct {
	Anchor          Vec3
	Offset, Heading float64
	AlignNormal     bool
}

// SurfacePlacementTransform is an allocation-free, constant-work placement
// solver. It maps Anchor to position + unit(normal)*Offset, without scaling or
// decomposing template geometry. Normal uses the surface winding, not the view
// direction; callers may explicitly flip it for a different mounting side.
// Invalid/zero normals, nonfinite input or unrepresentable results return no
// matrix. The result composes with existing Template/EditSession instantiation.
func SurfacePlacementTransform(position, normal Vec3, options SurfacePlacementOptions) (Mat4, error) {
	n, ok := projectionUnit(normal)
	if !ok || !position.valid() || !options.Anchor.valid() || !finite(options.Offset) || !finite(options.Heading) {
		return Mat4{}, ErrGeometry
	}
	rotation := Identity()
	if options.AlignNormal {
		sine := math.Hypot(n.X, n.Z)
		if sine == 0 {
			if n.Y < 0 {
				rotation = Scaling(Vec3{1, -1, -1})
			}
		} else {
			var err error
			rotation, err = RotationAxis(Vec3{n.Z, 0, -n.X}, math.Atan2(sine, n.Y))
			if err != nil {
				return Mat4{}, err
			}
		}
	}
	if options.Heading != 0 {
		rotation = rotation.Mul(RotationXYZ(Vec3{Y: options.Heading}))
	}
	target := position.Add(n.Mul(options.Offset))
	translation := target.Sub(rotation.Point(options.Anchor))
	rotation[12], rotation[13], rotation[14] = translation.X, translation.Y, translation.Z
	if !target.valid() || !rotation.affine() {
		return Mat4{}, ErrGeometry
	}
	return rotation, nil
}

// Bounds returns the cached template-space bounds of visible authored geometry,
// before instance placement or live bindings. Geometry-free/hidden-only or
// numerically unrepresentable bounds return ErrGeometry. It does not scan meshes.
func (t *Template) Bounds() (lo, hi Vec3, err error) {
	if t == nil || !t.boundsValid {
		return lo, hi, ErrGeometry
	}
	return t.boundsLo, t.boundsHi, nil
}

// BaseAnchor is the bottom-center of the authored visible AABB. The midpoint
// calculation avoids overflowing the sum of finite coordinates. This is an
// explicit mounting convention, not a model-origin edit or collision solution.
func (t *Template) BaseAnchor() (Vec3, error) {
	lo, hi, err := t.Bounds()
	if err != nil {
		return Vec3{}, err
	}
	return Vec3{lo.X/2 + hi.X/2, lo.Y, lo.Z/2 + hi.Z/2}, nil
}

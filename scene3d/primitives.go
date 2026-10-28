package scene3d

import "math"

// Cylinder builds an opaque, closed Y-axis cylinder centered at the origin.
// Geometry is immutable and can be shared by many differently transformed nodes.
func Cylinder(radius, height float64, segments int) (*Mesh, error) {
	if !finite(radius) || !finite(height) || radius <= 0 || height <= 0 || radius > 1e12 || height > 1e12 {
		return nil, ErrGeometry
	}
	if segments < 3 || segments > 4096 {
		return nil, ErrLimit
	}
	vertices := make([]Vec3, 2*segments+2)
	indices := make([]uint32, 0, 12*segments)
	for i := 0; i < segments; i++ {
		s, c := math.Sincos(float64(i) * 2 * math.Pi / float64(segments))
		vertices[i] = Vec3{radius * c, -height / 2, radius * s}
		vertices[i+segments] = Vec3{radius * c, height / 2, radius * s}
	}
	bottom, top := uint32(2*segments), uint32(2*segments+1)
	vertices[bottom] = Vec3{Y: -height / 2}
	vertices[top] = Vec3{Y: height / 2}
	for i := 0; i < segments; i++ {
		a, b := uint32(i), uint32((i+1)%segments)
		c, d := a+uint32(segments), b+uint32(segments)
		indices = append(indices, a, c, d, a, d, b, bottom, a, b, top, d, c)
	}
	return NewMesh(vertices, indices)
}

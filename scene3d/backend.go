package scene3d

import "image"

// FrameView is the read-only CPU-accessible result shared by rendering backends.
// Storage belongs to the backend until its next Render. It is not a thread-safe
// live view: copy pixels before handing a frame to another goroutine.
type FrameView interface {
	RGBA() *image.RGBA
	Pick(x, y int) string
	RenderSequence() uint64
	IsCached() bool
}

func (f *Frame) RGBA() *image.RGBA      { return f.Image }
func (f *Frame) RenderSequence() uint64 { return f.Sequence }

// RenderInstance is a value copy of one visible, world-space draw instance.
// Mesh remains immutable; changing this value does not mutate the snapshot.
type RenderInstance struct {
	ID        string
	Mesh      *Mesh
	Transform Mat4
	Color     [4]uint8
}

func (s *Snapshot) Instance(index int) (RenderInstance, bool) {
	if s == nil || index < 0 || index >= len(s.instances) {
		return RenderInstance{}, false
	}
	n := s.instances[index]
	return RenderInstance{n.id, n.mesh, n.world, [4]uint8{n.color.R, n.color.G, n.color.B, n.color.A}}, true
}

// Vertex reads a value without exposing mesh storage. CopyIndices writes to
// caller-owned storage, reusing dst capacity when available.
func (m *Mesh) Vertex(index int) (Vec3, bool) {
	if m == nil || index < 0 || index >= len(m.vertices) {
		return Vec3{}, false
	}
	return m.vertices[index], true
}
func (m *Mesh) CopyIndices(dst []uint32) []uint32 {
	if m == nil {
		return dst[:0]
	}
	return append(dst[:0], m.indices...)
}

// ViewProjection validates the camera and returns its column-major matrix.
func (c Camera) ViewProjection(aspect float64) (Mat4, error) { return c.matrix(aspect) }

func (f *Frame) IsCached() bool { return f.Stats.Cached }

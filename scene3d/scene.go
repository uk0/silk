package scene3d

import (
	"errors"
	"fmt"
	"image/color"
)

var (
	ErrGeometry    = errors.New("scene3d: invalid geometry")
	ErrNode        = errors.New("scene3d: invalid node")
	ErrCamera      = errors.New("scene3d: invalid camera")
	ErrLimit       = errors.New("scene3d: resource limit exceeded")
	ErrUnsupported = errors.New("scene3d: unsupported model feature")
)

const MaxVertices = 250000
const MaxTriangles = 500000
const MaxNodes = 10000

// Snapshot budgets count every visible instance, not only unique shared meshes.
// They bound CPU work even when a single large mesh is instanced many times.
const MaxSnapshotVertices = 1000000
const MaxSnapshotTriangles = 1000000

// Mesh owns copies of its validated inputs. Its storage is never exposed.
type Mesh struct {
	vertices []Vec3
	indices  []uint32
	min, max Vec3
}

func NewMesh(vertices []Vec3, indices []uint32) (*Mesh, error) {
	if len(vertices) > MaxVertices || len(indices)/3 > MaxTriangles {
		return nil, ErrLimit
	}
	if len(vertices) == 0 || len(indices) == 0 || len(indices)%3 != 0 {
		return nil, ErrGeometry
	}
	lo, hi := vertices[0], vertices[0]
	for _, v := range vertices {
		if !v.valid() || v.Length() > 1e12 {
			return nil, ErrGeometry
		}
		lo = Vec3{min(lo.X, v.X), min(lo.Y, v.Y), min(lo.Z, v.Z)}
		hi = Vec3{max(hi.X, v.X), max(hi.Y, v.Y), max(hi.Z, v.Z)}
	}
	for _, i := range indices {
		if uint64(i) >= uint64(len(vertices)) {
			return nil, ErrGeometry
		}
	}
	return &Mesh{vertices: append([]Vec3(nil), vertices...), indices: append([]uint32(nil), indices...), min: lo, max: hi}, nil
}
func (m *Mesh) VertexCount() int {
	if m == nil {
		return 0
	}
	return len(m.vertices)
}
func (m *Mesh) TriangleCount() int {
	if m == nil {
		return 0
	}
	return len(m.indices) / 3
}
func (m *Mesh) Bounds() (Vec3, Vec3) {
	if m == nil {
		return Vec3{}, Vec3{}
	}
	return m.min, m.max
}
func Box() *Mesh {
	m, _ := NewMesh([]Vec3{{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1}, {-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1}}, []uint32{0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, 0, 1, 5, 0, 5, 4, 3, 7, 6, 3, 6, 2, 1, 2, 6, 1, 6, 5, 0, 4, 7, 0, 7, 3})
	return m
}

// Node is a value specification; a zero transform means identity, and a zero
// color selects the default. Only opaque materials are supported at this stage.
type Node struct {
	ID, Parent, Name string
	Mesh             *Mesh
	Transform        Mat4
	Color            color.RGBA
	Hidden           bool
}
type Scene struct {
	nodes    map[string]Node
	order    []string
	revision uint64
	cached   *Snapshot
}
type instance struct {
	id    string
	mesh  *Mesh
	world Mat4
	color color.RGBA
}

// Snapshot owns immutable draw state; later Scene edits cannot change it.
type Snapshot struct{ instances []instance }

func (s *Snapshot) InstanceCount() int {
	if s == nil {
		return 0
	}
	return len(s.instances)
}
func NewScene() *Scene                       { return &Scene{nodes: make(map[string]Node)} }
func (s *Scene) Revision() uint64            { return s.revision }
func (s *Scene) Node(id string) (Node, bool) { n, ok := s.nodes[id]; return n, ok }
func (s *Scene) Add(n Node) error {
	if len(s.nodes) >= MaxNodes {
		return ErrLimit
	}
	if n.ID == "" || len(n.ID) > 256 || n.Parent == n.ID {
		return ErrNode
	}
	if _, ok := s.nodes[n.ID]; ok {
		return fmt.Errorf("%w: duplicate %q", ErrNode, n.ID)
	}
	if n.Parent != "" {
		if _, ok := s.nodes[n.Parent]; !ok {
			return fmt.Errorf("%w: missing parent %q", ErrNode, n.Parent)
		}
	}
	if n.Transform == (Mat4{}) {
		n.Transform = Identity()
	}
	if !n.Transform.affine() {
		return ErrNode
	}
	if n.Color == (color.RGBA{}) {
		n.Color = color.RGBA{65, 160, 220, 255}
	}
	if n.Color.A != 255 {
		return ErrUnsupported
	}
	if s.nodes == nil {
		s.nodes = make(map[string]Node)
	}
	s.nodes[n.ID] = n
	s.order = append(s.order, n.ID)
	s.revision++
	s.cached = nil
	return nil
}
func (s *Scene) update(n Node) {
	if s.nodes[n.ID] != n {
		s.nodes[n.ID] = n
		s.revision++
		s.cached = nil
	}
}
func (s *Scene) SetTransform(id string, m Mat4) error {
	n, ok := s.nodes[id]
	if !ok || !m.affine() {
		return ErrNode
	}
	n.Transform = m
	s.update(n)
	return nil
}
func (s *Scene) SetVisible(id string, v bool) error {
	n, ok := s.nodes[id]
	if !ok {
		return ErrNode
	}
	n.Hidden = !v
	s.update(n)
	return nil
}
func (s *Scene) SetColor(id string, c color.RGBA) error {
	n, ok := s.nodes[id]
	if !ok {
		return ErrNode
	}
	if c.A != 255 {
		return ErrUnsupported
	}
	n.Color = c
	s.update(n)
	return nil
}

// Remove removes a node and its descendants. Existing snapshots remain valid.
func (s *Scene) Remove(id string) bool {
	if _, ok := s.nodes[id]; !ok {
		return false
	}
	removed := map[string]bool{id: true}
	out := s.order[:0]
	for _, key := range s.order {
		n := s.nodes[key]
		if removed[key] || removed[n.Parent] {
			removed[key] = true
			delete(s.nodes, key)
		} else {
			out = append(out, key)
		}
	}
	clear(s.order[len(out):])
	s.order = out
	s.revision++
	s.cached = nil
	return true
}
func (s *Scene) Snapshot() (*Snapshot, error) {
	if s.cached != nil {
		return s.cached, nil
	}
	type state struct {
		world  Mat4
		hidden bool
	}
	vertices, triangles := 0, 0
	states := make(map[string]state, len(s.nodes))
	snap := &Snapshot{instances: make([]instance, 0, len(s.nodes))}
	for _, id := range s.order {
		n := s.nodes[id]
		w, hidden := n.Transform, n.Hidden
		if n.Parent != "" {
			p := states[n.Parent]
			w = p.world.Mul(w)
			hidden = hidden || p.hidden
		}
		if !w.affine() {
			return nil, ErrNode
		}
		states[id] = state{w, hidden}
		if n.Mesh != nil && !hidden {
			vertices += n.Mesh.VertexCount()
			triangles += n.Mesh.TriangleCount()
			if vertices > MaxSnapshotVertices || triangles > MaxSnapshotTriangles {
				return nil, ErrLimit
			}
			snap.instances = append(snap.instances, instance{id, n.Mesh, w, n.Color})
		}
	}
	s.cached = snap
	return snap, nil
}

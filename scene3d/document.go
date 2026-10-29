package scene3d

import (
	"bytes"
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"unicode/utf8"
)

const DocumentVersion = 1
const MaxDocumentBytes = 32 << 20
const maxDocumentTextBytes = 4 << 20

// Document is a portable static scene asset (.silkscene). Geometry is embedded
// once per mesh; nodes reference it by ID. No paths, network resources, code or
// subscriptions are executed when decoding. It describes this engine's opaque
// geometry subset, not original glTF textures/animations or an interchange PBR
// format. All slices are caller-owned; do not mutate them concurrently.
type Document struct {
	Version  int            `json:"version"`
	Meshes   []DocumentMesh `json:"meshes,omitempty"`
	Nodes    []DocumentNode `json:"nodes,omitempty"`
	Bindings []Binding      `json:"bindings,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}
type DocumentMesh struct {
	ID       string   `json:"id"`
	Vertices []Vec3   `json:"vertices"`
	Indices  []uint32 `json:"indices"`
}
type DocumentNode struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
	Name   string `json:"name,omitempty"`
	Mesh   string `json:"mesh,omitempty"`
	// Omitted means identity. An explicit matrix must have exactly 16 elements.
	Transform []float64  `json:"transform,omitempty"`
	Color     color.RGBA `json:"color"`
	Hidden    bool       `json:"hidden,omitempty"`
}

// Nodes returns value copies in stable parent-before-child rendering order.
// Mesh pointers remain immutable shared assets.
func (s *Scene) Nodes() []Node {
	if s == nil {
		return nil
	}
	out := make([]Node, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.nodes[id])
	}
	return out
}

// CaptureDocument copies an authored scene and deduplicates meshes by identity.
// For an active binding session use BindingSet.CaptureDocument instead, so live
// tag values do not become new authored transform baselines.
func CaptureDocument(s *Scene, bindings []Binding) (*Document, error) {
	if s == nil {
		return nil, ErrNode
	}
	if _, err := s.Snapshot(); err != nil {
		return nil, err
	}
	if _, err := NewBindingSet(s, bindings); err != nil {
		return nil, err
	}
	d := &Document{Version: DocumentVersion, Bindings: append([]Binding(nil), bindings...)}
	meshes := make(map[*Mesh]string)
	vertices, triangles := 0, 0
	for _, n := range s.Nodes() {
		meshID := ""
		if n.Mesh != nil {
			var ok bool
			meshID, ok = meshes[n.Mesh]
			if !ok {
				vertices += n.Mesh.VertexCount()
				triangles += n.Mesh.TriangleCount()
				if vertices > MaxVertices || triangles > MaxTriangles {
					return nil, ErrLimit
				}
				meshID = fmt.Sprintf("mesh/%d", len(d.Meshes))
				meshes[n.Mesh] = meshID
				d.Meshes = append(d.Meshes, DocumentMesh{meshID, append([]Vec3(nil), n.Mesh.vertices...), append([]uint32(nil), n.Mesh.indices...)})
			}
		}
		d.Nodes = append(d.Nodes, DocumentNode{n.ID, n.Parent, n.Name, meshID, append([]float64(nil), n.Transform[:]...), n.Color, n.Hidden})
	}
	if err := d.checkLimits(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Document) checkLimits() error {
	if d == nil {
		return ErrGeometry
	}
	if d.Version != DocumentVersion {
		return fmt.Errorf("%w: scene document version %d", ErrUnsupported, d.Version)
	}
	if len(d.Meshes) > MaxNodes || len(d.Nodes) > MaxNodes || len(d.Bindings) > MaxBindings || len(d.Warnings) > 256 {
		return ErrLimit
	}
	textBytes, vertices, triangles := 0, 0, 0
	for _, m := range d.Meshes {
		if !validSceneText(m.ID, 256, false) {
			return ErrNode
		}
		if len(m.Vertices) > MaxVertices || len(m.Indices) > MaxTriangles*3 {
			return ErrLimit
		}
		vertices += len(m.Vertices)
		triangles += len(m.Indices) / 3
		textBytes += len(m.ID)
		if vertices > MaxVertices || triangles > MaxTriangles {
			return ErrLimit
		}
	}
	for _, n := range d.Nodes {
		if len(n.Transform) != 0 && len(n.Transform) != 16 {
			return ErrNode
		}
		if !validSceneText(n.ID, 256, false) || !validSceneText(n.Parent, 256, true) || !validSceneText(n.Name, 4096, true) || !validSceneText(n.Mesh, 256, true) {
			return ErrNode
		}
		textBytes += len(n.ID) + len(n.Parent) + len(n.Name) + len(n.Mesh)
	}
	for _, b := range d.Bindings {
		if !validSceneText(b.Node, 256, false) || !validSceneText(b.Tag, 512, false) || !validSceneText(string(b.Property), 32, false) {
			return ErrBinding
		}
		textBytes += len(b.Node) + len(b.Tag) + len(b.Property)
	}
	for _, w := range d.Warnings {
		if !validSceneText(w, 4096, true) {
			return ErrLimit
		}
		textBytes += len(w)
	}
	if textBytes > maxDocumentTextBytes {
		return ErrLimit
	}
	return nil
}

type documentOrder []int

func (h documentOrder) Len() int           { return len(h) }
func (h documentOrder) Less(i, j int) bool { return h[i] < h[j] }
func (h documentOrder) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *documentOrder) Push(x any)        { *h = append(*h, x.(int)) }
func (h *documentOrder) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// Build validates and creates an independent mutable scene, sharing each copied
// immutable mesh among its instances. Input node order is retained whenever it
// is already topological; unordered documents are stably topologically sorted.
// This preserves coplanar depth/picking order on a capture/save/load roundtrip.
func (d *Document) Build(ctx context.Context) (*Scene, error) { return d.buildWithMeshes(ctx, nil) }

// Reused meshes belong only to a private EditSession whose mesh definitions are
// immutable. Public Build never accepts an external cache or skips validation.
func (d *Document) buildWithMeshes(ctx context.Context, reusable map[string]*Mesh) (*Scene, error) {
	if ctx == nil {
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := d.checkLimits(); err != nil {
		return nil, err
	}
	meshes := make(map[string]*Mesh, len(d.Meshes))
	for _, m := range d.Meshes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, ok := meshes[m.ID]; ok {
			return nil, fmt.Errorf("%w: duplicate mesh", ErrGeometry)
		}
		mesh := reusable[m.ID]
		if mesh == nil {
			var err error
			mesh, err = NewMesh(m.Vertices, m.Indices)
			if err != nil {
				return nil, err
			}
			if reusable != nil {
				reusable[m.ID] = mesh
			}
		}
		meshes[m.ID] = mesh
	}
	indexes := make(map[string]int, len(d.Nodes))
	nodes := make([]Node, len(d.Nodes))
	for i, n := range d.Nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, ok := indexes[n.ID]; ok {
			return nil, ErrNode
		}
		indexes[n.ID] = i
		m := Identity()
		if len(n.Transform) != 0 {
			if len(n.Transform) != 16 {
				return nil, ErrNode
			}
			copy(m[:], n.Transform)
		}
		if !m.affine() {
			return nil, ErrNode
		}
		var mesh *Mesh
		if n.Mesh != "" {
			var ok bool
			mesh, ok = meshes[n.Mesh]
			if !ok {
				return nil, ErrNode
			}
		}
		nodes[i] = Node{ID: n.ID, Parent: n.Parent, Name: n.Name, Mesh: mesh, Transform: m, Color: n.Color, Hidden: n.Hidden}
	}
	children := make([][]int, len(nodes))
	ready := &documentOrder{}
	for i, n := range nodes {
		if n.Parent == "" {
			heap.Push(ready, i)
		} else {
			p, ok := indexes[n.Parent]
			if !ok || p == i {
				return nil, ErrNode
			}
			children[p] = append(children[p], i)
		}
	}
	s := NewScene()
	visited := 0
	for ready.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		i := heap.Pop(ready).(int)
		if err := s.Add(nodes[i]); err != nil {
			return nil, err
		}
		visited++
		for _, child := range children[i] {
			heap.Push(ready, child)
		}
	}
	if visited != len(nodes) {
		return nil, fmt.Errorf("%w: hierarchy cycle", ErrNode)
	}
	if _, err := s.Snapshot(); err != nil {
		return nil, err
	}
	if _, err := NewBindingSet(s, d.Bindings); err != nil {
		return nil, err
	}
	return s, nil
}

// LoadDocument is bounded and side-effect free. A blocking Reader still needs
// caller-owned close/deadline support. Unknown fields, duplicate keys, trailing
// JSON values and excessive metadata are rejected before building any scene.
func LoadDocument(ctx context.Context, r io.Reader) (*Document, error) {
	return loadDocumentWithLimits(ctx, r, LoadLimits{})
}

func loadDocumentWithLimits(ctx context.Context, r io.Reader, limits LoadLimits) (*Document, error) {
	if ctx == nil || r == nil {
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limits, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(&budgetReader{ctx: ctx, r: r, max: min(int64(MaxDocumentBytes), limits.MaxBytes)})
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxDocumentBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(raw) {
		return nil, ErrGeometry
	}
	if err = checkBoundedJSON(ctx, raw, 8_000_000, MaxVertices+MaxBindings+MaxNodes*4, 32, true); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var d Document
	if err = dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%w: scene document JSON: %v", ErrGeometry, err)
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing scene document JSON", ErrGeometry)
	}
	vertices, triangles := 0, 0
	for _, m := range d.Meshes {
		if len(m.Vertices) > limits.MaxVertices || len(m.Indices) > limits.MaxTriangles*3 {
			return nil, ErrLimit
		}
		vertices += len(m.Vertices)
		triangles += len(m.Indices) / 3
		if vertices > limits.MaxVertices || triangles > limits.MaxTriangles {
			return nil, ErrLimit
		}
	}
	if _, err = d.Build(ctx); err != nil {
		return nil, err
	}
	return &d, nil
}

// SaveDocument validates before touching w. Geometry/input budgets bound work;
// invalid/oversized documents write nothing. Writer failures may still leave a
// partial output, so use an atomic-file facility when replacing saved assets.
func SaveDocument(ctx context.Context, w io.Writer, d *Document) error {
	if ctx == nil || w == nil {
		return ErrGeometry
	}
	if _, err := d.Build(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if len(raw)+1 > MaxDocumentBytes {
		return ErrLimit
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	raw = append(raw, '\n')
	n, err := w.Write(raw)
	if err == nil && n != len(raw) {
		return io.ErrShortWrite
	}
	return err
}

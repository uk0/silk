package scene3d

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

var ErrEditClosed = errors.New("scene3d: edit session closed")
var ErrEditHistory = errors.New("scene3d: no edit history")

const MaxEditHistory = 32
const MaxEditHistoryBytes = 32 << 20

type editState struct {
	document *Document
	meshes   map[string]*Mesh // immutable resources; copied before adding a resource
	cost     int
}

// EditSession owns a private, inert authored document. It creates no tag DB,
// subscriptions, goroutines, files or graphics contexts. Mutations validate a
// candidate graph and bindings before publication; failures and cancellations
// leave the document, preview snapshot, revision and history unchanged.
//
// All methods are single-owner. Meshes and returned snapshots remain immutable
// and may be rendered by independent owners. Scene and Document return independent
// mutable copies. Geometry is compiled once and shared across node edits/history.
// Undo/redo retains at most 32 states and 32 MiB of conservatively estimated
// history metadata; shared geometry and the current/initial state are separate.
type EditSession struct {
	state      editState
	scene      *Scene
	initial    *Document
	undo, redo []editState
	revision   uint64
	closed     bool
}

func cloneDocumentOwned(d *Document) *Document {
	c := *d
	c.Nodes = cloneDocumentNodes(d.Nodes)
	c.Bindings = append([]Binding(nil), d.Bindings...)
	c.Warnings = append([]string(nil), d.Warnings...)
	c.Meshes = append([]DocumentMesh(nil), d.Meshes...)
	for i := range c.Meshes {
		c.Meshes[i].Vertices = append([]Vec3(nil), c.Meshes[i].Vertices...)
		c.Meshes[i].Indices = append([]uint32(nil), c.Meshes[i].Indices...)
	}
	return &c
}
func cloneDocumentNodes(nodes []DocumentNode) []DocumentNode {
	out := append([]DocumentNode(nil), nodes...)
	for i := range out {
		out[i].Transform = append([]float64(nil), out[i].Transform...)
	}
	return out
}
func editCost(d *Document) int {
	cost := len(d.Nodes)*192 + len(d.Bindings)*160 + len(d.Meshes)*96
	for _, n := range d.Nodes {
		cost += len(n.ID) + len(n.Parent) + len(n.Name) + len(n.Mesh) + len(n.Transform)*8
	}
	for _, b := range d.Bindings {
		cost += len(b.Node) + len(b.Tag) + len(b.Property)
	}
	for _, m := range d.Meshes {
		cost += len(m.ID)
	}
	for _, w := range d.Warnings {
		cost += len(w)
	}
	return cost
}
func NewEditSession(ctx context.Context, d *Document) (*EditSession, error) {
	if ctx == nil {
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := d.checkLimits(); err != nil {
		return nil, err
	}
	owned := cloneDocumentOwned(d)
	meshes := make(map[string]*Mesh, len(owned.Meshes))
	scene, err := owned.buildWithMeshes(ctx, meshes)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &EditSession{state: editState{owned, meshes, editCost(owned)}, scene: scene, initial: owned}, nil
}
func (e *EditSession) ready(ctx context.Context) error {
	if e == nil || e.closed || e.scene == nil {
		return ErrEditClosed
	}
	if ctx == nil {
		return ErrGeometry
	}
	return ctx.Err()
}
func (e *EditSession) Revision() uint64 {
	if e == nil {
		return 0
	}
	return e.revision
}
func (e *EditSession) Modified() bool { return e != nil && !e.closed && e.state.document != e.initial }
func (e *EditSession) CanUndo() bool  { return e != nil && !e.closed && len(e.undo) > 0 }
func (e *EditSession) CanRedo() bool  { return e != nil && !e.closed && len(e.redo) > 0 }
func (e *EditSession) HistoryUsage() (states, metadataBytes int) {
	if e == nil || e.closed {
		return 0, 0
	}
	for _, s := range e.undo {
		states++
		metadataBytes += s.cost
	}
	for _, s := range e.redo {
		states++
		metadataBytes += s.cost
	}
	return
}
func (e *EditSession) Document() (*Document, error) {
	if err := e.ready(context.Background()); err != nil {
		return nil, err
	}
	return cloneDocumentOwned(e.state.document), nil
}
func (e *EditSession) Nodes() []DocumentNode {
	if e == nil || e.closed {
		return nil
	}
	return cloneDocumentNodes(e.state.document.Nodes)
}
func (e *EditSession) Bindings() []Binding {
	if e == nil || e.closed {
		return nil
	}
	return append([]Binding(nil), e.state.document.Bindings...)
}
func (e *EditSession) MeshIDs() []string {
	if e == nil || e.closed {
		return nil
	}
	out := make([]string, len(e.state.document.Meshes))
	for i, m := range e.state.document.Meshes {
		out[i] = m.ID
	}
	return out
}
func (e *EditSession) Mesh(id string) (*Mesh, bool) {
	if e == nil || e.closed {
		return nil, false
	}
	m, ok := e.state.meshes[id]
	return m, ok
}
func (e *EditSession) Scene() (*Scene, error) {
	if err := e.ready(context.Background()); err != nil {
		return nil, err
	}
	out := *e.scene
	out.nodes = make(map[string]Node, len(e.scene.nodes))
	for id, n := range e.scene.nodes {
		out.nodes[id] = n
	}
	out.order = append([]string(nil), e.scene.order...)
	return &out, nil
}
func (e *EditSession) Close() {
	if e == nil {
		return
	}
	e.closed = true
	e.scene = nil
	e.initial = nil
	e.state = editState{}
	clear(e.undo)
	clear(e.redo)
	e.undo = nil
	e.redo = nil
}
func (e *EditSession) candidate() *Document {
	d := *e.state.document
	// Existing transforms/geometry are privately owned and never changed in place.
	d.Nodes = append([]DocumentNode(nil), d.Nodes...)
	d.Bindings = append([]Binding(nil), d.Bindings...)
	return &d
}
func sameSnapshot(a, b *Snapshot) bool {
	if a == nil || b == nil || len(a.instances) != len(b.instances) {
		return false
	}
	for i := range a.instances {
		if a.instances[i] != b.instances[i] {
			return false
		}
	}
	return true
}
func (e *EditSession) prepare(ctx context.Context, d *Document, meshes map[string]*Mesh) (*Scene, error) {
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	scene, err := d.buildWithMeshes(ctx, meshes)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	// Renaming or changing inert binding metadata must not invalidate render caches
	// or re-upload an entire imported assembly. Snapshot contents, not intent, decide.
	if sameSnapshot(scene.cached, e.scene.cached) {
		scene.cached = e.scene.cached
	}
	scene.revision = e.scene.revision + 1
	return scene, nil
}
func (e *EditSession) commit(ctx context.Context, d *Document, meshes map[string]*Mesh) error {
	scene, err := e.prepare(ctx, d, meshes)
	if err != nil {
		return err
	}
	e.undo = append(e.undo, e.state)
	clear(e.redo)
	e.redo = nil
	e.state = editState{d, meshes, editCost(d)}
	e.scene = scene
	e.revision++
	e.trimHistory()
	return nil
}
func dropFirst(states []editState) []editState {
	copy(states, states[1:])
	states[len(states)-1] = editState{}
	return states[:len(states)-1]
}
func (e *EditSession) trimHistory() {
	for {
		count, bytes := e.HistoryUsage()
		if count <= MaxEditHistory && bytes <= MaxEditHistoryBytes {
			return
		}
		if len(e.undo) > 0 {
			e.undo = dropFirst(e.undo)
		} else if len(e.redo) > 0 {
			e.redo = dropFirst(e.redo)
		} else {
			return
		}
	}
}
func (e *EditSession) Undo(ctx context.Context) error { return e.restore(ctx, false) }
func (e *EditSession) Redo(ctx context.Context) error { return e.restore(ctx, true) }
func (e *EditSession) restore(ctx context.Context, redo bool) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	from, to := &e.undo, &e.redo
	if redo {
		from, to = &e.redo, &e.undo
	}
	if len(*from) == 0 {
		return ErrEditHistory
	}
	state := (*from)[len(*from)-1]
	scene, err := e.prepare(ctx, state.document, state.meshes)
	if err != nil {
		return err
	}
	*to = append(*to, e.state)
	(*from)[len(*from)-1] = editState{}
	*from = (*from)[:len(*from)-1]
	e.state = state
	e.scene = scene
	e.revision++
	e.trimHistory()
	return nil
}
func (e *EditSession) AddNode(ctx context.Context, node DocumentNode) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	if len(node.Transform) != 0 && len(node.Transform) != 16 {
		return ErrNode
	}
	d := e.candidate()
	node.Transform = append([]float64(nil), node.Transform...)
	d.Nodes = append(d.Nodes, node)
	return e.commit(ctx, d, e.state.meshes)
}

// AddMeshNode adds a resource and its first node in one undoable transaction.
// An existing mesh ID may be reused only with the same immutable mesh pointer;
// use AddNode to reference an existing resource without passing geometry again.
func (e *EditSession) AddMeshNode(ctx context.Context, node DocumentNode, meshID string, mesh *Mesh) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	if mesh == nil || mesh.VertexCount() == 0 || mesh.TriangleCount() == 0 || !validSceneText(meshID, 256, false) {
		return ErrGeometry
	}
	if len(node.Transform) != 0 && len(node.Transform) != 16 {
		return ErrNode
	}
	if node.Mesh != "" && node.Mesh != meshID {
		return ErrNode
	}
	d := e.candidate()
	node.Mesh = meshID
	node.Transform = append([]float64(nil), node.Transform...)
	d.Nodes = append(d.Nodes, node)
	cache := e.state.meshes
	if old, ok := cache[meshID]; ok {
		if old != mesh {
			return fmt.Errorf("%w: immutable mesh ID already exists", ErrNode)
		}
	} else {
		d.Meshes = append(append([]DocumentMesh(nil), d.Meshes...), DocumentMesh{ID: meshID, Vertices: mesh.vertices, Indices: mesh.indices})
		if err := d.checkLimits(); err != nil {
			return err
		}
		d.Meshes[len(d.Meshes)-1].Vertices = append([]Vec3(nil), mesh.vertices...)
		d.Meshes[len(d.Meshes)-1].Indices = append([]uint32(nil), mesh.indices...)
		cache = make(map[string]*Mesh, len(e.state.meshes)+1)
		for id, m := range e.state.meshes {
			cache[id] = m
		}
		cache[meshID] = mesh
	}
	return e.commit(ctx, d, cache)
}
func (e *EditSession) UpdateNode(ctx context.Context, node DocumentNode) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	if len(node.Transform) != 0 && len(node.Transform) != 16 {
		return ErrNode
	}
	d := e.candidate()
	for i, n := range d.Nodes {
		if n.ID == node.ID {
			if reflect.DeepEqual(n, node) {
				return nil
			}
			node.Transform = append([]float64(nil), node.Transform...)
			d.Nodes[i] = node
			return e.commit(ctx, d, e.state.meshes)
		}
	}
	return ErrNode
}
func (e *EditSession) SetBinding(ctx context.Context, binding Binding) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	d := e.candidate()
	for i, b := range d.Bindings {
		if b.Node == binding.Node && b.Property == binding.Property {
			if b == binding {
				return nil
			}
			d.Bindings[i] = binding
			return e.commit(ctx, d, e.state.meshes)
		}
	}
	d.Bindings = append(d.Bindings, binding)
	return e.commit(ctx, d, e.state.meshes)
}
func (e *EditSession) RemoveBinding(ctx context.Context, node string, property BindingProperty) (bool, error) {
	if err := e.ready(ctx); err != nil {
		return false, err
	}
	d := e.candidate()
	for i, b := range d.Bindings {
		if b.Node == node && b.Property == property {
			d.Bindings = append(d.Bindings[:i], d.Bindings[i+1:]...)
			err := e.commit(ctx, d, e.state.meshes)
			return err == nil, err
		}
	}
	return false, nil
}

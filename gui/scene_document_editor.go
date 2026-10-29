package gui

import (
	"context"
	"reflect"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

// SceneDocumentEditor edits a private, inert copy of a portable scene. Opening
// it never reads files, starts timers, creates tags or subscribes to drivers.
// Use Document to capture the staged result, then let the host own undo/save.
// All methods belong to the UI thread. Close releases the preview renderer.
// Node/geometry validation runs on explicit edits, not on paint or idle.
type SceneDocumentEditor struct {
	Widget
	document         *scene3d.Document
	scene            *scene3d.Scene
	tree             *TreeView
	model            *sceneDocumentTree
	preview          *SceneView
	bindings         *ListWidget
	status           *Label
	buttons          []*Button
	selected         string
	modified, closed bool
	revision         uint64
}

func cloneSceneDocument(d *scene3d.Document) *scene3d.Document {
	c := *d
	c.Nodes = append([]scene3d.DocumentNode(nil), d.Nodes...)
	for i := range c.Nodes {
		c.Nodes[i].Transform = append([]float64(nil), c.Nodes[i].Transform...)
	}
	c.Meshes = append([]scene3d.DocumentMesh(nil), d.Meshes...)
	for i := range c.Meshes {
		c.Meshes[i].Vertices = append([]scene3d.Vec3(nil), c.Meshes[i].Vertices...)
		c.Meshes[i].Indices = append([]uint32(nil), c.Meshes[i].Indices...)
	}
	c.Bindings = append([]scene3d.Binding(nil), d.Bindings...)
	c.Warnings = append([]string(nil), d.Warnings...)
	return &c
}

func NewSceneDocumentEditor(d *scene3d.Document) (*SceneDocumentEditor, error) {
	s, err := d.Build(context.Background())
	if err != nil {
		return nil, err
	}
	e := &SceneDocumentEditor{document: cloneSceneDocument(d), scene: s}
	e.Widget.Init(e)
	e.tree = NewTreeView()
	e.tree.SetParent(e)
	e.preview = NewSceneView()
	e.preview.SetParent(e)
	e.preview.SetScene(s)
	e.bindings = NewListWidget()
	e.bindings.SetFillAvailable(true)
	e.bindings.SetParent(e)
	e.status = NewLabel("")
	e.status.SetParent(e)
	e.tree.SetActivatedCallback(func(_ interface{}, index ModelIndex) {
		if id, ok := index.Param.(string); ok {
			e.SelectNode(id)
		}
	})
	e.preview.SigSelected(func(id string) {
		if id != "" {
			e.SelectNode(id)
		}
	})
	e.bindings.SigSelectionChanged(func(_ interface{}, _ []int) { e.syncActions() })
	for _, spec := range []struct {
		label, icon string
		run         func()
	}{
		{"Edit node...", "propsheet", e.editNode},
		{"Set binding...", "plus", e.editBinding},
		{"Remove binding", "close", e.removeSelectedBinding},
		{"Fit scene", "preview", func() {
			if err := e.preview.FitScene(); err != nil {
				e.report(err)
			}
		}},
	} {
		b := NewButton()
		b.SetText(i18n.T(spec.label))
		b.SetIcon(LoadIcon(spec.icon))
		b.SetTextVisible(true)
		SetToolTip(b, i18n.T(spec.label))
		b.SetParent(e)
		b.Action().BindFunc0(spec.run)
		e.buttons = append(e.buttons, b)
	}
	e.rebuildTree()
	if len(d.Nodes) > 0 {
		e.SelectNode(d.Nodes[0].ID)
	} else {
		e.syncActions()
	}
	if snap, err := s.Snapshot(); err == nil && snap.InstanceCount() > 0 {
		_ = e.preview.FitScene()
	}
	e.Layout()
	return e, nil
}

func (e *SceneDocumentEditor) Modified() bool         { return e.modified }
func (e *SceneDocumentEditor) SelectedNodeID() string { return e.selected }
func (e *SceneDocumentEditor) Document() (*scene3d.Document, error) {
	if e.closed {
		return nil, async.ErrClosed
	}
	return cloneSceneDocument(e.document), nil
}
func (e *SceneDocumentEditor) Close() {
	if e.closed {
		return
	}
	e.closed = true
	e.preview.Close()
}
func (e *SceneDocumentEditor) SelectNode(id string) bool {
	if e.closed {
		return false
	}
	if _, ok := e.scene.Node(id); !ok {
		return false
	}
	e.selected = id
	// Expand only the ancestry of the picked node, not a 10,000-node forest.
	chain := []string{}
	for p := e.model.parents[id]; p != ""; p = e.model.parents[p] {
		chain = append(chain, p)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if r := e.tree.rmap[e.model.indexes[chain[i]]]; r != nil {
			e.tree.expand(r)
		}
	}
	e.tree.Layout()
	e.fitTreeColumn()
	if r := e.tree.rmap[e.model.indexes[id]]; r != nil {
		e.tree.SetCurrentRow(r.ri)
	}
	e.rebuildBindings()
	e.syncActions()
	return true
}
func (e *SceneDocumentEditor) rebuildTree() {
	e.model = newSceneDocumentTree(e.document.Nodes)
	e.tree.SetModel(e.model)
	e.fitTreeColumn()
}

// This inspector has one column. Do not leave the generic header's 128-pixel
// default clipping ordinary node names while most of the tree pane is empty.
func (e *SceneDocumentEditor) fitTreeColumn() {
	if e.tree.header.SectionCount() != 1 {
		return
	}
	w := e.tree.clientRect().Width
	if bar := e.tree.VertScrollBar(); bar.IsVisible() {
		w -= max(bar.Width(), Theme().ScrollWidth)
	}
	e.tree.header.sections[0].Size = max(8, w)
	e.tree.Layout()
}
func (e *SceneDocumentEditor) rebuildBindings() {
	e.bindings.Clear()
	for _, b := range e.document.Bindings {
		if b.Node == e.selected {
			e.bindings.Append(ListItem{Text: string(b.Property) + " ← " + b.Tag, Data: b.Property})
		}
	}
	if e.bindings.Count() > 0 {
		e.bindings.SetActiveIndex(0)
	}
}
func (e *SceneDocumentEditor) syncActions() {
	selected := e.selected != "" && !e.closed
	if len(e.buttons) < 4 {
		return
	}
	e.buttons[0].SetEnabled(selected)
	e.buttons[1].SetEnabled(selected)
	e.buttons[2].SetEnabled(selected && e.bindings.ActiveIndex() >= 0)
	text := i18n.Tf("%d nodes · %d bindings", len(e.document.Nodes), len(e.document.Bindings))
	if e.selected != "" {
		text += " · " + e.selected
	}
	e.status.SetText(text)
	SetToolTip(e.status, text)
}
func (e *SceneDocumentEditor) report(err error) {
	if err != nil {
		ShowToast(e, err.Error(), 4500, ToastWarning)
	}
}

// UpdateNode preserves mesh ownership and validates hierarchy/world bounds as a
// transaction. IDs identify existing nodes; insertion/deletion is not provided.
func (e *SceneDocumentEditor) UpdateNode(node scene3d.DocumentNode) error {
	if e.closed {
		return async.ErrClosed
	}
	candidate := *e.document
	candidate.Nodes = append([]scene3d.DocumentNode(nil), candidate.Nodes...)
	found := false
	for i, old := range candidate.Nodes {
		if old.ID == node.ID {
			if old.Mesh != node.Mesh {
				return scene3d.ErrUnsupported
			}
			if reflect.DeepEqual(old, node) {
				return nil
			}
			node.Transform = append([]float64(nil), node.Transform...)
			candidate.Nodes[i] = node
			found = true
			break
		}
	}
	if !found {
		return scene3d.ErrNode
	}
	s, err := candidate.Build(context.Background())
	if err != nil {
		return err
	}
	e.document = &candidate
	e.scene = s
	e.modified = true
	e.revision++
	e.preview.SetScene(s)
	e.rebuildTree()
	e.SelectNode(node.ID)
	return nil
}

// SetBinding inserts/replaces one node/property channel. The SDK rejects
// invalid tags/types, duplicate destinations, non-finite values and group color
// targets. The editor does not require a running TagDB or activate the binding.
func (e *SceneDocumentEditor) SetBinding(spec scene3d.Binding) error {
	if e.closed {
		return async.ErrClosed
	}
	next := append([]scene3d.Binding(nil), e.document.Bindings...)
	found := false
	for i, b := range next {
		if b.Node == spec.Node && b.Property == spec.Property {
			if b == spec {
				return nil
			}
			next[i] = spec
			found = true
			break
		}
	}
	if !found {
		next = append(next, spec)
	}
	if _, err := scene3d.NewBindingSet(e.scene, next); err != nil {
		return err
	}
	e.document.Bindings = next
	e.modified = true
	e.revision++
	e.rebuildBindings()
	e.syncActions()
	return nil
}
func (e *SceneDocumentEditor) RemoveBinding(node string, property scene3d.BindingProperty) bool {
	if e.closed {
		return false
	}
	for i, b := range e.document.Bindings {
		if b.Node == node && b.Property == property {
			e.document.Bindings = append(e.document.Bindings[:i:i], e.document.Bindings[i+1:]...)
			e.modified = true
			e.revision++
			e.rebuildBindings()
			e.syncActions()
			return true
		}
	}
	return false
}
func (e *SceneDocumentEditor) removeSelectedBinding() {
	i := e.bindings.ActiveIndex()
	if i < 0 || i >= e.bindings.Count() {
		return
	}
	if p, ok := e.bindings.Item(i).Data.(scene3d.BindingProperty); ok {
		e.RemoveBinding(e.selected, p)
	}
}
func (e *SceneDocumentEditor) SizeHints() SizeHints { return SizeHints{Width: 960, Height: 560} }
func (e *SceneDocumentEditor) Layout() {
	if e.status == nil {
		return
	}
	w, h := e.Size()
	w = max(0, w)
	h = max(0, h)
	gap := 8.0
	row := max(34.0, Theme().ItemHeight+8)
	// Small hosts wrap actions instead of putting text over the preview.
	cols := 4
	if w < 760 {
		cols = 2
	}
	if w < 360 {
		cols = 1
	}
	rows := (len(e.buttons) + cols - 1) / cols
	bh := min(row, h/float64(max(1, rows)))
	cell := max(0, (w-float64(cols-1)*gap)/float64(cols))
	for i, b := range e.buttons {
		b.SetBounds(float64(i%cols)*(cell+gap), float64(i/cols)*(bh+gap), cell, bh)
	}
	top := min(h, float64(rows)*(bh+gap))
	bottom := min(row, max(0, h-top))
	body := max(0, h-top-bottom-gap)
	left := max(0, (w-gap)*.35)
	rightX := min(w, left+gap)
	e.tree.SetBounds(0, top, left, body*.62)
	e.fitTreeColumn()
	e.bindings.SetBounds(0, top+body*.62+min(gap, body*.38), left, max(0, body*.38-gap))
	e.preview.SetBounds(rightX, top, max(0, w-rightX), body)
	e.status.SetBounds(0, max(top, h-bottom), w, bottom)
}

// O(1) parent/index lookup keeps deep CAD trees from recursively rescanning.
type sceneDocumentTree struct {
	GuiModel
	nodes    map[string]scene3d.DocumentNode
	parents  map[string]string
	children map[string][]string
	indexes  map[string]ModelIndex
}

func newSceneDocumentTree(nodes []scene3d.DocumentNode) *sceneDocumentTree {
	m := &sceneDocumentTree{nodes: make(map[string]scene3d.DocumentNode), parents: make(map[string]string), children: make(map[string][]string), indexes: make(map[string]ModelIndex)}
	m.Init(m)
	for _, n := range nodes {
		m.nodes[n.ID] = n
		m.parents[n.ID] = n.Parent
		row := len(m.children[n.Parent])
		m.children[n.Parent] = append(m.children[n.Parent], n.ID)
		m.indexes[n.ID] = ModelIndex{Row: row, Param: n.ID, Model: m}
	}
	return m
}
func (m *sceneDocumentTree) Index(row, col int, parent ModelIndex) ModelIndex {
	if col != 0 {
		return ModelIndex{}
	}
	p, _ := parent.Param.(string)
	ids := m.children[p]
	if row < 0 || row >= len(ids) {
		return ModelIndex{}
	}
	return m.indexes[ids[row]]
}
func (m *sceneDocumentTree) Parent(index ModelIndex) ModelIndex {
	id, _ := index.Param.(string)
	return m.indexes[m.parents[id]]
}
func (m *sceneDocumentTree) RowCount(parent ModelIndex) int {
	id, _ := parent.Param.(string)
	return len(m.children[id])
}
func (m *sceneDocumentTree) ColCount() int              { return 1 }
func (m *sceneDocumentTree) Flags(ModelIndex) ItemFlags { return ItemIsSelectable | ItemIsEnabled }
func (m *sceneDocumentTree) HeaderData(_ int, _ bool, role ItemDataRole) interface{} {
	if role == DisplayRole {
		return i18n.T("Scene nodes")
	}
	return nil
}
func (m *sceneDocumentTree) Data(index ModelIndex, role ItemDataRole) interface{} {
	id, _ := index.Param.(string)
	n := m.nodes[id]
	switch role {
	case DisplayRole:
		if n.Name != "" {
			return n.Name
		}
		return id
	case ToolTipRole:
		if n.Name != "" {
			return n.Name + " · " + id
		}
		return id
	case DecorationRole:
		if n.Mesh != "" {
			return LoadIcon("preview")
		}
		return LoadIcon("folder")
	}
	return nil
}

package gui

import (
	"context"

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
	document            *scene3d.Document
	session             *scene3d.EditSession
	nodeMenu            *Menu
	primitiveMeshes     map[string]*scene3d.Mesh
	primitiveIDs        map[string]string
	scene               *scene3d.Scene
	tree                *TreeView
	model               *sceneDocumentTree
	preview             *SceneView
	bindings            *ListWidget
	status              *Label
	buttons             []*Button
	selected            string
	modified, closed    bool
	translationEnabled  bool
	translationSnapStep float64
	revision            uint64
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
	session, err := scene3d.NewEditSession(context.Background(), d)
	if err != nil {
		return nil, err
	}
	s, _ := session.Scene()
	owned, _ := session.Document()
	e := &SceneDocumentEditor{document: owned, scene: s, session: session, primitiveMeshes: make(map[string]*scene3d.Mesh), primitiveIDs: make(map[string]string)}
	e.Widget.Init(e)
	e.tree = NewTreeView()
	e.tree.SetParent(e)
	e.preview = NewSceneView()
	e.preview.SetParent(e)
	e.preview.SetScene(s)
	e.preview.translation = &sceneTranslationTool{editor: e, axis: -1}
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
		{"Node actions", "menu", func() {}},
		{"Undo", "edit-undo", func() { e.report(e.Undo()) }},
		{"Redo", "edit-redo", func() { e.report(e.Redo()) }},
		{"Move node", "transform-move", func() { e.SetTranslationEnabled(!e.TranslationEnabled()) }},
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
	SetToolTip(e.buttons[7], i18n.T("Move on X/Y/Z; Shift snaps to 1 unit; Escape cancels drag"))
	e.initNodeMenu()
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
	for _, b := range e.buttons {
		b.HideSubPopup()
	}
	e.session.Close()
	clear(e.primitiveMeshes)
	clear(e.primitiveIDs)
	e.preview.Close()
	e.syncActions()
}
func (e *SceneDocumentEditor) SelectNode(id string) bool {
	if e.closed {
		return false
	}
	if e.preview.translation != nil {
		e.preview.translation.cancel()
	}
	if _, ok := e.scene.Node(id); !ok {
		return false
	}
	e.selected = id
	e.preview.selected = id
	e.preview.translation.sync()
	e.preview.Update()
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
	// Group-only or fully hidden scenes have no bounds to fit. Snapshot is
	// already cached by the edit session; selection changes do not walk geometry.
	canFit := false
	if !e.closed && e.scene != nil {
		snapshot, err := e.scene.Snapshot()
		canFit = err == nil && snapshot.InstanceCount() > 0
	}
	e.buttons[3].SetEnabled(canFit)
	if len(e.buttons) >= 7 {
		e.buttons[4].SetEnabled(!e.closed)
		e.buttons[5].SetEnabled(e.CanUndo())
		e.buttons[6].SetEnabled(e.CanRedo())
	}
	if len(e.buttons) >= 8 {
		e.buttons[7].SetEnabled(selected)
		e.buttons[7].Action().SetChecked(e.translationEnabled)
	}
	if e.nodeMenu != nil {
		for i, item := range e.nodeMenu.items {
			if b, ok := item.(*Button); ok {
				b.SetEnabled(!e.closed && (i == 0 || selected))
			}
		}
	}
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

// UpdateNode preserves resource ownership and validates the whole graph/bindings.
// Existing node edits retain the original mesh ID; AddMeshNode creates resources.
func (e *SceneDocumentEditor) UpdateNode(node scene3d.DocumentNode) error {
	if e.closed {
		return async.ErrClosed
	}
	for _, old := range e.document.Nodes {
		if old.ID == node.ID && old.Mesh != node.Mesh {
			return scene3d.ErrUnsupported
		}
	}
	if err := e.session.UpdateNode(context.Background(), node); err != nil {
		return err
	}
	e.syncSession(node.ID)
	return nil
}

// SetBinding validates inert metadata; it never activates a driver/subscription.
func (e *SceneDocumentEditor) SetBinding(spec scene3d.Binding) error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.SetBinding(context.Background(), spec); err != nil {
		return err
	}
	e.syncSession(e.selected)
	return nil
}
func (e *SceneDocumentEditor) RemoveBinding(node string, property scene3d.BindingProperty) bool {
	if e.closed {
		return false
	}
	removed, err := e.session.RemoveBinding(context.Background(), node, property)
	if err != nil {
		e.report(err)
		return false
	}
	e.syncSession(e.selected)
	return removed
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
	if w < 300 {
		cols = 1
	}
	rows := (len(e.buttons) + cols - 1) / cols
	gx := min(gap, w/float64(max(1, cols)))
	gy := min(gap, h/float64(max(1, rows)))
	bh := min(row, max(0, h-float64(rows-1)*gy)/float64(max(1, rows)))
	cell := max(0, (w-float64(cols-1)*gx)/float64(cols))
	for i, b := range e.buttons {
		b.SetBounds(float64(i%cols)*(cell+gx), float64(i/cols)*(bh+gy), cell, bh)
	}
	top := min(h, float64(rows)*bh+float64(rows-1)*gy+gap)
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

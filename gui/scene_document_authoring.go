package gui

import (
	"context"
	"fmt"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

// syncSession publishes a validated staged edit to the existing owned widgets.
// Geometry is copied for GUI document storage only when a resource is first
// introduced, never on every name/binding change or history traversal.
func (e *SceneDocumentEditor) syncSession(selection string) {
	e.preview.translation.cancel()
	if e.revision == e.session.Revision() {
		return
	}
	current := make(map[string]scene3d.DocumentMesh, len(e.document.Meshes))
	for _, m := range e.document.Meshes {
		current[m.ID] = m
	}
	meshes := make([]scene3d.DocumentMesh, 0, len(current))
	for _, id := range e.session.MeshIDs() {
		m, ok := current[id]
		if !ok {
			mesh, _ := e.session.Mesh(id)
			m = scene3d.DocumentMesh{ID: id, Vertices: make([]scene3d.Vec3, mesh.VertexCount()), Indices: mesh.CopyIndices(nil)}
			for i := range m.Vertices {
				m.Vertices[i], _ = mesh.Vertex(i)
			}
		}
		meshes = append(meshes, m)
	}
	e.document.Meshes = meshes
	e.document.Nodes = e.session.Nodes()
	e.document.Bindings = e.session.Bindings()
	e.document.Warnings = e.session.Warnings()
	e.scene, _ = e.session.Scene()
	e.modified = e.session.Modified()
	e.revision = e.session.Revision()
	target := e.preview.target
	e.preview.SetScene(e.scene)
	e.preview.target = target
	e.rebuildTree()
	if !e.SelectNode(selection) {
		e.selected = ""
		e.preview.selected = ""
		if len(e.document.Nodes) > 0 {
			e.SelectNode(e.document.Nodes[0].ID)
		} else {
			e.rebuildBindings()
			e.syncActions()
		}
	}
	e.preview.translation.sync()
	if e.libraryVisible && e.devicePlacementMode != SceneDeviceGround {
		e.report(e.PrepareDeviceSurface(context.Background()))
	}
}
func (e *SceneDocumentEditor) CanUndo() bool { return !e.closed && e.session.CanUndo() }
func (e *SceneDocumentEditor) CanRedo() bool { return !e.closed && e.session.CanRedo() }
func (e *SceneDocumentEditor) Undo() error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.Undo(context.Background()); err != nil {
		return err
	}
	e.syncSession(e.selected)
	return nil
}
func (e *SceneDocumentEditor) Redo() error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.Redo(context.Background()); err != nil {
		return err
	}
	e.syncSession(e.selected)
	return nil
}
func (e *SceneDocumentEditor) AddNode(node scene3d.DocumentNode) error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.AddNode(context.Background(), node); err != nil {
		return err
	}
	e.syncSession(node.ID)
	return nil
}
func (e *SceneDocumentEditor) AddMeshNode(node scene3d.DocumentNode, id string, mesh *scene3d.Mesh) error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.AddMeshNode(context.Background(), node, id, mesh); err != nil {
		return err
	}
	e.syncSession(node.ID)
	return nil
}
func (e *SceneDocumentEditor) DuplicateSubtree(id string, opts scene3d.DuplicateOptions) (map[string]string, error) {
	if e.closed {
		return nil, async.ErrClosed
	}
	ids, err := e.session.DuplicateSubtree(context.Background(), id, opts)
	if err != nil {
		return nil, err
	}
	e.syncSession(opts.RootID)
	return ids, nil
}
func (e *SceneDocumentEditor) RemoveSubtree(id string) error {
	if e.closed {
		return async.ErrClosed
	}
	n, ok := e.scene.Node(id)
	if !ok {
		return scene3d.ErrNode
	}
	if err := e.session.RemoveSubtree(context.Background(), id); err != nil {
		return err
	}
	e.syncSession(n.Parent)
	return nil
}
func (e *SceneDocumentEditor) ReparentNode(id, parent string, keepWorld bool) error {
	if e.closed {
		return async.ErrClosed
	}
	if err := e.session.Reparent(context.Background(), id, parent, keepWorld); err != nil {
		return err
	}
	e.syncSession(id)
	return nil
}
func (e *SceneDocumentEditor) nextNodeID(prefix string) string {
	for i := 1; i <= scene3d.MaxNodes+1; i++ {
		id := fmt.Sprintf("%s-%d", prefix, i)
		if _, exists := e.scene.Node(id); !exists {
			return id
		}
	}
	return ""
}
func (e *SceneDocumentEditor) primitive(kind string) (string, *scene3d.Mesh, error) {
	mesh := e.primitiveMeshes[kind]
	if mesh != nil {
		current, exists := e.session.Mesh(e.primitiveIDs[kind])
		if !exists || current == mesh {
			return e.primitiveIDs[kind], mesh, nil
		}
	} else {
		var err error
		switch kind {
		case "box":
			mesh = scene3d.Box()
		case "cylinder":
			mesh, err = scene3d.Cylinder(1, 2, 32)
		default:
			return "", nil, scene3d.ErrUnsupported
		}
		if err != nil {
			return "", nil, err
		}
	}
	for i := 1; i <= scene3d.MaxNodes+1; i++ {
		id := fmt.Sprintf("builtin-%s-%d", kind, i)
		if _, exists := e.session.Mesh(id); !exists {
			e.primitiveMeshes[kind] = mesh
			e.primitiveIDs[kind] = id
			return id, mesh, nil
		}
	}
	return "", nil, scene3d.ErrLimit
}
func (e *SceneDocumentEditor) initNodeMenu() {
	menu := NewPopupMenu()
	e.nodeMenu = menu
	for _, spec := range []struct {
		label, icon string
		run         func()
	}{
		{"Add node...", "plus", e.addNodeDialog},
		{"Duplicate subtree...", "edit-copy", e.duplicateNodeDialog},
		{"Change parent...", "folder", e.reparentNodeDialog},
		{"Delete subtree...", "close", e.deleteNodeDialog},
		{"Device library", "project", func() { e.SetDeviceLibraryVisible(!e.DeviceLibraryVisible()) }},
	} {
		b := menu.AddButton1(i18n.T(spec.label), LoadIcon(spec.icon))
		b.Action().BindFunc0(spec.run)
		SetToolTip(b, i18n.T(spec.label))
	}
	e.buttons[4].SetSubPopup(menu)
	e.buttons[4].SetSubPopupCallback(func(IButton) { e.syncActions() })
}

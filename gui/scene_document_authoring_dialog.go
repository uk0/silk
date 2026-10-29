package gui

import (
	"strings"

	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

func (e *SceneDocumentEditor) authoringDialogCurrent(dlg *Dialog, revision uint64) bool {
	if e.closed || e.revision != revision {
		ShowToast(dlg, i18n.T("Scene changed; reopen the editor"), 4000, ToastWarning)
		return false
	}
	return true
}
func (e *SceneDocumentEditor) finishAuthoringDialog(dlg *Dialog, err error) {
	if err != nil {
		ShowToast(dlg, err.Error(), 4000, ToastWarning)
		return
	}
	dlg.onBtnClick(dialogResultKey(DialogOK))
}
func (e *SceneDocumentEditor) addNodeDialog() {
	if e.closed {
		return
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Add scene node", e)
	id := sceneEditorText(form, "Node ID", "scene.add.id", e.nextNodeID("node"))
	name := sceneEditorText(form, "Name", "scene.add.name", "")
	parent := sceneEditorText(form, "Parent ID (empty = root)", "scene.add.parent", e.selected)
	kind := NewComboBox()
	kind.SetEditWidget(nil)
	kind.SetExtraData("scene.add.kind")
	for _, label := range []string{"Group", "Box", "Cylinder"} {
		kind.Append(ListItem{Text: i18n.T(label)})
	}
	kind.SetActiveIndex(0)
	form.AddRow(i18n.T("Node type"), kind)
	xyz := [3]*Edit{}
	for i, axis := range []string{"X", "Y", "Z"} {
		xyz[i] = sceneEditorText(form, "Local position "+axis, "scene.add."+axis, "0")
	}
	form.AddRow(i18n.T("Geometry"), NewLabel(i18n.T("Shared mesh; no driver or tag activated")))
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Add node"), DialogOK)
	apply.Action().BindFunc0(func() {
		if !e.authoringDialogCurrent(dlg, revision) {
			return
		}
		n := scene3d.DocumentNode{ID: strings.TrimSpace(id.Text()), Name: name.Text(), Parent: strings.TrimSpace(parent.Text())}
		m := scene3d.Identity()
		for i, field := range xyz {
			v, err := sceneEditorFloat(field.Text())
			if err != nil {
				e.finishAuthoringDialog(dlg, err)
				return
			}
			m[12+i] = v
		}
		if m != scene3d.Identity() {
			n.Transform = m[:]
		}
		var err error
		switch kind.ActiveIndex() {
		case 0:
			err = e.AddNode(n)
		case 1, 2:
			k := "box"
			if kind.ActiveIndex() == 2 {
				k = "cylinder"
			}
			var mesh *scene3d.Mesh
			var meshID string
			meshID, mesh, err = e.primitive(k)
			if err == nil {
				err = e.AddMeshNode(n, meshID, mesh)
			}
		default:
			err = scene3d.ErrNode
		}
		e.finishAuthoringDialog(dlg, err)
	})
	sizeSceneEditorForm(dlg, form)
	dlg.SetInitialFocus(id)
	_ = showSceneDocumentDialog(dlg)
}
func (e *SceneDocumentEditor) duplicateNodeDialog() {
	if e.closed || e.selected == "" {
		return
	}
	source, ok := e.scene.Node(e.selected)
	if !ok {
		return
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Duplicate scene subtree", e)
	form.AddRow(i18n.T("Source ID"), NewLabel(source.ID))
	id := sceneEditorText(form, "New root ID", "scene.duplicate.id", e.nextNodeID("copy"))
	name := sceneEditorText(form, "Name", "scene.duplicate.name", source.Name)
	offset := sceneEditorText(form, "Local X offset", "scene.duplicate.offset", "2.5")
	bindings := NewCheckBox()
	bindings.SetExtraData("scene.duplicate.bindings")
	bindings.SetText(i18n.T("Copy bindings (same process points)"))
	form.AddRow(i18n.T("Bindings"), bindings)
	warning := NewLabel(i18n.T("Off by default; copying reuses original tag names"))
	SetToolTip(warning, warning.Text())
	form.AddRow(i18n.T("Note"), warning)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Duplicate subtree"), DialogOK)
	apply.Action().BindFunc0(func() {
		if !e.authoringDialogCurrent(dlg, revision) {
			return
		}
		x, err := sceneEditorFloat(offset.Text())
		if err == nil {
			_, err = e.DuplicateSubtree(source.ID, scene3d.DuplicateOptions{RootID: strings.TrimSpace(id.Text()), RootName: name.Text(), Offset: scene3d.Vec3{X: x}, CopyBindings: bindings.IsChecked()})
		}
		e.finishAuthoringDialog(dlg, err)
	})
	sizeSceneEditorForm(dlg, form)
	dlg.SetInitialFocus(id)
	_ = showSceneDocumentDialog(dlg)
}
func (e *SceneDocumentEditor) reparentNodeDialog() {
	if e.closed || e.selected == "" {
		return
	}
	node, ok := e.scene.Node(e.selected)
	if !ok {
		return
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Change scene parent", e)
	form.AddRow(i18n.T("Node ID"), NewLabel(node.ID))
	parent := sceneEditorText(form, "Parent ID (empty = root)", "scene.reparent.parent", node.Parent)
	keep := NewCheckBox()
	keep.SetExtraData("scene.reparent.keepWorld")
	keep.SetText(i18n.T("Preserve authored world position"))
	keep.SetChecked(true)
	form.AddRow(i18n.T("Transform"), keep)
	form.AddRow(i18n.T("Note"), NewLabel(i18n.T("Cycles and unsafe inverse transforms are rejected")))
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Change parent"), DialogOK)
	apply.Action().BindFunc0(func() {
		if e.authoringDialogCurrent(dlg, revision) {
			e.finishAuthoringDialog(dlg, e.ReparentNode(node.ID, strings.TrimSpace(parent.Text()), keep.IsChecked()))
		}
	})
	sizeSceneEditorForm(dlg, form)
	dlg.SetInitialFocus(parent)
	_ = showSceneDocumentDialog(dlg)
}
func (e *SceneDocumentEditor) deleteNodeDialog() {
	if e.closed || e.selected == "" {
		return
	}
	id := e.selected
	// The document can be unordered. An explicit stack visits each child once,
	// including hidden descendants, without recursive depth on imported CAD trees.
	removed := map[string]bool{}
	pending := []string{id}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		removed[n] = true
		pending = append(pending, e.model.children[n]...)
	}
	bindings := 0
	for _, b := range e.document.Bindings {
		if removed[b.Node] {
			bindings++
		}
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Delete scene subtree", e)
	form.AddRow(i18n.T("Node ID"), NewLabel(id))
	form.AddRow(i18n.T("Remove"), NewLabel(i18n.Tf("%d nodes · %d bindings", len(removed), bindings)))
	form.AddRow(i18n.T("Undo"), NewLabel(i18n.T("Mesh resources retained; deletion can be undone")))
	cancel := dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Delete subtree"), DialogOK)
	apply.Action().BindFunc0(func() {
		if e.authoringDialogCurrent(dlg, revision) {
			e.finishAuthoringDialog(dlg, e.RemoveSubtree(id))
		}
	})
	dlg.SetDefaultButton(cancel)
	dlg.SetInitialFocus(cancel)
	sizeSceneEditorForm(dlg, form)
	_ = showSceneDocumentDialog(dlg)
}

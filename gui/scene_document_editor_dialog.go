package gui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

var showSceneDocumentDialog = (*Dialog).ShowModal

// ShowSceneDocumentEditor stages edits in a modal inspector. Only Use changes
// calls commit, with an owned document copy. A failing commit keeps the dialog
// open. Cancel never modifies the caller's document. The host must check its
// source revision in commit and supply its own undo/save transaction.
func ShowSceneDocumentEditor(parent IWidget, document *scene3d.Document, commit func(*scene3d.Document) error) error {
	if commit == nil {
		return ErrUIInvalid
	}
	e, err := NewSceneDocumentEditor(document)
	if err != nil {
		return err
	}
	defer e.Close()
	dlg := NewDialog(i18n.T("3D Scene Editor"), parent)
	dlg.SetContent(e)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	use := dlg.AddButton(i18n.T("Use changes"), DialogOK)
	use.Action().BindFunc0(func() {
		if e.modified {
			d, err := e.Document()
			if err == nil {
				err = commit(d)
			}
			if err != nil {
				e.report(err)
				return
			}
		}
		dlg.onBtnClick(dialogResultKey(DialogOK))
	})
	dlg.SetSize(1000, 660)
	dlg.SetInitialFocus(e.tree)
	_ = showSceneDocumentDialog(dlg)
	return dlg.LastError()
}

func sceneEditorForm(title string, parent IWidget) (*Dialog, *FormLayout) {
	dlg := NewDialog(i18n.T(title), parent)
	form := NewFormLayout()
	form.SetRowHeight(34)
	form.SetSpacing(6)
	form.SetLabelWidth(150)
	dlg.SetContent(form)
	return dlg, form
}
func sceneEditorText(form *FormLayout, label, id, value string) *Edit {
	e := NewEdit()
	e.SetExtraData(id)
	e.SetText(value)
	form.AddRow(i18n.T(label), e)
	return e
}
func sizeSceneEditorForm(dlg *Dialog, form *FormLayout) {
	lw, rh := 150.0, 34.0
	for _, r := range form.rows {
		a, b := formRowHints(r)
		lw = max(lw, a.Width+8)
		rh = max(rh, max(a.Height, b.Height)+8)
	}
	form.SetLabelWidth(math.Ceil(lw))
	form.SetRowHeight(math.Ceil(rh))
	dlg.SetSize(max(620, lw+350), max(260, form.SizeHints().Height+90))
}
func sceneEditorFloat(text string) (float64, error) {
	x, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
		return 0, fmt.Errorf("%s", i18n.T("Enter a finite number"))
	}
	return x, nil
}
func sceneEditorColor(text string) (color.RGBA, error) {
	text = strings.TrimSpace(text)
	if len(text) != 7 || text[0] != '#' {
		return color.RGBA{}, fmt.Errorf("%s", i18n.T("Use an opaque color in #RRGGBB format"))
	}
	x, err := strconv.ParseUint(text[1:], 16, 24)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: byte(x >> 16), G: byte(x >> 8), B: byte(x), A: 255}, nil
}
func sceneColorText(c color.RGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func (e *SceneDocumentEditor) editNode() {
	if e.closed {
		return
	}
	var n scene3d.DocumentNode
	found := false
	for _, node := range e.document.Nodes {
		if node.ID == e.selected {
			n = node
			found = true
			break
		}
	}
	if !found {
		return
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Edit scene node", e)
	form.AddRow("ID", NewLabel(n.ID))
	name := sceneEditorText(form, "Name", "scene.node.name", n.Name)
	parent := sceneEditorText(form, "Parent ID (empty = root)", "scene.node.parent", n.Parent)
	matrix := scene3d.Identity()
	if len(n.Transform) == 16 {
		copy(matrix[:], n.Transform)
	}
	xyz := [3]*Edit{}
	for i, axis := range []string{"X", "Y", "Z"} {
		xyz[i] = sceneEditorText(form, "Local position "+axis, "scene.node."+axis, strconv.FormatFloat(matrix[12+i], 'g', -1, 64))
	}
	actual, _ := e.scene.Node(n.ID)
	tint := sceneEditorText(form, "Color (#RRGGBB)", "scene.node.color", sceneColorText(actual.Color))
	tint.SetEnabled(actual.Mesh != nil)
	SetToolTip(tint, i18n.T("Color affects this node's mesh, not child materials"))
	hidden := NewCheckBox()
	hidden.SetExtraData("scene.node.hidden")
	hidden.SetText(i18n.T("Hide this subtree"))
	hidden.SetChecked(n.Hidden)
	form.AddRow(i18n.T("Visibility"), hidden)
	form.AddRow(i18n.T("Transform"), NewLabel(i18n.T("Authored rotation / scale preserved")))
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Apply node"), DialogOK)
	apply.Action().BindFunc0(func() {
		if e.closed || e.revision != revision {
			ShowToast(dlg, i18n.T("Scene changed; reopen the editor"), 4000, ToastWarning)
			return
		}
		next := n
		next.Name = name.Text()
		next.Parent = strings.TrimSpace(parent.Text())
		next.Hidden = hidden.IsChecked()
		m := matrix
		for i, field := range xyz {
			v, err := sceneEditorFloat(field.Text())
			if err != nil {
				ShowToast(dlg, err.Error(), 4000, ToastWarning)
				return
			}
			m[12+i] = v
		}
		if m != matrix {
			next.Transform = append([]float64(nil), m[:]...)
		}
		c, err := sceneEditorColor(tint.Text())
		if err != nil {
			ShowToast(dlg, err.Error(), 4000, ToastWarning)
			return
		}
		if c != actual.Color {
			next.Color = c
		}
		if err := e.UpdateNode(next); err != nil {
			ShowToast(dlg, err.Error(), 4000, ToastWarning)
			return
		}
		dlg.onBtnClick(dialogResultKey(DialogOK))
	})
	sizeSceneEditorForm(dlg, form)
	dlg.SetInitialFocus(name)
	_ = showSceneDocumentDialog(dlg)
}

var sceneEditorProperties = []scene3d.BindingProperty{
	scene3d.PositionX, scene3d.PositionY, scene3d.PositionZ, scene3d.RotationX, scene3d.RotationY, scene3d.RotationZ,
	scene3d.ScaleX, scene3d.ScaleY, scene3d.ScaleZ, scene3d.Visibility, scene3d.AlarmColor,
}

func (e *SceneDocumentEditor) editBinding() {
	if e.closed || e.selected == "" {
		return
	}
	spec := scene3d.Binding{Node: e.selected, Property: scene3d.PositionX, Gain: 1}
	index := e.bindings.ActiveIndex()
	if index >= 0 && index < e.bindings.Count() {
		p, _ := e.bindings.Item(index).Data.(scene3d.BindingProperty)
		for _, b := range e.document.Bindings {
			if b.Node == e.selected && b.Property == p {
				spec = b
				break
			}
		}
	}
	revision := e.revision
	dlg, form := sceneEditorForm("Set scene binding", e)
	form.AddRow(i18n.T("Node ID"), NewLabel(spec.Node))
	property := NewComboBox()
	property.SetEditWidget(nil)
	property.SetExtraData("scene.binding.property")
	selected := 0
	for i, p := range sceneEditorProperties {
		property.Append(ListItem{Text: string(p)})
		if p == spec.Property {
			selected = i
		}
	}
	property.SetActiveIndex(selected)
	form.AddRow(i18n.T("Property"), property)
	tag := sceneEditorText(form, "Tag name", "scene.binding.tag", spec.Tag)
	gain := sceneEditorText(form, "Gain", "scene.binding.gain", strconv.FormatFloat(spec.Gain, 'g', -1, 64))
	offset := sceneEditorText(form, "Offset", "scene.binding.offset", strconv.FormatFloat(spec.Offset, 'g', -1, 64))
	off, on := spec.FalseColor, spec.TrueColor
	if off.A == 0 {
		n, _ := e.scene.Node(spec.Node)
		off = n.Color
	}
	if on.A == 0 {
		on = color.RGBA{R: 230, G: 65, B: 65, A: 255}
	}
	falseColor := sceneEditorText(form, "False color", "scene.binding.false", sceneColorText(off))
	trueColor := sceneEditorText(form, "True color", "scene.binding.true", sceneColorText(on))
	hint := NewLabel("")
	form.AddRow(i18n.T("Mapping"), hint)
	syncFields := func() {
		p := property.ActiveIndex()
		boolean := p >= 9
		gain.SetEnabled(!boolean)
		offset.SetEnabled(!boolean)
		falseColor.SetEnabled(p == 10)
		trueColor.SetEnabled(p == 10)
		message := "value × gain + offset; rotation in radians"
		if boolean {
			message = "Boolean tag; no numeric conversion"
		}
		hint.SetText(i18n.T(message))
		SetToolTip(hint, i18n.T(message))
	}
	property.SigSelectionChanged(func(_ interface{}, _ int) { syncFields() })
	syncFields()
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Set binding"), DialogOK)
	apply.Action().BindFunc0(func() {
		if e.closed || e.revision != revision {
			ShowToast(dlg, i18n.T("Scene changed; reopen the editor"), 4000, ToastWarning)
			return
		}
		i := property.ActiveIndex()
		if i < 0 || i >= len(sceneEditorProperties) {
			return
		}
		next := scene3d.Binding{Node: spec.Node, Tag: strings.TrimSpace(tag.Text()), Property: sceneEditorProperties[i]}
		var err error
		if i < 9 {
			next.Gain, err = sceneEditorFloat(gain.Text())
			if err == nil {
				next.Offset, err = sceneEditorFloat(offset.Text())
			}
		}
		if i == 10 {
			next.FalseColor, err = sceneEditorColor(falseColor.Text())
			if err == nil {
				next.TrueColor, err = sceneEditorColor(trueColor.Text())
			}
		}
		if err == nil {
			err = e.SetBinding(next)
		}
		if err != nil {
			ShowToast(dlg, err.Error(), 4000, ToastWarning)
			return
		}
		dlg.onBtnClick(dialogResultKey(DialogOK))
	})
	sizeSceneEditorForm(dlg, form)
	dlg.SetInitialFocus(tag)
	_ = showSceneDocumentDialog(dlg)
}

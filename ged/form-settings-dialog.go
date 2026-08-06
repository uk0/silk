package ged

import (
	"strconv"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// formSettings is everything the 表单设置 dialog edits, in one value so the
// parse and apply steps stay pure and testable without a live dialog.
type formSettings struct {
	Title  string
	Width  float64
	Height float64
	Grid   GridModel
}

// readFormSettings snapshots the scene into the dialog's value struct.
func readFormSettings(scene *GedScene) formSettings {
	_, _, w, h := scene.Bounds()
	return formSettings{
		Title:  scene.FormTitle(),
		Width:  w,
		Height: h,
		Grid:   scene.Grid(),
	}
}

// parseFormSettings turns the dialog's raw fields into a formSettings. A
// numeric field that does not parse keeps its current value instead of
// becoming zero — the property sheet does the same, leaving a numeric property
// untouched when the scanner rejects the text (prop.parsePropertyValue). A
// field that does parse but sits outside its legal range is clamped, so
// "0.1" becomes the minimum pitch rather than a silent no-op.
func parseFormSettings(cur formSettings, title, width, height, pitch string, visible, snap bool) formSettings {
	next := cur
	next.Title = title
	if v, ok := parseMm(width); ok {
		next.Width = clampFormSize(v)
	}
	if v, ok := parseMm(height); ok {
		next.Height = clampFormSize(v)
	}
	if v, ok := parseMm(pitch); ok {
		next.Grid.Pitch = clampGridPitch(v)
	}
	next.Grid.Visible = visible
	next.Grid.Snap = snap
	return next
}

// applyFormSettings writes next onto the scene. Title and size are scene state
// the user can undo, so they go on the undo stack as one step; the grid prefs
// are view preferences that happen to live with the document and are applied
// directly. Reports whether anything changed.
func applyFormSettings(scene *GedScene, next formSettings) bool {
	cur := readFormSettings(scene)
	changed := cur != next

	cmd := gui.NewCompoundCommand()
	cmd.SetText("Form Settings")
	pushable := false
	if next.Title != cur.Title {
		cmd.Append(&formTitleCommand{scene: scene, title: next.Title})
		pushable = true
	}
	if next.Width != cur.Width || next.Height != cur.Height {
		rc := scene.Bounds1()
		rc.Width, rc.Height = next.Width, next.Height
		resize := graph.NewResizeCommand()
		resize.AddItem(scene, rc)
		cmd.Append(resize)
		pushable = true
	}
	if pushable {
		scene.PushCommand(cmd)
	}
	scene.SetGrid(next.Grid)
	return changed
}

// formTitleCommand makes a form-title edit undoable. graph ships commands for
// geometry (add/delete/move/resize) but none for the scene's own title, and
// prop.PropertyCommand — the undo step the property sheet's 标题 row produces —
// has no exported constructor.
type formTitleCommand struct {
	scene  *GedScene
	title  string
	isUndo bool
}

func (cmd *formTitleCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	cmd.swap()
	cmd.isUndo = true
}

func (cmd *formTitleCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	cmd.swap()
	cmd.isUndo = false
}

func (cmd *formTitleCommand) swap() {
	old := cmd.scene.FormTitle()
	cmd.scene.SetFormTitle(cmd.title)
	cmd.title = old
}

func (cmd *formTitleCommand) Text() string {
	return "Set Form Title"
}

// ShowFormSettingsDialog opens 表单设置 for the view's scene and applies the
// entered values on 确定. Reports whether anything was applied.
func ShowFormSettingsDialog(parent gui.IWidget, view *GedView) bool {
	if view == nil {
		return false
	}
	scene := view.GedScene()
	if scene == nil {
		return false
	}
	cur := readFormSettings(scene)

	dlg := gui.NewDialog("表单设置", parent)

	editTitle := gui.NewEdit()
	editTitle.SetText(cur.Title)
	editWidth := gui.NewEdit()
	editWidth.SetText(formatMm(cur.Width))
	editHeight := gui.NewEdit()
	editHeight.SetText(formatMm(cur.Height))
	editPitch := gui.NewEdit()
	editPitch.SetText(formatMm(cur.Grid.Pitch))
	chkVisible := gui.NewCheckBox()
	chkVisible.SetChecked(cur.Grid.Visible)
	chkSnap := gui.NewCheckBox()
	chkSnap.SetChecked(cur.Grid.Snap)

	form := gui.NewFormLayout()
	form.SetLabelWidth(110)
	form.SetSpacing(8)
	form.AddRow("标题", editTitle)
	form.AddRow("宽度 (mm)", editWidth)
	form.AddRow("高度 (mm)", editHeight)
	form.AddRow("网格间距 (mm)", editPitch)
	form.AddRow("显示网格", chkVisible)
	form.AddRow("对齐网格", chkSnap)

	dlg.SetContent(form)
	dlg.AddButton("确定", gui.DialogOK)
	dlg.AddButton("取消", gui.DialogCancel)
	dlg.SetSize(380, 320)

	if dlg.ShowModal() != gui.DialogOK {
		return false
	}

	next := parseFormSettings(cur, editTitle.Text(), editWidth.Text(), editHeight.Text(),
		editPitch.Text(), chkVisible.IsChecked(), chkSnap.IsChecked())
	changed := applyFormSettings(scene, next)
	view.Self().Update()
	return changed
}

// formatMm renders a millimetre value for an editable field: no trailing
// zeroes, so a 5mm pitch shows as "5" rather than "5.000000".
func formatMm(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

package ged

import (
	"strconv"
	"strings"

	"github.com/uk0/silk/gui"
)

// Dictionary table columns, in display order. There is one column per core.Meta
// field and nothing else: the dialog is an editor for that struct, so a column
// with no field behind it would be a promise the runtime cannot keep.
const (
	tagColName = iota
	tagColUnit
	tagColMin
	tagColMax
	tagColLoLo
	tagColLo
	tagColHi
	tagColHiHi
	tagColDesc
	tagColCount
)

var tagDictHeaders = [tagColCount]string{
	"名称", "单位", "最小值", "最大值", "LoLo", "Lo", "Hi", "HiHi", "描述",
}

// tagDictColWidths sizes the columns to their content; the table scrolls when
// the dialog (capped at 600px by gui.Dialog) is narrower than their sum.
var tagDictColWidths = [tagColCount]float64{110, 55, 60, 60, 55, 55, 55, 55, 130}

// tagDeclCells renders one declaration as a table row. Zeroes print as "0"
// rather than blank: core.Meta has no "unset" state — a limit is a float64, and
// core.limitSet decides what an armed limit means.
func tagDeclCells(d TagDecl) []string {
	return []string{
		d.Name,
		d.Meta.Unit,
		formatTagLimit(d.Meta.Min),
		formatTagLimit(d.Meta.Max),
		formatTagLimit(d.Meta.LoLo),
		formatTagLimit(d.Meta.Lo),
		formatTagLimit(d.Meta.Hi),
		formatTagLimit(d.Meta.HiHi),
		d.Meta.Desc,
	}
}

// setTagDeclCell applies one in-place cell edit and returns the updated
// declaration. A numeric cell that does not parse keeps its current value
// instead of collapsing to zero — the rule the form settings dialog and the
// property sheet already follow (prop.parsePropertyValue). Text cells are
// trimmed; an out-of-range column changes nothing.
func setTagDeclCell(d TagDecl, col int, text string) TagDecl {
	text = strings.TrimSpace(text)
	num := func(dst *float64) {
		if v, err := strconv.ParseFloat(text, 64); err == nil {
			*dst = v
		}
	}
	switch col {
	case tagColName:
		d.Name = text
	case tagColUnit:
		d.Meta.Unit = text
	case tagColMin:
		num(&d.Meta.Min)
	case tagColMax:
		num(&d.Meta.Max)
	case tagColLoLo:
		num(&d.Meta.LoLo)
	case tagColLo:
		num(&d.Meta.Lo)
	case tagColHi:
		num(&d.Meta.Hi)
	case tagColHiHi:
		num(&d.Meta.HiHi)
	case tagColDesc:
		d.Meta.Desc = text
	}
	return d
}

// formatTagLimit renders a limit for an editable cell: no trailing zeroes, so
// 10 shows as "10" rather than "10.000000".
func formatTagLimit(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// tagDeclFromCells rebuilds a declaration from one table row.
func tagDeclFromCells(cells []string) TagDecl {
	var d TagDecl
	for col, text := range cells {
		d = setTagDeclCell(d, col, text)
	}
	return d
}

// tagDictFromModel reads the dictionary back out of the table model, in the
// model's current row order. gui.SimpleTableModel is a SortableTableModel, so a
// header click reorders its rows in place; without re-reading them the dialog's
// declarations would keep the pre-sort order and every later edit would land on
// the wrong row.
func tagDictFromModel(model *gui.SimpleTableModel) []TagDecl {
	decls := make([]TagDecl, 0, model.RowCount())
	for row := 0; row < model.RowCount(); row++ {
		decls = append(decls, tagDeclFromCells(model.RowData(row)))
	}
	return decls
}

// applyTagDict writes the edited dictionary onto the scene, reporting whether
// anything changed. Unlike the grid preferences — which the form settings dialog
// deliberately keeps off the undo stack — the dictionary is document content: it
// changes the code the design generates, and the undo stack is what marks the
// document dirty for 保存 and auto-save (GedScene.IsClean).
func applyTagDict(scene *GedScene, next []TagDecl) bool {
	next = normalizeTagDict(next)
	if tagDictEqual(scene.TagDict(), next) {
		return false
	}
	scene.PushCommand(&tagDictCommand{scene: scene, decls: next})
	return true
}

// tagDictEqual compares two dictionaries element-wise. core.Meta is a plain
// comparable struct, so == is well defined per entry.
func tagDictEqual(a, b []TagDecl) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tagDictCommand makes a dictionary edit undoable by swapping the whole list,
// the shape formTitleCommand uses for the form title: graph ships commands for
// geometry only, and the scene's own document state has none.
type tagDictCommand struct {
	scene  *GedScene
	decls  []TagDecl
	isUndo bool
}

func (cmd *tagDictCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	cmd.swap()
	cmd.isUndo = true
}

func (cmd *tagDictCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	cmd.swap()
	cmd.isUndo = false
}

func (cmd *tagDictCommand) swap() {
	old := cmd.scene.TagDict()
	cmd.scene.SetTagDict(cmd.decls)
	cmd.decls = old
}

func (cmd *tagDictCommand) Text() string {
	return "Edit Tag Dictionary"
}

// ShowTagDictionaryDialog opens 标签字典 for the view's scene: the tags the
// design declares, with the unit / engineering range / alarm limits the generated
// app seeds them with, plus a 未声明 column naming the tags widgets are bound to
// that nothing declares. Applies on 确定 and reports whether anything changed.
func ShowTagDictionaryDialog(parent gui.IWidget, view *GedView) bool {
	if view == nil {
		return false
	}
	scene := view.GedScene()
	if scene == nil {
		return false
	}

	decls := append([]TagDecl(nil), scene.TagDict()...)
	referenced := sceneTagNames(scene)

	model := gui.NewSimpleTableModel(tagDictHeaders[:])
	for col, w := range tagDictColWidths {
		model.SetColumnWidth(col, w)
	}
	table := gui.NewTable()
	table.SetModel(model)
	table.SetCellsEditable(true)

	undeclared := gui.NewSimpleTableModel([]string{"未声明"})
	undeclared.SetColumnWidth(0, 160)
	hint := gui.NewTable()
	hint.SetModel(undeclared)

	// refreshHint re-runs the cross-check: what counts as undeclared changes with
	// every name typed into the table, so it cannot be computed once up front.
	refreshHint := func() {
		for undeclared.RowCount() > 0 {
			undeclared.RemoveRow(0)
		}
		for _, name := range undeclaredTags(decls, referenced) {
			undeclared.AddRow(name)
		}
		hint.Self().Update()
	}
	for _, d := range decls {
		model.AddRow(tagDeclCells(d)...)
	}
	refreshHint()

	table.SigCellEdited(func(row, col int, text string) {
		if row < 0 || row >= len(decls) {
			return
		}
		decls[row] = setTagDeclCell(decls[row], col, text)
		// Render the row back from the declaration: the table has already stored
		// the raw text, and a rejected numeric edit has to visibly snap back to
		// the value that is still in effect.
		model.SetRow(row, tagDeclCells(decls[row])...)
		table.Self().Update()
		refreshHint()
	})
	// A header click re-sorts the model's rows in place; re-read them so the
	// declarations keep matching the rows the next edit or 删除 addresses.
	table.SigSortChanged(func(col int, ascending bool) {
		decls = tagDictFromModel(model)
	})

	btnAdd := gui.NewButton1("添加", nil)
	btnAdd.Action().BindFunc0(func() {
		decls = append(decls, TagDecl{})
		model.AddRow(tagDeclCells(TagDecl{})...)
		table.SetSelectedRow(len(decls) - 1)
		table.Self().Update()
	})
	btnDelete := gui.NewButton1("删除", nil)
	btnDelete.Action().BindFunc0(func() {
		row := table.SelectedRow()
		if row < 0 || row >= len(decls) {
			return
		}
		decls = append(decls[:row], decls[row+1:]...)
		model.RemoveRow(row)
		table.SetSelectedRow(-1)
		table.Self().Update()
		refreshHint()
	})

	buttons := gui.NewHBox()
	buttons.SetSpacing(8)
	buttons.AddWidget(btnAdd)
	buttons.AddWidget(btnDelete)

	content := gui.NewVBox()
	content.SetSpacing(8)
	content.AddWidget(table)
	content.AddWidget(buttons)
	content.AddWidget(hint)

	dlg := gui.NewDialog("标签字典", parent)
	dlg.SetContent(content)
	dlg.AddButton("确定", gui.DialogOK)
	dlg.AddButton("取消", gui.DialogCancel)
	dlg.SetSize(600, 500)

	if dlg.ShowModal() != gui.DialogOK {
		return false
	}
	changed := applyTagDict(scene, decls)
	view.Self().Update()
	return changed
}

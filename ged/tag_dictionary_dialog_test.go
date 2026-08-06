package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// TestTagDeclCellsRoundTrip: every column the dialog shows must be an editable
// view of the field behind it — render a declaration, type each cell back, and
// the declaration has to come out unchanged.
func TestTagDeclCellsRoundTrip(t *testing.T) {
	want := TagDecl{Name: "PT-101", Meta: core.Meta{
		Unit: "bar", Min: 0.5, Max: 10,
		LoLo: 1, Lo: 2, Hi: 8, HiHi: 9.5,
		Desc: "反应釜压力",
	}}

	cells := tagDeclCells(want)
	if len(cells) != tagColCount {
		t.Fatalf("tagDeclCells returned %d cells, want %d", len(cells), tagColCount)
	}
	if got := tagDeclFromCells(cells); !reflect.DeepEqual(got, want) {
		t.Errorf("declaration rebuilt from its own cells =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestTagDictFromModelFollowsSort: the table model is what the dialog reads back,
// and gui.SimpleTableModel reorders its own rows when a header is clicked — so
// the declarations must be re-read from it in the order it now holds, or an edit
// after a sort would be applied to a different tag than the one on screen.
func TestTagDictFromModelFollowsSort(t *testing.T) {
	tt := TagDecl{Name: "TT-102", Meta: core.Meta{Unit: "℃", Max: 200}}
	pt := TagDecl{Name: "PT-101", Meta: core.Meta{Unit: "bar", Max: 10, Hi: 8, Desc: "压力"}}

	model := gui.NewSimpleTableModel(tagDictHeaders[:])
	model.AddRow(tagDeclCells(tt)...)
	model.AddRow(tagDeclCells(pt)...)

	if got := tagDictFromModel(model); !reflect.DeepEqual(got, []TagDecl{tt, pt}) {
		t.Fatalf("dictionary read back = %+v, want %+v", got, []TagDecl{tt, pt})
	}

	model.SortByColumn(tagColName, true) // what a header click does to this model
	if got := tagDictFromModel(model); !reflect.DeepEqual(got, []TagDecl{pt, tt}) {
		t.Errorf("dictionary after sorting by 名称 = %+v, want %+v", got, []TagDecl{pt, tt})
	}
}

// TestSetTagDeclCellRejectsGarbage: a numeric cell that does not parse keeps its
// current value instead of collapsing to zero — the rule parseFormSettings and
// the property sheet already follow. Text cells take what was typed, trimmed.
func TestSetTagDeclCellRejectsGarbage(t *testing.T) {
	cur := TagDecl{Name: "PT-101", Meta: core.Meta{Unit: "bar", Max: 10, Hi: 8}}

	if got := setTagDeclCell(cur, tagColMax, "abc"); got.Meta.Max != 10 {
		t.Errorf("max after %q = %g, want 10 unchanged", "abc", got.Meta.Max)
	}
	if got := setTagDeclCell(cur, tagColHi, ""); got.Meta.Hi != 8 {
		t.Errorf("hi after an empty edit = %g, want 8 unchanged", got.Meta.Hi)
	}
	if got := setTagDeclCell(cur, tagColHi, "0"); got.Meta.Hi != 0 {
		t.Errorf("hi after %q = %g, want 0 — a limit must be clearable", "0", got.Meta.Hi)
	}
	if got := setTagDeclCell(cur, tagColName, "  TT-102 "); got.Name != "TT-102" {
		t.Errorf("name = %q, want %q", got.Name, "TT-102")
	}
	if got := setTagDeclCell(cur, tagColDesc, " 出口温度 "); got.Meta.Desc != "出口温度" {
		t.Errorf("desc = %q, want %q", got.Meta.Desc, "出口温度")
	}
	if got := setTagDeclCell(cur, tagColCount, "x"); !reflect.DeepEqual(got, cur) {
		t.Errorf("out-of-range column changed the declaration: %+v", got)
	}
}

// TestApplyTagDictUndo: the dictionary is document content — it changes the code
// the design generates — so editing it goes on the undo stack, which is also
// what marks the document dirty for 保存 / auto-save (GedScene.IsClean).
func TestApplyTagDictUndo(t *testing.T) {
	scene := NewGedScene()
	scene.SetTagDict([]TagDecl{{Name: "PT-101", Meta: core.Meta{Unit: "bar"}}})

	next := []TagDecl{
		{Name: "PT-101", Meta: core.Meta{Unit: "kPa"}},
		{Name: "TT-102", Meta: core.Meta{Unit: "℃"}},
	}
	if !applyTagDict(scene, next) {
		t.Fatal("applyTagDict reported no change")
	}
	if got := scene.TagDict(); !reflect.DeepEqual(got, next) {
		t.Fatalf("dictionary = %+v, want %+v", got, next)
	}
	if scene.IsClean() {
		t.Error("editing the dictionary left the document clean; it would never be saved")
	}

	scene.UndoStack().Undo()
	want := []TagDecl{{Name: "PT-101", Meta: core.Meta{Unit: "bar"}}}
	if got := scene.TagDict(); !reflect.DeepEqual(got, want) {
		t.Errorf("dictionary after undo = %+v, want %+v", got, want)
	}

	scene.UndoStack().Redo()
	if got := scene.TagDict(); !reflect.DeepEqual(got, next) {
		t.Errorf("dictionary after redo = %+v, want %+v", got, next)
	}
}

// TestApplyTagDictNoOp: pressing 确定 without editing anything must not dirty the
// document or leave an empty step on the undo stack.
func TestApplyTagDictNoOp(t *testing.T) {
	scene := NewGedScene()
	scene.SetTagDict([]TagDecl{{Name: "PT-101"}})

	if applyTagDict(scene, scene.TagDict()) {
		t.Error("applying the current dictionary reported a change")
	}
	if scene.UndoStack().CanUndo() {
		t.Error("no-op apply pushed an undo command")
	}
}

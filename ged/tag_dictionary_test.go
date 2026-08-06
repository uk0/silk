package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
)

// TestTagDictRoundTripsThroughDesign: a declared tag carries engineering data
// the generated app seeds the registry with, so every core.Meta field — free
// text included — has to survive save/load, not just the name.
func TestTagDictRoundTripsThroughDesign(t *testing.T) {
	scene := NewGedScene()
	decls := []TagDecl{
		{Name: "PT-101", Meta: core.Meta{
			Unit: "bar", Min: 0, Max: 10,
			LoLo: 0.5, Lo: 1, Hi: 8, HiHi: 9.5,
			Desc: "反应釜压力, 一号线",
		}},
		{Name: "TT-102", Meta: core.Meta{Unit: "℃", Max: 200, Hi: 180}},
	}
	scene.SetTagDict(decls)

	loaded := NewGedScene()
	if err := loaded.LoadDesign(scene.SaveDesign()); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}
	if got := loaded.TagDict(); !reflect.DeepEqual(got, decls) {
		t.Errorf("dictionary after save/load =\n  %+v\nwant\n  %+v", got, decls)
	}
}

// TestTagDictAbsentBlockLoadsEmpty: documents written before the 标签字典
// existed have no tags block, and must open as an empty dictionary — including
// in a designer that already had one open, which is why the loader assigns
// unconditionally instead of reading onto what is already there.
func TestTagDictAbsentBlockLoadsEmpty(t *testing.T) {
	old := NewGedScene().SaveDesign() // no dictionary was ever set

	scene := NewGedScene()
	scene.SetTagDict([]TagDecl{{Name: "PT-101"}})
	if err := scene.LoadDesign(old); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}
	if got := scene.TagDict(); len(got) != 0 {
		t.Errorf("dictionary after loading a document without the block = %+v, want empty", got)
	}
}

// TestSetTagDictNormalizes: the dialog hands over whatever the table holds,
// including the blank row 添加 creates. A nameless entry declares nothing and a
// repeated name cannot be seeded twice (GetOrCreate keys on the name), so both
// are dropped before the generator ever sees them.
func TestSetTagDictNormalizes(t *testing.T) {
	scene := NewGedScene()
	scene.SetTagDict([]TagDecl{
		{Name: "  PT-101  ", Meta: core.Meta{Unit: "bar"}},
		{Name: "   "},
		{Name: "PT-101", Meta: core.Meta{Unit: "kPa"}},
		{Name: "TT-102"},
	})

	want := []TagDecl{
		{Name: "PT-101", Meta: core.Meta{Unit: "bar"}},
		{Name: "TT-102"},
	}
	if got := scene.TagDict(); !reflect.DeepEqual(got, want) {
		t.Errorf("normalized dictionary = %+v, want %+v", got, want)
	}
}

// TestUndeclaredTags: the cross-check is a set difference, deduplicated and
// sorted so the hint column reads the same on every open. A tag bound by a
// widget but never declared is legal — this is the only place it is reported.
func TestUndeclaredTags(t *testing.T) {
	decls := []TagDecl{{Name: "PT-101"}, {Name: "TT-102"}}

	got := undeclaredTags(decls, []string{"TT-102", "LT-9", "", "LT-9", "FT-3", "PT-101"})
	want := []string{"FT-3", "LT-9"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("undeclaredTags = %v, want %v", got, want)
	}

	if got := undeclaredTags(decls, nil); len(got) != 0 {
		t.Errorf("undeclaredTags with nothing referenced = %v, want empty", got)
	}
	if got := undeclaredTags(nil, []string{"LT-9"}); !reflect.DeepEqual(got, []string{"LT-9"}) {
		t.Errorf("undeclaredTags against an empty dictionary = %v, want [LT-9]", got)
	}
}

// TestSceneTagNamesWalksNestedWidgets: the referenced set is read off the same
// TagName() seam scada.BindScreen resolves against, and a widget dropped into a
// layout container is bound exactly like a top-level one — so the walk has to
// recurse or the hint column would call a bound tag undeclared.
func TestSceneTagNamesWalksNestedWidgets(t *testing.T) {
	scene := NewGedScene()
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "PT-101")
	addFakeWithTag(t, scene, "gui.Button", "btn1", "") // no seam, contributes nothing

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create gui.VBox: %v", err)
	}
	box.SetWidgetName("box")
	box.SetBounds(5, 40, 60, 40)
	cmd := graph.NewAddCommand()
	cmd.AddItem(box, scene)
	scene.PushCommand(cmd)

	nested, err := NewFakeWidgetFromFactory("gui.Valve")
	if err != nil {
		t.Fatalf("create gui.Valve: %v", err)
	}
	nested.Widget().(interface{ SetTagName(string) }).SetTagName("XV-7")
	nested.SetParent(box)

	got := sceneTagNames(scene)
	want := []string{"PT-101", "XV-7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sceneTagNames = %v, want %v", got, want)
	}
}

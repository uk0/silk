package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestManagedLayoutConversionKeepsSiblingSlotAndReadingOrder(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(230, 160)
	left := sceneWidget(t, s, "gui.Label", "Before", 2, 2, 18, 8)
	source := sceneWidget(t, s, "gui.GridLayout", "Controls", 20, 20, 120, 80)
	source.layoutManaged = true
	source.SetLocalCoord(true)
	right := sceneWidget(t, s, "gui.Label", "After", 170, 20, 18, 8)
	a := sceneWidget(t, s, "gui.Button", "A", 10, 10, 30, 9)
	b := sceneWidget(t, s, "gui.Button", "B", 10, 10, 30, 9)
	c := sceneWidget(t, s, "gui.Card", "C", 10, 10, 30, 12)
	value := sceneWidget(t, s, "gui.Label", "Nested", 12, 12, 8, 6)
	for i, f := range []*FakeWidget{a, b, c} {
		f.SetParent(source)
		f.layoutCell = layoutPlacement{Row: []int{1, 0, 0}[i], Column: []int{0, 2, 0}[i], RowSpan: 1, ColumnSpan: 1, Valid: true}
	}
	value.SetParent(c)
	source.SetCode("layout comment")
	source.SetEventHandler("Custom", "OnLayout")
	commonPad := gui.Padding{L: 5, T: 6, R: 9, B: 8}
	source.widget.(*gui.GridLayout).SetSpacing(7)
	source.widget.(*gui.GridLayout).SetPadding(commonPad)
	source.Layout()
	v.Selection().Clear()
	v.Selection().Add(source)
	prior := s.SaveDesign().String()
	if !v.LayoutSelection(LayoutVertical) {
		t.Fatal("Grid to vertical refused")
	}
	next := s.Children()[1].(*FakeWidget)
	if !reflect.DeepEqual(s.Children(), []graph.IItem{left, next, right}) || next == source ||
		next.factoryName != string(LayoutVertical) || next.Bounds1() != source.Bounds1() || !next.HasLocalCoord() ||
		next.WidgetName() != "Controls" || next.GetCode() != "layout comment" || next.EventHandlers()["Custom"] != "OnLayout" {
		t.Fatal("conversion changed enclosure, parent slot or layout identity")
	}
	nw := next.widget.(*gui.VBox)
	if nw.Spacing() != 7 || nw.Padding() != commonPad {
		t.Fatal("conversion dropped shared spacing or padding")
	}
	if !reflect.DeepEqual(next.Children(), []graph.IItem{c, b, a}) || value.Parent() != c {
		t.Fatal("conversion must follow the grid's visual row/column order and keep nested ownership")
	}
	if next.Children()[0].Y() >= next.Children()[2].Y() || !v.Selection().Contains(next) || s.UndoStack().Count() != 1 {
		t.Fatal("converted layout did not arrange or select its controls with a single undo edit")
	}
	converted := s.SaveDesign().String()
	v.Undo()
	if s.SaveDesign().String() != prior || !v.Selection().Contains(source) {
		t.Fatal("undo did not restore exact geometry, cells, or selection")
	}
	v.Redo()
	if s.SaveDesign().String() != converted || !v.Selection().Contains(next) {
		t.Fatal("redo did not reproduce exact conversion")
	}
}

func TestManagedLayoutConversionPreservesFormAndTitledOwner(t *testing.T) {
	for _, name := range []string{"form", "gui.Card", "gui.GroupBox"} {
		t.Run(name, func(t *testing.T) {
			var v *GedView
			var owner *FakeWidget
			if name == "form" {
				v = NewGedView()
				s := v.GedScene()
				s.SetSize(140, 90)
				sceneWidget(t, s, "gui.Button", "Go", 12, 18, 32, 10)
			} else {
				v, owner = newContainerLayoutFixture(t, name, 1)
			}
			s := v.GedScene()
			if !v.LayoutSelection(LayoutHorizontal) {
				t.Fatal("initial layout refused")
			}
			active := func() *FakeWidget {
				if owner == nil {
					return s.activeFormLayout()
				}
				return owner.activeContentLayout()
			}
			before := active()
			v.Selection().Clear()
			if owner != nil {
				v.Selection().Add(owner)
			}
			if !v.CanLayoutSelection(LayoutGrid) || !v.LayoutSelection(LayoutGrid) {
				t.Fatal("conversion from selected owner refused")
			}
			root := active()
			if root == nil || root == before || root.factoryName != string(LayoutGrid) || !managedGeometryItem(root) {
				t.Fatal("root lost its owner-managed geometry")
			}
			if owner == nil {
				s.SetSize(155, 102)
				if root.Bounds1() != s.Bounds1() {
					t.Fatalf("form layout failed resize: %v", root.Bounds1())
				}
			} else {
				owner.SetSize(170, 105)
				checkContentDesignBounds(t, owner)
			}
			built, err := gui.LoadFormFromDoc(s.SaveDesign())
			if err != nil {
				t.Fatal(err)
			}
			if owner == nil {
				if _, ok := built.LayoutRoot().(*gui.GridLayout); !ok {
					t.Fatal("load lost the converted form root")
				}
			} else {
				checkContentRuntime(t, built, 1)
			}
			v.Undo()
			if active() != before || !managedGeometryItem(before) {
				t.Fatal("undo lost the original owner relation")
			}
			v.Redo()
			if active() != root {
				t.Fatal("redo lost the converted owner relation")
			}
		})
	}
}

func TestManagedLayoutConversionPreservesSparseFormRows(t *testing.T) {
	v, host := newContainerLayoutFixture(t, "gui.Card", 2)
	if !v.LayoutSelection(LayoutForm) {
		t.Fatal("initial Form refused")
	}
	form := host.activeContentLayout()
	items := form.Children()
	items[0].(*FakeWidget).layoutCell = layoutPlacement{Row: 3, Column: 0, RowSpan: 1, ColumnSpan: 1, Valid: true}
	items[1].(*FakeWidget).layoutCell = layoutPlacement{Row: 7, Column: 1, RowSpan: 1, ColumnSpan: 1, Valid: true}
	form.Layout()
	if !v.LayoutSelection(LayoutGrid) {
		t.Fatal("Form to Grid refused")
	}
	grid := host.activeContentLayout()
	if grid.childPlacement(items[0].(*FakeWidget), 0).Row != 3 || grid.childPlacement(items[1].(*FakeWidget), 1).Row != 7 {
		t.Fatal("a compatible sparse form position should survive conversion to Grid")
	}
}

func TestManagedLayoutConversionRejectsSpansAndLocks(t *testing.T) {
	v, host := newContainerLayoutFixture(t, "gui.GroupBox", 2)
	if !v.LayoutSelection(LayoutGrid) {
		t.Fatal("initial Grid refused")
	}
	root := host.activeContentLayout()
	child := root.Children()[0].(*FakeWidget)
	snapshot := child.layoutCell
	child.layoutCell.ColumnSpan = 2
	before := v.GedScene().SaveDesign().String()
	for _, kind := range []LayoutKind{LayoutGrid, LayoutVertical, LayoutForm, LayoutHorizontal} {
		if v.CanLayoutSelection(kind) || v.LayoutSelection(kind) {
			t.Fatal("conversion should refuse a no-op or loss of span")
		}
	}
	if v.GedScene().SaveDesign().String() != before || v.Scene().UndoStack().Count() != 1 {
		t.Fatal("refusal mutated the scene")
	}
	child.layoutCell = snapshot
	child.SetLockSize(true)
	if v.CanLayoutSelection(LayoutVertical) || v.LayoutSelection(LayoutVertical) {
		t.Fatal("conversion moved locked descendants")
	}
}

func TestManagedLayoutConversionAcceptsEveryCompatiblePair(t *testing.T) {
	kinds := []LayoutKind{LayoutHorizontal, LayoutVertical, LayoutGrid, LayoutForm}
	for _, from := range kinds {
		for _, to := range kinds {
			if from == to {
				continue
			}
			t.Run(string(from)+"/"+string(to), func(t *testing.T) {
				v, host := newContainerLayoutFixture(t, "gui.Card", 2)
				if !v.LayoutSelection(from) {
					t.Fatal("initial layout refused")
				}
				original := host.activeContentLayout()
				controls := original.Children()
				if !v.LayoutSelection(to) {
					t.Fatal("target layout refused")
				}
				changed := checkContentDesignBounds(t, host)
				if changed.factoryName != string(to) || !reflect.DeepEqual(changed.Children(), controls) {
					t.Fatal("conversion lost the selected layout type or controls")
				}
				form, err := gui.LoadFormFromDoc(v.GedScene().SaveDesign())
				if err != nil {
					t.Fatal(err)
				}
				checkContentRuntime(t, form, 2)
				v.Undo()
				if host.activeContentLayout() != original || !reflect.DeepEqual(original.Children(), controls) {
					t.Fatal("undo lost layout or controls")
				}
			})
		}
	}
}

func TestManagedLayoutConversionRetainsOuterGridSpan(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(220, 150)
	grid := sceneWidget(t, s, "gui.GridLayout", "Outer", 14, 14, 150, 90)
	grid.layoutManaged = true
	old := sceneWidget(t, s, "gui.HBox", "Inner", 20, 20, 60, 50)
	old.layoutManaged = true
	old.SetParent(grid)
	span := layoutPlacement{Row: 1, Column: 2, RowSpan: 2, ColumnSpan: 3, Valid: true}
	old.layoutCell = span
	label := sceneWidget(t, s, "gui.Label", "Caption", 30, 22, 24, 9)
	label.SetParent(old)
	grid.Layout()
	v.Selection().Add(old)
	if !v.LayoutSelection(LayoutForm) {
		t.Fatal("cannot convert a nested layout with a span owned by its parent")
	}
	next := grid.Children()[0].(*FakeWidget)
	if next.layoutCell != span || next.Bounds1() != old.Bounds1() || label.Parent() != next {
		t.Fatal("conversion changed placement in the outer grid")
	}
	v.Undo()
	if old.Parent() != grid || old.layoutCell != span || label.Parent() != old {
		t.Fatal("undo lost the outer grid cell")
	}
}

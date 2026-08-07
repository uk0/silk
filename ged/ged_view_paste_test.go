package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestUniqueWidgetName checks the pure unique-name helper: a free name
// is returned as-is, a taken name gets the first free "_N" suffix, and
// repeated calls keep incrementing without ever colliding.
func TestUniqueWidgetName(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		taken map[string]bool
		want  string
	}{
		{"free name unchanged", "Button", map[string]bool{}, "Button"},
		{"empty stays empty", "", map[string]bool{"": true}, ""},
		{"first collision -> _1", "Button", map[string]bool{"Button": true}, "Button_1"},
		{
			"skips taken suffix",
			"Button",
			map[string]bool{"Button": true, "Button_1": true},
			"Button_2",
		},
		{
			"gap is filled",
			"Button",
			map[string]bool{"Button": true, "Button_2": true},
			"Button_1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueWidgetName(tc.base, func(n string) bool { return tc.taken[n] })
			if got != tc.want {
				t.Fatalf("uniqueWidgetName(%q, taken=%v) = %q, want %q",
					tc.base, tc.taken, got, tc.want)
			}
		})
	}
}

// TestUniqueWidgetNameRepeatedCallsNeverCollide simulates handing out
// successive names while recording each one as taken — the way a
// multi-item paste does. Every result must be distinct.
func TestUniqueWidgetNameRepeatedCallsNeverCollide(t *testing.T) {
	taken := map[string]bool{"Button": true, "Button_1": true}
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		n := uniqueWidgetName("Button", func(s string) bool { return taken[s] })
		if taken[n] {
			t.Fatalf("call %d returned already-taken name %q", i, n)
		}
		if seen[n] {
			t.Fatalf("call %d repeated name %q", i, n)
		}
		seen[n] = true
		taken[n] = true
	}
}

// TestPasteAssignsUniqueName drives the real copy+paste pipeline: a named
// widget is copied and pasted, and the pasted widget must NOT reuse the
// source name verbatim (duplicate names break generated Go code).
func TestPasteAssignsUniqueName(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	fake.SetPos(10, 20)
	fake.SetSize(80, 40)
	fake.SetWidgetName("myButton")

	view.Selection().Add(fake)
	view.CopySelected()
	view.PasteItems()

	children := scene.Children()
	if len(children) != 2 {
		t.Fatalf("after paste: scene has %d children, want 2", len(children))
	}

	// Collect the two names and assert they differ.
	names := map[string]int{}
	for _, item := range children {
		if w, ok := item.(*FakeWidget); ok {
			names[w.WidgetName()]++
		}
	}
	for n, c := range names {
		if c > 1 {
			t.Fatalf("name %q used by %d widgets after paste; want unique", n, c)
		}
	}
	if names["myButton"] != 1 {
		t.Fatalf("original name myButton present %d times, want 1", names["myButton"])
	}
}

// TestPasteMultipleNoSelfCollision pastes twice in a row so the second
// paste must avoid both the original and the first copy's name.
func TestPasteMultipleNoSelfCollision(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	fake.SetWidgetName("btn")
	view.Selection().Add(fake)

	view.CopySelected()
	view.PasteItems()
	view.PasteItems()

	if got := len(scene.Children()); got != 3 {
		t.Fatalf("after two pastes: %d children, want 3", got)
	}

	names := map[string]bool{}
	for _, item := range scene.Children() {
		if w, ok := item.(*FakeWidget); ok {
			n := w.WidgetName()
			if names[n] {
				t.Fatalf("duplicate widget name %q after two pastes", n)
			}
			names[n] = true
		}
	}
}

// sceneChildSet is the scene's top-level membership as a lookup, so the paste
// undo/redo below can be checked on WHICH items are on the canvas rather than
// on a count a stray leftover would happen to satisfy.
func sceneChildSet(scene *GedScene) map[graph.IItem]bool {
	held := map[graph.IItem]bool{}
	for _, item := range scene.Children() {
		held[item] = true
	}
	return held
}

// TestPasteUndoesInOneStep pastes three copied widgets and takes them back with
// a single Undo. A paste is one user gesture, so it has to cost one Ctrl+Z —
// one AddCommand per clipboard root meant undoing a 3-widget paste took three
// presses, leaving two copies stranded on the canvas in between.
func TestPasteUndoesInOneStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 10)
	b := addFakeAt(t, scene, "b", 30, 0, 10, 10)
	c := addFakeAt(t, scene, "c", 60, 0, 10, 10)
	originals := map[graph.IItem]bool{a: true, b: true, c: true}

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(c)
	view.CopySelected()
	view.PasteItems()

	var copies []*FakeWidget
	for _, item := range scene.Children() {
		if originals[item] {
			continue
		}
		fake, ok := item.(*FakeWidget)
		if !ok {
			t.Fatalf("paste put a %T on the canvas, want a *FakeWidget", item)
		}
		copies = append(copies, fake)
	}
	if len(copies) != 3 {
		t.Fatalf("setup: pasting 3 copied widgets produced %d copies, want 3", len(copies))
	}

	scene.UndoStack().Undo()
	held := sceneChildSet(scene)
	for _, dup := range copies {
		if held[dup] {
			t.Fatalf("after ONE undo of a 3-widget paste the copy %q is still on the"+
				" canvas: a paste is one gesture and must undo in one step",
				dup.WidgetName())
		}
	}
	for _, orig := range []*FakeWidget{a, b, c} {
		if !held[orig] {
			t.Fatalf("undoing the paste also took the original %q off the canvas",
				orig.WidgetName())
		}
	}
	if len(held) != 3 {
		t.Fatalf("after ONE undo the canvas holds %d items, want exactly the 3 originals",
			len(held))
	}

	scene.UndoStack().Redo()
	held = sceneChildSet(scene)
	for _, dup := range copies {
		if !held[dup] {
			t.Fatalf("redoing the paste left the copy %q off the canvas", dup.WidgetName())
		}
	}
	if len(held) != 6 {
		t.Fatalf("after ONE redo the canvas holds %d items, want 6 (3 originals + 3 copies)",
			len(held))
	}
}

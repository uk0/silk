package ged

import (
	"testing"
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

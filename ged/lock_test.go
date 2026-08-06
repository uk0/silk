package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
)

// TestLockMenuLabel pins the state marker the context-menu toggles carry:
// popup-menu buttons never render Action.SetChecked, so an entry that is on
// has to say so in its text.
func TestLockMenuLabel(t *testing.T) {
	on := lockMenuLabel("锁定位置", true)
	off := lockMenuLabel("锁定位置", false)

	if on == off {
		t.Fatalf("checked and unchecked labels are identical: %q", on)
	}
	if !strings.Contains(on, "锁定位置") || !strings.Contains(off, "锁定位置") {
		t.Errorf("labels lost the command name: on=%q off=%q", on, off)
	}
	if !strings.Contains(on, "✓") {
		t.Errorf("checked label %q carries no check mark", on)
	}
	if strings.Contains(off, "✓") {
		t.Errorf("unchecked label %q carries a check mark", off)
	}
}

// TestSelectionLockState covers what the two menu entries show: checked only
// when EVERY selected item carries the flag. A mixed selection reads as
// unchecked so the next click locks the whole selection instead of unlocking
// the half that was already locked.
func TestSelectionLockState(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 5)
	b := addFakeAt(t, scene, "b", 20, 0, 10, 5)

	cases := []struct {
		name             string
		aPos, aSize      bool
		bPos, bSize      bool
		wantPos, wantSze bool
	}{
		{"none locked", false, false, false, false, false, false},
		{"all pos locked", true, false, true, false, true, false},
		{"all size locked", false, true, false, true, false, true},
		{"all locked", true, true, true, true, true, true},
		{"mixed pos", true, false, false, false, false, false},
		{"mixed size", false, true, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a.SetLockPos(tc.aPos)
			a.SetLockSize(tc.aSize)
			b.SetLockPos(tc.bPos)
			b.SetLockSize(tc.bSize)

			pos, size := selectionLockState([]graph.IItem{a, b})
			if pos != tc.wantPos || size != tc.wantSze {
				t.Errorf("selectionLockState() = (%v, %v), want (%v, %v)",
					pos, size, tc.wantPos, tc.wantSze)
			}
		})
	}

	// An empty selection has nothing locked, so both entries read unchecked.
	if pos, size := selectionLockState(nil); pos || size {
		t.Errorf("selectionLockState(nil) = (%v, %v), want (false, false)", pos, size)
	}
}

// TestSetSelectionLockPosAppliesToWholeSelection drives the menu entry's glue:
// the flag lands on every selected item, not just the one that was right
// clicked, and the move path (drag and arrow-key alike) then refuses to move
// them.
func TestSetSelectionLockPosAppliesToWholeSelection(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 10, 10, 10, 5)
	b := addFakeAt(t, scene, "b", 30, 10, 10, 5)
	view.Selection().Add(a)
	view.Selection().Add(b)

	view.setSelectionLockPos(true)

	if !a.IsLockPos() || !b.IsLockPos() {
		t.Fatalf("lock reached %v / %v, want both locked", a.IsLockPos(), b.IsLockPos())
	}
	if a.IsLockSize() || b.IsLockSize() {
		t.Errorf("position lock also set the size lock")
	}
	if cmd := view.Selection().GenerateMoveCommand(1, 1); cmd != nil {
		t.Errorf("move command generated for a fully position-locked selection")
	}

	view.setSelectionLockPos(false)
	if a.IsLockPos() || b.IsLockPos() {
		t.Errorf("unlock left %v / %v locked", a.IsLockPos(), b.IsLockPos())
	}
}

// TestSetSelectionLockSizeDropsResizeHandles pins the failure mode that makes a
// size lock look broken: decors are generated once, when an item is selected,
// so locking an already-selected widget left its resize handles on screen — and
// ResizeDecor.OnEndMoveHandle resizes its own item without consulting the lock.
func TestSetSelectionLockSizeDropsResizeHandles(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 10, 10, 20, 10)
	view.Selection().Add(a)

	// Sanity: an unlocked widget has grab handles on its corners.
	if _, h := view.Selection().FindHandleAt(10, 10); h == 0 {
		t.Fatal("unlocked widget has no resize handle at its top-left corner")
	}

	view.setSelectionLockSize(true)

	if !a.IsLockSize() {
		t.Fatalf("size lock did not reach the item")
	}
	if a.IsLockPos() {
		t.Errorf("size lock also set the position lock")
	}
	if _, h := view.Selection().FindHandleAt(10, 10); h != 0 {
		t.Errorf("size-locked widget still offers resize handle %d", h)
	}
	if cmd := view.Selection().GenerateResizeCommand(5, 5, minWidgetSize); cmd != nil {
		t.Errorf("resize command generated for a size-locked selection")
	}
}

// TestLockGlyphBox covers the canvas badge: it appears for either lock (not
// only for the both-locked case the designer used to draw), and is skipped on
// a widget too small to carry it without swallowing what it marks.
func TestLockGlyphBox(t *testing.T) {
	const w, h = 20.0, 10.0

	if _, _, _, ok := lockGlyphBox(false, false, w, h); ok {
		t.Errorf("glyph drawn on an unlocked widget")
	}
	for _, tc := range []struct{ pos, size bool }{{true, false}, {false, true}, {true, true}} {
		if _, _, _, ok := lockGlyphBox(tc.pos, tc.size, w, h); !ok {
			t.Errorf("no glyph for lockPos=%v lockSize=%v", tc.pos, tc.size)
		}
	}

	x, y, s, ok := lockGlyphBox(true, true, w, h)
	if !ok {
		t.Fatal("no glyph on a normal-sized widget")
	}
	if s <= 0 {
		t.Errorf("glyph size = %g, want positive", s)
	}
	if x < 0 || y < 0 || x+s > w || y+s > h {
		t.Errorf("glyph box (%g, %g, %g) escapes the %gx%g item", x, y, s, w, h)
	}
	// Top-right corner: past the horizontal middle, above the vertical one.
	if x < w*0.5 || y > h*0.5 {
		t.Errorf("glyph at (%g, %g) is not in the top-right corner of %gx%g", x, y, w, h)
	}

	// Too small to carry the badge.
	if _, _, _, ok := lockGlyphBox(true, true, 2, 2); ok {
		t.Errorf("glyph drawn on a 2x2 mm widget")
	}
}

// TestPartialLockRoundTrip covers persistence of the two flags now that they
// can be set independently: the old "locked" attribute was written only when
// both were on, so a position-only lock vanished on save/load.
func TestPartialLockRoundTrip(t *testing.T) {
	cases := []struct{ pos, size bool }{
		{true, false},
		{false, true},
		{true, true},
		{false, false},
	}
	for _, tc := range cases {
		src, err := NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatalf("create fake: %v", err)
		}
		src.SetBounds(10, 20, 30, 12)
		src.SetLockPos(tc.pos)
		src.SetLockSize(tc.size)

		dst, err := NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatalf("create fake: %v", err)
		}
		if err := dst.LoadDesign(src.SaveDesign()); err != nil {
			t.Fatalf("LoadDesign: %v", err)
		}

		if dst.IsLockPos() != tc.pos || dst.IsLockSize() != tc.size {
			t.Errorf("round-trip of (pos=%v, size=%v) = (pos=%v, size=%v)",
				tc.pos, tc.size, dst.IsLockPos(), dst.IsLockSize())
		}
	}
}

// TestLegacyLockedAttrStillLoads keeps designs saved before the flags split
// readable: one "locked" attribute meant both.
func TestLegacyLockedAttrStillLoads(t *testing.T) {
	doc := core.NewTDoc()
	doc.SetValue("gui.Button")
	doc.WriteAttr("locked", true)

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	if err := fake.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}
	if !fake.IsLockPos() || !fake.IsLockSize() {
		t.Errorf("legacy locked=true loaded as (pos=%v, size=%v), want both",
			fake.IsLockPos(), fake.IsLockSize())
	}
}

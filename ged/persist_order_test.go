package ged

import (
	"bytes"
	"sort"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
)

// blockKeys returns the child keys of doc's named block, in the order they were
// written — the order they will reach the file.
func blockKeys(t *testing.T, doc *core.TDoc, block string) []string {
	t.Helper()
	b := doc.ChildByKey(block, false)
	if b == nil {
		t.Fatalf("design has no %q block", block)
	}
	var keys []string
	for _, c := range b.Childdren() {
		keys = append(keys, c.Key())
	}
	if len(keys) < 2 {
		t.Fatalf("%q block has %d entries, too few to detect an ordering defect", block, len(keys))
	}
	return keys
}

// saveBytes serializes a design the way GedScene.Save writes it to disk.
func saveBytes(t *testing.T, doc *core.TDoc) string {
	t.Helper()
	var buf bytes.Buffer
	if err := doc.Save(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// designWithHandlers builds a one-button scene carrying several event handlers,
// enough entries that a randomized order is overwhelmingly unlikely to come out
// sorted by chance.
func designWithHandlers(t *testing.T) *GedScene {
	t.Helper()
	scene := NewGedScene()
	scene.SetFormTitle("OrderStability")
	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	btn.SetWidgetName("btn")
	btn.SetBounds(10, 10, 30, 8)
	for _, e := range []string{"OnClick", "OnDoubleClick", "OnFocus", "OnHover", "OnKeyDown", "OnBlur"} {
		btn.SetEventHandler(e, "handle"+e)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)
	return scene
}

// TestSaveDesignIsByteStable pins the whole design file: saving an untouched
// design twice must produce identical bytes. The "events" and "props" blocks
// were written by ranging straight over their maps, and Go randomizes map
// iteration, so opening a design and saving it reshuffled those lines every
// time. The user saw a git diff on a file they had not edited, and two people
// saving the same design conflicted over reordered lines.
//
// Repeating the save is what makes this deterministic rather than a coin flip:
// six handlers give many possible orders, so an unsorted writer cannot produce
// the same bytes 30 times running.
func TestSaveDesignIsByteStable(t *testing.T) {
	scene := designWithHandlers(t)

	first := saveBytes(t, scene.SaveDesign())
	for i := 1; i < 30; i++ {
		got := saveBytes(t, scene.SaveDesign())
		if got != first {
			t.Fatalf("save %d of an untouched design differs from save 0.\nsave 0:\n%s\nsave %d:\n%s",
				i, first, i, got)
		}
	}
}

// TestSaveDesignBlocksAreSorted names the order the file is stable in. Byte
// stability alone would also be satisfied by an accidental order that a later
// refactor could silently change; sorting by key is the order codegen already
// writes the same event map in (handlerBindingsFor), so the two stay readable
// side by side.
func TestSaveDesignBlocksAreSorted(t *testing.T) {
	scene := designWithHandlers(t)
	children := scene.SaveDesign().ChildByKey("children", false)
	if children == nil || len(children.Childdren()) != 1 {
		t.Fatal("design did not save its one widget")
	}
	widget := children.Childdren()[0]

	for _, block := range []string{"events", "props"} {
		keys := blockKeys(t, widget, block)
		if !sort.StringsAreSorted(keys) {
			t.Errorf("%q block is written in %v, not sorted by key", block, keys)
		}
	}
}

package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// dropCtx is a minimal gui.IDndContext carrying a single text/plain payload,
// so the canvas drop path can be driven without a live drag loop.
type dropCtx struct {
	pa      gui.DndAction
	act     gui.DndAction
	payload interface{}
}

func (this *dropCtx) PosibleActions() gui.DndAction { return this.pa }

func (this *dropCtx) Action() gui.DndAction { return this.act }

func (this *dropCtx) SetAction(act gui.DndAction) {
	if this.pa&act == 0 {
		this.act = gui.DndIgnore
		return
	}
	this.act = act
}

func (this *dropCtx) From() interface{} { return nil }

func (this *dropCtx) Formats() []string { return []string{"text/plain"} }

func (this *dropCtx) HasFormat(format string) bool { return format == "text/plain" }

func (this *dropCtx) Data(format string) interface{} {
	if format == "text/plain" {
		return this.payload
	}
	return nil
}

// TestGedViewOnDropClaimsTheDrop pins the action the canvas leaves behind after
// it has consumed a palette drop. Both backends now offer the drop to the
// widget under the cursor and then to its ancestors, stopping at the first one
// that sets an action; a canvas that creates the widget but leaves the action
// at DndIgnore is treated as having refused, so the drop is offered again to
// the enclosing Dock and Frame and DoDragDrop reports DndIgnore to the palette.
func TestGedViewOnDropClaimsTheDrop(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	// The walk resets the action before offering the drop to each target.
	ctx.SetAction(gui.DndIgnore)
	view.OnDrop(20, 20, ctx)

	if got := len(scene.Children()); got != 1 {
		t.Fatalf("scene children after the drop = %d, want 1", got)
	}
	if ctx.Action() != gui.DndCopy {
		t.Errorf("action after a drop that created a widget = %v, want DndCopy", ctx.Action())
	}
}

// TestGedViewOnDropRefusesUnknownPayload keeps the refusal path intact: a drop
// whose payload is not a factory name must leave DndIgnore so the walk carries
// on to the ancestors instead of the canvas swallowing it.
func TestGedViewOnDropRefusesUnknownPayload(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	// Not a string: the canvas cannot read a factory name out of it, which is
	// the refusal that does not reach the modal error box.
	ctx := &dropCtx{pa: gui.DndCopy, payload: 42}
	ctx.SetAction(gui.DndIgnore)
	view.OnDrop(20, 20, ctx)

	if got := len(scene.Children()); got != 0 {
		t.Fatalf("scene children after a refused drop = %d, want 0", got)
	}
	if ctx.Action() != gui.DndIgnore {
		t.Errorf("action after a refused drop = %v, want DndIgnore", ctx.Action())
	}
}

// TestIsContainerItem verifies the drop-into-container predicate: every
// layout/AddWidget/window container reports true, leaf widgets report false,
// and a non-FakeWidget (the scene root) reports false. This is the headless
// core of the drag-drop nesting decision in OnDrop.
func TestIsContainerItem(t *testing.T) {
	containers := []string{
		"gui.VBox", "gui.HBox", "gui.GridLayout", "gui.FormLayout",
		"gui.Card", "gui.GroupBox", "gui.Accordion", "gui.StackedWidget",
		"gui.TabWidget", "gui.Splitter", "gui.ScrollArea", "gui.Form", "gui.Dialog",
	}
	for _, name := range containers {
		fw, err := NewFakeWidgetFromFactory(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if !isContainerItem(fw) {
			t.Errorf("isContainerItem(%s) = false, want true", name)
		}
	}

	leaves := []string{"gui.Button", "gui.Label", "gui.Edit", "gui.CheckBox"}
	for _, name := range leaves {
		fw, err := NewFakeWidgetFromFactory(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if isContainerItem(fw) {
			t.Errorf("isContainerItem(%s) = true, want false", name)
		}
	}

	// The scene root is not a FakeWidget → never a container.
	if isContainerItem(NewGedScene()) {
		t.Error("isContainerItem(scene) = true, want false")
	}
}

// TestContainerUnderPoint builds a scene with a VBox holding a Button and
// asserts the container hit-test finds the VBox both when the drop lands on
// the nested Button (walk up from the leaf) and on the container's own body,
// and returns nil over empty canvas (so OnDrop falls back to the scene root).
func TestContainerUnderPoint(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(10, 10, 80, 60)
	vbox.SetParent(scene)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetBounds(20, 20, 25, 7)
	btn.SetParent(vbox)

	// Drop over the nested Button → walk up to the containing VBox.
	if got := nearestContainerAncestor(scene.FindItemAt(25, 24, nil)); got != graph.IItem(vbox) {
		t.Errorf("container under nested button = %v, want vbox", got)
	}
	// Drop over the VBox body (not on the Button) → the VBox itself.
	if got := nearestContainerAncestor(scene.FindItemAt(12, 12, nil)); got != graph.IItem(vbox) {
		t.Errorf("container under vbox body = %v, want vbox", got)
	}
	// Drop over empty canvas → nil (fall back to scene root).
	if got := nearestContainerAncestor(scene.FindItemAt(150, 130, nil)); got != nil {
		t.Errorf("container under empty canvas = %v, want nil", got)
	}
}

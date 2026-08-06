package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// buildConfiguredPanel drops a VBox holding two configured children onto the
// scene: a Button carrying an edited property and an event binding, and a Tank
// carrying a tag binding. Between them they cover every kind of state a copy
// has to bring along.
func buildConfiguredPanel(t *testing.T, scene *GedScene) *FakeWidget {
	t.Helper()

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("panel")
	box.SetBounds(10, 10, 80, 60)
	box.SetParent(scene)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetWidgetName("btnOK")
	btn.SetBounds(12, 14, 30, 8)
	btn.SetParent(box)
	btn.Widget().(*gui.Button).SetText("确定")
	btn.SetEventHandler("OnClick", "onOK")

	tank, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatalf("create Tank: %v", err)
	}
	tank.SetWidgetName("tank1")
	tank.SetBounds(12, 26, 30, 20)
	tank.SetParent(box)
	tank.Widget().(interface{ SetTagName(string) }).SetTagName("PLC1.Level")

	return box
}

// pastedRootOf returns the scene child that is not orig — the copy PasteItems
// just added.
func pastedRootOf(t *testing.T, scene *GedScene, orig *FakeWidget) *FakeWidget {
	t.Helper()
	var found *FakeWidget
	for _, it := range scene.Children() {
		fw, ok := it.(*FakeWidget)
		if !ok || fw == orig {
			continue
		}
		if found != nil {
			t.Fatalf("scene has more than one pasted root: %q and %q",
				found.WidgetName(), fw.WidgetName())
		}
		found = fw
	}
	if found == nil {
		t.Fatal("nothing was pasted")
	}
	return found
}

// childByFactory finds the single child of box built from factoryName.
func childByFactory(t *testing.T, box *FakeWidget, factoryName string) *FakeWidget {
	t.Helper()
	for _, c := range box.Children() {
		if fw, ok := c.(*FakeWidget); ok && fw.WidgetFactoryName() == factoryName {
			return fw
		}
	}
	t.Fatalf("container %q has no %s child (children: %d)", box.WidgetName(), factoryName, len(box.Children()))
	return nil
}

// TestCopyPasteCarriesConfiguredSubtree is the round-trip at the heart of the
// clipboard: copying a container must bring its children along, and each child
// must arrive with the property, the event binding and the tag binding it was
// configured with. The old clipboard stored six geometry/name fields, so the
// paste produced an empty default container.
func TestCopyPasteCarriesConfiguredSubtree(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 200)

	box := buildConfiguredPanel(t, scene)

	view.Selection().Add(box)
	view.CopySelected()
	view.PasteItems()

	copyBox := pastedRootOf(t, scene, box)
	if copyBox.WidgetFactoryName() != "gui.VBox" {
		t.Fatalf("pasted root factory = %q, want gui.VBox", copyBox.WidgetFactoryName())
	}
	if n := len(copyBox.Children()); n != 2 {
		t.Fatalf("pasted container has %d children, want 2 (the subtree was dropped)", n)
	}

	copyBtn := childByFactory(t, copyBox, "gui.Button")
	if got := copyBtn.Widget().(*gui.Button).Text(); got != "确定" {
		t.Errorf("copied button text = %q, want %q", got, "确定")
	}
	if got := copyBtn.EventHandlers()["OnClick"]; got != "onOK" {
		t.Errorf("copied button OnClick handler = %q, want %q", got, "onOK")
	}

	copyTank := childByFactory(t, copyBox, "gui.Tank")
	namer, ok := copyTank.Widget().(interface{ TagName() string })
	if !ok {
		t.Fatal("copied gui.Tank has no TagName")
	}
	if got := namer.TagName(); got != "PLC1.Level" {
		t.Errorf("copied tank tag = %q, want %q", got, "PLC1.Level")
	}

	// Names must be fresh throughout the copy, not only at its root.
	if copyBox.WidgetName() == box.WidgetName() {
		t.Errorf("copied container reused the name %q", copyBox.WidgetName())
	}
	if copyBtn.WidgetName() == "btnOK" {
		t.Error("copied button reused the name btnOK")
	}
	if copyTank.WidgetName() == "tank1" {
		t.Error("copied tank reused the name tank1")
	}
}

// TestCopySkipsChildOfSelectedContainer: selecting a container together with
// one of its own children must paste the subtree once. The child already rides
// inside the container's serialized form, so copying it as a second root would
// paste it twice — once nested, once loose on the canvas.
func TestCopySkipsChildOfSelectedContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 200)

	box := buildConfiguredPanel(t, scene)
	btn := childByFactory(t, box, "gui.Button")

	view.Selection().Add(box)
	view.Selection().Add(btn)
	view.CopySelected()
	view.PasteItems()

	if n := len(scene.Children()); n != 2 {
		t.Fatalf("scene has %d top-level items after paste, want 2 (original + one copy)", n)
	}
	copyBox := pastedRootOf(t, scene, box)
	if n := len(copyBox.Children()); n != 2 {
		t.Fatalf("pasted container has %d children, want 2", n)
	}
}

// TestPasteNamesUniqueAcrossWholeDocument pastes the same subtree twice and
// checks every name in the document — nested ones included — is still unique.
// Generated code declares one struct field per widget name, so a collision
// anywhere in the tree produces a file that does not compile.
func TestPasteNamesUniqueAcrossWholeDocument(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 200)

	box := buildConfiguredPanel(t, scene)

	view.Selection().Add(box)
	view.CopySelected()
	view.PasteItems()
	view.PasteItems()

	seen := map[string]int{}
	var walk func(items []graph.IItem)
	walk = func(items []graph.IItem) {
		for _, it := range items {
			if fw, ok := it.(*FakeWidget); ok {
				seen[fw.WidgetName()]++
			}
			walk(it.Children())
		}
	}
	walk(scene.Children())

	if len(seen) != 9 {
		t.Fatalf("document holds %d distinct widget names, want 9 (3 widgets x 3 copies): %v", len(seen), seen)
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("name %q used by %d widgets", name, n)
		}
	}
}

// TestPasteOffsetsWholeSubtree: the copy's children must land at the same
// offset as its root. Child bounds are scene coordinates, so offsetting only
// the container would drop the copied children exactly on top of the
// originals' — the collision the paste offset exists to avoid.
func TestPasteOffsetsWholeSubtree(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 200)
	view.SetGridStep(5)

	box := buildConfiguredPanel(t, scene)
	origBtnX, origBtnY := childByFactory(t, box, "gui.Button").Pos()

	view.Selection().Add(box)
	view.CopySelected()
	view.PasteItems()

	copyBox := pastedRootOf(t, scene, box)
	if x, y := copyBox.Pos(); x != 15 || y != 15 {
		t.Fatalf("copied container at (%g, %g), want (15, 15) — one 5 mm pitch off (10, 10)", x, y)
	}
	// The container moved by (+5, +5); every child must move with it.
	if x, y := childByFactory(t, copyBox, "gui.Button").Pos(); x != origBtnX+5 || y != origBtnY+5 {
		t.Errorf("copied button at (%g, %g), want (%g, %g)", x, y, origBtnX+5, origBtnY+5)
	}
}

// TestGenerateCodeEmitsSharedHandlerOnce: two widgets carrying the same
// handler source — what a duplicate now produces, since the copy carries the
// widget's whole serialized form — must not emit the func twice. A second
// declaration is a redeclaration error, so the generated program would not
// build.
func TestGenerateCodeEmitsSharedHandlerOnce(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Shared")
	scene.SetSize(100, 80)

	const handler = "func onOK() {\n\tprintln(\"ok\")\n}"
	for _, name := range []string{"btnA", "btnB"} {
		fw, err := NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		fw.SetWidgetName(name)
		fw.SetBounds(5, 5, 25, 7)
		fw.SetCode(handler)
		fw.SetEventHandler("OnClick", "onOK")
		cmd := graph.NewAddCommand()
		cmd.AddItem(fw, scene)
		scene.PushCommand(cmd)
	}

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "SharedUI"})
	if n := strings.Count(code, "func onOK()"); n != 1 {
		t.Errorf("generated code declares onOK %d times, want 1 (a duplicate declaration does not compile)", n)
	}
	if !strings.Contains(code, "ui.BtnA.Action().BindFunc0(onOK)") ||
		!strings.Contains(code, "ui.BtnB.Action().BindFunc0(onOK)") {
		t.Error("both buttons should still bind the shared handler")
	}
}

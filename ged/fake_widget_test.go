package ged

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
)

// These tests cover the skip-unknown-factory load path: a .silkui saved when a
// widget factory existed, then opened after that widget was renamed/removed or
// its plugin isn't registered. Loading must skip the unknown node (and its
// subtree, since children can't attach to a missing parent), keep every valid
// sibling, and still succeed so the file opens. The design nodes are crafted
// directly in the TDoc persist form so a "bogus" factory name can be mixed
// among valid ones without the designer ever having to instantiate it.

// widgetNode builds a single serialized widget node: factory name as the node
// value (matching FakeWidget.SaveDesign) plus an optional "name" attr.
func widgetNode(factory, name string) *core.TDoc {
	n := core.NewTDoc()
	n.SetValue(factory)
	if name != "" {
		n.WriteAttr("name", name)
	}
	return n
}

// attachChildren wraps kids in a "children" block and adds it to parent,
// mirroring the on-disk layout produced by SaveDesign.
func attachChildren(parent *core.TDoc, kids ...*core.TDoc) {
	block := core.NewTDoc()
	block.SetKey("children")
	for _, k := range kids {
		block.AddChild(k)
	}
	parent.AddChild(block)
}

// sceneDoc builds a top-level "form" node with the given top-level widget nodes.
func sceneDoc(title string, top ...*core.TDoc) *core.TDoc {
	root := core.NewTDoc()
	root.SetValue("form")
	root.WriteAttr("title", title)
	attachChildren(root, top...)
	return root
}

// TestLoadSkipsUnknownFactoryMixedSiblings is the core regression: a design
// with a bogus top-level node (itself carrying a valid child) AND a valid
// container that holds one valid + one bogus child must load the valid widgets,
// drop both bogus subtrees, and return nil (partial success).
func TestLoadSkipsUnknownFactoryMixedSiblings(t *testing.T) {
	keepBtn := widgetNode("gui.Button", "keepBtn")

	// Valid container with one good child and one bogus child.
	keepBox := widgetNode("gui.VBox", "keepBox")
	keepChild := widgetNode("gui.Label", "keepChild")
	bogusChild := widgetNode("gui.NoSuchWidget_child", "bogusChild")
	attachChildren(keepBox, keepChild, bogusChild)

	// Bogus top-level node that itself has a (valid) child — the whole
	// subtree must be dropped because its parent can never be created.
	bogusTop := widgetNode("gui.NoSuchWidget_top", "bogusTop")
	orphan := widgetNode("gui.Button", "orphanUnderBogus")
	attachChildren(bogusTop, orphan)

	root := sceneDoc("Mixed", keepBtn, bogusTop, keepBox)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("partial load must not error, got %v", err)
	}

	top := scene.Children()
	if len(top) != 2 {
		t.Fatalf("top-level children = %d, want 2 (keepBtn + keepBox; bogusTop dropped)", len(top))
	}

	byName := map[string]*FakeWidget{}
	for _, c := range top {
		fw, ok := c.(*FakeWidget)
		if !ok {
			t.Fatalf("top child is %T, want *FakeWidget", c)
		}
		byName[fw.WidgetName()] = fw
	}
	if byName["bogusTop"] != nil {
		t.Error("bogusTop should have been skipped, but it is present")
	}
	if byName["keepBtn"] == nil || byName["keepBtn"].WidgetFactoryName() != "gui.Button" {
		t.Errorf("keepBtn missing or wrong factory: %+v", byName["keepBtn"])
	}
	box := byName["keepBox"]
	if box == nil {
		t.Fatal("keepBox (valid container) was dropped")
	}
	if box.WidgetFactoryName() != "gui.VBox" {
		t.Errorf("keepBox factory = %q, want gui.VBox", box.WidgetFactoryName())
	}

	// The valid container keeps its valid child and drops only the bogus one.
	kids := box.Children()
	if len(kids) != 1 {
		t.Fatalf("keepBox children = %d, want 1 (keepChild; bogusChild dropped)", len(kids))
	}
	kid, ok := kids[0].(*FakeWidget)
	if !ok || kid.WidgetName() != "keepChild" || kid.WidgetFactoryName() != "gui.Label" {
		t.Fatalf("surviving child = %+v, want keepChild/gui.Label", kids[0])
	}
}

// TestLoadAllUnknownFactories documents the all-bogus case: no valid widgets at
// all must not panic or error; the scene simply ends up empty (the file opens
// blank rather than refusing to open).
func TestLoadAllUnknownFactories(t *testing.T) {
	root := sceneDoc("AllBogus",
		widgetNode("gui.Ghost_A", "a"),
		widgetNode("gui.Ghost_B", "b"),
	)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("all-unknown load must not error, got %v", err)
	}
	if n := len(scene.Children()); n != 0 {
		t.Fatalf("all-unknown scene children = %d, want 0 (empty scene)", n)
	}
}

// TestLoadUnknownFactoryWarns confirms a warning is logged naming the missing
// factory when a node is skipped.
func TestLoadUnknownFactoryWarns(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	root := sceneDoc("Warn",
		widgetNode("gui.Button", "ok"),
		widgetNode("gui.TotallyMissingWidget", "gone"),
	)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("load must not error, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "warning") {
		t.Errorf("expected a warning to be logged, got: %q", out)
	}
	if !strings.Contains(out, "gui.TotallyMissingWidget") {
		t.Errorf("warning should name the missing factory, got: %q", out)
	}
	// The valid widget still loaded.
	if len(scene.Children()) != 1 {
		t.Errorf("valid widget count = %d, want 1", len(scene.Children()))
	}
}

// TestLoadValidNestedNoWarning guards against false positives: an all-valid
// nested design loads with no skip warning and full structure intact. The full
// structural round-trip is covered by TestFakeWidgetNestedSaveLoadRoundTrip;
// this focuses on the "no warning when nothing is skipped" contract.
func TestLoadValidNestedNoWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	box := widgetNode("gui.VBox", "box")
	attachChildren(box, widgetNode("gui.Button", "b1"), widgetNode("gui.Label", "l1"))
	root := sceneDoc("Valid", box)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("valid load errored: %v", err)
	}
	if strings.Contains(buf.String(), "unknown widget factory") {
		t.Errorf("no skip warning expected for an all-valid design, got: %q", buf.String())
	}
	if len(scene.Children()) != 1 {
		t.Fatalf("top children = %d, want 1", len(scene.Children()))
	}
	if kids := scene.Children()[0].(*FakeWidget).Children(); len(kids) != 2 {
		t.Fatalf("nested children = %d, want 2", len(kids))
	}
}

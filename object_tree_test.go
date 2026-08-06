package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// TestObjectTreeFollowsTheSecondDocument guards the state the designer shipped
// in: the tree was wired once at createPanels time to whichever view happened to
// be current, and the view-level attach/detach signals hold a single callback
// each. Open a second document and the tree refreshed only at the moment its tab
// came forward — every widget dropped after that went unlisted, and the panel
// sat there showing the document you were no longer editing.
func TestObjectTreeFollowsTheSecondDocument(t *testing.T) {
	objectTree = ged.NewObjectInspector()

	a := ged.NewGedView()
	bindObjectTreeTo(a)
	dropWidget(t, a, "gui.Button")
	if got := objectTree.RowNames(); len(got) != 2 || got[1] != "button" {
		t.Fatalf("first document: rows = %v, want the form plus one button", got)
	}

	// The user opens a second document; every panel re-binds on tab change.
	b := ged.NewGedView()
	bindObjectTreeTo(b)
	dropWidget(t, b, "gui.Label")

	if got := objectTree.RowNames(); len(got) != 2 || got[1] != "label" {
		t.Fatalf("second document: rows = %v, want the form plus one label", got)
	}

	// And it keeps following that document through undo.
	b.Scene().UndoStack().Undo()
	if got := objectTree.RowNames(); len(got) != 1 {
		t.Errorf("after undo in the second document: rows = %v, want the form alone", got)
	}
}

// TestObjectTreeSelectionIsBoundBothWays: the tree and the canvas share one
// selection. bindObjectTreeTo is the only wire — the canvas half rides the same
// AddSelectionCallback the property sheet uses.
//
// The view has to sit in a frame that owns a status bar: GedView.Init routes
// every selection callback through the status-bar update and returns early when
// there is no bar to write to, so a bare view dispatches nothing.
func TestObjectTreeSelectionIsBoundBothWays(t *testing.T) {
	objectTree = ged.NewObjectInspector()
	f := gui.NewFrame()
	f.SetStatusBar(gui.NewStatusBar())
	gv := ged.NewGedView()
	f.SuggestDocDock().AddView(gv)
	bindObjectTreeTo(gv)

	dropWidget(t, gv, "gui.Button")
	item := gv.Scene().Children()[0]

	sel := gv.Selection()
	sel.Clear()
	sel.Add(item)
	gv.OnIdle() // the view coalesces selection changes and emits them on idle
	if got := objectTree.SelectedItem(); got != item {
		t.Errorf("selecting on the canvas highlighted %v, want the dropped widget's row", got)
	}

	sel.Clear()
	gv.OnIdle()
	if got := objectTree.SelectedItem(); got != nil {
		t.Errorf("clearing the canvas selection left row %v highlighted", got)
	}
}

// TestObjectTreeIsDockedAsAToolView: a panel that is not registered as a tool
// view is indistinguishable from a document, and CurrentDocView will hand it to
// Run / Preview / Save, which type assert it, get nil and give up in silence.
// Registration only takes effect if the frame has synced the registry before
// the panel is docked, so this replays both steps in the order createPanels
// performs them.
func TestObjectTreeIsDockedAsAToolView(t *testing.T) {
	f := gui.NewFrame()
	_ = f.ToolViewActions() // what createPanels calls before docking its panels

	tree := ged.NewObjectInspector()
	f.SuggestDocDock().AddView(tree)

	if !f.IsToolView(tree) {
		t.Error("the object tree is not one of the frame's tool views; CurrentDocView can hand it back as the current document")
	}

	body := funcBody(t, "design.go", "func createPanels(mainFrame *gui.Frame) {")
	sync := strings.Index(body, "mainFrame.ToolViewActions()")
	dock := strings.Index(body, "rightDockI.AddView(objectTree)")
	if sync < 0 || dock < 0 || sync > dock {
		t.Errorf("createPanels docks the object tree at %d and syncs the tool-view registry at %d; the sync has to come first or AddView cannot claim the panel", dock, sync)
	}
}

// TestObjectTreeListsWidgetsByName is what "usable" means for the panel that
// replaced the debug tree: rows read as the widget's name, not as the Go type
// every FakeWidget shares.
func TestObjectTreeListsWidgetsByName(t *testing.T) {
	objectTree = ged.NewObjectInspector()
	gv := ged.NewGedView()
	bindObjectTreeTo(gv)

	dropWidget(t, gv, "gui.Button")
	if fw, ok := gv.Scene().Children()[0].(*ged.FakeWidget); ok {
		fw.SetWidgetName("btnSubmit")
	}
	objectTree.Rebuild()

	rows := objectTree.RowNames()
	if len(rows) != 2 || rows[1] != "btnSubmit" {
		t.Errorf("rows = %v, want the form plus btnSubmit", rows)
	}
	for _, r := range rows {
		if strings.Contains(r, "FakeWidget") {
			t.Errorf("row %q is a debug dump of the Go type, not a widget name", r)
		}
	}
}

// TestObjectTreeDropsTheDbgTree: the graph debug tree must not come back as the
// designer's object panel — its rows are Item.DebugLabel(), the Go type name,
// which is "ged.FakeWidget" for every widget on the canvas.
func TestObjectTreeDropsTheDbgTree(t *testing.T) {
	src := funcBody(t, "design.go", "func createPanels(mainFrame *gui.Frame) {")
	if strings.Contains(src, "NewDbgTreeView") {
		t.Error("createPanels docks graph.DbgTreeView again; every row of it reads \"ged.FakeWidget\"")
	}
}

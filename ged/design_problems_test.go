package ged

import (
	"github.com/uk0/silk/graph"
	"strings"
	"testing"
)

// TestDesignProblemsNamesTheWidgetAtFault: a widget whose factory this build
// does not know vanishes from Generate, from the exported file and from the
// run, taking its children with it. The row has to carry the widget itself,
// because selecting it on the canvas is the only useful thing a user can do
// with the news.
func TestDesignProblemsNamesTheWidgetAtFault(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	sceneWidget(t, scene, "gui.Button", "ok", 10, 10, 30, 8)
	broken := sceneWidget(t, scene, "gui.Button", "ghost", 10, 30, 30, 8)
	broken.SetWidget(&unregisteredWidget{})

	problems := DesignProblems(scene)
	if len(problems) != 1 {
		t.Fatalf("DesignProblems = %+v, want exactly the one unbuildable widget", problems)
	}
	p := problems[0]
	if p.Item != broken {
		t.Errorf("row carries Item %v, want the offending widget %v", p.Item, broken)
	}
	if p.Severity != SeverityError {
		t.Errorf("severity = %v, want error: the widget is lost, not merely suspect", p.Severity)
	}
	if p.File != "ghost (未知类型)" {
		t.Errorf("locator = %q, want the designer's own name for the widget", p.File)
	}
	if p.Line != 0 {
		t.Errorf("Line = %d, want 0: a design widget has no source line", p.Line)
	}
}

// TestDesignProblemsReachesIntoContainers: Generate recurses into containers,
// so a nested unbuildable widget is lost just as quietly as a top-level one.
func TestDesignProblemsReachesIntoContainers(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	box := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 60, 40)
	child, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create gui.Button: %v", err)
	}
	child.SetWidgetName("ghost")
	child.SetParent(box)
	child.SetWidget(&unregisteredWidget{})

	problems := DesignProblems(scene)
	if len(problems) != 1 || problems[0].Item != child {
		t.Fatalf("DesignProblems = %+v, want the nested widget", problems)
	}
}

// TestDesignProblemsAgreesWithPreviewBlockers pins the single walk: the dialog
// 预览 puts up and the rows the 问题 pane lists are two projections of
// unbuildableWidgets. Two walks would eventually disagree about what a build
// loses, and the pane would be listing a different design than the dialog.
func TestDesignProblemsAgreesWithPreviewBlockers(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	a := sceneWidget(t, scene, "gui.Button", "ghostA", 10, 10, 30, 8)
	a.SetWidget(&unregisteredWidget{})
	sceneWidget(t, scene, "gui.Label", "fine", 10, 30, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "ghostB", 10, 50, 30, 8)
	b.SetWidget(&unregisteredWidget{})

	blocked := previewBlockers(scene)
	problems := DesignProblems(scene)
	if len(blocked) != len(problems) {
		t.Fatalf("previewBlockers listed %d, DesignProblems listed %d", len(blocked), len(problems))
	}
	for i := range blocked {
		if blocked[i] != problems[i].File {
			t.Errorf("row %d: dialog says %q, pane says %q", i, blocked[i], problems[i].File)
		}
	}
}

// TestDesignProblemsCleanDesign: a design this build can construct in full
// produces no rows, so a clean run cannot leave stale ones behind.
func TestDesignProblemsCleanDesign(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	sceneWidget(t, scene, "gui.Button", "ok", 10, 10, 30, 8)
	if got := DesignProblems(scene); len(got) != 0 {
		t.Fatalf("DesignProblems on a buildable design = %+v, want none", got)
	}
	if got := DesignProblems(nil); got != nil {
		t.Errorf("DesignProblems(nil) = %+v, want nil", got)
	}
}

// TestLoadDesignRecordsSkippedFactories: the loader drops an unknown widget and
// carries on, which keeps one dead widget from taking the whole file down — but
// it only said so in the log. The scene has to keep which factories went
// missing, because a 保存 over the same path makes the loss permanent.
func TestLoadDesignRecordsSkippedFactories(t *testing.T) {
	root := sceneDoc("Partial",
		widgetNode("gui.Button", "ok"),
		widgetNode("gui.TotallyMissingWidget", "gone"),
	)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("partial load must not error, got %v", err)
	}
	missing := scene.MissingWidgets()
	if len(missing) != 1 || missing[0] != "gui.TotallyMissingWidget" {
		t.Fatalf("MissingWidgets = %v, want [gui.TotallyMissingWidget]", missing)
	}
}

// TestLoadDesignRecordsNestedSkips: a skip inside a container reaches the same
// record, since loadChildWidgets threads one accumulator through the recursion.
func TestLoadDesignRecordsNestedSkips(t *testing.T) {
	box := widgetNode("gui.VBox", "box")
	attachChildren(box, widgetNode("gui.Button", "ok"), widgetNode("gui.Ghost_child", "gone"))
	root := sceneDoc("Nested", box)

	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("load errored: %v", err)
	}
	missing := scene.MissingWidgets()
	if len(missing) != 1 || missing[0] != "gui.Ghost_child" {
		t.Fatalf("MissingWidgets = %v, want [gui.Ghost_child]", missing)
	}
}

// TestLoadDesignCleanFileRecordsNothing guards the other direction: an
// all-valid design must not raise a row.
func TestLoadDesignCleanFileRecordsNothing(t *testing.T) {
	root := sceneDoc("Clean", widgetNode("gui.Button", "ok"))
	scene := NewGedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("load errored: %v", err)
	}
	if got := scene.MissingWidgets(); len(got) != 0 {
		t.Fatalf("MissingWidgets = %v, want none", got)
	}
}

// TestLoadProblemsCarryNoItem: the load-skipped widgets were never built, so
// there is nothing in the scene for a row to select. Handing such a row an
// Item would point the canvas at a widget that does not exist.
func TestLoadProblemsCarryNoItem(t *testing.T) {
	problems := LoadProblems([]string{"gui.Ghost", ""})
	if len(problems) != 2 {
		t.Fatalf("LoadProblems = %+v, want one row per dropped widget", problems)
	}
	for i, p := range problems {
		if p.Item != nil {
			t.Errorf("row %d carries an Item; the widget was never loaded", i)
		}
		if p.Severity != SeverityWarning {
			t.Errorf("row %d severity = %v, want warning", i, p.Severity)
		}
		if !strings.Contains(p.Message, "保存") {
			t.Errorf("row %d message %q does not name the consequence", i, p.Message)
		}
	}
	if problems[0].File != "gui.Ghost" {
		t.Errorf("locator = %q, want the missing factory name", problems[0].File)
	}
	if problems[1].File == "" {
		t.Error("a node with no factory name at all still needs a readable locator")
	}
}

// TestProblemRowWithItemSelectsTheWidget: a row naming a design widget must
// reach the canvas, not the editor. Routed to the file callback it would ask
// the editor to open "" at line 0.
func TestProblemRowWithItemSelectsTheWidget(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	broken := sceneWidget(t, scene, "gui.Button", "ghost", 10, 10, 30, 8)
	broken.SetWidget(&unregisteredWidget{})

	p := NewProblemsPanel()
	p.SetProblems(DesignProblems(scene))

	openedFile := false
	p.SigProblemActivated(func(string, int, int) { openedFile = true })
	var selected graph.IItem
	p.SigItemActivated(func(item graph.IItem) { selected = item })

	p.OnLeftDown(5, problemsHeaderH+0.5*p.rowHeight)

	if openedFile {
		t.Error("a widget row was routed to the file-opening callback")
	}
	if selected != broken {
		t.Errorf("selected %v, want the offending widget %v", selected, broken)
	}
}

// TestProblemRowWithFileStillOpensTheFile: compiler diagnostics keep the
// behaviour they had.
func TestProblemRowWithFileStillOpensTheFile(t *testing.T) {
	p := NewProblemsPanel()
	p.SetProblems([]Problem{{File: "main.go", Line: 7, Col: 2, Message: "undefined: Foo"}})

	var gotFile string
	var gotLine int
	p.SigProblemActivated(func(file string, line, col int) { gotFile, gotLine = file, line })
	p.SigItemActivated(func(graph.IItem) { t.Error("a compiler row reached the widget callback") })

	p.OnLeftDown(5, problemsHeaderH+0.5*p.rowHeight)

	if gotFile != "main.go" || gotLine != 7 {
		t.Errorf("activated with (%s, %d), want (main.go, 7)", gotFile, gotLine)
	}
}

// TestProblemLocatorOmitsAbsentLine: a design widget has no line, and
// appending ":0" to its name only claimed a line nothing can open.
func TestProblemLocatorOmitsAbsentLine(t *testing.T) {
	if got := problemLocator(Problem{File: "ghost (未知类型)"}); got != "ghost (未知类型)" {
		t.Errorf("problemLocator = %q, want the bare name", got)
	}
	if got := problemLocator(Problem{File: "main.go", Line: 7}); got != "main.go:7" {
		t.Errorf("problemLocator = %q, want main.go:7", got)
	}
}

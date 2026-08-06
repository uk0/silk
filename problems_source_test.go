package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// newProblemsPane installs the two panes the reporters write to. reportFailure
// falls back to a modal dialog when there is no 输出 pane, which a headless
// test cannot answer, and it puts its title in the status bar through the
// default frame, so both have to exist.
func newProblemsPane(t *testing.T) {
	t.Helper()
	if gui.DefaultFrame() == nil {
		gui.NewFrame()
	}
	problemsPanel = ged.NewProblemsPanel()
	buildOutput = ged.NewBuildOutput()
	t.Cleanup(func() {
		problemsPanel = nil
		buildOutput = nil
	})
}

// paneRows is the 问题 pane as the user reads it, one locator per row.
func paneRows() []string {
	out := []string{}
	for _, p := range problemsPanel.Problems() {
		out = append(out, p.File)
	}
	return out
}

func sameRows(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// designWithUnknownWidget opens a design whose only widget names a factory
// this build does not have. The loader drops it and carries on, which is the
// state the whole 问题 row exists to report: the design in memory is short a
// widget and a 保存 would write that loss to disk.
func designWithUnknownWidget(t *testing.T, filename string) *ged.GedView {
	t.Helper()
	ghost := core.NewTDoc()
	ghost.SetValue("gui.Ghost")
	ghost.WriteAttr("name", "ghost")
	children := core.NewTDoc()
	children.SetKey("children")
	children.AddChild(ghost)
	root := core.NewTDoc()
	root.SetValue("form")
	root.WriteAttr("title", "A")
	root.AddChild(children)

	gv := ged.NewGedView()
	scene := gv.GedScene()
	if err := scene.LoadDesign(root); err != nil {
		t.Fatalf("partial load must not error: %v", err)
	}
	// Without this the test would pass on an empty pane and prove nothing.
	if got := scene.MissingWidgets(); len(got) != 1 {
		t.Fatalf("fixture loaded %v missing widgets, want exactly gui.Ghost", got)
	}
	scene.SetFilename(filename)
	return gv
}

// TestUnrelatedFailureKeepsAnOpenDesignsWarning is the reported defect, played
// out: a design is open having lost a widget on load, then a completely
// different command fails. That failure is not compiler output, so it parsed
// to zero rows — and, replacing the whole pane, took the warning with it while
// the design was still open and still missing the widget.
func TestUnrelatedFailureKeepsAnOpenDesignsWarning(t *testing.T) {
	newProblemsPane(t)
	gv := designWithUnknownWidget(t, "A.silkui")
	reportLoadProblems(gv.GedScene())
	if got := paneRows(); !sameRows(got, "gui.Ghost") {
		t.Fatalf("opening the design left %v in the pane, want the load warning", got)
	}

	reportFailure("打开失败", "B.silkui\n\nopen B.silkui: no such file or directory")

	if got := paneRows(); !sameRows(got, "gui.Ghost") {
		t.Errorf("after an unrelated failure the pane reads %v; A is still open and still missing gui.Ghost", got)
	}
}

// TestSuccessfulBuildKeepsTheLoadWarning: a build that passes says the
// generated code compiles, which it does precisely because the dropped widget
// is not in it. Clearing the pane on success announced that the design was
// whole again.
func TestSuccessfulBuildKeepsTheLoadWarning(t *testing.T) {
	newProblemsPane(t)
	gv := designWithUnknownWidget(t, "A.silkui")
	reportLoadProblems(gv.GedScene())
	reportBuildFailure("main.go:10:6: undefined: Foo\n")
	if got := paneRows(); !sameRows(got, "gui.Ghost", "main.go") {
		t.Fatalf("after a failed build the pane reads %v, want the warning and the compiler row", got)
	}

	reportBuildSuccess()

	if got := paneRows(); !sameRows(got, "gui.Ghost") {
		t.Errorf("after a clean build the pane reads %v, want the load warning kept and only the build row gone", got)
	}
}

// TestBuildFailureReplacesThePreviousBuildsRows: the build's own rows are a
// statement about this build, so they are the one thing the next build is
// entitled to overwrite.
func TestBuildFailureReplacesThePreviousBuildsRows(t *testing.T) {
	newProblemsPane(t)
	reportBuildFailure("first.go:1:1: undefined: Foo\n")
	reportBuildFailure("second.go:2:2: undefined: Bar\n")

	if got := paneRows(); !sameRows(got, "second.go") {
		t.Errorf("pane reads %v, want the previous build's row replaced", got)
	}
}

// TestRunningADesignDoesNotWipeItsLoadWarning covers the first thing 运行 does:
// it reports the widgets a generate would drop. There are none — the widget
// that went missing never entered the scene — and filing that empty result
// must not reach the warning about the widget that did.
func TestRunningADesignDoesNotWipeItsLoadWarning(t *testing.T) {
	newProblemsPane(t)
	gv := designWithUnknownWidget(t, "A.silkui")
	reportLoadProblems(gv.GedScene())

	if reportDesignProblems(gv.GedScene(), "运行") {
		t.Fatal("the loaded design has nothing unbuildable in it; the fixture no longer reproduces the case")
	}

	if got := paneRows(); !sameRows(got, "gui.Ghost") {
		t.Errorf("after 运行 checked the design the pane reads %v, want the load warning", got)
	}
}

// TestClosingTheDesignDropsItsRows drives the dock's own close path rather
// than the callback: a GedView with no Close() method is closed without the
// application ever hearing about it, and the rows of a design nobody can open
// any more stay on screen for ever.
func TestClosingTheDesignDropsItsRows(t *testing.T) {
	newProblemsPane(t)
	ged.DocClosedCallback = onDesignViewClosed // what main registers
	t.Cleanup(func() { ged.DocClosedCallback = nil })

	f := gui.NewFrame()
	dock := f.SuggestDocDock()
	gv := designWithUnknownWidget(t, "A.silkui")
	dock.AddView(gv)
	reportLoadProblems(gv.GedScene())
	reportBuildFailure("main.go:10:6: undefined: Foo\n")
	if got := paneRows(); !sameRows(got, "gui.Ghost", "main.go") {
		t.Fatalf("pane reads %v before the close, want both rows", got)
	}

	if !dock.CloseView(gv) {
		t.Fatal("the dock did not close the design view")
	}

	if got := paneRows(); !sameRows(got, "main.go") {
		t.Errorf("after closing the design the pane reads %v, want its rows gone and the build's kept", got)
	}
}

// TestRunReportsThroughTheBuildReporters: onRun hands its build result to a
// gui.Post closure no test can enter — it wants a live frame, a scene and a
// toolchain — so the two reporters exercised above are reached only if onRun
// still calls them. Writing the pane inline there is how the success path came
// to clear rows that were never the build's to clear.
func TestRunReportsThroughTheBuildReporters(t *testing.T) {
	body := funcBody(t, "design.go", "func onRun() {")
	for _, want := range []string{"reportBuildFailure(", "reportBuildSuccess()"} {
		if !strings.Contains(body, want) {
			t.Errorf("onRun no longer calls %s; its build result is reported some other way", want)
		}
	}
	if strings.Contains(body, "problemsPanel") {
		t.Error("onRun writes the 问题 pane directly; only the reporters know which rows the build owns")
	}
}

// TestMainRegistersTheDocClosedCallback: the close above only reaches the pane
// if the application has claimed the callback, and main is the one place that
// does. It cannot be called from a test — it opens a window — so this reads
// the assignment out of the syntax tree instead.
func TestMainRegistersTheDocClosedCallback(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "design.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot parse design.go: %v", err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("design.go has no func main")
	}

	handler := ""
	ast.Inspect(mainFn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		sel, ok := as.Lhs[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "DocClosedCallback" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "ged" {
			return true
		}
		if fn, ok := as.Rhs[0].(*ast.Ident); ok {
			handler = fn.Name
		}
		return false
	})

	if handler == "" {
		t.Fatal("main never sets ged.DocClosedCallback; a closed design keeps its 问题 rows for the rest of the session")
	}
	if handler != "onDesignViewClosed" {
		t.Errorf("main registers %s as the doc-closed handler; the tested one is onDesignViewClosed", handler)
	}
}

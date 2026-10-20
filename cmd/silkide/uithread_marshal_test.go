package main

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Widgets in this framework have no locking: mutating one from a worker
// goroutine races the paint pass and corrupts state silently — no panic, no
// error, just a pane that eventually draws garbage or a map written while the
// event loop ranges it. Every one of these helpers ends in a widget mutation,
// so a goroutine may only reach them through onUI / gui.Post.
//
// Two of the toolchain runners had drifted off that rule (runSingleTest, whose
// own doc comment claimed it matched runProjectTests, and runProjectWithCoverage,
// which also ranged openEditors off-thread), as had the hot-reload callback,
// which reopened the whole scene on the file watcher's goroutine.
var uiMutatingCalls = regexp.MustCompile(`\b(reportBuildOutput|silkideToast|dockSetActiveView|adoptCoverage|applyCoverageToOpenEditors|logEvent|globalTestResults\.SetOutput|scene\.OpenFile|reloadCleanDesign|canvas\.GedScene)\(`)

var (
	goroutineStart = regexp.MustCompile(`\bgo func\s*\(`)
	marshalStart   = regexp.MustCompile(`\b(onUI|gui\.Post)\(func\s*\(`)
)

// TestNoWidgetMutationOffTheUIThread scans main.go for a call into any of the
// helpers above that sits inside a `go func` body but outside an onUI /
// gui.Post closure. Source-level because the offending paths need a live IDE
// (a frame, a dock, a scene) to run at all, so no headless test can reach them
// — but the shape is exactly what a reviewer checks by eye, and this does not
// get tired.
func TestNoWidgetMutationOffTheUIThread(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}

	// depth tracks braces; the stack records where each goroutine / marshalling
	// closure opened, so both can nest in either order.
	type scope struct {
		marshalled bool
		depth      int
	}
	var stack []scope
	depth := 0

	for i, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		startsGoroutine := goroutineStart.MatchString(line)
		startsMarshal := marshalStart.MatchString(line)

		inGoroutine, marshalled := false, false
		for _, s := range stack {
			if s.marshalled {
				marshalled = true
			} else {
				inGoroutine = true
			}
		}
		if inGoroutine && !marshalled && !startsMarshal &&
			!strings.HasPrefix(trimmed, "//") && uiMutatingCalls.MatchString(line) {
			t.Errorf("main.go:%d mutates the UI from a goroutine; wrap it in onUI:\n\t%s", i+1, trimmed)
		}

		switch {
		case startsGoroutine:
			stack = append(stack, scope{depth: depth})
		case startsMarshal:
			stack = append(stack, scope{marshalled: true, depth: depth})
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		for len(stack) > 0 && depth <= stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}
	}
}

// TestRunSingleTestMarshalsItsResult pins the one that regressed by copy-paste:
// runSingleTest was cloned from runProjectTests, kept the "main-thread dispatch
// matches runProjectTests" comment, and dropped the onUI wrapper. The scanner
// above would also catch it, but only while the helper names stay recognisable;
// this states the requirement for the function outright.
func TestRunSingleTestMarshalsItsResult(t *testing.T) {
	body := silkideFuncBody(t, "func runSingleTest(canvas *ged.GedView, name string) {")
	spawn := strings.Index(body, "go func() {")
	if spawn < 0 {
		t.Fatal("runSingleTest no longer runs the toolchain on a goroutine; it would block the event loop for the whole test run")
	}
	post := strings.Index(body, "onUI(func() {")
	if post < 0 || post < spawn {
		t.Error("runSingleTest does not hand its result back through onUI; the panes and toast would be mutated off the event loop")
	}
}

// TestHotReloadReopensOnTheUIThread pins the file-watcher path. fswatch calls
// onReload on its own goroutine, and GedScene.OpenFile rebuilds every item and
// repaints — the single largest off-thread mutation in the IDE.
func TestHotReloadReopensOnTheUIThread(t *testing.T) {
	body := silkideFuncBody(t, "func startReloader(canvas *ged.GedView) {")
	open := strings.Index(body, "reloadCleanDesign(canvas.GedScene(), path)")
	if open < 0 {
		t.Fatal("startReloader no longer calls the guarded reload helper; update this test to match")
	}
	post := strings.Index(body, "onUI(func() {")
	if post < 0 || post > open {
		t.Error("the hot-reload callback reopens the scene on the watcher goroutine instead of posting to the UI thread")
	}
}

// silkideFuncBody returns the text between the braces of the named function in
// main.go.
func silkideFuncBody(t *testing.T, header string) string {
	t.Helper()
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	start := strings.Index(text, header)
	if start < 0 {
		t.Fatalf("main.go does not contain %q", header)
	}
	set := gotoken.NewFileSet()
	parsed, err := parser.ParseFile(set, "source.go", text, 0)
	if err != nil {
		t.Fatalf("cannot parse source: %v", err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Body != nil && set.Position(fn.Pos()).Offset == start {
			return text[start+len(header) : set.Position(fn.Body.Rbrace).Offset]
		}
	}
	t.Fatalf("%q is not a function declaration", header)
	return ""
}

func TestSilkideFuncBodyWithCRLFAndRawStringBraces(t *testing.T) {
	t.Chdir(t.TempDir())
	source := "package fixture\r\nfunc fixture() {\r\n_ = `\r\n}\r\n`\r\nlastCall()\r\n}"
	if err := os.WriteFile("main.go", []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	body := silkideFuncBody(t, "func fixture() {")
	if !strings.Contains(body, "lastCall()") || strings.Contains(body, "\r") {
		t.Fatalf("invalid function boundary: %q", body)
	}
}

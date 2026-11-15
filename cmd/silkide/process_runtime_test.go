package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/point"
)

func processRuntimeProject() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "disk project", Width: 640, Height: 360, Points: []hmi.Point{{Definition: point.Definition{Name: "BI11", Kind: point.Int}, Initial: int64(41)}}, Nodes: []hmi.Node{{ID: "value", Kind: "value", X: 20, Y: 20, Width: 180, Height: 80, Tag: "BI11"}}}
}
func testProcessRuntime(t *testing.T) (*workbench, *processRuntimePane, *gui.CodeEditor, string) {
	t.Helper()
	w := testWorkbench(t)
	bottom := w.center.SplitNewDock(false, true).(*gui.Dock)
	globalBottomDock = bottom
	bottom.SetRetainClosedViews(true)
	p := newProcessRuntimePane(w, bottom)
	bottom.AddView(p)
	w.processRuntime = p
	w.frame.SetSize(1400, 900)
	t.Cleanup(func() {
		p.shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.wait(ctx); err != nil {
			t.Error(err)
		}
	})
	data, err := processRuntimeProject().JSON()
	if err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, t.TempDir(), "plant.silkhmi", string(data))
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	p.load = func(_ context.Context, s string, done func(hmi.Project, error)) error {
		project, err := hmi.Parse([]byte(s))
		done(project, err)
		return nil
	}
	p.startRuntime = func() {}
	return w, p, ed, string(data)
}
func TestProcessRuntimeMenuPaletteAndF5UseUnsavedOwnedSource(t *testing.T) {
	w, p, ed, disk := testProcessRuntime(t)
	project := processRuntimeProject()
	project.Title = "unsaved project"
	project.Points[0].Initial = int64(42)
	data, _ := project.JSON()
	ed.ReplaceAllText(string(data))
	ed.SetFocus()
	started := 0
	p.startRuntime = func() {
		started++
		snapshot := p.view.Diagram.Project()
		raw, err := point.Convert(snapshot.Points[0].Initial, point.Int)
		if snapshot.Title != "unsaved project" || err != nil || raw != int64(42) {
			t.Fatal("did not use source buffer", snapshot.Points[0].Initial)
		}
	}
	menu := buildWorkbenchSubMenu("Tools", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	b := menuCommandButton(menu, "tools.processRun")
	if b == nil || !b.IsEnabled() || b.Icon() == nil {
		t.Fatal("missing/disabled Run 2D menu/icon")
	}
	b.Action().Trigger(nil)
	if started != 1 || !p.current() || !ed.Modified() || !ed.CanUndo() || globalBottomDock.ActiveView() != p {
		t.Fatal("run changed source/ownership or failed reveal")
	}
	actual, _ := os.ReadFile(ed.FilePath())
	if string(actual) != disk {
		t.Fatal("runtime wrote source")
	}
	p.stop()
	if !dispatchProcessRuntime(w.canvas) || started != 2 {
		t.Fatal("F5 route did not dispatch .silkhmi")
	}
	registerPaletteCommands(w.tabs, w.canvas)
	found := false
	for _, cmd := range paletteCommands {
		if cmd.Name == i18n.T("Run 2D Process") {
			found = true
			if cmd.Bind == nil || !cmd.Available() {
				t.Fatal("palette binding missing")
			}
			cmd.Bind().Fn()
		}
	}
	if !found || started != 3 {
		t.Fatal("palette did not run")
	}
	ed.Undo()
	if ed.Text() != disk || p.view.CanStart() {
		t.Fatal("source Undo not preserved/invalidation missing")
	}
	p.focusSource()
	if activeEditor(w.tabs) != ed {
		t.Fatal("source navigation failed after edit")
	}
}
func TestProcessRuntimeLateDecodeCancelCloseAndIdentityGuards(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	var complete func(hmi.Project, error)
	var ctx context.Context
	starts := 0
	p.startRuntime = func() { starts++ }
	p.load = func(c context.Context, _ string, done func(hmi.Project, error)) error {
		ctx = c
		complete = done
		return nil
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	if !p.loading {
		t.Fatal("decode not reserved")
	}
	ed.ReplaceAllText(ed.Text() + "\n")
	if p.loading || ctx.Err() == nil {
		t.Fatal("change did not cancel decode")
	}
	complete(processRuntimeProject(), nil)
	if starts != 0 {
		t.Fatal("late decode started I/O")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	p.stop()
	complete(processRuntimeProject(), nil)
	if starts != 0 || p.loading {
		t.Fatal("stopped decode started")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	p.shutdown()
	complete(processRuntimeProject(), nil)
	if starts != 0 || p.current() {
		t.Fatal("closed owner started")
	}
}
func TestProcessRuntimeSourceCloseRenameAndHiddenDockRetire(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	runProcessRuntime(w.tabs, w.canvas, ed)
	if !p.view.CanStart() {
		t.Fatal("valid snapshot cannot restart")
	}
	if !globalBottomDock.PromptSaveCloseView(p) {
		t.Fatal("dock hide failed")
	}
	if !p.reveal() || globalBottomDock.ActiveView() != p {
		t.Fatal("retained tool did not reopen")
	}
	if !globalBottomDock.CloseView(p) || !p.reveal() {
		t.Fatal("removed tool did not reattach")
	}
	retireProcessRuntime(ed)
	if p.owner != nil || p.view.CanStart() {
		t.Fatal("rename/close kept editor ownership")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	idx := editorTabIndex(w.tabs, ed)
	if !closeEditorTab(w.tabs, idx) {
		t.Fatal("clean tab close failed")
	}
	if p.owner != nil || p.current() || p.view.CanStart() {
		t.Fatal("closing editor kept runtime snapshot")
	}
}
func TestProcessRuntimeDecodeFailureSwitchBusyAndStoreNamespace(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	starts := 0
	p.startRuntime = func() { starts++ }
	p.load = func(context.Context, string, func(hmi.Project, error)) error {
		return errors.New("fixture decode unavailable")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	if p.loading || starts != 0 || p.view.CanStart() {
		t.Fatal("failed decode left runnable old snapshot")
	}
	var complete func(hmi.Project, error)
	p.load = func(_ context.Context, _ string, done func(hmi.Project, error)) error { complete = done; return nil }
	runProcessRuntime(w.tabs, w.canvas, ed)
	other := writeFile(t, t.TempDir(), "other.go", "package qa\n")
	openFileInEditor(w.tabs, other)
	complete(processRuntimeProject(), nil)
	if starts != 0 {
		t.Fatal("late decode ran in other editor context")
	}
	if dispatchProcessRuntime(w.canvas) {
		t.Fatal("ordinary Go file stolen by process runner")
	}
	a := processRuntimeDatabase(ed.FilePath())
	b := processRuntimeDatabase(filepath.Join(filepath.Dir(ed.FilePath()), "other.silkhmi"))
	if a == b || strings.Contains(filepath.Base(a), "plant") {
		t.Fatal("point stores not isolated/opaque")
	}
	if processRuntimeDatabase(filepath.Join(filepath.Dir(ed.FilePath()), ".", filepath.Base(ed.FilePath()))) != a {
		t.Fatal("equivalent path changed namespace")
	}
	w.activate(modeDesign)
	if dispatchProcessRuntime(w.canvas) {
		t.Fatal("design run stolen by hidden source")
	}
}

func TestProcessRuntimeAncestorHideCancelsPendingDecodeWithoutAutoRestart(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	var finish func(hmi.Project, error)
	var loadingContext context.Context
	starts := 0
	p.load = func(ctx context.Context, _ string, done func(hmi.Project, error)) error {
		loadingContext = ctx
		finish = done
		return nil
	}
	p.startRuntime = func() { starts++ }
	runProcessRuntime(w.tabs, w.canvas, ed)
	globalBottomDock.SetVisible(false)
	if p.loading || loadingContext.Err() == nil {
		t.Fatal("hidden dock kept pending acquisition start")
	}
	globalBottomDock.SetVisible(true)
	finish(processRuntimeProject(), nil)
	if starts != 0 {
		t.Fatal("showing dock restarted cancelled load")
	}
}

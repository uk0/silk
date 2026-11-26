package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	openTestEditorReady(t, w.tabs, path)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	ed.SetFocus()
	p.load = func(_ context.Context, s string, done func(indexedProcessAsset, error)) error {
		asset, err := decodeProcessSource(context.Background(), s)
		done(asset, err)
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
	if activeTestEditorReady(t, w.tabs) != ed {
		t.Fatal("source navigation failed after edit")
	}
}

func TestProcessRuntimeDockRunsLatestBoundBufferWithoutSwitchingEditors(t *testing.T) {
	w, p, ed, disk := testProcessRuntime(t)
	started := 0
	p.startRuntime = func() { started++ }
	runProcessRuntime(w.tabs, w.canvas, ed)
	if started != 1 {
		t.Fatal("initial bound source did not stage")
	}

	getRunButton := func() *gui.Button {
		t.Helper()
		button, ok := p.view.FindWidgetAt(12, 12).(*gui.Button)
		if !ok {
			t.Fatal("missing Run action in the runtime toolbar")
		}
		return button
	}
	ownedPath := ed.FilePath()
	otherPath := writeFile(t, t.TempDir(), "other.silkhmi", disk)
	openTestEditorReady(t, w.tabs, otherPath)
	other := activeTestEditorReady(t, w.tabs)
	if other == ed || other.FilePath() != otherPath {
		t.Fatal("wrong active document")
	}

	ed.ReplaceAllText("{invalid draft")
	generation := p.generation
	for _, draft := range []string{"{invalid", "{invali", "{invalid draft"} {
		ed.ReplaceAllText(draft)
		if p.generation != generation || !p.invalidated {
			t.Fatal("keystroke retired an already invalidated snapshot")
		}
	}
	run := getRunButton()
	if !run.IsEnabled() || run.Text() != i18n.T("Run latest") || p.view.CanStart() {
		t.Fatal("invalidated snapshot was not replaced with an explicit source action")
	}
	run.Action().Trigger(nil)
	if started != 1 || p.view.CanStart() || !run.IsEnabled() || p.owner != ed || activeTestEditorReady(t, w.tabs) != other {
		t.Fatal("invalid source started old acquisition, lost retry or switched documents")
	}

	project := processRuntimeProject()
	project.Title = "latest unsaved source"
	project.Points[0].Initial = int64(42)
	latest, err := project.JSON()
	if err != nil {
		t.Fatal(err)
	}
	ed.ReplaceAllText(string(latest))
	run.Action().Trigger(nil)
	snapshot := p.view.Diagram.Project()
	raw, err := point.Convert(snapshot.Points[0].Initial, point.Int)
	if started != 2 || p.owner != ed || !p.current() || p.path != ownedPath ||
		snapshot.Title != project.Title || err != nil || raw != int64(42) || run.Text() != i18n.T("Run snapshot") {
		t.Fatal("dock did not stage its latest owned snapshot", snapshot.Title, raw, err)
	}
	if activeTestEditorReady(t, w.tabs) != other || ed.Text() != string(latest) || !ed.Modified() || !ed.CanUndo() {
		t.Fatal("source action changed the active editor, text or undo state")
	}
	onDisk, err := os.ReadFile(ownedPath)
	if err != nil || string(onDisk) != disk {
		t.Fatal("running a draft changed the source on disk", err)
	}

	p.retire(ed)
	if run.IsEnabled() || p.owner != nil {
		t.Fatal("retired editor retained a source action")
	}
	run.Action().Trigger(nil)
	if started != 2 {
		t.Fatal("retired source ran")
	}
}

func TestProcessRuntimeDockRejectsLateDecodeAfterFurtherEdit(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	runProcessRuntime(w.tabs, w.canvas, ed)
	ed.ReplaceAllText(ed.Text() + "\n")
	run, ok := p.view.FindWidgetAt(12, 12).(*gui.Button)
	if !ok || !run.IsEnabled() {
		t.Fatal("missing Run latest action")
	}
	var complete func(indexedProcessAsset, error)
	var ctx context.Context
	starts, queued := 0, 0
	p.startRuntime = func() { starts++ }
	p.load = func(c context.Context, _ string, finish func(indexedProcessAsset, error)) error {
		queued++
		ctx, complete = c, finish
		return nil
	}
	run.Action().Trigger(nil)
	if queued != 1 || complete == nil || !p.loading || run.IsEnabled() {
		t.Fatal("source run did not reserve a single bounded decode")
	}
	run.Action().Trigger(nil)
	if queued != 1 {
		t.Fatal("duplicate source run bypassed decode readiness")
	}
	asset, err := decodeProcessSource(context.Background(), ed.Text())
	ed.ReplaceAllText(ed.Text() + "\n")
	if ctx.Err() == nil || p.loading || !run.IsEnabled() {
		t.Fatal("second edit failed to retire and restore the source action")
	}
	complete(asset, err)
	if starts != 0 || p.index != nil || p.view.CanStart() {
		t.Fatal("obsolete source callback republished a retired snapshot")
	}
}

func TestProcessRuntimeItemJumpTracksExactUnsavedSourceAndHistory(t *testing.T) {
	w, p, ed, disk := testProcessRuntime(t)
	project := processRuntimeProject()
	project.Nodes = append(project.Nodes, hmi.Node{ID: "tank", Kind: "tank", X: 420, Y: 20, Width: 90, Height: 80})
	project.Links = []hmi.Link{{ID: "flow", From: hmi.Endpoint{Node: "value", Port: "east"}, To: hmi.Endpoint{Node: "tank", Port: "west"}}}
	project.Alarms = []hmi.AlarmRule{{ID: "压力报警", Point: "BI11", Condition: hmi.AlarmHigh, Threshold: int64(70), Reset: int64(65), Priority: hmi.AlarmWarning}}
	data, err := project.JSON()
	if err != nil {
		t.Fatal(err)
	}
	draft := strings.Replace(string(data), `"name": "BI11"`, `"description": "压力 🏭", "name": "BI11"`, 1)
	draft = strings.ReplaceAll(draft, "\n", "\r\n")
	ed.ReplaceAllText(draft)
	runProcessRuntime(w.tabs, w.canvas, ed)
	if p.index == nil || !p.current() {
		t.Fatal("exact snapshot is unavailable")
	}
	otherFile := writeFile(t, t.TempDir(), "other.go", "package qa\n")
	openTestEditorReady(t, w.tabs, otherFile)
	other := activeTestEditorReady(t, w.tabs)
	other.NavigateToLineCol(0, 5)
	origin := other.NavigationPosition()
	clearEditorNavigation(w.tabs)
	verify := func(target hmi.SourceTarget, marker string) {
		t.Helper()
		p.focusSourceTarget(target)
		s := ed.Text()
		at := strings.Index(s, marker)
		if at < 0 {
			t.Fatal("source fixture lacks marker", marker)
		}
		at += len(marker) - len(target.ID) - 1
		wantLine := strings.Count(s[:at], "\n")
		wantCol := utf8.RuneCountInString(s[strings.LastIndexByte(s[:at], '\n')+1 : at])
		pos := ed.NavigationPosition()
		if activeEditor(w.tabs) != ed || pos.Line != wantLine || pos.Column != wantCol {
			t.Fatal("wrong editor coordinate", pos, wantLine, wantCol)
		}
	}
	verify(hmi.SourceTarget{Kind: hmi.SourcePoint, ID: "BI11"}, `"description": "压力 🏭", "name": "BI11"`)
	navigateEditorHistory(w.tabs, false)
	if activeEditor(w.tabs) != other || other.NavigationPosition() != origin {
		t.Fatal("Back did not return to the other buffer", editorNavigationPosition(w.tabs), origin)
	}
	navigateEditorHistory(w.tabs, true)
	if activeEditor(w.tabs) != ed {
		t.Fatal("Forward lost the runtime document")
	}
	verify(hmi.SourceTarget{Kind: hmi.SourceNode, ID: "tank"}, `"id": "tank"`)
	verify(hmi.SourceTarget{Kind: hmi.SourceLink, ID: "flow"}, `"id": "flow"`)
	verify(hmi.SourceTarget{Kind: hmi.SourceAlarm, ID: "压力报警"}, `"id": "压力报警"`)
	if ed.Text() != draft || !ed.Modified() || !ed.CanUndo() || p.view.Busy() {
		t.Fatal("navigation changed document/history or started acquisition")
	}
	raw, err := os.ReadFile(ed.FilePath())
	if err != nil || string(raw) != disk {
		t.Fatal("navigation changed disk", err)
	}
	focusEditorTab(w.tabs, other)
	beforeFocus, beforeHistory := ed.NavigationPosition(), editorNavigationCount(w.tabs)
	// A newer synchronous tab choice is authoritative even when the original
	// source and its revision are still valid.
	w.tabs.SetCurrentChangedCallback(func(_ interface{}, _ int) {
		if activeEditor(w.tabs) == ed {
			focusEditorTab(w.tabs, other)
		}
	})
	p.focusSourceTarget(hmi.SourceTarget{Kind: hmi.SourcePoint, ID: "BI11"})
	if activeEditor(w.tabs) != other || ed.NavigationPosition() != beforeFocus || editorNavigationCount(w.tabs) != beforeHistory {
		t.Fatal("runtime jump overrode reentrant tab selection")
	}
	ed.ReplaceAllText("\r\n" + draft)
	openTestEditorReady(t, w.tabs, otherFile)
	before := editorNavigationCount(w.tabs)
	p.focusSourceTarget(hmi.SourceTarget{Kind: hmi.SourceAlarm, ID: "压力报警"})
	if p.index != nil || activeEditor(w.tabs) != other || editorNavigationCount(w.tabs) != before {
		t.Fatal("stale item moved editor or created history")
	}
}
func TestProcessRuntimeLateDecodeCancelCloseAndIdentityGuards(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	var complete func(indexedProcessAsset, error)
	var ctx context.Context
	starts := 0
	p.startRuntime = func() { starts++ }
	p.load = func(c context.Context, _ string, done func(indexedProcessAsset, error)) error {
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
	complete(indexedProcessAsset{project: processRuntimeProject()}, nil)
	if starts != 0 {
		t.Fatal("late decode started I/O")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	p.stop()
	complete(indexedProcessAsset{project: processRuntimeProject()}, nil)
	if starts != 0 || p.loading {
		t.Fatal("stopped decode started")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	p.shutdown()
	complete(indexedProcessAsset{project: processRuntimeProject()}, nil)
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
	if p.owner != nil || p.index != nil || p.view.CanStart() {
		t.Fatal("rename/close kept editor ownership")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	idx := editorTabIndex(w.tabs, ed)
	if !closeEditorTab(w.tabs, idx) {
		t.Fatal("clean tab close failed")
	}
	if p.owner != nil || p.index != nil || p.current() || p.view.CanStart() {
		t.Fatal("closing editor kept runtime snapshot")
	}
}
func TestProcessRuntimeDecodeFailureSwitchBusyAndStoreNamespace(t *testing.T) {
	w, p, ed, _ := testProcessRuntime(t)
	starts := 0
	p.startRuntime = func() { starts++ }
	p.load = func(context.Context, string, func(indexedProcessAsset, error)) error {
		return errors.New("fixture decode unavailable")
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	if p.loading || starts != 0 || p.view.CanStart() {
		t.Fatal("failed decode left runnable old snapshot")
	}
	var complete func(indexedProcessAsset, error)
	p.load = func(_ context.Context, _ string, done func(indexedProcessAsset, error)) error {
		complete = done
		return nil
	}
	runProcessRuntime(w.tabs, w.canvas, ed)
	other := writeFile(t, t.TempDir(), "other.go", "package qa\n")
	openTestEditorReady(t, w.tabs, other)
	complete(indexedProcessAsset{project: processRuntimeProject()}, nil)
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
	var finish func(indexedProcessAsset, error)
	var loadingContext context.Context
	starts := 0
	p.load = func(ctx context.Context, _ string, done func(indexedProcessAsset, error)) error {
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
	finish(indexedProcessAsset{project: processRuntimeProject()}, nil)
	if starts != 0 {
		t.Fatal("showing dock restarted cancelled load")
	}
}

func TestProcessRuntimeLoadingAndFailedSourceCannotFallThroughToGoRun(t *testing.T) {
	w, _, _, _ := testProcessRuntime(t)
	p := newRestorePage(editorSession{Path: filepath.Join(t.TempDir(), "loading.silkhmi")})
	w.tabs.AddTab(p, "loading.silkhmi", nil)
	w.tabs.SetCurrentIndex(w.tabs.Count() - 1)
	w.activate(modeEdit)
	if !dispatchProcessRuntime(w.canvas) {
		t.Fatal("pending process document fell through to unrelated Go Run")
	}
	p.label.SetText("failed to read source")
	if !dispatchProcessRuntime(w.canvas) {
		t.Fatal("failed process document fell through to unrelated Go Run")
	}
}

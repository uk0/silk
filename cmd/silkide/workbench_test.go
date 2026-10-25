package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func testWorkbench(t *testing.T) *workbench {
	t.Helper()
	old, oldEditors := globalWorkbench, openEditors
	oldMacros := macroEditors
	macroEditors = map[string]*macroEditorState{}
	left, right, bottom := globalLeftDock, globalRightDock, globalBottomDock
	globalLeftDock, globalRightDock, globalBottomDock = nil, nil, nil
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() {
		for _, ed := range openEditors {
			if ed != nil {
				ed.CancelGitStatus()
				delete(editorGitManualRefresh, ed)
			}
		}
		retireMacroEditors()
		macroEditors = oldMacros
		globalWorkbench, openEditors = old, oldEditors
		globalLeftDock, globalRightDock, globalBottomDock = left, right, bottom
	})
	frame := gui.NewFrame()
	dock := frame.SuggestDocDock().(*gui.Dock)
	w := &workbench{frame: frame, center: dock, tabs: buildEditorTabs(nil), canvas: ged.NewGedView(), welcome: ged.NewWelcomeScreen()}
	dock.AddView(w.tabs)
	dock.AddView(w.canvas)
	dock.AddView(w.welcome)
	w.canvas.GedScene().SetFilename(filepath.Join(t.TempDir(), "Form.silkui"))
	globalWorkbench = w
	dock.SetTabChangedCallback(func(_ int) { w.syncHistoryActions() })
	t.Cleanup(w.closeHistoryActions)
	return w
}

func TestWorkbenchStartsWithoutFakeDocuments(t *testing.T) {
	w := testWorkbench(t)
	if w.tabs.Count() != 0 || len(openEditors) != 0 {
		t.Fatal("synthetic unsaveable tabs remain")
	}
	if w.tabs.Title() == "" {
		t.Fatal("editor dock has no visible title")
	}
}

func TestWorkbenchModeButtonsUseActualDockView(t *testing.T) {
	w := testWorkbench(t)
	tb := gui.NewToolBar()
	addWorkspaceActions(tb)
	for _, mode := range []workspaceMode{modeEdit, modeDesign, modeWelcome} {
		w.buttons[mode].Action().Trigger(nil)
		if got := documentMode(w.tabs, w.canvas); got != mode {
			t.Fatalf("mode = %q, want %q", got, mode)
		}
		for m, button := range w.buttons {
			if button.Action().IsChecked() != (m == mode) {
				t.Fatalf("wrong selected button %s", m)
			}
		}
	}
	// A direct dock-tab click must override the old toolbar state.
	w.center.SetActiveIndex(w.center.IndexOfView(w.canvas))
	w.syncMode()
	if !w.buttons[modeDesign].Action().IsChecked() {
		t.Fatal("mode state drifted from dock")
	}
}

func TestSaveEntrypointsTargetVisibleEditor(t *testing.T) {
	for _, entry := range []string{"shortcut", "palette", "toolbar"} {
		t.Run(entry, func(t *testing.T) {
			w := testWorkbench(t)
			path := writeFile(t, t.TempDir(), "main.go", "package main\n")
			if !openFileInEditor(w.tabs, path) {
				t.Fatal("open failed")
			}
			ed := activeEditor(w.tabs)
			ed.ReplaceAllText("package main\nfunc added(){}\n")
			switch entry {
			case "shortcut":
				registerShortcuts(w.tabs, w.canvas)
				gui.ShortcutHandler(gui.ModAction, 'S')()
			case "palette":
				old := paletteCommands
				paletteCommands = nil
				t.Cleanup(func() { paletteCommands = old })
				registerPaletteCommands(w.tabs, w.canvas)
				found := false
				for _, cmd := range paletteCommands {
					if cmd.Name == "Save" {
						cmd.Fn()
						found = true
						break
					}
				}
				if !found {
					t.Fatal("Save command missing")
				}
			case "toolbar":
				buildToolBar(w.frame, w.tabs, w.canvas)
				found := false
				for _, child := range w.frame.Children() {
					if bar, ok := child.(*gui.ToolBar); ok {
						for _, item := range bar.Items() {
							if button, ok := item.(*gui.Button); ok && button.Icon() == paint.LoadIcon("save") {
								button.Action().Trigger(nil)
								found = true
							}
						}
					}
				}
				if !found {
					t.Fatal("Save toolbar button missing")
				}
			}
			if got := diskText(t, path); got != "package main\n\nfunc added() {}\n" {
				t.Fatalf("saved %q", got)
			}
			if ed.Modified() {
				t.Fatal("saved editor still dirty")
			}
			if _, err := os.Stat(w.canvas.GedScene().Filename()); !os.IsNotExist(err) {
				t.Fatal("saved hidden design instead")
			}
		})
	}
}

func TestSaveFailureDoesNotFallThroughToDesign(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	openFileInEditor(w.tabs, path)
	ed := activeEditor(w.tabs)
	ed.ReplaceAllText("package main\nfunc unsaved(){}")
	before := ed.Text()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	registerShortcuts(w.tabs, w.canvas)
	gui.ShortcutHandler(gui.ModAction, 'S')()
	if ed.Text() != before || !ed.Modified() {
		t.Fatal("failed write changed or marked the buffer saved")
	}
	if _, err := os.Stat(w.canvas.GedScene().Filename()); !os.IsNotExist(err) {
		t.Fatal("failure fell through to hidden design")
	}
}

func TestDesignSaveDoesNotSaveHiddenCode(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	openFileInEditor(w.tabs, path)
	activeEditor(w.tabs).ReplaceAllText("package changed\n")
	w.activate(modeDesign)
	if !saveCurrentDocument(w.tabs, w.canvas) {
		t.Fatal("design save failed")
	}
	if got := diskText(t, path); got != "package main\n" {
		t.Fatal("saved hidden code")
	}
	if _, err := os.Stat(w.canvas.GedScene().Filename()); err != nil {
		t.Fatal(err)
	}
}

func TestEditorUndoRedoAndDirtyTabThroughShortcuts(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "sample.txt", "original")
	openFileInEditor(w.tabs, path)
	ed := activeEditor(w.tabs)
	ed.ReplaceAllText("edited")
	if w.tabs.TabTitle(0) != "sample.txt *" {
		t.Fatalf("dirty tab: %q", w.tabs.TabTitle(0))
	}
	registerShortcuts(w.tabs, w.canvas)
	gui.ShortcutHandler(gui.ModAction, 'Z')()
	if ed.Text() != "original" || w.tabs.TabTitle(0) != "sample.txt" {
		t.Fatal("undo did not restore clean code buffer")
	}
	gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'Z')()
	if ed.Text() != "edited" || !ed.Modified() {
		t.Fatal("redo failed")
	}
	if gui.ShortcutHandler(0, 'F') != nil {
		t.Fatal("plain F steals editor input")
	}
}

func TestSaveAllSavesEveryEditedTab(t *testing.T) {
	w := testWorkbench(t)
	for _, name := range []string{"b.txt", "a.txt"} {
		path := writeFile(t, t.TempDir(), name, "before")
		openFileInEditor(w.tabs, path)
		activeEditor(w.tabs).ReplaceAllText("after")
	}
	w.activate(modeWelcome)
	if !saveAllDocuments(w.tabs, w.canvas) {
		t.Fatal("Save All failed")
	}
	for path, ed := range openEditors {
		if diskText(t, path) != "after" || ed.Modified() {
			t.Fatal("unsaved tab", path)
		}
	}
}

func TestOpenFileNormalizesEquivalentPaths(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	openFileInEditor(w.tabs, path)
	equivalent := filepath.Dir(path) + string(os.PathSeparator) + "." + string(os.PathSeparator) + "main.go"
	openFileInEditor(w.tabs, equivalent)
	if w.tabs.Count() != 1 || len(openEditors) != 1 {
		t.Fatal("duplicate document for same path")
	}
}

func TestWorkspaceSessionRoundTripPreservesOrderActiveCursor(t *testing.T) {
	w := testWorkbench(t)
	prefs := seedLastLaunch(t)
	dir := t.TempDir()
	paths := []string{writeFile(t, dir, "b.txt", "one\n中文ABC\nthree"), writeFile(t, dir, "a.txt", "second")}
	for _, path := range paths {
		openFileInEditor(w.tabs, path)
	}
	focusEditorTab(w.tabs, openEditors[paths[0]])
	activeEditor(w.tabs).ScrollToLineCol(1, 3)
	activeEditor(w.tabs).SetScrollY(12)
	state := captureWorkspaceSession(w.tabs, nil)
	prefs.SetWorkspaceSession(state)
	flushTestPreferences(t, prefs)
	loaded, ok := testPreferences(t).WorkspaceSession()
	if !ok || !reflect.DeepEqual(state, loaded) {
		t.Fatalf("session round trip: %+v", loaded)
	}
	openEditors = map[string]*gui.CodeEditor{}
	restored := buildEditorTabs(nil)
	restoreWorkspaceSession(loaded, restored, nil)
	if got := orderedEditorPaths(restored); !reflect.DeepEqual(got, paths) {
		t.Fatalf("order = %v", got)
	}
	ed := activeEditor(restored)
	if activeEditorPath(restored) != paths[0] || ed.CursorLine() != 1 || ed.CursorCol() != 3 || ed.ScrollY() != 12 {
		t.Fatal("active editor / caret / scroll lost")
	}
}

func TestWorkspaceSessionIgnoresMissingFilesAndClampsCursor(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "x.txt", "short")
	state := workspaceSession{Version: 1, Editors: []editorSession{{Path: "/nonexistent/silkide.go"}, {Path: path, Line: 999, Column: 999, ScrollY: -10}}}
	restoreWorkspaceSession(state, w.tabs, nil)
	if w.tabs.Count() != 1 {
		t.Fatal("missing document created a tab")
	}
	ed := activeEditor(w.tabs)
	if ed.CursorLine() != 0 || ed.CursorCol() != 5 {
		t.Fatal("cursor was not clamped")
	}
	if len(existingPaths([]string{t.TempDir()})) != 0 {
		t.Fatal("directory is not a session document")
	}
}

func TestChineseLocaleAliasesTranslateModeActions(t *testing.T) {
	previous := i18n.Locale()
	defer i18n.SetLocale(previous)
	for _, locale := range []string{"zh", "zh_CN", "zh-CN", "zh-Hans"} {
		t.Setenv("SILKIDE_LOCALE", locale)
		installLocale()
		if i18n.T("Design") != "设计" || i18n.T("Save All") != "全部保存" {
			t.Fatalf("untranslated locale %s", locale)
		}
	}
}

func TestPreviewEntrypointsSelectDesignWorkspace(t *testing.T) {
	for _, entry := range []string{"shortcut", "palette", "toolbar"} {
		t.Run(entry, func(t *testing.T) {
			w := testWorkbench(t)
			previous, previousCommands := ged.PreviewCallback, paletteCommands
			t.Cleanup(func() { ged.PreviewCallback, paletteCommands = previous, previousCommands })
			calls := 0
			ged.PreviewCallback = func() {
				calls++
				if documentMode(w.tabs, w.canvas) != modeDesign {
					t.Fatal("preview did not activate the design workspace")
				}
			}
			w.activate(modeWelcome)
			switch entry {
			case "shortcut":
				registerShortcuts(w.tabs, w.canvas)
				gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'R')()
			case "palette":
				paletteCommands = nil
				registerPaletteCommands(w.tabs, w.canvas)
				for _, command := range paletteCommands {
					if command.Name == i18n.T("Preview") {
						command.Fn()
						break
					}
				}
			case "toolbar":
				buildToolBar(w.frame, w.tabs, w.canvas)
				for _, child := range w.frame.Children() {
					if bar, ok := child.(*gui.ToolBar); ok {
						for _, item := range bar.Items() {
							if button, ok := item.(*gui.Button); ok && button.Icon() == paint.LoadIcon("preview") {
								button.Action().Trigger(nil)
							}
						}
					}
				}
			}
			if calls != 1 {
				t.Fatalf("preview calls = %d", calls)
			}
		})
	}
}

func TestDesignHistoryNotifiesDependentPanels(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	calls := 0
	w.canvas.GedScene().AddDesignChangedCallback(func() { calls++ })
	undoCurrentDocument(w.tabs, w.canvas, false)
	undoCurrentDocument(w.tabs, w.canvas, true)
	if calls != 0 {
		t.Fatalf("empty history notified panels: %d", calls)
	}
	w.canvas.GedScene().PushCommand(gui.NewCompoundCommand())
	undoCurrentDocument(w.tabs, w.canvas, false)
	undoCurrentDocument(w.tabs, w.canvas, true)
	if calls != 3 { // one push, one actual Undo, one actual Redo
		t.Fatalf("design history notifications = %d", calls)
	}
}

func TestGuardSaveRegeneratesGoWithoutToast(t *testing.T) {
	w := testWorkbench(t)
	oldToast := silkideToast
	t.Cleanup(func() { silkideToast = oldToast })
	silkideToast = func(string, gui.ToastLevel) { t.Fatal("guard save must not open a toast") }
	w.canvas.GedScene().SetFormTitle("Saved on exit")
	if !saveDesignDocument(w.canvas) {
		t.Fatal("guard save failed")
	}
	path := filepath.Join(filepath.Dir(w.canvas.GedScene().Filename()), "Form.silk.go")
	if content := diskText(t, path); !strings.Contains(content, "Saved on exit") {
		t.Fatalf("generated Go is stale: %s", content)
	}
}

func TestWelcomePreservesManualDockVisibility(t *testing.T) {
	w := testWorkbench(t)
	globalLeftDock = gui.NewDock()
	globalRightDock = gui.NewDock()
	globalLeftDock.SetVisible(true)
	globalRightDock.SetVisible(false)
	w.activate(modeEdit)
	w.syncMode()
	if globalRightDock.IsVisible() {
		t.Fatal("heartbeat reopened hidden dock")
	}
	w.activate(modeWelcome)
	if globalLeftDock.IsVisible() {
		t.Fatal("welcome did not hide tools")
	}
	w.activate(modeEdit)
	if !globalLeftDock.IsVisible() || globalRightDock.IsVisible() {
		t.Fatal("welcome lost manual dock visibility")
	}
}

func TestCloseCapturesModeBeforeDockTeardown(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	openFileInEditor(w.tabs, path)
	w.activate(modeDesign)
	prefs := seedLastLaunch(t)
	previousQuit := quitApp
	quitApp = func() {}
	t.Cleanup(func() { quitApp = previousQuit })
	installClosedPersist(w.frame, w.canvas, prefs)
	w.frame.Close()
	state, ok := prefs.WorkspaceSession()
	if !ok || state.Mode != modeDesign || state.ActiveFile != path {
		t.Fatalf("closed session lost visible mode: %+v", state)
	}
}

func TestProjectDirFindsModuleAboveDesignWithoutOverridingExplicitRoot(t *testing.T) {
	old := globalProjectRoot
	globalProjectRoot = ""
	t.Cleanup(func() { globalProjectRoot = old })
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/project\n")
	designDir := filepath.Join(root, "ui", "dialogs")
	if err := os.MkdirAll(designDir, 0o755); err != nil {
		t.Fatal(err)
	}
	canvas := ged.NewGedView()
	canvas.GedScene().SetFilename(filepath.Join(designDir, "form.silkui"))
	if got := projectDir(canvas); got != root {
		t.Fatalf("build/run root = %q, want module %q", got, root)
	}
	// A nested module is not accidentally built as part of its parent.
	writeFile(t, filepath.Join(root, "ui"), "go.mod", "module example.com/nested\n")
	if got := projectDir(canvas); got != filepath.Join(root, "ui") {
		t.Fatalf("ignored nearest module: %q", got)
	}
	globalProjectRoot = t.TempDir()
	if got := projectDir(canvas); got != globalProjectRoot {
		t.Fatalf("explicit Open Project overridden: %q", got)
	}
}

func TestEnclosingGoProjectIgnoresGoModDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/project\n")
	nested := filepath.Join(root, "ui")
	if err := os.MkdirAll(filepath.Join(nested, "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := enclosingGoProject(nested); got != root {
		t.Fatalf("go.mod directory treated as a module: %q", got)
	}
}

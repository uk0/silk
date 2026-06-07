// silkide is a reference implementation of the JetBrains-style IDE
// layout for the silk framework. It demonstrates how the existing
// silk widgets (Frame, ToolBar, Splitter, TabWidget, FileExplorer,
// CodeEditor, StatusBar) compose into the four-zone IDE shell shown
// in the project's design mockup:
//
//   ┌──────────────────────────────────────────────┐
//   │ ☰ 📁 ↻ 💾 ← →    title    ▶ 🐛 🔍 ⚙        │  ← top toolbar
//   ├──┬──────────┬───────────────────────────────┤
//   │📁│myproject │ main.go [×] | server.go [×]  │  ← left strip + editor tabs
//   │🌿│ ▼ cmd    ├───────────────────────────────┤
//   │🏗│   main.go│ package main                  │
//   │ │ ▼ pkg    │ ...                           │  ← editor body
//   │⚙│  go.mod  ├───────────────────────────────┤
//   │>_│         │ Terminal      Output          │
//   │⚠│          │ ...                           │
//   ├──┴──────────┴───────────────────────────────┤
//   │ myproject | main | Ln 8 Col 12 | UTF-8 …  │  ← status bar
//   └──────────────────────────────────────────────┘
//
// Run with `go run ./cmd/silkide`. The binary opens a sample workspace
// rooted at the current directory; clicking any file in the tree
// opens it in the editor.
//
// This is a demonstration / reference layout — wire-up work like
// run/debug/build hooks live in the parent silk designer (design.go);
// silkide stays focused on the chrome shape so the layout can be
// reviewed and copy-paste-extended without breaking the working
// designer.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"silk/a11y"
	"silk/core"
	"silk/decl"
	"silk/ged"
	"silk/graph"
	"silk/gui"
	"silk/hotreload"
	"silk/i18n"
	"silk/paint"
	"silk/pdfexport"
	"silk/svgexport"
)

func main() {
	// Compatibility note: ged widgets (WidgetList / GedView /
	// ObjectInspector) currently have rendering issues under the
	// silk_no_cairo build tag — text is left-clipped, the design
	// canvas renders blank. The default (Cairo) build path works
	// end-to-end (drag-drop, selection handles, inspector tree).
	// See ROADMAP §3.3.7 "未闭合: ged 子系统在 silk_no_cairo 模式下"
	// for the open follow-up. silkide opts into the default Cairo
	// path automatically (no build tag); users who explicitly want
	// the pure-OpenGL binary should expect the documented ged
	// rendering gaps until that follow-up lands.
	// Locale + persisted preferences come up before the frame so
	// every translated string in the toolbar / status bar resolves
	// correctly the first time, and the saved window size is honoured
	// instead of bouncing through the default and resizing.
	installLocale()
	prefs := newPreferences()
	globalPrefs = prefs

	frame := gui.NewFrameWindow()
	frame.SetUuidStr("c1d8e2f0-1a3b-4c2d-9e7f-silkide00001")
	frame.SetTitle(idTitle())
	gui.SetDefaultFrame(frame)
	globalFrame = frame

	// Order: panels first so the toolbar can capture references to
	// the editor tabs and design canvas, wiring Open / Save buttons
	// through to GedScene.OpenFile / Save without the global plumbing
	// SuggestDocDock would otherwise force.
	editorTabs, designCanvas := buildPanels(frame)
	buildToolBar(frame, editorTabs, designCanvas)
	statusBar := buildStatusBar(frame)
	registerShortcuts(editorTabs, designCanvas)
	registerPaletteCommands(editorTabs, designCanvas)
	startTitleSync(frame, designCanvas)
	if designCanvas != nil {
		rebindAutoSaver(designCanvas.GedScene())
	}

	// Live selection feedback in the status bar's transient message
	// slot. Without this the user has to mouse over to the right-side
	// inspector to confirm what got selected after a click.
	if designCanvas != nil && statusBar != nil {
		designCanvas.AddSelectionCallback(func(items []graph.IItem) {
			n := len(items)
			if n == 0 {
				statusBar.SetMessage("")
				return
			}
			if n == 1 {
				name := itemDisplayName(items[0])
				statusBar.SetMessage(i18n.Tf("Selected: %s", name) + " " + itemBoundsLabel(items[0]))
				return
			}
			statusBar.SetMessage(i18n.Tf("Selected: %d items", n))
		})

		// Status-bar zoom % cell stays in sync with Ctrl+wheel zoom
		// thanks to GraphView.SigZoomChanged. Without this hookup the
		// cell would only refresh on the keyboard shortcuts in
		// prefs.go's zoomCanvas helpers.
		designCanvas.SigZoomChanged(func(_ interface{}, zoom float64) {
			setZoomLabel(zoom)
		})
	}

	// Persist the final window size + position on close so the next
	// launch restores the user's geometry instead of bouncing through
	// a default.
	frame.SetClosedCallback(func(*gui.Frame) {
		if win := frame.Window(); win != nil {
			x, y, w, h := win.Bounds()
			prefs.SetWindowSize(int(w), int(h))
			prefs.SetWindowPos(int(x), int(y))
		}
		// Persist the open file set so the next launch reopens it
		// (Qt Creator-style session restore). Mirrors the window-geometry
		// save right above — both are last-known-good state captured on
		// close and replayed at startup.
		prefs.SetOpenSession(currentSessionPaths(designCanvas))
		core.Quit()
	})

	if win := frame.Window(); win != nil {
		w, h := prefs.WindowSize()
		win.SetSize(float64(w), float64(h))
		// Restore prior position when present, else centre. Negative
		// saved coords (multi-monitor reshuffle) fall back to centre
		// so silkide doesn't open off-screen.
		if x, y := prefs.WindowPos(); x >= 0 && y >= 0 {
			win.SetPos(float64(x), float64(y))
		} else {
			win.MoveToCenter()
		}
	}

	// Reopen whatever was open at last close. Done after the frame is
	// fully built (panels/tabs exist) but before Show() so the restored
	// tabs are visible on first paint rather than popping in afterwards.
	restoreSession(editorTabs, designCanvas)

	frame.Show()
	core.EventLoop()
}

// idTitle composes the window title in JetBrains style: "<project> —
// <active file>". For the demo we just use the working directory as
// the project name.
func idTitle() string {
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = "silkide"
	}
	return filepath.Base(cwd) + " — silkide"
}

// titleSyncTimer holds the polling timer that mirrors the design
// canvas's scene Title() into the frame's window title. Kept at the
// package level so closing the frame can stop it cleanly.
var titleSyncTimer gui.Timer

// startTitleSync wires a 500ms-tick timer that watches
// designCanvas.GedScene().Title() and reflects it into the frame's
// window-bar title. Adds the dirty marker, the project basename, and
// the "silkide" trailing token.
//
// Polling instead of subscribing to scene events because graph's
// SceneItem doesn't currently fire a TitleChanged signal — adding
// one would touch the graph package, which has wider blast radius
// than a 500ms poll for a title string.
func startTitleSync(frame *gui.Frame, canvas *ged.GedView) {
	if frame == nil || canvas == nil {
		return
	}
	last := ""
	titleSyncTimer.Start(500, func() {
		title := composeTitle(canvas)
		if title == last {
			return
		}
		// Frame.SetTitle just stores the value; the live window title
		// goes through Window.SetTitle (which calls glfw.SetTitle).
		// Updating both keeps a future Frame query consistent with the
		// visible chrome.
		frame.SetTitle(title)
		if win := frame.Window(); win != nil {
			win.SetTitle(title)
		}
		last = title
	})
}

// composeTitle builds the window title from the canvas's scene
// state. Pattern matches JetBrains / VS Code: "<file> — <project>"
// with the optional dirty asterisk inside the file slot, matching
// what SceneItem.Title() returns directly. So:
//
//   <project> — silkide                          (untitled, clean)
//   <project> — silkide *                        (untitled, dirty)
//   form.silkui — <project> — silkide            (clean)
//   form.silkui * — <project> — silkide          (dirty)
//
// SceneItem.Title() returns "<title>" when clean and "<title> *"
// when dirty, so composeTitle splits on " *" suffix to detect dirty
// state and re-attaches the marker after the project token. An
// empty title (no file loaded yet) collapses to base + maybe-marker.
func composeTitle(canvas *ged.GedView) string {
	base := idTitle()
	scene := canvas.GedScene()
	if scene == nil {
		return base
	}
	raw := scene.Title()
	dirty := strings.HasSuffix(raw, " *")
	name := raw
	if dirty {
		name = strings.TrimSuffix(raw, " *")
	}
	switch {
	case name == "" && dirty:
		return base + " *"
	case name == "":
		return base
	case dirty:
		return name + " * — " + base
	default:
		return name + " — " + base
	}
}

// buildToolBar adds the icon top toolbar matching the JetBrains-style
// mockup. Each AddAction takes a paint.Icon loaded from the silk icon
// catalog (icon/16x16/*.png). The text label remains as a tooltip-
// style fallback when the icon fails to load — paint.LoadIcon returns
// the "image-missing" red-cross sentinel rather than nil for unknown
// names, so the toolbar always renders SOMETHING.
//
// `editorTabs` and `designCanvas` are captured so Open / Save / Run
// callbacks can route to the right view. Open dispatches by file
// extension: .silkui files load into the design canvas, anything else
// opens in a code editor tab.
func buildToolBar(frame *gui.Frame, editorTabs *gui.TabWidget, designCanvas *ged.GedView) {
	tb := gui.NewToolBar()

	// addIconAction wraps tb.AddAction + gui.SetToolTip so every
	// icon-only button announces what it does on hover. Tooltip text
	// goes through i18n.T() so a Chinese locale shows "保存" instead of
	// "Save", matching the i18n contract for the rest of the IDE
	// chrome.
	addIconAction := func(label, iconName, tipKey string, cb func()) {
		btn := tb.AddAction(label, paint.LoadIcon(iconName), cb)
		if btn != nil && tipKey != "" {
			gui.SetToolTip(btn, i18n.T(tipKey))
		}
	}


	// Hamburger menu — no glyph in the silk icon catalog, so we keep
	// the unicode bars. Click pops a menu with File→New / Open /
	// Save followed by the recent files MRU. Closes the visibility
	// gap on the prefs.RecentFiles list (data was tracked but had
	// no UI to surface it before).
	// Empty initial callback then re-bind: lets us reference the
	// returned *Button from inside the closure (used as the popup
	// anchor point).
	hamburger := tb.AddAction("", paint.LoadIcon("menu"), func() {})
	if hamburger != nil {
		gui.SetToolTip(hamburger, i18n.T("Menu"))
		hamburger.Action().BindFunc0(func() {
			showHamburgerMenu(hamburger, editorTabs, designCanvas)
		})
	}
	tb.AddSeparator()

	// New: clears the design canvas to a fresh scene. Bound to
	// Cmd+N too via registerShortcuts. silk's resource theme has no
	// "plus" PNG; the procedural fallback in paint draws a simple
	// crosshair so the toolbar reads icon-only consistently.
	addIconAction("", "plus", "New", func() {
		newDesignCanvas(designCanvas)
	})

	// Open: route .silkui to the design canvas, everything else to a
	// new editor tab. SaveFileDialog / OpenFileDialog are the only
	// platform-aware bits and silk's gui package wraps each OS's
	// native dialog.
	addIconAction("", "folder", "Open", func() {
		path := gui.OpenFileDialog()
		if path == "" {
			return
		}
		openFromTree(path, editorTabs, designCanvas, nil)
	})
	addIconAction("", "refresh", "Refresh", func() {
		// Force-redraw the active design canvas. Useful when the
		// underlying .silkui has been edited in another editor (the
		// hotreload watcher should pick that up on its own; the
		// button is a manual fallback).
		if designCanvas != nil {
			designCanvas.Update()
		}
	})
	addIconAction("", "save", "Save", func() {
		// Save the current design canvas as .silkui. performSave covers
		// the SaveFileDialog popup (filenameless scenes), the .silk.go
		// regen on success, and the success toast. The dirty-discard
		// flow has its own save path so it doesn't double-toast.
		performSave(designCanvas)
	})
	tb.AddSeparator()

	// Navigation. Mock-up shows back / forward arrows; we re-use the
	// undo / redo glyphs which carry the same left / right semantics
	// in most icon sets.
	addIconAction("", "edit-undo", "Undo", func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanUndo() {
				stack.Undo()
				designCanvas.Update()
			}
		}
	})
	addIconAction("", "edit-redo", "Redo", func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanRedo() {
				stack.Redo()
				designCanvas.Update()
			}
		}
	})
	tb.AddSeparator()

	// Run / Build / Debug / Preview / PropSheet. run.png and
	// preview.png exist natively; build and debug don't have icons
	// yet so they fall back to short text labels. Tooltips include
	// the keyboard shortcut so users can discover F5/F6 from the
	// hover affordance, JetBrains-style.
	if btn := tb.AddAction("", paint.LoadIcon("run"), func() { runProjectInTerminal(designCanvas) }); btn != nil {
		gui.SetToolTip(btn, i18n.T("Run")+" (F5)")
	}
	if btn := tb.AddAction(i18n.T("Build"), nil, func() { buildProject(designCanvas) }); btn != nil {
		gui.SetToolTip(btn, i18n.T("Build")+" (F6)")
	}
	if btn := tb.AddAction(i18n.T("Debug"), nil, func() {}); btn != nil {
		gui.SetToolTip(btn, i18n.T("Debug"))
	}
	tb.AddSeparator()
	// Export (preview-eye icon): pops SaveFileDialog, dispatches by
	// extension to silk/svgexport or silk/pdfexport, draws the active
	// design canvas via scene.DrawAll(painter), writes the resulting
	// document to the chosen path. Restores the missing
	// "designer scene → SVG/PDF" path that cairo_*_surface used to
	// provide before the Cairo removal effort split out export
	// surfaces into pure-Go packages.
	addIconAction("", "preview", "Export...", func() {
		if designCanvas == nil {
			return
		}
		path := gui.SaveFileDialog()
		if path == "" {
			return
		}
		if err := exportDesignCanvas(path, designCanvas); err != nil {
			core.Warn("export failed: ", err)
		}
	})
	addIconAction("", "propsheet", "Settings", func() {
		showProjectSettingsDialog(designCanvas)
	})

	frame.SetToolBar(tb)
}

// exportDesignCanvas renders the design canvas's scene to the file at
// `path`, picking SVG vs PDF by file extension. The unrecognised case
// defaults to SVG since it's the more universal format. Both painter
// implementations satisfy paint.Painter, so scene.DrawAll() drives the
// export the same way it drives the live screen render.
func exportDesignCanvas(path string, designCanvas *ged.GedView) error {
	scene := designCanvas.GedScene()
	if scene == nil {
		return fmt.Errorf("design canvas has no scene")
	}
	_, _, w, h := scene.Bounds()
	if w <= 0 || h <= 0 {
		w, h = 200, 150
	}

	lower := strings.ToLower(path)
	var painter paint.Painter
	var writeOut func(io.Writer) error

	switch {
	case strings.HasSuffix(lower, ".pdf"):
		pp := pdfexport.New(w, h)
		painter = pp
		writeOut = func(w io.Writer) error { _, err := pp.WriteTo(w); return err }
	default:
		// Default to SVG for unknown extensions; rename the path if it
		// has none so the user gets a recognisable file.
		if !strings.HasSuffix(lower, ".svg") {
			path += ".svg"
		}
		sp := svgexport.New(w, h)
		painter = sp
		writeOut = func(w io.Writer) error { _, err := sp.WriteTo(w); return err }
	}

	scene.DrawAll(painter)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeOut(f)
}

// buildPanels installs the central content area:
//
//   - Center dock: editor tabs (code mode) + GedView (design mode)
//     coexist as sibling tabs, so designers flip between editing
//     code and dragging widgets onto the canvas without leaving the
//     window.
//   - Left dock: FileExplorer (project mode) + WidgetList (design
//     mode) — WidgetList is the drag source that pairs with
//     GedView's drag target via the gui.IDndContext protocol, so
//     dropping a Button widget anywhere on the canvas is the same
//     gesture as in the standalone designer.
//   - Bottom dock: Terminal + Output tabs.
//
// The widget palette ↔ design canvas drag-drop is the heart of the
// silk designer. Without it silkide is just a code IDE; with it the
// IDE doubles as the visual form designer.
func buildPanels(frame *gui.Frame) (*gui.TabWidget, *ged.GedView) {
	dock, ok := frame.SuggestDocDock().(*gui.Dock)
	if !ok || dock == nil {
		return nil, nil
	}

	editorTabs := buildEditorTabs(dock)
	dock.AddView(editorTabs)

	// Design canvas — sibling of the editor tabs in the center dock.
	// The user clicks the dock's tab strip to flip between coding and
	// designing. GedView wires its own selection / drop / paste
	// handlers via Init, so just adding the view is enough.
	designCanvas := ged.NewGedView()
	dock.AddView(designCanvas)

	// Welcome screen — third sibling tab. Mirrors Qt Creator's start
	// page: title, recent projects, New / Open buttons. The user
	// clicks the dock's tab strip to flip between welcome and the
	// real workspace; selecting any recent file or pressing New /
	// Open dispatches through the same handlers the toolbar uses.
	if globalPrefs != nil {
		welcome := ged.NewWelcomeScreen()
		welcome.SetRecentFiles(globalPrefs.RecentFiles())
		welcome.SetNewProjectCallback(func() {
			newDesignCanvas(designCanvas)
			dockSetActiveView(dock, designCanvas)
		})
		welcome.SetOpenFileCallback(func() {
			path := gui.OpenFileDialog()
			if path == "" {
				return
			}
			openFromTree(path, editorTabs, designCanvas, dock)
		})
		welcome.SetOpenRecentCallback(func(path string) {
			openFromTree(path, editorTabs, designCanvas, dock)
		})
		dock.AddView(welcome)
		// Make welcome the visible tab on first launch — user sees
		// the recent-files list and the action buttons rather than a
		// blank canvas.
		dockSetActiveView(dock, welcome)
	}

	leftDockI := dock.SplitNewDock(true, false)
	leftDock, _ := leftDockI.(*gui.Dock)

	if leftDock != nil {
		fileExplorer := ged.NewFileExplorer()
		fileExplorer.SetRootDir(".")
		fileExplorer.SigFileOpen(func(path string) {
			openFromTree(path, editorTabs, designCanvas, dock)
		})
		leftDock.AddView(fileExplorer)

		// Widget palette — sibling tab in the same left dock so the
		// user picks a Button / Label / Edit etc. in the same panel
		// they were just browsing files in. Drag-and-drop into the
		// design canvas works because WidgetList implements gui's
		// drag-source protocol and GedView implements the matching
		// drop target (OnDragEnter / OnDragMove / OnDrop).
		widgetList := ged.NewWidgetList()
		leftDock.AddView(widgetList)

		// Global search panel — VSCode-style search-across-files. The
		// panel walks the project root and reports matches via SigOpen,
		// which we route into openFileInEditorAt so a click jumps
		// straight to file:line in the editor tabs (and refocuses the
		// editor tab via the Stack().Page lookup).
		globalSearch = ged.NewGlobalSearchPanel()
		if cwd, err := os.Getwd(); err == nil {
			globalSearch.SetRootDir(cwd)
		}
		globalSearch.SigOpen(func(path string, line int) {
			openFileInEditorAt(editorTabs, path, line, 0)
		})
		leftDock.AddView(globalSearch)
		globalLeftDock = leftDock
	}

	// Right dock: property inspector. Conventional IDE layout (Qt
	// Creator, Visual Studio, IntelliJ) puts the property panel on
	// the right edge so the user's eye flows code/canvas (centre) →
	// properties (right) without crossing the bottom toolchain panel.
	rightDockI := dock.SplitNewDock(false, false)
	if rightDock, ok := rightDockI.(*gui.Dock); ok {
		inspector := ged.NewObjectInspector()
		inspector.SetScene(designCanvas.GedScene())
		// Expose at the package level so the File→New action
		// (registerShortcuts and the toolbar "+" button) can retarget
		// the inspector at the freshly-created scene without threading
		// a third return value out of buildPanels.
		globalInspector = inspector
		rightDock.AddView(inspector)

		// Trigger an inspector rebuild whenever the design canvas's
		// selection changes. Without this the inspector stays stuck
		// on whatever was selected when SetScene fired.
		designCanvas.AddSelectionCallback(func(items []graph.IItem) {
			inspector.Rebuild()
		})

		// Code outline — sibling tab of the Object Inspector in the
		// right dock. The panel existed and was factory-registered but
		// was never added to a dock, so the symbol tree was invisible.
		// It mirrors the active editor: SetEditor re-parses the current
		// file's functions / types / vars, and clicking a symbol scrolls
		// that editor to the declaration (the same file:line jump the
		// BuildOutput pane uses). syncOutlineToActiveEditor keeps it in
		// step with the editor tabs.
		outline := ged.NewCodeOutlinePanel()
		outline.SetNavigateCallback(func(line int) {
			if ed := activeEditor(editorTabs); ed != nil {
				ed.ScrollToLine(line)
			}
		})
		globalOutline = outline
		rightDock.AddView(outline)

		// Stash the right dock so the Cmd+Shift+O shortcut can flip its
		// active tab to the outline without threading the dock reference
		// through the shortcut wiring (mirrors globalLeftDock).
		globalRightDock = rightDock

		// Cross-file bookmarks — sibling tab of Inspector + Outline in
		// the right dock. The right dock is the natural home for nav
		// surfaces (the outline already lives here), and putting the
		// bookmarks pane next door means "find a symbol" and "jump to a
		// bookmark" sit side by side. SigActivated routes to the same
		// openFileInEditorAt path BuildOutput uses, so a bookmark click
		// behaves identically to a build-error click.
		bookmarks := ged.NewBookmarksPanel()
		bookmarks.SigActivated(func(file string, line int) {
			openFileInEditorAt(editorTabs, file, line, 0)
		})
		globalBookmarks = bookmarks
		rightDock.AddView(bookmarks)

		// Seed the outline from whichever editor tab is active now, then
		// re-bind it every time the user switches tabs. The panel
		// self-refreshes on content change via its own Draw-time hash
		// check, so binding the editor once per tab switch is enough —
		// no per-keystroke hook needed.
		syncOutlineToActiveEditor(editorTabs)
		editorTabs.SetCurrentChangedCallback(func(_ interface{}, _ int) {
			syncOutlineToActiveEditor(editorTabs)
		})
	}

	// Bottom dock: terminal + build output + problems. Toolchain stuff
	// a developer glances at without leaving their main work. Stash
	// the dock at package level so Run / Build can flip the active
	// tab when their respective panes start receiving output —
	// users shouldn't have to manually click the right tab to see
	// what just happened.
	bottomDockI := dock.SplitNewDock(false, true)
	if bottomDock, ok := bottomDockI.(*gui.Dock); ok {
		bottomDock.AddView(buildTerminalPane())
		bottomDock.AddView(buildOutputPane())

		// Structured Problems pane — sibling of BuildOutput. Where
		// BuildOutput is the raw log, this is the parsed list (sortable,
		// per-row severity glyph, file:line locator). Same compile feeds
		// both via reportBuildOutput. Click a row to jump to the error,
		// mirroring BuildOutput's SigErrorClick wiring.
		problems := ged.NewProblemsPanel()
		problems.SigProblemActivated(func(file string, line, col int) {
			if !filepath.IsAbs(file) {
				if dir := projectDir(designCanvas); dir != "" {
					file = filepath.Join(dir, file)
				}
			}
			openFileInEditorAt(editorTabs, file, line, col)
		})
		globalProblems = problems
		bottomDock.AddView(problems)

		// Structured `go test -v` view — sibling of BuildOutput.
		// runProjectTests pipes its captured stdout/stderr through
		// SetOutput, and a failing row's File/Line locator routes to
		// openFileInEditorAt (same jump path the BuildOutput error
		// click uses). The locator from the test parser is the bare
		// "file_test.go" basename `t.Errorf` prints, so resolve it
		// against projectDir like the Problems panel does.
		testResults := ged.NewTestResultsPanel()
		testResults.SigResultActivated(func(r ged.TestResult) {
			if r.File == "" {
				return
			}
			file := r.File
			if !filepath.IsAbs(file) {
				if dir := projectDir(designCanvas); dir != "" {
					file = filepath.Join(dir, file)
				}
			}
			openFileInEditorAt(editorTabs, file, r.Line, 0)
		})
		// Right-click "运行此测试" path: the panel signals a test name, we
		// translate that into `go test -run ^<name>$ -v ./...` in projectDir
		// and feed BuildOutput + the same panel via runSingleTest. Mirrors
		// the existing F7/Run Tests wiring so the two paths stay consistent.
		testResults.SigRunTestRequested(func(name string) {
			runSingleTest(designCanvas, name)
		})
		globalTestResults = testResults
		bottomDock.AddView(testResults)

		globalBottomDock = bottomDock
	}

	// Wire build-error click navigation: when the user clicks a
	// "file:line:col: ..." row in the BuildOutput pane, open that
	// file in the editor tabs and scroll to the line. Resolves
	// relative paths against projectDir(designCanvas) so the
	// click-target matches what go build emitted regardless of
	// silkide's own cwd.
	if globalBuildOutput != nil {
		globalBuildOutput.SigErrorClick(func(file string, line, col int) {
			if !filepath.IsAbs(file) {
				if dir := projectDir(designCanvas); dir != "" {
					file = filepath.Join(dir, file)
				}
			}
			openFileInEditorAt(editorTabs, file, line, col)
		})
	}

	return editorTabs, designCanvas
}

// showProjectSettingsDialog pops a modal dialog wrapping
// ged.ProjectSettingsPanel — the panel reads go.mod from the
// project directory and displays module / Go version / build tags
// / output directory rows. Standalone widget exists in ged; silkide
// surfaces it here on the Settings toolbar click.
//
// Sized to a JetBrains-style 560x420 dialog so the read-only rows
// + 2 editable rows have room without scrolling.
func showProjectSettingsDialog(parent gui.IWidget) {
	if parent == nil {
		return
	}
	dlg := gui.NewDialog(i18n.T("Project Settings"), parent)
	panel := ged.NewProjectSettingsPanel()
	panel.Refresh()
	box := gui.NewVBox()
	box.SetSpacing(0)
	box.AddWidget(panel)
	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Close"), gui.DialogOK)
	dlg.SetSize(560, 420)
	dlg.ShowModal()
}

// dockSetActiveView flips a Dock to show `view`, which must be one
// of the views previously AddView'd. Wraps the IndexOfView →
// SetActiveIndex round-trip so callers don't have to bounce through
// the type-asserted index lookup.
func dockSetActiveView(d *gui.Dock, view gui.IWidget) {
	if d == nil || view == nil {
		return
	}
	if idx := d.IndexOfView(view); idx >= 0 {
		d.SetActiveIndex(idx)
	}
}

// buildEditorTabs composes the multi-tab code editor view. Each tab
// holds a CodeEditor; closing tabs from the X button cycles to the
// next tab automatically (TabWidget native behaviour).
//
// Pre-seeded with three sample tabs so the layout matches the
// mockup the moment the binary starts. Real callers replace this
// with FileExplorer.SigFileOpen → tab open wiring (see
// openFileInEditor below).
func buildEditorTabs(centerDock *gui.Dock) *gui.TabWidget {
	tabs := gui.NewTabWidget()
	tabs.AddTab(makeCodeEditor(sampleMainGo()), "main.go", nil)
	tabs.AddTab(makeCodeEditor(sampleServerGo()), "server.go", nil)
	tabs.AddTab(makeCodeEditor(sampleGoMod()), "go.mod", nil)
	return tabs
}

// makeCodeEditor seeds a CodeEditor with the given text. The editor
// already handles Go-flavoured syntax highlighting via the standard
// silk theme.
func makeCodeEditor(text string) *gui.CodeEditor {
	ed := gui.NewCodeEditor()
	ed.SetText(text)
	return ed
}

// buildTerminalPane returns a live integrated terminal panel.
// ged.TerminalPanel runs one shell command at a time in the project
// directory, streaming stdout / stderr back into the scrollback —
// the user can run go build, git, etc. without leaving silkide.
// Held in a package global so future code (e.g. a "Run" toolbar
// button) can dispatch commands into the same scrollback the user
// sees.
var globalTerminal *ged.TerminalPanel

// globalBottomDock holds the dock containing the terminal +
// build-output panes. Used by runProjectInTerminal / buildProject /
// reportBuildOutput to focus the relevant tab when a long-running
// action's output starts arriving.
var globalBottomDock *gui.Dock

// globalAutoSaver writes `.autosave` companions next to the active
// scene's filename every 60 seconds while the UndoStack is dirty.
// Initial bind happens in main() right after the design canvas is
// created; rebindAutoSaver swaps it onto the new scene whenever
// File→New or an Open replaces the current scene.
var globalAutoSaver *ged.AutoSaver

// globalSearch is the VSCode-style search-across-files panel that
// lives in the left dock. The Cmd+Shift+F shortcut focuses it.
var globalSearch *ged.GlobalSearchPanel

// globalLeftDock holds the dock containing FileExplorer / WidgetList
// / GlobalSearchPanel. Used by the Cmd+Shift+F shortcut to flip the
// active tab to globalSearch without threading the dock reference
// through every shortcut handler.
var globalLeftDock *gui.Dock

func buildTerminalPane() gui.IWidget {
	if globalTerminal == nil {
		globalTerminal = ged.NewTerminalPanel()
	}
	return globalTerminal
}

// buildOutputPane returns the build-output panel that ged ships.
// SetOutput parses Go compiler errors and exposes a per-error
// click callback the IDE can wire to "jump to file:line:col".
// Held globally so a future "Build" action can route compiler
// stdout through SetOutput without threading the reference back
// up the call stack.
var globalBuildOutput *ged.BuildOutput

func buildOutputPane() gui.IWidget {
	if globalBuildOutput == nil {
		globalBuildOutput = ged.NewBuildOutput()
	}
	return globalBuildOutput
}

// buildStatusBar populates the bottom status strip with project /
// branch / cursor / encoding / runtime / version cells. StatusBar
// uses AddPermanentWidget for the right-aligned cells that show
// project metadata; SetMessage drives the transient left-aligned
// message slot which we leave blank initially.
// statusBarZoomLabel is the package-level reference to the status bar's
// "100%" zoom-percent label so the canvas zoom shortcuts can update it
// without threading another argument through the shortcut wiring.
var statusBarZoomLabel *gui.Label

// statusBarBuildLabel shows the latest build's outcome — "build ok"
// for clean compiles, "build: N errors" otherwise. Empty until the
// first buildProject call.
var statusBarBuildLabel *gui.Label

func buildStatusBar(frame *gui.Frame) *gui.StatusBar {
	sb := gui.NewStatusBar()

	cwd, _ := os.Getwd()
	project := filepath.Base(cwd)
	if project == "" {
		project = "silkide"
	}
	// Module path lives next to the project basename so the user can
	// tell "silkide rooted at ./examples/foo" apart from "rooted at
	// the real silk module" at a glance. Empty when no go.mod is
	// reachable from the cwd; we leave the cell off entirely in that
	// case so designer-only projects stay uncluttered.
	//
	// go.work detection runs in parallel — when one is reachable from
	// the cwd, append " · workspace(N)" so multi-module setups are
	// glanceably distinguishable from a plain single-module project.
	// LoadGoWork errors are non-fatal (e.g. malformed go.work returns
	// the partial parse alongside an error); we still take the Uses
	// count it managed to recover.
	modulePath := loadModulePath(cwd)
	var work *core.GoWork
	if _, ok := core.FindGoWork(cwd); ok {
		gw, _ := core.LoadGoWork(cwd)
		work = gw
	}
	sb.AddPermanentWidget(gui.NewLabel(formatProjectStatus(project, modulePath, work)))
	sb.AddPermanentWidget(gui.NewLabel("main"))
	sb.AddPermanentWidget(gui.NewLabel("Ln 1, Col 1"))
	sb.AddPermanentWidget(gui.NewLabel("UTF-8"))
	sb.AddPermanentWidget(gui.NewLabel("Go 1.25"))

	// Canvas zoom percentage cell. Updated by setZoomLabel whenever
	// Cmd+= / Cmd+- / Cmd+0 fires.
	statusBarZoomLabel = gui.NewLabel("100%")
	sb.AddPermanentWidget(statusBarZoomLabel)

	// Build status cell. Empty before the first build runs; after a
	// buildProject completes setBuildStatus updates it to "build ok"
	// (no errors) or "build: N errors" — the user gets a glanceable
	// summary even when the BuildOutput pane is hidden behind the
	// Terminal tab.
	statusBarBuildLabel = gui.NewLabel("")
	sb.AddPermanentWidget(statusBarBuildLabel)

	sb.AddPermanentWidget(gui.NewLabel("v0.1.3"))

	frame.SetStatusBar(sb)
	return sb
}

// setBuildStatus refreshes the status-bar build cell from the latest
// BuildOutput pane content. Called after reportBuildOutput so the
// cell reflects whatever just landed. Counts ErrorMap entries —
// non-error info lines (silkgen messages, "$ go build" headers)
// don't bump the count.
func setBuildStatus() {
	if statusBarBuildLabel == nil || globalBuildOutput == nil {
		return
	}
	n := len(globalBuildOutput.ErrorMap())
	switch {
	case n == 0 && globalBuildOutput.HasErrors():
		statusBarBuildLabel.SetText(i18n.T("build: error"))
	case n == 0:
		statusBarBuildLabel.SetText(i18n.T("build ok"))
	default:
		statusBarBuildLabel.SetText(fmt.Sprintf(i18n.T("build: %d errors"), n))
	}
}

// setZoomLabel formats `zoom` as a percentage and pushes it into the
// status-bar zoom cell. Called by zoomCanvas / zoomCanvasTo after
// SetZoomFactor.
func setZoomLabel(zoom float64) {
	if statusBarZoomLabel == nil {
		return
	}
	statusBarZoomLabel.SetText(fmt.Sprintf("%.0f%%", zoom*100))
}

// itemDisplayName returns a human-friendly identifier for a scene
// item. Prefers the item's Title() when present (matches the IDE's
// inspector tree column); falls back to the item's Go-type name so
// even un-named freshly-dropped widgets get a reasonable label.
func itemDisplayName(item graph.IItem) string {
	if item == nil {
		return "(nil)"
	}
	if t, ok := item.(interface{ Title() string }); ok {
		if name := t.Title(); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%T", item)
}

// itemBoundsLabel formats a "(x, y) w×h" description of an item's
// current bounds for the status-bar selection cell. Click-time
// snapshot only — the cell goes stale once the user starts dragging
// the item; that's by design (keeping it live across drags would
// need a per-Item move signal that doesn't currently exist on
// graph.IItem).
func itemBoundsLabel(item graph.IItem) string {
	if item == nil {
		return ""
	}
	x, y := item.Pos()
	w, h := item.Size()
	return fmt.Sprintf("(%.0f, %.0f) %.0f×%.0f", x, y, w, h)
}

// openFromTree dispatches a FileExplorer click. .silkui files load
// straight into the design canvas (closing the declarative loop —
// designer-authored layouts open in the designer); everything else
// goes into a fresh code-editor tab as plain text.
//
// Switching the active dock view is intentional: when the user opens
// a .silkui we want them looking at the design canvas, not at the
// code editor that was visible before.
//
// Every successful open also touches preferences.AddRecentFile so the
// MRU list survives across launches. The package-level globalPrefs
// reference is set in main() right after newPreferences().
func openFromTree(path string, tabs *gui.TabWidget, canvas *ged.GedView, centerDock *gui.Dock) {
	if filepath.Ext(path) == ".silkui" {
		if canvas == nil {
			return
		}
		// Replacing the scene wipes the current design, so guard the
		// open against losing unsaved work the same way newDesignCanvas
		// does. confirmDiscardDirty no-ops on a clean scene.
		if !confirmDiscardDirty(canvas) {
			return
		}
		// Recovery prompt: if a sibling .autosave is newer than the
		// .silkui the user is trying to open, offer to load it
		// instead. Picks up where a crash or sudden quit left off.
		recovered := false
		if recovery := ged.CheckRecovery(path); recovery != "" {
			if confirmRecoverFromAutosave(canvas, path, recovery) {
				path = recovery
				recovered = true
			}
		}
		if err := canvas.GedScene().OpenFile(path); err == nil {
			rebindAutoSaver(canvas.GedScene())
			recordRecentFile(path)
			watchForReload(canvas, path)
			if centerDock != nil {
				// Bring the design canvas to the front so the user sees
				// the loaded scene immediately.
				if idx := centerDock.IndexOfView(canvas); idx >= 0 {
					centerDock.SetActiveIndex(idx)
				}
			}
			if recovered {
				silkideToast(i18n.T("Recovered from autosave"), gui.ToastSuccess)
			} else {
				silkideToast(i18n.Tf("Opened %s", filepath.Base(path)), gui.ToastInfo)
			}
		}
		return
	}
	if openFileInEditor(tabs, path) {
		recordRecentFile(path)
	}
}

// globalPrefs is the package-level preferences instance set up by
// main(). openFromTree's recordRecentFile reaches it without
// threading another argument through every call site.
var globalPrefs *preferences

// globalReloader watches every .silkui file silkide opens for
// external edits and re-applies them to the design canvas. Lazily
// constructed when the first .silkui opens — silkide instances that
// only edit code never spin up the watcher.
var globalReloader *hotreload.Reloader

// globalInspector is the package-level reference to the right-dock
// Object Inspector. The "New" toolbar action and Cmd+N shortcut both
// need to retarget the inspector at the fresh scene after a
// SetScene, and the buildPanels closure that originally captured it
// has gone out of scope by the time those callbacks fire.
var globalInspector *ged.ObjectInspector

// globalOutline is the right-dock code-outline panel — a symbol tree
// for the active editor. The Cmd+Shift+O shortcut focuses it, and the
// editor-tabs current-changed callback re-binds it to the active
// editor via syncOutlineToActiveEditor.
var globalOutline *ged.CodeOutlinePanel

// globalRightDock holds the dock containing the Object Inspector and
// the code outline. Used by the Cmd+Shift+O shortcut to bring the
// outline tab to the front (mirrors globalLeftDock for global search).
var globalRightDock *gui.Dock

// globalProblems is the bottom-dock structured compiler-issues pane.
// Sibling tab of Terminal + BuildOutput. reportBuildOutput feeds it
// the same text that lands in BuildOutput so a single compile populates
// both panes. The "Show Problems" palette command focuses this tab.
var globalProblems *ged.ProblemsPanel

// globalBookmarks is the right-dock cross-file bookmark list. Sibling
// tab of the Object Inspector + Outline. addBookmarkAtCursor pushes
// entries in; clicking a row jumps the editor to (file, line) via the
// same openFileInEditorAt path BuildOutput uses for clickable errors.
var globalBookmarks *ged.BookmarksPanel

// globalTestResults is the bottom-dock structured `go test -v` view.
// Sibling tab of Terminal + BuildOutput + Problems. runProjectTests
// feeds it the same captured output that lands in BuildOutput; on any
// failure the panel is brought to the front (BuildOutput keeps the raw
// log, this pane shows pass/fail rows you can click to jump to source).
var globalTestResults *ged.TestResultsPanel

// coverageTempFile is the path of the cover profile written by the most
// recent runProjectWithCoverage invocation. Kept at the package level
// so the next coverage run can delete the previous one — keeps a single
// cover.out in os.TempDir rather than growing a pile.
var coverageTempFile string

// showHamburgerMenu pops the silkide application menu next to the
// hamburger toolbar button. Hosts the four standard file actions
// (New / Open / Save / Save As-via-Open) plus a separator and the
// recent-files MRU. Built fresh on every click so the recent list
// reflects whatever the user just opened.
func showHamburgerMenu(anchor *gui.Button, editorTabs *gui.TabWidget, designCanvas *ged.GedView) {
	if anchor == nil {
		return
	}
	menu := gui.NewPopupMenu()

	// File actions — same handlers as the toolbar / Cmd shortcuts
	// so all three entry points end up in the same code path.
	menu.AddButton1(i18n.T("New"), nil).Action().BindFunc0(func() {
		newDesignCanvas(designCanvas)
	})
	menu.AddButton1(i18n.T("Open"), nil).Action().BindFunc0(func() {
		path := gui.OpenFileDialog()
		if path == "" {
			return
		}
		openFromTree(path, editorTabs, designCanvas, nil)
	})

	// "Open Recent" submenu — Qt Creator-style MRU surfaced as a
	// nested menu so the top level stays short. Each entry opens that
	// file through the same openFromTree path the toolbar / tree use;
	// a disabled "(empty)" placeholder shows when nothing's been
	// opened yet so the submenu never dangles with no rows.
	recentSub := gui.NewPopupMenu()
	var recent []string
	if globalPrefs != nil {
		recent = globalPrefs.RecentFiles()
	}
	if len(recent) == 0 {
		empty := recentSub.AddButton1(i18n.T("(empty)"), nil)
		empty.SetEnabled(false)
	} else {
		for _, path := range recent {
			p := path // capture per-iteration
			btn := recentSub.AddButton1(filepath.Base(p), nil)
			gui.SetToolTip(btn, p)
			btn.Action().BindFunc0(func() {
				openFromTree(p, editorTabs, designCanvas, nil)
			})
		}
	}
	menu.AddSubMenu(i18n.T("Open Recent"), nil, recentSub)

	menu.AddButton1(i18n.T("Save"), nil).Action().BindFunc0(func() {
		// Same Save action as the toolbar / Cmd+S — performSave keeps
		// the regen + success toast consistent across entry points.
		performSave(designCanvas)
	})

	// "Dump a11y tree" — surfaces the cherry-picked silk/a11y package
	// inside the IDE. Useful for verifying that custom widgets expose
	// sane Roles to screen readers, or for snapshotting the visual
	// hierarchy in a bug report. Output goes to stderr.
	menu.AddSeparator()
	menu.AddButton1(i18n.T("Dump A11y Tree"), nil).Action().BindFunc0(func() {
		dumpA11yTree()
	})
	menu.AddButton1(i18n.T("Project Settings"), nil).Action().BindFunc0(func() {
		showProjectSettingsDialog(designCanvas)
	})

	// "About" entry — shows the version banner + runtime info via
	// ged.ShowAboutDialog. Standard "main app menu → About" surface
	// every IDE has.
	menu.AddSeparator()
	menu.AddButton1(i18n.T("About"), nil).Action().BindFunc0(func() {
		ged.ShowAboutDialog(designCanvas)
	})

	xg, yg := anchor.MapToGlobal(0, 0)
	_, h := anchor.Size()
	menu.ShowAsPopup(xg, yg+h, true)
}

// dumpA11yTree renders the active frame's accessibility hierarchy
// to stderr. Wired to both the hamburger "Dump A11y Tree" menu
// item and the Cmd+Shift+A shortcut so the same code path serves
// menu users and keyboard users.
func dumpA11yTree() {
	root := gui.DefaultFrame()
	if root == nil {
		fmt.Fprintln(os.Stderr, "a11y: no DefaultFrame")
		return
	}
	tree := a11y.Walk(root)
	if tree == nil {
		fmt.Fprintln(os.Stderr, "a11y: nil tree (root not visible)")
		return
	}
	fmt.Fprintln(os.Stderr, "a11y tree:")
	dumpA11yNode(tree, 0)
}

// dumpA11yNode renders an a11y.Node as an indented tree on stderr.
// Each line: "<indent><Role> <Name>" — bounds and state are skipped
// to keep the dump readable; callers needing every field should walk
// the tree directly.
func dumpA11yNode(n *a11y.Node, depth int) {
	if n == nil {
		return
	}
	indent := strings.Repeat("  ", depth)
	name := n.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Fprintf(os.Stderr, "%s%s %s\n", indent, n.Role, name)
	for _, c := range n.Children {
		dumpA11yNode(c, depth+1)
	}
}

// regenerateGoForSilkui writes a .silk.go file alongside the .silkui
// at `silkuiPath`, mirroring what cmd/silkgen does. Called every time
// silkide saves a .silkui so the user has a compilable Go snippet
// next to the designer file without leaving the IDE.
//
// Errors are silently swallowed (logged via core.Warn): silkide is a
// designer first; missing the codegen step shouldn't block the
// designer-side save. Re-running on the same input is byte-stable
// (subject to gofmt) so go:generate-style flows can call this
// repeatedly without churn.
func regenerateGoForSilkui(silkuiPath string) {
	if silkuiPath == "" || filepath.Ext(silkuiPath) != ".silkui" {
		return
	}
	doc, err := core.LoadTDocFile(silkuiPath)
	if err != nil {
		reportBuildOutput(fmt.Sprintf("%s:1:0: silkgen load failed: %v", silkuiPath, err))
		core.Warn("silkgen: load ", silkuiPath, ": ", err)
		return
	}
	tree, err := decl.FromTDoc(doc)
	if err != nil {
		reportBuildOutput(fmt.Sprintf("%s:1:0: silkgen parse failed: %v", silkuiPath, err))
		core.Warn("silkgen: parse ", silkuiPath, ": ", err)
		return
	}
	body := decl.ToGo(tree)
	stem := strings.TrimSuffix(filepath.Base(silkuiPath), ".silkui")
	funcName := "Build" + capitalise(stem)
	src := fmt.Sprintf(`// Code generated by silkide — DO NOT EDIT.
// Source: %s

package ui

import "silk/decl"

// %s constructs the widget tree decoded from %s. Pair with
// (*decl.Node).Build() at runtime to materialise the actual widgets.
func %s() *decl.Node {
	return %s
}
`, filepath.Base(silkuiPath), funcName, filepath.Base(silkuiPath), funcName, body)

	outPath := filepath.Join(filepath.Dir(silkuiPath), stem+".silk.go")
	if err := os.WriteFile(outPath, []byte(src), 0o644); err != nil {
		reportBuildOutput(fmt.Sprintf("%s:1:0: silkgen write failed: %v", outPath, err))
		core.Warn("silkgen: write ", outPath, ": ", err)
		return
	}
	reportBuildOutput(fmt.Sprintf("silkgen: wrote %s", outPath))
}

// reportBuildOutput pushes one line into the BuildOutput pane if the
// pane is wired up. Lines that match Go's "file:line:col: …" format
// become clickable in the pane; informational lines just show as
// plain rows. Safe to call before buildOutputPane has been built —
// the function noops in that case.
//
// Side effect: brings the BuildOutput tab to the front of the
// bottom dock if a dock has been recorded. Build / silkgen output
// is the kind of thing the user expects to see immediately, not
// "buried under whatever tab was last visible".
func reportBuildOutput(line string) {
	if globalBuildOutput == nil {
		return
	}
	globalBuildOutput.SetOutput(line)
	// Also feed the structured Problems pane the same text. SetOutput
	// re-parses, so a clean "build ok" line resets the row list to
	// empty — the Problems tally stays consistent with what BuildOutput
	// is showing. Mirroring the BuildOutput auto-focus on the Problems
	// tab would fight BuildOutput for the active slot on every compile;
	// BuildOutput already lands in front, so leave the Problems tab as
	// a manual click / palette command.
	if globalProblems != nil {
		globalProblems.SetOutput(line)
	}
	dockSetActiveView(globalBottomDock, globalBuildOutput)
	setBuildStatus()
}

// runProjectInTerminal dispatches "go run ." through the integrated
// terminal panel. The terminal's worker spawns the Go toolchain in
// the cwd of the panel — we point it at the directory of the active
// .silkui first so a multi-project workspace runs the right module.
//
// No-op when there's no terminal yet (terminal pane built lazily
// on first focus) or no design canvas. Filenameless scenes fall
// back to the terminal's existing cwd, which is the silkide
// process's working directory.
func runProjectInTerminal(canvas *ged.GedView) {
	if globalTerminal == nil {
		// Force-build the terminal pane so the user sees output.
		buildTerminalPane()
	}
	// Auto-detected project dir is the fallback; the persisted
	// RunWorkingDir overrides it when set (host-override rule). Empty
	// override yields the historical projectDir behaviour, so legacy
	// configs that never touched the field keep working.
	autoDir := projectDir(canvas)
	cwd := autoDir
	if globalPrefs != nil {
		cwd = effectiveRunDir(strings.TrimSpace(globalPrefs.RunWorkingDir()), autoDir)
	}
	if cwd != "" {
		globalTerminal.SetCwd(cwd)
	}
	dockSetActiveView(globalBottomDock, globalTerminal)
	// Pre-flight: a designer-only project that hasn't grown a
	// main.go yet would crash "go run ." with the cryptic
	// "package . is not a main package" error. Detect that case up
	// front and surface a friendly explanation in the terminal
	// instead — points the user at the .silk.go silkgen produced
	// and at silkide's File→New (Cmd+N).
	if cwd != "" && !hasMainPackage(cwd) {
		// Hint instead of "echo …": platform-neutral, no subprocess,
		// no shell-quoting trap (cmd.exe doesn't understand POSIX
		// single quotes).
		globalTerminal.Hint(i18n.T(
			"silkide: no main package found — add a main.go that imports the generated .silk.go to make this directory runnable."))
		silkideToast(i18n.T("Run skipped: no main package"), gui.ToastWarning)
		return
	}
	// Append the persisted Run Configuration args, if any. The terminal
	// panel hands the string to its underlying shell verbatim, so the
	// shell tokenises it the same way it would a typed line — single
	// strings.TrimSpace keeps a trailing space from emitting an empty
	// argv slot. splitRunArgs is the unit-testable parser; here we re-
	// join the parsed tokens so the terminal still sees one command
	// line rather than argv pieces.
	cmdline := "go run ."
	if globalPrefs != nil {
		if args := strings.TrimSpace(globalPrefs.RunArgs()); args != "" {
			cmdline += " " + args
		}
		// Environment variables: TerminalPanel.Run takes only a single
		// cmd string and its worker hard-codes c.Env = os.Environ(), so
		// we can't pass the env list through the existing public API.
		// Surface it as a visible Hint + toast so the user can confirm
		// the prefs are loaded; tightening this to real env-injection
		// is a follow-up that needs a TerminalPanel.RunEnv overload.
		if env := globalPrefs.RunEnv(); len(env) > 0 {
			globalTerminal.Hint(i18n.Tf("silkide: %d env entries configured (not yet forwarded to go run)", len(env)))
		}
	}
	globalTerminal.Run(cmdline)
	silkideToast(i18n.T("Running..."), gui.ToastInfo)
}

// splitRunArgs parses a user-supplied run-args string into argv tokens
// using shell-style splitting: whitespace separates, single and double
// quotes group, backslash escapes the next byte. Round-trips through
// the same scanner go's flag package uses when the OS shell passes argv
// in, so `-port 8080 -msg "hello world"` becomes
// ["-port", "8080", "-msg", "hello world"].
//
// Empty input returns nil (not an empty slice) so a missing
// RunArgs preference is indistinguishable from an explicitly-empty one
// at the call site. Unterminated quotes flush whatever was collected so
// far rather than dropping the partial token — the spawn either fails
// loudly with a real error or runs with the recovered argv, both of
// which beat silently swallowing the user's input.
func splitRunArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var (
		out     []string
		cur     strings.Builder
		quote   byte // 0, '\'', or '"'
		escaped bool
	)
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, cur.String())
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			cur.WriteByte(c)
			escaped = false
		case c == '\\' && quote != '\'':
			// Inside single quotes the backslash is literal (POSIX
			// rules); everywhere else it escapes the next byte.
			escaped = true
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// renameSymbolAtActiveEditor pops a rename-input dialog seeded with the
// identifier under the active editor's cursor and applies the new name
// across the file via the editor's RenameSymbolAtCursor method. The
// rename method lives in gui.CodeEditor (added in a parallel agent's
// work on gui/codeeditor.go). We address it through a runtime interface
// assertion so silkide still builds before that change merges; on a
// build where the editor doesn't satisfy the interface yet the dialog
// pops, the user types, submit, and gets a "not available" toast — no
// crash, no silent swallow.
func renameSymbolAtActiveEditor(tabs *gui.TabWidget) {
	ed := activeEditor(tabs)
	if ed == nil {
		return
	}
	// Prefill: silk's CodeEditor doesn't currently expose a public
	// WordAtCursor (the internal wordBoundsAt is unexported), so the
	// dialog comes up with an empty default. When the parallel rename
	// work lands a public accessor we can swap it in here.
	prefill := ""
	parent := gui.IWidget(globalFrame)
	if parent == nil {
		parent = gui.DefaultFrame()
	}
	newName, ok := gui.ShowInputBox(parent, nil,
		i18n.T("Rename Symbol"), i18n.T("New name:"), prefill)
	if !ok {
		return
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return
	}
	// Runtime interface check: the parallel CodeEditor work adds
	// RenameSymbolAtCursor(string) (string, int, error). Until that
	// merges, the assertion fails and we toast a placeholder so the
	// user knows the wiring is in place and the missing piece is in
	// the editor, not the IDE.
	renamer, ok := interface{}(ed).(interface {
		RenameSymbolAtCursor(newName string) (oldName string, count int, err error)
	})
	if !ok {
		silkideToast(i18n.T("Rename Symbol not available"), gui.ToastWarning)
		return
	}
	oldName, count, err := renamer.RenameSymbolAtCursor(newName)
	if err != nil {
		silkideToast(i18n.Tf("Rename failed: %v", err), gui.ToastError)
		return
	}
	silkideToast(i18n.Tf("Renamed %s → %s (%d occurrences)", oldName, newName, count), gui.ToastSuccess)
}

// configureRun pops a structured "Run Configuration" modal — a Dialog
// hosting a fresh ged.RunConfigPanel — letting the user edit args,
// working directory, and environment variables in one surface. Used by
// the "Configure Run..." palette command. The panel emits SigChanged
// on every in-panel edit; we shadow the value in a local mutable
// RunConfig and commit all three prefs on OK (Cancel discards). The
// saved values feed runProjectInTerminal on the next F5 / Run.
func configureRun() {
	if globalPrefs == nil {
		return
	}
	parent := gui.IWidget(globalFrame)
	if parent == nil {
		parent = gui.DefaultFrame()
	}
	dlg := gui.NewDialog(i18n.T("Run Configuration"), parent)
	panel := ged.NewRunConfigPanel()
	initial := ged.RunConfig{
		Args:       globalPrefs.RunArgs(),
		WorkingDir: globalPrefs.RunWorkingDir(),
		Env:        globalPrefs.RunEnv(),
	}
	panel.SetConfig(initial)
	// Mirror in-panel edits into a local working copy. The dialog model
	// is decoupled from prefs so Cancel discards cleanly — only OK
	// commits.
	pending := initial
	panel.SigChanged(func(cfg ged.RunConfig) {
		pending = cfg
	})

	box := gui.NewVBox()
	box.SetSpacing(0)
	box.AddWidget(panel)
	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	dlg.AddButton(i18n.T("OK"), gui.DialogOK)
	dlg.SetSize(560, 420)
	if dlg.ShowModal() != gui.DialogOK {
		return
	}
	globalPrefs.SetRunArgs(strings.TrimSpace(pending.Args))
	globalPrefs.SetRunWorkingDir(strings.TrimSpace(pending.WorkingDir))
	globalPrefs.SetRunEnv(pending.Env)
	silkideToast(i18n.T("Run config saved"), gui.ToastSuccess)
}

// hasMainPackage scans `dir` for any .go file whose first non-blank
// non-comment line is `package main`. Cheap pre-flight before
// dispatching `go run .`. Returns false when the directory has no
// .go files at all (designer-only project) or only library files.
func hasMainPackage(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Conservative: when we can't tell, let go run . handle the
		// real check. Better one confusing error than a false
		// negative that blocks valid projects.
		return true
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if firstPackageLineIsMain(string(data)) {
			return true
		}
	}
	return false
}

// firstPackageLineIsMain returns true if the first non-blank,
// non-comment line of `src` is "package main". Stops scanning at
// the first `package …` directive — comments and blank lines above
// don't count as code.
func firstPackageLineIsMain(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "//") {
			continue
		}
		return strings.HasPrefix(trim, "package main")
	}
	return false
}


// buildProject runs "go build ./..." in the project directory and
// pushes combined stdout+stderr into the BuildOutput pane. Build
// errors come through in Go's standard "file:line:col: msg" format
// which BuildOutput already parses for click-to-jump navigation.
//
// Spawning happens on a goroutine so the IDE stays responsive while
// the toolchain works; the result lands in the pane via
// reportBuildOutput on the main thread (BuildOutput's SetOutput is
// idempotent and replaces the prior content, so the pane shows the
// latest run's full result).
func buildProject(canvas *ged.GedView) {
	if globalBuildOutput == nil {
		buildOutputPane()
	}
	dir := projectDir(canvas)
	reportBuildOutput(fmt.Sprintf("$ go build ./...   (cwd: %s)", dir))
	go func() {
		cmd := exec.Command("go", "build", "./...")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil && text == "" {
			text = err.Error()
		} else if err == nil {
			text += "\nbuild ok"
		}
		reportBuildOutput(text)
		// Toast on completion. Goroutine-thread call into ShowToast is
		// the same shape reportBuildOutput already uses to push into
		// BuildOutput.SetOutput from here — the toast manager has its
		// own mutex around the entry list, and the popup AttachWindow
		// runs once on the next idle tick.
		if err == nil {
			silkideToast(i18n.T("Build successful"), gui.ToastSuccess)
		} else {
			silkideToast(i18n.T("Build failed"), gui.ToastError)
		}
	}()
}

// runProjectTests runs "go test -v ./..." in the project directory and
// fans the captured output into two panes: the raw BuildOutput log
// (where the user reads the runner's verbatim stream) and the
// structured TestResultsPanel (one row per test, click a FAIL to jump
// to source). On any failure — non-nil exit error OR failed>0 in the
// parsed tally — the test-results tab is brought to the front so the
// pass/fail list lands in view instead of the raw scrollback that
// reportBuildOutput just focused. Goroutine + main-thread dispatch
// match buildProject so the IDE stays responsive while the toolchain
// works.
//
// Wired to F7 + Cmd+Shift+T + the "Run Tests" palette command.
func runProjectTests(canvas *ged.GedView) {
	if globalBuildOutput == nil {
		buildOutputPane()
	}
	dir := projectDir(canvas)
	reportBuildOutput(fmt.Sprintf("$ go test -v ./...   (cwd: %s)", dir))
	silkideToast(i18n.T("Running tests..."), gui.ToastInfo)
	go func() {
		cmd := exec.Command("go", "test", "-v", "./...")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil && text == "" {
			text = err.Error()
		}
		reportBuildOutput(text)
		// Structured pass/fail view. SetOutput re-parses the same text
		// BuildOutput just consumed, so the two panes always agree on
		// the latest run's content.
		if globalTestResults != nil {
			globalTestResults.SetOutput(text)
		}
		// Auto-focus the test-results tab on failure — the raw log was
		// already promoted by reportBuildOutput, but a failed run is the
		// case where the structured view earns its keep, so it wins.
		_, failed, _ := testResultCounts()
		if err != nil || failed > 0 {
			if globalTestResults != nil {
				dockSetActiveView(globalBottomDock, globalTestResults)
			}
			silkideToast(i18n.T("Tests failed"), gui.ToastError)
		} else {
			silkideToast(i18n.T("Tests passed"), gui.ToastSuccess)
		}
	}()
}

// testResultCounts is a nil-safe wrapper around globalTestResults.Counts()
// so callers don't have to repeat the guard. Returns zeros when the
// panel hasn't been built yet (e.g. very early test paths).
func testResultCounts() (passed, failed, skipped int) {
	if globalTestResults == nil {
		return 0, 0, 0
	}
	return globalTestResults.Counts()
}

// runProjectWithCoverage runs "go test -coverprofile=<tmp> -covermode=set
// ./..." in the project directory, parses the resulting profile through
// core.ParseCoverage + core.BuildFileCoverage, and pushes per-file
// line→covered maps into every editor tab whose tracked path matches a
// covered file. Path matching is exact-first then suffix-fallback (the
// cover profile records module-relative paths while openEditors keys are
// absolute), so "silk/foo/bar.go" and "/abs/.../silk/foo/bar.go" line up.
//
// The cover profile lives in os.TempDir and is overwritten on each run;
// the previous file is deleted at the start of the next run so the temp
// dir doesn't accumulate. Bound to Cmd+Shift+F7 + the "Run with Coverage"
// palette command.
func runProjectWithCoverage(canvas *ged.GedView) {
	if globalBuildOutput == nil {
		buildOutputPane()
	}
	dir := projectDir(canvas)
	// Tear down the previous run's profile before this one writes a new
	// one — os.TempDir is shared with the rest of the system, and we don't
	// want a stale cover.out from a previous silkide session leaking in
	// if go test bails before writing.
	if coverageTempFile != "" {
		_ = os.Remove(coverageTempFile)
	}
	tmp, err := os.CreateTemp("", "silkide-cover-*.out")
	if err != nil {
		reportBuildOutput(fmt.Sprintf("coverage: temp file: %v", err))
		return
	}
	_ = tmp.Close()
	coverageTempFile = tmp.Name()

	reportBuildOutput(fmt.Sprintf(
		"$ go test -coverprofile=%s -covermode=set ./...   (cwd: %s)",
		coverageTempFile, dir))
	silkideToast(i18n.T("Running with coverage..."), gui.ToastInfo)
	go func() {
		cmd := exec.Command("go", "test",
			"-coverprofile="+coverageTempFile, "-covermode=set", "./...")
		if dir != "" {
			cmd.Dir = dir
		}
		out, runErr := cmd.CombinedOutput()
		text := string(out)
		if runErr != nil && text == "" {
			text = runErr.Error()
		}
		reportBuildOutput(text)
		if globalTestResults != nil {
			globalTestResults.SetOutput(text)
		}

		data, readErr := os.ReadFile(coverageTempFile)
		if readErr != nil {
			reportBuildOutput(fmt.Sprintf("coverage: read profile: %v", readErr))
			silkideToast(i18n.T("Coverage failed"), gui.ToastError)
			return
		}
		_, blocks, parseErr := core.ParseCoverage(string(data))
		if parseErr != nil {
			// Non-fatal: ParseCoverage returns the blocks it managed to
			// recover alongside the error, so still render what we got.
			reportBuildOutput(fmt.Sprintf("coverage: %v", parseErr))
		}
		fileCov := core.BuildFileCoverage(blocks)
		applyCoverageToOpenEditors(fileCov)
		if runErr != nil {
			silkideToast(i18n.T("Coverage failed"), gui.ToastError)
		} else {
			silkideToast(i18n.T("Coverage applied"), gui.ToastSuccess)
		}
	}()
}

// applyCoverageToOpenEditors iterates the openEditors map and pushes the
// matching per-file coverage map into each CodeEditor. coverageForPath
// owns the exact-match-then-suffix-match policy; this helper is just
// the side-effecting walk, so the matcher stays pure and unit-testable.
func applyCoverageToOpenEditors(fc map[string]*core.FileCoverage) {
	for path, ed := range openEditors {
		if ed == nil {
			continue
		}
		if cov, ok := coverageForPath(fc, path); ok {
			ed.SetCoverage(cov.Covered)
		}
	}
}

// coverageForPath looks up `editorPath` in the per-file coverage map. The
// `go test -coverprofile` output records paths the way the toolchain
// observed them (typically "<module>/<pkg>/<file>.go" — module-relative)
// while the editor tracks absolute filesystem paths, so an exact match
// rarely hits first try; the suffix fallback rescues the common case
// where the editor's path ENDS with the profile's recorded path.
//
// Returns (nil, false) when nothing matches. Pure helper so unit tests
// can pin the policy without spawning go test.
func coverageForPath(fc map[string]*core.FileCoverage, editorPath string) (*core.FileCoverage, bool) {
	if editorPath == "" {
		return nil, false
	}
	if cov, ok := fc[editorPath]; ok {
		return cov, true
	}
	// Suffix match. Use "/" + key so we only match on a directory
	// boundary — a profile key "foo.go" must not match an editor path
	// ending in "wfoo.go".
	for key, cov := range fc {
		if key == "" {
			continue
		}
		if strings.HasSuffix(editorPath, string(filepath.Separator)+key) ||
			strings.HasSuffix(editorPath, "/"+key) {
			return cov, true
		}
	}
	return nil, false
}

// runProjectVet runs "go vet ./..." in the project directory. The
// diagnostics it emits are in the same "file:line:col: msg" shape Go
// build errors use, so feeding the captured text through
// globalProblems.SetOutput (which already parses that format) gives
// the user a structured, clickable issues list for free — no separate
// parser. The raw log also lands in BuildOutput so the verbatim
// runner stream is still visible if the user wants it.
//
// Bound to Shift+F6 + the "Run go vet" palette command.
func runProjectVet(canvas *ged.GedView) {
	if globalBuildOutput == nil {
		buildOutputPane()
	}
	dir := projectDir(canvas)
	reportBuildOutput(fmt.Sprintf("$ go vet ./...   (cwd: %s)", dir))
	silkideToast(i18n.T("Running go vet..."), gui.ToastInfo)
	go func() {
		cmd := exec.Command("go", "vet", "./...")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil && text == "" {
			text = err.Error()
		}
		reportBuildOutput(text)
		// Problems gets a separate SetOutput because reportBuildOutput
		// also feeds it but its content is the raw stream including the
		// "$ go vet …" header. The header line is harmless (Problems
		// skips lines that don't match the file:line:col shape), so
		// we don't have to gate it.
		if globalProblems != nil {
			globalProblems.SetOutput(text)
		}
		if err == nil {
			silkideToast(i18n.T("go vet ok"), gui.ToastSuccess)
		} else {
			silkideToast(i18n.T("go vet failed"), gui.ToastError)
		}
	}()
}

// gofmtSource pipes `src` through the system `gofmt` and returns the
// formatted output on success. Errors propagate verbatim — the caller
// decides whether a malformed buffer (syntax error from a WIP edit) is
// fatal or "leave the buffer alone". Pure helper so a unit test can
// round-trip a known-bad and known-good input without touching the
// editor.
//
// We use exec.Command("gofmt") on stdin/stdout rather than calling
// go/format.Source directly so the result matches whatever the user's
// installed toolchain ships — same gofmt the project's CI/precommit
// runs, no version drift.
func gofmtSource(src string) (string, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(src)
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return "", fmt.Errorf("gofmt: %s", msg)
		}
		return "", err
	}
	return out.String(), nil
}

// formatProjectStatus builds the status-bar project-cell label from
// the three sources the IDE pulls at startup: the project basename
// (filepath.Base(cwd)), the module path read out of go.mod, and the
// optional go.work workspace. Pure helper (no globals, no I/O) so the
// resulting label can be unit-tested without standing up a frame.
//
// Variants:
//
//	"silk"                                 // no go.mod, no go.work
//	"silk · silk/example"                  // go.mod only
//	"silk · workspace"                     // go.work only, empty Uses
//	"silk · workspace(3)"                  // go.work, 3 use modules
//	"silk · silk/example · workspace(3)"   // both go.mod and go.work
//
// The " · workspace" badge appears whenever `work` is non-nil — the
// caller is expected to only pass non-nil when FindGoWork succeeded.
// Empty `project` falls back to "silkide" so the cell never collapses.
func formatProjectStatus(project, module string, work *core.GoWork) string {
	if project == "" {
		project = "silkide"
	}
	out := project
	if module != "" {
		out += " · " + module
	}
	if work != nil {
		if n := len(work.Uses); n > 0 {
			out += fmt.Sprintf(" · workspace(%d)", n)
		} else {
			out += " · workspace"
		}
	}
	return out
}

// showDiffVsSaved pops a modal Dialog containing a gui.DiffView that
// compares the active editor's in-buffer text against the file's
// saved-on-disk content. First iteration: a popup dialog (simplest
// path that gets the diff in front of the user); a tabbed editor diff
// is a follow-up.
//
// No-ops silently when there is no active code editor or the active
// tab has no tracked file path (a fresh sample-seed tab never lived in
// openEditors). Toast-and-return on read failure so the user sees why
// nothing opened.
func showDiffVsSaved(tabs *gui.TabWidget) {
	ed := activeEditor(tabs)
	if ed == nil {
		return
	}
	path := activeEditorPath(tabs)
	if path == "" {
		return
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		silkideToast(i18n.T("Failed to read saved file"), gui.ToastError)
		return
	}
	parent := gui.IWidget(globalFrame)
	if parent == nil {
		parent = gui.DefaultFrame()
	}
	dlg := gui.NewDialog(i18n.Tf("Diff vs Saved: %s", filepath.Base(path)), parent)
	dv := gui.NewDiffView()
	dv.SetTexts(string(saved), ed.Text())
	box := gui.NewVBox()
	box.SetSpacing(0)
	box.AddWidget(dv)
	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Close"), gui.DialogOK)
	dlg.SetSize(820, 560)
	dlg.ShowModal()
}

// escapeTestRunRegex escapes the regex metacharacters in a Go test
// name so it can be passed verbatim to `go test -run ^<name>$`. Go's
// `-run` flag is an unanchored regex; we anchor with ^…$ at the call
// site and escape the body here so subtest paths with "." or "/" don't
// accidentally widen the match (e.g. "Foo/Bar" would otherwise let "/"
// match anything). Idempotent for plain ASCII names with no
// metacharacters.
func escapeTestRunRegex(name string) string {
	const meta = `\.+*?()|[]{}^$`
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if strings.IndexByte(meta, c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// runSingleTest spawns `go test -run ^<escaped name>$ -v ./...` in the
// project directory and fans the captured output into BuildOutput +
// globalTestResults, mirroring runProjectTests. Used by the
// TestResultsPanel right-click "运行此测试" entry — the panel signals
// a test name, we translate it here and stream the new output back
// through the same two panes the F7 path drives.
//
// Goroutine + main-thread dispatch matches runProjectTests so the IDE
// stays responsive while the toolchain works. No-op on an empty name —
// the panel only fires for rows where `r.Name` is non-empty, but the
// guard makes the function safe to call from anywhere.
func runSingleTest(canvas *ged.GedView, name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	if globalBuildOutput == nil {
		buildOutputPane()
	}
	dir := projectDir(canvas)
	pattern := "^" + escapeTestRunRegex(name) + "$"
	reportBuildOutput(fmt.Sprintf("$ go test -run %s -v ./...   (cwd: %s)", pattern, dir))
	silkideToast(i18n.Tf("Running %s...", name), gui.ToastInfo)
	go func() {
		cmd := exec.Command("go", "test", "-run", pattern, "-v", "./...")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil && text == "" {
			text = err.Error()
		}
		reportBuildOutput(text)
		if globalTestResults != nil {
			globalTestResults.SetOutput(text)
		}
		_, failed, _ := testResultCounts()
		if err != nil || failed > 0 {
			if globalTestResults != nil {
				dockSetActiveView(globalBottomDock, globalTestResults)
			}
			silkideToast(i18n.T("Tests failed"), gui.ToastError)
		} else {
			silkideToast(i18n.T("Tests passed"), gui.ToastSuccess)
		}
	}()
}

// loadModulePath walks upward from `cwd` looking for the first go.mod,
// then scans its lines for "module <path>". Returns the bare module
// path (no quotes, no inline-comment tail) or "" when no go.mod is
// found in any ancestor. Inline scan — a richer parser will replace
// this once the concurrent core/gomod package lands.
//
// Walks until filepath.Dir returns the same directory (filesystem
// root) so the loop terminates on every platform.
func loadModulePath(cwd string) string {
	if cwd == "" {
		return ""
	}
	dir := cwd
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "module ") {
					name := strings.TrimSpace(strings.TrimPrefix(trim, "module"))
					name = strings.TrimSpace(strings.SplitN(name, "//", 2)[0])
					name = strings.Trim(name, `"`)
					return name
				}
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// saveActiveEditorToDisk writes the active code editor's buffer back
// to the file it was opened from. .go files get a `gofmt` pass on the
// way out so the saved buffer matches the toolchain's formatting
// (same convention every Go IDE uses on Cmd+S). A gofmt failure (WIP
// code with a syntax error) leaves the buffer untouched and surfaces a
// quiet warning toast — the user still gets their save, just not the
// reformat.
//
// Returns false when there is no active editor or no tracked path —
// performSave's design-canvas branch already covered the no-canvas
// case, so the Cmd+S handler can call both and the right side fires.
func saveActiveEditorToDisk(tabs *gui.TabWidget) bool {
	ed := activeEditor(tabs)
	if ed == nil {
		return false
	}
	path := activeEditorPath(tabs)
	if path == "" {
		return false
	}
	text := ed.Text()
	out := text
	if strings.EqualFold(filepath.Ext(path), ".go") {
		if formatted, err := gofmtSource(text); err == nil {
			out = formatted
			if formatted != text {
				ed.SetText(formatted)
			}
		} else {
			silkideToast(i18n.T("gofmt failed; saved unformatted"), gui.ToastWarning)
		}
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		silkideToast(i18n.Tf("Save failed: %v", err), gui.ToastError)
		return false
	}
	silkideToast(i18n.Tf("Saved %s", filepath.Base(path)), gui.ToastSuccess)
	return true
}

// projectDir resolves the directory the toolchain should run in:
// the active .silkui's containing directory if one is open, else
// the silkide process's cwd. Empty string means "fall back to the
// caller's existing cwd".
func projectDir(canvas *ged.GedView) string {
	if canvas != nil {
		if scene := canvas.GedScene(); scene != nil {
			if fn := scene.Filename(); fn != "" {
				return filepath.Dir(fn)
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// capitalise upper-cases the first byte of s if it's an ASCII lower
// letter. Identical to cmd/silkgen's helper, kept inline here so
// silkide doesn't pull cmd/silkgen as a library dependency (cmd/
// imports flow toward leaves, not the other way).
func capitalise(s string) string {
	if s == "" {
		return s
	}
	c := s[0]
	if c >= 'a' && c <= 'z' {
		return string(c-32) + s[1:]
	}
	return s
}

// newDesignCanvas wipes the active design canvas and replaces it
// with a fresh GedScene — the IDE-level File→New. Selection
// callbacks survive (they hang off the GedView, not the scene), but
// the inspector needs to be re-pointed at the new scene's tree.
//
// Prompts to save when the current scene has unsaved work; an
// accidental Cmd+N on a dirty design used to wipe the work
// silently. Save / Discard / Cancel — Cancel aborts the new.
func newDesignCanvas(canvas *ged.GedView) {
	if canvas == nil {
		return
	}
	if !confirmDiscardDirty(canvas) {
		return
	}
	scene := ged.NewGedScene()
	canvas.SetScene(scene)
	if globalInspector != nil {
		globalInspector.SetScene(scene)
		globalInspector.Rebuild()
	}
	rebindAutoSaver(scene)
	canvas.Update()
}

// rebindAutoSaver retargets the package-level auto-saver at `scene`.
// Called whenever the active scene changes (File→New, opening a
// .silkui from the tree, etc.) so the next 60-second tick writes
// the `.autosave` companion next to the right file. Lazily
// constructs the saver on first call.
//
// AutoSaver.tick is a no-op when the scene is clean or has no
// filename, so re-binding to a fresh untitled scene doesn't
// generate spurious empty `.autosave` files.
func rebindAutoSaver(scene *ged.GedScene) {
	if scene == nil {
		return
	}
	if globalAutoSaver == nil {
		globalAutoSaver = ged.NewAutoSaver()
	} else {
		globalAutoSaver.Stop()
	}
	globalAutoSaver.Start(scene)
}

// confirmRecoverFromAutosave prompts the user when an .autosave
// companion exists that's newer than the .silkui being opened —
// the autosave was written by ged.AutoSaver after the file was
// last hand-saved, so it represents work-in-progress that would
// otherwise be lost. Returns true when the user picks Recover and
// the caller should load `recoveryPath` instead of the original
// `path`.
//
// Yes/No only — picking No proceeds with the regular .silkui open.
// The .autosave isn't deleted in either case so the user can still
// inspect it manually if curiosity strikes.
func confirmRecoverFromAutosave(parent gui.IWidget, path, recoveryPath string) bool {
	msg := i18n.Tf(
		"A more recent autosave was found for %s. Recover from it?",
		filepath.Base(path))
	return gui.ShowConfirmDialog(parent, i18n.T("Recover Autosave"), msg)
}

// confirmDiscardDirty returns true when it's safe to wipe / replace
// the current scene. If the scene has unsaved changes (UndoStack
// not clean) the user gets a Save / Discard / Cancel dialog:
//
//   - Save     → call scene.Save(); proceed iff the save succeeded.
//   - Discard  → proceed without saving.
//   - Cancel   → abort the calling action.
//
// Returns true when the scene is clean (no dialog), the user picked
// Discard, or the user picked Save and the save completed. Returns
// false on Cancel and on a Save that the user aborted from the
// SaveFileDialog. Sharing this helper keeps File→New, Open, and
// future close/quit paths consistent.
func confirmDiscardDirty(canvas *ged.GedView) bool {
	if canvas == nil {
		return true
	}
	scene := canvas.GedScene()
	if scene == nil {
		return true
	}
	if stack := scene.UndoStack(); stack == nil || stack.IsClean() {
		return true
	}
	parent := gui.IWidget(canvas)
	dlg := gui.NewDialog(i18n.T("Unsaved changes"), parent)
	content := gui.NewVBox()
	content.SetSpacing(12)
	msg := gui.NewLabel(i18n.T("The current design has unsaved changes. Save before continuing?"))
	msg.SetWrap(true)
	content.AddWidget(msg)
	dlg.SetContent(content)
	dlg.AddButton(i18n.T("Save"), gui.DialogOK)
	dlg.AddButton(i18n.T("Discard"), gui.DialogNo)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	switch dlg.ShowModal() {
	case gui.DialogOK:
		// Save() returns false when the user cancelled the
		// SaveFileDialog or the write failed; in both cases we
		// must not proceed and lose the work.
		return scene.Save()
	case gui.DialogNo:
		return true
	default:
		return false
	}
}

// startReloader spins up the file-system watcher on first .silkui
// open. The onReload closure captures the design canvas so changes
// to a watched .silkui flow back through GedScene.OpenFile.
func startReloader(canvas *ged.GedView) {
	if globalReloader != nil || canvas == nil {
		return
	}
	r, err := hotreload.New(
		func(path string, _ *decl.Node) error {
			scene := canvas.GedScene()
			if scene == nil {
				return nil
			}
			// Reload on the watcher goroutine. silk's render loop
			// polls glfw events; OpenFile's internal Update() fires
			// the next paint pass off whatever pixels we land. Not
			// ideal cross-thread but matches how every other silk
			// callback (fswatch, signal-slot) behaves.
			_ = scene.OpenFile(path)
			return nil
		},
		func(path string, err error) {
			core.Warn("hotreload: ", path, ": ", err)
		},
		hotreload.Options{
			AllowedExt: []string{".silkui"},
		},
	)
	if err != nil {
		core.Warn("hotreload.New: ", err)
		return
	}
	globalReloader = r
}

// watchForReload registers a .silkui path with the file watcher so
// external edits flow back into the design canvas. Idempotent —
// re-watching a path is a no-op.
func watchForReload(canvas *ged.GedView, path string) {
	if filepath.Ext(path) != ".silkui" {
		return
	}
	startReloader(canvas)
	if globalReloader != nil {
		_ = globalReloader.Watch(path)
	}
}

// recordRecentFile updates the MRU list when a file open succeeds.
// nil-safe so unit tests can call openFromTree without setting up a
// full preferences store.
func recordRecentFile(path string) {
	if globalPrefs == nil {
		return
	}
	globalPrefs.AddRecentFile(path)
}

// currentSessionPaths snapshots what's open right now so the next
// launch can reopen it: the active design canvas's .silkui (if it has
// a filename) followed by every open editor-tab path. The .silkui goes
// first so restoreSession lands the user back on the design canvas the
// same way openFromTree does for a .silkui. Editor paths come from the
// openEditors map; iteration order isn't deterministic but the set is
// what matters for restore. De-dup happens in existingPaths on the way
// back in.
func currentSessionPaths(canvas *ged.GedView) []string {
	var paths []string
	if canvas != nil {
		if scene := canvas.GedScene(); scene != nil {
			if fn := scene.Filename(); fn != "" {
				paths = append(paths, fn)
			}
		}
	}
	for path := range openEditors {
		paths = append(paths, path)
	}
	return paths
}

// restoreSession reopens the files persisted by SetOpenSession on the
// previous close, skipping any that no longer exist (existingPaths).
// Routes each through openFromTree so a .silkui re-lands in the design
// canvas and code files re-open as editor tabs — identical to the user
// having clicked them in the tree. No-op when prefs is unset (tests) or
// nothing was saved.
func restoreSession(editorTabs *gui.TabWidget, designCanvas *ged.GedView) {
	if globalPrefs == nil {
		return
	}
	for _, path := range existingPaths(globalPrefs.OpenSession()) {
		openFromTree(path, editorTabs, designCanvas, nil)
	}
}

// openEditors tracks which paths are already loaded in the editor
// tabs and the CodeEditor that holds them. Lets a second open of
// the same path re-focus the existing tab instead of stacking a
// duplicate, and lets BuildOutput's click-to-jump scroll the right
// editor without re-reading the file from disk.
var openEditors = map[string]*gui.CodeEditor{}

// openFileInEditor adds a fresh code-editor tab for path in tabs,
// or focuses an existing one if the path is already open. Returns
// true on success so the caller can record it in the MRU list.
func openFileInEditor(tabs *gui.TabWidget, path string) bool {
	if tabs == nil {
		return false
	}
	if ed, ok := openEditors[path]; ok && ed != nil {
		focusEditorTab(tabs, ed)
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	ed := makeCodeEditor(string(data))
	tabs.AddTab(ed, filepath.Base(path), nil)
	openEditors[path] = ed
	focusEditorTab(tabs, ed)
	return true
}

// openFileInEditorAt is openFileInEditor + ScrollToLine; the entry
// point BuildOutput's SigErrorClick uses to jump to a compile error.
// `line` is 1-based per the standard Go error format; CodeEditor
// uses 0-based indexing internally.
func openFileInEditorAt(tabs *gui.TabWidget, path string, line, col int) {
	if !openFileInEditor(tabs, path) {
		return
	}
	ed := openEditors[path]
	if ed == nil {
		return
	}
	target := line - 1
	if target < 0 {
		target = 0
	}
	ed.ScrollToLine(target)
}

// focusEditorTab walks the editor tabs to find the one whose stack
// page is `ed` and switches to it. Without this, clicking a
// build-error in the BuildOutput pane would land on the right
// scroll position but in whatever tab happened to be active.
func focusEditorTab(tabs *gui.TabWidget, ed *gui.CodeEditor) {
	if tabs == nil || ed == nil {
		return
	}
	stack := tabs.Stack()
	if stack == nil {
		return
	}
	for i := 0; i < tabs.Count(); i++ {
		if stack.Page(i) == gui.IWidget(ed) {
			tabs.SetCurrentIndex(i)
			return
		}
	}
}

// activeEditor returns the CodeEditor backing the currently-active
// editor tab, or nil when the active tab isn't a code editor (the
// design canvas and welcome screen live in the center dock, not these
// tabs, but a future caller could add non-editor pages here). The
// outline panel's navigate callback uses it to scroll the right
// editor to a clicked symbol.
func activeEditor(tabs *gui.TabWidget) *gui.CodeEditor {
	if tabs == nil {
		return nil
	}
	stack := tabs.Stack()
	if stack == nil {
		return nil
	}
	idx := tabs.CurrentIndex()
	if idx < 0 || idx >= stack.Count() {
		return nil
	}
	ed, _ := stack.Page(idx).(*gui.CodeEditor)
	return ed
}

// activeEditorPath returns the file path of the active editor tab by
// reverse-looking-up the editor in the openEditors map. Empty string
// when there is no active editor or it isn't a tracked open file (a
// fresh sample-seed tab from buildEditorTabs has no path). Pure read of
// the package map; no I/O.
func activeEditorPath(tabs *gui.TabWidget) string {
	ed := activeEditor(tabs)
	if ed == nil {
		return ""
	}
	for path, e := range openEditors {
		if e == ed {
			return path
		}
	}
	return ""
}

// bookmarkLabelForLine builds the human-readable label for a bookmark
// from the line's text content. Trims leading/trailing whitespace and
// caps at ~50 characters with a one-character "…" ellipsis so the
// BookmarksPanel rows stay readable. Returns an empty string for an
// empty / whitespace-only source line — the panel renders the
// "file:line" locator without a trailing label in that case.
func bookmarkLabelForLine(text string) string {
	const max = 50
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	if len([]rune(t)) <= max {
		return t
	}
	r := []rune(t)
	return string(r[:max-1]) + "…"
}

// addBookmarkAtCursor pushes a bookmark for the active editor's cursor
// line into the BookmarksPanel. No-op when there's no active editor,
// no tracked file path, or no panel — the F2 shortcut and the "Add
// Bookmark" palette command both route here so the same guard covers
// every entry point. Uses Add (idempotent on (file, line)) rather than
// Toggle so repeated invocations refresh the label rather than
// silently removing the bookmark.
func addBookmarkAtCursor(tabs *gui.TabWidget) {
	if globalBookmarks == nil {
		return
	}
	ed := activeEditor(tabs)
	if ed == nil {
		return
	}
	path := activeEditorPath(tabs)
	if path == "" {
		return
	}
	// CursorLine is 0-based; the bookmarks panel renders 1-based
	// "basename:line" locators (Qt Creator convention), so shift by +1
	// here. openFileInEditorAt also takes 1-based lines, so a click on
	// the row navigates back to the same line.
	cursor0 := ed.CursorLine()
	line1 := cursor0 + 1
	lines := ed.Lines()
	src := ""
	if cursor0 >= 0 && cursor0 < len(lines) {
		src = lines[cursor0]
	}
	globalBookmarks.Add(path, line1, bookmarkLabelForLine(src))
}

// syncOutlineToActiveEditor re-points the right-dock outline panel at
// the active editor tab so its symbol tree reflects the file the user
// is looking at. Called once at startup and again on every tab switch
// (editorTabs.SetCurrentChangedCallback). SetEditor re-parses symbols
// immediately; the panel keeps following content edits on its own via
// a Draw-time line-count/length hash, so no per-keystroke hook is
// needed here. No-op when the active tab isn't a code editor.
func syncOutlineToActiveEditor(tabs *gui.TabWidget) {
	if globalOutline == nil {
		return
	}
	globalOutline.SetEditor(activeEditor(tabs))
}

// sampleMainGo returns the canonical "Hello, gogpu!" main.go
// matching the mockup so visual reviewers can verify the syntax
// highlighting and tab placement against the source image.
func sampleMainGo() string {
	return `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", handler)
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, gogpu!")
}
`
}

func sampleServerGo() string {
	return `package server

import "net/http"

type Server struct {
	mux *http.ServeMux
}

func New() *Server {
	return &Server{mux: http.NewServeMux()}
}

func (s *Server) Handle(pattern string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, h)
}
`
}

func sampleGoMod() string {
	return `module github.com/user/myproject

go 1.25

require (
	github.com/some/dep v1.2.3
)
`
}

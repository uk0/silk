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
	"path/filepath"
	"strings"

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

	// Order: panels first so the toolbar can capture references to
	// the editor tabs and design canvas, wiring Open / Save buttons
	// through to GedScene.OpenFile / Save without the global plumbing
	// SuggestDocDock would otherwise force.
	editorTabs, designCanvas := buildPanels(frame)
	buildToolBar(frame, editorTabs, designCanvas)
	statusBar := buildStatusBar(frame)
	registerShortcuts(editorTabs, designCanvas)
	startTitleSync(frame, designCanvas)

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
				statusBar.SetMessage(i18n.Tf("Selected: %s", name))
				return
			}
			statusBar.SetMessage(i18n.Tf("Selected: %d items", n))
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
	// the unicode bars. AddAction's icon param accepts nil for text-
	// only buttons.
	if btn := tb.AddAction("☰", nil, func() {}); btn != nil {
		gui.SetToolTip(btn, i18n.T("Menu"))
	}
	tb.AddSeparator()

	// New (+ glyph): clears the design canvas to a fresh scene. Bound
	// to Cmd+N too via registerShortcuts. silk doesn't ship a "new"
	// icon yet, so we fall back to a glyph label.
	if btn := tb.AddAction("+", nil, func() {
		newDesignCanvas(designCanvas)
	}); btn != nil {
		gui.SetToolTip(btn, i18n.T("New"))
	}

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
	if btn := tb.AddAction("↻", nil, func() {
		// Refresh: force-redraw active design canvas. Useful when
		// editing the underlying .silkui file in another editor.
		if designCanvas != nil {
			designCanvas.Update()
		}
	}); btn != nil {
		gui.SetToolTip(btn, i18n.T("Refresh"))
	}
	addIconAction("", "save", "Save", func() {
		// Save the current design canvas as .silkui. GedScene.Save()
		// pops a SaveFileDialog if the scene has no filename yet.
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			scene.Save()
		}
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

	// Run / Debug / Preview / PropSheet. run.png and preview.png
	// exist natively; debug doesn't yet have an asset so we fall
	// back to a short text label.
	addIconAction("", "run", "Run", func() {})
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
	addIconAction("", "propsheet", "Settings", func() {})

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
	}

	// Bottom dock: terminal + build output. Toolchain stuff a
	// developer glances at without leaving their main work.
	bottomDockI := dock.SplitNewDock(false, true)
	if bottomDock, ok := bottomDockI.(*gui.Dock); ok {
		bottomDock.AddView(buildTerminalPane())
		bottomDock.AddView(buildOutputPane())
	}

	return editorTabs, designCanvas
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

// buildTerminalPane returns a placeholder code-editor view mocked up
// as a terminal session. A real Terminal widget would be wired here
// (silk has ged.TerminalPanel); for the demo we keep dependencies
// minimal so silkide builds in any silk checkout.
func buildTerminalPane() gui.IWidget {
	ed := gui.NewCodeEditor()
	ed.SetText(`PS> go build ./...
PS> go test ./... -count=1
ok  github.com/user/myproject/pkg/server   0.042s
ok  github.com/user/myproject/internal     0.038s
PS> go run cmd/main.go
Server starting on :8080
PS> _`)
	return ed
}

// buildOutputPane: another placeholder tab that real apps would
// replace with a build-output / problems view.
func buildOutputPane() gui.IWidget {
	ed := gui.NewCodeEditor()
	ed.SetText(`(build output)`)
	return ed
}

// buildStatusBar populates the bottom status strip with project /
// branch / cursor / encoding / runtime / version cells. StatusBar
// uses AddPermanentWidget for the right-aligned cells that show
// project metadata; SetMessage drives the transient left-aligned
// message slot which we leave blank initially.
func buildStatusBar(frame *gui.Frame) *gui.StatusBar {
	sb := gui.NewStatusBar()

	cwd, _ := os.Getwd()
	project := filepath.Base(cwd)
	if project == "" {
		project = "silkide"
	}
	sb.AddPermanentWidget(gui.NewLabel(project))
	sb.AddPermanentWidget(gui.NewLabel("main"))
	sb.AddPermanentWidget(gui.NewLabel("Ln 1, Col 1"))
	sb.AddPermanentWidget(gui.NewLabel("UTF-8"))
	sb.AddPermanentWidget(gui.NewLabel("Go 1.25"))
	sb.AddPermanentWidget(gui.NewLabel("v0.1.3"))

	frame.SetStatusBar(sb)
	return sb
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
		if err := canvas.GedScene().OpenFile(path); err == nil {
			recordRecentFile(path)
			watchForReload(canvas, path)
			if centerDock != nil {
				// Bring the design canvas to the front so the user sees
				// the loaded scene immediately.
				if idx := centerDock.IndexOfView(canvas); idx >= 0 {
					centerDock.SetActiveIndex(idx)
				}
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

// newDesignCanvas wipes the active design canvas and replaces it
// with a fresh GedScene — the IDE-level File→New. Selection
// callbacks survive (they hang off the GedView, not the scene), but
// the inspector needs to be re-pointed at the new scene's tree.
//
// Doesn't prompt to save the current dirty state; that's a follow-up
// once silkide grows a confirmation dialog. The 99% case for File→New
// is "I'm exploring, throw it away".
func newDesignCanvas(canvas *ged.GedView) {
	if canvas == nil {
		return
	}
	scene := ged.NewGedScene()
	canvas.SetScene(scene)
	if globalInspector != nil {
		globalInspector.SetScene(scene)
		globalInspector.Rebuild()
	}
	canvas.Update()
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

// openFileInEditor adds a fresh code-editor tab for path in tabs.
// Used as the default branch of openFromTree. Returns true on
// success so the caller can record it in the MRU list.
func openFileInEditor(tabs *gui.TabWidget, path string) bool {
	if tabs == nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	ed := makeCodeEditor(string(data))
	tabs.AddTab(ed, filepath.Base(path), nil)
	return true
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

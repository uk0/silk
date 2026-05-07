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
	"os"
	"path/filepath"

	"silk/core"
	"silk/ged"
	"silk/gui"
)

func main() {
	frame := gui.NewFrameWindow()
	frame.SetUuidStr("c1d8e2f0-1a3b-4c2d-9e7f-silkide00001")
	frame.SetTitle(idTitle())
	gui.SetDefaultFrame(frame)

	buildToolBar(frame)
	buildPanels(frame)
	buildStatusBar(frame)

	frame.SetClosedCallback(func(*gui.Frame) { core.Quit() })

	if win := frame.Window(); win != nil {
		win.SetSize(1280, 800)
		win.MoveToCenter()
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

// buildToolBar adds the icon-only top toolbar matching the mockup's
// "hamburger / open / refresh / save / back / forward … run / debug
// / search / settings" layout. We use AddAction with empty icons
// (text labels only) so the demo runs without bundled icon assets;
// real apps register paint.Icon values via paint.LoadIcon.
func buildToolBar(frame *gui.Frame) {
	tb := gui.NewToolBar()

	tb.AddAction("☰", nil, func() {})
	tb.AddSeparator()

	tb.AddAction("Open", nil, func() {})
	tb.AddAction("Refresh", nil, func() {})
	tb.AddAction("Save", nil, func() {})
	tb.AddSeparator()

	tb.AddAction("Back", nil, func() {})
	tb.AddAction("Forward", nil, func() {})
	tb.AddSeparator()

	// The mockup keeps run/debug/search/settings on the right-hand
	// side. ToolBar lays children left-to-right; for the demo we
	// just append everything in order. A future enhancement could
	// add ToolBar.AddSpacer() to push trailing actions to the right
	// edge — out of scope for this round.
	tb.AddAction("Run", nil, func() {})
	tb.AddAction("Debug", nil, func() {})
	tb.AddSeparator()
	tb.AddAction("Search", nil, func() {})
	tb.AddAction("Settings", nil, func() {})

	frame.SetToolBar(tb)
}

// buildPanels installs the central content area: a left dock with
// the file explorer, a centre dock with the multi-tab code editor
// stack, and a bottom dock with the Terminal / Output tabs. The
// existing FileExplorer + CodeEditor widgets carry the actual
// content; layout glue is the only thing this file owns.
func buildPanels(frame *gui.Frame) {
	dock, ok := frame.SuggestDocDock().(*gui.Dock)
	if !ok || dock == nil {
		return
	}

	editorTabs := buildEditorTabs(dock)
	dock.AddView(editorTabs)

	leftDockI := dock.SplitNewDock(true, false)
	leftDock, _ := leftDockI.(*gui.Dock)

	if leftDock != nil {
		fileExplorer := ged.NewFileExplorer()
		fileExplorer.SetRootDir(".")
		fileExplorer.SigFileOpen(func(path string) {
			openFileInEditor(editorTabs, path)
		})
		leftDock.AddView(fileExplorer)
	}

	bottomDockI := dock.SplitNewDock(false, true)
	if bottomDock, ok := bottomDockI.(*gui.Dock); ok {
		bottomDock.AddView(buildTerminalPane())
		bottomDock.AddView(buildOutputPane())
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
func buildStatusBar(frame *gui.Frame) {
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
}

// openFileInEditor adds (or focuses) a tab for path in tabs. Used by
// the FileExplorer click handler so the demo behaves like a real IDE.
func openFileInEditor(tabs *gui.TabWidget, path string) {
	if tabs == nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	ed := makeCodeEditor(string(data))
	tabs.AddTab(ed, filepath.Base(path), nil)
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

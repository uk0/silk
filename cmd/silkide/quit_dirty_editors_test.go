package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/gui"
)

// Quitting is the same data loss as closing a tab, one gesture wider: Cmd+Q and
// the title bar's close button used to end the process over every unsaved
// editor buffer at once. Worse than plain loss — SetOpenSession plus
// restoreSession reopen those files FROM DISK next launch, so the tabs all come
// back and the missing edits read as a restore that worked.
//
// These drive the registered Cmd+Q handler and the frame guard the close button
// consults, answering through the askQuitDirtyEditors seam: a real ShowModal
// attaches a GLFW window, which segfaults off the main thread (see
// editor_tab_close_test.go for the same arrangement).

// answerQuitDirty makes the quit prompt answer `r` for the rest of the test and
// hands back the paths it was asked about, one entry per time it was put up.
func answerQuitDirty(t *testing.T, r gui.DialogResult) *[][]string {
	t.Helper()
	var asked [][]string
	saved := askQuitDirtyEditors
	askQuitDirtyEditors = func(paths []string) gui.DialogResult {
		asked = append(asked, paths)
		return r
	}
	t.Cleanup(func() { askQuitDirtyEditors = saved })
	return &asked
}

// recordQuit swaps the real core.Quit out and reports how many times the IDE
// asked to end the event loop. Whether the process goes down is the whole
// question these tests ask.
func recordQuit(t *testing.T) *int {
	t.Helper()
	quits := 0
	saved := quitApp
	quitApp = func() { quits++ }
	t.Cleanup(func() { quitApp = saved })
	return &quits
}

// cmdQ registers the IDE's shortcuts and hands back the handler bound to Cmd+Q,
// so the test presses the binding a real key event reaches.
func cmdQ(t *testing.T, tabs *gui.TabWidget) func() {
	t.Helper()
	registerShortcuts(tabs, nil)
	press := gui.ShortcutHandler(gui.ModAction, 'Q')
	if press == nil {
		t.Fatal("registerShortcuts left Cmd+Q unbound")
	}
	return press
}

// openSecondEditedTab opens another file into an already-wired TabWidget and
// leaves its buffer holding `edited`.
func openSecondEditedTab(t *testing.T, tabs *gui.TabWidget, path, edited string) *gui.CodeEditor {
	t.Helper()
	if !openFileInEditor(tabs, path) {
		t.Fatalf("openFileInEditor returned false for %s", path)
	}
	ed := openEditors[path]
	if ed == nil {
		t.Fatalf("openEditors did not track %s", path)
	}
	ed.SetText(edited)
	return ed
}

// writeFile drops content at dir/name and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	quitOriginal = "package main\n"
	// gofmt-clean already, so "saved" is byte-identical to the buffer and the
	// disk comparison stays exact.
	quitEditedA = "package main\n\nfunc unsavedA() {}\n"
	quitEditedB = "package main\n\nfunc unsavedB() {}\n"
)

// TestCmdQCancelKeepsTheAppAndEveryUnsavedBuffer: the branch that used to lose
// the work. Cancel has to leave the process running with every buffer, tab and
// file exactly where the user left them — and the prompt has to name the files
// that are actually at stake, since that is what the user answers on.
func TestCmdQCancelKeepsTheAppAndEveryUnsavedBuffer(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFile(t, dir, "a.go", quitOriginal)
	pathB := writeFile(t, dir, "b.go", quitOriginal)
	pathClean := writeFile(t, dir, "clean.go", quitOriginal)

	tabs, edA, _ := openEditedTab(t, pathA, quitEditedA)
	edB := openSecondEditedTab(t, tabs, pathB, quitEditedB)
	edClean := openSecondEditedTab(t, tabs, pathClean, quitOriginal)

	asked := answerQuitDirty(t, gui.DialogCancel)
	quits := recordQuit(t)

	cmdQ(t, tabs)()

	if *quits != 0 {
		t.Errorf("Cmd+Q quit %d times after the prompt was cancelled", *quits)
	}
	if want := [][]string{{pathA, pathB}}; !reflect.DeepEqual(*asked, want) {
		t.Errorf("prompt asked about %v; want one prompt naming exactly the drifted files %v", *asked, want)
	}
	if got := edA.Text(); got != quitEditedA {
		t.Errorf("buffer for a.go = %q; want the unsaved edit %q", got, quitEditedA)
	}
	if got := edB.Text(); got != quitEditedB {
		t.Errorf("buffer for b.go = %q; want the unsaved edit %q", got, quitEditedB)
	}
	if got := edClean.Text(); got != quitOriginal {
		t.Errorf("buffer for clean.go = %q; want %q", got, quitOriginal)
	}
	for _, path := range []string{pathA, pathB, pathClean} {
		if disk := diskText(t, path); disk != quitOriginal {
			t.Errorf("%s = %q; a cancelled quit must not write", filepath.Base(path), disk)
		}
		if openEditors[path] == nil {
			t.Errorf("openEditors stopped tracking %s after a cancelled quit", filepath.Base(path))
		}
	}
}

// TestCmdQSaveAllWritesEveryDirtyBufferBeforeQuitting: "Save all" means all of
// them. A quit that saved only the active tab would still drop the rest, and
// the session restore would reopen them looking clean.
func TestCmdQSaveAllWritesEveryDirtyBufferBeforeQuitting(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFile(t, dir, "a.go", quitOriginal)
	pathB := writeFile(t, dir, "b.go", quitOriginal)

	tabs, _, idxA := openEditedTab(t, pathA, quitEditedA)
	openSecondEditedTab(t, tabs, pathB, quitEditedB)
	// The active tab is A, so B is the one a save-the-focused-editor fix would
	// silently leave behind.
	tabs.SetCurrentIndex(idxA)
	answerQuitDirty(t, gui.DialogOK)
	quits := recordQuit(t)

	cmdQ(t, tabs)()

	if disk := diskText(t, pathA); disk != quitEditedA {
		t.Errorf("a.go = %q; Save All must write %q", disk, quitEditedA)
	}
	if disk := diskText(t, pathB); disk != quitEditedB {
		t.Errorf("b.go = %q; Save All must write the background tab too, %q", disk, quitEditedB)
	}
	if *quits != 1 {
		t.Errorf("quit called %d times after Save All; want 1", *quits)
	}
}

// TestCmdQSaveAllSurvivesATabClosedUnderIt: the prompt runs a nested event
// loop, and shortcuts reach the dialog's own window — so Cmd+S then Cmd+W
// inside it takes one of the listed files out of openEditors before Save All
// walks the list. Saving that hole would panic and take the buffers the prompt
// was protecting down with it.
func TestCmdQSaveAllSurvivesATabClosedUnderIt(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFile(t, dir, "a.go", quitOriginal)
	pathB := writeFile(t, dir, "b.go", quitOriginal)

	tabs, _, _ := openEditedTab(t, pathA, quitEditedA)
	edB := openSecondEditedTab(t, tabs, pathB, quitEditedB)
	quits := recordQuit(t)

	saved := askQuitDirtyEditors
	askQuitDirtyEditors = func([]string) gui.DialogResult {
		// Cmd+S then Cmd+W on b.go while the prompt is up: the save makes the
		// buffer match disk, so the close goes through without a prompt of its
		// own and b.go leaves openEditors.
		if !saveEditorToDisk(edB, pathB) {
			t.Fatal("saving the background tab failed")
		}
		if idx := editorTabIndex(tabs, edB); !tabs.TabBar().CloseTab(idx) {
			t.Fatal("closing the saved background tab failed")
		}
		return gui.DialogOK
	}
	t.Cleanup(func() { askQuitDirtyEditors = saved })

	cmdQ(t, tabs)()

	if disk := diskText(t, pathA); disk != quitEditedA {
		t.Errorf("a.go = %q; Save All must still write the tab that is still open, %q", disk, quitEditedA)
	}
	if disk := diskText(t, pathB); disk != quitEditedB {
		t.Errorf("b.go = %q; want the bytes its own save wrote, %q", disk, quitEditedB)
	}
	if *quits != 1 {
		t.Errorf("quit called %d times after Save All; want 1", *quits)
	}
}

// TestCmdQDiscardQuitsWithoutWriting: Discard is the answer that says "yes, I
// know". It must quit, and it must not write the buffers the user just told it
// to throw away.
func TestCmdQDiscardQuitsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", quitOriginal)

	tabs, _, _ := openEditedTab(t, path, quitEditedA)
	answerQuitDirty(t, gui.DialogNo)
	quits := recordQuit(t)

	cmdQ(t, tabs)()

	if *quits != 1 {
		t.Errorf("quit called %d times after Discard; want 1", *quits)
	}
	if disk := diskText(t, path); disk != quitOriginal {
		t.Errorf("a.go = %q; Discard must not write %q", disk, quitEditedA)
	}
}

// TestCmdQWithNothingUnsavedQuitsSilently: the guard is there to catch lost
// work, not to make every quit cost a dialog.
func TestCmdQWithNothingUnsavedQuitsSilently(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", quitOriginal)

	tabs, _, _ := openEditedTab(t, path, quitOriginal)
	asked := answerQuitDirty(t, gui.DialogCancel)
	quits := recordQuit(t)

	cmdQ(t, tabs)()

	if len(*asked) != 0 {
		t.Errorf("prompted about %v; nothing had drifted from disk", *asked)
	}
	if *quits != 1 {
		t.Errorf("quit called %d times with clean buffers; want 1", *quits)
	}
}

// TestCmdQFailedSaveDoesNotQuit: the requested save can fail — a read-only
// file, a full disk. Quitting anyway destroys exactly the edits the prompt
// offered to save, and the process is gone before the toast can be read.
func TestCmdQFailedSaveDoesNotQuit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through the read-only mode bit, so the save would succeed")
	}
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", quitOriginal)

	tabs, ed, _ := openEditedTab(t, path, quitEditedA)
	// Read-only only after the open, so the buffer still reads as drifted.
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) // else TempDir can't clean up
	answerQuitDirty(t, gui.DialogOK)
	quits := recordQuit(t)

	cmdQ(t, tabs)()

	if *quits != 0 {
		t.Errorf("quit called %d times after the save failed; want 0", *quits)
	}
	if got := ed.Text(); got != quitEditedA {
		t.Errorf("buffer = %q; want the unsaved edit %q", got, quitEditedA)
	}
	if disk := diskText(t, path); disk != quitOriginal {
		t.Errorf("a.go = %q; want the file untouched at %q", disk, quitOriginal)
	}
}

// TestCloseButtonAsksBeforeQuittingOverUnsavedEditors: the red button had no
// dirty check of any kind. It goes through the frame's can-close guard rather
// than the closed callback next to it, because the closed callback runs from
// inside Frame.Close() — after CloseAllViews — where a Cancel would be
// cancelling a close that already happened.
func TestCloseButtonAsksBeforeQuittingOverUnsavedEditors(t *testing.T) {
	t.Run("cancel keeps the window and the work", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "a.go", quitOriginal)
		_, ed, _ := openEditedTab(t, path, quitEditedA)
		asked := answerQuitDirty(t, gui.DialogCancel)

		frame := gui.NewFrame()
		installQuitGuard(frame, nil)

		if frame.CanClose() {
			t.Error("the close button was allowed to quit over an unsaved buffer")
		}
		if len(*asked) != 1 {
			t.Errorf("prompt put up %d times; want 1", len(*asked))
		}
		if got := ed.Text(); got != quitEditedA {
			t.Errorf("buffer = %q; want the unsaved edit %q", got, quitEditedA)
		}
		if disk := diskText(t, path); disk != quitOriginal {
			t.Errorf("a.go = %q; a cancelled close must not write", disk)
		}
	})

	t.Run("save all lands on disk before the window goes", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "a.go", quitOriginal)
		openEditedTab(t, path, quitEditedA)
		answerQuitDirty(t, gui.DialogOK)

		frame := gui.NewFrame()
		installQuitGuard(frame, nil)

		if !frame.CanClose() {
			t.Error("the close button was refused after the prompt was answered with Save All")
		}
		if disk := diskText(t, path); disk != quitEditedA {
			t.Errorf("a.go = %q; Save All must write %q", disk, quitEditedA)
		}
	})

	t.Run("clean buffers close without asking", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "a.go", quitOriginal)
		openEditedTab(t, path, quitOriginal)
		asked := answerQuitDirty(t, gui.DialogCancel)

		frame := gui.NewFrame()
		installQuitGuard(frame, nil)

		if !frame.CanClose() {
			t.Error("the close button was refused with nothing unsaved")
		}
		if len(*asked) != 0 {
			t.Errorf("prompted about %v; nothing had drifted from disk", *asked)
		}
	})
}

// TestMainInstallsTheQuitGuard: everything above installs the guard itself, so
// the whole package stays green with the one line that installs it in the
// running IDE deleted — and the red button is back to ending the process over
// every unsaved buffer. main cannot be called from a test (it opens a window
// and enters the event loop), so this reads the call out of the syntax tree,
// the way problems_source_test.go reads the designer's own main.
//
// The arguments are checked, not just the name: handed a frame other than the
// one the window is built on, the guard protects nothing; handed a nil canvas
// it drops the design half of canQuit and quits over an unsaved .tdoc.
func TestMainInstallsTheQuitGuard(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot parse main.go: %v", err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("main.go has no func main")
	}

	// The two things the guard has to be wired to, named as main names them.
	frame := boundBy(mainFn, "gui.NewFrameWindow", 0)
	if frame == "" {
		t.Fatal("main no longer builds its frame with gui.NewFrameWindow; update this test to match")
	}
	canvas := boundBy(mainFn, "buildPanels", 1)
	if canvas == "" {
		t.Fatal("main no longer takes its design canvas from buildPanels; update this test to match")
	}

	var args []string
	calls := 0
	ast.Inspect(mainFn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "installQuitGuard" {
			return true
		}
		calls++
		args = nil
		for _, a := range call.Args {
			if id, ok := a.(*ast.Ident); ok {
				args = append(args, id.Name)
			} else {
				args = append(args, "?")
			}
		}
		return false
	})

	if calls == 0 {
		t.Fatal("main never calls installQuitGuard; the title bar's close button quits over every unsaved editor and the unsaved design")
	}
	if want := []string{frame, canvas}; !reflect.DeepEqual(args, want) {
		t.Errorf("main calls installQuitGuard(%v), want (%v)", args, want)
	}
}

// boundBy returns the name the i-th result of the call to fn is assigned to
// inside body, or "" when body never calls it. fn is matched on its printed
// form, so both "buildPanels" and "gui.NewFrameWindow" work.
func boundBy(body ast.Node, fn string, i int) (name string) {
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 || i >= len(as.Lhs) {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		var got string
		switch f := call.Fun.(type) {
		case *ast.Ident:
			got = f.Name
		case *ast.SelectorExpr:
			if pkg, ok := f.X.(*ast.Ident); ok {
				got = pkg.Name + "." + f.Sel.Name
			}
		}
		if got != fn {
			return true
		}
		if id, ok := as.Lhs[i].(*ast.Ident); ok {
			name = id.Name
		}
		return false
	})
	return
}

// TestSecondCmdWDoesNotStackASecondPrompt: dispatchShortcut runs from every
// window's key callback including the prompt dialog's own, and ShowModal
// disables the parent chain only — so a second Cmd+W arriving while the prompt
// is up re-entered the close for the same tab and put an identical prompt on
// top of it, one per keystroke.
func TestSecondCmdWDoesNotStackASecondPrompt(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", quitOriginal)
	tabs, ed, idx := openEditedTab(t, path, quitEditedA)
	tabs.SetCurrentIndex(idx)
	press := cmdW(t, tabs)

	asks := 0
	saved := askCloseDirtyEditor
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult {
		asks++
		if asks == 1 {
			press() // the user's second Cmd+W, landing while this prompt is up
		}
		return gui.DialogCancel
	}
	t.Cleanup(func() { askCloseDirtyEditor = saved })

	press()

	if asks != 1 {
		t.Errorf("prompt put up %d times for one tab; the second Cmd+W stacked another", asks)
	}
	if editorTabIndex(tabs, ed) < 0 {
		t.Error("the tab went away after a cancelled close")
	}
	if got := ed.Text(); got != quitEditedA {
		t.Errorf("buffer = %q; want the unsaved edit %q", got, quitEditedA)
	}

	// The guard is for the duration of one prompt, not for the session: the
	// next Cmd+W has to ask again or the dirty check is dead after the first.
	press()
	if asks != 2 {
		t.Errorf("prompt put up %d times in total; a later Cmd+W must ask again", asks)
	}
}

// TestSecondCmdQDoesNotStackASecondPrompt: same re-entrancy, one gesture over.
// The second Cmd+Q must not add a prompt, and must not quit behind the one
// still waiting for an answer.
func TestSecondCmdQDoesNotStackASecondPrompt(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", quitOriginal)
	tabs, _, _ := openEditedTab(t, path, quitEditedA)
	press := cmdQ(t, tabs)
	quits := recordQuit(t)

	asks := 0
	saved := askQuitDirtyEditors
	askQuitDirtyEditors = func([]string) gui.DialogResult {
		asks++
		if asks == 1 {
			press() // the user's second Cmd+Q, landing while this prompt is up
		}
		return gui.DialogCancel
	}
	t.Cleanup(func() { askQuitDirtyEditors = saved })

	press()

	if asks != 1 {
		t.Errorf("prompt put up %d times for one quit; the second Cmd+Q stacked another", asks)
	}
	if *quits != 0 {
		t.Errorf("quit called %d times while the prompt was still up; want 0", *quits)
	}

	press()
	if asks != 2 {
		t.Errorf("prompt put up %d times in total; a later Cmd+Q must ask again", asks)
	}
}

package gui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestEditorReadOnlyBlocksDocumentMutations(t *testing.T) {
	commands := map[string]func(*CodeEditor){
		"input":      func(e *CodeEditor) { e.OnTextInput("中🙂\nX") },
		"backspace":  func(e *CodeEditor) { e.onKeyDownWithShift(KeyBackSpace, false, false) },
		"delete-key": func(e *CodeEditor) { e.onKeyDownWithShift(KeyDelete, false, false) },
		"enter":      func(e *CodeEditor) { e.onKeyDownWithShift(KeyEnter, false, false) },
		"tab":        func(e *CodeEditor) { e.onKeyDownWithShift(KeyTab, false, false) },
		"shift-tab":  func(e *CodeEditor) { e.onKeyDownWithShift(KeyTab, false, true) },
		"undo":       (*CodeEditor).Undo, "redo": (*CodeEditor).Redo,
		"indent": (*CodeEditor).IndentLines, "outdent": (*CodeEditor).OutdentLines,
		"indent-selection": (*CodeEditor).IndentSelection, "dedent-selection": (*CodeEditor).DedentSelection,
		"delete-selection":  (*CodeEditor).DeleteSelection,
		"replace-selection": func(e *CodeEditor) { e.ReplaceSelection("new") },
		"replace-all":       func(e *CodeEditor) { e.ReplaceAllText("new") },
		"replace-at":        func(e *CodeEditor) { e.ReplaceAllTextAt("new", 0, 1) },
		"move-up":           (*CodeEditor).MoveLinesUp, "move-down": (*CodeEditor).MoveLinesDown,
		"duplicate": (*CodeEditor).DuplicateLines, "delete-lines": (*CodeEditor).DeleteLines,
		"join": (*CodeEditor).JoinLines, "trim": (*CodeEditor).TrimTrailingWhitespace,
		"comment":        (*CodeEditor).ToggleLineComment,
		"paste-delivery": func(e *CodeEditor) { e.pasteText("late") },
		"cut": func(e *CodeEditor) {
			if err := e.copyOrCut(true, func(string) error { t.Fatal("read-only cut wrote clipboard"); return nil }); err != nil {
				t.Fatal(err)
			}
		},
		"rename": func(e *CodeEditor) {
			if _, _, err := e.RenameSymbolAtCursor("new"); !errors.Is(err, ErrEditorReadOnly) {
				t.Fatal(err)
			}
		},
	}
	for name, run := range commands {
		for _, selection := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/carets", true: "/selection"}[selection], func(t *testing.T) {
				e := NewCodeEditor()
				e.SetFilePath("readonly.txt")
				e.SetText("base")
				e.ReplaceAllText("  first\r\n中🙂 beta  \r\nlast\n")
				e.ReplaceAllText("branch")
				e.Undo()
				e.MarkSaved()
				e.ScrollToLineCol(1, 2)
				e.AddCursorBelow()
				if selection {
					e.SelectAll()
				}
				e.SetReadOnly(true)
				text, rev, sel, cursors := e.Text(), e.TextRevision(), e.selectionSnapshot(), e.allCursors()
				undo, redo := len(e.undoStack), len(e.redoStack)
				changes := 0
				e.SigChanged(func(string) { changes++ })
				run(e)
				if e.Text() != text || e.TextRevision() != rev || !reflect.DeepEqual(e.selectionSnapshot(), sel) || !reflect.DeepEqual(e.allCursors(), cursors) || len(e.undoStack) != undo || len(e.redoStack) != redo || changes != 0 || e.Modified() {
					t.Fatal("blocked command changed document state")
				}
			})
		}
	}
}

func TestEditorReadOnlyHistoryAvailabilityAndHostReload(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a")
	e.ReplaceAllText("b")
	e.ReplaceAllText("c")
	e.Undo()
	e.MarkSaved()
	var states [][2]bool
	stop := e.SubscribeHistoryChanged(func() { states = append(states, [2]bool{e.CanUndo(), e.CanRedo()}) })
	defer stop()
	rev := e.TextRevision()
	e.SetReadOnly(true)
	e.SetReadOnly(true)
	if e.CanUndo() || e.CanRedo() || e.CanCut() || e.CanPaste() || e.CanDeleteLines() || e.CanDuplicateLines() || e.CanMoveLinesDown() || !e.CanCopy() || !e.CanSelectAll() {
		t.Fatal("read-only availability")
	}
	e.Undo()
	e.Redo()
	if e.Text() != "b" || e.TextRevision() != rev || e.Modified() {
		t.Fatal("read-only history mutated")
	}
	e.SetReadOnly(false)
	e.SetReadOnly(false)
	if !reflect.DeepEqual(states, [][2]bool{{false, false}, {true, true}}) {
		t.Fatal(states)
	}
	e.Redo()
	if e.Text() != "c" {
		t.Fatal("lost Redo")
	}
	e.Undo()
	e.Undo()
	if e.Text() != "a" {
		t.Fatal("lost Undo")
	}
	e.SetReadOnly(true)
	e.SetText("host refreshed\n")
	if !e.IsReadOnly() || e.Text() != "host refreshed\n" || len(e.undoStack) != 0 || len(e.redoStack) != 0 {
		t.Fatal("host reload contract")
	}
}

func TestEditorReadOnlyFindNavigationAndMenu(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first\nsecond first\n")
	e.SetReadOnly(true)
	e.onKeyDownWithShift(KeyRight, false, true)
	if e.SelectedText() != "f" {
		t.Fatal("selection disabled")
	}
	e.SelectAll()
	var copied string
	if err := e.copyOrCut(false, func(s string) error { copied = s; return nil }); err != nil || copied != e.Text() {
		t.Fatal("copy blocked", err)
	}
	e.ScrollToLineCol(0, 0)
	e.ShowFind(true)
	e.SetFindQuery("first")
	e.replaceText = "oops"
	e.syncFindBar()
	if e.FindSnapshot().Count != 2 || !e.findBar.next.IsEnabled() || e.findBar.one.IsEnabled() || e.findBar.all.IsEnabled() {
		t.Fatal("find/replace state")
	}
	e.replaceAll()
	e.replaceCurrent()
	if e.Text() != "first\nsecond first\n" {
		t.Fatal("replacement escaped read-only")
	}
	e.findNext()
	if e.CursorLine() != 1 {
		t.Fatal("find navigation blocked")
	}
	e.focusFindInput(false, true)
	e.OnTextInput("second")
	if e.findText != "second" || e.FindSnapshot().Count != 1 {
		t.Fatal("query entry blocked", e.findText)
	}
	e.closeFindBar()
	e.ScrollToLineCol(0, 1)
	for _, label := range []string{"Cut", "Paste", "Rename Symbol"} {
		if findMenuItem(e.contextMenuItems("first"), label).Enabled {
			t.Fatal("enabled edit", label)
		}
	}
	for _, label := range []string{"Copy", "Select All", "Go to Definition", "Find References"} {
		if !findMenuItem(e.contextMenuItems("first"), label).Enabled {
			t.Fatal("disabled reader", label)
		}
	}
	if e.WantsTab(false) {
		t.Fatal("read-only editor traps keyboard focus")
	}
}

func TestEditorReadOnlyCancelsPendingPasteAndFormatAcrossUnlock(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("original")
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	e.Paste()
	e.SetReadOnly(true)
	e.SetReadOnly(false)
	r.done("late", nil)
	if e.Text() != "original" || e.PastePending() || r.ctx.Err() == nil {
		t.Fatal("late paste escaped lock/unlock")
	}
	e.SetReadOnly(true)
	e.Paste()
	if r.calls != 1 {
		t.Fatal("read-only clipboard read")
	}
	f, posts := formatEditor(t, "json", `{"x":1}`)
	if err := f.FormatDocumentAsync(EditorFormatOptions{}, func(error) { t.Error("retired formatter callback") }); err != nil {
		t.Fatal(err)
	}
	deliver := waitCompletionPost(t, posts)
	f.SetReadOnly(true)
	f.SetReadOnly(false)
	deliver()
	if f.Text() != `{"x":1}` || f.CanUndo() || f.Formatting() {
		t.Fatal("late format escaped lock/unlock")
	}
	f.SetReadOnly(true)
	if err := f.FormatDocument(EditorFormatOptions{}); !errors.Is(err, ErrEditorReadOnly) {
		t.Fatal(err)
	}
	if err := f.FormatDocumentAsync(EditorFormatOptions{}, nil); !errors.Is(err, ErrEditorReadOnly) {
		t.Fatal(err)
	}
}

func TestEditorReadOnlyCancelsAsyncReplacementAcrossUnlock(t *testing.T) {
	e, posts := asyncFindEditor(t)
	original := e.Text()
	e.findText = "foo"
	e.replaceText = "bar"
	e.requestFind(true)
	deliver := waitCompletionPost(t, posts)
	waitFindWorker(t, e.findState.worker)
	e.SetReadOnly(true)
	e.SetReadOnly(false)
	deliver()
	if e.Text() != original || e.CanUndo() {
		t.Fatal("late replacement escaped lock/unlock")
	}
	waitCompletionPost(t, posts)()
	if e.FindSnapshot().Count != 5000 {
		t.Fatal("search did not recover")
	}
}

func TestEditorReadOnlyCancelsCompletionAndRename(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first\nsecond")
	e.cursorCol = 2
	e.SetExternalCompletions([]ExternalCompletion{{Label: "external", Insert: "external"}})
	e.TriggerCompletion()
	// The shared rename command now opens a validated modal, rather than an
	// inline editor. Simulate a permission change while its nested loop runs;
	// never create native windows from the unit-test goroutine.
	previousPrompt := showRenameSymbolInput
	t.Cleanup(func() { showRenameSymbolInput = previousPrompt })
	prompts := 0
	showRenameSymbolInput = func(parent IWidget, icon paint.Icon, title, label, def string, validator Validator) (string, bool) {
		prompts++
		if parent != e || def != "first" || validator == nil {
			t.Fatal("unexpected rename prompt")
		}
		e.SetReadOnly(true)
		return "renamed", true
	}
	e.activateRename()
	if prompts != 1 || e.Text() != "first\nsecond" || e.CanUndo() {
		t.Fatal("rename result escaped a read-only transition")
	}
	if e.CompletionVisible() || len(e.externalCompletions) != 0 || e.renameActive || e.snippetSess != nil {
		t.Fatal("editing overlay remained active")
	}
	calls := 0
	e.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: func(*CodeEditor) { calls++ }})
	e.RequestRenameSymbol()
	e.SetExternalCompletions([]ExternalCompletion{{Label: "late", Insert: "late"}})
	e.TriggerCompletion()
	p := &CompletionPopup{visible: true, filtered: []CompletionItem{{Text: "late", apply: &completionApply{run: func(*CodeEditor) { calls++ }}}}}
	p.Accept(e)
	if calls != 0 || e.CompletionVisible() || len(e.externalCompletions) != 0 || e.Text() != "first\nsecond" {
		t.Fatal("read-only completion/rename")
	}
}

func TestEditorReadOnlyCutRechecksPermissionAfterNativeWrite(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("keep\nnext")
	e.SelectAll()
	err := e.copyOrCut(true, func(string) error { e.SetReadOnly(true); return nil })
	if err != nil || e.Text() != "keep\nnext" || e.CanUndo() {
		t.Fatal("permission changed while clipboard was writing")
	}
}

func TestEditorReadOnlyQueriesAllocateNothingOnLargeSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("line\n", 100000))
	e.SelectAll()
	e.SetReadOnly(true)
	if n := testing.AllocsPerRun(100, func() {
		_ = e.IsReadOnly()
		_ = e.CanCopy()
		_ = e.CanCut()
		_ = e.CanPaste()
		_ = e.CanUndo()
		_ = e.CanRedo()
		_ = e.CanDeleteLines()
		_ = e.CanDuplicateLines()
		_ = e.CanMoveLinesUp()
		e.SetReadOnly(true)
	}); n != 0 {
		t.Fatal(n)
	}
}

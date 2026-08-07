package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
)

// The three tests below all drive tabs.TabBar().CloseTab(idx) — the exact
// call TabBar's OnLeftUp makes when the X button is released, and the one
// OnMiddleUp funnels into — on a TabWidget wired by buildEditorTabs, so they
// exercise the production close path rather than closeEditorTab directly.
//
// They answer the prompt through the askCloseDirtyEditor seam: a real
// ShowModal attaches a GLFW window, which segfaults off the main thread the
// same way the SaveFileDialog does (see feedback_test.go).

// answerCloseDirty makes the close prompt answer `r` for the rest of the test
// and reports whether it was ever asked.
func answerCloseDirty(t *testing.T, r gui.DialogResult) *bool {
	t.Helper()
	asked := false
	saved := askCloseDirtyEditor
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult {
		asked = true
		return r
	}
	t.Cleanup(func() { askCloseDirtyEditor = saved })
	return &asked
}

// openEditedTab opens path in a freshly-wired editor TabWidget, replaces the
// buffer with `edited`, and returns the tabs, the editor and its tab index.
// Restores the package-global openEditors when the test ends so the tests stay
// order-independent.
func openEditedTab(t *testing.T, path, edited string) (*gui.TabWidget, *gui.CodeEditor, int) {
	t.Helper()
	saved := openEditors
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() { openEditors = saved })

	tabs := buildEditorTabs(nil)
	if !openFileInEditor(tabs, path) {
		t.Fatal("openFileInEditor returned false")
	}
	ed := openEditors[path]
	if ed == nil {
		t.Fatal("openEditors did not track the opened file")
	}
	ed.SetText(edited)
	idx := editorTabIndex(tabs, ed)
	if idx < 0 {
		t.Fatal("opened editor has no tab")
	}
	return tabs, ed, idx
}

// editorTabIndex returns the tab index whose page is ed, or -1.
func editorTabIndex(tabs *gui.TabWidget, ed *gui.CodeEditor) int {
	stack := tabs.Stack()
	for i := 0; i < tabs.Count(); i++ {
		if stack.Page(i) == gui.IWidget(ed) {
			return i
		}
	}
	return -1
}

// TestCloseEditorTabCancelKeepsUnsavedEdits: closing a tab whose buffer has
// drifted from disk must not throw the edits away. This is the branch that
// used to lose work silently — after a cancelled close the tab, the buffer
// and the openEditors entry all survive and the file on disk is untouched.
func TestCloseEditorTabCancelKeepsUnsavedEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cancel.go")
	const original = "package main\n"
	const edited = "package main\n\nfunc unsavedWork() {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	tabs, ed, idx := openEditedTab(t, path, edited)
	answerCloseDirty(t, gui.DialogCancel)

	if tabs.TabBar().CloseTab(idx) {
		t.Error("CloseTab reported the tab closed after a cancelled prompt")
	}
	if editorTabIndex(tabs, ed) < 0 {
		t.Error("cancelled close removed the tab anyway")
	}
	if got := ed.Text(); got != edited {
		t.Errorf("editor buffer = %q; want the unsaved edit %q", got, edited)
	}
	if openEditors[path] != ed {
		t.Errorf("openEditors[%q] = %v; cancelled close untracked the editor", path, openEditors[path])
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != original {
		t.Errorf("disk = %q; a cancelled close must not write %q", string(disk), edited)
	}
}

// TestCloseEditorTabSaveAndDiscard: the two answers that let the close go
// through. Save must land the edited bytes on disk before the tab goes away;
// Discard closes and leaves the file as it was. Both must untrack the editor,
// same as before the prompt existed.
func TestCloseEditorTabSaveAndDiscard(t *testing.T) {
	const original = "package main\n"
	// gofmt-clean already, so "saved verbatim" and "saved formatted" are the
	// same bytes and the comparison stays exact.
	const edited = "package main\n\nfunc unsavedWork() {}\n"

	cases := []struct {
		name     string
		answer   gui.DialogResult
		wantDisk string
	}{
		{"save", gui.DialogOK, edited},
		{"discard", gui.DialogNo, original},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "answer.go")
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			tabs, ed, idx := openEditedTab(t, path, edited)
			answerCloseDirty(t, c.answer)

			if !tabs.TabBar().CloseTab(idx) {
				t.Fatal("CloseTab refused to close after the prompt was answered")
			}
			if editorTabIndex(tabs, ed) >= 0 {
				t.Error("tab survived an answered close")
			}
			if openEditors[path] != nil {
				t.Errorf("openEditors still tracks %q after close", path)
			}
			disk, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(disk) != c.wantDisk {
				t.Errorf("disk = %q; want %q", string(disk), c.wantDisk)
			}
		})
	}
}

// TestCloseEditorTabCleanBufferClosesSilently: a buffer that still matches the
// file has nothing to lose, so the close must go through without asking — the
// prompt is a guard, not a toll booth on every tab.
func TestCloseEditorTabCleanBufferClosesSilently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.go")
	const original = "package main\n\nfunc clean() {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	tabs, ed, idx := openEditedTab(t, path, original)
	asked := answerCloseDirty(t, gui.DialogCancel)

	if !tabs.TabBar().CloseTab(idx) {
		t.Fatal("CloseTab refused to close a clean tab")
	}
	if *asked {
		t.Error("a clean buffer was prompted about")
	}
	if editorTabIndex(tabs, ed) >= 0 {
		t.Error("clean tab survived close")
	}
	if openEditors[path] != nil {
		t.Errorf("openEditors still tracks %q after close", path)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != original {
		t.Errorf("disk = %q; closing a clean tab must not rewrite the file", string(disk))
	}
}

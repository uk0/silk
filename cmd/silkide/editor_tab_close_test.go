package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
)

// The tests below drive tabs.TabBar().CloseTab(idx) — the exact call TabBar's
// OnLeftUp makes when the X button is released, and the one OnMiddleUp funnels
// into — on a TabWidget wired by buildEditorTabs, so they exercise the
// production close path rather than closeEditorTab directly. The Cmd+W tests
// go one step further out and fire the registered shortcut handler itself.
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

// diskText reads path or fails the test. These tests compare against the bytes
// on disk constantly — that is where a dropped buffer actually shows up.
func diskText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// cmdW registers the IDE's shortcuts against tabs and hands back the handler
// bound to Cmd+W, so the test presses the binding a real key event reaches
// instead of a copy of its body.
func cmdW(t *testing.T, tabs *gui.TabWidget) func() {
	t.Helper()
	registerShortcuts(tabs, nil)
	press := gui.ShortcutHandler(gui.ModAction, 'W')
	if press == nil {
		t.Fatal("registerShortcuts left Cmd+W unbound")
	}
	return press
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

// TestCmdWClosesThroughTheDirtyGuard: Cmd+W is the most-used close gesture on
// the primary platform, and it used to call RemoveTab straight — the tab
// vanished with no prompt, still listed in openEditors and still open as far as
// gopls knew. It has to land on the same guard the X button does.
func TestCmdWClosesThroughTheDirtyGuard(t *testing.T) {
	const original = "package main\n"
	// gofmt-clean already, so "saved" is byte-identical to the buffer.
	const edited = "package main\n\nfunc unsavedWork() {}\n"

	t.Run("cancel keeps the tab and the work", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "cmdw.go")
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		tabs, ed, idx := openEditedTab(t, path, edited)
		tabs.SetCurrentIndex(idx)
		asked := answerCloseDirty(t, gui.DialogCancel)

		cmdW(t, tabs)()

		if !*asked {
			t.Error("Cmd+W closed a dirty tab without asking about the unsaved buffer")
		}
		if editorTabIndex(tabs, ed) < 0 {
			t.Error("Cmd+W removed the tab after a cancelled prompt")
		}
		if got := ed.Text(); got != edited {
			t.Errorf("editor buffer = %q; want the unsaved edit %q", got, edited)
		}
		if openEditors[path] != ed {
			t.Errorf("openEditors[%q] = %v; a cancelled Cmd+W untracked the editor", path, openEditors[path])
		}
		if disk := diskText(t, path); disk != original {
			t.Errorf("disk = %q; a cancelled Cmd+W must not write %q", disk, edited)
		}
	})

	t.Run("save lands on disk before the tab goes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "cmdw.go")
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		tabs, ed, idx := openEditedTab(t, path, edited)
		tabs.SetCurrentIndex(idx)
		answerCloseDirty(t, gui.DialogOK)

		cmdW(t, tabs)()

		if disk := diskText(t, path); disk != edited {
			t.Errorf("disk = %q; Cmd+W answered with Save must write %q", disk, edited)
		}
		if editorTabIndex(tabs, ed) >= 0 {
			t.Error("tab survived an answered Cmd+W")
		}
		if openEditors[path] != nil {
			t.Errorf("openEditors still tracks %q after Cmd+W", path)
		}
	})
}

// TestCmdWClosesTheCurrentTabWhenItIsTheFirstOfSeveral: two things the tests
// above cannot see, because openEditedTab opens behind buildEditorTabs' three
// mockup seeds and openFileInEditor always appends.
//
// The current tab is therefore never index 0 there, and it is never anything
// but the rightmost one — so "close the current tab" and "close the last tab"
// are the same tab in every one of them, and the leftmost tab (the one you get
// after closing the mockup seeds, which is the state the IDE settles into) is
// never the one closed. Here the two real files are the only tabs left, the
// first is current, and the second has to come through untouched.
func TestCmdWClosesTheCurrentTabWhenItIsTheFirstOfSeveral(t *testing.T) {
	const originalFirst = "package main\n\nfunc first() {}\n"
	const editedFirst = "package main\n\nfunc firstEdited() {}\n"
	const originalSecond = "package main\n\nfunc second() {}\n"
	const editedSecond = "package main\n\nfunc secondEdited() {}\n"

	dir := t.TempDir()
	pathFirst := filepath.Join(dir, "first.go")
	pathSecond := filepath.Join(dir, "second.go")
	if err := os.WriteFile(pathFirst, []byte(originalFirst), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathSecond, []byte(originalSecond), 0o644); err != nil {
		t.Fatal(err)
	}
	tabs, edFirst, _ := openEditedTab(t, pathFirst, editedFirst)
	if !openFileInEditor(tabs, pathSecond) {
		t.Fatal("openFileInEditor returned false for the second file")
	}
	edSecond := openEditors[pathSecond]
	if edSecond == nil {
		t.Fatal("openEditors did not track the second file")
	}
	edSecond.SetText(editedSecond)

	// Close the three sample seeds the way a user does — through the X button —
	// so the real files sit at 0 and 1. They carry no path, so they close
	// without a prompt.
	for tabs.Count() > 2 {
		if !tabs.TabBar().CloseTab(0) {
			t.Fatalf("a seeded sample tab refused to close (%d tabs left)", tabs.Count())
		}
	}
	if editorTabIndex(tabs, edFirst) != 0 || editorTabIndex(tabs, edSecond) != 1 {
		t.Fatalf("tabs are at %d and %d, want the two opened files at 0 and 1",
			editorTabIndex(tabs, edFirst), editorTabIndex(tabs, edSecond))
	}
	tabs.SetCurrentIndex(0)
	if tabs.CurrentIndex() != 0 {
		t.Fatalf("CurrentIndex = %d, want 0 — the closing tab has to be the first one", tabs.CurrentIndex())
	}
	answerCloseDirty(t, gui.DialogOK)

	cmdW(t, tabs)()

	if disk := diskText(t, pathFirst); disk != editedFirst {
		t.Errorf("first file = %q; Cmd+W on tab 0 answered with Save must write %q", disk, editedFirst)
	}
	if editorTabIndex(tabs, edFirst) >= 0 {
		t.Error("Cmd+W left the current tab open")
	}
	if openEditors[pathFirst] != nil {
		t.Errorf("openEditors still tracks %q after Cmd+W", pathFirst)
	}
	// The tab that was not asked about keeps its unsaved buffer, its entry and
	// its place. Closing the wrong tab is the other half of the same bug.
	if editorTabIndex(tabs, edSecond) < 0 {
		t.Error("Cmd+W on the first tab closed the second one instead")
	}
	if got := edSecond.Text(); got != editedSecond {
		t.Errorf("second editor buffer = %q; want its untouched unsaved edit %q", got, editedSecond)
	}
	if openEditors[pathSecond] != edSecond {
		t.Errorf("openEditors[%q] = %v; closing another tab untracked it", pathSecond, openEditors[pathSecond])
	}
	if disk := diskText(t, pathSecond); disk != originalSecond {
		t.Errorf("second file = %q; closing another tab must not write its buffer %q", disk, editedSecond)
	}
}

// TestCloseEditorTabSavesTheClosingTabNotTheActiveOne: a middle-click close
// never activates the tab it closes, which is why the save takes an editor and
// a path instead of reading the active tab. Closing a background tab must write
// that tab's buffer — and must leave the active tab's own unsaved buffer where
// the user left it.
func TestCloseEditorTabSavesTheClosingTabNotTheActiveOne(t *testing.T) {
	const originalA = "package main\n\nfunc a() {}\n"
	const editedA = "package main\n\nfunc aEdited() {}\n"
	const originalB = "package main\n\nfunc b() {}\n"
	const editedB = "package main\n\nfunc bEdited() {}\n"

	dir := t.TempDir()
	pathA := filepath.Join(dir, "active.go")
	pathB := filepath.Join(dir, "background.go")
	if err := os.WriteFile(pathA, []byte(originalA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte(originalB), 0o644); err != nil {
		t.Fatal(err)
	}
	tabs, edA, idxA := openEditedTab(t, pathA, editedA)
	// The second dirty file shares the TabWidget; it is the one being closed.
	if !openFileInEditor(tabs, pathB) {
		t.Fatal("openFileInEditor returned false for the background file")
	}
	edB := openEditors[pathB]
	if edB == nil {
		t.Fatal("openEditors did not track the background file")
	}
	edB.SetText(editedB)
	idxB := editorTabIndex(tabs, edB)
	if idxB < 0 {
		t.Fatal("background editor has no tab")
	}
	tabs.SetCurrentIndex(idxA)
	if tabs.CurrentIndex() != idxA {
		t.Fatalf("CurrentIndex = %d, want %d — the closing tab has to be the non-active one", tabs.CurrentIndex(), idxA)
	}
	answerCloseDirty(t, gui.DialogOK)

	if !tabs.TabBar().CloseTab(idxB) {
		t.Fatal("CloseTab refused to close the background tab")
	}
	if disk := diskText(t, pathB); disk != editedB {
		t.Errorf("background file = %q; Save must write the closing tab's buffer %q", disk, editedB)
	}
	if disk := diskText(t, pathA); disk != originalA {
		t.Errorf("active file = %q; closing another tab must not write the active tab's buffer %q", disk, editedA)
	}
	if openEditors[pathB] != nil {
		t.Errorf("openEditors still tracks %q after close", pathB)
	}
	if openEditors[pathA] != edA {
		t.Errorf("openEditors[%q] = %v; closing the background tab untracked the active editor", pathA, openEditors[pathA])
	}
	if editorTabIndex(tabs, edA) < 0 {
		t.Error("closing the background tab removed the active tab")
	}
}

// TestCloseEditorTabFailedSaveKeepsTheBuffer: the requested save can fail — a
// read-only file, a full disk, a directory that went away. The tab holds the
// only copy of that work, so a failed write has to leave everything standing;
// closing anyway destroys exactly the edits the prompt offered to save.
func TestCloseEditorTabFailedSaveKeepsTheBuffer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through the read-only mode bit, so the save would succeed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.go")
	const original = "package main\n"
	const edited = "package main\n\nfunc unsavedWork() {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	tabs, ed, idx := openEditedTab(t, path, edited)
	// Read-only only after the open, so the editor still holds the file's
	// content and the buffer still reads as drifted from disk.
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) // else TempDir can't clean up
	answerCloseDirty(t, gui.DialogOK)

	if tabs.TabBar().CloseTab(idx) {
		t.Error("CloseTab reported the tab closed after the save failed")
	}
	if editorTabIndex(tabs, ed) < 0 {
		t.Error("a failed save still removed the tab, taking the buffer with it")
	}
	if got := ed.Text(); got != edited {
		t.Errorf("editor buffer = %q; want the unsaved edit %q", got, edited)
	}
	if openEditors[path] != ed {
		t.Errorf("openEditors[%q] = %v; a failed save untracked the editor", path, openEditors[path])
	}
	if disk := diskText(t, path); disk != original {
		t.Errorf("disk = %q; want the file untouched at %q", disk, original)
	}
}

package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func tabContext(t *testing.T, tabs *gui.TabWidget, index int) *gui.Menu {
	t.Helper()
	m := gui.NewPopupMenu()
	buildEditorTabContextMenu(m, tabs, index)
	for _, id := range []string{"tab.save", "tab.close", "tab.reopen", "tab.copyPath", "tab.reveal"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil || gui.GetToolTip(b) != "" {
			t.Fatal("missing entry/icon or globally retained tooltip", id)
		}
	}
	return m
}

func TestEditorTabContextTargetsInactiveIdentityAndRename(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a, b := writeFile(t, dir, "a.txt", "alpha"), writeFile(t, dir, "b.txt", "beta")
	openFileInEditor(w.tabs, a)
	ea := activeEditor(w.tabs)
	openFileInEditor(w.tabs, b)
	eb := activeEditor(w.tabs)
	focusEditorTab(w.tabs, ea)
	eb.ReplaceAllText("edited beta")
	m := tabContext(t, w.tabs, 1)
	menuCommandButton(m, "tab.save").Action().Trigger(nil)
	if diskText(t, b) != "edited beta" || diskText(t, a) != "alpha" || activeEditor(w.tabs) != ea {
		t.Fatal("saved/activated wrong target")
	}
	copyOld := copyEditorTabPath
	t.Cleanup(func() { copyEditorTabPath = copyOld })
	copied := ""
	copyEditorTabPath = func(p string) error { copied = p; return nil }
	// Reindex and rename while popup actions still refer to the original object.
	if !closeEditorTab(w.tabs, 0) {
		t.Fatal("remove earlier tab")
	}
	renamed := filepath.Join(dir, "renamed.txt")
	delete(openEditors, b)
	openEditors[renamed] = eb
	eb.SetFilePath(renamed)
	menuCommandButton(m, "tab.copyPath").Action().Trigger(nil)
	if copied != renamed {
		t.Fatal("stale slot/path copied", copied)
	}
	menuCommandButton(m, "tab.save").Action().Trigger(nil)
	if diskText(t, renamed) != "edited beta" {
		t.Fatal("renamed target not saved")
	}
	menuCommandButton(m, "tab.close").Action().Trigger(nil)
	if w.tabs.Count() != 0 || openEditors[renamed] != nil {
		t.Fatal("wrong renamed page closed")
	}
	openFileInEditor(w.tabs, b)
	replacement := activeEditor(w.tabs)
	replacement.ReplaceAllText("replacement unsaved")
	copied = ""
	for _, id := range []string{"tab.save", "tab.copyPath", "tab.close"} {
		menuCommandButton(m, id).Action().Trigger(nil)
	}
	if copied != "" || activeEditor(w.tabs) != replacement || replacement.Text() != "replacement unsaved" || diskText(t, b) != "edited beta" {
		t.Fatal("stale action affected replacement")
	}
}

func TestEditorTabContextCloseGuardAndReopen(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogCancel, gui.DialogOK, gui.DialogNo} {
		t.Run(string(rune('A'+choice)), func(t *testing.T) {
			w := testWorkbench(t)
			p := writeFile(t, t.TempDir(), "a.txt", "saved")
			openFileInEditor(w.tabs, p)
			ed := activeEditor(w.tabs)
			ed.ReplaceAllText("unsaved")
			m := tabContext(t, w.tabs, 0)
			answerCloseDirty(t, choice)
			menuCommandButton(m, "tab.close").Action().Trigger(nil)
			if choice == gui.DialogCancel {
				if w.tabs.Count() != 1 || ed.Text() != "unsaved" || diskText(t, p) != "saved" {
					t.Fatal("cancel lost data")
				}
				return
			}
			if w.tabs.Count() != 0 || !canReopenEditor(w.tabs) {
				t.Fatal("close missed cleanup/history")
			}
			if !reopenClosedEditor(w.tabs) {
				t.Fatal("reopen")
			}
			want := "saved"
			if choice == gui.DialogOK {
				want = "unsaved"
			}
			if activeEditor(w.tabs).Text() != want || diskText(t, p) != want {
				t.Fatal("close decision lost")
			}
		})
	}
}

func TestEditorTabContextRevealAndCopyError(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	p := writeFile(t, root, "a.txt", "a")
	openFileInEditor(w.tabs, p)
	old := globalFileExplorer
	globalFileExplorer = ged.NewFileExplorer()
	t.Cleanup(func() { globalFileExplorer.SetRootDir(""); globalFileExplorer = old })
	globalFileExplorer.SetRootDir(root)
	m := tabContext(t, w.tabs, 0)
	if !menuCommandButton(m, "tab.reveal").IsEnabled() {
		t.Fatal("project file disabled")
	}
	revealOld := revealEditorTabPath
	t.Cleanup(func() { revealEditorTabPath = revealOld })
	revealed := ""
	revealEditorTabPath = func(path string) bool { revealed = path; return true }
	menuCommandButton(m, "tab.reveal").Action().Trigger(nil)
	if revealed != p {
		t.Fatal("wrong file revealed")
	}
	if canRevealEditorTab(filepath.Join(filepath.Dir(root), "outside.txt")) {
		t.Fatal("outside project enabled")
	}
	copyOld := copyEditorTabPath
	toastOld := silkideToast
	t.Cleanup(func() { copyEditorTabPath = copyOld; silkideToast = toastOld })
	copyEditorTabPath = func(string) error { return errors.New("clipboard unavailable") }
	reported := false
	silkideToast = func(_ string, kind gui.ToastLevel) { reported = kind == gui.ToastError }
	menuCommandButton(m, "tab.copyPath").Action().Trigger(nil)
	if !reported {
		t.Fatal("copy failure swallowed")
	}
}

func TestEditorTabContextInvalidAndUntrackedPages(t *testing.T) {
	tabs := gui.NewTabWidget()
	menu := gui.NewPopupMenu()
	for _, index := range []int{-1, 0, 9} {
		buildEditorTabContextMenu(menu, tabs, index)
	}
	tabs.AddTab(gui.NewLabel("display"), "Display", nil)
	buildEditorTabContextMenu(menu, tabs, 0)
	if len(menu.Items()) != 0 {
		t.Fatal("non-editor target got commands")
	}
	ed := gui.NewCodeEditor()
	ed.SetText("not tracked; keep this content")
	tabs.AddTab(ed, "Transient", nil)
	m := tabContext(t, tabs, 1)
	for _, id := range []string{"tab.save", "tab.close", "tab.copyPath", "tab.reveal"} {
		if menuCommandButton(m, id).IsEnabled() {
			t.Fatal("unsafe untracked command enabled", id)
		}
	}
}

func BenchmarkEditorTabContextMenu(b *testing.B) {
	old := openEditors
	defer func() { openEditors = old }()
	tabs := gui.NewTabWidget()
	ed := gui.NewCodeEditor()
	path := filepath.Join(b.TempDir(), "buffer.txt")
	ed.SetFilePath(path)
	ed.SetText("buffer")
	tabs.AddTab(ed, "buffer.txt", nil)
	openEditors = map[string]*gui.CodeEditor{path: ed}
	menu := gui.NewPopupMenu()
	buildEditorTabContextMenu(menu, tabs, 0)
	menu.Clear()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildEditorTabContextMenu(menu, tabs, 0)
		menu.Clear()
	}
}

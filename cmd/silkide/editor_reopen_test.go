package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

func TestReopenEditorEntrypointsRestoreLocationAndFocus(t *testing.T) {
	for _, entry := range []string{"shortcut", "menu", "palette"} {
		t.Run(entry, func(t *testing.T) {
			w := testWorkbench(t)
			w.tabs.SetSize(640, 440)
			path := writeFile(t, t.TempDir(), "unicode.txt", strings.Repeat("a中🙂z\n", 100))
			openFileInEditor(w.tabs, path)
			ed := activeEditor(w.tabs)
			ed.NavigateToLineCol(40, 3)
			pos := ed.NavigationPosition()
			if !w.tabs.TabBar().CloseTab(0) || !canReopenEditor(w.tabs) || openEditors[path] != nil {
				t.Fatal("successful close not recorded/untracked")
			}
			w.activate(modeWelcome) // Must work even when no editor is active.
			switch entry {
			case "shortcut":
				registerShortcuts(w.tabs, w.canvas)
				gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'T')()
				if gui.ShortcutHandler(0, gui.KeyF7) == nil {
					t.Fatal("test shortcut removed")
				}
			case "menu":
				m := buildFileMenu(w.tabs, w.canvas)
				b := menuCommandButton(m, "file.reopen")
				if b == nil || !b.IsEnabled() || b.Icon() == nil {
					t.Fatal("reopen missing/disabled/iconless")
				}
				b.Action().Trigger(nil)
			case "palette":
				old := paletteCommands
				t.Cleanup(func() { paletteCommands = old })
				registerPaletteCommands(w.tabs, w.canvas)
				found := false
				for _, c := range paletteCommands {
					if c.Name == "Reopen Closed Editor" {
						c.Fn()
						found = true
					}
				}
				if !found {
					t.Fatal("palette command missing")
				}
			}
			got := activeEditor(w.tabs)
			if got == nil || got == ed || got.FilePath() != path || got.CursorLine() != pos.Line || got.CursorCol() != pos.Column || !got.HasFocus() || documentMode(w.tabs, w.canvas) != modeEdit {
				t.Fatal("reopen lost identity, caret, workspace or focus")
			}
			if canReopenEditor(w.tabs) || w.tabs.Count() != 1 || got.Modified() {
				t.Fatal("reopen left history/duplicate/dirty editor")
			}
			if got.ScrollY() != pos.ScrollY {
				t.Fatalf("viewport = %v, want %v", got.ScrollY(), pos.ScrollY)
			}
			if menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.reopen").IsEnabled() {
				t.Fatal("empty history menu enabled")
			}
		})
	}
}

func TestReopenEditorRespectsDirtyCloseDecision(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogCancel, gui.DialogOK, gui.DialogNo} {
		t.Run(fmt.Sprint(choice), func(t *testing.T) {
			w := testWorkbench(t)
			path := writeFile(t, t.TempDir(), "a.txt", "saved\n")
			openFileInEditor(w.tabs, path)
			old := activeEditor(w.tabs)
			old.ReplaceAllText("unsaved\n")
			answerCloseDirty(t, choice)
			closed := w.tabs.TabBar().CloseTab(0)
			if choice == gui.DialogCancel {
				if closed || canReopenEditor(w.tabs) || activeEditor(w.tabs) != old || old.Text() != "unsaved\n" {
					t.Fatal("cancel changed state")
				}
				return
			}
			if !closed || !reopenClosedEditor(w.tabs) {
				t.Fatal("close/reopen failed")
			}
			want := "saved\n"
			if choice == gui.DialogOK {
				want = "unsaved\n"
			}
			if activeEditor(w.tabs).Text() != want || diskText(t, path) != want {
				t.Fatal("discarded text resurrected or saved text lost")
			}
		})
	}
}

func TestReopenEditorSaveFailureAndBusyRetainState(t *testing.T) {
	w := renameWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "disk")
	openFileInEditor(w.tabs, path)
	ed := activeEditor(w.tabs)
	ed.ReplaceAllText("buffer")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	answerCloseDirty(t, gui.DialogOK)
	if w.tabs.TabBar().CloseTab(0) || canReopenEditor(w.tabs) || activeEditor(w.tabs) != ed {
		t.Fatal("failed save recorded/closed")
	}
	// A rename reservation must leave the pending reopen location available.
	w.closedEditors.remember(editorSession{Path: path})
	activeFileRename = &editorRename{old: path, next: path + "-new"}
	if reopenClosedEditor(w.tabs) || !canReopenEditor(w.tabs) {
		t.Fatal("busy reopen consumed history")
	}
}

func TestReopenEditorLIFOManualOpenMissingAndDiskChange(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "old")
	b := writeFile(t, dir, "b.txt", "b")
	for _, p := range []string{a, b} {
		openFileInEditor(w.tabs, p)
		w.tabs.TabBar().CloseTab(0)
	}
	if !reopenClosedEditor(w.tabs) || activeEditorPath(w.tabs) != b {
		t.Fatal("not LIFO")
	}
	if err := os.WriteFile(a, []byte("new disk"), 0644); err != nil {
		t.Fatal(err)
	}
	if !reopenClosedEditor(w.tabs) || activeEditor(w.tabs).Text() != "new disk" {
		t.Fatal("reopened stale snapshot")
	}
	w.tabs.TabBar().CloseTab(w.tabs.CurrentIndex())
	openFileInEditor(w.tabs, a)
	if canReopenEditor(w.tabs) {
		t.Fatal("manual open left stale close history")
	}
	w.tabs.TabBar().CloseTab(w.tabs.CurrentIndex())
	w.closedEditors.remember(editorSession{Path: filepath.Join(dir, "missing.txt")})
	var notices []string
	old := silkideToast
	silkideToast = func(s string, _ gui.ToastLevel) { notices = append(notices, s) }
	t.Cleanup(func() { silkideToast = old })
	if reopenClosedEditor(w.tabs) || len(notices) != 1 || !canReopenEditor(w.tabs) {
		t.Fatal("failed path silently opened/blocked history")
	}
	if !reopenClosedEditor(w.tabs) || activeEditorPath(w.tabs) != a {
		t.Fatal("older entry not reachable")
	}
}

func TestReopenEditorRenameAndOwnerIsolation(t *testing.T) {
	w := renameWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "a")
	openFileInEditor(w.tabs, a)
	w.tabs.TabBar().CloseTab(0)
	next := filepath.Join(dir, "renamed.txt")
	if err := os.Rename(a, next); err != nil {
		t.Fatal(err)
	}
	migrateRenamedMetadata(&editorRename{old: a, next: next}, w.tabs, w.canvas)
	if canReopenEditor(buildEditorTabs(nil)) || canReopenEditor(nil) {
		t.Fatal("history leaked to unrelated tabs")
	}
	if !reopenClosedEditor(w.tabs) || activeEditorPath(w.tabs) != next {
		t.Fatal("renamed closed file not remapped")
	}
}

func TestClosedEditorHistoryBoundedUniqueAndFinite(t *testing.T) {
	var h closedEditorHistory
	for i := 0; i < 100; i++ {
		h.remember(editorSession{Path: fmt.Sprint(i), Line: i})
	}
	if len(h.items) != closedEditorLimit || h.items[0].Path != "68" {
		t.Fatal("unbounded history")
	}
	h.remember(editorSession{Path: "75", Line: 200, ScrollY: math.Inf(1)})
	if len(h.items) != closedEditorLimit || h.items[closedEditorLimit-1].Line != 200 || h.items[closedEditorLimit-1].ScrollY != 0 {
		t.Fatal("duplicate/nonfinite location retained")
	}
	h.remember(editorSession{})
	if len(h.items) != closedEditorLimit {
		t.Fatal("non-file page recorded")
	}
}

func TestReopenEditorClampsLocationAfterExternalTruncation(t *testing.T) {
	w := testWorkbench(t)
	w.tabs.SetSize(640, 440)
	path := writeFile(t, t.TempDir(), "truncate.txt", strings.Repeat("long line\n", 100))
	openFileInEditor(w.tabs, path)
	activeEditor(w.tabs).NavigateToLineCol(90, 8)
	w.tabs.TabBar().CloseTab(0)
	if err := os.WriteFile(path, []byte("中🙂"), 0644); err != nil {
		t.Fatal(err)
	}
	if !reopenClosedEditor(w.tabs) {
		t.Fatal("reopen failed")
	}
	ed := activeEditor(w.tabs)
	if ed.CursorLine() != 0 || ed.CursorCol() != 2 || ed.ScrollY() != 0 {
		t.Fatalf("location not clamped to current disk contents: %+v", ed.NavigationPosition())
	}
}

func BenchmarkClosedEditorHistory(b *testing.B) {
	var h closedEditorHistory
	for i := 0; i < closedEditorLimit; i++ {
		h.remember(editorSession{Path: fmt.Sprint(i)})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.remember(editorSession{Path: "0", Line: i})
	}
}

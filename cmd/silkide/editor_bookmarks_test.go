package main

import (
	"context"
	"encoding/json"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/ide/bookmarks"
	"github.com/uk0/silk/settings"
	"path/filepath"
	"slices"
	"testing"
)

func testBookmarkWorkbench(t *testing.T) (*workbench, *editorBookmarks) {
	t.Helper()
	w := testWorkbench(t)
	prefs := &preferences{store: settings.New(filepath.Join(t.TempDir(), "prefs.silkui"))}
	panel := ged.NewBookmarksPanel()
	c := newEditorBookmarks(w.tabs, panel, prefs)
	w.bookmarks = c
	t.Cleanup(func() {
		c.close()
		if err := prefs.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return w, c
}
func TestBookmarkEditorPanelPersistenceAndDetach(t *testing.T) {
	w, c := testBookmarkWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "first\ntarget\nlast")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.NavigateToLineCol(1, 0)
	runBookmarkCommand(w.tabs, w.canvas, "bookmark.toggle", nil)
	if got := c.panel.Bookmarks(); len(got) != 1 || got[0].Line != 2 || !ed.HasBookmark(1) {
		t.Fatal("toggle not unified", got)
	}
	ed.NavigateToLineCol(0, 0)
	ed.OnTextInput("new\n")
	if c.panel.Bookmarks()[0].Line != 3 {
		t.Fatal("panel stayed at stale line")
	}
	ed.Undo()
	if c.panel.Bookmarks()[0].Line != 2 {
		t.Fatal("undo drift")
	}
	ed.Redo()
	if c.panel.Bookmarks()[0].Line != 3 {
		t.Fatal("redo drift")
	}
	ed.Undo()
	flushTestPreferences(t, c.prefs)
	var saved []bookmarks.Bookmark
	if err := json.Unmarshal([]byte(c.prefs.store.String("navigation/bookmarks", "")), &saved); err != nil || len(saved) != 1 || saved[0].Context != "target" {
		t.Fatal(saved, err)
	}
	if !detachEditorTab(w.tabs, w.tabs.Stack().Page(w.tabs.CurrentIndex()), ed, path) {
		t.Fatal("detach")
	}
	ed.ClearBookmarks()
	if c.store.Count() != 1 || len(c.subscriptions) != 0 {
		t.Fatal("detached callback affected persisted marks")
	}
	c.navigate(path, 2)
	restored := activeEditor(w.tabs)
	if restored == ed || restored.CursorLine() != 1 || !slices.Equal(restored.BookmarkLines(), []int{1}) {
		t.Fatal("reopen did not restore mark")
	}
	c.panel.OnKeyDown(gui.KeyDown, false)
	c.panel.OnKeyDown(gui.KeyDelete, false)
	if c.store.Count() != 0 || restored.BookmarkCount() != 0 {
		t.Fatal("panel Delete not unified")
	}
}
func TestBookmarkCrossFileHistoryAndClosedReanchor(t *testing.T) {
	w, c := testBookmarkWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "first\ntarget\nlast")
	b := writeFile(t, dir, "b.txt", "b")
	openFileInEditor(w.tabs, a)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetBookmark(1, true)
	detachEditorTab(w.tabs, w.tabs.Stack().Page(w.tabs.CurrentIndex()), ed, a)
	writeFile(t, dir, "a.txt", "new\nfirst\ntarget\nlast")
	openFileInEditor(w.tabs, b)
	other := activeEditor(w.tabs)
	other.SetBookmark(0, true)
	c.next(other, false)
	if activeEditor(w.tabs).FilePath() != a || activeEditor(w.tabs).CursorLine() != 2 || c.panel.Bookmarks()[0].Line != 3 {
		t.Fatal("closed-file reanchor failed")
	}
	navigateEditorHistory(w.tabs, false)
	if activeEditor(w.tabs) != other {
		t.Fatal("missing navigation history")
	}
	c.next(other, true)
	if activeEditor(w.tabs).FilePath() != a {
		t.Fatal("prev")
	}
}
func TestBookmarkMenuPaletteStaleIdentityAndFocus(t *testing.T) {
	w, c := testBookmarkWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "a\nb")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetReadOnly(true)
	ed.SetFocus()
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	root := menuCommandButton(menu, "editor.bookmarks")
	if root == nil {
		t.Fatal("submenu absent")
	}
	sub := root.SubPopup().(*gui.Menu)
	button := menuCommandButton(sub, "bookmark.toggle")
	if button == nil || button.Icon() == nil || !button.IsEnabled() {
		t.Fatal("toggle missing")
	}
	bound := bindBookmarkCommand(w.tabs, w.canvas, bookmarkCommands()[0])
	button.Action().Trigger(nil)
	if !ed.HasBookmark(0) || c.store.Count() != 1 {
		t.Fatal("menu toggle")
	}
	bound.Fn()
	if ed.BookmarkCount() != 0 {
		t.Fatal("palette toggle")
	}
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "other"))
	other := activeEditor(w.tabs)
	button.Action().Trigger(nil)
	bound.Fn()
	if other.BookmarkCount() != 0 || ed.BookmarkCount() != 0 {
		t.Fatal("stale binding stole target")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	runBookmarkCommand(w.tabs, w.canvas, "bookmark.toggle", nil)
	if other.BookmarkCount() != 0 {
		t.Fatal("shortcut stole focused field")
	}
}
func TestBookmarkStorePathMigrationAndPreferenceReload(t *testing.T) {
	w, c := testBookmarkWorkbench(t)
	dir := t.TempDir()
	prefsPath := filepath.Join(dir, "bookmarks-prefs.silkui")
	c.prefs = &preferences{store: settings.New(prefsPath)}
	t.Cleanup(func() { _ = c.prefs.Shutdown(context.Background()) })
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	c.store.Add(bookmarks.Bookmark{Path: a, Line: 2, Context: "target", Note: "target"})
	c.refresh(true)
	migrateRenamedMetadata(&editorRename{old: a, next: b}, w.tabs, w.canvas)
	if c.panel.Bookmarks()[0].File != b || len(c.store.ByFile(a)) != 0 {
		t.Fatal("migration missed store")
	}
	flushTestPreferences(t, c.prefs)
	loaded := newEditorBookmarks(w.tabs, ged.NewBookmarksPanel(), &preferences{store: settings.New(prefsPath)})
	defer loaded.close()
	if got := loaded.store.List(); len(got) != 1 || got[0].Path != b {
		t.Fatal("preference restore", got)
	}
}

func TestBookmarkCorruptPreferencesAreNotOverwritten(t *testing.T) {
	w := testWorkbench(t)
	p := &preferences{store: settings.NewMemory()}
	const broken = `[{"path":`
	_ = p.store.SetString("navigation/bookmarks", broken)
	c := newEditorBookmarks(w.tabs, ged.NewBookmarksPanel(), p)
	w.bookmarks = c
	t.Cleanup(c.close)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "a\nb"))
	activeEditor(w.tabs).SetBookmark(1, true)
	if !c.saveBlocked || p.store.String("navigation/bookmarks", "") != broken {
		t.Fatal("corrupt source overwritten")
	}
	if c.store.Count() != 1 {
		t.Fatal("live annotations unavailable")
	}
}

package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/ide/bookmarks"
)

// One UI-owned coordinator projects the editor's live row anchors into the
// cross-file store, view and existing coalesced preference writer. No polling,
// disk I/O in Draw, or per-editor worker. Closed files retain their annotations.
type editorBookmarks struct {
	tabs          *gui.TabWidget
	panel         *ged.BookmarksPanel
	prefs         *preferences
	store         *bookmarks.Store
	subscriptions map[*gui.CodeEditor]func()
	saveBlocked   bool
	saveWarned    bool
}

func bookmarkController(tabs *gui.TabWidget) *editorBookmarks {
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		return w.bookmarks
	}
	return nil
}
func newEditorBookmarks(tabs *gui.TabWidget, panel *ged.BookmarksPanel, prefs *preferences) *editorBookmarks {
	c := &editorBookmarks{tabs: tabs, panel: panel, prefs: prefs, store: bookmarks.NewStore(), subscriptions: map[*gui.CodeEditor]func(){}}
	if prefs != nil {
		raw := prefs.store.String("navigation/bookmarks", "")
		var rows []bookmarks.Bookmark
		if raw != "" && (len(raw) > 4<<20 || json.Unmarshal([]byte(raw), &rows) != nil) {
			c.saveBlocked = true
		} else {
			for _, m := range rows {
				if !filepath.IsAbs(m.Path) || len(m.Path) > 4096 || strings.ContainsRune(m.Path, 0) || m.Line < 1 {
					continue
				}
				m.Path = canonicalEditorPath(m.Path)
				m.Context = bookmarkContext(m.Context)
				m.Note = bookmarkLabelForLine(m.Note)
				c.store.Add(m)
			}
		}
	}
	panel.SigActivated(c.navigate)
	panel.SigRemoveRequested(c.remove)
	c.refresh(false)
	return c
}
func bookmarkContext(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 1024 {
		return ""
	} // do not retain or misidentify a truncated huge row
	return strings.Clone(text)
}
func (c *editorBookmarks) bind(ed *gui.CodeEditor) {
	c.detach(ed)
	path := ed.FilePath()
	if path == "" {
		return
	}
	if len(c.store.ByFile(path)) > 0 {
		c.store.Reanchor(path, ed.Lines())
	}
	var lines []int
	for _, m := range c.store.ByFile(path) {
		lines = append(lines, max(0, min(m.Line-1, ed.LineCount()-1)))
	}
	ed.SetBookmarkLines(lines)
	c.subscriptions[ed] = ed.SubscribeBookmarksChanged(func() {
		if openEditors[ed.FilePath()] == ed {
			c.capture(ed)
		}
	})
	c.capture(ed)
	c.refresh(true) // Reanchor may have updated the store before capture.
}
func (c *editorBookmarks) detach(ed *gui.CodeEditor) {
	if stop := c.subscriptions[ed]; stop != nil {
		stop()
		delete(c.subscriptions, ed)
	}
}
func (c *editorBookmarks) close() {
	for ed := range c.subscriptions {
		c.detach(ed)
	}
	c.panel.SigActivated(nil)
	c.panel.SigRemoveRequested(nil)
}
func (c *editorBookmarks) capture(ed *gui.CodeEditor) {
	path := ed.FilePath()
	var rows []bookmarks.Bookmark
	for _, line := range ed.BookmarkLines() {
		text, ok := ed.LineText(line)
		if !ok {
			continue
		}
		rows = append(rows, bookmarks.Bookmark{Path: path, Line: line + 1, Context: bookmarkContext(text), Note: bookmarkLabelForLine(text)})
	}
	if slices.Equal(c.store.ByFile(path), rows) {
		return
	}
	c.store.ReplaceFile(path, rows)
	c.refresh(true)
}
func (c *editorBookmarks) refresh(persist bool) {
	rows := c.store.List()
	panel := make([]ged.Bookmark, 0, len(rows))
	for _, m := range rows {
		panel = append(panel, ged.Bookmark{File: m.Path, Line: m.Line, Label: m.Note})
	}
	c.panel.SetBookmarks(panel)
	if persist && c.prefs != nil {
		raw, err := json.Marshal(rows)
		if c.saveBlocked || len(raw) > 4<<20 {
			if !c.saveWarned {
				c.saveWarned = true
				silkideToast(i18n.T("Bookmarks were not saved; existing data was preserved"), gui.ToastWarning)
			}
			return
		}
		if err == nil && c.prefs.store.String("navigation/bookmarks", "") != string(raw) {
			_ = c.prefs.store.SetString("navigation/bookmarks", string(raw))
			_ = c.prefs.requestSave()
		}
	}
}
func (c *editorBookmarks) remove(path string, line int) {
	if ed := openEditors[path]; ed != nil && ed.HasBookmark(line-1) {
		ed.SetBookmark(line-1, false)
		return
	}
	if c.store.Remove(path, line) {
		c.refresh(true)
	}
}
func (c *editorBookmarks) navigate(path string, line int) {
	var context string
	for _, m := range c.store.ByFile(path) {
		if m.Line == line {
			context = m.Context
			break
		}
	}
	from := editorNavigationPosition(c.tabs)
	if !openFileInEditor(c.tabs, path) {
		silkideToast(i18n.T("Could not open bookmarked file"), gui.ToastWarning)
		return
	}
	ed := activeEditor(c.tabs)
	if ed == nil {
		return
	}
	// Opening a closed file may reanchor its persisted context before this jump.
	if context != "" {
		best := int(^uint(0) >> 1)
		originalLine := line
		for _, m := range c.store.ByFile(path) {
			if m.Context == context {
				d := m.Line - originalLine
				if d < 0 {
					d = -d
				}
				if d < best {
					best = d
					line = m.Line
				}
			}
		}
	}
	ed.NavigateToLineCol(max(0, min(line-1, ed.LineCount()-1)), 0)
	if h := editorNavigation(c.tabs); h != nil && from.FilePath != "" {
		h.RecordJump(from, ed.NavigationPosition())
	}
}
func (c *editorBookmarks) next(ed *gui.CodeEditor, back bool) {
	rows := c.store.List()
	if len(rows) == 0 {
		return
	}
	path, line := ed.FilePath(), ed.CursorLine()+1
	chosen := rows[0]
	if back {
		chosen = rows[len(rows)-1]
		for i := len(rows) - 1; i >= 0; i-- {
			m := rows[i]
			if m.Path < path || m.Path == path && m.Line < line {
				chosen = m
				break
			}
		}
	} else {
		for _, m := range rows {
			if m.Path > path || m.Path == path && m.Line > line {
				chosen = m
				break
			}
		}
	}
	c.navigate(chosen.Path, chosen.Line)
}
func configureEditorBookmarks(tabs *gui.TabWidget, ed *gui.CodeEditor) {
	if c := bookmarkController(tabs); c != nil {
		c.bind(ed)
	}
}

type bookmarkCommand struct{ id, label, icon, hint string }

func bookmarkCommands() []bookmarkCommand {
	return []bookmarkCommand{
		{"bookmark.toggle", "Toggle Bookmark", "bookmarks", actionKeyHint() + "+F2"},
		{"bookmark.next", "Next Bookmark", "navigate-forward", "Alt+F2"},
		{"bookmark.previous", "Previous Bookmark", "navigate-back", "Alt+Shift+F2"},
		{"bookmark.clear", "Clear File Bookmarks", "line-delete", ""},
	}
}
func bookmarkCommandAvailable(c *editorBookmarks, ed *gui.CodeEditor, id string) bool {
	if c == nil || ed == nil {
		return false
	}
	switch id {
	case "bookmark.next", "bookmark.previous":
		return c.store.Count() > 0
	case "bookmark.clear":
		return ed.BookmarkCount() > 0
	}
	return true
}
func runBookmarkCommand(tabs *gui.TabWidget, canvas *ged.GedView, id string, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if expected == nil {
		ed = focusedClipboardEditor(tabs, canvas)
	}
	c := bookmarkController(tabs)
	if expected != nil && expected != ed || !bookmarkCommandAvailable(c, ed, id) {
		return
	}
	switch id {
	case "bookmark.add":
		ed.SetBookmark(ed.CursorLine(), true)
	case "bookmark.toggle":
		ed.ToggleBookmark()
	case "bookmark.next":
		c.next(ed, false)
	case "bookmark.previous":
		c.next(ed, true)
	case "bookmark.clear":
		ed.ClearBookmarks()
	}
}
func bindBookmarkCommand(tabs *gui.TabWidget, canvas *ged.GedView, s bookmarkCommand) paletteCommand {
	ed := focusedClipboardEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T(s.label), Hint: s.hint, Available: func() bool {
		return ed != nil && visibleCaretEditor(tabs, canvas) == ed && bookmarkCommandAvailable(bookmarkController(tabs), ed, s.id)
	}, Fn: func() {
		if ed != nil {
			runBookmarkCommand(tabs, canvas, s.id, ed)
		}
	}}
}

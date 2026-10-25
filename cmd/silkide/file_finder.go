package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/locator"
)

// fileEntry is one row in the quick-file-open list. Path is absolute
// (so the editor open-call doesn't have to resolve again) and Display
// is the project-relative form shown to the user — that's the form
// the subsequence filter runs against, since users type "gui/butt"
// not "/Users/firshme/Desktop/dc/gui/button.go".
type fileEntry struct {
	Path    string
	Display string
}

// fileFinderSkipDirs lists directory basenames the walker skips
// outright. Anything starting with "." is also skipped (covers .git,
// .idea, .vscode, .DS_Store) so this set only needs to handle non-
// dotfile directories that bloat the index.
var fileFinderSkipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	"target":       true,
	"build":        true,
	"dist":         true,
	"out":          true,
	"bin":          true,
}

// fileFinderSkipExts lists file extensions that are almost never
// useful to open in a code editor. The walker drops them before they
// reach the list so the user's type-ahead doesn't have to compete
// against thousands of binary asset names. Keep this list short —
// being over-aggressive here means the user can't find a real file.
var fileFinderSkipExts = map[string]bool{
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".ico":   true,
	".bmp":   true,
	".webp":  true,
	".pdf":   true,
	".zip":   true,
	".tar":   true,
	".tgz":   true,
	".gz":    true,
	".bz2":   true,
	".exe":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
	".a":     true,
	".o":     true,
	".class": true,
}

// Compatibility helper for pure walker tests. Interactive UI always uses the
// bounded asynchronous service; do not call this from a native event callback.
func walkProjectFiles(root string) []fileEntry {
	files, _ := scanFileFinder(context.Background(), root)
	return files
}

// rankFiles filters and ranks entries against query using the shared
// locator fuzzy engine — the same scorer behind the Qt-Creator-style
// quick-open box — instead of a hand-rolled subsequence-plus-length
// sort. Each entry becomes a locator.Item: Name is the project-relative
// Display (the string the user types against), Detail carries the
// absolute Path through untouched, and Kind tags it "file". locator.Match
// does the subsequence filtering and scoring (word/camelCase/separator
// boundary, consecutive-run, and prefix/exact-match bonuses), so a tight
// prefix hit like "main"→"main.go" outranks a scattered match, and gappy
// matches fall to the bottom rather than being ordered purely by length.
//
// query is trimmed but NOT lower-cased: locator folds case internally and
// needs the original case to award its exact/prefix tier bonuses. An empty
// (or all-space) query returns every entry, Name-sorted, per Match's
// contract. Path survives the Item.Detail round-trip so the caller still
// opens the same absolute file it selected.
func rankFiles(files []fileEntry, query string) []fileEntry {
	items := make([]locator.Item, len(files))
	for i, f := range files {
		items[i] = locator.Item{Name: f.Display, Detail: f.Path, Kind: "file"}
	}
	ranked := locator.Match(filepath.FromSlash(strings.TrimSpace(query)), items)
	out := make([]fileEntry, len(ranked))
	for i, it := range ranked {
		out[i] = fileEntry{Path: it.Detail, Display: it.Name}
	}
	return out
}

type fileFinder struct {
	dialog   *gui.Dialog
	input    *gui.Edit
	list     *gui.ListWidget
	status   *gui.Label
	service  *fileSearchService
	session  *fileSearchSession
	closed   bool
	selected *fileEntry
	reported bool
}

// Construction never scans, ranks or waits for disk I/O on the GUI thread.
func showFileFinder(parent gui.IWidget, root string, tabs *gui.TabWidget) {
	if parent == nil {
		return
	}
	f := newFileFinder(parent, root, quickOpenSearch)
	defer f.close()
	result := f.dialog.ShowModal()
	f.close()
	if reportDialogFailure(f.dialog.LastError()) {
		return
	}
	if result == gui.DialogOK && f.selected != nil && tabs != nil {
		openFileInEditor(tabs, f.selected.Path)
	}
}
func newFileFinder(parent gui.IWidget, root string, service *fileSearchService) *fileFinder {
	f := &fileFinder{dialog: gui.NewDialog(i18n.T("Open File"), parent), input: gui.NewEdit(), list: gui.NewListWidget(), status: gui.NewLabel(i18n.T("Indexing project files...")), service: service}
	box := gui.NewVBox()
	box.SetSpacing(6)
	box.AddWidget(f.input)
	f.list.SetFillAvailable(true)
	f.list.SetSelectionVisible(true)
	box.AddWidget(f.list)
	f.input.SetNavigationList(f.list)
	box.AddWidget(f.status)
	f.session = service.open(root, func(r fileSearchResult) {
		if f.closed {
			return
		}
		items := make([]gui.ListItem, 0, len(r.files))
		for _, entry := range r.files {
			items = append(items, gui.ListItem{Text: entry.Display, Data: entry})
		}
		fillFinderList(f.list, items)
		switch {
		case r.err != nil && !errors.Is(r.err, context.Canceled):
			code := diagnostic.CodeOf(r.err)
			f.status.SetText(fmt.Sprintf(i18n.T("[%s] Incomplete file index (%d matches)"), code, r.total))
			var d *diagnostic.Error
			hint := ""
			if errors.As(r.err, &d) {
				hint = d.Hint
			}
			gui.SetToolTip(f.status, r.err.Error()+"\n"+hint)
			if !f.reported {
				f.reported = true
				logEvent(ged.LogWarn, r.err.Error()+"\n"+hint)
			}
		case len(r.files) == 0:
			f.status.SetText(i18n.T("No files found"))
		case r.total > len(r.files):
			f.status.SetText(fmt.Sprintf(i18n.T("Showing %d of %d files; refine your search"), len(r.files), r.total))
		default:
			f.status.SetText(fmt.Sprintf(i18n.T("%d files"), r.total))
		}
	})
	f.input.SigTextChanged(func(_ interface{}, q string) {
		if f.closed {
			return
		}
		fillFinderList(f.list, nil) // Old rows cannot be opened while a new query runs.
		f.status.SetText(i18n.T("Searching files..."))
		gui.SetToolTip(f.status, "")
		f.service.update(f.session, q)
	})
	f.input.SigSubmit(func(_ interface{}, _ string) { f.choose(f.list.ActiveIndex()) })
	f.list.SigSubmit(func(interface{}) { f.choose(f.list.ActiveIndex()) })
	f.dialog.SetContent(box)
	f.dialog.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	f.dialog.SetSize(560, 480)
	f.dialog.SetInitialFocus(f.input)
	return f
}
func (f *fileFinder) choose(idx int) {
	if f.closed || idx < 0 || idx >= f.list.Count() {
		return
	}
	entry, ok := f.list.Item(idx).Data.(fileEntry)
	if !ok {
		return
	}
	f.selected = &entry
	dismissDialog(f.dialog)
}
func (f *fileFinder) close() {
	if f.closed {
		return
	}
	f.closed = true
	f.input.SetNavigationList(nil)
	f.service.close(f.session)
}

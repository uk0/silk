package main

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/core"
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

// walkProjectFiles returns every regular file under root (recursively)
// excluding hidden dirs/files, vendor-style trees, and known binary
// extensions. Display is set to the path relative to root using the
// host's separator — on macOS / Linux that's a forward slash, which
// matches what users typically type in a file finder.
//
// Walks once per call. For projects with thousands of files this is
// O(n) but takes well under 10 ms in practice — silkide is not
// targeting million-file monorepos. If that changes, the call site
// can cache the slice and refresh on filesystem events from
// silk/fswatch.
//
// Returns empty slice when root is empty or unreachable; an empty
// slice yields an empty list dialog, not an error popup.
func walkProjectFiles(root string) []fileEntry {
	if root == "" {
		return nil
	}
	var out []fileEntry
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// One unreadable subtree shouldn't kill the whole walk.
			// Returning nil here lets WalkDir keep going on siblings;
			// SkipDir would only stop the current dir.
			return nil
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if fileFinderSkipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if fileFinderSkipExts[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		out = append(out, fileEntry{Path: path, Display: rel})
		return nil
	})
	return out
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
	ranked := locator.Match(strings.TrimSpace(query), items)
	out := make([]fileEntry, len(ranked))
	for i, it := range ranked {
		out[i] = fileEntry{Path: it.Detail, Display: it.Name}
	}
	return out
}

// showFileFinder opens the modal Cmd+P quick-open dialog. The list
// is built fresh each invocation by walking root once — predictable
// over caching when the user creates files in a side editor and
// expects them to show up on the next Cmd+P.
//
// Enter on the search field opens the top match; Enter / double-
// click on the list opens whatever is selected. Both paths route
// through openFileInEditor, which knows how to dispatch .silkui
// files into the design canvas vs everything else into a new tab.
//
// parent owns the modal so the dialog inherits the IDE's window
// position; tabs is the editor TabWidget that catches non-.silkui
// files. canvas is required for the .silkui open path.
func showFileFinder(parent gui.IWidget, root string, tabs *gui.TabWidget) {
	if parent == nil {
		return
	}
	files := walkProjectFiles(root)
	if len(files) == 0 {
		// Nothing to show — popping an empty modal would just confuse
		// the user. Log so a regression that breaks walking still
		// surfaces in the dev console.
		core.Warn("file finder: 0 files under ", root)
		return
	}

	dlg := gui.NewDialog(i18n.T("Open File"), parent)
	box := gui.NewVBox()
	box.SetSpacing(6)

	input := gui.NewEdit()
	box.AddWidget(input)

	list := gui.NewListWidget()
	list.SetSelectionVisible(true)
	box.AddWidget(list)

	repopulate := func(query string) {
		list.Clear()
		for _, f := range rankFiles(files, query) {
			list.Append(gui.ListItem{Text: f.Display, Data: f})
		}
		if list.Count() > 0 {
			list.SetSelectionVisible(true)
		}
	}
	repopulate("")

	input.SigTextChanged(func(_ interface{}, q string) { repopulate(q) })

	openSelected := func(idx int) {
		if idx < 0 || idx >= list.Count() {
			return
		}
		f, ok := list.Item(idx).Data.(fileEntry)
		if !ok {
			return
		}
		dismissDialog(dlg)
		if tabs != nil {
			openFileInEditor(tabs, f.Path)
		}
	}

	// Enter on the search field opens the top match.
	input.SigSubmit(func(_ interface{}, _ string) {
		openSelected(0)
	})

	// Enter / double-click on the list opens the selected entry.
	list.SigSubmit(func(o interface{}) {
		openSelected(list.ActiveIndex())
	})

	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	dlg.SetSize(560, 480)
	input.SetFocus()
	dlg.ShowModal()
}

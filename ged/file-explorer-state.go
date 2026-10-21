package ged

import (
	"errors"
	"math"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Public state/query APIs are UI-thread-only, like other widget setters.
func (e *FileExplorer) RootDir() string             { return e.rootDir }
func (e *FileExplorer) Loading() bool               { return e.loading }
func (e *FileExplorer) LastError() error            { return e.scanErr }
func (e *FileExplorer) Filter() string              { return e.filter }
func (e *FileExplorer) SigScanError(fn func(error)) { e.onScanError = fn }
func (e *FileExplorer) SetEntryLimit(n int)         { e.entryLimit = max(1, min(n, 1000000)); e.requestScan() }
func (e *FileExplorer) SetShowHidden(v bool) {
	if e.showHidden != v {
		e.showHidden = v
		e.requestScan()
	}
}
func (e *FileExplorer) ShowHidden() bool { return e.showHidden }
func (e *FileExplorer) SetExcludedDirs(names []string) {
	e.excluded = map[string]bool{}
	for _, n := range names {
		e.excluded[n] = true
	}
	e.requestScan()
}
func (e *FileExplorer) SetFilter(query string) {
	query = strings.TrimSpace(query)
	r := []rune(query)
	if len(r) > 256 {
		query = string(r[:256])
	}
	if query == e.filter {
		return
	}
	e.filter = query
	e.searchCollapsed = map[string]bool{}
	e.scrollY = 0
	e.requestScan()
}
func (e *FileExplorer) SelectedPath() string {
	if e.selectedIdx >= 0 && e.selectedIdx < len(e.flatList) {
		return e.flatList[e.selectedIdx].path
	}
	return ""
}

// RevealPath expands only the ancestors of an in-project path; it never scans
// unrelated siblings recursively. The row is selected when its scan arrives.
func (e *FileExplorer) RevealPath(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil || e.rootDir == "" {
		return false
	}
	rel, err := filepath.Rel(e.rootDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	e.filter = ""
	e.revealPath = filepath.Clean(abs)
	e.selectionPath = e.revealPath
	if e.expanded == nil {
		e.expanded = map[string]bool{}
	}
	for dir := filepath.Dir(abs); abs != e.rootDir; dir = filepath.Dir(dir) {
		e.expanded[dir] = true
		if dir == e.rootDir || filepath.Dir(dir) == dir {
			break
		}
	}
	e.requestScan()
	return true
}
func copyExplorerSet(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (e *FileExplorer) requestScan() {
	if e.rootDir == "" {
		return
	}
	if path := e.SelectedPath(); path != "" && e.revealPath == "" {
		e.selectionPath = path
	}
	e.scanGeneration++
	e.loading = true
	req := explorerScanRequest{root: e.rootDir, filter: e.filter, expanded: copyExplorerSet(e.expanded), collapsed: copyExplorerSet(e.searchCollapsed), excluded: copyExplorerSet(e.excluded), hidden: e.showHidden, git: true, limit: e.entryLimit, generation: e.scanGeneration}
	if e.scanWorker == nil {
		post := e.post
		if post == nil {
			post = gui.Post
		}
		delivery := &explorerDelivery{post: post, apply: e.applyScan}
		e.scanWorker = &explorerScanWorker{scan: scanExplorer, deliver: delivery.offer}
	}
	e.scanWorker.submit(req)
	e.Update()
}
func (e *FileExplorer) applyScan(r explorerScanResult) {
	if r.generation != e.scanGeneration {
		return
	}
	if r.gitOnly {
		e.gitStatus = r.gitStatus
		e.Update()
		return
	}
	e.loading = false
	e.root = r.root
	e.gitStatus = nil
	e.scanErr = r.err
	e.hoverIdx = -1
	e.rebuildFlatList()
	e.selectedIdx = -1
	for i, row := range e.flatList {
		if row.path == e.selectionPath {
			e.selectedIdx = i
			break
		}
	}
	if e.revealPath != "" {
		e.ensureSelectionVisible()
		e.revealPath = ""
	}
	e.clampScroll()
	e.Update()
	if r.err != nil && e.onScanError != nil {
		e.onScanError(r.err)
	}
}

// CancelScan retires pending/queued snapshots; hosts should call it when the
// explorer is being permanently removed. A later SetRootDir can reuse it.
func (e *FileExplorer) CancelScan() {
	e.scanGeneration++
	e.loading = false
	if e.scanWorker != nil {
		e.scanWorker.stop()
	}
}
func (e *FileExplorer) setExpanded(entry *fileEntry, expanded bool) {
	if entry == nil || !entry.isDir {
		return
	}
	if e.filter != "" {
		e.searchCollapsed[entry.path] = !expanded
	} else {
		e.expanded[entry.path] = expanded
	}
	entry.expanded = expanded
	e.selectionPath = entry.path
	e.rebuildFlatList()
	e.selectedIdx = -1
	for i, row := range e.flatList {
		if row.path == entry.path {
			e.selectedIdx = i
			break
		}
	}
	e.requestScan()
}
func (e *FileExplorer) ensureFonts() {
	f := gui.Theme().Font
	if e.normalFont != nil && e.fontRevision == gui.ThemeRev() && e.normalFont.Family() == f.Family() && e.normalFont.Size() == f.Size() {
		return
	}
	e.normalFont = f.Dup()
	e.boldFont = paint.NewFont(f.Family(), f.Size(), true, false)
	e.rowHeight = math.Ceil(math.Max(22, e.normalFont.FontExtents().Height+8))
	e.fontRevision = gui.ThemeRev()
	for _, entry := range e.flatList {
		entry.textCached = false
	}
}
func (e *FileExplorer) headerHeight() float64 { e.ensureFonts(); return e.rowHeight }
func (e *FileExplorer) clampScroll() {
	e.ensureFonts()
	e.scrollY = math.Max(0, math.Min(e.scrollY, math.Max(0, float64(len(e.flatList))*e.rowHeight-(e.Height()-e.headerHeight()))))
}
func (e *FileExplorer) ensureSelectionVisible() {
	e.ensureFonts()
	if e.selectedIdx < 0 {
		return
	}
	top := float64(e.selectedIdx) * e.rowHeight
	available := math.Max(e.rowHeight, e.Height()-e.headerHeight())
	if top < e.scrollY {
		e.scrollY = top
	} else if top+e.rowHeight > e.scrollY+available {
		e.scrollY = top + e.rowHeight - available
	}
	e.clampScroll()
}

// ShowFilterDialog prompts for a path filter on the UI thread.
func (e *FileExplorer) ShowFilterDialog() { e.promptFilter() }

func (e *FileExplorer) promptFilter() {
	if q, ok := gui.ShowInputBox(e, nil, i18n.T("Filter Files"), i18n.T("Name or path contains:"), e.filter); ok {
		e.SetFilter(q)
	}
}
func (e *FileExplorer) showScanError() {
	if e.scanErr == nil {
		return
	}
	message := e.scanErr.Error()
	var d *diagnostic.Error
	if errors.As(e.scanErr, &d) {
		message += "\n" + d.Hint
	}
	gui.ShowMessageBox(e, nil, i18n.T("File scan"), message, []string{i18n.T("OK")})
}
func (e *FileExplorer) OnKeyDown(key int, repeat bool) {
	if !e.IsEnabled() {
		return
	}
	if key == 'F' && (gui.IsActionModifier() || gui.IsKeyDown(gui.KeyCtrl)) {
		if !repeat {
			e.promptFilter()
		}
		return
	}
	if repeat && (key == gui.KeyF2 || key == gui.KeyF5) {
		return
	}
	switch key {
	case gui.KeyF5:
		e.Refresh()
		return
	case gui.KeyEsc:
		e.SetFilter("")
		return
	case gui.KeyHome:
		e.selectedIdx = min(0, len(e.flatList)-1)
	case gui.KeyEnd:
		e.selectedIdx = len(e.flatList) - 1
	case gui.KeyDown:
		if e.selectedIdx < len(e.flatList)-1 {
			e.selectedIdx++
		}
	case gui.KeyUp:
		if e.selectedIdx > 0 {
			e.selectedIdx--
		}
	case gui.KeyPageDown:
		e.selectedIdx = min(len(e.flatList)-1, max(0, e.selectedIdx)+max(1, int((e.Height()-e.headerHeight())/e.rowHeight)))
	case gui.KeyPageUp:
		e.selectedIdx = min(len(e.flatList)-1, max(0, e.selectedIdx-max(1, int((e.Height()-e.headerHeight())/e.rowHeight))))
	case gui.KeyEnter, gui.KeyRight:
		if e.selectedIdx >= 0 && e.selectedIdx < len(e.flatList) {
			row := e.flatList[e.selectedIdx]
			if row.isDir {
				if !row.expanded {
					e.setExpanded(row, true)
				} else if e.selectedIdx+1 < len(e.flatList) && e.flatList[e.selectedIdx+1].depth > row.depth {
					e.selectedIdx++
				}
			} else if key == gui.KeyEnter {
				e.onDoubleClick(e.selectedIdx)
			}
		}
	case gui.KeyLeft:
		if e.selectedIdx >= 0 && e.selectedIdx < len(e.flatList) {
			row := e.flatList[e.selectedIdx]
			if row.isDir && row.expanded {
				e.setExpanded(row, false)
			} else {
				parent := filepath.Dir(row.path)
				for i, r := range e.flatList {
					if r.path == parent {
						e.selectedIdx = i
						break
					}
				}
			}
		}
	case gui.KeyF2:
		if e.selectedIdx > 0 && e.selectedIdx < len(e.flatList) {
			e.promptRename(e.flatList[e.selectedIdx])
		}
		return
	default:
		return
	}
	e.selectionPath = e.SelectedPath()
	e.ensureSelectionVisible()
	e.Update()
}

// HandleShortcut keeps explorer-local refresh/rename from triggering global
// Run or Rename Symbol in the editor behind the focused file tree. All other
// application shortcuts retain their normal priority.
func (e *FileExplorer) HandleShortcut(key int, mods uint8) bool {
	if !e.IsEnabled() {
		return false
	}
	if mods == 0 && (key == gui.KeyF5 || key == gui.KeyF2) {
		e.OnKeyDown(key, false)
		return true
	}
	if key == 'F' && mods == gui.ModAction {
		e.promptFilter()
		return true
	}
	return false
}

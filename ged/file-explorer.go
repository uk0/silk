package ged

import (
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.FileExplorer", gui.TypeOf(FileExplorer{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.FileExplorer",
		Name: "文件",
		Icon: "folder",
		Desc: "项目文件浏览器",
	})
}

// fileEntry represents a single node in the directory tree.
type fileEntry struct {
	text              string
	textWidth         float64
	textRevision      uint64
	textCached        bool
	loaded, isSymlink bool
	name              string
	path              string
	isDir             bool
	depth             int
	expanded          bool
	children          []*fileEntry
}

// FileExplorer is a tree-style file browser panel that scans and displays
// the project directory structure. Double-clicking a file fires a callback
// to open it in an editor.
type FileExplorer struct {
	expanded, searchCollapsed, excluded map[string]bool
	showHidden                          bool
	filter, selectionPath, revealPath   string
	entryLimit                          int
	loading                             bool
	scanGeneration                      uint64
	scanErr                             error
	scanWorker                          *explorerScanWorker
	gitWorker                           *explorerScanWorker
	gitGeneration                       uint64
	post                                func(func())
	onScanError                         func(error)
	normalFont, boldFont                paint.Font
	fontRevision                        uint64
	gui.Widget

	rootDir  string
	root     *fileEntry
	flatList []*fileEntry // flattened visible entries

	scrollY     float64
	hoverIdx    int
	selectedIdx int
	rowHeight   float64

	// double-click detection
	lastClickTime time.Time
	lastClickPath string

	// git status badges, keyed by absolute file path; refreshed once per
	// scan. Empty when rootDir is not inside a git repository.
	gitStatus map[string]rune

	renameBusy        bool
	renameIO          func(string, string) (string, error) // test seam; defaults to fsRename
	renameCoordinator func(string, string) (RenamePreparation, error)
	renameWaitTimeout time.Duration // zero uses the five-second preparation deadline

	cbFileOpen func(path string)
}

// Directories and file patterns to skip during scanning.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "Package": true,
	".idea": true, "__pycache__": true, "vendor": true, ".DS_Store": true,
	".claude": true,
}

func NewFileExplorer() *FileExplorer {
	p := new(FileExplorer)
	p.Init(p)
	return p
}

func (this *FileExplorer) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.hoverIdx = -1
	this.selectedIdx = -1
	this.expanded = map[string]bool{}
	this.searchCollapsed = map[string]bool{}
	this.excluded = copyExplorerSet(skipDirs)
	this.entryLimit = defaultExplorerEntryLimit
	this.SetFocusPolicy(gui.TabFocus)
	gui.SetToolTip(this, "Ctrl/Cmd+F: filter files · F5: refresh · Enter: open · Esc: clear filter")
}

// SetRootDir starts a cancellable background scan. Collapsed subdirectories
// are not read until expanded. UI-thread-only; observe Loading/SigScanError.
func (this *FileExplorer) SetRootDir(dir string) {
	this.CancelScan()
	this.lastClickPath = ""
	this.lastClickTime = time.Time{}
	this.root = nil
	this.rootDir = ""
	this.flatList = nil
	this.selectedIdx = -1
	this.selectionPath = ""
	this.revealPath = ""
	this.scrollY = 0
	this.gitStatus = nil
	this.scanErr = nil
	this.expanded = map[string]bool{}
	this.searchCollapsed = map[string]bool{}
	this.filter = ""
	if dir == "" {
		this.rootDir = ""
		this.Update()
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		this.scanErr = diagnostic.New(diagnostic.FileScan, "cannot resolve project directory", err)
		if this.onScanError != nil {
			this.onScanError(this.scanErr)
		}
		this.Update()
		return
	}
	this.rootDir = filepath.Clean(abs)
	this.expanded[this.rootDir] = true
	this.requestScan()
}

// SigFileOpen registers a callback invoked when a file is double-clicked.
func (this *FileExplorer) SigFileOpen(fn func(path string)) {
	this.cbFileOpen = fn
}

// rebuildFlatList flattens the tree respecting expanded state.
func (this *FileExplorer) rebuildFlatList() {
	this.flatList = nil
	if this.root == nil {
		return
	}
	this.flattenNode(this.root)
}

func (this *FileExplorer) flattenNode(e *fileEntry) {
	this.flatList = append(this.flatList, e)
	if e.isDir && e.expanded {
		for _, child := range e.children {
			this.flattenNode(child)
		}
	}
}

// iconNameForEntry maps a tree entry to a resource icon name. Directories use
// "folder"; every file uses "document" for now (per-extension art such as a
// dedicated Go glyph is a follow-up). The ext argument is the lower-cased
// filepath.Ext result (e.g. ".go"). The return value is always one of the
// guaranteed-present resource names {folder, document}, so a row never renders
// the red-X missing-icon placeholder.
func iconNameForEntry(isDir bool, ext string) string {
	if isDir {
		return "folder"
	}
	_ = ext // reserved for future per-extension glyphs
	return "document"
}

// parseGitPorcelain turns `git status --porcelain` output into a map from
// absolute file path to a single status badge rune. Porcelain lines are
// "XY <path>" where X is the staged (index) state and Y the worktree
// state; untracked entries are "?? <path>" and renames are
// "R  <old> -> <new>". Paths are repo-relative, so each is joined onto
// gitRoot to produce the absolute key the tree is indexed by. For a
// rename the new path is keyed. The badge prefers, in order: untracked
// (?), then the staged column, then the worktree column, collapsed to the
// documented set M / A / D / ? (a staged rename maps to A on the new
// path). It is a pure function so it can be tested without a real repo.
func parseGitPorcelain(output string, gitRoot string) map[string]rune {
	result := map[string]rune{}
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1]
		rest := line[3:]

		// Renames/copies report "old -> new"; key the new path.
		path := rest
		if i := strings.Index(rest, " -> "); i >= 0 {
			path = rest[i+4:]
		}
		path = strings.TrimSpace(path)
		// Porcelain quotes paths with unusual characters; strip the quotes.
		path = strings.Trim(path, `"`)
		if path == "" {
			continue
		}

		var badge rune
		switch {
		case x == '?' && y == '?':
			badge = '?'
		case x == 'D' || y == 'D':
			badge = 'D'
		case x == 'A' || x == 'R' || x == 'C':
			badge = 'A' // staged add / rename-new / copy
		case x == 'M' || y == 'M':
			badge = 'M'
		case y == 'A':
			badge = 'A'
		default:
			continue
		}
		result[filepath.Join(gitRoot, path)] = badge
	}
	return result
}

// gitStatusFor returns the git badge rune for the given absolute path, or
// 0 when the path has no tracked change (or there is no repo).
func (this *FileExplorer) gitStatusFor(path string) rune {
	if this.gitStatus == nil {
		return 0
	}
	return this.gitStatus[path]
}

// gitBadgeColor maps a status badge rune to its display color, matching
// the convention used by Qt Creator / VS Code: amber modified, green
// added/staged, gray untracked, red deleted.
func gitBadgeColor(badge rune) paint.Color {
	switch badge {
	case 'M':
		return paint.Color{R: 220, G: 160, B: 40, A: 255} // amber
	case 'A':
		return paint.Color{R: 60, G: 170, B: 90, A: 255} // green
	case '!', 'D':
		return paint.Color{R: 210, G: 70, B: 70, A: 255} // red
	default:
		return paint.Color{R: 150, G: 150, B: 160, A: 255} // gray (untracked)
	}
}

// hitTest returns the flat list index for a given y coordinate, or -1.
func (this *FileExplorer) hitTest(y float64) int {
	headerH := this.headerHeight()
	if y < headerH || y >= this.Height() {
		return -1
	}
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.flatList) {
		return -1
	}
	return idx
}

// OnLeftDown handles click events: expand/collapse folders, select entries.
func (this *FileExplorer) OnLeftDown(x, y float64) {
	this.SetFocus()
	if y < this.headerHeight() {
		switch {
		case x >= this.Width()-28:
			this.Refresh()
		case x >= this.Width()-56:
			this.promptFilter()
		case this.scanErr != nil && x >= this.Width()-84:
			this.showScanError()
		}
		return
	}
	idx := this.hitTest(y)
	if idx < 0 {
		return
	}
	now := time.Now()
	double := this.flatList[idx].path == this.lastClickPath && now.Sub(this.lastClickTime) < 400*time.Millisecond
	this.lastClickTime = now
	this.lastClickPath = this.flatList[idx].path
	this.selectedIdx = idx
	this.selectionPath = this.flatList[idx].path
	entry := this.flatList[idx]
	if entry.isDir {
		if !double {
			this.setExpanded(entry, !entry.expanded)
		}
	} else if double {
		this.onDoubleClick(idx)
		this.lastClickTime = time.Time{}
	}
	this.Update()
}

// onDoubleClick fires the file open callback for non-directory entries.
func (this *FileExplorer) onDoubleClick(idx int) {
	if idx < 0 || idx >= len(this.flatList) {
		return
	}
	entry := this.flatList[idx]
	if entry.isDir {
		return
	}
	this.selectedIdx = idx
	this.selectionPath = entry.path
	if this.cbFileOpen != nil {
		this.cbFileOpen(entry.path)
	}
	this.Self().Update()
}

// OnMouseMove updates hover state.
func (this *FileExplorer) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover.
func (this *FileExplorer) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling through the file tree.
func (this *FileExplorer) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	headerH := this.headerHeight()
	totalRows := float64(len(this.flatList))
	maxScroll := totalRows*this.rowHeight - (this.Height() - headerH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// Refresh rescans the root directory.
func (this *FileExplorer) Refresh() { this.refreshKeepingExpansion() }

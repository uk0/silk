package ged

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"silk/core"
	"silk/gui"
	"silk/paint"
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
	name     string
	path     string
	isDir    bool
	depth    int
	expanded bool
	children []*fileEntry
}

// FileExplorer is a tree-style file browser panel that scans and displays
// the project directory structure. Double-clicking a file fires a callback
// to open it in an editor.
type FileExplorer struct {
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
	lastClickIdx  int

	// git status badges, keyed by absolute file path; refreshed once per
	// scan. Empty when rootDir is not inside a git repository.
	gitStatus map[string]rune

	cbFileOpen func(path string)
}

// Directories and file patterns to skip during scanning.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "Package": true, "icon": true,
	".idea": true, "__pycache__": true, "vendor": true, ".DS_Store": true,
	"docs": true, ".claude": true,
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
}

// SetRootDir scans the given directory and rebuilds the tree.
func (this *FileExplorer) SetRootDir(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	this.rootDir = abs
	this.root = this.scanDir(abs, 0)
	this.rebuildFlatList()
	this.Self().Update()
}

// SigFileOpen registers a callback invoked when a file is double-clicked.
func (this *FileExplorer) SigFileOpen(fn func(path string)) {
	this.cbFileOpen = fn
}

// scanDir recursively reads a directory, skipping hidden and noisy entries.
func (this *FileExplorer) scanDir(dir string, depth int) *fileEntry {
	// Collect git status once at the top of a fresh scan/refresh — both
	// SetRootDir and refreshKeepingExpansion enter here with depth 0 — so
	// the badges are computed a single time per tree rebuild, not per row.
	if depth == 0 {
		this.collectGitStatus()
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	entry := &fileEntry{
		name:  info.Name(),
		path:  dir,
		isDir: info.IsDir(),
		depth: depth,
	}
	if !info.IsDir() {
		return entry
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return entry
	}

	var dirs, files []*fileEntry
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files/dirs
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Skip noise directories
		if e.IsDir() && skipDirs[name] {
			continue
		}
		// Skip noise files
		if !e.IsDir() && skipDirs[name] {
			continue
		}

		child := this.scanDir(filepath.Join(dir, name), depth+1)
		if child != nil {
			if child.isDir {
				dirs = append(dirs, child)
			} else {
				files = append(files, child)
			}
		}
	}

	// Sort: directories first (alphabetical), then files (alphabetical)
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].name) < strings.ToLower(files[j].name)
	})

	entry.children = append(dirs, files...)

	// Auto-expand top level
	if depth == 0 {
		entry.expanded = true
	}

	return entry
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

// Draw renders the file tree.
func (this *FileExplorer) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := 22.0
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	dirName := filepath.Base(this.rootDir)
	if dirName == "" {
		dirName = "Project"
	}
	g.DrawText1(8, headerH-5, dirName)

	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	rh := this.rowHeight
	startY := headerH - this.scrollY

	for i, entry := range this.flatList {
		rowY := startY + float64(i)*rh
		if rowY+rh < headerH || rowY > h {
			continue
		}

		indent := float64(entry.depth) * 16.0
		textX := 8 + indent

		// Selected highlight
		if i == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 51, G: 120, B: 215, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		} else if i == this.hoverIdx {
			// Hover highlight
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Expand/collapse triangle for directories
		if entry.isDir && len(entry.children) > 0 {
			triX := textX
			triY := rowY + rh/2
			g.Save()
			if entry.expanded {
				// Down-pointing triangle
				g.MoveTo(triX, triY-3)
				g.LineTo(triX+6, triY-3)
				g.LineTo(triX+3, triY+3)
			} else {
				// Right-pointing triangle
				g.MoveTo(triX, triY-4)
				g.LineTo(triX+6, triY)
				g.LineTo(triX, triY+4)
			}
			if i == this.selectedIdx {
				g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
			} else {
				g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			}
			g.Fill()
			g.Restore()
			textX += 10
		} else if !entry.isDir {
			textX += 10
		} else {
			textX += 10
		}

		// File type indicator dot
		dotR := 3.0
		dotX := textX + dotR
		dotY := rowY + rh/2
		dotColor := this.fileTypeColor(entry)
		g.Save()
		if entry.isDir {
			// Folder: small filled rectangle
			g.SetBrush1(dotColor)
			g.Rectangle(dotX-3, dotY-3, 6, 5)
			g.Fill()
			// Folder tab
			g.Rectangle(dotX-3, dotY-4, 3, 1)
			g.Fill()
		} else {
			// File: colored dot
			g.SetBrush1(dotColor)
			g.Arc(dotX, dotY, dotR, 0, 2*math.Pi)
			g.Fill()
		}
		g.Restore()
		textX += dotR*2 + 4

		// Text
		if entry.isDir {
			g.SetFont(boldFont)
		} else {
			g.SetFont(normalFont)
		}
		if i == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.DrawText1(textX, rowY+rh-6, entry.name)

		// Git status badge: a small status letter after the filename. On
		// the selected (blue) row it's drawn white so it stays legible;
		// otherwise it uses the conventional M/A/?/D colors.
		if !entry.isDir {
			if badge := this.gitStatusFor(entry.path); badge != 0 {
				ext := g.Font().TextExtents(entry.name)
				badgeFont := paint.NewFont(t.Font.Family(), 10, true, false)
				g.SetFont(badgeFont)
				if i == this.selectedIdx {
					g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
				} else {
					g.SetBrush1(gitBadgeColor(badge))
				}
				g.DrawText1(textX+ext.XAdvance+6, rowY+rh-6, string(badge))
			}
		}

		// Bottom separator
		if i != this.selectedIdx {
			g.SetPen1(paint.Color{R: 235, G: 235, B: 240, A: 60}, 0.5)
			g.MoveTo(8+indent, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}
}

// fileTypeColor returns the indicator color for the given entry.
func (this *FileExplorer) fileTypeColor(entry *fileEntry) paint.Color {
	if entry.isDir {
		return paint.Color{R: 220, G: 180, B: 60, A: 255} // yellow/gold
	}
	ext := strings.ToLower(filepath.Ext(entry.name))
	switch ext {
	case ".go":
		return paint.Color{R: 0, G: 173, B: 131, A: 255} // green
	case ".mod", ".sum":
		return paint.Color{R: 230, G: 140, B: 50, A: 255} // orange
	case ".md", ".txt":
		return paint.Color{R: 80, G: 140, B: 220, A: 255} // blue
	default:
		return paint.Color{R: 160, G: 160, B: 170, A: 255} // gray
	}
}

// collectGitStatus runs `git status --porcelain` in rootDir and rebuilds
// the path->badge map. When rootDir is not inside a git repository (git
// exits non-zero) the map is left empty so no badges are drawn and no
// error is surfaced — the file tree works the same outside a repo.
func (this *FileExplorer) collectGitStatus() {
	this.gitStatus = nil
	if this.rootDir == "" {
		return
	}

	top, err := exec.Command("git", "-C", this.rootDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return // not a git repo (or git unavailable)
	}
	gitRoot := strings.TrimSpace(string(top))
	if gitRoot == "" {
		return
	}

	out, err := exec.Command("git", "-C", this.rootDir, "status", "--porcelain").Output()
	if err != nil {
		return
	}
	this.gitStatus = parseGitPorcelain(string(out), gitRoot)
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
	case 'D':
		return paint.Color{R: 210, G: 70, B: 70, A: 255} // red
	default:
		return paint.Color{R: 150, G: 150, B: 160, A: 255} // gray (untracked)
	}
}

// hitTest returns the flat list index for a given y coordinate, or -1.
func (this *FileExplorer) hitTest(y float64) int {
	headerH := 22.0
	if y < headerH {
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
	idx := this.hitTest(y)
	if idx < 0 {
		return
	}

	now := time.Now()
	// Double-click detection
	if idx == this.lastClickIdx && now.Sub(this.lastClickTime) < 400*time.Millisecond {
		this.onDoubleClick(idx)
		this.lastClickTime = time.Time{} // reset to avoid triple-click
		return
	}
	this.lastClickTime = now
	this.lastClickIdx = idx

	entry := this.flatList[idx]
	if entry.isDir {
		entry.expanded = !entry.expanded
		this.rebuildFlatList()
		// Adjust selected index if needed
		if this.selectedIdx >= len(this.flatList) {
			this.selectedIdx = len(this.flatList) - 1
		}
	} else {
		this.selectedIdx = idx
	}
	this.Self().Update()
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
	headerH := 22.0
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
func (this *FileExplorer) Refresh() {
	if this.rootDir != "" {
		this.SetRootDir(this.rootDir)
	}
}

package ged

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.GlobalSearchPanel", gui.TypeOf(GlobalSearchPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GlobalSearchPanel",
		Name: "搜索",
		Icon: "search",
		Desc: "全局搜索",
	})
}

// SearchMatch represents a single search hit in a file.
type SearchMatch struct {
	FilePath string
	Line     int
	Column   int
	LineText string
	MatchLen int
}

// GlobalSearchPanel provides a search-across-files panel like VS Code's sidebar.
type GlobalSearchPanel struct {
	gui.Widget

	query     string
	queryRunes []rune
	queryCursor int
	results   []SearchMatch
	grouped   map[string][]SearchMatch // grouped by file
	fileOrder []string                 // ordered file keys
	collapsed map[string]bool          // collapsed file groups

	hoverIdx  int
	scrollY   float64
	rowHeight float64
	searching bool
	rootDir   string
	cbOpen    func(path string, line int)

	// flattened visible rows for rendering
	flatRows  []searchRow
}

type searchRowKind int

const (
	searchRowFile  searchRowKind = iota // file header row
	searchRowMatch                      // match result row
)

type searchRow struct {
	kind     searchRowKind
	filePath string
	match    *SearchMatch
}

func NewGlobalSearchPanel() *GlobalSearchPanel {
	p := new(GlobalSearchPanel)
	p.Init(p)
	return p
}

func (this *GlobalSearchPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.collapsed = make(map[string]bool)
}

// SetRootDir sets the root directory for searching.
func (this *GlobalSearchPanel) SetRootDir(dir string) {
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	this.rootDir = dir
}

// SigOpen registers a callback invoked when a search result is clicked.
func (this *GlobalSearchPanel) SigOpen(fn func(path string, line int)) {
	this.cbOpen = fn
}

// Search executes a search across all .go files in rootDir.
func (this *GlobalSearchPanel) Search(query string) {
	this.query = query
	this.queryRunes = []rune(query)
	this.queryCursor = len(this.queryRunes)
	this.results = nil
	this.grouped = make(map[string][]SearchMatch)
	this.fileOrder = nil
	this.scrollY = 0
	this.hoverIdx = -1

	if query == "" || this.rootDir == "" {
		this.rebuildFlatRows()
		this.Self().Update()
		return
	}

	this.searching = true
	this.Self().Update()

	// Walk files
	_ = filepath.Walk(this.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()

		// Skip hidden
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip noise directories (reuse skipDirs from file-explorer.go)
		if info.IsDir() && skipDirs[name] {
			return filepath.SkipDir
		}
		// Only search .go files
		if info.IsDir() || !strings.HasSuffix(name, ".go") {
			return nil
		}

		this.searchFile(path, query)
		return nil
	})

	this.searching = false
	this.rebuildFlatRows()
	this.Self().Update()
}

// searchFile searches a single file for the query string.
func (this *GlobalSearchPanel) searchFile(path, query string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	lowerQuery := strings.ToLower(query)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		lowerLine := strings.ToLower(line)

		// Find all matches on this line
		offset := 0
		for {
			idx := strings.Index(lowerLine[offset:], lowerQuery)
			if idx < 0 {
				break
			}
			col := offset + idx
			match := SearchMatch{
				FilePath: path,
				Line:     lineNum,
				Column:   col,
				LineText: line,
				MatchLen: len(query),
			}
			this.results = append(this.results, match)

			if _, exists := this.grouped[path]; !exists {
				this.fileOrder = append(this.fileOrder, path)
			}
			this.grouped[path] = append(this.grouped[path], match)

			offset = col + len(query)
		}
	}
}

// rebuildFlatRows flattens grouped results into a list of renderable rows.
func (this *GlobalSearchPanel) rebuildFlatRows() {
	this.flatRows = nil
	for _, filePath := range this.fileOrder {
		this.flatRows = append(this.flatRows, searchRow{
			kind:     searchRowFile,
			filePath: filePath,
		})
		if !this.collapsed[filePath] {
			matches := this.grouped[filePath]
			for i := range matches {
				this.flatRows = append(this.flatRows, searchRow{
					kind:     searchRowMatch,
					filePath: filePath,
					match:    &matches[i],
				})
			}
		}
	}
}

// totalMatchCount returns the total number of matches.
func (this *GlobalSearchPanel) totalMatchCount() int {
	return len(this.results)
}

// totalFileCount returns the number of files with matches.
func (this *GlobalSearchPanel) totalFileCount() int {
	return len(this.fileOrder)
}

// Draw renders the search panel.
func (this *GlobalSearchPanel) Draw(g paint.Painter) {
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
	g.DrawText1(8, headerH-5, "搜索")

	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	smallFont := paint.NewFont(t.Font.Family(), 10, false, false)

	// Search input area
	inputY := headerH + 4
	inputH := 24.0
	inputX := 8.0
	inputW := w - 16

	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Fill()
	g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Stroke()

	g.SetFont(normalFont)
	if len(this.queryRunes) > 0 {
		g.SetBrush1(t.TextColor)
		g.DrawText1(inputX+4, inputY+inputH-7, string(this.queryRunes))
	} else {
		g.SetBrush1(paint.Color{R: 160, G: 160, B: 170, A: 180})
		g.DrawText1(inputX+4, inputY+inputH-7, "Search... (Enter)")
	}

	// Summary line
	summaryY := inputY + inputH + 4
	summaryH := 16.0

	if len(this.results) > 0 {
		g.SetFont(smallFont)
		g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 255})
		summary := fmt.Sprintf("%d matches in %d files", this.totalMatchCount(), this.totalFileCount())
		g.DrawText1(8, summaryY+summaryH-3, summary)
	} else if this.query != "" && !this.searching {
		g.SetFont(smallFont)
		g.SetBrush1(paint.Color{R: 160, G: 120, B: 120, A: 200})
		g.DrawText1(8, summaryY+summaryH-3, "No results found")
	}

	// Results list
	listY := summaryY + summaryH + 2
	rh := this.rowHeight
	startY := listY - this.scrollY

	for i, row := range this.flatRows {
		rowY := startY + float64(i)*rh
		if rowY+rh < listY || rowY > h {
			continue
		}

		// Hover highlight
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		switch row.kind {
		case searchRowFile:
			// File header
			relPath := row.filePath
			if this.rootDir != "" {
				if rel, err := filepath.Rel(this.rootDir, row.filePath); err == nil {
					relPath = rel
				}
			}

			// Expand/collapse triangle
			triX := 8.0
			triY := rowY + rh/2
			isCollapsed := this.collapsed[row.filePath]
			if isCollapsed {
				g.MoveTo(triX, triY-4)
				g.LineTo(triX+6, triY)
				g.LineTo(triX, triY+4)
			} else {
				g.MoveTo(triX, triY-3)
				g.LineTo(triX+6, triY-3)
				g.LineTo(triX+3, triY+3)
			}
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			g.Fill()

			// File name (bold)
			g.SetFont(boldFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(20, rowY+rh-5, filepath.Base(relPath))

			// Directory path (dimmed)
			dir := filepath.Dir(relPath)
			if dir != "." && dir != "" {
				nameExt := boldFont.TextExtents(filepath.Base(relPath))
				g.SetFont(smallFont)
				g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 200})
				g.DrawText1(20+nameExt.XAdvance+6, rowY+rh-5, dir)
			}

			// Match count badge
			matchCount := len(this.grouped[row.filePath])
			countStr := fmt.Sprintf("%d", matchCount)
			g.SetFont(smallFont)
			countExt := smallFont.TextExtents(countStr)
			badgeX := w - countExt.XAdvance - 16
			g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 200})
			g.DrawText1(badgeX, rowY+rh-5, countStr)

		case searchRowMatch:
			if row.match == nil {
				continue
			}
			m := row.match
			lineText := strings.TrimSpace(m.LineText)
			// Truncate long lines
			if len(lineText) > 120 {
				lineText = lineText[:120] + "..."
			}

			// Line number
			g.SetFont(smallFont)
			g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 200})
			lineStr := fmt.Sprintf("%d:", m.Line)
			g.DrawText1(24, rowY+rh-4, lineStr)

			// Line text with match highlight
			lineNumExt := smallFont.TextExtents(lineStr)
			textX := 24 + lineNumExt.XAdvance + 4

			// Find match position in the trimmed text
			trimOffset := len(m.LineText) - len(strings.TrimLeft(m.LineText, " \t"))
			matchCol := m.Column - trimOffset
			if matchCol < 0 {
				matchCol = 0
			}

			g.SetFont(normalFont)
			lineRunes := []rune(lineText)

			if matchCol >= 0 && matchCol < len(lineRunes) && matchCol+m.MatchLen <= len(lineRunes) {
				// Pre-match text
				if matchCol > 0 {
					pre := string(lineRunes[:matchCol])
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, pre)
					preExt := normalFont.TextExtents(pre)
					textX += preExt.XAdvance
				}

				// Match highlight background
				matchText := string(lineRunes[matchCol : matchCol+m.MatchLen])
				matchExt := normalFont.TextExtents(matchText)
				g.SetBrush1(paint.Color{R: 255, G: 200, B: 60, A: 120})
				g.Rectangle(textX, rowY+2, matchExt.XAdvance, rh-4)
				g.Fill()

				// Match text
				g.SetBrush1(paint.Color{R: 180, G: 120, B: 20, A: 255})
				g.DrawText1(textX, rowY+rh-4, matchText)
				textX += matchExt.XAdvance

				// Post-match text
				postStart := matchCol + m.MatchLen
				if postStart < len(lineRunes) {
					post := string(lineRunes[postStart:])
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, post)
				}
			} else {
				// Fallback: just draw the line
				g.SetBrush1(t.TextColor)
				g.DrawText1(textX, rowY+rh-4, lineText)
			}
		}
	}
}

// hitTest returns the row index for a given y coordinate, or -1.
func (this *GlobalSearchPanel) hitTest(y float64) int {
	headerH := 22.0
	inputH := 24.0
	summaryH := 16.0
	listY := headerH + 4 + inputH + 4 + summaryH + 2

	if y < listY {
		return -1
	}
	idx := int(math.Floor((y - listY + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.flatRows) {
		return -1
	}
	return idx
}

// isInInputArea checks if a click is in the search input area.
func (this *GlobalSearchPanel) isInInputArea(y float64) bool {
	headerH := 22.0
	inputY := headerH + 4
	inputH := 24.0
	return y >= inputY && y <= inputY+inputH
}

// OnLeftDown handles clicks.
func (this *GlobalSearchPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if this.isInInputArea(y) {
		// Focus on input
		return
	}

	idx := this.hitTest(y)
	if idx < 0 || idx >= len(this.flatRows) {
		return
	}

	row := this.flatRows[idx]
	switch row.kind {
	case searchRowFile:
		// Toggle collapse
		this.collapsed[row.filePath] = !this.collapsed[row.filePath]
		this.rebuildFlatRows()
	case searchRowMatch:
		if row.match != nil && this.cbOpen != nil {
			this.cbOpen(row.match.FilePath, row.match.Line)
		}
	}
	this.Self().Update()
}

// OnMouseMove updates hover.
func (this *GlobalSearchPanel) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover.
func (this *GlobalSearchPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *GlobalSearchPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	headerH := 22.0
	inputH := 24.0
	summaryH := 16.0
	listY := headerH + 4 + inputH + 4 + summaryH + 2
	totalRows := float64(len(this.flatRows))
	maxScroll := totalRows*this.rowHeight - (this.Height() - listY)
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

// OnTextInput handles typing in the search input.
func (this *GlobalSearchPanel) OnTextInput(s string) {
	this.queryRunes = append(this.queryRunes, []rune(s)...)
	this.queryCursor = len(this.queryRunes)
	this.Self().Update()
}

// OnKeyDown handles keyboard input.
func (this *GlobalSearchPanel) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyBackSpace:
		if len(this.queryRunes) > 0 {
			this.queryRunes = this.queryRunes[:len(this.queryRunes)-1]
			this.queryCursor = len(this.queryRunes)
			this.Self().Update()
		}
	case gui.KeyEnter:
		this.Search(string(this.queryRunes))
	case gui.KeyEsc:
		this.queryRunes = nil
		this.queryCursor = 0
		this.results = nil
		this.grouped = nil
		this.fileOrder = nil
		this.flatRows = nil
		this.Self().Update()
	}
}

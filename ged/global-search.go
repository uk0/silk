package ged

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
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

	query       string
	queryRunes  []rune
	queryCursor int
	results     []SearchMatch
	grouped     map[string][]SearchMatch // grouped by file
	fileOrder   []string                 // ordered file keys
	collapsed   map[string]bool          // collapsed file groups

	replaceRunes  []rune // replace-with text
	replaceCursor int
	focusReplace  bool // true when the replace input holds keyboard focus

	hoverIdx  int
	scrollY   float64
	rowHeight float64
	searching bool
	rootDir   string
	cbOpen    func(path string, line int)

	// flattened visible rows for rendering
	flatRows []searchRow
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

// Search runs a find-in-files search under rootDir for query. The filesystem
// walk is handed to the shared filesearch engine on a background goroutine and
// the results are marshalled back onto the main thread with gui.Post before any
// panel state is mutated, so a search triggered from the input never blocks the
// UI on disk I/O.
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

	root := this.rootDir
	go func() {
		matches := runSearch(root, query)
		gui.Post(func() {
			// A newer search may have superseded this one while it ran.
			if this.query != query {
				return
			}
			this.searching = false
			this.applyResults(matches)
			this.rebuildFlatRows()
			this.Self().Update()
		})
	}()
}

// runSearch runs the filesearch engine and maps its matches onto SearchMatch.
// It is pure (touches no panel state and no GUI), so it is safe to call from a
// goroutine and directly from tests. filesearch reports a 1-based BYTE column;
// Column is stored 0-based in bytes — the same units Draw slices the line by —
// so search and highlight agree and multibyte text before a match no longer
// shifts the highlight. Every text file is scanned (binary files and
// .git/vendor/node_modules are skipped by the engine), broadening the previous
// .go-only search.
func runSearch(root, query string) []SearchMatch {
	matches, err := filesearch.Search(root, query, filesearch.Options{
		IgnoreCase: true,
		SkipBinary: true,
	})
	if err != nil {
		return nil
	}
	out := make([]SearchMatch, 0, len(matches))
	for _, m := range matches {
		out = append(out, SearchMatch{
			FilePath: m.Path,
			Line:     m.Line,
			Column:   m.Col - 1,
			LineText: m.Text,
			MatchLen: len(query),
		})
	}
	return out
}

// applyResults installs a fresh result set and rebuilds the per-file grouping.
// It must run on the main thread (Search marshals into it via gui.Post).
func (this *GlobalSearchPanel) applyResults(matches []SearchMatch) {
	this.results = matches
	this.grouped = make(map[string][]SearchMatch)
	this.fileOrder = nil
	for i := range matches {
		path := matches[i].FilePath
		if _, ok := this.grouped[path]; !ok {
			this.fileOrder = append(this.fileOrder, path)
		}
		this.grouped[path] = append(this.grouped[path], matches[i])
	}
}

// ReplaceAll rewrites every match of the current query with the replace text
// across the files the last search touched, using the filesearch engine to
// compute each file's new contents (the same case-insensitive literal matching
// the search used). A file whose contents are unchanged is not rewritten, and a
// read or write failure is surfaced via the log instead of being dropped
// silently. Afterwards the search is re-run synchronously so the results list
// reflects the new state.
func (this *GlobalSearchPanel) ReplaceAll() {
	if this.query == "" || len(this.fileOrder) == 0 {
		return
	}
	query := this.query
	replacement := string(this.replaceRunes)
	opt := filesearch.Options{IgnoreCase: true}

	// Snapshot the file set; the refresh below rebuilds fileOrder underneath us.
	files := make([]string, len(this.fileOrder))
	copy(files, this.fileOrder)

	failed := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			failed++
			core.Error(fmt.Sprintf("global search: read %s: %v", path, err))
			continue
		}
		// The query never spans a newline, so replacing across the whole file
		// is identical to replacing per matched line, and reuses the engine's
		// tested matcher rather than a bespoke pass.
		out := filesearch.Replace(filesearch.Match{Text: string(data)}, query, replacement, opt)
		if out == string(data) {
			continue // nothing changed; don't rewrite the file
		}
		if err := os.WriteFile(path, []byte(out), 0644); err != nil {
			failed++
			core.Error(fmt.Sprintf("global search: write %s: %v", path, err))
			continue
		}
	}
	if failed > 0 {
		core.Warn(fmt.Sprintf("global search: replace all left %d file(s) unwritten", failed))
	}

	// Refresh results against the rewritten files.
	this.applyResults(runSearch(this.rootDir, query))
	this.rebuildFlatRows()
	this.Self().Update()
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
	// Highlight the border of the focused input.
	if this.focusReplace {
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 100, G: 140, B: 200, A: 255}, 1)
	}
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

	// Replace input area (mirrors the search input, directly underneath).
	replaceY := inputY + inputH + 4
	replaceH := 24.0
	btnW := 64.0
	replaceW := inputW - btnW - 4

	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.Rectangle(inputX, replaceY, replaceW, replaceH)
	g.Fill()
	if this.focusReplace {
		g.SetPen1(paint.Color{R: 100, G: 140, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
	}
	g.Rectangle(inputX, replaceY, replaceW, replaceH)
	g.Stroke()

	g.SetFont(normalFont)
	if len(this.replaceRunes) > 0 {
		g.SetBrush1(t.TextColor)
		g.DrawText1(inputX+4, replaceY+replaceH-7, string(this.replaceRunes))
	} else {
		g.SetBrush1(paint.Color{R: 160, G: 160, B: 170, A: 180})
		g.DrawText1(inputX+4, replaceY+replaceH-7, "Replace...")
	}

	// Replace All button.
	btnX := inputX + replaceW + 4
	g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 255})
	g.Rectangle(btnX, replaceY, btnW, replaceH)
	g.Fill()
	g.SetFont(smallFont)
	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	btnLabel := "Replace All"
	btnExt := smallFont.TextExtents(btnLabel)
	g.DrawText1(btnX+(btnW-btnExt.XAdvance)/2, replaceY+replaceH-8, btnLabel)

	// Summary line
	summaryY := replaceY + replaceH + 4
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

			// m.Column and m.MatchLen are BYTE offsets into m.LineText (the
			// filesearch engine reports byte columns). Leading indent is ASCII,
			// so the byte trim offset maps straight onto the trimmed line. Slice
			// lineText by bytes so multibyte text before the match does not
			// shift the highlight (search and highlight agree on byte offsets).
			trimOffset := len(m.LineText) - len(strings.TrimLeft(m.LineText, " \t"))
			matchCol := m.Column - trimOffset
			if matchCol < 0 {
				matchCol = 0
			}

			g.SetFont(normalFont)

			if matchCol >= 0 && matchCol < len(lineText) && matchCol+m.MatchLen <= len(lineText) {
				// Pre-match text
				if matchCol > 0 {
					pre := lineText[:matchCol]
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, pre)
					preExt := normalFont.TextExtents(pre)
					textX += preExt.XAdvance
				}

				// Match highlight background
				matchText := lineText[matchCol : matchCol+m.MatchLen]
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
				if postStart < len(lineText) {
					post := lineText[postStart:]
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

// listTop returns the y coordinate where the results list begins. It accounts
// for the header, the search input, the replace input, and the summary line.
func (this *GlobalSearchPanel) listTop() float64 {
	headerH := 22.0
	inputH := 24.0
	replaceH := 24.0
	summaryH := 16.0
	return headerH + 4 + inputH + 4 + replaceH + 4 + summaryH + 2
}

// hitTest returns the row index for a given y coordinate, or -1.
func (this *GlobalSearchPanel) hitTest(y float64) int {
	listY := this.listTop()

	if y < listY {
		return -1
	}
	idx := int(math.Floor((y - listY + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.flatRows) {
		return -1
	}
	return idx
}

// isInSearchInput checks if a click is in the search input area.
func (this *GlobalSearchPanel) isInSearchInput(y float64) bool {
	headerH := 22.0
	inputY := headerH + 4
	inputH := 24.0
	return y >= inputY && y <= inputY+inputH
}

// isInReplaceInput checks if a click is in the replace input area (excluding
// the Replace All button to its right).
func (this *GlobalSearchPanel) isInReplaceInput(x, y float64) bool {
	headerH := 22.0
	inputH := 24.0
	replaceY := headerH + 4 + inputH + 4
	replaceH := 24.0
	w := this.Width()
	replaceW := (w - 16) - 64 - 4 // inputW - btnW - gap
	return y >= replaceY && y <= replaceY+replaceH && x >= 8 && x <= 8+replaceW
}

// isInReplaceButton checks if a click hits the Replace All button.
func (this *GlobalSearchPanel) isInReplaceButton(x, y float64) bool {
	headerH := 22.0
	inputH := 24.0
	replaceY := headerH + 4 + inputH + 4
	replaceH := 24.0
	w := this.Width()
	btnX := 8 + (w - 16) - 64 - 4 + 4 // inputX + replaceW + gap
	return y >= replaceY && y <= replaceY+replaceH && x >= btnX && x <= btnX+64
}

// OnLeftDown handles clicks.
func (this *GlobalSearchPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if this.isInSearchInput(y) {
		this.focusReplace = false
		this.Self().Update()
		return
	}
	if this.isInReplaceInput(x, y) {
		this.focusReplace = true
		this.Self().Update()
		return
	}
	if this.isInReplaceButton(x, y) {
		this.ReplaceAll()
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
	listY := this.listTop()
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

// OnTextInput handles typing into the focused (search or replace) input.
func (this *GlobalSearchPanel) OnTextInput(s string) {
	if this.focusReplace {
		this.replaceRunes = append(this.replaceRunes, []rune(s)...)
		this.replaceCursor = len(this.replaceRunes)
	} else {
		this.queryRunes = append(this.queryRunes, []rune(s)...)
		this.queryCursor = len(this.queryRunes)
	}
	this.Self().Update()
}

// OnKeyDown handles keyboard input.
func (this *GlobalSearchPanel) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyTab:
		// Toggle focus between the search and replace inputs.
		this.focusReplace = !this.focusReplace
		this.Self().Update()
	case gui.KeyBackSpace:
		if this.focusReplace {
			if len(this.replaceRunes) > 0 {
				this.replaceRunes = this.replaceRunes[:len(this.replaceRunes)-1]
				this.replaceCursor = len(this.replaceRunes)
				this.Self().Update()
			}
		} else if len(this.queryRunes) > 0 {
			this.queryRunes = this.queryRunes[:len(this.queryRunes)-1]
			this.queryCursor = len(this.queryRunes)
			this.Self().Update()
		}
	case gui.KeyEnter:
		// Enter in the replace field runs Replace All; otherwise it searches.
		if this.focusReplace {
			this.ReplaceAll()
		} else {
			this.Search(string(this.queryRunes))
		}
	case gui.KeyEsc:
		this.queryRunes = nil
		this.queryCursor = 0
		this.results = nil
		this.grouped = nil
		this.fileOrder = nil
		this.flatRows = nil
		this.focusReplace = false
		this.Self().Update()
	}
}

package ged

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"silk/paint"
)

// QuickOpenPopup provides a Ctrl+P file finder popup like VS Code.
type QuickOpenPopup struct {
	allFiles    []string
	filtered    []filteredFile
	filterText  string
	filterRunes []rune
	selectedIdx int
	scrollY     int
	visible     bool
	rootDir     string
	cbOpen      func(path string)
	maxVisible  int
}

// filteredFile pairs a file path with its match score for sorting.
type filteredFile struct {
	path     string
	relPath  string
	fileName string
	score    int
}

// quickOpen is the package-level instance.
var quickOpen *QuickOpenPopup

// GetQuickOpen returns the global QuickOpenPopup, creating it if needed.
func GetQuickOpen() *QuickOpenPopup {
	if quickOpen == nil {
		quickOpen = NewQuickOpenPopup()
	}
	return quickOpen
}

// NewQuickOpenPopup creates a new quick file finder popup.
func NewQuickOpenPopup() *QuickOpenPopup {
	return &QuickOpenPopup{
		maxVisible: 20,
	}
}

// SetRootDir sets the root directory and scans for files.
func (this *QuickOpenPopup) SetRootDir(dir string) {
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	this.rootDir = dir
	this.scanFiles()
}

// SetOpenCallback sets the callback for when a file is selected.
func (this *QuickOpenPopup) SetOpenCallback(fn func(path string)) {
	this.cbOpen = fn
}

// Show opens the popup and resets the filter.
func (this *QuickOpenPopup) Show() {
	this.visible = true
	this.filterText = ""
	this.filterRunes = nil
	this.selectedIdx = 0
	this.scrollY = 0
	this.filter()
}

// Dismiss closes the popup.
func (this *QuickOpenPopup) Dismiss() {
	this.visible = false
	this.filterText = ""
	this.filterRunes = nil
}

// Visible returns whether the popup is currently shown.
func (this *QuickOpenPopup) Visible() bool {
	return this.visible
}

// scanFiles walks the root directory and collects all files.
func (this *QuickOpenPopup) scanFiles() {
	this.allFiles = nil
	if this.rootDir == "" {
		return
	}

	_ = filepath.Walk(this.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() && skipDirs[name] {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		this.allFiles = append(this.allFiles, path)
		return nil
	})
}

// filter narrows files by the current filter text using fuzzy matching.
func (this *QuickOpenPopup) filter() {
	this.filtered = nil

	for _, path := range this.allFiles {
		relPath := path
		if this.rootDir != "" {
			if rel, err := filepath.Rel(this.rootDir, path); err == nil {
				relPath = rel
			}
		}
		fileName := filepath.Base(path)

		if this.filterText == "" {
			this.filtered = append(this.filtered, filteredFile{
				path:     path,
				relPath:  relPath,
				fileName: fileName,
				score:    0,
			})
		} else {
			score := fuzzyScore(this.filterText, fileName, relPath)
			if score > 0 {
				this.filtered = append(this.filtered, filteredFile{
					path:     path,
					relPath:  relPath,
					fileName: fileName,
					score:    score,
				})
			}
		}
	}

	// Sort by score descending, then by file name
	sort.Slice(this.filtered, func(i, j int) bool {
		if this.filtered[i].score != this.filtered[j].score {
			return this.filtered[i].score > this.filtered[j].score
		}
		return strings.ToLower(this.filtered[i].fileName) < strings.ToLower(this.filtered[j].fileName)
	})

	// Limit results
	if len(this.filtered) > this.maxVisible {
		this.filtered = this.filtered[:this.maxVisible]
	}

	if this.selectedIdx >= len(this.filtered) {
		this.selectedIdx = 0
	}
	this.scrollY = 0
}

// fuzzyScore computes a match score for query against fileName and relPath.
// Returns 0 if no match. Higher is better.
func fuzzyScore(query, fileName, relPath string) int {
	lowerQuery := strings.ToLower(query)
	lowerName := strings.ToLower(fileName)
	lowerPath := strings.ToLower(relPath)

	// Exact prefix match on filename: highest score
	if strings.HasPrefix(lowerName, lowerQuery) {
		return 1000 + (100 - len(fileName))
	}

	// Substring match on filename
	if strings.Contains(lowerName, lowerQuery) {
		return 500 + (100 - len(fileName))
	}

	// Substring match on full path
	if strings.Contains(lowerPath, lowerQuery) {
		return 300 + (100 - len(relPath))
	}

	// Fuzzy match: each character in query must appear in order in filename
	qi := 0
	qRunes := []rune(lowerQuery)
	nameRunes := []rune(lowerName)
	consecutive := 0
	bonus := 0

	for ni := 0; ni < len(nameRunes) && qi < len(qRunes); ni++ {
		if nameRunes[ni] == qRunes[qi] {
			qi++
			consecutive++
			bonus += consecutive * 2
			if ni == qi-1 { // match at start
				bonus += 5
			}
		} else {
			consecutive = 0
		}
	}
	if qi == len(qRunes) {
		return 100 + bonus
	}

	// Try fuzzy on full path as fallback
	qi = 0
	consecutive = 0
	bonus = 0
	pathRunes := []rune(lowerPath)
	for pi := 0; pi < len(pathRunes) && qi < len(qRunes); pi++ {
		if pathRunes[pi] == qRunes[qi] {
			qi++
			consecutive++
			bonus += consecutive
		} else {
			consecutive = 0
		}
	}
	if qi == len(qRunes) {
		return 50 + bonus
	}

	return 0 // no match
}

// SelectNext moves selection down.
func (this *QuickOpenPopup) SelectNext() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectedIdx++
	if this.selectedIdx >= len(this.filtered) {
		this.selectedIdx = 0
	}
}

// SelectPrev moves selection up.
func (this *QuickOpenPopup) SelectPrev() {
	if len(this.filtered) == 0 {
		return
	}
	this.selectedIdx--
	if this.selectedIdx < 0 {
		this.selectedIdx = len(this.filtered) - 1
	}
}

// Accept opens the selected file and closes the popup.
func (this *QuickOpenPopup) Accept() {
	if !this.visible || len(this.filtered) == 0 {
		return
	}
	if this.selectedIdx >= 0 && this.selectedIdx < len(this.filtered) {
		path := this.filtered[this.selectedIdx].path
		this.Dismiss()
		if this.cbOpen != nil {
			this.cbOpen(path)
		}
	}
}

// OnTextInput handles typing in the filter field.
func (this *QuickOpenPopup) OnTextInput(s string) {
	this.filterRunes = append(this.filterRunes, []rune(s)...)
	this.filterText = string(this.filterRunes)
	this.filter()
}

// OnBackspace handles backspace in the filter field.
func (this *QuickOpenPopup) OnBackspace() {
	if len(this.filterRunes) > 0 {
		this.filterRunes = this.filterRunes[:len(this.filterRunes)-1]
		this.filterText = string(this.filterRunes)
		this.filter()
	}
}

// fileTypeColorForExt returns a color dot for the file extension.
func fileTypeColorForExt(name string) paint.Color {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return paint.Color{R: 0, G: 173, B: 131, A: 255}
	case ".mod", ".sum":
		return paint.Color{R: 230, G: 140, B: 50, A: 255}
	case ".md", ".txt":
		return paint.Color{R: 80, G: 140, B: 220, A: 255}
	case ".json", ".yaml", ".yml", ".toml":
		return paint.Color{R: 200, G: 180, B: 60, A: 255}
	default:
		return paint.Color{R: 160, G: 160, B: 170, A: 255}
	}
}

// DrawPopup renders the quick-open popup as an overlay on the given widget.
func (this *QuickOpenPopup) DrawPopup(g paint.Painter, hostW, hostH float64) {
	if !this.visible {
		return
	}

	font := paint.NewFont("Menlo", 12, false, false)
	boldFont := paint.NewFont("Menlo", 12, true, false)
	smallFont := paint.NewFont("Menlo", 10, false, false)
	fe := font.FontExtents()

	// Popup dimensions: 60% width, up to 400px height
	popupW := hostW * 0.6
	if popupW < 300 {
		popupW = 300
	}
	if popupW > hostW-40 {
		popupW = hostW - 40
	}

	itemH := fe.Height + 8
	inputH := fe.Height + 12
	visibleCount := len(this.filtered)
	if visibleCount > this.maxVisible {
		visibleCount = this.maxVisible
	}
	contentH := float64(visibleCount) * itemH
	popupH := inputH + contentH + 4
	if popupH > 400 {
		popupH = 400
		contentH = popupH - inputH - 4
		visibleCount = int(contentH / itemH)
	}

	px := (hostW - popupW) / 2
	py := 40.0

	// Shadow
	g.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 80})
	g.Rectangle(px+3, py+3, popupW, popupH)
	g.Fill()

	// Background
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 45, A: 250})
	g.Rectangle(px, py, popupW, popupH)
	g.Fill()

	// Border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 100, A: 255}, 1)
	g.Rectangle(px, py, popupW, popupH)
	g.Stroke()

	// Input field
	inputX := px + 8
	inputY := py + 4
	inputW := popupW - 16

	g.SetBrush1(paint.Color{R: 28, G: 28, B: 35, A: 255})
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Fill()
	g.SetPen1(paint.Color{R: 80, G: 80, B: 120, A: 255}, 1)
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Stroke()

	g.SetFont(font)
	if this.filterText != "" {
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 220, A: 255})
		g.DrawText1(inputX+6, inputY+fe.Ascent+4, this.filterText)
	} else {
		g.SetBrush1(paint.Color{R: 100, G: 100, B: 120, A: 180})
		g.DrawText1(inputX+6, inputY+fe.Ascent+4, "Type to search files...")
	}

	// Cursor
	prefix := this.filterText
	fcx := inputX + 6 + font.TextExtents(prefix).XAdvance
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
	g.Rectangle(fcx, inputY+3, 1, inputH-6)
	g.Fill()

	// File list
	listY := inputY + inputH + 2

	for vi := 0; vi < visibleCount; vi++ {
		idx := this.scrollY + vi
		if idx >= len(this.filtered) {
			break
		}
		ff := this.filtered[idx]
		iy := listY + float64(vi)*itemH

		// Selection highlight
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 50, G: 80, B: 160, A: 255})
			g.Rectangle(px+2, iy, popupW-4, itemH)
			g.Fill()
		}

		// File type color dot
		dotColor := fileTypeColorForExt(ff.fileName)
		g.SetBrush1(dotColor)
		dotX := px + 12
		dotY := iy + itemH/2
		g.Arc(dotX, dotY, 3, 0, 6.283185307)
		g.Fill()

		// File name (bold)
		g.SetFont(boldFont)
		if idx == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 215, A: 255})
		}
		g.DrawText1(px+22, iy+fe.Ascent+4, ff.fileName)

		// Directory path (dimmed)
		nameExt := boldFont.TextExtents(ff.fileName)
		dir := filepath.Dir(ff.relPath)
		if dir != "." && dir != "" {
			g.SetFont(smallFont)
			g.SetBrush1(paint.Color{R: 100, G: 100, B: 120, A: 180})
			g.DrawText1(px+22+nameExt.XAdvance+8, iy+fe.Ascent+4, dir)
		}
	}
}

package ged

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"silk/core"
	"silk/geom"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.EditorTabs", gui.TypeOf(EditorTabs{}))
}

// editorTab represents a single open file tab with its associated editor.
type editorTab struct {
	filePath string
	fileName string
	editor   *gui.CodeEditor
	modified bool
}

// EditorTabs is a multi-tab code editor widget. Each tab holds a CodeEditor
// for one file. It draws a Chrome-style tab bar at the top and the active
// editor below.
type EditorTabs struct {
	gui.Widget

	tabs      []*editorTab
	activeIdx int
	tabBarH   float64

	hoverTab   int
	hoverClose int // which tab's close button is hovered (-1 = none)
	scrollX    float64

	// --- Split View ---
	splitMode     bool
	splitEditor   *gui.CodeEditor
	splitRatio    float64 // 0.5 = equal split
	splitVertical bool    // true = side-by-side (left/right), false = stacked (top/bottom)
}

func NewEditorTabs() *EditorTabs {
	p := new(EditorTabs)
	p.Init(p)
	return p
}

func (this *EditorTabs) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.tabBarH = 30
	this.hoverTab = -1
	this.hoverClose = -1
	this.activeIdx = -1
}

// Title returns a display title for the dock tab bar.
func (this *EditorTabs) Title() string {
	if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
		return this.tabs[this.activeIdx].fileName
	}
	return "编辑器"
}

// OpenFile opens a file in a new tab, or switches to an existing tab if the
// file is already open.
func (this *EditorTabs) OpenFile(path string) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}

	// Check if already open
	for i, tab := range this.tabs {
		if tab.filePath == path {
			this.switchToTab(i)
			return
		}
	}

	// Read file content
	data, err := os.ReadFile(path)
	if err != nil {
		core.Warn("EditorTabs: cannot read file: ", err)
		return
	}

	// Create editor
	editor := gui.NewCodeEditor()
	editor.SetParent(this.Self())
	editor.Hide() // hidden until activated
	editor.SetText(string(data))
	editor.SetFilePath(path)

	// Wire up cross-file navigation: when Cmd/Ctrl+Click targets a different file
	editor.SetNavigateCallback(func(targetFile string, targetLine int) {
		this.OpenFileAtLine(targetFile, targetLine)
	})

	tab := &editorTab{
		filePath: path,
		fileName: filepath.Base(path),
		editor:   editor,
		modified: false,
	}

	// Track modifications
	editor.SigChanged(func(text string) {
		tab.modified = true
		this.Self().Update()
	})

	this.tabs = append(this.tabs, tab)
	this.switchToTab(len(this.tabs) - 1)
}

// OpenFileAtLine opens a file and scrolls to the given line (0-based).
// If the file is already open, it switches to that tab and scrolls.
func (this *EditorTabs) OpenFileAtLine(path string, line int) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}

	// Check if already open
	for i, tab := range this.tabs {
		if tab.filePath == path {
			this.switchToTab(i)
			tab.editor.ScrollToLine(line)
			return
		}
	}

	// Open the file first, then scroll
	this.OpenFile(path)
	if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
		this.tabs[this.activeIdx].editor.ScrollToLine(line)
	}
}

// switchToTab activates the given tab index.
func (this *EditorTabs) switchToTab(idx int) {
	if idx < 0 || idx >= len(this.tabs) {
		return
	}
	// Hide current
	if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
		this.tabs[this.activeIdx].editor.Hide()
	}
	this.activeIdx = idx
	// Show new
	this.tabs[idx].editor.Show()
	this.Layout()
	this.Self().Update()
}

// CloseTab closes the tab at the given index.
func (this *EditorTabs) CloseTab(idx int) {
	if idx < 0 || idx >= len(this.tabs) {
		return
	}

	tab := this.tabs[idx]
	// Detach editor from parent
	tab.editor.SetParent(nil)

	// Remove from list
	this.tabs = append(this.tabs[:idx], this.tabs[idx+1:]...)

	// Adjust active index
	if len(this.tabs) == 0 {
		this.activeIdx = -1
	} else if this.activeIdx >= len(this.tabs) {
		this.switchToTab(len(this.tabs) - 1)
	} else if this.activeIdx == idx {
		// Activate same index (now next tab) or previous
		if this.activeIdx >= len(this.tabs) {
			this.switchToTab(len(this.tabs) - 1)
		} else {
			this.switchToTab(this.activeIdx)
		}
	} else if this.activeIdx > idx {
		this.activeIdx--
	}

	this.Layout()
	this.Self().Update()
}

// ActiveEditor returns the CodeEditor of the currently active tab, or nil.
func (this *EditorTabs) ActiveEditor() *gui.CodeEditor {
	if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
		return this.tabs[this.activeIdx].editor
	}
	return nil
}

// SaveCurrent saves the active tab's file to disk.
func (this *EditorTabs) SaveCurrent() {
	if this.activeIdx < 0 || this.activeIdx >= len(this.tabs) {
		return
	}
	tab := this.tabs[this.activeIdx]
	content := tab.editor.Text()
	err := os.WriteFile(tab.filePath, []byte(content), 0644)
	if err != nil {
		core.Warn("EditorTabs: save failed: ", err)
		return
	}
	tab.modified = false
	tab.editor.RefreshGitStatus()
	this.Self().Update()
}

// SaveAll saves all modified tabs.
func (this *EditorTabs) SaveAll() {
	for _, tab := range this.tabs {
		if tab.modified {
			content := tab.editor.Text()
			err := os.WriteFile(tab.filePath, []byte(content), 0644)
			if err != nil {
				core.Warn("EditorTabs: save failed: ", err)
				continue
			}
			tab.modified = false
			tab.editor.RefreshGitStatus()
		}
	}
	this.Self().Update()
}

// splitDividerSize is the gap (in pixels) reserved between the two split
// panes for the drag divider.
const splitDividerSize = 4.0

// splitPaneRects computes the two pane rectangles for a split editor area.
// The area (x, y, w, h) is divided into a primary and secondary rect with a
// `gap`-pixel divider between them: left/right when vertical is true,
// top/bottom otherwise. `ratio` is the primary pane's fraction of the usable
// space (clamped to a sane range); each pane is kept to a small minimum so it
// never collapses. This is pure math so it can be unit-tested without GL.
func splitPaneRects(x, y, w, h float64, vertical bool, ratio, gap float64) (primary, secondary geom.Rect) {
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	const minPane = 20.0
	if vertical {
		usable := w - gap
		if usable < 0 {
			usable = 0
		}
		leftW := usable * ratio
		rightW := usable - leftW
		if leftW < minPane {
			leftW = minPane
		}
		if rightW < minPane {
			rightW = minPane
		}
		primary = geom.Rect{X: x, Y: y, Width: leftW, Height: h}
		secondary = geom.Rect{X: x + leftW + gap, Y: y, Width: rightW, Height: h}
		return
	}
	usable := h - gap
	if usable < 0 {
		usable = 0
	}
	topH := usable * ratio
	bottomH := usable - topH
	if topH < minPane {
		topH = minPane
	}
	if bottomH < minPane {
		bottomH = minPane
	}
	primary = geom.Rect{X: x, Y: y, Width: w, Height: topH}
	secondary = geom.Rect{X: x, Y: y + topH + gap, Width: w, Height: bottomH}
	return
}

// Layout positions the tab bar and the active editor(s).
func (this *EditorTabs) Layout() {
	w, h := this.Size()
	editorY := this.tabBarH
	editorH := h - editorY
	if editorH < 0 {
		editorH = 0
	}

	if this.splitMode && this.splitEditor != nil && this.activeIdx >= 0 {
		// Split layout: active tab in the primary pane, splitEditor in the
		// secondary pane, with a divider gap between them.
		primary, secondary := splitPaneRects(0, editorY, w, editorH, this.splitVertical, this.splitRatio, splitDividerSize)

		for i, tab := range this.tabs {
			if i == this.activeIdx {
				tab.editor.SetBounds(primary.X, primary.Y, primary.Width, primary.Height)
				tab.editor.Show()
			} else {
				tab.editor.Hide()
			}
		}
		this.splitEditor.SetBounds(secondary.X, secondary.Y, secondary.Width, secondary.Height)
		this.splitEditor.Show()
	} else {
		// Normal single-editor layout
		for i, tab := range this.tabs {
			if i == this.activeIdx {
				tab.editor.SetBounds(0, editorY, w, editorH)
				tab.editor.Show()
			} else {
				tab.editor.Hide()
			}
		}
		if this.splitEditor != nil {
			this.splitEditor.Hide()
		}
	}
}

// ToggleSplit toggles the split editor view on or off.
func (this *EditorTabs) ToggleSplit() {
	if this.splitMode {
		// Close split — restore original callback to prevent leak
		this.splitMode = false
		if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
			tab := this.tabs[this.activeIdx]
			// Reset to the standard modification-tracking callback
			tab.editor.SigChanged(func(text string) {
				tab.modified = true
				this.Self().Update()
			})
		}
		if this.splitEditor != nil {
			this.splitEditor.SigChanged(nil)
			this.splitEditor.SetParent(nil)
			this.splitEditor = nil
		}
		this.Layout()
		this.Self().Update()
		return
	}

	// Open split: clone the current editor content into a secondary editor
	if this.activeIdx < 0 || this.activeIdx >= len(this.tabs) {
		return
	}
	tab := this.tabs[this.activeIdx]
	this.splitEditor = gui.NewCodeEditor()
	this.splitEditor.SetParent(this.Self())
	this.splitEditor.SetText(tab.editor.Text())
	this.splitRatio = 0.5
	this.splitVertical = true // default to side-by-side (left/right)
	this.splitMode = true

	// Sync content changes: when primary changes, update secondary
	origChanged := tab.editor.SigChangedFn()
	tab.editor.SigChanged(func(text string) {
		if origChanged != nil {
			origChanged(text)
		}
		if this.splitEditor != nil && this.splitMode {
			// Preserve secondary scroll position
			sy := this.splitEditor.ScrollY()
			this.splitEditor.SetText(text)
			this.splitEditor.SetScrollY(sy)
		}
	})

	// Sync in reverse: when secondary changes, update primary
	this.splitEditor.SigChanged(func(text string) {
		if this.activeIdx >= 0 && this.activeIdx < len(this.tabs) {
			primary := this.tabs[this.activeIdx].editor
			sy := primary.ScrollY()
			primary.SetText(text)
			primary.SetScrollY(sy)
			this.tabs[this.activeIdx].modified = true
		}
	})

	this.Layout()
	this.Self().Update()
}

// IsSplit reports whether the split editor view is currently active.
func (this *EditorTabs) IsSplit() bool {
	return this.splitMode
}

// OnResize re-layouts when the widget size changes.
func (this *EditorTabs) OnResize() {
	this.Layout()
}

// tabWidth returns the pixel width of a tab title.
func (this *EditorTabs) tabWidth(tab *editorTab) float64 {
	base := 12.0 // left padding
	nameW := float64(len([]rune(tab.fileName))) * 7.5
	if nameW < 40 {
		nameW = 40
	}
	if nameW > 140 {
		nameW = 140
	}
	closeW := 20.0 // close button space
	rightPad := 8.0
	return base + nameW + closeW + rightPad
}

// tabHitTest returns (tabIndex, isCloseButton) for a point in the tab bar.
func (this *EditorTabs) tabHitTest(x, y float64) (int, bool) {
	if y < 0 || y >= this.tabBarH {
		return -1, false
	}
	tx := -this.scrollX
	for i, tab := range this.tabs {
		tw := this.tabWidth(tab)
		if x >= tx && x < tx+tw {
			// Check close button (right side of tab)
			closeX := tx + tw - 20
			if x >= closeX && x < closeX+16 {
				return i, true
			}
			return i, false
		}
		tx += tw
	}
	return -1, false
}

// Draw renders the tab bar and active editor background.
func (this *EditorTabs) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Tab bar background
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, this.tabBarH)
	g.Fill()

	// Bottom line of tab bar
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, this.tabBarH)
	g.LineTo(w, this.tabBarH)
	g.Stroke()

	if len(this.tabs) == 0 {
		// Empty state: show placeholder
		g.SetBrush1(t.ViewBGColor)
		g.Rectangle(0, this.tabBarH, w, h-this.tabBarH)
		g.Fill()

		placeholderFont := paint.NewFont(t.Font.Family(), 13, false, false)
		g.SetFont(placeholderFont)
		g.SetBrush1(paint.Color{R: 160, G: 160, B: 170, A: 255})
		msg := "双击文件浏览器中的文件以打开"
		g.DrawText1(w/2-100, h/2, msg)
		return
	}

	tabFont := paint.NewFont(t.Font.Family(), 11, false, false)
	tabFontBold := paint.NewFont(t.Font.Family(), 11, true, false)

	// Draw each tab
	tx := -this.scrollX
	for i, tab := range this.tabs {
		tw := this.tabWidth(tab)
		isActive := i == this.activeIdx
		isHover := i == this.hoverTab

		// Tab background
		if isActive {
			g.SetBrush1(t.ViewBGColor) // white/light for active
			g.Rectangle(tx, 2, tw, this.tabBarH-2)
			g.Fill()
			// Active indicator line at top
			g.SetBrush1(paint.Color{R: 51, G: 120, B: 215, A: 255})
			g.Rectangle(tx, 0, tw, 2)
			g.Fill()
		} else if isHover {
			g.SetBrush1(paint.Color{R: 225, G: 228, B: 235, A: 255})
			g.Rectangle(tx, 2, tw, this.tabBarH-2)
			g.Fill()
		}

		// Tab text
		if isActive {
			g.SetFont(tabFontBold)
		} else {
			g.SetFont(tabFont)
		}

		// File type color dot
		ext := strings.ToLower(filepath.Ext(tab.fileName))
		dotColor := paint.Color{R: 160, G: 160, B: 170, A: 255}
		switch ext {
		case ".go":
			dotColor = paint.Color{R: 0, G: 173, B: 131, A: 255}
		case ".mod", ".sum":
			dotColor = paint.Color{R: 230, G: 140, B: 50, A: 255}
		case ".md", ".txt":
			dotColor = paint.Color{R: 80, G: 140, B: 220, A: 255}
		}
		dotX := tx + 8
		dotY := this.tabBarH/2 + 1
		g.Save()
		g.SetBrush1(dotColor)
		g.Arc(dotX, dotY, 3, 0, 2*math.Pi)
		g.Fill()
		g.Restore()

		// Tab name
		nameX := tx + 16
		nameY := this.tabBarH - 9
		if isActive {
			g.SetBrush1(t.TextColor)
		} else {
			g.SetBrush1(paint.Color{R: 100, G: 100, B: 110, A: 255})
		}

		displayName := tab.fileName
		if tab.modified {
			displayName = tab.fileName + " *"
		}
		g.DrawText1(nameX, nameY, displayName)

		// Close button (X)
		closeX := tx + tw - 18
		closeY := this.tabBarH/2 - 5
		if i == this.hoverClose {
			g.SetBrush1(paint.Color{R: 220, G: 100, B: 100, A: 200})
			g.Arc(closeX+5, closeY+5, 7, 0, 2*math.Pi)
			g.Fill()
			g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 255}, 1.5)
		} else {
			g.SetPen1(paint.Color{R: 150, G: 150, B: 160, A: 200}, 1.2)
		}
		// Draw X
		g.MoveTo(closeX+2, closeY+2)
		g.LineTo(closeX+8, closeY+8)
		g.Stroke()
		g.MoveTo(closeX+8, closeY+2)
		g.LineTo(closeX+2, closeY+8)
		g.Stroke()

		// Tab separator
		if !isActive {
			g.SetPen1(paint.Color{R: 210, G: 210, B: 215, A: 120}, 1)
			g.MoveTo(tx+tw, 6)
			g.LineTo(tx+tw, this.tabBarH-6)
			g.Stroke()
		}

		tx += tw
	}

	// The active editor is drawn as a child widget, but we paint the
	// background below the tab bar in case there's any gap.
	if this.activeIdx >= 0 {
		g.SetBrush1(t.ViewBGColor)
		g.Rectangle(0, this.tabBarH, w, h-this.tabBarH)
		g.Fill()
	}

	// Draw split divider if split mode is active. The divider occupies the
	// gap between the primary and secondary panes (computed by the same
	// helper Layout uses, so the two never drift apart).
	if this.splitMode && this.splitEditor != nil && this.activeIdx >= 0 {
		editorY := this.tabBarH
		editorH := h - editorY
		primary, _ := splitPaneRects(0, editorY, w, editorH, this.splitVertical, this.splitRatio, splitDividerSize)

		var divX, divY, divW, divH float64
		if this.splitVertical {
			divX, divY = primary.X+primary.Width, editorY
			divW, divH = splitDividerSize, editorH
		} else {
			divX, divY = 0, primary.Y+primary.Height
			divW, divH = w, splitDividerSize
		}

		// Divider background
		g.SetBrush1(paint.Color{R: 180, G: 185, B: 200, A: 255})
		g.Rectangle(divX, divY, divW, divH)
		g.Fill()

		// Drag handle dots in the divider center
		centerX := divX + divW/2
		centerY := divY + divH/2
		g.SetBrush1(paint.Color{R: 120, G: 125, B: 140, A: 255})
		if this.splitVertical {
			for dy := -8.0; dy <= 8.0; dy += 4.0 {
				g.Arc(centerX, centerY+dy, 1.2, 0, 2*math.Pi)
				g.Fill()
			}
		} else {
			for dx := -8.0; dx <= 8.0; dx += 4.0 {
				g.Arc(centerX+dx, centerY, 1.2, 0, 2*math.Pi)
				g.Fill()
			}
		}
	}

	// Quick-open popup overlay
	qo := GetQuickOpen()
	if qo.Visible() {
		qo.DrawPopup(g, w, h)
	}
}

// OnLeftDown handles tab clicks and close button clicks.
func (this *EditorTabs) OnLeftDown(x, y float64) {
	this.SetFocus()
	if y < this.tabBarH {
		idx, isClose := this.tabHitTest(x, y)
		if idx >= 0 {
			if isClose {
				this.CloseTab(idx)
			} else {
				this.switchToTab(idx)
			}
		}
		return
	}
	// Forward to active editor (handled by child widget dispatch)
}

// OnMouseMove updates hover state for tab bar.
func (this *EditorTabs) OnMouseMove(x, y float64) {
	if y < this.tabBarH {
		idx, isClose := this.tabHitTest(x, y)
		changed := false
		if idx != this.hoverTab {
			this.hoverTab = idx
			changed = true
		}
		closeIdx := -1
		if isClose {
			closeIdx = idx
		}
		if closeIdx != this.hoverClose {
			this.hoverClose = closeIdx
			changed = true
		}
		if changed {
			this.Self().Update()
		}
		return
	}
	// Reset tab hover if mouse is below tab bar
	if this.hoverTab != -1 || this.hoverClose != -1 {
		this.hoverTab = -1
		this.hoverClose = -1
		this.Self().Update()
	}
}

// OnMouseLeave resets hover state.
func (this *EditorTabs) OnMouseLeave() {
	if this.hoverTab != -1 || this.hoverClose != -1 {
		this.hoverTab = -1
		this.hoverClose = -1
		this.Self().Update()
	}
}

// OnKeyDown handles keyboard shortcuts: Ctrl+W close tab, Ctrl+S save, Ctrl+\ split.
func (this *EditorTabs) OnKeyDown(key int, repeat bool) {
	// Intercept keys when QuickOpen popup is visible
	qo := GetQuickOpen()
	if qo.Visible() {
		switch key {
		case gui.KeyUp:
			qo.SelectPrev()
		case gui.KeyDown:
			qo.SelectNext()
		case gui.KeyEnter:
			qo.Accept()
		case gui.KeyEsc:
			qo.Dismiss()
		case gui.KeyBackSpace:
			qo.OnBackspace()
		}
		this.Self().Update()
		return
	}

	ctrl := gui.IsKeyDown(gui.KeyCtrl)
	if ctrl {
		switch key {
		case 'W':
			if this.activeIdx >= 0 {
				this.CloseTab(this.activeIdx)
			}
		case 'S':
			this.SaveCurrent()
		case 0x5C: // backslash '\' -- Ctrl+\ to toggle split
			this.ToggleSplit()
		case 'P', 'p':
			if QuickOpenCallback != nil {
				QuickOpenCallback()
				this.Self().Update()
			}
		}
	}
}

// OnTextInput forwards text input to QuickOpen popup when visible.
func (this *EditorTabs) OnTextInput(s string) {
	qo := GetQuickOpen()
	if qo.Visible() {
		qo.OnTextInput(s)
		this.Self().Update()
	}
}

// TabCount returns the number of open tabs.
func (this *EditorTabs) TabCount() int {
	return len(this.tabs)
}

// OpenFilePaths returns the absolute paths of all files currently open in
// tabs. Returned in tab order (left to right). Empty if no tabs are open.
func (this *EditorTabs) OpenFilePaths() []string {
	if len(this.tabs) == 0 {
		return nil
	}
	paths := make([]string, 0, len(this.tabs))
	for _, tab := range this.tabs {
		if tab != nil && tab.filePath != "" {
			paths = append(paths, tab.filePath)
		}
	}
	return paths
}

// ActiveFilePath returns the absolute path of the file in the active tab,
// or an empty string if no tab is active.
func (this *EditorTabs) ActiveFilePath() string {
	if this.activeIdx < 0 || this.activeIdx >= len(this.tabs) {
		return ""
	}
	tab := this.tabs[this.activeIdx]
	if tab == nil {
		return ""
	}
	return tab.filePath
}

// ActivateFile switches to the tab that holds the given file path if one
// is open, otherwise does nothing. Paths are compared after absolute-path
// resolution so callers can pass relative paths safely.
func (this *EditorTabs) ActivateFile(path string) bool {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	for i, tab := range this.tabs {
		if tab != nil && tab.filePath == path {
			this.switchToTab(i)
			return true
		}
	}
	return false
}

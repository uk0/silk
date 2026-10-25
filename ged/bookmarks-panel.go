package ged

import (
	"github.com/uk0/silk/i18n"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.BookmarksPanel", gui.TypeOf(BookmarksPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.BookmarksPanel",
		Name: "书签",
		Icon: "bookmarks",
		Desc: "跨文件代码书签列表",
	})
}

// Bookmark is a single code bookmark: a labelled (file, line) location.
type Bookmark struct {
	File  string
	Line  int
	Label string
}

// BookmarksPanel is the cross-file aggregate view of code bookmarks,
// modelled on Qt Creator's Bookmarks pane. The CodeEditor keeps its
// own per-file bookmark concept; this panel does not read the editor
// directly — the host pushes bookmarks in via Add/Remove/Toggle and
// the panel renders them as a flat, predictably-ordered list.
//
// Clicking a row fires SigActivated(file, line) so the host can jump
// the editor to the bookmarked location.
type BookmarksPanel struct {
	gui.Widget

	marks       []Bookmark
	scrollY     float64
	hoverIdx    int
	rowHeight   float64
	cbActivated func(file string, line int)
	cbRemove    func(file string, line int)
	selectedIdx int
}

// NewBookmarksPanel creates an empty bookmarks panel.
func NewBookmarksPanel() *BookmarksPanel {
	p := new(BookmarksPanel)
	p.Init(p)
	return p
}

func (this *BookmarksPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.hoverIdx = -1
	this.selectedIdx = -1
	this.SetFocusPolicy(gui.TabFocus)
}

// sortBookmarks returns a stably sorted copy of the given bookmarks,
// ordered by file then line. Pulled out as a pure helper so the
// ordering can be unit-tested without a widget or GL context.
func sortBookmarks(in []Bookmark) []Bookmark {
	out := make([]Bookmark, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// indexOf returns the slice index of the bookmark at (file, line), or -1.
func (this *BookmarksPanel) indexOf(file string, line int) int {
	for i := range this.marks {
		if this.marks[i].File == file && this.marks[i].Line == line {
			return i
		}
	}
	return -1
}

// Add inserts a bookmark at (file, line). If one already exists for that
// location its label is updated rather than adding a duplicate.
func (this *BookmarksPanel) Add(file string, line int, label string) {
	rows := this.Bookmarks()
	if idx := this.indexOf(file, line); idx >= 0 {
		rows[idx].Label = label
	} else {
		rows = append(rows, Bookmark{file, line, label})
	}
	this.SetBookmarks(rows)
}

// SetBookmarks atomically replaces the view model, merging duplicate locations
// and preserving selection by identity. UI-thread-only. Only visible rows draw.
func (this *BookmarksPanel) SetBookmarks(rows []Bookmark) {
	var selected Bookmark
	if this.selectedIdx >= 0 && this.selectedIdx < len(this.marks) {
		selected = this.marks[this.selectedIdx]
	}
	next := sortBookmarks(rows)
	out := next[:0]
	for _, row := range next {
		if row.File == "" || row.Line < 1 {
			continue
		}
		if len(out) > 0 && out[len(out)-1].File == row.File && out[len(out)-1].Line == row.Line {
			out[len(out)-1] = row
		} else {
			out = append(out, row)
		}
	}
	if slices.Equal(this.marks, out) {
		return
	}
	this.marks = out
	this.selectedIdx = this.indexOf(selected.File, selected.Line)
	this.hoverIdx = -1
	this.clampScroll()
	this.Update()
}
func (this *BookmarksPanel) Remove(file string, line int) {
	rows := this.Bookmarks()
	i := this.indexOf(file, line)
	if i < 0 {
		return
	}
	this.SetBookmarks(slices.Delete(rows, i, i+1))
}

// Toggle adds the bookmark at (file, line) when absent and removes it
// when present.
func (this *BookmarksPanel) Toggle(file string, line int, label string) {
	if this.indexOf(file, line) >= 0 {
		this.Remove(file, line)
	} else {
		this.Add(file, line, label)
	}
}

// Bookmarks returns the bookmarks, stably sorted by file then line.
func (this *BookmarksPanel) Bookmarks() []Bookmark {
	return slices.Clone(this.marks)
}

// Clear removes all bookmarks.
func (this *BookmarksPanel) Clear() { this.SetBookmarks(nil) }

// SigActivated registers the callback invoked when a row is clicked.
func (this *BookmarksPanel) SigActivated(fn func(file string, line int)) {
	this.cbActivated = fn
}

// SigRemoveRequested leaves ownership with the host, allowing the view and
// gutter/persistent store to change together. Delete does not edit source text.
func (this *BookmarksPanel) SigRemoveRequested(fn func(string, int)) { this.cbRemove = fn }
func (this *BookmarksPanel) headerHeight() float64 {
	return max(22, float64(gui.Theme().Font.Size())+8)
}
func (this *BookmarksPanel) clampScroll() {
	this.rowHeight = this.headerHeight()
	this.scrollY = max(0, min(this.scrollY, float64(len(this.marks))*this.rowHeight-max(0, this.Height()-this.headerHeight())))
}
func (this *BookmarksPanel) visibleRows(h float64) (int, int) {
	first := max(0, min(len(this.marks), int(this.scrollY/this.rowHeight)))
	last := max(first, min(len(this.marks), int(math.Ceil((this.scrollY+max(0, h-this.headerHeight()))/this.rowHeight))))
	return first, last
}
func (this *BookmarksPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	if w <= 0 || h <= 0 {
		return
	}
	this.clampScroll()
	t := gui.Theme()
	hh := this.headerHeight()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, hh)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.MoveTo(0, hh-.5)
	g.LineTo(w, hh-.5)
	g.Stroke()
	drawPanelText(g, t.Font, i18n.T("Bookmarks")+" ("+strconv.Itoa(len(this.marks))+")", 8, 0, w-16, hh, t.TextColor)
	if h <= hh {
		return
	}
	g.Rectangle(0, hh, w, h-hh)
	g.Clip()
	if len(this.marks) == 0 {
		drawPanelText(g, t.Font, i18n.T("No bookmarks"), 8, hh, w-16, 26, t.TextColor)
		return
	}
	first, last := this.visibleRows(h)
	for i := first; i < last; i++ {
		bm := this.marks[i]
		y := hh - this.scrollY + float64(i)*this.rowHeight
		color := t.TextColor
		if i == this.selectedIdx {
			g.SetBrush1(t.HighLightColor)
			color = t.MenuActiveTextColor
		} else {
			g.SetBrush1(t.ButtonHoverBGColor)
		}
		if i == this.selectedIdx || i == this.hoverIdx {
			g.Rectangle(0, y, w, this.rowHeight)
			g.Fill()
		}
		g.DrawIcon1(gui.LoadIcon("bookmarks"), 6, y+(this.rowHeight-16)/2, 16, false)
		label := filepath.Base(bm.File) + ":" + strconv.Itoa(bm.Line)
		if bm.Label != "" {
			label += "  " + bm.Label
		}
		drawPanelText(g, t.Font, label, 28, y, w-36, this.rowHeight, color)
	}
}
func (this *BookmarksPanel) OnResize() { this.clampScroll() }
func (this *BookmarksPanel) OnKeyDown(key int, repeat bool) {
	this.clampScroll()
	if !this.IsEnabled() || len(this.marks) == 0 {
		return
	}
	i := this.selectedIdx
	page := max(1, int((this.Height()-this.headerHeight())/this.rowHeight))
	switch key {
	case gui.KeyDown:
		i = min(len(this.marks)-1, i+1)
	case gui.KeyUp:
		i = max(0, i-1)
	case gui.KeyHome:
		i = 0
	case gui.KeyEnd:
		i = len(this.marks) - 1
	case gui.KeyPageDown:
		i = min(len(this.marks)-1, max(0, i)+page)
	case gui.KeyPageUp:
		i = max(0, i-page)
	case gui.KeyEnter:
		if i >= 0 && this.cbActivated != nil {
			m := this.marks[i]
			this.cbActivated(m.File, m.Line)
		}
		return
	case gui.KeyDelete, gui.KeyBackSpace:
		if i >= 0 && this.cbRemove != nil {
			m := this.marks[i]
			this.cbRemove(m.File, m.Line)
		}
		return
	default:
		return
	}
	this.selectedIdx = i
	top := float64(i) * this.rowHeight
	if top < this.scrollY {
		this.scrollY = top
	}
	if bottom := top + this.rowHeight; bottom > this.scrollY+this.Height()-this.headerHeight() {
		this.scrollY = bottom - max(0, this.Height()-this.headerHeight())
	}
	this.clampScroll()
	this.Update()
}

// hitTest returns the bookmark index for a y coordinate, or -1.
func (this *BookmarksPanel) hitTest(y float64) int {
	this.clampScroll()
	headerH := this.headerHeight()
	if y < headerH {
		return -1
	}
	idx := int((y - headerH + this.scrollY) / this.rowHeight)
	if idx < 0 || idx >= len(this.marks) {
		return -1
	}
	return idx
}

// OnLeftDown fires SigActivated for the clicked bookmark row.
func (this *BookmarksPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.hitTest(y)
	if idx < 0 {
		return
	}
	this.selectedIdx = idx
	this.Update()
	bm := this.marks[idx]
	if this.cbActivated != nil {
		this.cbActivated(bm.File, bm.Line)
	}
}

// OnMouseMove tracks hover state for visual feedback.
func (this *BookmarksPanel) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover state.
func (this *BookmarksPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the bookmark list vertically.
func (this *BookmarksPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

func (this *BookmarksPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

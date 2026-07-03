package ged

import (
	"path/filepath"
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
		Icon: "tree-view",
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
	if idx := this.indexOf(file, line); idx >= 0 {
		this.marks[idx].Label = label
	} else {
		this.marks = append(this.marks, Bookmark{File: file, Line: line, Label: label})
	}
	this.marks = sortBookmarks(this.marks)
	this.Self().Update()
}

// Remove deletes the bookmark at (file, line) if present.
func (this *BookmarksPanel) Remove(file string, line int) {
	idx := this.indexOf(file, line)
	if idx < 0 {
		return
	}
	this.marks = append(this.marks[:idx], this.marks[idx+1:]...)
	if this.hoverIdx >= len(this.marks) {
		this.hoverIdx = -1
	}
	this.Self().Update()
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
	return sortBookmarks(this.marks)
}

// Clear removes all bookmarks.
func (this *BookmarksPanel) Clear() {
	this.marks = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// SigActivated registers the callback invoked when a row is clicked.
func (this *BookmarksPanel) SigActivated(fn func(file string, line int)) {
	this.cbActivated = fn
}

// Draw renders the bookmarks panel: a count header followed by one row
// per bookmark (a glyph, "basename:line", then the label) with an
// alternating row tint.
func (this *BookmarksPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header with the bookmark count
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
	g.DrawText1(8, headerH-5, "书签 ("+strconv.Itoa(len(this.marks))+")")

	if len(this.marks) == 0 {
		emptyFont := paint.NewFont(t.Font.Family(), 11, false, false)
		g.SetFont(emptyFont)
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 200})
		g.DrawText1(8, headerH+20, "No bookmarks")
		return
	}

	rowFont := paint.NewFont(t.Font.Family(), 11, false, false)
	locFont := paint.NewFont(t.Font.Family(), 11, true, false)
	g.SetFont(rowFont)
	fe := rowFont.FontExtents()
	rh := this.rowHeight
	startY := headerH - this.scrollY

	for i := range this.marks {
		rowY := startY + float64(i)*rh
		if rowY+rh < headerH || rowY > h {
			continue
		}
		bm := this.marks[i]

		// Alternating row tint for readability.
		if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 245, G: 247, B: 250, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}
		// Hover highlight
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		textY := rowY + fe.Ascent + (rh-fe.Ascent-fe.Descent)/2

		// Bookmark glyph
		g.SetFont(rowFont)
		g.SetBrush1(paint.Color{R: 230, G: 160, B: 40, A: 255})
		g.DrawText1(8, textY, "⚑")

		// "basename:line" location, in bold accent.
		loc := filepath.Base(bm.File) + ":" + strconv.Itoa(bm.Line)
		g.SetFont(locFont)
		g.SetBrush1(t.HighLightColor)
		g.DrawText1(24, textY, loc)
		ext := locFont.TextExtents(loc)

		// Label, in normal text colour.
		if bm.Label != "" {
			g.SetFont(rowFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(24+ext.Width+8, textY, bm.Label)
		}
	}
}

// hitTest returns the bookmark index for a y coordinate, or -1.
func (this *BookmarksPanel) hitTest(y float64) int {
	headerH := 22.0
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
	headerH := 22.0
	_, h := this.Size()
	maxScroll := float64(len(this.marks))*this.rowHeight - (h - headerH)
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

func (this *BookmarksPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

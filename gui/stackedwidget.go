package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.StackedWidget", core.TypeOf((*StackedWidget)(nil)))
}

// StackedWidget shows one page at a time from a stack of child widgets,
// equivalent to QStackedWidget in Qt.
type StackedWidget struct {
	Widget
	currentIndex int
	pages        []IWidget
}

func NewStackedWidget() *StackedWidget {
	p := new(StackedWidget)
	p.Init(p)
	return p
}

func (this *StackedWidget) Init(self IWidget) {
	this.Widget.Init(self)
	this.currentIndex = -1
}

func (this *StackedWidget) AddPage(w IWidget) {
	w.SetParent(this.Self())
	w.Hide()
	this.pages = append(this.pages, w)

	// If this is the first page, show it
	if len(this.pages) == 1 {
		this.SetCurrentIndex(0)
	}
}

func (this *StackedWidget) RemovePage(idx int) {
	if idx < 0 || idx >= len(this.pages) {
		return
	}
	page := this.pages[idx]
	page.Hide()
	page.SetParent(nil)

	this.pages = append(this.pages[:idx], this.pages[idx+1:]...)

	if this.currentIndex == idx {
		if len(this.pages) > 0 {
			if this.currentIndex >= len(this.pages) {
				this.currentIndex = len(this.pages) - 1
			}
			this.pages[this.currentIndex].Show()
		} else {
			this.currentIndex = -1
		}
	} else if this.currentIndex > idx {
		this.currentIndex--
	}
	this.Self().Update()
}

// replacePage is called only after TabWidget has validated detached ownership.
// Publish identity first; reentrant lifecycle handlers must not see the old
// page in the slot being replaced. Never overwrite a later removal/selection.
func (this *StackedWidget) replacePage(idx int, w IWidget) {
	old := this.pages[idx]
	this.pages[idx] = w
	old.Hide()
	if old.Parent() == this.Self() {
		old.SetParent(nil)
	}
	current := -1
	for i, p := range this.pages {
		if p == w {
			current = i
			break
		}
	}
	if current < 0 {
		return
	}
	w.SetParent(this.Self())
	if w.Parent() == this.Self() && this.Page(current) == w {
		w.SetVisible(current == this.currentIndex)
	}
	this.Layout()
	this.Update()
}

// MovePage reorders the pages in place, the way a tab strip does when a tab is
// dragged to a new position. It only renumbers: no page is reparented, hidden
// or shown, so whatever was on screen stays on screen.
func (this *StackedWidget) MovePage(from, to int) {
	n := len(this.pages)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return
	}
	cur := this.CurrentPage()
	page := this.pages[from]
	if from < to {
		copy(this.pages[from:to], this.pages[from+1:to+1])
	} else {
		copy(this.pages[to+1:from+1], this.pages[to:from])
	}
	this.pages[to] = page

	// currentIndex names a slot and the move renumbered the slots, so follow
	// the page that was current. Leave the number alone and the index points at
	// whatever page slid into that slot while a different page stays visible.
	if cur != nil {
		for i, p := range this.pages {
			if p == cur {
				this.currentIndex = i
				break
			}
		}
	}
}

func (this *StackedWidget) SetCurrentIndex(idx int) {
	if idx < 0 || idx >= len(this.pages) {
		return
	}
	if idx == this.currentIndex {
		return
	}

	// Hide current page
	if this.currentIndex >= 0 && this.currentIndex < len(this.pages) {
		this.pages[this.currentIndex].Hide()
	}

	this.currentIndex = idx

	// Show new page and layout
	this.pages[this.currentIndex].Show()
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *StackedWidget) CurrentIndex() int {
	return this.currentIndex
}

func (this *StackedWidget) CurrentPage() IWidget {
	if this.currentIndex >= 0 && this.currentIndex < len(this.pages) {
		return this.pages[this.currentIndex]
	}
	return nil
}

func (this *StackedWidget) Count() int {
	return len(this.pages)
}

func (this *StackedWidget) Page(idx int) IWidget {
	if idx < 0 || idx >= len(this.pages) {
		return nil
	}
	return this.pages[idx]
}

func (this *StackedWidget) Layout() {
	if this.currentIndex < 0 || this.currentIndex >= len(this.pages) {
		return
	}
	w, h := this.Self().Size()
	page := this.pages[this.currentIndex]
	page.SetBounds(0, 0, w, h)
}

func (this *StackedWidget) Draw(g paint.Painter) {
	// Only the current page is visible; framework draws visible children.
}

func (this *StackedWidget) SizeHints() SizeHints {
	var maxW, maxH float64
	for _, p := range this.pages {
		hints := p.SizeHints()
		if hints.Width > maxW {
			maxW = hints.Width
		}
		if hints.Height > maxH {
			maxH = hints.Height
		}
	}
	return SizeHints{Width: maxW, Height: maxH, Policy: GrowHorizontal | GrowVertical}
}

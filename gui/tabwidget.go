package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.TabWidget", core.TypeOf((*TabWidget)(nil)))
}

// tabPageRef holds metadata for each tab page.
type tabPageRef struct {
	title string
	icon  paint.Icon
	page  IWidget
}

func (this *tabPageRef) Title() string {
	return this.title
}

func (this *tabPageRef) Text() string {
	return this.title
}

func (this *tabPageRef) Icon() paint.Icon {
	return this.icon
}

// TabWidget combines a TabBar with a StackedWidget to provide
// a tabbed page container, equivalent to QTabWidget in Qt.
type TabWidget struct {
	Widget
	tabBar *TabBar
	stack  *StackedWidget
	refs   []*tabPageRef

	cbCurrentChanged func(interface{}, int)
}

func NewTabWidget() *TabWidget {
	p := new(TabWidget)
	p.Init(p)
	return p
}

func (this *TabWidget) Init(self IWidget) {
	this.Widget.Init(self)

	this.tabBar = NewTabBar()
	this.tabBar.SetParent(this.Self())

	this.stack = NewStackedWidget()
	this.stack.SetParent(this.Self())

	// When a tab is clicked, switch the stacked widget page
	this.tabBar.SetActivateCallback(func(tb *TabBar, idx int) {
		if idx >= 0 && idx < this.stack.Count() {
			oldIdx := this.stack.CurrentIndex()
			this.stack.SetCurrentIndex(idx)
			if oldIdx != idx && this.cbCurrentChanged != nil {
				this.cbCurrentChanged(this.Self(), idx)
			}
		}
	})
}

func (this *TabWidget) AddTab(w IWidget, title string, icon paint.Icon) {
	ref := &tabPageRef{title: title, icon: icon, page: w}
	this.refs = append(this.refs, ref)

	this.tabBar.AddTab(ref, false)
	this.stack.AddPage(w)

	// Activate first tab
	if len(this.refs) == 1 {
		this.tabBar.SetActiveTab(0)
	}

	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

func (this *TabWidget) RemoveTab(idx int) {
	if idx < 0 || idx >= len(this.refs) {
		return
	}

	this.tabBar.RemoveTab(idx)
	this.stack.RemovePage(idx)

	this.refs = append(this.refs[:idx], this.refs[idx+1:]...)

	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *TabWidget) SetCurrentIndex(idx int) {
	if idx < 0 || idx >= len(this.refs) {
		return
	}
	this.tabBar.SetActiveTab(idx)
	this.stack.SetCurrentIndex(idx)
	if this.cbCurrentChanged != nil {
		this.cbCurrentChanged(this.Self(), idx)
	}
}

func (this *TabWidget) CurrentIndex() int {
	return this.tabBar.ActiveTab()
}

func (this *TabWidget) Count() int {
	return len(this.refs)
}

func (this *TabWidget) TabBar() *TabBar {
	return this.tabBar
}

func (this *TabWidget) Stack() *StackedWidget {
	return this.stack
}

func (this *TabWidget) SetCurrentChangedCallback(cb func(interface{}, int)) {
	this.cbCurrentChanged = cb
}

func (this *TabWidget) Layout() {
	w, h := this.Self().Size()

	// TabBar at top
	tabHints := this.tabBar.SizeHints()
	tabH := tabHints.Height
	if tabH < Theme().TabBarHeight {
		tabH = Theme().TabBarHeight
	}

	this.tabBar.SetBounds(0, 0, w, tabH)
	this.tabBar.Layout()

	// StackedWidget fills the rest
	stackY := tabH
	stackH := h - tabH
	if stackH < 0 {
		stackH = 0
	}
	this.stack.SetBounds(0, stackY, w, stackH)
	if i, ok := this.stack.Self().(ILayout); ok {
		i.Layout()
	}
}

func (this *TabWidget) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()
	tabH := this.tabBar.Height()

	// Draw a border around the content area
	g.Rectangle(0, tabH-1, w, h-tabH+1)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Fill the content background
	g.Rectangle(1, tabH, w-2, h-tabH-1)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()
}

func (this *TabWidget) SizeHints() SizeHints {
	tabHints := this.tabBar.SizeHints()
	stackHints := this.stack.SizeHints()

	w := math.Max(tabHints.Width, stackHints.Width)
	h := tabHints.Height + stackHints.Height

	if w < 200 {
		w = 200
	}
	if h < 100 {
		h = 100
	}

	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

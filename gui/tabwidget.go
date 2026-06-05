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

// nextTabIndex returns the index to switch to when moving one step from cur
// among count tabs. forward selects the next tab (wrapping past the last back
// to 0); otherwise the previous tab (wrapping past 0 to the last). With fewer
// than two tabs there is nothing to move to, so cur is returned unchanged.
func nextTabIndex(cur, count int, forward bool) int {
	if count < 2 {
		return cur
	}
	if forward {
		return (cur + 1) % count
	}
	return (cur - 1 + count) % count
}

// stepCurrent moves the current tab one step forward (next) or backward
// (previous) with wrap-around, routing through SetCurrentIndex so the
// current-changed callback and repaint fire exactly as a click would. It is a
// no-op with fewer than two tabs. The key handler delegates here so the
// wrap math stays unit-testable without simulating modifier key state.
func (this *TabWidget) stepCurrent(forward bool) {
	count := this.Count()
	if count < 2 {
		return
	}
	this.SetCurrentIndex(nextTabIndex(this.CurrentIndex(), count, forward))
}

// OnKeyDown implements IEventKeyDown, giving the TabWidget Qt QTabWidget style
// keyboard tab switching while it (or its tab strip) holds focus:
//   - Ctrl+PageDown / Ctrl+Tab           -> next tab (wraps to first)
//   - Ctrl+PageUp / Ctrl+Shift+Tab       -> previous tab (wraps to last)
//   - Left/Up (previous), Right/Down (next) when focused, no Ctrl required
//
// All moves wrap and are no-ops with fewer than two tabs. Note: focus is only
// reached when the TabWidget itself holds focus; the tab strip's click path
// lives in TabBar (a sibling file) and does not call SetFocus, so clicking a
// tab does not by itself arm these shortcuts.
func (this *TabWidget) OnKeyDown(key int, repeat bool) {
	if this.Count() < 2 {
		return
	}
	ctrl := IsKeyDown(KeyCtrl)
	shift := IsKeyDown(KeyShift)
	switch key {
	case KeyPageDown:
		if ctrl {
			this.stepCurrent(true)
		}
	case KeyPageUp:
		if ctrl {
			this.stepCurrent(false)
		}
	case KeyTab:
		// Ctrl+Tab -> next, Ctrl+Shift+Tab -> previous.
		if ctrl {
			this.stepCurrent(!shift)
		}
	case KeyRight, KeyDown:
		// Plain arrows step within the strip when it has focus.
		this.stepCurrent(true)
	case KeyLeft, KeyUp:
		this.stepCurrent(false)
	}
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

package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
	"slices"
	"strings"
)

// NotificationLevel represents the severity level of a notification.
type NotificationLevel int

const (
	NotifyInfo NotificationLevel = iota
	NotifySuccess
	NotifyWarning
	NotifyError
)

// NotificationItem represents a single notification entry.
type NotificationItem struct {
	Title   string
	Message string
	Level   NotificationLevel
	Time    string
	Read    bool
}

// NotificationPanel displays a scrollable list of notification cards.
// Like other widgets, its methods and callbacks are UI-thread-only.
type NotificationPanel struct {
	Widget
	items              []NotificationItem
	scrollY            float64
	hoverIdx           int
	cbClick            func(int)
	selectedIdx        int
	limit              int
	revision           uint64
	cbItemsChanged     func()
	cbSelectionChanged func(int)
}

func init() {
	core.RegisterFactory("gui.NotificationPanel", core.TypeOf((*NotificationPanel)(nil)))
}

func NewNotificationPanel() *NotificationPanel {
	p := new(NotificationPanel)
	p.Init(p)
	return p
}

// Init carries the hover sentinel, not NewNotificationPanel: the designer and
// the .tdoc loader build widgets through the core factory, which reflects on
// Init and never sees the constructor. At hoverIdx 0 the first card paints the
// hover background before the mouse has entered the panel.
func (this *NotificationPanel) Init(iw IWidget) {
	this.Widget.Init(iw)
	this.hoverIdx = -1
	this.selectedIdx = -1
	this.SetFocusPolicy(TabFocus)
}

// Items returns a detached snapshot. Mutations must use the control's APIs.
func (this *NotificationPanel) Items() []NotificationItem { return slices.Clone(this.items) }
func (this *NotificationPanel) Count() int                { return len(this.items) }
func (this *NotificationPanel) SelectedIndex() int        { return this.selectedIdx }
func (this *NotificationPanel) SelectedItem() (NotificationItem, bool) {
	if this.selectedIdx < 0 || this.selectedIdx >= len(this.items) {
		return NotificationItem{}, false
	}
	return this.items[this.selectedIdx], true
}
func (this *NotificationPanel) UnreadCount() int {
	n := 0
	for _, item := range this.items {
		if !item.Read {
			n++
		}
	}
	return n
}

// Limit bounds retained rows; zero (the compatibility default) is unlimited.
// Reducing the limit drops oldest rows, preserving a surviving selection.
func (this *NotificationPanel) Limit() int { return this.limit }
func (this *NotificationPanel) SetLimit(limit int) {
	limit = max(0, limit)
	if limit == this.limit {
		return
	}
	this.limit = limit
	previous := this.selectedIdx
	if this.trimToLimit() {
		this.changed()
		this.selectionChanged(previous)
	}
}
func (this *NotificationPanel) trimToLimit() bool {
	if this.limit == 0 || len(this.items) <= this.limit {
		return false
	}
	this.revision++
	drop := len(this.items) - this.limit
	copy(this.items, this.items[drop:])
	clear(this.items[this.limit:])
	this.items = this.items[:this.limit]
	this.selectedIdx = max(-1, this.selectedIdx-drop)
	this.hoverIdx = -1
	this.scrollY = max(0, this.scrollY-float64(drop)*this.itemHeight())
	return true
}
func (this *NotificationPanel) SigItemsChanged(fn func())        { this.cbItemsChanged = fn }
func (this *NotificationPanel) SigSelectionChanged(fn func(int)) { this.cbSelectionChanged = fn }
func (this *NotificationPanel) changed() {
	this.clampScroll()
	this.Update()
	if this.cbItemsChanged != nil {
		this.cbItemsChanged()
	}
}
func (this *NotificationPanel) selectionChanged(previous int) {
	if this.selectedIdx != previous && this.cbSelectionChanged != nil {
		this.cbSelectionChanged(this.selectedIdx)
	}
}
func (this *NotificationPanel) AddNotification(item NotificationItem) {
	previous := this.selectedIdx
	this.revision++
	this.items = append(this.items, item)
	this.trimToLimit()
	this.changed()
	this.selectionChanged(previous)
}
func (this *NotificationPanel) RemoveNotification(idx int) {
	if idx < 0 || idx >= len(this.items) {
		return
	}
	previous := this.selectedIdx
	this.revision++
	copy(this.items[idx:], this.items[idx+1:])
	this.items[len(this.items)-1] = NotificationItem{}
	this.items = this.items[:len(this.items)-1]
	if idx == this.selectedIdx {
		this.selectedIdx = -1
	} else if idx < this.selectedIdx {
		this.selectedIdx--
	}
	this.hoverIdx = -1
	this.changed()
	this.selectionChanged(previous)
}
func (this *NotificationPanel) ClearAll() {
	if len(this.items) == 0 {
		return
	}
	previous := this.selectedIdx
	this.revision++
	this.items = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selectedIdx = -1
	this.changed()
	this.selectionChanged(previous)
}
func (this *NotificationPanel) MarkAllRead() {
	changed := false
	for i := range this.items {
		if !this.items[i].Read {
			this.items[i].Read = true
			changed = true
		}
	}
	if changed {
		this.changed()
	}
}

// Select reveals and marks one row read, without activating its click action.
// -1 clears selection. Invalid indices leave the selection/model unchanged.
func (this *NotificationPanel) Select(idx int) bool {
	if idx < -1 || idx >= len(this.items) {
		return false
	}
	moved := this.selectedIdx != idx
	oldScroll := this.scrollY
	this.selectedIdx = idx
	read := false
	if idx >= 0 {
		read = !this.items[idx].Read
		this.items[idx].Read = true
		top := float64(idx) * this.itemHeight()
		bottom := top + this.itemHeight()
		if top < this.scrollY {
			this.scrollY = top
		} else if bottom > this.scrollY+this.Height() {
			this.scrollY = bottom - this.Height()
		}
	}
	if moved || read || this.scrollY != oldScroll {
		this.changed()
	}
	if moved && this.selectedIdx == idx && this.cbSelectionChanged != nil {
		this.cbSelectionChanged(idx)
	}
	return true
}

func (this *NotificationPanel) SigClick(fn func(int)) {
	this.cbClick = fn
}

func (this *NotificationPanel) itemHeight() float64 {
	return math.Max(60, 2*Theme().Font.FontExtents().Height+20)
}

func (this *NotificationPanel) totalHeight() float64 {
	return float64(len(this.items)) * this.itemHeight()
}

func (this *NotificationPanel) clampScroll() {
	_, h := this.Size()
	maxScroll := this.totalHeight() - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
}

func (this *NotificationPanel) hitTest(y float64) int {
	if y < 0 || y >= this.Height() {
		return -1
	}
	itemH := this.itemHeight()
	idx := int((y + this.scrollY) / itemH)
	if idx >= 0 && idx < len(this.items) {
		return idx
	}
	return -1
}

func (this *NotificationPanel) levelColor(level NotificationLevel) paint.Color {
	return toastSemanticColor(ToastLevel(level))
}

// --- Events ---

func (this *NotificationPanel) OnMouseEnter() {
	this.Self().Update()
}

func (this *NotificationPanel) OnMouseLeave() {
	this.hoverIdx = -1
	this.Self().Update()
}

func (this *NotificationPanel) OnMouseMove(x, y float64) {
	old := this.hoverIdx
	this.hoverIdx = -1
	if x >= 0 && x < this.Width() {
		this.hoverIdx = this.hitTest(y)
	}
	if old != this.hoverIdx {
		this.Self().Update()
	}
}

func (this *NotificationPanel) OnLeftDown(x, y float64) {
	if x < 0 || x >= this.Width() {
		return
	}
	idx := this.hitTest(y)
	if idx >= 0 {
		this.SetFocus()
		revision := this.revision
		this.Select(idx)
		if this.revision == revision && this.selectedIdx == idx && this.cbClick != nil {
			this.cbClick(idx)
		}
	}
}
func (this *NotificationPanel) OnResize() { this.clampScroll() }
func (this *NotificationPanel) OnKeyDown(key int, repeat bool) {
	if len(this.items) == 0 || isActionModifier() || IsKeyDown(KeyMenu) {
		return
	}
	idx := this.selectedIdx
	switch key {
	case KeyDown:
		idx = min(len(this.items)-1, idx+1)
	case KeyUp:
		idx = max(0, idx-1)
	case KeyHome:
		idx = 0
	case KeyEnd:
		idx = len(this.items) - 1
	case KeyPageDown:
		idx = min(len(this.items)-1, max(0, idx)+max(1, int(this.Height()/this.itemHeight())))
	case KeyPageUp:
		idx = max(0, idx-max(1, int(this.Height()/this.itemHeight())))
	case KeyEnter, KeySpace:
		if idx >= 0 && this.cbClick != nil {
			this.cbClick(idx)
		}
		return
	default:
		return
	}
	this.Select(idx)
}

func (this *NotificationPanel) OnMouseWheel(x, y, z float64) {
	old := this.scrollY
	this.scrollY -= z * defaultWheelScrollLines * 10
	this.clampScroll()
	if old != this.scrollY {
		this.Update()
	}
}

// --- Drawing ---

func (this *NotificationPanel) Draw(g paint.Painter) {
	this.clampScroll()
	t := Theme()
	w, h := this.Size()
	itemH := this.itemHeight()
	barW := 4.0

	// background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.FormColor)
	g.Fill()

	if len(this.items) == 0 {
		g.SetFont(t.Font)
		g.SetBrush1(t.MenuGrayTextColor)
		g.DrawText1(4, h/2+4, "Notifications")
		return
	}

	g.Save()
	g.Rectangle(0, 0, w, h)
	g.Clip()

	f := t.Font
	g.SetFont(f)
	fe := f.FontExtents()

	startIdx := int(this.scrollY / itemH)
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(this.items); i++ {
		iy := float64(i)*itemH - this.scrollY
		if iy >= h {
			break
		}
		if iy+itemH <= 0 {
			continue
		}

		item := this.items[i]

		// card background
		g.Rectangle(0, iy, w, itemH-2)
		if this.selectedIdx == i {
			g.SetBrush1(t.MenuActiveBGColor)
		} else if this.hoverIdx == i {
			g.SetBrush1(t.FormLightColor)
		} else {
			g.SetBrush1(t.ViewBGColor)
		}
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 0.5)
		g.Stroke()

		// colored left bar
		g.Rectangle(0, iy, barW, itemH-2)
		g.SetBrush1(this.levelColor(item.Level))
		g.Fill()

		titleX := barW + 8
		available := max(0, w-titleX-8)
		stamp := elideToWidth(f, notificationPreview(item.Time), available*.35)
		timeW := 0.0
		if stamp != "" {
			timeW = f.TextExtents(stamp).Width + 12
		}
		title := elideToWidth(f, notificationPreview(item.Title), max(0, available-timeW))
		titleY := iy + fe.Height + 4
		ink := t.TextColor
		muted := t.MenuGrayTextColor
		if this.selectedIdx == i {
			ink = t.MenuActiveTextColor
			muted = ink
		}
		g.SetBrush1(ink)
		g.DrawText1(titleX, titleY, title)
		if stamp != "" {
			g.SetBrush1(muted)
			g.DrawText1(w-8-f.TextExtents(stamp).Width, titleY, stamp)
		}
		msg := elideToWidth(f, notificationPreview(item.Message), available)
		g.SetBrush1(muted)
		g.DrawText1(titleX, iy+fe.Height*2+8, msg)
		if !item.Read {
			g.Arc(w-6, iy+itemH-8, 2, 0, 2*math.Pi)
			g.SetBrush1(this.levelColor(item.Level))
			g.Fill()
		}

	}

	g.Restore()

}

// A preview is intentionally bounded; SelectedItem/Items retain the complete
// message for a host's scrollable details view. Never shape a megabyte log line.
func notificationPreview(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= 256 {
			b.WriteRune('…')
			break
		}
		if r < ' ' || r == '\u007f' {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func (this *NotificationPanel) SizeHints() SizeHints {
	h := math.Max(float64(len(this.items))*this.itemHeight(), 100)
	return SizeHints{Width: 280, Height: math.Min(h, 300), Policy: GrowHorizontal | GrowVertical}
}

func (this *NotificationPanel) EnumProperties(list core.IPropertyList) {
	list.AddProperty("可见", this.IsVisible, this.SetVisible)
	list.AddProperty("保留上限", this.Limit, this.SetLimit)
}

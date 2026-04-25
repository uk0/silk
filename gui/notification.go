package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// NotificationLevel represents the severity level of a notification.
type NotificationLevel int

const (
	NotifyInfo    NotificationLevel = iota
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
type NotificationPanel struct {
	Widget
	items   []NotificationItem
	scrollY float64
	hoverIdx int
	cbClick func(int)
}

func init() {
	core.RegisterFactory("gui.NotificationPanel", core.TypeOf((*NotificationPanel)(nil)))
}

func NewNotificationPanel() *NotificationPanel {
	p := new(NotificationPanel)
	p.Init(p)
	p.hoverIdx = -1
	return p
}

func (this *NotificationPanel) Items() []NotificationItem { return this.items }

func (this *NotificationPanel) AddNotification(item NotificationItem) {
	this.items = append(this.items, item)
	this.Self().Update()
}

func (this *NotificationPanel) RemoveNotification(idx int) {
	if idx < 0 || idx >= len(this.items) {
		return
	}
	this.items = append(this.items[:idx], this.items[idx+1:]...)
	this.clampScroll()
	this.Self().Update()
}

func (this *NotificationPanel) ClearAll() {
	this.items = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

func (this *NotificationPanel) Count() int { return len(this.items) }

func (this *NotificationPanel) SigClick(fn func(int)) {
	this.cbClick = fn
}

func (this *NotificationPanel) itemHeight() float64 {
	return 60.0
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
	itemH := this.itemHeight()
	idx := int((y + this.scrollY) / itemH)
	if idx >= 0 && idx < len(this.items) {
		return idx
	}
	return -1
}

func (this *NotificationPanel) levelColor(level NotificationLevel) paint.Color {
	switch level {
	case NotifySuccess:
		return paint.Color{52, 199, 89, 255} // green
	case NotifyWarning:
		return paint.Color{255, 149, 0, 255} // orange
	case NotifyError:
		return paint.Color{255, 59, 48, 255} // red
	default:
		return paint.Color{66, 133, 244, 255} // blue (info)
	}
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
	this.hoverIdx = this.hitTest(y)
	if old != this.hoverIdx {
		this.Self().Update()
	}
}

func (this *NotificationPanel) OnLeftDown(x, y float64) {
	idx := this.hitTest(y)
	if idx >= 0 {
		this.items[idx].Read = true
		if this.cbClick != nil {
			this.cbClick(idx)
		}
		this.Self().Update()
	}
}

func (this *NotificationPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * defaultWheelScrollLines * 10
	this.clampScroll()
	this.Self().Update()
}

// --- Drawing ---

func (this *NotificationPanel) Draw(g paint.Painter) {
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
		g.SetBrush1(paint.Color{180, 185, 200, 255})
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
		if this.hoverIdx == i {
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

		// title
		titleX := barW + 8.0
		titleY := iy + fe.Height + 4
		ext := f.TextExtents(item.Title)
		if item.Read {
			g.SetBrush1(t.FormDarkColor)
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.Translate(titleX-ext.XBearing, titleY)
		g.DrawText(item.Title)
		g.Translate(-(titleX - ext.XBearing), -titleY)

		// message
		msgY := iy + fe.Height*2 + 8
		if item.Message != "" {
			msgExt := f.TextExtents(item.Message)
			g.SetBrush1(t.MenuGrayTextColor)
			g.Translate(titleX-msgExt.XBearing, msgY)
			g.DrawText(item.Message)
			g.Translate(-(titleX - msgExt.XBearing), -msgY)
		}

		// time
		if item.Time != "" {
			timeExt := f.TextExtents(item.Time)
			timeX := w - timeExt.Width - 8
			g.SetBrush1(t.MenuGrayTextColor)
			g.Translate(timeX-timeExt.XBearing, titleY)
			g.DrawText(item.Time)
			g.Translate(-(timeX - timeExt.XBearing), -titleY)
		}
	}

	g.Restore()

	_ = fe
}

func (this *NotificationPanel) SizeHints() SizeHints {
	h := math.Max(float64(len(this.items))*this.itemHeight(), 100)
	return SizeHints{Width: 280, Height: math.Min(h, 300), Policy: GrowHorizontal | GrowVertical}
}

func (this *NotificationPanel) EnumProperties(list core.IPropertyList) {
	list.AddProperty("可见", this.IsVisible, this.SetVisible)
}

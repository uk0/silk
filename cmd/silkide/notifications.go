package main

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

const notificationHistoryLimit = 200
const notificationMessageLimit = 64 << 10

// Session-only notification history. No disk writes, scanning, polling or
// background renderer. The existing SDK panel owns selection/read state.
type notificationsPane struct {
	gui.Widget
	owner      *gui.Frame
	dock       *gui.Dock
	list       *gui.NotificationPanel
	detail     *gui.Edit
	summary    *gui.Label
	buttons    []*gui.Button
	badge      *gui.Button
	toast      *gui.Toast
	detailText string
	copyText   func(string) error
}

func newNotificationsPane(owner *gui.Frame, dock *gui.Dock) *notificationsPane {
	p := new(notificationsPane)
	p.Init(p)
	p.owner, p.dock = owner, dock
	p.SetFocusPolicy(gui.TabFocus)
	p.list = gui.NewNotificationPanel()
	p.list.SetParent(p)
	p.list.SetLimit(notificationHistoryLimit)
	p.detail = gui.NewEdit()
	p.detail.SetMultiLine(true)
	p.detail.SetWrap(true)
	p.detail.SetReadOnly(true)
	p.detail.SetFocusPolicy(gui.TabFocus)
	p.detail.SetParent(p)
	p.summary = gui.NewLabel("")
	p.summary.SetParent(p)
	p.copyText = func(text string) error { _, err := gui.Clipboard.SetData(text); return err }
	add := func(label, icon string, fn func()) {
		b := gui.NewButton1(i18n.T(label), gui.LoadIcon(icon))
		b.SetTextVisible(true)
		b.SetParent(p)
		b.SetFocusPolicy(gui.TabFocus)
		b.Action().BindFunc0(fn)
		p.buttons = append(p.buttons, b)
	}
	add("Copy Message", "edit-copy", func() {
		if item, ok := p.list.SelectedItem(); ok {
			if err := p.copyText(item.Message); err != nil {
				p.summary.SetText(i18n.Tf("Copy failed: %v", err))
			}
		}
	})
	add("Dismiss Selected", "close", func() { p.toast.Dismiss(); p.list.RemoveNotification(p.list.SelectedIndex()) })
	add("Mark All Read", "checkbox-checked", p.list.MarkAllRead)
	add("Clear Notifications", "line-delete", func() { p.toast.Dismiss(); p.list.ClearAll() })
	p.list.SigItemsChanged(p.refresh)
	p.refresh()
	return p
}
func (p *notificationsPane) Title() string    { return i18n.T("Notifications") }
func (p *notificationsPane) Icon() paint.Icon { return gui.LoadIcon("notifications") }
func (p *notificationsPane) OnFocus()         { p.list.SetFocus() }

func boundedNotificationMessage(message string) string {
	if len(message) <= notificationMessageLimit {
		return strings.Clone(message)
	}
	end := notificationMessageLimit
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return strings.Clone(message[:end]) + "\n" + i18n.T("[Notification truncated at 64 KiB]")
}
func notificationLevelTitle(level gui.ToastLevel) string {
	switch level {
	case gui.ToastSuccess:
		return i18n.T("Success")
	case gui.ToastWarning:
		return i18n.T("Warning")
	case gui.ToastError:
		return i18n.T("Error")
	default:
		return i18n.T("Information")
	}
}
func (p *notificationsPane) record(message string, level gui.ToastLevel) {
	if p == nil || p.owner == nil {
		return
	}
	message = boundedNotificationMessage(message)
	p.list.AddNotification(gui.NotificationItem{Title: notificationLevelTitle(level), Message: message, Level: gui.NotificationLevel(level), Time: time.Now().Format("15:04:05")})
	// One visible banner per workbench, so bursts never grow over document tabs.
	p.toast.Dismiss()
	duration := uint32(2500)
	if level == gui.ToastWarning || level == gui.ToastError {
		duration = 5000
	}
	p.toast = gui.ShowToastWithOptions(p.owner, message, gui.ToastOptions{
		DurationMs: duration, Level: level, Position: gui.ToastBottomRight,
		OnActivate: func() { p.list.Select(p.list.Count() - 1); p.reveal() },
	})
}
func (p *notificationsPane) refresh() {
	item, selected := p.list.SelectedItem()
	text := ""
	if selected {
		text = item.Message
	}
	if text != p.detailText {
		p.detailText = text
		p.detail.SetText(text)
		p.detail.SetCaretPos(0)
	}
	unread, count := p.list.UnreadCount(), p.list.Count()
	p.summary.SetText(i18n.Tf("%d notifications · %d unread", count, unread))
	if len(p.buttons) == 4 {
		for i, enabled := range []bool{selected, selected, unread > 0, count > 0} {
			b := p.buttons[i]
			if b.IsEnabled() != enabled {
				b.SetEnabled(enabled)
				b.Action().SetEnabled(enabled)
				b.Update()
			}
		}
	}
	if p.badge != nil {
		label := fmt.Sprint(unread)
		if p.badge.Text() != label {
			p.badge.SetText(label)
			if sb, ok := p.badge.Parent().(*gui.StatusBar); ok {
				sb.Layout()
				sb.Update()
			}
		}
		gui.SetToolTip(p.badge, i18n.Tf("Notifications: %d unread / %d total", unread, count))
	}
	p.Update()
}
func (p *notificationsPane) reveal() {
	if p == nil || p.owner == nil || p.dock == nil {
		return
	}
	p.toast.Dismiss()
	if w := globalWorkbench; w != nil && w.frame == p.owner {
		if !revealToolView(w.tabs, w.canvas, p.dock, p, modeEdit) {
			return
		}
	} else {
		p.dock.RevealView(p)
	}
	if p.list.SelectedIndex() < 0 && p.list.Count() > 0 {
		p.list.Select(p.list.Count() - 1)
	}
	p.list.SetFocus()
}
func (p *notificationsPane) close() {
	if p == nil {
		return
	}
	p.toast.Dismiss()
	p.owner = nil
	p.list.SigItemsChanged(nil)
	p.list.SigSelectionChanged(nil)
	p.list.SigClick(nil)
}
func notificationsFor(frame *gui.Frame) *notificationsPane {
	if w := globalWorkbench; w != nil && w.frame == frame {
		return w.notifications
	}
	return nil
}
func showNotifications(frame *gui.Frame) {
	if p := notificationsFor(frame); p != nil {
		p.reveal()
	}
}
func newNotificationsStatusButton(frame *gui.Frame) *gui.Button {
	b := gui.NewButton1("0", gui.LoadIcon("notifications"))
	b.SetTextVisible(true)
	b.SetFocusPolicy(gui.TabFocus)
	gui.SetToolTip(b, i18n.T("Show Notifications"))
	b.Action().BindFunc0(func() { showNotifications(frame) })
	if p := notificationsFor(frame); p != nil {
		p.badge = b
		p.refresh()
	}
	return b
}
func (p *notificationsPane) OnResize() {
	if p.list == nil {
		return
	}
	w, h := p.Size()
	w, h = math.Max(0, w), math.Max(0, h)
	place := func(c gui.IWidget, x, y, cw, ch float64) {
		x, y = math.Max(0, math.Min(x, w)), math.Max(0, math.Min(y, h))
		c.SetBounds(x, y, math.Max(0, math.Min(cw, w-x)), math.Max(0, math.Min(ch, h-y)))
	}
	x, y, row := 8.0, 8.0, 0.0
	for _, b := range p.buttons {
		hint := b.SizeHints()
		bw, bh := math.Max(100, hint.Width+8), math.Max(30, hint.Height)
		if x > 8 && x+bw > w-8 {
			x = 8
			y += row + 6
			row = 0
		}
		row = math.Max(row, bh)
		place(b, x, y, bw, bh)
		x += bw + 6
	}
	y += row + 6
	place(p.summary, 8, y, w-16, 24)
	y += 30
	body := math.Max(0, h-y-8)
	if w >= 640 {
		left := math.Max(0, (w-24)*.42)
		place(p.list, 8, y, left, body)
		place(p.detail, 16+left, y, w-left-24, body)
	} else {
		top := math.Max(0, (body-8)*.45)
		place(p.list, 8, y, w-16, top)
		place(p.detail, 8, y+top+8, w-16, math.Max(0, body-top-8))
	}
}

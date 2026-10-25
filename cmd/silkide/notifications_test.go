package main

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func testNotifications(t *testing.T) (*workbench, *notificationsPane) {
	w := testWorkbench(t)
	old := globalFrame
	zoom, build, lsp := statusBarZoomLabel, statusBarBuildLabel, statusBarLSPButton
	globalFrame = w.frame
	t.Cleanup(func() {
		globalFrame = old
		statusBarZoomLabel, statusBarBuildLabel, statusBarLSPButton = zoom, build, lsp
	})
	bottom := w.center.SplitNewDock(false, true).(*gui.Dock)
	globalBottomDock = bottom
	p := newNotificationsPane(w.frame, bottom)
	bottom.AddView(p)
	w.notifications = p
	t.Cleanup(p.close)
	w.frame.SetSize(1000, 700)
	buildStatusBar(w.frame)
	w.activate(modeEdit)
	return w, p
}
func TestNotificationsHistoryExactDetailsCopyAndClear(t *testing.T) {
	_, p := testNotifications(t)
	message := "Cleanup would change a standalone CR; no text was modified\n中🙂\tfull details"
	silkideToast(message, gui.ToastWarning)
	if p.list.Count() != 1 || p.list.UnreadCount() != 1 || !p.toast.Active() || p.badge.Text() != "1" {
		t.Fatal("unrecorded/unread")
	}
	p.list.Select(0)
	if p.detail.Text() != message || !p.detail.IsReadOnly() || !p.detail.MultiLine() || !p.detail.Warp() {
		t.Fatal("lost details/readonly")
	}
	if p.badge.Text() != "0" || !p.buttons[0].IsEnabled() {
		t.Fatal("unread/copy state")
	}
	var copied string
	p.copyText = func(s string) error { copied = s; return nil }
	p.buttons[0].Action().Trigger(nil)
	if copied != message {
		t.Fatal("copy elided details")
	}
	p.copyText = func(string) error { return errors.New("fixture clipboard failure") }
	p.buttons[0].Action().Trigger(nil)
	if !strings.Contains(p.summary.Text(), "fixture clipboard failure") || p.list.Count() != 1 {
		t.Fatal("copy feedback recursion")
	}
	p.buttons[3].Action().Trigger(nil)
	if p.list.Count() != 0 || p.detail.Text() != "" || p.toast.Active() || p.badge.Text() != "0" {
		t.Fatal("clear did not release state")
	}
	for _, b := range p.buttons {
		if b.IsEnabled() {
			t.Fatal("empty button enabled")
		}
	}
}
func TestNotificationsBoundedOwnerLifecycleAndNoAutoNavigation(t *testing.T) {
	w, p := testNotifications(t)
	path := writeFile(t, t.TempDir(), "a.txt", "unchanged")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	big := strings.Repeat("中🙂", 20000)
	for i := 0; i < notificationHistoryLimit+15; i++ {
		silkideToast(big, gui.ToastInfo)
	}
	if p.list.Count() != notificationHistoryLimit || p.list.UnreadCount() != notificationHistoryLimit || p.list.SelectedIndex() != -1 {
		t.Fatal("unbounded/auto-read")
	}
	item := p.list.Items()[0]
	if len(item.Message) > notificationMessageLimit+200 || !utf8.ValidString(item.Message) || !strings.Contains(item.Message, "64 KiB") {
		t.Fatal("unbounded/broken Unicode")
	}
	if ed.Text() != "unchanged" || documentMode(w.tabs, w.canvas) != modeEdit || p.dock.ActiveView() == nil {
		t.Fatal("notification changed document")
	}
	handle := p.toast
	p.close()
	if handle.Active() {
		t.Fatal("closed owner keeps banner")
	}
	p.record("after close", gui.ToastError)
	if p.list.Count() != notificationHistoryLimit {
		t.Fatal("closed history changed")
	}
}
func TestNotificationsMenuPaletteAndClosedDockRecovery(t *testing.T) {
	w, p := testNotifications(t)
	silkideToast("saved", gui.ToastSuccess)
	menu := buildWorkbenchSubMenu("View", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	b := menuCommandButton(menu, "view.notifications")
	if b == nil || b.Icon() == nil || !b.IsEnabled() {
		t.Fatal("missing menu")
	}
	b.Action().Trigger(nil)
	if p.dock.ActiveView() != p || p.detail.Text() != "saved" || p.toast.Active() {
		t.Fatal("menu reveal")
	}
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	count := 0
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Show Notifications") {
			count++
			c.Fn()
		}
	}
	if count != 1 || p.dock.ActiveView() != p {
		t.Fatal("palette reveal")
	}
	p.dock.SetRetainClosedViews(true)
	p.dock.PromptSaveCloseView(p)
	showNotifications(w.frame)
	if p.dock.ActiveView() != p || p.list.Count() != 1 {
		t.Fatal("hidden panel lost model/recovery")
	}
}
func TestNotificationsResponsiveBounds(t *testing.T) {
	_, p := testNotifications(t)
	for _, size := range [][2]float64{{900, 240}, {640, 400}, {480, 560}, {220, 200}, {20, 8}, {0, 0}} {
		p.SetSize(size[0], size[1])
		p.OnResize()
		for _, c := range p.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatal("outside", size, c.Bounds1())
			}
		}
		if p.list.Width() > 0 && p.list.Height() > 0 && p.detail.Width() > 0 && p.detail.Height() > 0 {
			a, b := p.list, p.detail
			if a.X() < b.X()+b.Width() && b.X() < a.X()+a.Width() && a.Y() < b.Y()+b.Height() && b.Y() < a.Y()+a.Height() {
				t.Fatal("list/details overlap")
			}
		}
	}
}

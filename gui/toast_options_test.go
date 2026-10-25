package gui

import "testing"

func TestToastBottomRightProtectsFrameChromeAndNeverStealsFocus(t *testing.T) {
	frame := NewFrame()
	frame.SetSize(800, 600)
	frame.SetToolBar(NewToolBar())
	frame.SetStatusBar(NewStatusBar())
	frame.Layout()
	t.Cleanup(func() { dismissOwnedToasts(frame) })
	child := NewButton()
	frame.MainDock().AddView(child)
	child.SetFocus()
	mgr := new(ToastManager)
	handle := mgr.showWithOptions(child, "Saved", ToastOptions{DurationMs: 60000, Position: ToastBottomRight, Level: ToastSuccess})
	e := handle.entry
	if !handle.Active() || e.Window() != nil || e.FocusPolicy() != NoFocus {
		t.Fatal("native/focus/handle")
	}
	x, y, w, h := frame.root_.Bounds()
	if e.X()+e.Width() > x+w || e.Y()+e.Height() > y+h || e.Y() < y+h*.5 {
		t.Fatal("wrong corner", e.Bounds1())
	}
	if e.Y()+e.Height() > frame.statusBar.Y() || e.Y() < frame.toolBar.Y()+frame.toolBar.Height() {
		t.Fatal("covered chrome")
	}
	before := e.Bounds1()
	frame.SetPos(50, 80)
	if e.Bounds1() != before {
		t.Fatal("global coordinates")
	}
	frame.SetSize(400, 300)
	if e.X()+e.Width() > 400 || e.Y()+48 > frame.statusBar.Y() {
		t.Fatal("resize")
	}
	handle.Dismiss()
	handle.Dismiss()
	if handle.Active() || e.timer != 0 || e.parent != nil || e.message != "" {
		t.Fatal("retained owner/timer/message")
	}
}
func TestToastMixedStacksBoundedIsolatedAndNonOverlapping(t *testing.T) {
	one, two := toastTestOwner(t, 640, 400), toastTestOwner(t, 500, 240)
	mgr := new(ToastManager)
	keep := mgr.showWithOptions(two, "other", ToastOptions{DurationMs: 60000, Position: ToastBottomRight})
	var handles []*Toast
	for i := 0; i < 30; i++ {
		handles = append(handles, mgr.showWithOptions(one, "burst", ToastOptions{DurationMs: 60000, Position: ToastPosition(i % 2)}))
	}
	count := 0
	for _, e := range mgr.toasts {
		if e.parent == one {
			count++
		}
	}
	if count != 8 || !keep.Active() || len(mgr.toasts) != 9 {
		t.Fatal("unbounded or cross-owner eviction", count, len(mgr.toasts))
	}
	for _, h := range handles[:22] {
		if h.Active() || h.entry.timer != 0 || h.entry.onActivate != nil {
			t.Fatal("overflow retained work")
		}
	}
	for i, a := range mgr.toasts {
		if a.parent != one || !a.IsVisible() {
			continue
		}
		for _, b := range mgr.toasts[i+1:] {
			if b.parent == one && b.IsVisible() && a.Y() < b.Y()+b.Height() && b.Y() < a.Y()+a.Height() {
				t.Fatal("mixed stacks overlap")
			}
		}
	}
	one.Hide()
	if len(mgr.toasts) != 1 || !keep.Active() {
		t.Fatal("hidden owner cleanup")
	}
}
func TestToastActionDismissesBeforeCallbackAndCloseNeverActivates(t *testing.T) {
	owner := toastTestOwner(t, 640, 400)
	mgr := new(ToastManager)
	calls := 0
	var h *Toast
	makeToast := func() {
		h = mgr.showWithOptions(owner, "Details", ToastOptions{DurationMs: 60000, Position: ToastBottomRight, OnActivate: func() {
			calls++
			if h.Active() || owner.toastManager != nil {
				t.Fatal("callback before dismissal")
			}
			mgr.show(owner, "next", 60000, ToastInfo)
		}})
	}
	makeToast()
	h.entry.OnLeftDown(toastCloseX(h.entry.Width()), 24)
	if calls != 0 || h.Active() {
		t.Fatal("close activated")
	}
	makeToast()
	e := h.entry
	e.OnLeftDown(50, 24)
	e.OnLeftDown(50, 24)
	if calls != 1 || h.Active() || len(mgr.toasts) != 1 {
		t.Fatal("activation/reentry", calls)
	}
	owner.Hide()
	if e.onActivate != nil || e.parent != nil {
		t.Fatal("callback retained")
	}
	var nilHandle *Toast
	nilHandle.Dismiss()
	if nilHandle.Active() {
		t.Fatal("nil handle")
	}
}

func TestToastBottomRightNeverCoversUpperContentWhenTiny(t *testing.T) {
	owner := toastTestOwner(t, 240, 100)
	mgr := new(ToastManager)
	h := mgr.showWithOptions(owner, "Saved", ToastOptions{DurationMs: 60000, Position: ToastBottomRight})
	if !h.Active() || h.entry.IsVisible() {
		t.Fatal("tiny owner toast covers upper content")
	}
	owner.SetSize(240, 200)
	if !h.entry.IsVisible() || h.entry.Y() < 100 {
		t.Fatal("resize did not restore lower-half toast")
	}
}

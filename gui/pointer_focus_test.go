package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

func pointerTabPoint(t *testing.T, b *TabBar, index int, close bool) (float64, float64) {
	t.Helper()
	y := b.Height() / 2
	for x := 1.0; x < b.viewportW; x++ {
		if i, c := b.HitTest(x, y); i == index && c == close {
			return x, y
		}
	}
	t.Fatal("no tab hit point", index, close)
	return 0, 0
}

func TestTabPointerFocusReleaseAndKeyboardStayDistinct(t *testing.T) {
	old := focusWidget
	defer func() { focusWidget = old }()
	w := newTabWidgetABC()
	w.SetSize(600, 200)
	w.Layout()
	b := w.TabBar()
	calls := 0
	bFocus := func(b *TabBar) {
		if b.downTab != -1 || b.downCloseBtn || b.pointerTab != nil {
			t.Fatal("handoff saw armed gesture")
		}
		calls++
	}
	b.SetPointerFocusCallback(bFocus)
	b.SetFocus()
	w.SetCurrentIndex(1)
	b.OnKeyDown(KeyRight, false)
	if calls != 0 || !b.HasFocus() {
		t.Fatal("keyboard/programmatic selection handed off focus")
	}
	x, y := pointerTabPoint(t, b, b.ActiveTab(), false)
	b.OnLeftDown(x, y)
	if calls != 0 {
		t.Fatal("handoff happened on press")
	}
	b.OnLeftUp(x, y)
	if calls != 1 {
		t.Fatal("same-tab click did not hand off", calls)
	}
	b.OnLeftUp(x, y)
	if calls != 1 {
		t.Fatal("duplicate release handed off")
	}
	b.OnLeftDown(x, y)
	b.OnLeftUp(-5, y)
	if calls != 1 {
		t.Fatal("outside release handed off")
	}
	b.OnLeftDown(x, y)
	other := NewEdit()
	other.SetFocus()
	b.OnLeftUp(x, y)
	if calls != 1 || !other.HasFocus() {
		t.Fatal("handoff stole explicitly changed focus")
	}
	// Replacing a numeric slot while held is not a click on its new occupant.
	w.SetCurrentIndex(0)
	x, y = pointerTabPoint(t, b, 0, false)
	b.OnLeftDown(x, y)
	w.RemoveTab(0)
	b.OnLeftUp(x, y)
	if calls != 1 {
		t.Fatal("stale slot handed off")
	}
}

func TestTabPointerFocusCloseResetsBeforeReentrantHost(t *testing.T) {
	old := focusWidget
	defer func() { focusWidget = old }()
	w := newTabWidgetABC()
	w.SetSize(600, 200)
	w.Layout()
	b := w.TabBar()
	b.SetCloseCallback(func(_ *TabBar, idx int) bool { w.RemoveTab(idx); return true })
	calls := 0
	b.SetPointerFocusCallback(func(b *TabBar) {
		calls++
		if b.downTab != -1 || b.downCloseBtn {
			t.Fatal("armed close")
		}
		w.SetCurrentIndex(1)
	})
	x, y := pointerTabPoint(t, b, 0, true)
	b.OnLeftDown(x, y)
	b.OnLeftUp(x, y)
	if calls != 1 || w.Count() != 2 || w.CurrentIndex() != 1 {
		t.Fatal("close/reentry", calls, w.Count(), w.CurrentIndex())
	}
}

func TestTabPointerDragHandoffReorderCancelAndFocusChange(t *testing.T) {
	for _, mode := range []string{"reorder", "cancel", "focus-other", "remove", "panic"} {
		t.Run(mode, func(t *testing.T) {
			old := focusWidget
			defer func() { focusWidget = old }()
			w := newTabWidgetABC()
			w.SetSize(600, 200)
			w.Layout()
			b := w.TabBar()
			calls := 0
			b.SetPointerFocusCallback(func(b *TabBar) {
				calls++
				if b.downTab != -1 || b.pointerTab != nil {
					t.Fatal("armed drag")
				}
			})
			x, y := pointerTabPoint(t, b, 2, false)
			b.OnLeftDown(x, y)
			other := NewEdit()
			panicked := false
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				b.runPointerDrag(func(p paint.Pixmap, a DndAction, data interface{}) DndAction {
					if p == nil || a != DndMove || data != b.pointerTab {
						t.Fatal("invalid preview/payload")
					}
					switch mode {
					case "reorder":
						b.OnDrop(1, y, &dndContext{pa: DndMove, from: b})
					case "focus-other":
						other.SetFocus()
					case "remove":
						w.RemoveTab(2)
						ctx := &dndContext{pa: DndMove, from: b}
						b.OnDrop(1, y, ctx)
						if ctx.Action() != DndIgnore {
							t.Fatal("removed source accepted")
						}
					case "panic":
						panic("native drag failure")
					}
					return DndIgnore
				})
			}()
			if b.downTab != -1 || b.pointerTab != nil {
				t.Fatal("drag did not retire gesture")
			}
			want := 0
			if mode == "reorder" || mode == "cancel" {
				want = 1
			}
			if calls != want {
				t.Fatal("handoff count", calls, want)
			}
			if panicked != (mode == "panic") {
				t.Fatal("panic result")
			}
			if mode == "reorder" && w.TabTitle(0) != "C" {
				t.Fatal("reorder did not commit")
			}
			if mode == "focus-other" && !other.HasFocus() {
				t.Fatal("stole focus")
			}
		})
	}
}

func TestToolbarPointerPreservesContentWithoutChangingKeyboardOrInputs(t *testing.T) {
	old := focusWidget
	defer func() { focusWidget = old }()
	bar := NewToolBar()
	button := bar.AddAction("Save", nil, nil)
	field := NewEdit()
	field.SetFocus()
	button.OnLeftDown(1, 1)
	if !button.HasFocus() {
		t.Fatal("default button behavior changed")
	}
	bar.SetPreserveContentFocus(true)
	field.SetFocus()
	button.OnLeftDown(1, 1)
	if !field.HasFocus() || !button.pushed {
		t.Fatal("tool click stole focus or failed to arm")
	}
	button.SetFocus()
	if !button.HasFocus() || !isTabFocusable(button) {
		t.Fatal("keyboard accessibility lost")
	}
	embedded := NewEdit()
	bar.AddWidget(embedded)
	field.SetFocus()
	embedded.OnLeftDown(1, 1)
	if !embedded.HasFocus() {
		t.Fatal("embedded input could not focus")
	}
	button.SetParent(nil)
	field.SetFocus()
	button.OnLeftDown(1, 1)
	if !button.HasFocus() {
		t.Fatal("old toolbar policy leaked after reparenting")
	}
	bar.AddWidget(button)
	field.SetFocus()
	button.Action().BindFunc0(func() { embedded.SetFocus() })
	button.OnLeftDown(1, 1)
	button.Action().Trigger(nil)
	if !embedded.HasFocus() {
		t.Fatal("explicit action focus was rolled back")
	}
}

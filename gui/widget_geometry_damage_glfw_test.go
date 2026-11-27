//go:build !windows

package gui

import (
	"testing"

	"github.com/uk0/silk/geom"
)

type geometryViewportFixture struct {
	Widget
	clip geom.Rect
}

func (p *geometryViewportFixture) ChildViewport() geom.Rect { return p.clip }

func geometryDamageFixture() (*Window, *geometryViewportFixture, *Widget) {
	root := NewForm()
	root.SetBounds(600, 400, 420, 340)
	owner := &Window{widget: root}
	root.win = owner
	host := &geometryViewportFixture{clip: geom.Rect{X: 5, Y: 6, Width: 160, Height: 137}}
	host.Init(host)
	host.SetBounds(40, 30, 200, 170)
	host.SetParent(root)
	w := &Widget{}
	w.Init(w)
	w.SetBounds(10, 15, 40, 30)
	w.SetParent(host)
	owner.takeFrameDamage()
	return owner, host, w
}

func TestInlineMoveDamagesOldAndNewVisiblePaintBounds(t *testing.T) {
	owner, _, w := geometryDamageFixture()
	w.SetPos(147, 130)
	full, damage := owner.takeFrameDamage()
	want := geom.Rect{X: 50, Y: 45, Width: 155, Height: 128}
	if full || damage != want {
		t.Fatalf("move damage %v, full=%t, want clipped client rectangle %v", damage, full, want)
	}
	w.SetPos(147, 130)
	if full, again := owner.takeFrameDamage(); full || again != (geom.Rect{}) {
		t.Fatal("unchanged coordinates repainted the owner")
	}
}

func TestInlineResizeErasesTheVacatedPixels(t *testing.T) {
	owner, _, w := geometryDamageFixture()
	w.SetSize(20, 18)
	full, damage := owner.takeFrameDamage()
	if want := (geom.Rect{X: 50, Y: 45, Width: 40, Height: 30}); full || damage != want {
		t.Fatalf("shrink damage %v full=%t; old footprint %v was not erased", damage, full, want)
	}
}

func TestInlineGeometryRepairsAnExitButSkipsInvisibleChildren(t *testing.T) {
	owner, host, w := geometryDamageFixture()
	w.SetPos(200, 155)
	full, damage := owner.takeFrameDamage()
	if want := (geom.Rect{X: 50, Y: 45, Width: 40, Height: 30}); full || damage != want {
		t.Fatalf("viewport exit damage %v full=%t; want only the old clipped footprint %v", damage, full, want)
	}
	w.SetPos(201, 160)
	if full, damage = owner.takeFrameDamage(); full || damage != (geom.Rect{}) {
		t.Fatal("fully clipped widget requested a repaint")
	}
	host.setVisible(false)
	owner.takeFrameDamage()
	w.SetPos(10, 15)
	if full, damage = owner.takeFrameDamage(); full || damage != (geom.Rect{}) {
		t.Fatal("hidden ancestor repainted its geometry")
	}
}

func TestDamageExposedPopupStopsAtItsIndependentForm(t *testing.T) {
	fake := func(parent IWidget, wt WindowType, w, h float64) *Window {
		widget := &Widget{}
		widget.Init(widget)
		widget.SetSize(w, h)
		if parent != nil {
			widget.SetParent(parent)
		}
		win := &Window{widget: widget, wt: wt}
		widget.win, widget.wt = win, wt
		return win
	}
	unrelated := fake(nil, WtForm, 400, 260)
	form := fake(unrelated.widget, WtForm, 200, 150)
	parentPopup := fake(form.widget, WtPopup, 80, 50)
	moving := fake(parentPopup.widget, WtPopup, 45, 25)
	old := geom.Rect{X: 44, Y: 40, Width: 52, Height: 31}
	damagePopupUnderlay(moving, old)
	for w, want := range map[*Window]geom.Rect{
		form: old, parentPopup: {X: 44, Y: 40, Width: 36, Height: 10},
		moving: {}, unrelated: {},
	} {
		if full, got := w.takeFrameDamage(); full || got != want {
			t.Fatalf("own exposed damage mismatch: wt %v, %v (full=%t), want %v", w.wt, got, full, want)
		}
	}
}

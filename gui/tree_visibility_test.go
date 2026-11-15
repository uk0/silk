package gui

import "testing"

type treeVisibilityProbe struct {
	Widget
	values       []bool
	shows, hides int
	changed      func(bool)
}

func (p *treeVisibilityProbe) OnTreeVisibilityChanged(on bool) {
	p.values = append(p.values, on)
	if p.changed != nil {
		p.changed(on)
	}
}
func (p *treeVisibilityProbe) OnShow() { p.shows++ }
func (p *treeVisibilityProbe) OnHide() { p.hides++ }
func TestWidgetTreeVisibilityOptInAncestorAndReparent(t *testing.T) {
	a, b := new(Widget), new(Widget)
	a.Init(a)
	b.Init(b)
	b.SetVisible(false)
	child := new(treeVisibilityProbe)
	child.Init(child)
	child.SetParent(a)
	if !WidgetTreeVisible(child) || WidgetTreeVisible(nil) {
		t.Fatal("logical visibility incorrect")
	}
	a.SetVisible(false)
	if len(child.values) == 0 || child.values[len(child.values)-1] || child.hides != 0 {
		t.Fatal("ancestor hide lost or local event broadcast")
	}
	a.SetVisible(true)
	if !child.values[len(child.values)-1] || child.shows != 0 {
		t.Fatal("ancestor show lost or local event broadcast")
	}
	child.SetParent(b)
	if child.values[len(child.values)-1] {
		t.Fatal("reparent into hidden tree visible")
	}
	child.SetParent(a)
	if !child.values[len(child.values)-1] {
		t.Fatal("reparent into visible tree hidden")
	}
	child.SetVisible(false)
	if child.hides != 1 || WidgetTreeVisible(child) {
		t.Fatal("local hide changed semantics")
	}
}
func TestWidgetTreeVisibilityReentrantLocalHide(t *testing.T) {
	parent := new(Widget)
	parent.Init(parent)
	child := new(treeVisibilityProbe)
	child.Init(child)
	child.SetParent(parent)
	child.changed = func(on bool) {
		if !on {
			child.SetVisible(false)
		}
	}
	parent.SetVisible(false)
	if child.hides != 1 || WidgetTreeVisible(child) {
		t.Fatal("reentrant hide was not finite/idempotent")
	}
}

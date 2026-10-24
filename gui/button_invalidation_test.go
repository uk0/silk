package gui

import "testing"

// Capture invalidation without an idle sweep or native mouse input.
type buttonInvalidationProbe struct {
	Button
	updates int
}

func (p *buttonInvalidationProbe) Update() { p.updates++ }

func TestButtonVisualSettersInvalidateWithoutIdle(t *testing.T) {
	p := new(buttonInvalidationProbe)
	p.Init(p)
	p.SetText("Cancel")
	p.updates = 0
	p.SetText("Download")
	if p.updates != 1 || p.Text() != "Download" {
		t.Fatalf("text changed without invalidation: %d %q", p.updates, p.Text())
	}
	p.SetText("Download")
	if p.updates != 1 {
		t.Fatal("unchanged text should not repaint")
	}
	p.SetIcon(nil)
	p.SetTextVisible(true)
	if p.updates != 3 {
		t.Fatalf("icon/visibility setters did not invalidate: %d", p.updates)
	}
	p.SetTextVisible(true)
	if p.updates != 3 {
		t.Fatal("unchanged visibility should not repaint")
	}
}

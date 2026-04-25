package gui

import (
	"testing"
)

func TestNewLabel(t *testing.T) {
	l := NewLabel("hello")
	if l.Text() != "hello" {
		t.Errorf("Label.Text() = %q, want %q", l.Text(), "hello")
	}
	l.SetText("world")
	if l.Text() != "world" {
		t.Errorf("after SetText, Text() = %q, want %q", l.Text(), "world")
	}
	l.SetAlign(AlignCenter)
	if l.Align() != AlignCenter {
		t.Errorf("Align() = %d, want AlignCenter", l.Align())
	}
}

func TestNewButton(t *testing.T) {
	b := NewButton1("click", nil)
	if b.Text() != "click" {
		t.Errorf("Button.Text() = %q, want %q", b.Text(), "click")
	}
	if !b.IsEnabled() {
		t.Error("Button should be enabled by default")
	}
	b.SetEnabled(false)
	if b.IsEnabled() {
		t.Error("Button should be disabled after SetEnabled(false)")
	}
}

func TestNewCheckBox(t *testing.T) {
	cb := NewCheckBox()
	cb.SetText("option")
	if cb.Text() != "option" {
		t.Errorf("CheckBox.Text() = %q", cb.Text())
	}
	if cb.IsChecked() {
		t.Error("CheckBox should be unchecked by default")
	}
	cb.SetChecked(true)
	if !cb.IsChecked() {
		t.Error("CheckBox should be checked after SetChecked(true)")
	}
}

func TestNewEdit(t *testing.T) {
	e := NewEdit()
	e.SetText("test input")
	if e.Text() != "test input" {
		t.Errorf("Edit.Text() = %q", e.Text())
	}
}

func TestNewProgressBar(t *testing.T) {
	pb := NewProgressBar()
	pb.SetValue(0.75)
	if pb.Value() != 0.75 {
		t.Errorf("ProgressBar.Value() = %f, want 0.75", pb.Value())
	}
	// Clamp to [0, 1]
	pb.SetValue(1.5)
	if pb.Value() > 1.0 {
		t.Error("ProgressBar should clamp to 1.0")
	}
	pb.SetValue(-0.5)
	if pb.Value() < 0.0 {
		t.Error("ProgressBar should clamp to 0.0")
	}
}

func TestNewSlider(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetValue(50)
	if s.Value() != 50 {
		t.Errorf("Slider.Value() = %f, want 50", s.Value())
	}
	s.SetRange(10, 20)
	if s.Min() != 10 || s.Max() != 20 {
		t.Errorf("Slider range = [%f, %f], want [10, 20]", s.Min(), s.Max())
	}
}

func TestNewSpinBox(t *testing.T) {
	sp := NewSpinBox()
	sp.SetRange(0, 50)
	sp.SetValue(25)
	if sp.Value() != 25 {
		t.Errorf("SpinBox.Value() = %d, want 25", sp.Value())
	}
}

func TestNewGroupBox(t *testing.T) {
	gb := NewGroupBox("Settings")
	if gb.Title() != "Settings" {
		t.Errorf("GroupBox.Title() = %q, want %q", gb.Title(), "Settings")
	}
	gb.SetTitle("Options")
	if gb.Title() != "Options" {
		t.Errorf("after SetTitle, Title() = %q", gb.Title())
	}
}

func TestVBoxLayout(t *testing.T) {
	vb := NewVBox()
	vb.SetSpacing(5)
	if vb.Spacing() != 5 {
		t.Errorf("VBox.Spacing() = %f, want 5", vb.Spacing())
	}
	// Add children
	l1 := NewLabel("a")
	l1.SetParent(vb)
	l2 := NewLabel("b")
	l2.SetParent(vb)

	children := vb.Children()
	if len(children) != 2 {
		t.Errorf("VBox has %d children, want 2", len(children))
	}
}

func TestHBoxLayout(t *testing.T) {
	hb := NewHBox()
	hb.SetSpacing(10)
	if hb.Spacing() != 10 {
		t.Errorf("HBox.Spacing() = %f, want 10", hb.Spacing())
	}
}

func TestWidgetVisibility(t *testing.T) {
	l := NewLabel("test")
	if !l.IsVisible() {
		t.Error("Widget should be visible by default")
	}
	l.Hide()
	if l.IsVisible() {
		t.Error("Widget should be hidden after Hide()")
	}
	l.Show()
	if !l.IsVisible() {
		t.Error("Widget should be visible after Show()")
	}
}

func TestWidgetBounds(t *testing.T) {
	l := NewLabel("test")
	l.SetBounds(10, 20, 100, 50)
	x, y, w, h := l.Bounds()
	if x != 10 || y != 20 || w != 100 || h != 50 {
		t.Errorf("Bounds() = (%v,%v,%v,%v), want (10,20,100,50)", x, y, w, h)
	}
}

func TestWidgetParentChild(t *testing.T) {
	parent := NewForm()
	child := NewLabel("child")
	child.SetParent(parent)

	if child.Parent() != parent.Self() {
		t.Error("child.Parent() should be parent")
	}
	children := parent.Children()
	if len(children) == 0 {
		t.Error("parent should have children")
	}
}

func TestThemeMode(t *testing.T) {
	// Save current mode
	orig := CurrentThemeMode()
	defer SetThemeMode(orig)

	SetThemeMode(ThemeLight)
	if CurrentThemeMode() != ThemeLight {
		t.Error("should be light mode")
	}
	SetThemeMode(ThemeDark)
	if CurrentThemeMode() != ThemeDark {
		t.Error("should be dark mode")
	}
}

func TestTextAlign(t *testing.T) {
	if AlignLeft != 0 || AlignCenter != 1 || AlignRight != 2 {
		t.Error("TextAlign constants incorrect")
	}
}

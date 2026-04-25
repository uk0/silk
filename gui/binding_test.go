package gui

import (
	"testing"
)

func TestBindingBasic(t *testing.T) {
	b := NewBinding("hello")
	if b.GetString() != "hello" {
		t.Error("initial value should be hello")
	}

	var received string
	b.Watch(func(v interface{}) { received = v.(string) })

	b.Set("world")
	if received != "world" {
		t.Error("watcher not called")
	}
	if b.GetString() != "world" {
		t.Error("value not updated")
	}
}

func TestBindingNoChangeSkip(t *testing.T) {
	b := NewBinding("same")
	called := false
	b.Watch(func(v interface{}) { called = true })

	b.Set("same")
	if called {
		t.Error("watcher should not fire when value is unchanged")
	}
}

func TestBindingTypes(t *testing.T) {
	b := NewBinding(42)
	if b.GetInt() != 42 {
		t.Error("GetInt")
	}
	if b.GetFloat() != 42.0 {
		t.Error("GetFloat from int")
	}

	b2 := NewBinding(true)
	if !b2.GetBool() {
		t.Error("GetBool")
	}

	b3 := NewBinding(3.14)
	if b3.GetFloat() != 3.14 {
		t.Error("GetFloat from float64")
	}
	if b3.GetInt() != 3 {
		t.Error("GetInt from float64")
	}

	b4 := NewBinding(float32(2.5))
	if b4.GetFloat() != 2.5 {
		t.Error("GetFloat from float32")
	}
	if b4.GetInt() != 2 {
		t.Error("GetInt from float32")
	}
}

func TestBindingGetStringFallback(t *testing.T) {
	b := NewBinding(42)
	s := b.GetString()
	if s != "42" {
		t.Errorf("GetString from int = %q, want 42", s)
	}
}

func TestBindingMultipleWatchers(t *testing.T) {
	b := NewBinding(0)
	count := 0
	b.Watch(func(v interface{}) { count++ })
	b.Watch(func(v interface{}) { count++ })

	b.Set(1)
	if count != 2 {
		t.Errorf("expected 2 watchers called, got %d", count)
	}
}

func TestBindingRecursionGuard(t *testing.T) {
	b := NewBinding(0)
	b.Watch(func(v interface{}) {
		// Attempt recursive Set -- should be a no-op
		b.Set(999)
	})

	b.Set(1)
	if b.GetInt() != 1 {
		t.Errorf("value should be 1 after guarded recursive set, got %d", b.GetInt())
	}
}

func TestBindLabel(t *testing.T) {
	lbl := NewLabel("")
	b := NewBinding("test")
	BindLabel(lbl, b)
	if lbl.Text() != "test" {
		t.Errorf("initial bind: got %q, want test", lbl.Text())
	}

	b.Set("updated")
	if lbl.Text() != "updated" {
		t.Errorf("after Set: got %q, want updated", lbl.Text())
	}
}

func TestBindEdit(t *testing.T) {
	edit := NewEdit()
	b := NewBinding("hello")
	BindEdit(edit, b)
	if edit.Text() != "hello" {
		t.Errorf("initial: got %q, want hello", edit.Text())
	}

	b.Set("world")
	if edit.Text() != "world" {
		t.Errorf("binding->edit: got %q, want world", edit.Text())
	}
}

func TestBindProgressBar(t *testing.T) {
	pb := NewProgressBar()
	b := NewBinding(0.5)
	BindProgressBar(pb, b)
	if pb.Value() != 0.5 {
		t.Errorf("initial: got %f, want 0.5", pb.Value())
	}

	b.Set(0.8)
	if pb.Value() != 0.8 {
		t.Errorf("after Set: got %f, want 0.8", pb.Value())
	}
}

func TestBindSlider(t *testing.T) {
	slider := NewSlider(0, 100)
	b := NewBinding(50.0)
	BindSlider(slider, b)
	if slider.Value() != 50 {
		t.Errorf("initial: got %f, want 50", slider.Value())
	}

	b.Set(75.0)
	if slider.Value() != 75 {
		t.Errorf("binding->slider: got %f, want 75", slider.Value())
	}
}

func TestBindCheckBox(t *testing.T) {
	cb := NewCheckBox()
	b := NewBinding(true)
	BindCheckBox(cb, b)
	if !cb.IsChecked() {
		t.Error("initial: should be checked")
	}

	b.Set(false)
	if cb.IsChecked() {
		t.Error("after Set(false): should be unchecked")
	}
}

func TestBindSpinBox(t *testing.T) {
	sp := NewSpinBox()
	sp.SetRange(0, 100)
	b := NewBinding(25)
	BindSpinBox(sp, b)
	if sp.Value() != 25 {
		t.Errorf("initial: got %d, want 25", sp.Value())
	}

	b.Set(50)
	if sp.Value() != 50 {
		t.Errorf("binding->spinbox: got %d, want 50", sp.Value())
	}
}

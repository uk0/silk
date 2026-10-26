package main

import (
	"fmt"
	"github.com/uk0/silk/gui"
	"testing"
)

func TestTabDemoPageIdentityAndWrapping(t *testing.T) {
	f := newTabDemo()
	f.SetSize(760, 430)
	f.OnResize()
	original := f.state()
	f.step(-1)
	if f.state().Index != 5 {
		t.Fatal("previous did not wrap")
	}
	f.step(1)
	if f.state().Text != original.Text {
		t.Fatal("page text changed during navigation")
	}
	page := f.tabs.Stack().CurrentPage().(*gui.CodeEditor)
	page.OnTextInput("X")
	text := page.Text()
	f.step(1)
	f.step(-1)
	if f.state().Text != text {
		t.Fatal("page edit lost")
	}
	f.tabs.RemoveTab(1)
	if f.state().Text != text || len(f.state().Order) != 5 {
		t.Fatal("removing another page lost identity")
	}
}
func TestTabDemoLayoutAndButtonAssets(t *testing.T) {
	f := newTabDemo()
	for _, b := range f.buttons {
		if !b.IsTextVisible() || b.Icon() == nil || b.Icon().IsAir() || fmt.Sprint(b.Icon()) == "image-missing" {
			t.Fatal("missing button label/icon", b.Text())
		}
	}
	for _, size := range [][2]float64{{760, 430}, {430, 420}, {185, 200}, {40, 20}, {0, 0}} {
		f.SetSize(size[0], size[1])
		for _, narrow := range []bool{false, true} {
			f.narrow = narrow
			f.OnResize()
			for _, c := range f.Children() {
				x, y, w, h := c.Bounds()
				if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
					t.Fatal("child escaped", size, c)
				}
			}
		}
	}
}

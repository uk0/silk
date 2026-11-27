package main

import (
	"testing"

	"github.com/uk0/silk/gui"
)

func TestPreviewWindowPreservesDesignedContentAfterDockAttachment(t *testing.T) {
	for _, size := range [][2]float64{{756, 756}, {600, 400}, {120, 80}} {
		form := gui.NewForm()
		form.SetSize(size[0], size[1])
		desiredW, desiredH := form.Size()
		frame := newPreviewFrame("edge placement", form)
		t.Cleanup(frame.Close)
		if frame.Window() != nil || frame.IsVisible() {
			t.Fatal("preview geometry must be ready before attaching a native window")
		}
		if form.Width() < desiredW || form.Height() < desiredH {
			t.Fatalf("preview clipped designed %vx%v to %vx%v", desiredW, desiredH, form.Width(), form.Height())
		}
		if frame.Width() < 320 || frame.Height() < 240 {
			t.Fatal("tiny preview lost minimum window size")
		}
		child := gui.NewButton1("edge", nil)
		child.SetBounds(desiredW-40, desiredH-25, 40, 25)
		child.SetParent(form)
		if child.X()+child.Width() > form.Width() || child.Y()+child.Height() > form.Height() {
			t.Fatal("edge child clipped by window chrome")
		}
	}
}

func TestPreviewWindowResizesManagedFormWithDock(t *testing.T) {
	form := gui.NewForm()
	form.SetSize(420, 330)
	root := gui.NewVBox()
	root.SetParent(form)
	root.AddWidget(gui.NewLabel("status"))
	if !form.SetLayoutRoot(root) {
		t.Fatal("could not configure a responsive form")
	}
	frame := newPreviewFrame("responsive form", form)
	t.Cleanup(frame.Close)
	if root.Width() != form.Width() || root.Height() != form.Height() {
		t.Fatal("opening preview did not arrange the form")
	}
	for _, size := range [][2]float64{{860, 640}, {520, 390}} {
		frame.SetSize(size[0], size[1])
		frame.Layout()
		if root.Parent() != form || root.Width() != form.Width() || root.Height() != form.Height() || root.Width() <= 0 || root.Height() <= 0 {
			t.Fatalf("preview dock failed to reflow after %vx%v", size[0], size[1])
		}
	}
}

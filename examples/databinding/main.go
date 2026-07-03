// Silk Data Binding Demo
//
// Demonstrates the reactive data binding system. A single Binding
// connects a Slider, ProgressBar, SpinBox, and Label so that
// changing any input widget automatically updates all the others.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o databinding ./examples/databinding/
//
// Run:
//
//	./databinding
package main

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Data Binding Demo")

	// --- Shared binding: progress value in range [0, 1] ---
	progress := gui.NewBinding(0.5)

	// Title
	title := gui.NewLabel("Reactive Data Binding")
	title.SetFont(paint.NewFont(gui.Theme().Font.Family(), 16, true, false))
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(20, 10, 360, 28)

	// ProgressBar bound to `progress`
	gui.NewLabel("Progress:").SetParent(form)
	form.Children()[len(form.Children())-1].(gui.IWidget).SetBounds(20, 50, 70, 22)

	pb := gui.NewProgressBar()
	pb.SetParent(form)
	pb.SetBounds(95, 48, 220, 22)
	gui.BindProgressBar(pb, progress)

	// Percentage label bound to `progress`
	pctLabel := gui.NewLabel("50%")
	pctLabel.SetParent(form)
	pctLabel.SetBounds(320, 50, 50, 22)
	progress.Watch(func(v interface{}) {
		pct := int(progress.GetFloat()*100 + 0.5)
		pctLabel.SetText(fmt.Sprintf("%d%%", pct))
	})

	// Slider: uses 0-100 range, converts to/from 0-1 binding
	gui.NewLabel("Slider:").SetParent(form)
	form.Children()[len(form.Children())-1].(gui.IWidget).SetBounds(20, 85, 70, 22)

	slider := gui.NewSlider(0, 100)
	slider.SetParent(form)
	slider.SetBounds(95, 83, 220, 22)
	slider.SetValue(progress.GetFloat() * 100)

	// Slider -> Binding (scaled)
	slider.SetValueChangedCallback(func(_ interface{}, v float64) {
		progress.Set(v / 100.0)
	})
	// Binding -> Slider (scaled)
	progress.Watch(func(v interface{}) {
		slider.SetValue(progress.GetFloat() * 100)
	})

	// --- Second binding: a text value ---
	textBinding := gui.NewBinding("Hello, Silk!")

	gui.NewLabel("Text:").SetParent(form)
	form.Children()[len(form.Children())-1].(gui.IWidget).SetBounds(20, 125, 70, 22)

	edit := gui.NewEdit()
	edit.SetParent(form)
	edit.SetBounds(95, 122, 220, 26)
	gui.BindEdit(edit, textBinding)

	mirrorLabel := gui.NewLabel("")
	mirrorLabel.SetParent(form)
	mirrorLabel.SetBounds(95, 155, 220, 22)
	gui.BindLabel(mirrorLabel, textBinding)

	// --- Third binding: boolean toggle ---
	boolBinding := gui.NewBinding(false)

	cb := gui.NewCheckBox()
	cb.SetText("Enable feature")
	cb.SetParent(form)
	cb.SetBounds(95, 185, 150, 22)
	gui.BindCheckBox(cb, boolBinding)

	statusLabel := gui.NewLabel("Disabled")
	statusLabel.SetParent(form)
	statusLabel.SetBounds(250, 185, 80, 22)
	boolBinding.Watch(func(v interface{}) {
		if boolBinding.GetBool() {
			statusLabel.SetText("Enabled")
		} else {
			statusLabel.SetText("Disabled")
		}
	})

	// Show form
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(400, 230)
	form.Window().MoveToCenter()
	form.Show()

	core.EventLoop()
}

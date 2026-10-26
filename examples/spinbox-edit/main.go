// Spinbox editing demonstrates integer entry, safe steps and modal composition.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"math"
)

type spinDemo struct {
	gui.Form
	count, edge    *gui.SpinBox
	status         *gui.Label
	widgets        []gui.IWidget
	changes, modal int
	dark           bool
}

func newSpinDemo() *spinDemo {
	f := new(spinDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Integer Editor")
	add := func(w gui.IWidget) { w.SetParent(f); f.widgets = append(f.widgets, w) }
	add(gui.NewLabel("Type a signed integer; Enter or Tab commits, Escape cancels the draft."))
	f.count = gui.NewSpinBox()
	f.count.SetRange(-100, 100)
	f.count.SetValue(10)
	f.count.SetSuffix(" items")
	add(f.count)
	add(gui.NewLabel("Full int range: arrows and wheel must saturate, never wrap."))
	f.edge = gui.NewSpinBox()
	f.edge.SetRange(math.MinInt, math.MaxInt)
	f.edge.SetValue(math.MaxInt - 1)
	f.edge.SetStep(4)
	add(f.edge)
	f.status = gui.NewLabel("")
	add(f.status)
	button := func(s string, fn func()) { b := gui.NewButton1(s, nil); b.Action().BindFunc0(fn); add(b) }
	button("Reset values", func() { f.count.SetValue(10); f.edge.SetValue(math.MaxInt - 1); f.refresh(); f.snapshot() })
	button("Toggle enabled", func() { f.count.SetEnabled(!f.count.IsEnabled()); f.refresh(); f.snapshot() })
	button("Toggle theme", func() {
		f.dark = !f.dark
		if f.dark {
			gui.SetThemeMode(gui.ThemeDark)
		} else {
			gui.SetThemeMode(gui.ThemeLight)
		}
		f.Update()
	})
	button("Open modal editor", f.openModal)
	button("Snapshot", f.snapshot)
	changed := func(_ interface{}, _ int) { f.changes++; f.refresh() }
	f.count.SetValueChangedCallback(changed)
	f.edge.SetValueChangedCallback(changed)
	f.refresh()
	return f
}
func (f *spinDemo) refresh() {
	f.status.SetText(fmt.Sprintf("Value: %d | changes: %d | modal: %d", f.count.Value(), f.changes, f.modal))
}
func (f *spinDemo) snapshot() {
	b, _ := json.Marshal(map[string]any{"Value": f.count.Value(), "Edge": f.edge.Value(), "Enabled": f.count.IsEnabled(), "Changes": f.changes, "Modal": f.modal})
	fmt.Println("SNAPSHOT " + string(b))
}
func (f *spinDemo) openModal() {
	d := gui.NewDialog("Integer modal", f)
	s := gui.NewSpinBox()
	s.SetRange(1, 99)
	s.SetValue(5)
	s.SetSuffix(" copies")
	d.SetContent(s)
	d.SetSize(420, 150)
	d.AddButton("Cancel", gui.DialogCancel)
	d.AddButton("Apply", gui.DialogOK)
	if d.ShowModal() == gui.DialogOK {
		f.modal = s.Value()
	}
	f.refresh()
	f.snapshot()
}
func (f *spinDemo) OnResize() {
	w, h := f.Size()
	w, h = math.Max(0, w), math.Max(0, h)
	for i, c := range f.widgets {
		y := 16 + float64(i)*44
		c.SetBounds(math.Min(16, w), math.Min(y, h), math.Max(0, w-32), math.Max(0, math.Min(36, h-y)))
	}
}
func main() {
	defer core.Close()
	f := newSpinDemo()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(720, 480)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}

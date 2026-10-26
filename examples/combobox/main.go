// ComboBox demonstrates selection-only values, free text and popup navigation.
package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type comboDemo struct {
	gui.Form
	choice, editable *gui.ComboBox
	widgets          []gui.IWidget
	changes, submits int
	status           *gui.Label
}

func newComboDemo() *comboDemo {
	f := new(comboDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - ComboBox Interactions")
	add := func(w gui.IWidget) { w.SetParent(f); f.widgets = append(f.widgets, w) }
	add(gui.NewLabel("Select-only: type a prefix, arrows/PageUp/PageDown, Home/End."))
	f.choice = gui.NewComboBox()
	f.choice.SetEditable(false)
	f.choice.Append(gui.ListItem{Text: "Apple", Icon: gui.Theme().CheckedIcon})
	for _, s := range []string{"Apricot", "简体中文", "😀 emoji"} {
		f.choice.Append(gui.ListItem{Text: s})
	}
	for i := 4; i < 100; i++ {
		f.choice.Append(gui.ListItem{Text: fmt.Sprintf("Item %03d", i)})
	}
	f.choice.SetActiveIndex(0)
	add(f.choice)
	add(gui.NewLabel("F4 or Alt+Down opens; Enter commits; Escape cancels the preview."))
	f.editable = gui.NewComboBox()
	f.editable.SetItems([]string{"Go", "Rust", "Python"})
	f.editable.SetActiveIndex(0)
	add(f.editable)
	add(gui.NewLabel("Editable: enter any text; Up/Down selects; Home/End moves the caret."))
	f.status = gui.NewLabel("")
	add(f.status)
	snapshot := gui.NewButton1("Snapshot", nil)
	snapshot.Action().BindFunc0(f.snapshot)
	add(snapshot)
	f.choice.SigSelectionChanged(func(interface{}, int) { f.changes++; f.refresh(); f.snapshot() })
	f.choice.SigSubmit(func(interface{}) { f.submits++; f.refresh(); f.snapshot() })
	f.refresh()
	return f
}
func (f *comboDemo) refresh() {
	f.status.SetText(fmt.Sprintf("Committed: %s | Changes: %d | Submits: %d", f.choice.Text(), f.changes, f.submits))
}
func (f *comboDemo) snapshot() {
	data, _ := json.Marshal(struct {
		Choice, Editable        string
		Index, Changes, Submits int
		Popup                   bool
	}{f.choice.Text(), f.editable.Text(), f.choice.ActiveIndex(), f.changes, f.submits, f.choice.IsSubPopupVisible()})
	fmt.Println("SNAPSHOT " + string(data))
}
func (f *comboDemo) OnResize() {
	w, h := f.Size()
	for i, c := range f.widgets {
		y := 16 + float64(i)*40
		c.SetBounds(16, math.Min(y, h), math.Max(0, w-32), math.Max(0, math.Min(32, h-y)))
	}
}
func main() {
	defer core.Close()
	f := newComboDemo()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(680, 330)
	f.Window().MoveToCenter()
	f.Show()
	f.choice.SetFocus()
	core.EventLoop()
}

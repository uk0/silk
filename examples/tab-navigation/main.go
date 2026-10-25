// Tab Navigation demonstrates child-focus keyboard routing and coherent,
// exactly-once TabWidget changes. Run: go run ./examples/tab-navigation
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func main() {
	defer core.Close()
	form := gui.NewForm()
	form.SetTitle("Silk - Tab Navigation")
	help := gui.NewLabel("Temporary values. Right-click / Shift+F10: page menu. Ctrl+Tab: switch.")
	help.SetParent(form)
	help.SetBounds(20, 15, 720, 28)
	status := gui.NewLabel("")
	status.SetParent(form)
	status.SetBounds(20, 300, 720, 28)
	tabs := gui.NewTabWidget()
	tabs.SetParent(form)
	tabs.SetBounds(20, 110, 720, 170)
	changes := 0
	var previous, next *gui.Button
	syncButtons := func() {
		for _, b := range []*gui.Button{previous, next} {
			if b != nil {
				b.SetEnabled(tabs.Count() > 1)
			}
		}
	}
	tabs.TabBar().SetCloseCallback(func(_ *gui.TabBar, index int) bool {
		tabs.RemoveTab(index) // this sample keeps no persistent documents
		syncButtons()         // inactive-page removal may not change the current page
		return true
	})
	tabs.SetCurrentChangedCallback(func(_ interface{}, index int) {
		changes++
		text := fmt.Sprintf("Changed %d: %d %s", changes, index, tabs.TabTitle(index))
		status.SetText(text)
		fmt.Println(text)
		syncButtons()
	})
	var fields []*gui.Edit
	for _, name := range []string{"Alpha", "Beta", "Zulu"} {
		page := gui.NewVBox()
		field := gui.NewEdit()
		field.SetText(name)
		page.AddWidget(gui.NewLabel("Type here; switching keeps each page's value."))
		page.AddWidget(field)
		tabs.AddTab(page, name, nil)
		fields = append(fields, field)
	}
	tabs.TabBar().SetContextMenuCallback(func(_ *gui.TabBar, menu *gui.Menu, index int) {
		page := tabs.Stack().Page(index)
		label := menu.AddButton1(tabs.TabTitle(index), nil)
		label.SetEnabled(false)
		menu.AddSeparator()
		menu.AddButton1("Close Page", paint.LoadIcon("close")).Action().BindFunc0(func() {
			// Resolve the original page, never a slot captured before a reorder.
			for i := 0; i < tabs.Count(); i++ {
				if tabs.Stack().Page(i) == page {
					tabs.TabBar().CloseTab(i)
					return
				}
			}
		})
	})
	add := func(x float64, label string, run func()) *gui.Button {
		b := gui.NewButton1(label, nil)
		b.SetParent(form)
		b.SetBounds(x, 60, 130, 34)
		b.Action().BindFunc0(run)
		return b
	}
	previous = add(20, "Previous", func() { tabs.NavigateTab(true) })
	next = add(165, "Next", func() { tabs.NavigateTab(false) })
	add(310, "Snapshot", func() {
		texts := make([]string, len(fields))
		for i, f := range fields {
			texts[i] = f.Text()
		}
		data, _ := json.Marshal(struct {
			Index, Changes int
			Texts          []string
		}{tabs.CurrentIndex(), changes, texts})
		fmt.Println("SNAPSHOT " + string(data))
	})
	add(455, "Close", core.Quit)
	syncButtons()
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(760, 350)
	form.Window().MoveToCenter()
	form.Show()
	fields[0].SetFocus()
	core.EventLoop()
}

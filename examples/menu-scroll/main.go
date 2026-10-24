// Menu Scroll demonstrates SDK popup navigation with more rows than a screen
// can hold. Run: go run ./examples/menu-scroll
package main

import (
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func main() {
	defer core.Close()
	form := gui.NewForm()
	form.SetTitle("Silk - Long Menu Playground")
	help := gui.NewLabel("Wheel / arrows / Page Up / Page Down / Home / End; type a label, Enter to run.")
	help.SetParent(form)
	help.SetBounds(20, 20, 700, 28)
	status := gui.NewLabel("No command executed")
	status.SetParent(form)
	status.SetBounds(20, 130, 700, 32)
	count := 0
	record := func(label string) {
		count++
		text := fmt.Sprintf("Executed %d: %s", count, label)
		status.SetText(text)
		fmt.Println(text)
	}
	menu := gui.NewPopupMenu()
	for i := 0; i < 80; i++ {
		label := fmt.Sprintf("Command %02d", i)
		if i == 0 {
			label = "Alpha - first command"
		}
		if i == 79 {
			label = "Zulu - last command"
		}
		if i == 40 {
			sub, _ := menu.AddSubMenu("Nested tasks", nil, nil)
			for j := 0; j < 60; j++ {
				child := fmt.Sprintf("Task %02d", j)
				if j == 59 {
					child = "Omega - last nested task"
				}
				sub.AddButton1(child, nil).Action().BindFunc0(func() { record(child) })
			}
		} else {
			b := menu.AddButton1(label, nil)
			b.Action().BindFunc0(func() { record(label) })
			if i == 5 {
				b.SetEnabled(false)
			}
			if i == 6 {
				b.Hide()
			}
		}
		if i%10 == 9 {
			menu.AddSeparator()
		}
	}
	open := gui.NewButton1("Commands (80)", nil)
	open.SetParent(form)
	open.SetBounds(20, 65, 190, 36)
	open.SetSubPopup(menu)
	close := gui.NewButton1("Close", nil)
	close.SetParent(form)
	close.SetBounds(230, 65, 110, 36)
	close.Action().BindFunc0(core.Quit)
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(760, 210)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

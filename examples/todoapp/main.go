package main

import (
	"silk/core"
	"silk/gui"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Todo App")

	// Input field
	edit := gui.NewEdit()
	edit.SetParent(form)
	edit.SetBounds(10, 10, 260, 28)
	edit.SetText("")

	// Add button
	btnAdd := gui.NewButton1("添加", nil)
	btnAdd.SetParent(form)
	btnAdd.SetBounds(280, 10, 60, 28)

	// Task list with checkboxes
	list := gui.NewListWidget()
	list.SetParent(form)
	list.SetBounds(10, 48, 330, 200)
	list.SetCheckBoxVisible(true)

	// Add button action
	btnAdd.Action().BindFunc0(func() {
		text := edit.Text()
		if text != "" {
			list.Append(gui.ListItem{Text: text})
			edit.SetText("")
		}
	})

	// Delete completed tasks button
	btnDel := gui.NewButton1("删除已完成", nil)
	btnDel.SetParent(form)
	btnDel.SetBounds(10, 258, 120, 28)
	btnDel.Action().BindFunc0(func() {
		// Remove checked items from bottom to top to preserve indices
		for i := list.Count() - 1; i >= 0; i-- {
			item := list.Item(i)
			if item.Checked {
				list.Remove(i)
			}
		}
	})

	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(350, 300)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

// Package gui provides a cross-platform GUI widget toolkit for Go.
//
// It includes 35+ widgets (Button, Label, Edit, CheckBox, Table, Dialog, etc.),
// a docking window system, theme support (light/dark), and a complete event model.
//
// Platform backends:
//   - Windows: Win32 API (native)
//   - macOS/Linux: GLFW + OpenGL
//
// Basic usage:
//
//	form := gui.NewForm()
//	form.SetTitle("My App")
//
//	btn := gui.NewButton1("Click Me", nil)
//	btn.SetParent(form)
//	btn.SetBounds(10, 10, 100, 30)
//	btn.Action().BindFunc0(func() { fmt.Println("clicked!") })
//
//	form.AttachWindow(gui.WtForm)
//	form.Show()
//	core.EventLoop()
package gui

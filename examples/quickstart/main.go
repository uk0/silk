// Silk Quick Start — Build your first GUI app in 5 minutes
//
// This example creates a simple login form with username/password
// fields, a "Login" button, and a status label.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o login ./examples/quickstart/
//
// Run:
//
//	./login
package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func main() {
	// 1. Create a Form (the main container)
	form := gui.NewForm()
	form.SetTitle("Silk Quick Start - Login")

	// 2. Add a title label
	title := gui.NewLabel("Welcome to Silk")
	title.SetFont(paint.NewFont(gui.Theme().Font.Family(), 18, true, false))
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(20, 15, 260, 28)

	// 3. Add username input
	lblUser := gui.NewLabel("Username:")
	lblUser.SetParent(form)
	lblUser.SetBounds(20, 55, 70, 22)

	editUser := gui.NewEdit()
	editUser.SetParent(form)
	editUser.SetBounds(95, 52, 185, 26)

	// 4. Add password input
	lblPass := gui.NewLabel("Password:")
	lblPass.SetParent(form)
	lblPass.SetBounds(20, 88, 70, 22)

	editPass := gui.NewEdit()
	editPass.SetParent(form)
	editPass.SetBounds(95, 85, 185, 26)

	// 5. Add a login button
	status := gui.NewLabel("")
	status.SetParent(form)
	status.SetBounds(20, 155, 260, 22)

	btnLogin := gui.NewButton1("Login", nil)
	btnLogin.SetParent(form)
	btnLogin.SetBounds(95, 120, 90, 30)
	btnLogin.Action().BindFunc0(func() {
		user := editUser.Text()
		if user == "" {
			status.SetText("Please enter username")
			status.SetTextColor(paint.Color{R: 200, G: 50, B: 50, A: 255})
		} else {
			status.SetText("Welcome, " + user + "!")
			status.SetTextColor(paint.Color{R: 50, G: 150, B: 50, A: 255})
		}
	})

	btnClear := gui.NewButton1("Clear", nil)
	btnClear.SetParent(form)
	btnClear.SetBounds(195, 120, 85, 30)
	btnClear.Action().BindFunc0(func() {
		editUser.SetText("")
		editPass.SetText("")
		status.SetText("")
	})

	// 6. Show the form as a window
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(300, 190)
	form.Window().MoveToCenter()
	form.Show()

	// 7. Run the event loop
	core.EventLoop()
}

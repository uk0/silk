//go:build ignore

// Silk Widget Showcase -- demonstrates the major widget categories.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o showcase ./examples/showcase/
//
// Run:
//
//	./showcase
package main

import (
	"fmt"
	"silk/core"
	"silk/gui"
	"silk/paint"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Widget Showcase")

	y := 10.0
	const col1 = 20.0
	const col2 = 320.0
	const w1 = 260.0
	const rowH = 34.0

	// ---- Section: Input Widgets ----
	sectionLabel(form, "Input Widgets", col1, y)
	y += 28

	// Edit (text input)
	lbl(form, "Edit:", col1, y)
	edit := gui.NewEdit()
	edit.SetParent(form)
	edit.SetBounds(col1+80, y, w1-80, 26)
	edit.SetText("Hello Silk")
	y += rowH

	// SearchBox
	lbl(form, "SearchBox:", col1, y)
	sb := gui.NewSearchBox()
	sb.SetParent(form)
	sb.SetBounds(col1+80, y, w1-80, 28)
	y += rowH

	// NumberInput
	lbl(form, "Number:", col1, y)
	ni := gui.NewNumberInput()
	ni.SetParent(form)
	ni.SetBounds(col1+80, y, 120, 28)
	ni.SetRange(0, 100)
	ni.SetValue(42)
	y += rowH

	// ComboBox
	lbl(form, "ComboBox:", col1, y)
	combo := gui.NewComboBox()
	combo.SetParent(form)
	combo.SetBounds(col1+80, y, w1-80, 26)
	combo.Append(gui.ListItem{Text: "Option A"})
	combo.Append(gui.ListItem{Text: "Option B"})
	combo.Append(gui.ListItem{Text: "Option C"})
	y += rowH

	// DatePicker
	lbl(form, "Date:", col1, y)
	dp := gui.NewDatePicker()
	dp.SetParent(form)
	dp.SetBounds(col1+80, y, w1-80, 28)
	y += rowH

	// ColorPicker
	lbl(form, "Color:", col1, y)
	cp := gui.NewColorPicker()
	cp.SetParent(form)
	cp.SetBounds(col1+80, y, w1-80, 28)
	y += rowH

	// ---- Section: Toggle / Selection ----
	sectionLabel(form, "Toggle / Selection", col1, y)
	y += 28

	// CheckBox
	chk := gui.NewCheckBox()
	chk.SetText("Enable notifications")
	chk.SetParent(form)
	chk.SetBounds(col1, y, w1, 22)
	y += rowH

	// RadioButton
	rg := gui.NewRadioGroup()
	rb := gui.NewRadioButton("Dark mode", rg)
	rb.SetParent(form)
	rb.SetBounds(col1, y, w1, 22)
	y += rowH

	// ToggleSwitch
	ts := gui.NewToggleSwitch()
	ts.SetText("Wi-Fi")
	ts.SetParent(form)
	ts.SetBounds(col1, y, w1, 24)
	ts.SetChecked(true)
	y += rowH

	// SwitchGroup (segmented control)
	sg := gui.NewSwitchGroup()
	sg.SetItems([]string{"Day", "Week", "Month"})
	sg.SetParent(form)
	sg.SetBounds(col1, y, w1, 30)
	y += rowH + 4

	// DropdownButton
	dd := gui.NewDropdownButton()
	dd.SetText("Select Language")
	dd.AddItem("Go", nil, nil)
	dd.AddItem("Rust", nil, nil)
	dd.AddItem("Python", nil, nil)
	dd.SetParent(form)
	dd.SetBounds(col1, y, w1, 30)
	y += rowH + 4

	// Rating
	rt := gui.NewRating()
	rt.SetParent(form)
	rt.SetBounds(col1, y, 140, 24)
	rt.SetValue(3)
	y += rowH

	// ---- Section: Display Widgets (right column) ----
	ry := 10.0
	sectionLabel(form, "Display Widgets", col2, ry)
	ry += 28

	// Label
	info := gui.NewLabel("Status: Ready")
	info.SetParent(form)
	info.SetBounds(col2, ry, w1, 22)
	ry += rowH

	// ProgressBar
	lbl(form, "Progress:", col2, ry)
	pb := gui.NewProgressBar()
	pb.SetParent(form)
	pb.SetBounds(col2+80, ry, w1-80, 18)
	pb.SetValue(65)
	ry += rowH

	// Badge
	badge := gui.NewBadge()
	badge.SetCount(5)
	badge.SetParent(form)
	badge.SetBounds(col2, ry, 50, 20)

	badge2 := gui.NewBadge()
	badge2.SetCount(99)
	badge2.SetParent(form)
	badge2.SetBounds(col2+60, ry, 40, 20)
	ry += rowH

	// Avatar
	av := gui.NewAvatar()
	av.SetText("JD")
	av.SetParent(form)
	av.SetBounds(col2, ry, 36, 36)
	ry += 44

	// Tag
	tag1 := gui.NewTag("Go")
	tag1.SetParent(form)
	tag1.SetBounds(col2, ry, 50, 22)

	tag2 := gui.NewTag("Silk")
	tag2.SetParent(form)
	tag2.SetBounds(col2+60, ry, 50, 22)

	tag3 := gui.NewTag("GUI")
	tag3.SetParent(form)
	tag3.SetBounds(col2+120, ry, 50, 22)
	ry += rowH

	// Link
	link := gui.NewLink("Visit Documentation", "https://silk.dev/docs")
	link.SetParent(form)
	link.SetBounds(col2, ry, w1, 22)
	ry += rowH

	// Placeholder
	ph := gui.NewPlaceholder("Drop widget here")
	ph.SetParent(form)
	ph.SetBounds(col2, ry, w1, 50)
	ry += 58

	// ---- Section: Container Widgets ----
	sectionLabel(form, "Container Widgets", col2, ry)
	ry += 28

	// Card
	card := gui.NewCard("User Info")
	card.SetParent(form)
	card.SetBounds(col2, ry, w1, 80)
	cardLbl := gui.NewLabel("Name: Jane Doe")
	card.SetContent(cardLbl)
	ry += 90

	// Accordion
	acc := gui.NewAccordion()
	acc.SetParent(form)
	acc.SetBounds(col2, ry, w1, 100)
	acc.AddSection("General", gui.NewLabel("General settings"))
	acc.AddSection("Advanced", gui.NewLabel("Advanced options"))
	ry += 110

	// GroupBox
	gb := gui.NewGroupBox("Options")
	gb.SetParent(form)
	gb.SetBounds(col2, ry, w1, 60)
	ry += 70

	// ---- Section: Buttons ----
	sectionLabel(form, "Buttons", col1, y)
	y += 28

	btn1 := gui.NewButton1("Primary", nil)
	btn1.SetParent(form)
	btn1.SetBounds(col1, y, 80, 30)

	btn2 := gui.NewButton1("Secondary", nil)
	btn2.SetParent(form)
	btn2.SetBounds(col1+90, y, 80, 30)

	btn3 := gui.NewButton1("Danger", nil)
	btn3.SetParent(form)
	btn3.SetBounds(col1+180, y, 80, 30)
	y += rowH + 4

	// ---- Section: Notification ----
	sectionLabel(form, "Notifications", col1, y)
	y += 28

	np := gui.NewNotificationPanel()
	np.SetParent(form)
	np.SetBounds(col1, y, w1, 120)
	np.AddNotification(gui.NotificationItem{
		Title:   "Build Succeeded",
		Message: "All 42 tests passed",
		Level:   gui.NotifySuccess,
		Time:    "2m ago",
	})
	np.AddNotification(gui.NotificationItem{
		Title:   "Warning",
		Message: "Disk usage at 85%",
		Level:   gui.NotifyWarning,
		Time:    "10m ago",
	})
	np.AddNotification(gui.NotificationItem{
		Title:   "System Update",
		Message: "Version 2.3.0 available",
		Level:   gui.NotifyInfo,
		Time:    "1h ago",
	})
	y += 130

	// ---- Status bar at bottom ----
	statusBar := gui.NewLabel(fmt.Sprintf("Silk Widget Showcase | %d widget types demonstrated", 30))
	statusBar.SetParent(form)
	statusBar.SetBounds(col1, y, 560, 22)

	// Window setup
	winH := y + 50
	if ry+50 > winH {
		winH = ry + 50
	}
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(620, winH)
	form.Window().MoveToCenter()
	form.Show()

	core.EventLoop()

	// Suppress unused warnings
	_ = paint.Color{}
}

// sectionLabel creates a bold section header label.
func sectionLabel(form *gui.Form, text string, x, y float64) {
	lbl := gui.NewLabel(text)
	lbl.SetFont(paint.NewFont(gui.Theme().Font.Family(), 15, true, false))
	lbl.SetParent(form)
	lbl.SetBounds(x, y, 280, 24)
}

// lbl creates a simple text label.
func lbl(form *gui.Form, text string, x, y float64) {
	l := gui.NewLabel(text)
	l.SetParent(form)
	l.SetBounds(x, y+3, 80, 22)
}

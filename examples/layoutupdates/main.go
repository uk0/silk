// Layout updates demonstrates event-driven preferred-size changes. No timer,
// polling worker, filesystem or network access is needed for ordinary UI edits.
package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type layoutForm struct {
	gui.Form
	body *gui.VBox
}

func (f *layoutForm) Layout() {
	if f.body != nil {
		f.body.SetBounds(18, 18, max(0, f.Width()-36), max(0, f.Height()-36))
	}
}

func main() {
	form := new(layoutForm)
	form.Init(form)
	form.SetTitle("Silk SDK - Event-driven layout updates")
	form.body = gui.NewVBox()
	form.body.SetSpacing(10)
	form.body.SetParent(form)

	title := gui.NewLabel("Live layout updates")
	title.SetFont(paint.NewFont(gui.Theme().Font.Family(), 22, true, false))
	form.body.AddWidget(title)
	help := gui.NewLabel("Change text, font, spacing or visibility. The following controls move automatically; there is no polling timer or manual Layout call in these actions.")
	help.SetWrap(true)
	form.body.AddWidget(help)

	group := gui.NewGroupBox("Runtime status")
	inner := gui.NewVBox()
	inner.SetSpacing(4)
	group.SetContent(inner)
	const short = "Ready. A short status fits on one row."
	const long = "A longer runtime status wraps over several visual rows. 中文状态也会自动换行。 " +
		"The nested group grows to fit the actual available width, and the controls below remain outside its border. " +
		"Return to the short status to release the extra space."
	details := gui.NewLabel(short)
	details.SetWrap(true)
	inner.AddWidget(details)
	extra := gui.NewLabel("Optional detail: visibility also changes preferred size.")
	extra.SetWrap(true)
	inner.AddWidget(extra)
	form.body.AddWidget(group)

	below := gui.NewLabel("This is a separate row below the status group.")
	below.SetWrap(true)
	form.body.AddWidget(below)
	actions := gui.NewFlexWrap()
	actions.SetSpacing(8)
	actions.SetRowGap(8)
	form.body.AddWidget(actions)
	addAction := func(text string, action func()) {
		button := gui.NewButton1(text, nil)
		button.Action().BindFunc0(action)
		actions.AddWidget(button)
	}
	addAction("Long status", func() { details.SetText(long) })
	addAction("Short status", func() { details.SetText(short) })
	large := false
	addAction("Toggle font", func() {
		large = !large
		if large {
			details.SetFont(paint.NewFont(gui.Theme().Font.Family(), 19, false, false))
		} else {
			details.SetFont(nil)
		}
	})
	addAction("Toggle detail", func() { extra.SetVisible(!extra.IsVisible()) })
	spaced := false
	addAction("Toggle spacing", func() {
		spaced = !spaced
		if spaced {
			inner.SetSpacing(18)
		} else {
			inner.SetSpacing(4)
		}
	})

	form.SetSize(640, 560)
	form.AttachWindow(gui.WtForm)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

// A hardware-free HMI screen. All values are local simulation, never PLC writes.
package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scada"
)

func main() {
	defer core.Close()
	gui.SetThemeMode(gui.ThemeDark)
	paint.SetPreferVectorIcons(true)
	paint.SetPreferSatinIcons(true)
	frame := gui.NewFrameWindow()
	frame.SetTitle("Silk Satin HMI - local simulation")
	gui.SetDefaultFrame(frame)
	form := gui.NewForm()
	form.SetTitle("Satin controls")
	title := gui.NewLabel("Satin HMI / Local simulation only")
	title.SetParent(form)
	title.SetBounds(20, 12, 650, 28)
	state := gui.NewSpriteView()
	state.SetTagName("running")
	state.SetParent(form)
	state.SetBounds(24, 58, 128, 128)
	value := gui.NewProcessValue()
	value.SetTitle("Pressure")
	value.SetUnit("bar")
	value.SetMax(120)
	value.SetAlarmAbove(80)
	value.SetTagName("pressure")
	value.SetParent(form)
	value.SetBounds(180, 65, 260, 114)
	gauge := gui.NewGauge()
	gauge.SetTitle("Pressure")
	gauge.SetUnit("bar")
	gauge.SetRange(0, 120)
	gauge.SetTagName("pressure")
	gauge.SetParent(form)
	gauge.SetBounds(460, 58, 220, 155)
	// Only tag-bound, read-only visual controls are used: no historian or device
	// services are needed. Real applications should use scada.New for full services.
	services := &scada.Services{Tags: core.NewTagDB()}
	running := services.Tags.GetOrCreate("running", core.Meta{})
	pressure := services.Tags.GetOrCreate("pressure", core.Meta{})
	running.SetValue(false)
	pressure.SetValue(42.5)
	stop, err := scada.BindScreen(services, form, scada.ScreenOptions{})
	if err != nil {
		panic(err)
	}
	defer stop()
	add := func(text, icon string, x float64, fn func()) {
		b := gui.NewButton1(text, paint.LoadIcon(icon))
		b.SetTextVisible(true)
		b.SetParent(form)
		b.SetBounds(x, 238, 150, 36)
		b.Action().BindFunc0(fn)
	}
	add("Toggle valve", "run", 20, func() { running.SetValue(!running.Value().Bool()) })
	add("Normal: 42.5", "save", 188, func() { pressure.SetValue(42.5) })
	add("Alarm: 95", "warning", 356, func() { pressure.SetValue(95.0) })
	add("Light / Dark", "settings", 524, func() {
		if gui.CurrentThemeMode() == gui.ThemeDark {
			gui.SetThemeMode(gui.ThemeLight)
		} else {
			gui.SetThemeMode(gui.ThemeDark)
		}
		frame.Update()
	})
	note := gui.NewLabel("No idle timer. Tag updates are coalesced; closing cancels pending animations.")
	note.SetParent(form)
	note.SetBounds(20, 298, 660, 28)
	frame.SuggestDocDock().AddView(form)
	frame.SetClosedCallback(func(*gui.Frame) { stop(); core.Quit() })
	frame.Window().SetSize(730, 420)
	frame.Show()
	frame.Window().SetIcon(paint.LoadIcon("satin:gauge"))
	core.EventLoop()
}

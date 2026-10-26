// Timer lifecycle demonstrates cancellable UI delays and repeating callbacks
// that open a modal dialog. All state changes stay on the UI thread.
package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type timerState struct {
	ModalCalls, Depth, MaxDepth, Heartbeats      int
	DelayGeneration, DelayFires, FiredGeneration int
	ModalOpen, DelayArmed                        bool
}
type timerDemo struct {
	gui.Form
	state                        timerState
	modalTimer, heartbeat, delay gui.Timer
	status, modalStatus          *gui.Label
	widgets                      []gui.IWidget
}

func newTimerDemo() *timerDemo {
	f := new(timerDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Timer Lifecycle")
	add := func(w gui.IWidget) { w.SetParent(f); f.widgets = append(f.widgets, w) }
	add(gui.NewLabel("UI timers: modal reentry, restartable delay and cancellation."))
	add(gui.NewLabel("Open the modal and leave it open: callback depth must stay at 1."))
	f.status = gui.NewLabel("")
	add(f.status)
	button := func(text string, fn func()) { b := gui.NewButton1(text, nil); b.Action().BindFunc0(fn); add(b) }
	button("Open timer modal", f.armModal)
	button("Restart 2 second delay", f.restartDelay)
	button("Cancel delay", func() { f.delay.Stop(); f.state.DelayArmed = false; f.refresh(); f.snapshot() })
	button("Snapshot", f.snapshot)
	f.refresh()
	return f
}
func (f *timerDemo) stopTimers() { f.modalTimer.Stop(); f.heartbeat.Stop(); f.delay.Stop() }
func (f *timerDemo) armModal() {
	if f.state.ModalOpen {
		return
	}
	f.state.ModalCalls, f.state.Depth, f.state.MaxDepth, f.state.Heartbeats = 0, 0, 0, 0
	f.modalTimer.Start(250, f.modalTick)
	f.refresh()
}
func (f *timerDemo) modalTick() {
	f.state.ModalCalls++
	f.state.Depth++
	f.state.MaxDepth = max(f.state.MaxDepth, f.state.Depth)
	defer func() { f.state.Depth--; f.refresh() }()
	if f.state.ModalOpen {
		f.refresh()
		f.snapshot()
		return
	} // keep regression bounded
	f.state.ModalOpen = true
	d := gui.NewDialog("Timer callback is still active", f)
	f.modalStatus = gui.NewLabel("")
	d.SetContent(f.modalStatus)
	d.SetSize(540, 180)
	d.AddButton("Close", gui.DialogCancel)
	f.heartbeat.Start(100, func() { f.state.Heartbeats++; f.refresh() })
	f.refresh()
	f.snapshot()
	d.ShowModal()
	f.modalTimer.Stop()
	f.heartbeat.Stop()
	f.modalStatus = nil
	f.state.ModalOpen = false
	f.refresh()
	f.snapshot()
}
func (f *timerDemo) cancelDelay() {
	f.delay.Stop()
	f.state.DelayArmed = false
	f.refresh()
	f.snapshot()
}
func (f *timerDemo) restartDelay() {
	f.state.DelayGeneration++
	generation := f.state.DelayGeneration
	f.state.DelayArmed = true
	f.delay.Start(2000, func() {
		f.delay.Stop()
		f.state.DelayArmed = false
		f.state.DelayFires++
		f.state.FiredGeneration = generation
		f.refresh()
		f.snapshot()
	})
	f.refresh()
	f.snapshot()
}
func (f *timerDemo) refresh() {
	f.status.SetText(fmt.Sprintf("Modal calls: %d | max depth: %d | delay: %d fired: %d", f.state.ModalCalls, f.state.MaxDepth, f.state.DelayGeneration, f.state.DelayFires))
	if f.modalStatus != nil {
		f.modalStatus.SetText(fmt.Sprintf("Calls: %d | depth: %d | heartbeats: %d", f.state.ModalCalls, f.state.MaxDepth, f.state.Heartbeats))
	}
}
func (f *timerDemo) snapshot() {
	data, _ := json.Marshal(f.state)
	fmt.Println("SNAPSHOT " + string(data))
}
func (f *timerDemo) OnResize() {
	w, h := f.Size()
	w, h = math.Max(0, w), math.Max(0, h)
	for i, c := range f.widgets {
		y := 16 + float64(i)*44
		c.SetBounds(math.Min(16, w), math.Min(y, h), math.Max(0, w-32), math.Max(0, math.Min(36, h-y)))
	}
}
func main() {
	defer core.Close()
	f := newTimerDemo()
	defer f.stopTimers()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(720, 350)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}

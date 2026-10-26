// UI dispatch demonstrates worker-to-UI typed calls, cancellable posts and
// coalesced progress without polling. Run with `go run ./examples/ui-dispatch`.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type dispatchState struct {
	Inline, Worker, Cancelled, Unexpected, Last, Delivered, Rejected, Modal int
	OnUI                                                                    bool
	Error                                                                   string
}
type dispatchDemo struct {
	gui.Form
	ctx      context.Context
	cancel   context.CancelFunc
	progress *async.Latest[int]
	state    dispatchState
	widgets  []gui.IWidget
	status   *gui.Label
}

func newDispatchDemo() *dispatchDemo {
	f := new(dispatchDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - SDK UI Dispatch")
	f.ctx, f.cancel = context.WithCancel(context.Background())
	f.state.Last = -1
	add := func(w gui.IWidget) { w.SetParent(f); f.widgets = append(f.widgets, w) }
	add(gui.NewLabel("Typed UI calls, cancellation and bounded progress delivery"))
	add(gui.NewLabel("Workers never mutate controls. No polling timer is required."))
	f.status = gui.NewLabel("")
	add(f.status)
	button := func(text, icon string, fn func()) {
		b := gui.NewButton1(text, gui.LoadIcon(icon))
		b.SetTextVisible(true)
		b.Action().BindFunc0(fn)
		add(b)
	}
	button("Inline typed call", "edit", func() {
		value, err := gui.InvokeValue(f.ctx, func() (int, error) { return f.state.Inline + 1, nil })
		f.check(err)
		if err == nil {
			f.state.Inline = value
		}
		f.refresh()
	})
	button("Worker typed call", "play", f.worker)
	button("Cancel queued update", "close", func() {
		ctx, cancel := context.WithCancel(f.ctx)
		_ = gui.PostContext(ctx, func() { f.state.Unexpected++; f.refresh() })
		cancel()
		f.state.Cancelled++
		_ = gui.PostContext(f.ctx, f.refresh)
	})
	button("Publish 100000 snapshots", "refresh", func() {
		progress, ctx := f.progress, f.ctx
		go func() {
			for i := 0; i < 100000; i++ {
				if ctx.Err() != nil || !progress.Publish(i) {
					return
				}
			}
		}()
	})
	button("Reject worker window creation", "warning", f.rejectWorker)
	button("Open modal through worker", "file", f.modalWorker)
	button("Snapshot", "log", f.snapshot)
	f.progress, _ = gui.NewLatestUI(func(v int) { f.state.Last = v; f.state.Delivered++; f.refresh() })
	f.refresh()
	return f
}
func (f *dispatchDemo) check(err error) {
	if err != nil {
		f.state.Error = err.Error()
	}
}
func (f *dispatchDemo) worker() {
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	go func() {
		defer cancel()
		value, err := gui.InvokeValue(ctx, func() (int, error) { f.state.Worker++; f.refresh(); return f.state.Worker, nil })
		_ = gui.PostContext(f.ctx, func() {
			f.check(err)
			if err == nil && value <= 0 {
				f.state.Error = "missing typed result"
			}
			f.refresh()
		})
	}()
}
func (f *dispatchDemo) rejectWorker() {
	target := gui.NewForm()
	ctx := f.ctx
	go func() {
		err := target.TryAttachWindow(gui.WtForm)
		_ = gui.PostContext(ctx, func() {
			if errors.Is(err, gui.ErrUIThread) {
				f.state.Rejected++
			} else {
				f.state.Error = fmt.Sprint("unexpected attach result: ", err)
			}
			f.refresh()
		})
	}()
}
func (f *dispatchDemo) modalWorker() {
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Minute)
	go func() {
		defer cancel()
		err := gui.Invoke(ctx, func() error {
			d := gui.NewDialog("Worker requested UI modal", f)
			d.SetContent(gui.NewLabel("The modal runs on the UI thread; its owner stays attached."))
			d.AddButton("Close", gui.DialogCancel)
			d.SetSize(560, 160)
			d.ShowModal()
			f.state.Modal++
			f.refresh()
			return d.LastError()
		})
		_ = gui.PostContext(f.ctx, func() { f.check(err); f.refresh() })
	}()
}
func (f *dispatchDemo) refresh() {
	f.state.OnUI = gui.IsUIThread()
	f.status.SetText(fmt.Sprintf("Inline %d | Worker %d | Cancel %d/%d | Last %d | deliveries %d | rejected %d | modal %d", f.state.Inline, f.state.Worker, f.state.Cancelled, f.state.Unexpected, f.state.Last, f.state.Delivered, f.state.Rejected, f.state.Modal))
	f.snapshot()
}
func (f *dispatchDemo) snapshot() {
	b, _ := json.Marshal(f.state)
	fmt.Println("SNAPSHOT " + string(b))
}
func (f *dispatchDemo) stop() { f.cancel(); f.progress.Close() }
func (f *dispatchDemo) OnResize() {
	w, h := f.Size()
	w, h = max(0, w), max(0, h)
	for i, c := range f.widgets {
		y := 16 + float64(i)*42
		c.SetBounds(min(16, w), min(y, h), max(0, w-32), max(0, min(34, h-y)))
	}
}
func main() {
	defer core.Close()
	gui.SetAppIcon(gui.LoadIcon("document"))
	f := newDispatchDemo()
	defer f.stop()
	if err := f.TryAttachWindow(gui.WtForm); err != nil {
		panic(err)
	}
	f.Window().SetSize(920, 480)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}

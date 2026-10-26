// Drag feedback demonstrates all four actions and packaged cursor artwork.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"os"
	"time"
)

type dragDemo struct {
	gui.Form
	source                 *dragTile
	targets                []*dragTile
	status                 *gui.Label
	snapshot               *gui.Button
	drops                  int
	lastAction             gui.DndAction
	result                 gui.DndAction
	lastData               string
	dragging               bool
	probeTicks, probePosts int
}
type dragTile struct {
	gui.Widget
	host         *dragDemo
	title        string
	action       gui.DndAction
	source, over bool
	icon         gui.CursorData
	paints       int
}

func (t *dragTile) Draw(p paint.Painter) {
	t.paints++
	color := paint.Color{R: 236, G: 242, B: 250, A: 255}
	if t.over {
		color = paint.Color{R: 192, G: 218, B: 255, A: 255}
	}
	p.SetBrush1(color)
	p.Rectangle(0, 0, t.Width(), t.Height())
	p.FillPreserve()
	p.SetPen1(paint.Color{R: 96, G: 119, B: 150, A: 255}, 1)
	p.Stroke()
	if t.icon.Pixmap != nil {
		p.DrawPixmap1(12, min(12, max(0, t.Height()-32)), t.icon.Pixmap)
	}
	p.SetFont(gui.Theme().Font)
	p.SetBrush1(paint.Color{R: 30, G: 44, B: 65, A: 255})
	p.DrawText1(12, min(75, max(0, t.Height()-12)), t.title)
}
func (t *dragTile) OnLeftDown(x, y float64) {
	if !t.source {
		return
	}
	t.PopCapture()
	pix := paint.NewPixmap(64, 24)
	p := pix.NewPainter()
	p.SetBrush1(paint.Color{R: 26, G: 117, B: 224, A: 255})
	p.Rectangle(0, 0, 64, 24)
	p.Fill()
	paint.ClosePainter(p)
	defer paint.CloseSurface(pix)
	fmt.Println("DRAG_BEGIN")
	t.host.dragging = true
	t.host.result = t.DoDragDrop(pix, gui.DndMove|gui.DndCopy|gui.DndLink, "Silk drag payload")
	t.host.dragging = false
	t.host.refresh()
	fmt.Println("DRAG_END", t.host.result)
}
func (t *dragTile) OnDragEnter(x, y float64, d gui.IDndContext) { t.OnDragMove(x, y, d) }
func (t *dragTile) OnDragMove(x, y float64, d gui.IDndContext) {
	action := t.action
	if t.source || !d.HasFormat("text/plain") {
		action = gui.DndIgnore
	}
	d.SetAction(action)
	over := action != gui.DndIgnore
	if t.over != over {
		t.over = over
		t.Update()
	}
}
func (t *dragTile) OnDrop(x, y float64, d gui.IDndContext) {
	t.OnDragMove(x, y, d)
	if d.Action() == gui.DndIgnore {
		return
	}
	t.host.drops++
	t.host.lastAction = d.Action()
	t.host.lastData, _ = d.Data("text/plain").(string)
	t.host.refresh()
}
func (t *dragTile) OnDragLeave() { t.over = false; t.Update() }
func (f *dragDemo) refresh() {
	f.status.SetText(fmt.Sprintf("Drops: %d  Action: %d  Result: %d  Payload: %s", f.drops, f.lastAction, f.result, f.lastData))
}
func (f *dragDemo) state() interface{} {
	return struct {
		Drops          int
		Action, Result gui.DndAction
		Data           string
	}{f.drops, f.lastAction, f.result, f.lastData}
}
func newDragDemo() *dragDemo {
	f := new(dragDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Drag Feedback")
	tile := func(title, name string, action gui.DndAction, source bool) *dragTile {
		t := &dragTile{host: f, title: title, action: action, source: source}
		t.Init(t)
		t.SetParent(f)
		t.icon, _ = gui.LoadCursorData(name)
		return t
	}
	f.source = tile("Press and drag from here", "arrow", gui.DndIgnore, true)
	for _, spec := range []struct {
		title, name string
		action      gui.DndAction
	}{{"Not allowed", "arrow-no", gui.DndIgnore}, {"Move", "arrow", gui.DndMove}, {"Copy", "arrow-copy", gui.DndCopy}, {"Link", "arrow-link", gui.DndLink}} {
		f.targets = append(f.targets, tile(spec.title, spec.name, spec.action, false))
	}
	f.status = gui.NewLabel("")
	f.status.SetParent(f)
	f.snapshot = gui.NewButton1("Snapshot", paint.LoadIcon("log"))
	f.snapshot.SetTextVisible(true)
	f.snapshot.SetParent(f)
	f.snapshot.Action().BindFunc0(func() { b, _ := json.Marshal(f.state()); fmt.Println("SNAPSHOT " + string(b)) })
	f.refresh()
	return f
}
func (f *dragDemo) OnResize() {
	if f.source == nil {
		return
	}
	w, h := max(0, f.Width()), max(0, f.Height())
	margin := min(20.0, w/8, h/12)
	inner := max(0, w-2*margin)
	buttonH := min(32.0, h/8)
	statusH := min(28.0, h/8)
	rowH := max(0, (h-5*margin-buttonH-statusH)/2)
	f.source.SetBounds(margin, margin, inner, rowH)
	gap := min(8.0, inner/16)
	cw := max(0, (inner-3*gap)/4)
	for i, t := range f.targets {
		t.SetBounds(margin+float64(i)*(cw+gap), 2*margin+rowH, cw, rowH)
	}
	f.status.SetBounds(margin, 3*margin+2*rowH, inner, statusH)
	f.snapshot.SetBounds(margin, 4*margin+2*rowH+statusH, min(140.0, inner), buttonH)
}
func (f *dragDemo) closeAssets() {
	paint.CloseSurface(f.source.icon.Pixmap)
	for _, t := range f.targets {
		paint.CloseSurface(t.icon.Pixmap)
	}
}

// Opt-in native-loop probe. Counters are mutated only on the UI thread, and
// remain observable while DoDragDrop has not returned. Normal demos stay idle.
func (f *dragDemo) startProgressProbe() func() {
	if os.Getenv("SILK_DRAG_PUMP_PROBE") != "1" {
		return func() {}
	}
	var timer gui.Timer
	closed := false
	timer.Start(250, func() {
		f.probeTicks++
		paints := make([]int, len(f.targets))
		for i, t := range f.targets {
			paints[i] = t.paints
		}
		b, _ := json.Marshal(struct {
			Timer, Posted int
			Dragging      bool
			Paints        []int
		}{f.probeTicks, f.probePosts, f.dragging, paints})
		fmt.Println("PUMP " + string(b))
	})
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				gui.Post(func() {
					if !closed {
						f.probePosts++
					}
				})
			}
		}
	}()
	return func() { closed = true; timer.Stop(); close(stop); <-done }
}
func main() {
	defer core.Close()
	f := newDragDemo()
	defer f.closeAssets()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(760, 370)
	f.Window().MoveToCenter()
	f.Show()
	defer f.startProgressProbe()()
	core.EventLoop()
}

// A local synthetic HTTP -> typed SQLite point -> HMI demonstration. No field I/O.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

type demo struct {
	gui.Form
	equipment []*gui.ProcessEquipment
	values    []*gui.ProcessValue
	buttons   []*gui.Button
	status    *gui.Label
	tags      *core.TagDB
	mode      atomic.Int32
	cancels   []func()
	cancel    context.CancelFunc
	done      chan error
	store     *pointsql.Store
	server    *httptest.Server
}

func newDemo() *demo {
	f := new(demo)
	f.Form.Init(f)
	f.SetTitle("Silk - 2D Process Points QA")
	f.tags = core.NewTagDB()
	f.done = make(chan error, 1)
	var e error
	f.store, e = pointsql.Open(filepath.Join(core.LocalDataDir(), "process-points.db"))
	if e != nil {
		panic(e)
	}
	for _, d := range []point.Definition{{Name: "AI101", Kind: point.Float, Category: "AI", Unit: "°C"}, {Name: "DI11", Kind: point.Bool, Category: "DI"}} {
		if e = f.store.Define(context.Background(), d); e != nil {
			panic(e)
		}
	}
	for i, kind := range []string{"reactor", "exchanger", "separator"} {
		w := gui.NewProcessEquipment()
		w.SetKind(kind)
		w.SetLabel([]string{"R-001", "E-001", "V-001"}[i])
		w.SetParent(f)
		w.SetTagName("DI11")
		f.equipment = append(f.equipment, w)
		f.cancels = append(f.cancels, gui.BindTagUI(f.tags.GetOrCreate("DI11", core.Meta{}), w.SetSample))
	}
	for _, title := range []string{"Reactor temperature", "Outlet temperature", "Shared point display"} {
		w := gui.NewProcessValue()
		w.SetParent(f)
		w.SetTitle(title)
		w.SetUnit("°C")
		w.SetMax(150)
		w.SetTransformJSON(`[{"type":"float"},{"expr":"x*0.1"}]`)
		w.SetColorRulesJSON(`[{"when":"x>=100","color":"#ef5350"},{"when":"x>=80","color":"#ffb300"},{"when":"true","color":"#26a269"}]`)
		w.SetTagName("AI101")
		f.values = append(f.values, w)
		f.cancels = append(f.cancels, gui.BindTagUI(f.tags.GetOrCreate("AI101", core.Meta{}), w.SetSample))
	}
	add := func(text string, fn func()) {
		b := gui.NewButton1(text, nil)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	for i, label := range []string{"Normal 72.5", "Warning 92.8", "Alarm 120.0", "HTTP offline", "Malformed value"} {
		mode := int32(i)
		add(label, func() { f.mode.Store(mode) })
	}
	add("Snapshot", f.snapshot)
	f.status = gui.NewLabel("Local synthetic source · one HTTP response feeds all displays · no field hardware")
	f.status.SetParent(f)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["device"] != "R-001" {
			w.WriteHeader(400)
			return
		}
		mode := f.mode.Load()
		if mode == 3 {
			w.WriteHeader(503)
			return
		}
		raw := []string{"725", "928", "1200", "0", "invalid"}[mode]
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"temperature": raw, "running": mode != 3}})
	}))
	req := httpx.Request{URL: f.server.URL, Method: "POST", Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"device":"R-001"}`)}
	source, e := point.NewHTTPSource(nil, req, []point.Mapping{{Name: "AI101", Pointer: "/data/temperature", Kind: point.Float}, {Name: "DI11", Pointer: "/data/running", Kind: point.Bool}}, f.tags, f.store)
	if e != nil {
		panic(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() { f.done <- source.Run(ctx, time.Second, true, nil) }()
	return f
}
func (f *demo) snapshot() {
	state := map[string]any{"mode": f.mode.Load(), "on_ui": gui.IsUIThread(), "text": f.values[0].DisplayText(), "color": f.values[0].DisplayColor(), "quality": f.values[0].SampleQuality(), "running": f.equipment[0].IsRunning()}
	if e := f.values[0].BindingError(); e != nil {
		state["error"] = e.Error()
	}
	b, _ := json.Marshal(state)
	fmt.Println("PROCESS_STATE", string(b))
	f.status.SetText(string(b))
}
func (f *demo) OnResize() {
	w, h := f.Size()
	columns := max(1, min(len(f.buttons), int(max(0, w-24)/160)))
	rows := (len(f.buttons) + columns - 1) / columns
	bw := (w - 24) / float64(columns)
	for i, b := range f.buttons {
		b.SetBounds(12+float64(i%columns)*bw, 12+float64(i/columns)*38, max(0, bw-6), 32)
	}
	top := 68 + float64(rows-1)*38
	col := max(0, (w-72)/3)
	for i, item := range f.equipment {
		item.SetBounds(24+float64(i)*(col+12), top, col, 200)
	}
	for i, item := range f.values {
		item.SetBounds(24+float64(i)*(col+12), top+218, col, 110)
	}
	f.status.SetBounds(12, h-46, w-24, 34)
}
func main() {
	defer core.Close()
	f := newDemo()
	defer func() {
		f.cancel()
		<-f.done
		for _, stop := range f.cancels {
			stop()
		}
		f.server.Close()
		f.store.Close()
	}()
	if e := f.TryAttachWindow(gui.WtForm); e != nil {
		panic(e)
	}
	f.Window().SetSize(1000, 510)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}
